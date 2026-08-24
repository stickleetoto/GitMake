#!/usr/bin/env bash
set -euo pipefail
ROOT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
BIN="$ROOT_DIR/dist/gitmake-e2e-v110"
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT
mkdir -p "$ROOT_DIR/dist" "$TMP/bin" "$TMP/remotes"
go build -o "$BIN" "$ROOT_DIR/cmd/gitmake"

cat > "$TMP/bin/gh" <<'GH'
#!/usr/bin/env bash
set -euo pipefail
root="${FAKE_GH_ROOT:?}"
if [[ "${1:-}" == "--version" ]]; then echo "gh version 2.fake"; exit 0; fi
if [[ "${1:-}" == "auth" && "${2:-}" == "status" ]]; then echo "Logged in"; exit 0; fi
if [[ "${1:-}" == "api" && "${2:-}" == "user" ]]; then echo "testuser"; exit 0; fi
if [[ "${1:-}" == "api" && "${2:-}" == repos/*/branches/*/protection ]]; then echo "HTTP 404: Branch not protected" >&2; exit 1; fi
if [[ "${1:-}" == "api" && "${2:-}" == repos/*/git/ref/tags/* ]]; then echo "HTTP 404: Not Found" >&2; exit 1; fi
if [[ "${1:-}" == "repo" && "${2:-}" == "view" ]]; then
  target="$3"; owner="${target%%/*}"; repo="${target#*/}"; remote="$root/$owner/$repo.git"
  if [[ ! -d "$remote" ]]; then echo "HTTP 404 repository not found" >&2; exit 1; fi
  url="https://example.test/$target"; vis="PRIVATE"; [[ -f "$remote.visibility" ]] && vis="$(cat "$remote.visibility")"
  if [[ " $* " == *" --jq .url "* ]]; then echo "$url"; exit 0; fi
  head="$(git --git-dir="$remote" symbolic-ref --short HEAD 2>/dev/null || true)"
  if [[ -n "$head" ]] && git --git-dir="$remote" rev-parse --verify "$head" >/dev/null 2>&1; then
    printf '{"nameWithOwner":"%s","url":"%s","visibility":"%s","defaultBranchRef":{"name":"%s"}}\n' "$target" "$url" "$vis" "$head"
  else
    printf '{"nameWithOwner":"%s","url":"%s","visibility":"%s","defaultBranchRef":null}\n' "$target" "$url" "$vis"
  fi
  exit 0
fi
if [[ "${1:-}" == "repo" && "${2:-}" == "clone" ]]; then
  target="$3"; dest="$4"; owner="${target%%/*}"; repo="${target#*/}"; git clone "$root/$owner/$repo.git" "$dest" >/dev/null 2>&1; exit 0
fi
if [[ "${1:-}" == "repo" && "${2:-}" == "create" ]]; then
  target="$3"; shift 3; source=""; vis="PRIVATE"
  while (($#)); do
    case "$1" in
      --source) source="$2"; shift 2;;
      --remote|--description) shift 2;;
      --public) vis="PUBLIC"; shift;;
      --internal) vis="INTERNAL"; shift;;
      --private|--push) shift;;
      *) shift;;
    esac
  done
  owner="${target%%/*}"; repo="${target#*/}"; remote="$root/$owner/$repo.git"
  mkdir -p "$(dirname "$remote")"; git init --bare "$remote" >/dev/null; echo "$vis" > "$remote.visibility"
  branch="$(git -C "$source" branch --show-current)"; git -C "$source" remote add origin "$remote"; git -C "$source" push -u origin "$branch" >/dev/null 2>&1; git --git-dir="$remote" symbolic-ref HEAD "refs/heads/$branch"
  echo "https://example.test/$target"; exit 0
fi
if [[ "${1:-}" == "release" && "${2:-}" == "view" ]]; then echo "release not found" >&2; exit 1; fi
if [[ "${1:-}" == "release" && "${2:-}" == "create" ]]; then echo "https://example.test/release"; exit 0; fi
echo "fake gh unsupported: $*" >&2; exit 2
GH
chmod +x "$TMP/bin/gh"
export PATH="$TMP/bin:$PATH" FAKE_GH_ROOT="$TMP/remotes" GIT_CONFIG_GLOBAL="$TMP/gitconfig" XDG_CACHE_HOME="$TMP/cache"
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
