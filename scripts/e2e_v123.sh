#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

go test ./internal/winreplace ./internal/installer ./internal/upgrader >/dev/null

tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go test -c -o "$tmp/installer.test.exe" ./internal/installer
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go test -c -o "$tmp/upgrader.test.exe" ./internal/upgrader
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go test -c -o "$tmp/winreplace.test.exe" ./internal/winreplace

grep -q "Get-Process -Name 'gitmake'" internal/winreplace/script.go
grep -q '\[string\]::Equals(\$processFull, \$dstFull' internal/winreplace/script.go
if grep -q 'taskkill /IM gitmake.exe' internal/winreplace/script.go; then
  echo "unsafe broad gitmake process termination found" >&2
  exit 1
fi

echo V123_LOCKED_EXECUTABLE_RECOVERY_E2E_PASS
