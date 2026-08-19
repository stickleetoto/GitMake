@echo off
setlocal
if not exist dist mkdir dist
echo Running tests...
go test ./... || exit /b 1
echo Building Windows amd64 binary...
set GOOS=windows
set GOARCH=amd64
set CGO_ENABLED=0
go build -trimpath -ldflags="-s -w" -o dist\gitmake.exe .\cmd\gitmake || exit /b 1
echo Built: dist\gitmake.exe
