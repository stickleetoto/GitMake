#!/usr/bin/env bash
set -euo pipefail
ROOT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
BIN="$ROOT_DIR/dist/gitmake-e2e-v07"
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT
mkdir -p "$ROOT_DIR/dist" "$TMP/bin" "$TMP/remotes" "$TMP/releases"
go build -o "$BIN" "$ROOT_DIR/cmd/gitmake"

source "$(dirname "${BASH_SOURCE[0]}")/fakegh.sh"
install_fake_gh "$TMP/bin" "$TMP/remotes"
export GIT_CONFIG_GLOBAL="$TMP/gitconfig"
require_fake_gh "$BIN"
git config --global user.name "GitMake V07"
git config --global user.email "v07@example.test"

makezip() {
  local zip="$1"; shift
  python - "$zip" "$@" <<'PY'
import sys, zipfile
with zipfile.ZipFile(sys.argv[1], 'w', zipfile.ZIP_DEFLATED) as z:
  for item in sys.argv[2:]:
    name, body = item.split('=',1); z.writestr(name, body)
PY
}

# 1) Managed sync protects remote-only files while deleting previously managed files.
P="$TMP/managed"; mkdir -p "$P"
makezip "$P/Managed.zip" 'root/a.txt=a1' 'root/old.txt=old'
printf '%s\n' '{"repo":{"name":"Managed"},"source":{"zip":"Managed.zip"},"git":{},"sync":{"mode":"managed"}}' > "$P/gitmake.json"
(cd "$P" && "$BIN" >/dev/null)
# Add a remote-only workflow and manual file through a normal Git commit.
CL="$TMP/manualclone"; git clone "$TMP/remotes/testuser/Managed.git" "$CL" >/dev/null 2>&1
mkdir -p "$CL/.github/workflows"; echo keep > "$CL/.github/workflows/ci.yml"; echo manual > "$CL/manual.md"
git -C "$CL" add -A; git -C "$CL" commit -m manual >/dev/null; git -C "$CL" push >/dev/null 2>&1
makezip "$P/Managed.zip" 'root/a.txt=a2' 'root/new.txt=new'
(cd "$P" && "$BIN" >/dev/null)
TREE="$(git --git-dir="$TMP/remotes/testuser/Managed.git" ls-tree -r --name-only HEAD)"
grep -qx '.github/workflows/ci.yml' <<<"$TREE"; grep -qx 'manual.md' <<<"$TREE"; ! grep -qx 'old.txt' <<<"$TREE"; grep -qx 'new.txt' <<<"$TREE"

# 2) Secret scan blocks before GitHub mutation.
S="$TMP/secret"; mkdir -p "$S"; makezip "$S/Secret.zip" 'root/.env=TOKEN=abc' 'root/app.txt=x'
printf '%s\n' '{"repo":{"name":"SecretRepo"},"source":{"zip":"Secret.zip"},"git":{}}' > "$S/gitmake.json"
if (cd "$S" && "$BIN" --json >out.json); then exit 21; fi
grep -q 'SECRET_DETECTED' "$S/out.json"; test ! -d "$TMP/remotes/testuser/SecretRepo.git"

# 3) Large direct-Git file gate can be tested with tiny thresholds.
L="$TMP/large"; mkdir -p "$L"; makezip "$L/Large.zip" 'root/big.bin=abcdefghijklmnopqrstuvwxyz'
printf '%s\n' '{"repo":{"name":"LargeRepo"},"source":{"zip":"Large.zip"},"git":{},"security":{"warn_file_bytes":10,"max_git_file_bytes":20}}' > "$L/gitmake.json"
if (cd "$L" && "$BIN" --json >out.json); then exit 22; fi
grep -q 'LARGE_FILE_BLOCKED' "$L/out.json"; test ! -d "$TMP/remotes/testuser/LargeRepo.git"

# 4) Protected branch requiring PR is detected and never bypassed.
B="$TMP/protected"; mkdir -p "$B"; makezip "$B/Protected.zip" 'root/a.txt=v1'
printf '%s\n' '{"repo":{"name":"Protected"},"source":{"zip":"Protected.zip"},"git":{}}' > "$B/gitmake.json"
(cd "$B" && "$BIN" >/dev/null)
makezip "$B/Protected.zip" 'root/a.txt=v2'
COUNT="$(git --git-dir="$TMP/remotes/testuser/Protected.git" rev-list --count HEAD)"
if (cd "$B" && FAKE_GH_REQUIRE_PR=1 "$BIN" --json >out.json); then exit 23; fi
grep -q 'BRANCH_REQUIRES_PR' "$B/out.json"; test "$(git --git-dir="$TMP/remotes/testuser/Protected.git" rev-list --count HEAD)" = "$COUNT"

