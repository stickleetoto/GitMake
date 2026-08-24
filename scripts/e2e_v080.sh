#!/usr/bin/env bash
set -euo pipefail
ROOT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
BIN="$ROOT_DIR/dist/gitmake-e2e-v080"
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
git config --global user.name "GitMake V073"
git config --global user.email "v073@example.test"

# 1) A stale real repo config must NEVER auto-retarget to the only ZIP beside it.
S="$TMP/source_mismatch"; mkdir -p "$S"
printf '%s\n' '{"repo":{"name":"GitMake"},"source":{"zip":"GitMake_Source.zip"},"git":{}}' > "$S/gitmake.json"
python - "$S/UnrelatedProject.zip" <<'PY'
import sys, zipfile
with zipfile.ZipFile(sys.argv[1], 'w') as z: z.writestr('README.md', '# unrelated\n')
PY
if (cd "$S" && "$BIN" --json >out.json); then exit 31; fi
grep -q 'PROJECT_SOURCE_MISMATCH' "$S/out.json"
test ! -d "$TMP/remotes/testuser/GitMake.git"

# 2) Build an ordinary managed repo with enough files to exercise the destructive threshold.
D="$TMP/destructive"; mkdir -p "$D"
python - "$D/Project.zip" <<'PY'
import sys, zipfile
with zipfile.ZipFile(sys.argv[1], 'w', zipfile.ZIP_DEFLATED) as z:
    z.writestr('root/README.md', '# destructive gate test\n')
    for i in range(1, 41): z.writestr(f'root/src/file_{i:02d}.txt', f'v1-{i}\n')
PY
printf '%s\n' '{"repo":{"name":"DestructiveGate","visibility":"private"},"source":{"zip":"Project.zip"},"git":{},"sync":{"mode":"managed"}}' > "$D/gitmake.json"
(cd "$D" && "$BIN" >/dev/null)
# Identity must be committed on first publish.
git --git-dir="$TMP/remotes/testuser/DestructiveGate.git" show HEAD:.gitmake/project.json | grep -q 'testuser/DestructiveGate'
# Simulate a remote visibility mismatch; update must report it but never mutate visibility.
echo PUBLIC > "$TMP/remotes/testuser/DestructiveGate.git.visibility"

# Replace snapshot with only 5 managed files: 36/41 managed files disappear (>30%, >=10 files).
python - "$D/Project.zip" <<'PY'
import sys, zipfile
with zipfile.ZipFile(sys.argv[1], 'w', zipfile.ZIP_DEFLATED) as z:
    z.writestr('root/README.md', '# destructive gate test v2\n')
    for i in range(1, 5): z.writestr(f'root/src/file_{i:02d}.txt', f'v2-{i}\n')
PY
(cd "$D" && "$BIN" plan --json >plan.json)
python - "$D/plan.json" "$D" <<'PY'
import json, os, sys
p=json.load(open(sys.argv[1], encoding='utf-8')); d=os.path.abspath(sys.argv[2])
assert p['mode']=='UPDATE', p
assert p['repository']=='testuser/DestructiveGate', p
assert p['working_directory']==d, p
assert p['config_path']==os.path.join(d,'gitmake.json'), p
assert p['source_path']==os.path.join(d,'Project.zip'), p
assert p['remote_visibility']=='public', p
assert p['project_identity']['status']=='verified', p
assert p['project_identity']['repository']=='testuser/DestructiveGate', p
assert p['risk']['destructive'] is True, p
assert p['risk']['level']=='high', p
assert p['risk']['managed_baseline'] >= 40, p
assert p['changes']['deleted'] >= 30, p
assert p['review_notes'], p
print(p['plan_id'])
PY
PID="$(python -c 'import json; print(json.load(open("'"$D"'/plan.json"))["plan_id"])')"

# Direct publish and ordinary apply are hard-blocked.
if (cd "$D" && "$BIN" --json >blocked_publish.json); then exit 32; fi
grep -q 'DESTRUCTIVE_CHANGE_BLOCKED' "$D/blocked_publish.json"
if (cd "$D" && "$BIN" apply "$PID" --json >blocked_apply.json); then exit 33; fi
grep -q 'DESTRUCTIVE_CHANGE_BLOCKED' "$D/blocked_apply.json"
# Ordinary approval cannot mint a token for a destructive plan.
if (cd "$D" && "$BIN" approve "$PID" --json >blocked_approve.json); then exit 34; fi
grep -q 'DESTRUCTIVE_CHANGE_BLOCKED' "$D/blocked_approve.json"

