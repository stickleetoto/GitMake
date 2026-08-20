$ErrorActionPreference = "Stop"
New-Item -ItemType Directory -Force -Path dist | Out-Null
Write-Host "Running tests..."
go test ./...
Write-Host "Running vet..."
go vet ./...
Write-Host "Building Windows amd64 binaries..."
$env:GOOS = "windows"
$env:GOARCH = "amd64"
$env:CGO_ENABLED = "0"
go build -trimpath -ldflags "-s -w" -o dist/gitmake.exe ./cmd/gitmake
go build -trimpath -ldflags "-s -w" -o dist/GitMake-Setup.exe ./cmd/setup
Write-Host "Built: dist/gitmake.exe"
Write-Host "Built: dist/GitMake-Setup.exe"
