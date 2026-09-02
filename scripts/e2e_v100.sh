#!/usr/bin/env bash
set -euo pipefail
ROOT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
BIN="$ROOT_DIR/dist/gitmake-e2e-v100"
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT
mkdir -p "$ROOT_DIR/dist" "$TMP/bin" "$TMP/remotes"
go build -o "$BIN" "$ROOT_DIR/cmd/gitmake"

source "$(dirname "${BASH_SOURCE[0]}")/fakegh.sh"
install_fake_gh "$TMP/bin" "$TMP/remotes"
export GIT_CONFIG_GLOBAL="$TMP/gitconfig" XDG_CACHE_HOME="$TMP/cache"
require_fake_gh "$BIN"
git config --global user.name "GitMake V100"
git config --global user.email "v100@example.test"

# 1) Decision explanations are machine-readable and supported by actual state.
W="$TMP/WhyProject"; mkdir -p "$W/src"
printf '# Why\n' > "$W/README.md"; printf 'print("why")\n' > "$W/src/main.py"
(cd "$W" && "$BIN" plan --json > "$TMP/why.json")
python - "$TMP/why.json" <<'PY'
import json,sys
p=json.load(open(sys.argv[1],encoding='utf-8'))
notes='\n'.join(p.get('decision_notes',[]))
assert 'inferred in memory' in notes,notes
assert 'Private visibility' in notes,notes
assert p['risk']['level']=='low',p
PY

# 2) Low-risk simple publish stays one-key and renders a compact success card.
(cd "$W" && "$BIN" . --yes > "$TMP/low.out")
grep -q '^✓ Published WhyProject' "$TMP/low.out"
grep -q '^Repository  testuser/WhyProject' "$TMP/low.out"
grep -q '^Changes     +' "$TMP/low.out"
grep -q '^Why$' "$TMP/low.out"
! grep -q '^✓ Source validated' "$TMP/low.out"

# 3) Medium risk (visibility mismatch) requires typing PUBLISH; --yes cannot bypass it.
cat > "$W/gitmake.json" <<'JSON'
{"repo":{"name":"WhyProject","visibility":"public"},"source":{"folder":"."},"git":{"branch":"main"}}
JSON
printf 'medium\n' > "$W/src/medium.txt"
if (cd "$W" && "$BIN" . --yes >"$TMP/medium_noninteractive.out" 2>&1); then exit 41; fi
grep -q 'medium-risk plan requires interactive confirmation' "$TMP/medium_noninteractive.out"
python - "$BIN" "$W" <<'PY'
import os,pty,select,subprocess,sys
bin,cwd=sys.argv[1:]
master,slave=pty.openpty(); p=subprocess.Popen([bin],cwd=cwd,stdin=slave,stdout=slave,stderr=slave,env=os.environ.copy(),close_fds=True); os.close(slave)
out=[]; sent=False
while p.poll() is None:
    r,_,_=select.select([master],[],[],0.1)
    if r:
        try: data=os.read(master,4096)
        except OSError: break
        out.append(data); text=b''.join(out).decode(errors='replace')
        if not sent and 'Type PUBLISH to continue:' in text:
            os.write(master,b'PUBLISH\n'); sent=True
p.wait()
try:
    while True: out.append(os.read(master,4096))
except OSError: pass
os.close(master); text=b''.join(out).decode(errors='replace')
if p.returncode: raise SystemExit(text)
assert sent,text
assert '✓ Published WhyProject' in text,text
PY

# Restore config visibility to remote reality before destructive test uses its own repo.
rm -f "$W/gitmake.json"

# 4) High/destructive update requires a per-plan DELETE-xxxxxx phrase and succeeds only after it.
D="$TMP/DestructiveProject"; mkdir -p "$D/src"
printf '# Destructive\n' > "$D/README.md"
for i in $(seq 1 15); do printf 'file %s\n' "$i" > "$D/src/file$i.txt"; done
(cd "$D" && "$BIN" . --yes >/dev/null)
for i in $(seq 1 12); do rm "$D/src/file$i.txt"; done
python - "$BIN" "$D" <<'PY'
import os,pty,re,select,subprocess,sys
bin,cwd=sys.argv[1:]
master,slave=pty.openpty(); p=subprocess.Popen([bin],cwd=cwd,stdin=slave,stdout=slave,stderr=slave,env=os.environ.copy(),close_fds=True); os.close(slave)
out=[]; sent=False
while p.poll() is None:
    r,_,_=select.select([master],[],[],0.1)
    if r:
        try: data=os.read(master,4096)
        except OSError: break
        out.append(data); text=b''.join(out).decode(errors='replace')
        m=re.search(r'Type (DELETE-[A-F0-9]{6}) to confirm:',text)
        if m and not sent:
            os.write(master,(m.group(1)+'\n').encode()); sent=True
