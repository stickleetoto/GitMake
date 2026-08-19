$ErrorActionPreference = "Stop"
New-Item -ItemType Directory -Force -Path dist | Out-Null
Write-Host "Running tests..."
go test ./...
Write-Host "Building Windows amd64 binary..."
$env:GOOS = "windows"
$env:GOARCH = "amd64"
$env:CGO_ENABLED = "0"
go build -trimpath -ldflags "-s -w" -o dist/gitmake.exe ./cmd/gitmake
Write-Host "Built: dist/gitmake.exe"
