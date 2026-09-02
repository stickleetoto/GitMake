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

grep -q 'function Stop-GitMakeAtTarget' internal/selfupdate/script.go
grep -q 'for (\$i = 0; \$i -lt 240; \$i++)' internal/selfupdate/script.go
grep -q 'Wait-Process -Id \$id -Timeout 2' internal/selfupdate/script.go
# The v1.2.4 contract is that a respawned target is evicted on every retry, not
# only once. v1.2.6 keeps that but makes eviction a recovery step: rename-aside
# normally succeeds without stopping anything, so nothing is killed until an
# attempt has actually failed. Assert the contract, not the old statement order.
python - <<'PY2'
from pathlib import Path
s=Path('internal/selfupdate/script.go').read_text()
loop=s.index('for ($i = 0; $i -lt 240; $i++)')
body=s[loop:]
assert 'Stop-GitMakeAtTarget' in body, 'retry loop must evict the exact-path target'
assert body.index('Write-GitMakeLog ("replacement retry "') < body.index('Stop-GitMakeAtTarget'), \
    'processes must only be stopped after an attempt has failed'
aside=body.index('Move-Item -LiteralPath $dstFull -Destination $backup')
install=body.index('Move-Item -LiteralPath $src -Destination $dstFull')
assert aside < install, 'the current executable must be renamed aside before the new one is installed'
assert 'Remove-Item -LiteralPath $dst -Force' not in s, \
    'the target must never be deleted before the replacement is in place'
PY2
if grep -q 'taskkill /IM gitmake.exe' internal/selfupdate/script.go; then
  echo "unsafe broad gitmake process termination found" >&2
  exit 1
fi

echo V124_RESPAWN_SAFE_REPLACEMENT_E2E_PASS
