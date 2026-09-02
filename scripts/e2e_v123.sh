#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

go test ./internal/selfupdate ./internal/installer ./internal/upgrader >/dev/null

tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go test -c -o "$tmp/installer.test.exe" ./internal/installer
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go test -c -o "$tmp/upgrader.test.exe" ./internal/upgrader
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go test -c -o "$tmp/selfupdate.test.exe" ./internal/selfupdate

grep -q "Get-Process -Name 'gitmake'" internal/selfupdate/script.go
grep -q '\[string\]::Equals(\$processFull, \$dstFull' internal/selfupdate/script.go
if grep -q 'taskkill /IM gitmake.exe' internal/selfupdate/script.go; then
  echo "unsafe broad gitmake process termination found" >&2
  exit 1
fi

echo V123_LOCKED_EXECUTABLE_RECOVERY_E2E_PASS
