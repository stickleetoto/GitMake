#!/usr/bin/env bash
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT
BIN="$TMP/gitmake"
(cd "$ROOT" && go build -trimpath -o "$BIN" ./cmd/gitmake)

make_zip() {
  local out="$1" root="$2"
  python3 - "$out" "$root" <<'PY'
import sys, zipfile
out, root = sys.argv[1:]
with zipfile.ZipFile(out, 'w', zipfile.ZIP_DEFLATED) as z:
    z.writestr(root + '/README.md', '# demo\n')
    z.writestr(root + '/src/main.txt', 'hello\n')
PY
}

# 1. Interactive one-ZIP wizard.
D1="$TMP/interactive"
mkdir -p "$D1"
make_zip "$D1/ContextDiet_v1.2.3.zip" "ContextDiet"
(
  cd "$D1"
  printf '\n2\nA compact demo\n\ny\n' | "$BIN" init >/dev/null
)
python3 - "$D1/gitmake.json" <<'PY'
import json, sys
p=sys.argv[1]
d=json.load(open(p,encoding='utf-8'))
assert d['repo']['name']=='ContextDiet', d
assert d['repo']['visibility']=='public', d
assert d['repo']['description']=='A compact demo', d
assert d['source']['zip']=='ContextDiet_v1.2.3.zip', d
assert d['git']['branch']=='main', d
PY

# 2. Multiple-ZIP selection.
D2="$TMP/multiple"
mkdir -p "$D2"
make_zip "$D2/Alpha_v1.0.zip" "Alpha"
make_zip "$D2/Beta_v2.0.zip" "Beta"
(
  cd "$D2"
  printf '2\n\n1\n\n\ny\n' | "$BIN" init >/dev/null
)
python3 - "$D2/gitmake.json" <<'PY'
import json, sys
x=json.load(open(sys.argv[1],encoding='utf-8'))
assert x['repo']['name']=='Beta', x
assert x['repo']['visibility']=='private', x
assert x['source']['zip']=='Beta_v2.0.zip', x
PY

# 3. --yes is non-interactive and keeps safe defaults.
D3="$TMP/yes"
mkdir -p "$D3"
make_zip "$D3/Quick_v0.1.zip" "Quick"
(cd "$D3" && "$BIN" init --yes >/dev/null)
python3 - "$D3/gitmake.json" <<'PY'
import json, sys
x=json.load(open(sys.argv[1],encoding='utf-8'))
assert x['repo']['name']=='Quick', x
assert x['repo']['visibility']=='private', x
PY

# 4. No ZIP => no placeholder config written.
D4="$TMP/empty"
mkdir -p "$D4"
(cd "$D4" && "$BIN" init >/dev/null)
test ! -e "$D4/gitmake.json"

# 5. Existing config is never overwritten by init.
before="$(cat "$D3/gitmake.json")"
(cd "$D3" && printf 'anything\n' | "$BIN" init >/dev/null)
after="$(cat "$D3/gitmake.json")"
test "$before" = "$after"

# 6. Version/help surface.
"$BIN" --version | grep -q '0.6.1'
"$BIN" help | grep -q -- '--yes'

echo V04_E2E_PASS