p.wait()
try:
    while True: out.append(os.read(master,4096))
except OSError: pass
os.close(master); text=b''.join(out).decode(errors='replace')
if p.returncode: raise SystemExit(text)
assert sent,text
assert 'HIGH' in text,text
assert '✓ Published DestructiveProject' in text,text
PY
R="$TMP/remotes/testuser/DestructiveProject.git"
! git --git-dir="$R" cat-file -e HEAD:src/file1.txt 2>/dev/null
git --git-dir="$R" cat-file -e HEAD:src/file13.txt

# 5) Safety failures now provide actionable guided recovery instead of a bare error.
S="$TMP/SecretProject"; mkdir -p "$S/secrets"
printf '# Secret\n' > "$S/README.md"
printf '%s%s\n%s\n%s%s\n' '-----BEGIN ' 'PRIVATE KEY-----' 'FAKE_TEST_ONLY' '-----END ' 'PRIVATE KEY-----' > "$S/secrets/key.txt"
if (cd "$S" && "$BIN" . --yes >"$TMP/secret.out" 2>&1); then exit 51; fi
grep -q 'SECRET_DETECTED' "$TMP/secret.out"
grep -q '^Recommended' "$TMP/secret.out"
grep -q '.gitmakeignore' "$TMP/secret.out"
grep -q 'Nothing was published' "$TMP/secret.out"

# 6) Windows first-run Setup still cross-compiles after readiness UX changes.
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -o "$TMP/GitMake-Setup.exe" "$ROOT_DIR/cmd/setup"
test -s "$TMP/GitMake-Setup.exe"

echo V0100_GUIDED_UX_E2E_PASS

# 7) Tokenless MCP approval: latest plan is approved locally, no token is copied, and grant is single-use.
A="$TMP/ApprovalProject"; mkdir -p "$A/src"
printf '# Approval\n' > "$A/README.md"; printf 'print("ok")\n' > "$A/src/main.py"
(cd "$A" && "$BIN" plan --json > "$TMP/approval-plan.json")
PID="$(python -c 'import json; print(json.load(open("'"$TMP"'/approval-plan.json"))["plan_id"])')"
python - "$BIN" "$A" <<'PYAPPROVE'
import os,pty,select,subprocess,sys
bin,cwd=sys.argv[1:]
master,slave=pty.openpty(); p=subprocess.Popen([bin,'approve'],cwd=cwd,stdin=slave,stdout=slave,stderr=slave,env=os.environ.copy(),close_fds=True); os.close(slave)
out=[]; sent=False
while p.poll() is None:
    r,_,_=select.select([master],[],[],0.1)
    if r:
        try: data=os.read(master,4096)
        except OSError: break
        out.append(data); text=b''.join(out).decode(errors='replace')
        if not sent and 'Approve this reviewed plan? [Y/n]:' in text:
            os.write(master,b'y\n'); sent=True
p.wait()
try:
    while True: out.append(os.read(master,4096))
except OSError: pass
os.close(master); text=b''.join(out).decode(errors='replace')
if p.returncode: raise SystemExit(text)
assert sent,text
assert 'No token to copy' in text,text
assert 'gma_' not in text,text
PYAPPROVE
python - "$BIN" "$A" "$PID" <<'PYMCP'
import json,os,subprocess,sys
bin,cwd,pid=sys.argv[1:]
def call():
    req={"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"gitmake_apply","arguments":{"project_dir":cwd,"plan_id":pid}}}
    p=subprocess.run([bin,'mcp','--allow-write'],cwd=cwd,env=os.environ.copy(),input=json.dumps(req)+'\n',text=True,capture_output=True)
    if p.returncode: raise SystemExit(p.stderr)
    return json.loads(p.stdout)
r=call(); assert r['result']['isError'] is False,r
r2=call(); assert r2['result']['isError'] is True,r2
assert 'already used' in r2['result']['content'][0]['text'],r2
PYMCP
test -d "$TMP/remotes/testuser/ApprovalProject.git"

echo V100_TOKENLESS_STABILITY_E2E_PASS
