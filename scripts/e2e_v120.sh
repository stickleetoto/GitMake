#!/usr/bin/env bash
set -euo pipefail
ROOT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
BIN="$ROOT_DIR/dist/gitmake-e2e-v120"
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
git config --global user.name "GitMake V120"
git config --global user.email "v120@example.test"

make_project() {
  local dir="$1"
  mkdir -p "$dir/src"
  printf '# One-shot publish\n' > "$dir/README.md"
  printf 'print("one-shot publish")\n' > "$dir/src/main.py"
}

# 1) Modern MCP: ONE gitmake_publish operation prepares, elicits, and applies.
M="$TMP/ModernOneShot"; make_project "$M"
python3 - "$BIN" "$M" <<'PYMODERN'
import json, os, subprocess, sys
bin,cwd=sys.argv[1:]
p=subprocess.Popen([bin,'mcp','--allow-write'],cwd=cwd,env=os.environ.copy(),stdin=subprocess.PIPE,stdout=subprocess.PIPE,stderr=subprocess.PIPE,text=True,bufsize=1)
meta={
  'io.modelcontextprotocol/protocolVersion':'2026-07-28',
  'io.modelcontextprotocol/clientInfo':{'name':'v120-e2e','version':'1'},
  'io.modelcontextprotocol/clientCapabilities':{'elicitation':{}},
}
def send(o): p.stdin.write(json.dumps(o,separators=(',',':'))+'\n'); p.stdin.flush()
def recv():
    line=p.stdout.readline()
    if not line: raise RuntimeError('MCP closed: '+p.stderr.read())
    return json.loads(line)
args={'project_dir':cwd}
first_req={'jsonrpc':'2.0','id':1,'method':'tools/call','params':{'name':'gitmake_publish','arguments':args,'_meta':meta}}
send(first_req); first=recv()
r=first['result']; assert r['resultType']=='input_required',first
assert 'gitmake_approval' in r['inputRequests'],first
state=r['requestState']; assert state
retry={'jsonrpc':'2.0','id':2,'method':'tools/call','params':{'name':'gitmake_publish','arguments':args,'_meta':meta,'requestState':state,'inputResponses':{'gitmake_approval':{'action':'accept','content':{}}}}}
send(retry); done=recv()
assert done['result']['resultType']=='complete',done
assert done['result']['isError'] is False,done
sc=done['result']['structuredContent']
assert sc['schema']=='gitmake.publish/v1' and sc['status']=='published',sc
assert sc['repository']=='testuser/ModernOneShot',sc
# Exact approval replay must fail after the one-shot grant was consumed.
send({**retry,'id':3}); replay=recv()
assert replay['result']['isError'] is True,replay
assert 'already used' in replay['result']['content'][0]['text'],replay
p.stdin.close(); p.terminate(); p.wait(timeout=5)
PYMODERN
test -d "$TMP/remotes/testuser/ModernOneShot.git"

# 2) Legacy MCP: the original gitmake_publish request stays pending while the
# Claude-style elicitation dialog is answered, then returns the final publish.
L="$TMP/LegacyOneShot"; make_project "$L"
python3 - "$BIN" "$L" <<'PYLEGACY'
import json, os, subprocess, sys
bin,cwd=sys.argv[1:]
p=subprocess.Popen([bin,'mcp','--allow-write'],cwd=cwd,env=os.environ.copy(),stdin=subprocess.PIPE,stdout=subprocess.PIPE,stderr=subprocess.PIPE,text=True,bufsize=1)
def send(o): p.stdin.write(json.dumps(o,separators=(',',':'))+'\n'); p.stdin.flush()
def recv():
    line=p.stdout.readline()
    if not line: raise RuntimeError('MCP closed: '+p.stderr.read())
    return json.loads(line)
send({'jsonrpc':'2.0','id':1,'method':'initialize','params':{'protocolVersion':'2025-11-25','capabilities':{'elicitation':{}},'clientInfo':{'name':'legacy-test','version':'1'}}})
init=recv(); assert init['result']['protocolVersion']=='2025-11-25',init
send({'jsonrpc':'2.0','method':'notifications/initialized'})
send({'jsonrpc':'2.0','id':2,'method':'tools/call','params':{'name':'gitmake_publish','arguments':{'project_dir':cwd}}})
elicit=recv(); assert elicit['method']=='elicitation/create',elicit
send({'jsonrpc':'2.0','id':elicit['id'],'result':{'action':'accept','content':{}}})
done=recv(); assert done['id']==2,done; assert done['result']['isError'] is False,done
sc=done['result']['structuredContent']; assert sc['schema']=='gitmake.publish/v1' and sc['status']=='published',sc
p.stdin.close(); p.terminate(); p.wait(timeout=5)
PYLEGACY
test -d "$TMP/remotes/testuser/LegacyOneShot.git"

# 3) No elicitation capability: high-level publish refuses to bypass the human
# boundary and points the agent to the stable prepare/approve/apply fallback.
F="$TMP/NoElicitation"; make_project "$F"
python3 - "$BIN" "$F" <<'PYFALLBACK'
import json, os, subprocess, sys
bin,cwd=sys.argv[1:]
p=subprocess.Popen([bin,'mcp','--allow-write'],cwd=cwd,env=os.environ.copy(),stdin=subprocess.PIPE,stdout=subprocess.PIPE,stderr=subprocess.PIPE,text=True,bufsize=1)
def send(o): p.stdin.write(json.dumps(o,separators=(',',':'))+'\n'); p.stdin.flush()
def recv(): return json.loads(p.stdout.readline())
send({'jsonrpc':'2.0','id':1,'method':'initialize','params':{'protocolVersion':'2025-11-25','capabilities':{},'clientInfo':{'name':'no-elicit','version':'1'}}}); recv()
send({'jsonrpc':'2.0','method':'notifications/initialized'})
send({'jsonrpc':'2.0','id':2,'method':'tools/call','params':{'name':'gitmake_publish','arguments':{'project_dir':cwd}}})
r=recv(); assert r['result']['isError'] is True,r
text=r['result']['content'][0]['text']
assert 'gitmake_prepare' in text and 'gitmake approve' in text and 'gitmake_apply' in text,text
p.stdin.close(); p.terminate(); p.wait(timeout=5)
PYFALLBACK

echo V120_ONE_SHOT_PUBLISH_E2E_PASS