# A human TTY can create a destructive-class tokenless one-shot grant.
python - "$BIN" "$D" "$PID" <<'PYAPPROVE'
import os, pty, re, select, subprocess, sys
bin,cwd,pid=sys.argv[1:]
master,slave=pty.openpty()
p=subprocess.Popen([bin,'approve',pid,'--destructive'],cwd=cwd,stdin=slave,stdout=slave,stderr=slave,env=os.environ.copy(),close_fds=True)
os.close(slave)
chunks=[]; sent=False
while p.poll() is None:
    r,_,_=select.select([master],[],[],0.1)
    if r:
        try: data=os.read(master,4096)
        except OSError: break
        chunks.append(data); text=b''.join(chunks).decode(errors='replace')
        m=re.search(r'Type (DELETE-[A-F0-9]{6}) to confirm:',text)
        if m and not sent:
            os.write(master,(m.group(1)+'\n').encode()); sent=True
p.wait()
try:
    while True: chunks.append(os.read(master,4096))
except OSError: pass
os.close(master)
out=b''.join(chunks).decode(errors='replace')
if p.returncode: raise SystemExit(out)
assert sent,out
assert 'No token to copy' in out,out
PYAPPROVE

# MCP apply consumes the destructive-class local grant once and only once.
python - "$BIN" "$D" "$PID" <<'PYMCP'
import json, os, subprocess, sys
bin,cwd,pid=sys.argv[1:]
def call():
    req={"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"gitmake_apply","arguments":{"project_dir":cwd,"plan_id":pid}}}
    p=subprocess.run([bin,'mcp','--allow-write'],cwd=cwd,env=os.environ.copy(),input=json.dumps(req)+'\n',text=True,capture_output=True)
    if p.returncode: raise SystemExit(p.stderr)
    return json.loads(p.stdout)
r=call()
assert r['result']['isError'] is False, r
r2=call()
assert r2['result']['isError'] is True, r2
assert 'already used' in r2['result']['content'][0]['text'], r2
PYMCP
# Remote visibility remains public metadata; GitMake update never changes it.
test "$(cat "$TMP/remotes/testuser/DestructiveGate.git.visibility")" = PUBLIC
# Deleted file is actually gone after explicitly approved destructive apply.
! git --git-dir="$TMP/remotes/testuser/DestructiveGate.git" cat-file -e HEAD:src/file_40.txt 2>/dev/null

# 3) Tampering the committed project identity to another valid repository binding is a hard stop.
I="$TMP/identity"; mkdir -p "$I"
python - "$I/Identity.zip" <<'PY'
import sys, zipfile
with zipfile.ZipFile(sys.argv[1], 'w') as z: z.writestr('root/a.txt','a')
PY
printf '%s\n' '{"repo":{"name":"IdentityRepo"},"source":{"zip":"Identity.zip"},"git":{}}' > "$I/gitmake.json"
(cd "$I" && "$BIN" >/dev/null)
CL="$TMP/identityclone"; git clone "$TMP/remotes/testuser/IdentityRepo.git" "$CL" >/dev/null 2>&1
python - "$CL/.gitmake/project.json" <<'PY'
import hashlib,json,sys
repo='testuser/OtherRepo'; pid='gmp_'+hashlib.sha256(repo.lower().encode()).digest()[:8].hex()
json.dump({'schema':'gitmake.project/v1','project_id':pid,'repository':repo,'bound_at':'2026-08-21T00:00:00Z'},open(sys.argv[1],'w'),indent=2); open(sys.argv[1],'a').write('\n')
PY
git -C "$CL" add .gitmake/project.json; git -C "$CL" commit -m tamper >/dev/null; git -C "$CL" push >/dev/null 2>&1
if (cd "$I" && "$BIN" plan --json >identity_block.json); then exit 35; fi
grep -q 'PROJECT_IDENTITY_MISMATCH' "$I/identity_block.json"


# 4) ZIP-only MCP authoring path: inspect -> suggest -> write -> plan, with no hand-authored config.
Z="$TMP/ziponly"; mkdir -p "$Z"
python - "$Z/NebulaNotes_v1.zip" <<'PYZIP'
import sys, zipfile
with zipfile.ZipFile(sys.argv[1], 'w', zipfile.ZIP_DEFLATED) as z:
    z.writestr('README.md','# Nebula Notes\n')
    z.writestr('src/main.py','print("hello")\n')
PYZIP
python - "$BIN" "$Z" <<'PYMCPZIP'
import json, os, subprocess, sys
bin,cwd=sys.argv[1:]
def tool(name,args=None,write=False):
    req={"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":name,"arguments":args or {}}}
    cmd=[bin,'mcp']+(['--allow-write'] if write else [])
    p=subprocess.run(cmd,cwd=cwd,env=os.environ.copy(),input=json.dumps(req)+'\n',text=True,capture_output=True)
    if p.returncode: raise SystemExit(p.stderr)
    r=json.loads(p.stdout)
    assert r['result']['isError'] is False, r
    return json.loads(r['result']['content'][0]['text'])
