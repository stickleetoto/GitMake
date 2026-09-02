#!/usr/bin/env bash
set -euo pipefail
ROOT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
BIN="$ROOT_DIR/dist/gitmake-e2e-v110"
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT
mkdir -p "$ROOT_DIR/dist" "$TMP/bin" "$TMP/remotes"
go build -o "$BIN" "$ROOT_DIR/cmd/gitmake"

source "$(dirname "${BASH_SOURCE[0]}")/fakegh.sh"
install_fake_gh "$TMP/bin" "$TMP/remotes"
export GIT_CONFIG_GLOBAL="$TMP/gitconfig" XDG_CACHE_HOME="$TMP/cache"
require_fake_gh "$BIN"
git config --global user.name "GitMake V110"
git config --global user.email "v110@example.test"

make_project() {
  local dir="$1"
  mkdir -p "$dir/src"
  printf '# Chat approval\n' > "$dir/README.md"
  printf 'print("chat approval")\n' > "$dir/src/main.py"
}

# 1) Modern 2026 MRTR: apply returns input_required, then the client-controlled
# elicitation response applies the plan without any terminal approval.
M="$TMP/ModernChatApproval"; make_project "$M"
(cd "$M" && "$BIN" plan --json > "$TMP/modern-plan.json")
MPID="$(python3 -c 'import json; print(json.load(open("'$TMP'/modern-plan.json"))["plan_id"])')"
python3 - "$BIN" "$M" "$MPID" <<'PYMODERN'
import json, os, subprocess, sys
bin,cwd,pid=sys.argv[1:]
p=subprocess.Popen([bin,'mcp','--allow-write'],cwd=cwd,env=os.environ.copy(),stdin=subprocess.PIPE,stdout=subprocess.PIPE,stderr=subprocess.PIPE,text=True,bufsize=1)
meta={
  'io.modelcontextprotocol/protocolVersion':'2026-07-28',
  'io.modelcontextprotocol/clientInfo':{'name':'v110-e2e','version':'1'},
  'io.modelcontextprotocol/clientCapabilities':{'elicitation':{}},
}
def send(obj):
    p.stdin.write(json.dumps(obj,separators=(',',':'))+'\n'); p.stdin.flush()
def recv():
    line=p.stdout.readline()
    if not line: raise RuntimeError('MCP closed: '+p.stderr.read())
    return json.loads(line)
req={'jsonrpc':'2.0','id':1,'method':'tools/call','params':{'name':'gitmake_apply','arguments':{'project_dir':cwd,'plan_id':pid},'_meta':meta}}
send(req); first=recv()
r=first['result']; assert r['resultType']=='input_required',first
assert 'gitmake_approval' in r['inputRequests'],first
state=r['requestState']
retry={'jsonrpc':'2.0','id':2,'method':'tools/call','params':{'name':'gitmake_apply','arguments':{'project_dir':cwd,'plan_id':pid},'_meta':meta,'requestState':state,'inputResponses':{'gitmake_approval':{'action':'accept','content':{}}}}}
send(retry); done=recv(); assert done['result']['resultType']=='complete',done; assert done['result']['isError'] is False,done
# Replaying the exact accepted elicitation must not mint a second grant.
send({**retry,'id':3}); replay=recv(); assert replay['result']['isError'] is True,replay
assert 'already used' in replay['result']['content'][0]['text'],replay
p.stdin.close(); p.terminate(); p.wait(timeout=5)
PYMODERN
test -d "$TMP/remotes/testuser/ModernChatApproval.git"

