#!/usr/bin/env bash
set -euo pipefail
ROOT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
BIN="$ROOT_DIR/dist/gitmake-e2e-v121"
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT
mkdir -p "$ROOT_DIR/dist" "$TMP/bin" "$TMP/project"
go build -o "$BIN" "$ROOT_DIR/cmd/gitmake"

source "$(dirname "${BASH_SOURCE[0]}")/fakegh.sh"
install_fake_gh "$TMP/bin" "$TMP/remotes"

printf '# v1.2.1 routing hardening\n' > "$TMP/project/README.md"
printf 'print("routing")\n' > "$TMP/project/main.py"
export GIT_CONFIG_GLOBAL="$TMP/gitconfig" XDG_CACHE_HOME="$TMP/cache"
require_fake_gh "$BIN"
git config --global user.name "GitMake V121"
git config --global user.email "v121@example.test"

python3 - "$BIN" "$TMP/project" <<'PY'
import json, os, subprocess, sys
bin,cwd=sys.argv[1:]
p=subprocess.Popen([bin,'mcp','--allow-write'],cwd=cwd,env=os.environ.copy(),stdin=subprocess.PIPE,stdout=subprocess.PIPE,stderr=subprocess.PIPE,text=True,bufsize=1)
def send(o):
    p.stdin.write(json.dumps(o,separators=(',',':'))+'\n'); p.stdin.flush()
def recv():
    line=p.stdout.readline()
    if not line:
        raise RuntimeError('MCP closed: '+p.stderr.read())
    return json.loads(line)

# A modern client may still perform initialize. This must NOT arm the legacy
# held-open elicitation path.
send({'jsonrpc':'2.0','id':1,'method':'initialize','params':{
    'protocolVersion':'2026-07-28',
    'capabilities':{'elicitation':{}},
    'clientInfo':{'name':'v121-e2e','version':'1'},
}})
init=recv()
assert init['id']==1, init

meta={
  'io.modelcontextprotocol/protocolVersion':'2026-07-28',
  'io.modelcontextprotocol/clientInfo':{'name':'v121-e2e','version':'1'},
  'io.modelcontextprotocol/clientCapabilities':{'elicitation':{}},
}
send({'jsonrpc':'2.0','id':2,'method':'tools/call','params':{
    'name':'gitmake_publish','arguments':{'project_dir':cwd},'_meta':meta,
}})
r=recv()
assert r.get('id')==2, r
assert r.get('result',{}).get('resultType')=='input_required', r
assert r.get('method') is None, r
assert r['result'].get('requestState'), r
assert 'gitmake_approval' in r['result'].get('inputRequests',{}), r

p.stdin.close(); p.terminate(); p.wait(timeout=5)
PY

echo V121_PROTOCOL_ROUTING_E2E_PASS