inspect=tool('gitmake_project_inspect',{'project_dir':cwd})
assert inspect['config']['present'] is False, inspect
assert inspect['discovery']['selected_source']=='NebulaNotes_v1.zip', inspect
suggest=tool('gitmake_config_suggest',{'project_dir':cwd,'visibility':'private','branch':'main'})
cfg=suggest['config']
assert cfg['repo']['name']=='NebulaNotes', cfg
written=tool('gitmake_config_write',{'project_dir':cwd,'config':cfg},write=True)
assert written['ok'] is True, written
plan=tool('gitmake_plan',{'project_dir':cwd})
assert plan['mode']=='CREATE', plan
assert plan['repository']=='testuser/NebulaNotes', plan
assert plan['working_directory']==os.path.abspath(cwd), plan
assert plan['source_path']==os.path.join(os.path.abspath(cwd),'NebulaNotes_v1.zip'), plan
assert plan['risk']['destructive'] is False, plan
assert plan['changes']['added']==2, plan
PYMCPZIP

# 5) High-level gitmake_prepare: one MCP call must reach a reviewed plan without host filesystem writes.
P="$TMP/prepare_readonly"; mkdir -p "$P"
python - "$P/OrbitPad.zip" <<'PYZIP'
import sys, zipfile
with zipfile.ZipFile(sys.argv[1], 'w', zipfile.ZIP_DEFLATED) as z:
    z.writestr('README.md','# Orbit Pad\n')
    z.writestr('src/main.py','print("orbit")\n')