# 5) Bare tag conflict is blocked before attaching a release to an old tag.
T="$TMP/tag"; mkdir -p "$T"; makezip "$T/TagRepo.zip" 'root/x.txt=x'
printf '%s\n' '{"repo":{"name":"TagRepo"},"source":{"zip":"TagRepo.zip"},"git":{}}' > "$T/gitmake.json"
(cd "$T" && "$BIN" >/dev/null)
CL2="$TMP/tagclone"; git clone "$TMP/remotes/testuser/TagRepo.git" "$CL2" >/dev/null 2>&1; git -C "$CL2" tag v1.0.0; git -C "$CL2" push origin v1.0.0 >/dev/null 2>&1
printf '%s\n' '{"repo":{"name":"TagRepo"},"source":{"zip":"TagRepo.zip"},"git":{},"release":{"enabled":true,"tag":"v1.0.0","notes":"x"}}' > "$T/gitmake.json"
if (cd "$T" && "$BIN" --json >out.json); then exit 24; fi
grep -q 'TAG_CONFLICT' "$T/out.json"

# 6) Multi-ZIP refuses a lone obvious binary asset as project source.
D="$TMP/discover"; mkdir -p "$D"; makezip "$D/App_Windows_x64.zip" 'app.exe=MZbinary'
(cd "$D" && "$BIN" discover --json >discover.json)
grep -q 'single_archive_looks_like_release_asset' "$D/discover.json"; grep -q '"needs_input": true' "$D/discover.json"

# 7) Generic MCP descriptor works without Claude-specific registration.
(cd "$D" && "$BIN" ai setup --client generic --json >generic.json)
grep -q 'gitmake.mcp-registration/v1' "$D/generic.json"; grep -q '"transport": "stdio"' "$D/generic.json"

# 8) Linux/macOS install path is supported in this Linux E2E.
HOME2="$TMP/home"; mkdir -p "$HOME2"
(HOME="$HOME2" SHELL=/bin/bash PATH="$TMP/bin:/usr/bin:/bin" "$BIN" install >/dev/null)
test -x "$HOME2/.local/bin/gitmake"; grep -q 'GitMake PATH' "$HOME2/.profile"

# 9) MCP GitHub apply requires and consumes a user-created one-shot token.
M="$TMP/mcpapply"; mkdir -p "$M"; makezip "$M/MCPDemo.zip" 'root/a.txt=a'
printf '%s\n' '{"repo":{"name":"MCPDemo"},"source":{"zip":"MCPDemo.zip"},"git":{}}' > "$M/gitmake.json"
python - "$BIN" "$M" <<'PYMCP'
import json, os, subprocess, sys
bin, cwd = sys.argv[1], sys.argv[2]
env=os.environ.copy()
def run(*args):
    p=subprocess.run([bin,*args],cwd=cwd,env=env,text=True,capture_output=True)
    if p.returncode: raise SystemExit(f"command failed {args}: {p.stdout} {p.stderr}")
    return json.loads(p.stdout)
plan=run('plan','--json'); pid=plan['plan_id']
# Approval deliberately requires a real TTY so an agent shell cannot mint its own token.
import pty, re, select, time
master, slave = pty.openpty()
p = subprocess.Popen([bin,'approve',pid], cwd=cwd, env=env, stdin=slave, stdout=slave, stderr=slave, text=False, close_fds=True)
os.close(slave)
os.write(master, (pid[-6:]+'\n').encode())
chunks=[]
while p.poll() is None:
    r,_,_=select.select([master],[],[],0.1)
    if r:
        try: chunks.append(os.read(master,4096))
        except OSError: break
p.wait()
try:
    while True: chunks.append(os.read(master,4096))
except OSError: pass
os.close(master)
out=b''.join(chunks).decode(errors='replace')
if p.returncode: raise SystemExit(out)
m=re.search(r'gma_[0-9a-f]+', out)
if not m: raise SystemExit('approval token missing: '+out)
token=m.group(0)
def mcp_call(tok):
    req={"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"gitmake_apply","arguments":{"project_dir":cwd,"plan_id":pid,"approval_token":tok}}}
    p=subprocess.run([bin,'mcp','--allow-write'],cwd=cwd,env=env,input=json.dumps(req)+'\n',text=True,capture_output=True)
    if p.returncode: raise SystemExit(p.stderr)
    return json.loads(p.stdout)
r=mcp_call(token)
assert r['result']['isError'] is False, r
r2=mcp_call(token)
assert r2['result']['isError'] is True, r2
assert 'already used' in r2['result']['content'][0]['text'], r2
PYMCP
test -d "$TMP/remotes/testuser/MCPDemo.git"

echo V07_E2E_PASS
