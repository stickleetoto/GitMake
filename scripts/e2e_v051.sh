#!/usr/bin/env bash
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT
BIN="$TMP/gitmake"
(cd "$ROOT" && go build -trimpath -o "$BIN" ./cmd/gitmake)
mkdir -p "$TMP/fakebin"
source "$(dirname "${BASH_SOURCE[0]}")/fakegh.sh"
install_fake_gh "$TMP/fakebin" "$TMP/remotes"

# 1. A single ZIP can be planned read-only without creating gitmake.json.
D="$TMP/single"; mkdir -p "$D"
python3 - "$D/GitMake_TestProject_v1.zip" <<'PY'
import sys,zipfile
with zipfile.ZipFile(sys.argv[1],'w',zipfile.ZIP_DEFLATED) as z:
    z.writestr('README.md','# Demo\n')
    z.writestr('src/main.py','print("hi")\n')
PY
require_fake_gh "$BIN"

(cd "$D" && "$BIN" --dry-run --read-only --json > out.json)
test ! -e "$D/gitmake.json"
python3 - "$D/out.json" <<'PY'
import json,sys
x=json.load(open(sys.argv[1],encoding='utf-8'))
assert x['ok'] is True, x
p=x['pipeline']
assert p['config']=={'source':'inferred','persisted':False}, p
assert p['source']=='GitMake_TestProject_v1.zip', p
assert p['repository']=='testuser/GitMake_TestProject', p
assert p['mode']=='CREATE' and p['changes']['added']==2, p
PY

# 2. Multi-ZIP discovery selects a source archive and classifies release assets.
M="$TMP/multi"; mkdir -p "$M"
python3 - "$M" <<'PY'
import sys,zipfile,os
D=sys.argv[1]
def wz(name, files):
    with zipfile.ZipFile(os.path.join(D,name),'w',zipfile.ZIP_DEFLATED) as z:
        for n,b in files.items(): z.writestr(n,b)
wz('Demo_v1.2.3_Source.zip', {'go.mod':'module demo\n','cmd/demo/main.go':'package main\n'})
wz('Demo_v1.2.3_Windows_x64.zip', {'demo.exe':'MZfake','README.md':'demo'})
wz('Demo_v1.2.3_Linux_x64.zip', {'demo':'ELFfake','README.md':'demo'})
PY
(cd "$M" && "$BIN" discover --json > discover.json)
python3 - "$M/discover.json" <<'PY'
import json,sys
x=json.load(open(sys.argv[1],encoding='utf-8'))
assert x['schema']=='gitmake.discovery/v1'
assert x['selected_source']=='Demo_v1.2.3_Source.zip', x
assert set(x['release_assets'])=={'Demo_v1.2.3_Windows_x64.zip','Demo_v1.2.3_Linux_x64.zip'}, x
assert x['needs_input'] is False
PY
(cd "$M" && "$BIN" --dry-run --read-only --json > plan.json)
test ! -e "$M/gitmake.json"
python3 - "$M/plan.json" <<'PY'
import json,sys
x=json.load(open(sys.argv[1],encoding='utf-8'))
p=x['pipeline']
assert x['ok'] is True, x
assert p['source']=='Demo_v1.2.3_Source.zip', p
assert p['repository']=='testuser/Demo', p
assert p['discovery']['selected_source']=='Demo_v1.2.3_Source.zip', p
assert len(p['discovery']['release_assets'])==2, p
PY

# 3. Two equally plausible source projects are never guessed.
A="$TMP/ambiguous"; mkdir -p "$A"
python3 - "$A" <<'PY'
import sys,zipfile,os
D=sys.argv[1]
for name,mod in [('Alpha.zip','a'),('Beta.zip','b')]:
    with zipfile.ZipFile(os.path.join(D,name),'w') as z:
        z.writestr('go.mod',f'module {mod}\n')
        z.writestr('src/main.go','package main\n')
PY
if (cd "$A" && "$BIN" --dry-run --read-only --json > ambiguous.json); then exit 41; fi
python3 - "$A/ambiguous.json" <<'PY'
import json,sys
x=json.load(open(sys.argv[1],encoding='utf-8'))
assert x['ok'] is False, x
assert x['pipeline']['discovery']['needs_input'] is True, x
assert x['pipeline']['discovery']['reason']=='multiple_source_candidates', x
assert x['error']['code']=='SOURCE_AMBIGUOUS', x
PY

# 4. Explicit source always wins in an ambiguous folder and stays read-only.
(cd "$A" && "$BIN" --dry-run --read-only --json Alpha.zip > explicit.json)
test ! -e "$A/gitmake.json"
python3 - "$A/explicit.json" <<'PY'
import json,sys
x=json.load(open(sys.argv[1],encoding='utf-8'))
assert x['ok'] is True, x
assert x['pipeline']['source']=='Alpha.zip', x
assert x['pipeline']['repository']=='testuser/Alpha', x
PY

echo V051_E2E_PASS
