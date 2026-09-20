@echo off
setlocal
set "UNMUS_INSTALL_TEMP=%TEMP%\unmus-bootstrap-%RANDOM%-%RANDOM%"
mkdir "%UNMUS_INSTALL_TEMP%" 2>nul
if errorlevel 1 exit /b 1
curl.exe -fsSL "https://raw.githubusercontent.com/LeifWebber/music-unlock/main/scripts/install.ps1" -o "%UNMUS_INSTALL_TEMP%\install.ps1"
if errorlevel 1 (
  rmdir /s /q "%UNMUS_INSTALL_TEMP%"
  exit /b 1
)
powershell.exe -NoProfile -ExecutionPolicy Bypass -File "%UNMUS_INSTALL_TEMP%\install.ps1" %*
set "UNMUS_INSTALL_EXIT=%ERRORLEVEL%"
rmdir /s /q "%UNMUS_INSTALL_TEMP%"
exit /b %UNMUS_INSTALL_EXIT%