# 2) Legacy 2025 stdio: server emits elicitation/create during gitmake_apply,
# Claude-style client accepts it, then the original tool call completes.
L="$TMP/LegacyChatApproval"; make_project "$L"
(cd "$L" && "$BIN" plan --json > "$TMP/legacy-plan.json")
LPID="$(python3 -c 'import json; print(json.load(open("'$TMP'/legacy-plan.json"))["plan_id"])')"
python3 - "$BIN" "$L" "$LPID" <<'PYLEGACY'
import json, os, subprocess, sys
bin,cwd,pid=sys.argv[1:]
p=subprocess.Popen([bin,'mcp','--allow-write'],cwd=cwd,env=os.environ.copy(),stdin=subprocess.PIPE,stdout=subprocess.PIPE,stderr=subprocess.PIPE,text=True,bufsize=1)
def send(obj): p.stdin.write(json.dumps(obj,separators=(',',':'))+'\n'); p.stdin.flush()
def recv():
    line=p.stdout.readline()
    if not line: raise RuntimeError('MCP closed: '+p.stderr.read())
    return json.loads(line)
send({'jsonrpc':'2.0','id':1,'method':'initialize','params':{'protocolVersion':'2025-11-25','capabilities':{'elicitation':{}},'clientInfo':{'name':'legacy-test','version':'1'}}})
init=recv(); assert init['result']['protocolVersion']=='2025-11-25',init
send({'jsonrpc':'2.0','method':'notifications/initialized'})
send({'jsonrpc':'2.0','id':2,'method':'tools/call','params':{'name':'gitmake_apply','arguments':{'project_dir':cwd,'plan_id':pid}}})
elicit=recv(); assert elicit['method']=='elicitation/create',elicit
send({'jsonrpc':'2.0','id':elicit['id'],'result':{'action':'accept','content':{}}})
done=recv(); assert done['id']==2,done; assert done['result']['isError'] is False,done
p.stdin.close(); p.terminate(); p.wait(timeout=5)
PYLEGACY
test -d "$TMP/remotes/testuser/LegacyChatApproval.git"

# 3) Legacy client without elicitation capability keeps the v1 terminal fallback.
F="$TMP/FallbackApproval"; make_project "$F"
(cd "$F" && "$BIN" plan --json > "$TMP/fallback-plan.json")
FPID="$(python3 -c 'import json; print(json.load(open("'$TMP'/fallback-plan.json"))["plan_id"])')"
python3 - "$BIN" "$F" "$FPID" <<'PYFALLBACK'
import json, os, subprocess, sys
bin,cwd,pid=sys.argv[1:]
p=subprocess.Popen([bin,'mcp','--allow-write'],cwd=cwd,env=os.environ.copy(),stdin=subprocess.PIPE,stdout=subprocess.PIPE,stderr=subprocess.PIPE,text=True,bufsize=1)
def send(o): p.stdin.write(json.dumps(o,separators=(',',':'))+'\n'); p.stdin.flush()
def recv(): return json.loads(p.stdout.readline())
send({'jsonrpc':'2.0','id':1,'method':'initialize','params':{'protocolVersion':'2025-11-25','capabilities':{},'clientInfo':{'name':'no-elicit','version':'1'}}}); recv()
send({'jsonrpc':'2.0','method':'notifications/initialized'})
send({'jsonrpc':'2.0','id':2,'method':'tools/call','params':{'name':'gitmake_apply','arguments':{'project_dir':cwd,'plan_id':pid}}})
r=recv(); assert r['result']['isError'] is True,r
assert 'gitmake approve' in r['result']['content'][0]['text'],r
p.stdin.close(); p.terminate(); p.wait(timeout=5)
PYFALLBACK

# 4) Modern discovery is available without initialize.
printf '%s\n' '{"jsonrpc":"2.0","id":"d1","method":"server/discover","params":{"_meta":{"io.modelcontextprotocol/protocolVersion":"2026-07-28","io.modelcontextprotocol/clientInfo":{"name":"test","version":"1"},"io.modelcontextprotocol/clientCapabilities":{}}}}' | "$BIN" mcp > "$TMP/discover.json"
grep -q '2026-07-28' "$TMP/discover.json"
grep -q 'resultType' "$TMP/discover.json"

echo V110_CHAT_APPROVAL_E2E_PASS
