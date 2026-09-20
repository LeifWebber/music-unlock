# Runs under both Windows PowerShell 5.1 and PowerShell 7; no network/account access.
$ErrorActionPreference = 'Stop'
$PSNativeCommandUseErrorActionPreference = $false
$installer = Join-Path $PSScriptRoot 'install.ps1'
. $installer
$task = Join-Path ([IO.Path]::GetTempPath()) ('unmus-tests-' + [Guid]::NewGuid().ToString('N'))
[void][IO.Directory]::CreateDirectory($task)
$previousTemp = $env:TEMP
$previousTmp = $env:TMP
$previousPath = $env:PATH
$previousEncoding = [Console]::OutputEncoding
$previousTls = [Net.ServicePointManager]::SecurityProtocol
$pathKey = [Microsoft.Win32.Registry]::CurrentUser.CreateSubKey('Environment')
$hadPath = $pathKey.GetValueNames() -contains 'Path'
$previousUserPath = $pathKey.GetValue('Path', '', [Microsoft.Win32.RegistryValueOptions]::DoNotExpandEnvironmentNames)
$previousKind = [Microsoft.Win32.RegistryValueKind]::ExpandString
if ($hadPath) { $previousKind = $pathKey.GetValueKind('Path') }

function Assert($Condition, [string]$Message) { if (-not $Condition) { throw $Message } }
function Assert-Fails([scriptblock]$Action, [string]$Pattern) {
    $failed = $false
    try { & $Action } catch { $failed = $true; Assert ($_.Exception.Message -match $Pattern) "Unexpected error: $_" }
    Assert $failed "Expected failure: $Pattern"
}
function Assert-Clean {
    Assert (@(Get-ChildItem -LiteralPath $task -Directory | Where-Object { $_.Name -like 'unmus-*' }).Count -eq 0) 'Temporary installer files leaked.'
}
function Save-UnmusDownload([string]$Uri, [string]$Destination) {
    $script:downloaded += $Uri
    Copy-Item -LiteralPath (Join-Path $script:assets ($Uri.Split('/')[-1])) -Destination $Destination
}
function Get-UnmusReleaseVersion { return 'v0.2.0' }
function Make-Archive([string]$Architecture, [switch]$Duplicate, [switch]$Missing) {
    $name = "music-unlock-v0.2.0-windows-$Architecture"
    $zipPath = Join-Path $script:assets "$name.zip"
    if (Test-Path $zipPath) { Remove-Item $zipPath }
    $zip = [IO.Compression.ZipFile]::Open($zipPath, [IO.Compression.ZipArchiveMode]::Create)
    try {
        [void][IO.Compression.ZipFileExtensions]::CreateEntryFromFile($zip, $script:fixture, '../../escape.exe')
        if (-not $Missing) { [void][IO.Compression.ZipFileExtensions]::CreateEntryFromFile($zip, $script:fixture, "$name/unmus.exe") }
        if ($Duplicate) { [void][IO.Compression.ZipFileExtensions]::CreateEntryFromFile($zip, $script:fixture, "$name/unmus.exe") }
    } finally { $zip.Dispose() }
    $hash = (Get-FileHash -LiteralPath $zipPath).Hash
    [IO.File]::WriteAllText((Join-Path $script:assets 'SHA256SUMS'), "$hash  $name.zip`n")
}
function Run-Cmd([string]$Extra = '') {
    $start = New-Object Diagnostics.ProcessStartInfo
    $start.FileName = $env:ComSpec
    $start.Arguments = '/d /s /c ""' + $script:cmdPath + '" -BinDir "' + $script:destination + '"' + $Extra + '"'
    $start.UseShellExecute = $false
    $start.RedirectStandardOutput = $true
    $start.RedirectStandardError = $true
    $process = [Diagnostics.Process]::Start($start)
    $output = $process.StandardOutput.ReadToEnd()
    $errorText = $process.StandardError.ReadToEnd()
    $process.WaitForExit()
    $result = @{ Code = $process.ExitCode; Output = $output; ErrorText = $errorText }
    $process.Dispose()
    return $result
}
try {
    $env:TEMP = $task
    $env:TMP = $task
    [Console]::OutputEncoding = New-Object Text.UTF8Encoding
    $script:assets = Join-Path $task 'assets'
    [void][IO.Directory]::CreateDirectory($script:assets)
    $script:fixture = Join-Path $task 'fixture.exe'
    & go build -o $script:fixture (Join-Path $PSScriptRoot 'testdata/installer_main.go')
    Assert ($LASTEXITCODE -eq 0) 'Could not build fixture.'
    Add-Type -AssemblyName System.IO.Compression.FileSystem
    $script:architecture = Get-UnmusArchitecture
    Assert ($script:architecture -in @('amd64', 'arm64')) 'Native architecture was not recognized.'
    Make-Archive $script:architecture
    $script:downloaded = @()
    $script:destination = Join-Path $task ('install with spaces ' + [char]0x97f3 + [char]0x4e50)
    Install-Unmus -BinDir $script:destination -NoPath
    $exe = Join-Path $script:destination 'unmus.exe'
    Assert (Test-Path -LiteralPath $exe) 'Executable not installed.'
    Assert ((Get-FileHash $exe).Hash -eq (Get-FileHash $script:fixture).Hash) 'Installed bytes differ.'
    Assert (-not (Test-Path (Join-Path $task 'escape.exe'))) 'ZIP path escaped.'
    Assert-Clean
    Assert-Fails { Install-Unmus -BinDir $script:destination -NoPath } 'Refusing to overwrite'
    [IO.File]::WriteAllText($exe, 'keep me')
    [IO.File]::WriteAllText((Join-Path $script:assets 'SHA256SUMS'), (('0' * 64) + "  music-unlock-v0.2.0-windows-$script:architecture.zip`n"))
    Assert-Fails { Install-Unmus -BinDir $script:destination -NoPath -Force } 'SHA256 verification failed'
    Assert ([IO.File]::ReadAllText($exe) -eq 'keep me') 'Bad checksum replaced existing installation.'
    Make-Archive $script:architecture
    Install-Unmus -BinDir $script:destination -NoPath -Force
    Assert ((Get-FileHash $exe).Hash -eq (Get-FileHash $script:fixture).Hash) 'Force upgrade failed.'
    $unicode = ([char]0x6b4c).ToString() + [char]0x66f2 + ' with spaces.ncm'
    $argumentResult = Install-Unmus -Run -Arguments @($unicode, 'output with spaces', '--offline') | ConvertFrom-Json
    Assert ($argumentResult.Count -eq 3 -and $argumentResult[0] -ceq $unicode -and $argumentResult[2] -ceq '--offline') 'Arguments changed.'
    $emptyResult = @(Install-Unmus -Run | ConvertFrom-Json)
    Assert (@($emptyResult).Count -eq 0) 'Zero-argument mode changed.'
    $null = Install-Unmus -Run -Arguments @('fail')
    Assert ($global:LASTEXITCODE -eq 7) 'Native exit code lost.'
    Assert-Clean
    $sumPath = Join-Path $script:assets 'SHA256SUMS'
    $sum = [IO.File]::ReadAllText($sumPath)
    [IO.File]::WriteAllText($sumPath, $sum + $sum)
    Assert-Fails { Install-Unmus -Run } 'ambiguous SHA256'
    Make-Archive $script:architecture -Duplicate
    Assert-Fails { Install-Unmus -Run } 'Invalid executable'
    Make-Archive $script:architecture -Missing
    Assert-Fails { Install-Unmus -Run } 'Invalid executable'
    Make-Archive $script:architecture
    Assert-Fails { Install-Unmus -Version '../bad' -Run } 'Invalid release version'
    $directoryTarget = Join-Path $task 'directory-target'
    [void][IO.Directory]::CreateDirectory((Join-Path $directoryTarget 'unmus.exe'))
    Assert-Fails { Install-Unmus -BinDir $directoryTarget -Force -NoPath } 'Refusing to overwrite'
    Assert-Clean
    Assert ([Net.ServicePointManager]::SecurityProtocol -eq $previousTls) 'TLS settings leaked.'
    Assert ($pathKey.GetValue('Path', '', [Microsoft.Win32.RegistryValueOptions]::DoNotExpandEnvironmentNames) -ceq $previousUserPath) 'NoPath changed user PATH.'
    Add-UnmusUserPath $script:destination
    $once = $pathKey.GetValue('Path', '', [Microsoft.Win32.RegistryValueOptions]::DoNotExpandEnvironmentNames)
    Add-UnmusUserPath $script:destination
    Assert ($pathKey.GetValue('Path', '', [Microsoft.Win32.RegistryValueOptions]::DoNotExpandEnvironmentNames) -ceq $once) 'Duplicate user PATH entry.'
    Assert (Test-UnmusPathEntry $once $script:destination) 'User PATH was not updated.'

    # Exercise CMD bootstrap without internet. curl.exe copies a local test script.
    $mock = Join-Path $task 'mock'
    [void][IO.Directory]::CreateDirectory($mock)
    Copy-Item $script:fixture (Join-Path $mock 'curl.exe')
    $env:PATH = $mock + ';' + $env:PATH
    $env:UNMUS_TEST_BOOTSTRAP_SOURCE = Join-Path $task 'bootstrap.ps1'
    [IO.File]::WriteAllText($env:UNMUS_TEST_BOOTSTRAP_SOURCE, 'param([string]$BinDir,[switch]$Run); Write-Output $BinDir; if ($Run) { exit 7 }; exit 0')
    $script:cmdPath = Join-Path $task 'install wrapper.cmd'
    Copy-Item (Join-Path $PSScriptRoot 'install.cmd') $script:cmdPath
    $result = Run-Cmd
    Assert ($result.Code -eq 0 -and $result.Output.Contains('install with spaces')) ('CMD arguments failed: ' + $result.ErrorText)
    $result = Run-Cmd ' -Run'
    Assert ($result.Code -eq 7) ('CMD exit code failed: ' + $result.ErrorText)
    $env:UNMUS_TEST_DOWNLOAD_FAIL = '1'
    $result = Run-Cmd
    Assert ($result.Code -ne 0) 'CMD download error ignored.'
    Assert-Clean
    # The expected native exit-7 test must not become the CI step's exit code.
    $global:LASTEXITCODE = 0
    Write-Host 'Windows installer contract tests passed.'
} finally {
    if ($hadPath) { $pathKey.SetValue('Path', $previousUserPath, $previousKind) } else { $pathKey.DeleteValue('Path', $false) }
    $pathKey.Dispose()
    $env:TEMP = $previousTemp
    $env:TMP = $previousTmp
    $env:PATH = $previousPath
    [Console]::OutputEncoding = $previousEncoding
    Remove-Item Env:UNMUS_TEST_BOOTSTRAP_SOURCE -ErrorAction SilentlyContinue
    Remove-Item Env:UNMUS_TEST_DOWNLOAD_FAIL -ErrorAction SilentlyContinue
    Remove-Item -LiteralPath $task -Recurse -Force
}
