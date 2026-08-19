@echo off
cd /d "%~dp0"
gitmake.exe
set "code=%ERRORLEVEL%"
echo.
if not "%code%"=="0" echo GitMake exited with code %code%.
pause
exit /b %code%
