#!/usr/bin/env bash
# v1.2.6 updater lifecycle E2E.
#
# The v1.2.3-v1.2.5 updater passed every test while never once replacing an
# executable, because the suite only asserted on the text of a generated
# PowerShell script. This gate runs the real thing: it drives the production
# replacement path against live processes and checks that a failed upgrade is
# non-destructive and never claims success.
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

EXE=""
case "$(uname -s 2>/dev/null || echo unknown)" in
  MINGW*|MSYS*|CYGWIN*|Windows_NT) EXE=".exe" ;;
esac

BIN="$TMP/gitmake$EXE"
(cd "$ROOT" && go build -trimpath -o "$BIN" ./cmd/gitmake)

# 1. The build under test is v1.2.6.
VERSION="$("$BIN" --version)"
if [[ "$VERSION" != "gitmake 1.2.6" ]]; then
  echo "unexpected version: $VERSION" >&2
  exit 60
fi

# 2. A failed upgrade must fail loudly, must not claim an install, and must not
#    touch the executable it was going to replace. An unroutable proxy stands in
#    for any network failure.
before="$(cd "$TMP" && sha256sum "gitmake$EXE" | cut -d' ' -f1)"
set +e
HTTPS_PROXY="http://127.0.0.1:1" HTTP_PROXY="http://127.0.0.1:1" \
  "$BIN" upgrade > "$TMP/upgrade.out" 2> "$TMP/upgrade.err"
code=$?
set -e
if [[ $code -eq 0 ]]; then
  echo "upgrade reported success with no reachable release API" >&2
  cat "$TMP/upgrade.out" >&2
  exit 61
fi
if grep -qE "Installed|Upgrade staged" "$TMP/upgrade.out"; then
  echo "failed upgrade claimed progress:" >&2
  cat "$TMP/upgrade.out" >&2
  exit 62
fi
after="$(cd "$TMP" && sha256sum "gitmake$EXE" | cut -d' ' -f1)"
if [[ "$before" != "$after" ]]; then
  echo "failed upgrade modified the target executable" >&2
  exit 63
fi

# 3. No temporary download directory may survive a failed upgrade.
if compgen -G "${TMPDIR:-/tmp}/gitmake-upgrade-*" > /dev/null 2>&1; then
  # Pre-existing directories from earlier manual runs are not this run's fault;
  # only fail when one was created during this test.
  newest="$(ls -dt "${TMPDIR:-/tmp}"/gitmake-upgrade-* 2>/dev/null | head -1)"
  if [[ -n "$newest" && -z "$(find "$newest" -maxdepth 0 -mmin +5 2>/dev/null)" ]]; then
    echo "failed upgrade left a temporary download directory: $newest" >&2
    exit 64
  fi
fi

# 4. Process-level replacement gates. These build real executables, hold them
#    open with live processes, and require the canonical path to report the new
#    version afterwards.
(cd "$ROOT" && go test -count=1 ./internal/winreplace/ ./internal/upgrader/ ./internal/installer/)

# 5. The v1.2.5 fixes must not regress.
D="$TMP/project"
mkdir -p "$D"
printf '# demo\n' > "$D/README.md"
if (cd "$D" && "$BIN" preview --json > "$TMP/preview.json"); then
  echo "gitmake preview unexpectedly treated as source" >&2
  exit 65
fi
python3 - "$TMP/preview.json" <<'PY'
import json,sys
x=json.load(open(sys.argv[1],encoding='utf-8'))
assert x['ok'] is False and x['error']['code']=='USAGE_ERROR', x
assert '--dry-run --read-only' in x['error']['message'], x
PY

echo V126_UPDATER_LIFECYCLE_E2E_PASS
