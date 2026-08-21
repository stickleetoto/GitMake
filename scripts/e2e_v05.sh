#!/usr/bin/env bash
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT
BIN="$TMP/gitmake"
(cd "$ROOT" && go build -trimpath -o "$BIN" ./cmd/gitmake)

# 1. AI capability discovery is standalone JSON, not prose wrapped in JSON.
"$BIN" ai describe --json > "$TMP/manifest.json"
python3 - "$TMP/manifest.json" <<'PY'
import json,sys
x=json.load(open(sys.argv[1],encoding='utf-8'))
assert x['schema']=='gitmake.ai/v1', x
assert x['name']=='gitmake' and x['version']=='0.5.2', x
assert x['safety']['force_push'] is False
assert '--dry-run' in x['commands']['preview']['command']
assert '--read-only' in x['commands']['preview']['command']
PY

# 2. AI install preserves an existing AGENTS.md and is idempotent.
D="$TMP/agent"; mkdir -p "$D"
printf '# Existing agent rules\n\nKeep me.\n' > "$D/AGENTS.md"
(cd "$D" && "$BIN" ai install --json > first.json)
cp "$D/AGENTS.md" "$TMP/agents-first"
(cd "$D" && "$BIN" ai install --json > second.json)
cmp "$D/AGENTS.md" "$TMP/agents-first"
grep -q 'Keep me.' "$D/AGENTS.md"
test "$(grep -c '<!-- gitmake:begin -->' "$D/AGENTS.md")" = 1
python3 - "$D/.gitmake/ai.json" <<'PY'
import json,sys
x=json.load(open(sys.argv[1],encoding='utf-8'))
assert x['schema']=='gitmake.ai/v1'
PY

# 3. Read-only mode blocks local setup mutations.
R="$TMP/readonly"; mkdir -p "$R"
if (cd "$R" && "$BIN" init --read-only --json > out.json); then exit 31; fi
python3 - "$R/out.json" <<'PY'
import json,sys
x=json.load(open(sys.argv[1],encoding='utf-8'))
assert x['ok'] is False and x['exit_code']==1
assert 'read-only mode blocks' in x['error']['message']
PY
test ! -e "$R/gitmake.json"

# 4. Read-only publish requires dry-run.
if (cd "$R" && "$BIN" --read-only --json > out2.json); then exit 32; fi
python3 - "$R/out2.json" <<'PY'
import json,sys
x=json.load(open(sys.argv[1],encoding='utf-8'))
assert x['ok'] is False
assert 'requires --dry-run' in x['error']['message']
PY

# 5. Read-only dry-run does not create a missing config.
if (cd "$R" && "$BIN" --read-only --dry-run --json > out3.json); then exit 33; fi
python3 - "$R/out3.json" <<'PY'
import json,sys
x=json.load(open(sys.argv[1],encoding='utf-8'))
assert x['ok'] is False
assert 'no project ZIP found' in x['error']['message']
PY
test ! -e "$R/gitmake.json"

# 6. Version JSON has a stable tiny schema.
"$BIN" --version --json > "$TMP/version.json"
python3 - "$TMP/version.json" <<'PY'
import json,sys
x=json.load(open(sys.argv[1],encoding='utf-8'))
assert x == {'schema':'gitmake.version/v1','name':'gitmake','version':'0.5.2'}, x
PY

# 7. Generic JSON surfaces output without contaminating stdout with prose.
"$BIN" help --json > "$TMP/help.json"
python3 - "$TMP/help.json" <<'PY'
import json,sys
x=json.load(open(sys.argv[1],encoding='utf-8'))
assert x['schema']=='gitmake.result/v1' and x['ok'] is True
assert 'gitmake ai describe' in x['output']
PY

# 8. AI-safe publish preview returns structured pipeline data and performs no GitHub mutation.
mkdir -p "$TMP/fakebin" "$TMP/preview"
cat > "$TMP/fakebin/gh" <<'GH'
#!/usr/bin/env bash
set -euo pipefail
if [[ "${1:-}" == "--version" ]]; then echo "gh version 2.fake"; exit 0; fi
if [[ "${1:-}" == "auth" && "${2:-}" == "status" ]]; then echo "Logged in"; exit 0; fi
if [[ "${1:-}" == "api" && "${2:-}" == "user" ]]; then echo "testuser"; exit 0; fi
if [[ "${1:-}" == "repo" && "${2:-}" == "view" ]]; then echo "repository not found (HTTP 404)" >&2; exit 1; fi
echo "unexpected gh args: $*" >&2
exit 2
GH
chmod +x "$TMP/fakebin/gh"
python3 - "$TMP/preview/project.zip" <<'PY'
import sys,zipfile
with zipfile.ZipFile(sys.argv[1],'w',zipfile.ZIP_DEFLATED) as z:
    z.writestr('Demo/README.md','# Demo\n')
    z.writestr('Demo/src/main.txt','hello\n')
PY
cat > "$TMP/preview/gitmake.json" <<'JSON'
{
  "repo": {"name": "AgentPreview", "visibility": "private"},
  "source": {"zip": "project.zip", "strip_root": true},
  "git": {"branch": "main"}
}
JSON
(cd "$TMP/preview" && PATH="$TMP/fakebin:$PATH" "$BIN" --dry-run --read-only --json > preview.json)
python3 - "$TMP/preview/preview.json" <<'PY'
import json,sys
x=json.load(open(sys.argv[1],encoding='utf-8'))
assert x['ok'] is True, x
p=x['pipeline']
assert p['mode']=='CREATE' and p['repository']=='testuser/AgentPreview', p
assert p['dry_run'] is True and p['read_only'] is True, p
assert p['files']==2 and p['changes']['added']==2, p
assert p['stage']=='REPORT', p
assert 'PUSH' not in p.get('completed_stages',[]), p
PY

# 9. Read-only preview refuses config auto-repair when source.zip is stale.
S="$TMP/stale"; mkdir -p "$S"
python3 - "$S/actual.zip" <<'PY'
import sys,zipfile
with zipfile.ZipFile(sys.argv[1],'w') as z: z.writestr('root/a.txt','a')
PY
cat > "$S/gitmake.json" <<'JSON'
{"repo":{"name":"Stale","visibility":"private"},"source":{"zip":"missing.zip","strip_root":true},"git":{"branch":"main"}}
JSON
cp "$S/gitmake.json" "$TMP/stale-before"
if (cd "$S" && "$BIN" --dry-run --read-only --json > stale.json); then exit 34; fi
cmp "$S/gitmake.json" "$TMP/stale-before"
python3 - "$S/stale.json" <<'PY'
import json,sys
x=json.load(open(sys.argv[1],encoding='utf-8'))
assert x['ok'] is False
assert 'will not repair gitmake.json' in x['error']['message']
PY

echo V05_E2E_PASS
