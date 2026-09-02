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

D="$TMP/project"
mkdir -p "$D"
printf '# demo\n' > "$D/README.md"
printf 'package main\n' > "$D/main.go"

cat > "$TMP/stdin.json" <<'JSON'
{
  "repo": {"name": "zzz-distinct-name", "visibility": "public"},
  "source": {"folder": "."},
  "git": {"branch": "develop"}
}
JSON

# 1. Root --stdin is authoritative ephemeral config, not silently ignored.
require_fake_gh "$BIN"

(cd "$D" && "$BIN" --stdin --dry-run --read-only --json < "$TMP/stdin.json" > "$TMP/stdin-ok.json")
python3 - "$TMP/stdin-ok.json" <<'PY'
import json,sys
x=json.load(open(sys.argv[1],encoding='utf-8'))
assert x['ok'] is True, x
p=x['pipeline']
assert p['config']['source']=='stdin' and p['config']['persisted'] is False, p
assert p['repository']=='testuser/zzz-distinct-name', p
assert p['visibility']=='public', p
assert p['branch']=='develop', p
assert not __import__('os').path.exists(__import__('os').path.join(__import__('os').path.dirname(sys.argv[1]),'project','gitmake.json'))
PY

# 2. Malformed --stdin fails closed with CONFIG_INVALID.
if (cd "$D" && printf 'this is not json at all {{{' | "$BIN" --stdin --dry-run --read-only --json > "$TMP/stdin-bad.json"); then
  echo "malformed stdin unexpectedly succeeded" >&2
  exit 51
fi
python3 - "$TMP/stdin-bad.json" <<'PY'
import json,sys
x=json.load(open(sys.argv[1],encoding='utf-8'))
assert x['ok'] is False, x
assert x['error']['code']=='CONFIG_INVALID', x
assert 'parse config' in x['error']['message'].lower(), x
PY

# 3. Scanner reports every supported secret kind across and within files.
python3 - "$D" <<'PYSECRETS'
import os,sys
d=sys.argv[1]
aws='AK'+'IA'+'ABCDEFGHIJKLMNOP'
gh='gh'+'p_'+'abcdefghijklmnopqrstuvwxyz012345'
slack='xox'+'b-'+'1234567890-abcdefghijklmnopqrstuv'
open(os.path.join(d,'leak_one.txt'),'w',encoding='utf-8').write(aws+'\n'+gh+'\n')
open(os.path.join(d,'leak_two.txt'),'w',encoding='utf-8').write(slack+'\n')
PYSECRETS
if (cd "$D" && "$BIN" --stdin --dry-run --read-only --json < "$TMP/stdin.json" > "$TMP/secrets.json"); then
  echo "secret preview unexpectedly succeeded" >&2
  exit 52
fi
python3 - "$TMP/secrets.json" <<'PY'
import json,sys
x=json.load(open(sys.argv[1],encoding='utf-8'))
assert x['ok'] is False, x
assert x['error']['code']=='SECRET_DETECTED', x
findings=x['pipeline']['security']['findings']
got={(f['path'],f['kind']) for f in findings}
want={
 ('leak_one.txt','aws_access_key'),
 ('leak_one.txt','github_token'),
 ('leak_two.txt','slack_token'),
}
assert want <= got, (want,got,x)
assert len(findings) >= 3, findings
PY

# 4. MCP preview preserves the CLI machine error payload instead of exit-code-only text.
"$BIN" mcp <<EOF_MCP > "$TMP/mcp.out"
{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"gitmake_preview","arguments":{"project_dir":"$D"}}}
EOF_MCP
python3 - "$TMP/mcp.out" <<'PY'
import json,sys
line=[l for l in open(sys.argv[1],encoding='utf-8') if l.strip()][0]
r=json.loads(line)
res=r['result']
assert res['isError'] is True, r
sc=res.get('structuredContent')
assert isinstance(sc,dict), r
assert sc['error']['code']=='SECRET_DETECTED', sc
assert sc['error']['stage']=='SECURITY', sc
assert sc['error']['recoverable'] is True, sc
assert sc['error']['suggested_action'], sc
assert len(sc['pipeline']['security']['findings']) >= 3, sc
PY

# 5. Common pseudo-subcommand gets an actionable usage error instead of a path error.
if (cd "$D" && "$BIN" preview --json > "$TMP/preview.json"); then
  echo "gitmake preview unexpectedly treated as source" >&2
  exit 53
fi
python3 - "$TMP/preview.json" <<'PY'
import json,sys
x=json.load(open(sys.argv[1],encoding='utf-8'))
assert x['ok'] is False and x['error']['code']=='USAGE_ERROR', x
assert '--dry-run --read-only' in x['error']['message'], x
PY

echo V125_REAL_WORLD_FIXES_E2E_PASS
