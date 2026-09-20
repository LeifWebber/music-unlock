# Windows PowerShell 5.1+ / PowerShell 7. No administrator rights required.
param(
    [string]$Version = 'latest',
    [string]$BinDir,
    [switch]$Force,
    [switch]$Run,
    [switch]$NoPath,
    [string[]]$Arguments = @()
)

function Initialize-UnmusNative {
    if (-not ('Unmus.Installer.Native' -as [type])) {
        Add-Type -TypeDefinition @'
using System;
using System.Runtime.InteropServices;
namespace Unmus.Installer {
    public static class Native {
        [DllImport("kernel32.dll", SetLastError=true)]
        public static extern bool IsWow64Process2(IntPtr process, out ushort processMachine, out ushort nativeMachine);
        [DllImport("user32.dll", CharSet=CharSet.Unicode, SetLastError=true)]
        public static extern IntPtr SendMessageTimeout(IntPtr window, uint message, IntPtr wParam, string lParam, uint flags, uint timeout, out IntPtr result);
    }
}
'@
    }
}

function Get-UnmusArchitecture {
    Initialize-UnmusNative
    [System.UInt16]$processMachine = 0
    [System.UInt16]$nativeMachine = 0
    try {
        if ([Unmus.Installer.Native]::IsWow64Process2([IntPtr](-1), [ref]$processMachine, [ref]$nativeMachine)) {
            switch ($nativeMachine) {
                0x8664 { return 'amd64' }
                0xaa64 { return 'arm64' }
                default { throw "Unsupported Windows architecture: $nativeMachine" }
            }
        }
    } catch [System.EntryPointNotFoundException] {
        # Older Windows 10 builds may lack IsWow64Process2.
    }
    $architecture = $env:PROCESSOR_ARCHITEW6432
    if (-not $architecture) { $architecture = $env:PROCESSOR_ARCHITECTURE }
    switch ($architecture) {
        'AMD64' { return 'amd64' }
        'ARM64' { return 'arm64' }
        default { throw "Unsupported Windows architecture: $architecture" }
    }
}

function Get-UnmusReleaseVersion {
    $release = Invoke-RestMethod -Uri 'https://api.github.com/repos/LeifWebber/music-unlock/releases/latest' -UseBasicParsing -ErrorAction Stop
    return $release.tag_name
}

function Save-UnmusDownload([string]$Uri, [string]$Destination) {
    Invoke-WebRequest -Uri $Uri -OutFile $Destination -UseBasicParsing -ErrorAction Stop
}

