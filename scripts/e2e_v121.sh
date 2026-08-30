#!/usr/bin/env bash
set -euo pipefail
ROOT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
BIN="$ROOT_DIR/dist/gitmake-e2e-v121"
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT
mkdir -p "$ROOT_DIR/dist" "$TMP/bin" "$TMP/project"
go build -o "$BIN" "$ROOT_DIR/cmd/gitmake"

cat > "$TMP/bin/gh" <<'GH'
#!/usr/bin/env bash
set -euo pipefail
if [[ "${1:-}" == "--version" ]]; then echo "gh version 2.fake"; exit 0; fi
if [[ "${1:-}" == "auth" && "${2:-}" == "status" ]]; then echo "Logged in"; exit 0; fi
if [[ "${1:-}" == "api" && "${2:-}" == "user" ]]; then echo "testuser"; exit 0; fi
if [[ "${1:-}" == "repo" && "${2:-}" == "view" ]]; then echo "HTTP 404 repository not found" >&2; exit 1; fi
if [[ "${1:-}" == "api" && "${2:-}" == repos/*/branches/*/protection ]]; then echo "HTTP 404: Branch not protected" >&2; exit 1; fi
if [[ "${1:-}" == "api" && "${2:-}" == repos/*/git/ref/tags/* ]]; then echo "HTTP 404: Not Found" >&2; exit 1; fi
echo "fake gh unsupported: $*" >&2
exit 2
GH
chmod +x "$TMP/bin/gh"

printf '# v1.2.1 routing hardening\n' > "$TMP/project/README.md"
printf 'print("routing")\n' > "$TMP/project/main.py"
export PATH="$TMP/bin:$PATH" GIT_CONFIG_GLOBAL="$TMP/gitconfig" XDG_CACHE_HOME="$TMP/cache"
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