PYZIP
python - "$BIN" "$P" <<'PYPREP'
import json, os, subprocess, sys
bin,cwd=sys.argv[1:]
req={"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"gitmake_prepare","arguments":{"project_dir":cwd}}}
p=subprocess.run([bin,'mcp'],cwd=cwd,env=os.environ.copy(),input=json.dumps(req)+'\n',text=True,capture_output=True)
if p.returncode: raise SystemExit(p.stderr)
r=json.loads(p.stdout)
assert r['result']['isError'] is False, r
body=r['result']['structuredContent']
assert body['schema']=='gitmake.prepare/v1', body
assert body['status']=='ready_for_approval', body
assert body['access']['mcp_write_enabled'] is False, body
assert body['access']['project_config_mutated'] is False, body
assert body['access']['github_mutated'] is False, body
assert body['config']['mode']=='in_memory', body
assert body['config']['persisted'] is False, body
assert body['plan']['repository']=='testuser/OrbitPad', body
assert body['plan']['mode']=='CREATE', body
assert body['plan']['changes']['added']==2, body
assert body['plan']['risk']['destructive'] is False, body
assert not os.path.exists(os.path.join(cwd,'gitmake.json'))
assert 'zero-config' in body['note'], body
print(body['plan']['plan_id'])
PYPREP

# 6) With explicit MCP write access, the same high-level tool may persist config through GitMake's atomic writer.
PW="$TMP/prepare_write"; mkdir -p "$PW"
python - "$PW/CometBoard.zip" <<'PYZIP'
import sys, zipfile
with zipfile.ZipFile(sys.argv[1], 'w', zipfile.ZIP_DEFLATED) as z:
    z.writestr('README.md','# Comet Board\n')
    z.writestr('src/app.txt','comet\n')
PYZIP
python - "$BIN" "$PW" <<'PYPREPWRITE'
import json, os, subprocess, sys
bin,cwd=sys.argv[1:]
req={"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"gitmake_prepare","arguments":{"project_dir":cwd,"persist_config":True}}}
p=subprocess.run([bin,'mcp','--allow-write'],cwd=cwd,env=os.environ.copy(),input=json.dumps(req)+'\n',text=True,capture_output=True)
if p.returncode: raise SystemExit(p.stderr)
r=json.loads(p.stdout)
assert r['result']['isError'] is False, r
body=r['result']['structuredContent']
assert body['access']['mcp_write_enabled'] is True, body
assert body['access']['project_config_mutated'] is True, body
assert body['config']['mode']=='gitmake_authored', body
assert body['config']['persisted'] is True, body
assert os.path.isfile(os.path.join(cwd,'gitmake.json'))
assert body['plan']['repository']=='testuser/CometBoard', body
assert body['plan']['mode']=='CREATE', body
PYPREPWRITE

echo V073_SAFETY_E2E_PASS

# 7) Folder mode publishes the live source tree while honoring ignore rules.
F="$TMP/folder_publish"; mkdir -p "$F/src" "$F/node_modules"
printf '# Folder Publish\n' > "$F/README.md"
printf 'print("folder")\n' > "$F/src/main.py"
printf 'node dep\n' > "$F/node_modules/dep.js"
printf 'TOKEN=local-only\n' > "$F/.env"
printf 'ignore me\n' > "$F/debug.log"
printf 'debug.log\n' > "$F/.gitignore"
(cd "$F" && "$BIN" . >/dev/null)
test ! -f "$F/gitmake.json"
test -f "$F/.gitmake/project.json"
R="$TMP/remotes/testuser/folder_publish.git"
test -d "$R"
git --git-dir="$R" cat-file -e HEAD:README.md
git --git-dir="$R" cat-file -e HEAD:src/main.py
git --git-dir="$R" cat-file -e HEAD:.gitignore
! git --git-dir="$R" cat-file -e HEAD:gitmake.json 2>/dev/null
! git --git-dir="$R" cat-file -e HEAD:.env 2>/dev/null
! git --git-dir="$R" cat-file -e HEAD:node_modules/dep.js 2>/dev/null
! git --git-dir="$R" cat-file -e HEAD:debug.log 2>/dev/null

# 8) Folder plans carry source_mode=folder and ignored changes do not stale them.
printf 'v2\n' > "$F/src/new.txt"
(cd "$F" && "$BIN" plan --json >"$TMP/folder_plan.json")
python - "$TMP/folder_plan.json" "$F" <<'PYFOLDERPLAN'
import json,os,sys
p=json.load(open(sys.argv[1],encoding='utf-8')); root=os.path.abspath(sys.argv[2])
assert p['source_mode']=='folder', p
assert p['source_path']==root, p
assert p['mode']=='UPDATE', p
assert p['changes']['added']>=1, p
print(p['plan_id'])
PYFOLDERPLAN
FPID="$(python -c 'import json; print(json.load(open("'"$TMP"'/folder_plan.json"))["plan_id"])')"
printf 'changed but ignored\n' > "$F/debug.log"
(cd "$F" && "$BIN" apply "$FPID" --json >"$TMP/folder_apply.json")
git --git-dir="$R" cat-file -e HEAD:src/new.txt
! git --git-dir="$R" cat-file -e HEAD:debug.log 2>/dev/null

# 9) Included folder changes invalidate a reviewed plan.
printf 'v3\n' > "$F/src/stale.txt"
(cd "$F" && "$BIN" plan --json >"$TMP/stale_plan.json")
SPID="$(python -c 'import json; print(json.load(open("'"$TMP"'/stale_plan.json"))["plan_id"])')"
printf 'changed after review\n' >> "$F/src/main.py"
if (cd "$F" && "$BIN" apply "$SPID" --json >"$TMP/stale_apply.json"); then exit 41; fi
grep -q 'PLAN_STALE' "$TMP/stale_apply.json"

# 10) High-level MCP prepare can infer a folder source with no ZIP and no config.
MF="$TMP/mcp_folder"; mkdir -p "$MF/src"
printf '# MCP Folder\n' > "$MF/README.md"
printf 'package main\nfunc main(){}\n' > "$MF/src/main.go"
printf 'module example.test/MCPFolder\n\ngo 1.22\n' > "$MF/go.mod"
python - "$BIN" "$MF" <<'PYMCPFOLDER'
import json,os,subprocess,sys
bin,cwd=sys.argv[1:]
req={"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"gitmake_prepare","arguments":{"project_dir":cwd}}}
p=subprocess.run([bin,'mcp'],cwd=cwd,env=os.environ.copy(),input=json.dumps(req)+'\n',text=True,capture_output=True)
if p.returncode: raise SystemExit(p.stderr)
r=json.loads(p.stdout); assert r['result']['isError'] is False, r
body=r['result']['structuredContent']; plan=body['plan']
assert plan['source_mode']=='folder', plan
assert plan['source_path']==os.path.abspath(cwd), plan
assert plan['mode']=='CREATE', plan
assert plan['changes']['added']==3, plan
assert body['config']['mode']=='in_memory', body
assert not os.path.exists(os.path.join(cwd,'gitmake.json'))
PYMCPFOLDER

echo V080_FOLDER_E2E_PASS
