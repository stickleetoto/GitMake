@echo off
setlocal
if not exist dist mkdir dist
echo Running tests...
go test ./... || goto :fail
echo Running vet...
go vet ./... || goto :fail
echo Building Windows amd64 binaries...
set GOOS=windows
set GOARCH=amd64
set CGO_ENABLED=0
go build -trimpath -ldflags="-s -w" -o dist\gitmake.exe .\cmd\gitmake || goto :fail
go build -trimpath -ldflags="-s -w" -o dist\GitMake-Setup.exe .\cmd\setup || goto :fail
echo.
echo Built: dist\gitmake.exe
echo Built: dist\GitMake-Setup.exe
pause
exit /b 0
:fail
echo.
echo Build failed with exit code %ERRORLEVEL%.
pause
exit /b %ERRORLEVEL%
