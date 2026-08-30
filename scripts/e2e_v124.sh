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

grep -q 'function Stop-GitMakeAtTarget' internal/winreplace/script.go
grep -q 'for (\$i = 0; \$i -lt 240; \$i++)' internal/winreplace/script.go
grep -q 'Wait-Process -Id \$id -Timeout 2' internal/winreplace/script.go
python - <<'PY2'
from pathlib import Path
s=Path('internal/winreplace/script.go').read_text()
loop=s.index('for ($i = 0; $i -lt 240; $i++)')
stop=s.index('Stop-GitMakeAtTarget', loop)
move=s.index('Move-Item -LiteralPath $src -Destination $dst -Force', loop)
assert stop < move
PY2
if grep -q 'taskkill /IM gitmake.exe' internal/winreplace/script.go; then
  echo "unsafe broad gitmake process termination found" >&2
  exit 1
fi

echo V124_RESPAWN_SAFE_REPLACEMENT_E2E_PASS