function Test-UnmusPathEntry([string]$PathValue, [string]$Directory) {
    foreach ($entry in ($PathValue -split ';')) {
        $expanded = [Environment]::ExpandEnvironmentVariables($entry.Trim().Trim('"')).TrimEnd('\', '/')
        if ($expanded -ieq $Directory.TrimEnd('\', '/')) { return $true }
    }
    return $false
}

function Add-UnmusUserPath([string]$Directory) {
    $key = [Microsoft.Win32.Registry]::CurrentUser.CreateSubKey('Environment')
    try {
        $previous = [string]$key.GetValue('Path', '', [Microsoft.Win32.RegistryValueOptions]::DoNotExpandEnvironmentNames)
        if (-not (Test-UnmusPathEntry $previous $Directory)) {
            $kind = [Microsoft.Win32.RegistryValueKind]::ExpandString
            if ($key.GetValueNames() -contains 'Path') { $kind = $key.GetValueKind('Path') }
            $updated = $Directory
            if ($previous) { $updated = $previous.TrimEnd(';') + ';' + $Directory }
            $key.SetValue('Path', $updated, $kind)
            Initialize-UnmusNative
            $result = [IntPtr]::Zero
            [void][Unmus.Installer.Native]::SendMessageTimeout([IntPtr]0xffff, 0x1a, [IntPtr]::Zero, 'Environment', 2, 1000, [ref]$result)
        }
    } finally { $key.Dispose() }
    if (-not (Test-UnmusPathEntry $env:PATH $Directory)) { $env:PATH = $Directory + ';' + $env:PATH }
}

function Install-Unmus {
    param([string]$Version = 'latest', [string]$BinDir, [switch]$Force, [switch]$Run, [switch]$NoPath, [string[]]$Arguments = @())
    $ErrorActionPreference = 'Stop'
    $ProgressPreference = 'SilentlyContinue'
    $PSNativeCommandUseErrorActionPreference = $false
    if ([Environment]::OSVersion.Platform -ne [PlatformID]::Win32NT) { throw 'Use install.sh on macOS/Linux.' }
    $originalTls = [Net.ServicePointManager]::SecurityProtocol
    $temporary = $null
    $staged = $null
    try {
        [Net.ServicePointManager]::SecurityProtocol = $originalTls -bor [Net.SecurityProtocolType]::Tls12
        $architecture = Get-UnmusArchitecture
        if ($Version -eq 'latest') { $Version = Get-UnmusReleaseVersion }
        if ($Version -notmatch '^v[0-9]+\.[0-9]+\.[0-9]+(?:[-+][A-Za-z0-9.-]+)?$') { throw 'Invalid release version (expected vX.Y.Z).' }
        if (-not $Run) {
            if (-not $BinDir) {
                $localData = [Environment]::GetFolderPath('LocalApplicationData')
                if (-not $localData) { throw 'Cannot locate LocalAppData; specify -BinDir.' }
                $BinDir = Join-Path $localData 'Programs\unmus'
            }
            $BinDir = $ExecutionContext.SessionState.Path.GetUnresolvedProviderPathFromPSPath($BinDir)
            $destination = Join-Path $BinDir 'unmus.exe'
            if (Test-Path -LiteralPath $destination) {
                $existing = Get-Item -LiteralPath $destination -Force
                if (-not $Force -or $existing.PSIsContainer -or ($existing.Attributes -band [IO.FileAttributes]::ReparsePoint)) {
                    throw "Refusing to overwrite $destination. Use -BinDir or -Force for an existing regular file."
                }
            }
        }
        $temporary = Join-Path ([IO.Path]::GetTempPath()) ('unmus-' + [Guid]::NewGuid().ToString('N'))
        [void][IO.Directory]::CreateDirectory($temporary)
        $name = "music-unlock-$Version-windows-$architecture"
        $archiveName = "$name.zip"
        $baseUrl = "https://github.com/LeifWebber/music-unlock/releases/download/$Version"
        $archivePath = Join-Path $temporary $archiveName
        $checksumPath = Join-Path $temporary 'SHA256SUMS'
        Save-UnmusDownload "$baseUrl/$archiveName" $archivePath
        Save-UnmusDownload "$baseUrl/SHA256SUMS" $checksumPath
        $digests = @()
        foreach ($line in (Get-Content -LiteralPath $checksumPath)) {
            if ($line -match ('^([0-9a-fA-F]{64})\s+\*?' + [regex]::Escape($archiveName) + '$')) { $digests += $Matches[1] }
        }
        if ($digests.Count -ne 1) { throw 'Missing or ambiguous SHA256 checksum.' }
        if ((Get-FileHash -LiteralPath $archivePath -Algorithm SHA256).Hash -ine $digests[0]) { throw 'SHA256 verification failed.' }
        Add-Type -AssemblyName System.IO.Compression, System.IO.Compression.FileSystem
        $archive = [IO.Compression.ZipFile]::OpenRead($archivePath)
        try {
            $entries = @($archive.Entries | Where-Object { $_.FullName -ceq "$name/unmus.exe" })
            if ($entries.Count -ne 1 -or $entries[0].Length -le 0 -or $entries[0].Length -gt 128MB) { throw 'Invalid executable in archive.' }
            $executable = Join-Path $temporary 'unmus.exe'
            $inputStream = $entries[0].Open()
            try {
                $outputStream = [IO.File]::Open($executable, [IO.FileMode]::CreateNew)
                try { $inputStream.CopyTo($outputStream) } finally { $outputStream.Dispose() }
            } finally { $inputStream.Dispose() }
        } finally { $archive.Dispose() }
        if ($Run) {
            & $executable @Arguments
            $global:LASTEXITCODE = $LASTEXITCODE
        } else {
            [void][IO.Directory]::CreateDirectory($BinDir)
            $staged = Join-Path $BinDir ('.unmus-' + [Guid]::NewGuid().ToString('N') + '.exe')
            [IO.File]::Copy($executable, $staged, $false)
            if ($Force -and (Test-Path -LiteralPath $destination)) {
                $existing = Get-Item -LiteralPath $destination -Force
                if ($existing.PSIsContainer -or ($existing.Attributes -band [IO.FileAttributes]::ReparsePoint)) { throw 'Refusing to replace a directory or link.' }
                # PowerShell converts $null to an empty string for string parameters.
                [IO.File]::Replace($staged, $destination, [System.Management.Automation.Language.NullString]::Value)
            } else { [IO.File]::Move($staged, $destination) }
            $staged = $null
            if (-not $NoPath) { Add-UnmusUserPath $BinDir }
            Write-Host "Installed: $destination"
            if (-not $NoPath) { Write-Host 'Open a new terminal and run unmus.' }
            $global:LASTEXITCODE = 0
        }
    } finally {
        if ($staged -and (Test-Path -LiteralPath $staged)) { Remove-Item -LiteralPath $staged -Force }
        if ($temporary -and (Test-Path -LiteralPath $temporary)) { Remove-Item -LiteralPath $temporary -Recurse -Force }
        [Net.ServicePointManager]::SecurityProtocol = $originalTls
    }
}

# Dot-sourcing exposes helpers for isolated tests without installing anything.
if (-not $PSCommandPath -or $MyInvocation.InvocationName -ne '.') {
    try {
        Install-Unmus -Version $Version -BinDir $BinDir -Force:$Force -Run:$Run -NoPath:$NoPath -Arguments (@($Arguments) + @($args))
    } catch {
        $global:LASTEXITCODE = 1
        throw
    }
    # IEX inherits its caller's MyInvocation but has no PSCommandPath. Never
    # exit that caller's script/session; only return an exit code for -File.
    if ($PSCommandPath -and $MyInvocation.MyCommand.CommandType -eq 'ExternalScript') { exit $global:LASTEXITCODE }
}
