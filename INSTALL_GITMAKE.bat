@echo off
cd /d "%~dp0"
if not exist gitmake.exe (
  echo ERROR: gitmake.exe is missing beside this installer.
  pause
  exit /b 1
)
gitmake.exe install
set "code=%ERRORLEVEL%"
echo.
if not "%code%"=="0" echo GitMake install exited with code %code%.
pause
exit /b %code%
