#!/usr/bin/env bash
set -euo pipefail
ROOT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
BIN="$ROOT_DIR/dist/gitmake-e2e-v090"
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
git config --global user.name "GitMake V090"
git config --global user.email "v090@example.test"

# 1) Zero-config folder publish: no gitmake.json is created, project memory is.
F="$TMP/ZeroConfigProject"; mkdir -p "$F/src"
printf '# Zero Config\n' > "$F/README.md"
printf 'print("hello")\n' > "$F/src/main.py"
(cd "$F" && "$BIN" . >/dev/null)
test ! -f "$F/gitmake.json"
test -f "$F/.gitmake/project.json"
grep -q 'testuser/ZeroConfigProject' "$F/.gitmake/project.json"
R="$TMP/remotes/testuser/ZeroConfigProject.git"
test -d "$R"
git --git-dir="$R" cat-file -e HEAD:README.md
! git --git-dir="$R" cat-file -e HEAD:gitmake.json 2>/dev/null

# 2) Project memory survives a local folder rename and keeps the original remote target.
REN="$TMP/RenamedLocalFolder"
mv "$F" "$REN"
printf 'v2\n' > "$REN/src/v2.txt"
(cd "$REN" && "$BIN" plan --json > "$TMP/memory_plan.json")
python - "$TMP/memory_plan.json" <<'PY'
import json,sys
p=json.load(open(sys.argv[1],encoding='utf-8'))
assert p['repository']=='testuser/ZeroConfigProject', p
assert p['mode']=='UPDATE', p
assert p['source_mode']=='folder', p
assert p['changes']['added']>=1, p
PY

# 3) Folder + plausible source ZIP is ambiguous for machine callers; GitMake must not guess.
A="$TMP/Ambiguous"; mkdir -p "$A/src"
printf '# Ambiguous\n' > "$A/README.md"
printf 'module example.test/ambiguous\n\ngo 1.23\n' > "$A/go.mod"
printf 'package main\nfunc main(){}\n' > "$A/src/main.go"
python - "$A/OtherProject_Source.zip" <<'PY'
import sys,zipfile
with zipfile.ZipFile(sys.argv[1],'w',zipfile.ZIP_DEFLATED) as z:
    z.writestr('README.md','# Other\n')
    z.writestr('pyproject.toml','[project]\nname="other-project"\n')
    z.writestr('src/main.py','print("other")\n')
PY
if (cd "$A" && "$BIN" plan --json > "$TMP/ambiguous.json"); then exit 51; fi
grep -q 'SOURCE_AMBIGUOUS' "$TMP/ambiguous.json"

# 4) Simple interactive mode makes a reviewed plan and asks once before publishing.
S="$TMP/SimpleProject"; mkdir -p "$S/src"
printf '# Simple\n' > "$S/README.md"
printf 'console.log("simple")\n' > "$S/src/app.js"
python - "$BIN" "$S" <<'PY'
import os,pty,select,subprocess,sys
bin,cwd=sys.argv[1:]
master,slave=pty.openpty()
p=subprocess.Popen([bin],cwd=cwd,stdin=slave,stdout=slave,stderr=slave,env=os.environ.copy(),close_fds=True)
os.close(slave)
out=[]; sent=False
while p.poll() is None:
    r,_,_=select.select([master],[],[],0.1)
    if r:
        try: data=os.read(master,4096)
        except OSError: break
        out.append(data)
        text=b''.join(out).decode(errors='replace')
        if not sent and 'Publish?' in text:
            os.write(master,b'y\n'); sent=True
p.wait()
try:
    while True: out.append(os.read(master,4096))
except OSError: pass
os.close(master)
text=b''.join(out).decode(errors='replace')
if p.returncode: raise SystemExit(text)
assert 'Changes' in text, text
assert 'Risk' in text, text
assert 'Publish?' in text, text
assert ('Repository created' in text) or ('✓ Published SimpleProject' in text), text
PY
test -d "$TMP/remotes/testuser/SimpleProject.git"
test ! -f "$S/gitmake.json"
test -f "$S/.gitmake/project.json"

# 5) Help is intentionally split into simple and expert surfaces.
"$BIN" help > "$TMP/help.txt"
grep -q 'Everyday use' "$TMP/help.txt"
! grep -q 'config schema' "$TMP/help.txt"
"$BIN" help --expert > "$TMP/expert.txt"
grep -q 'Expert help' "$TMP/expert.txt"
grep -q 'config schema' "$TMP/expert.txt"

# 6) Existing advanced ZIP config + Release path remains compatible.
C="$TMP/ConfiguredRelease"; mkdir -p "$C"
python - "$C/Source.zip" <<'PYCFGZIP'
import sys,zipfile
with zipfile.ZipFile(sys.argv[1],'w',zipfile.ZIP_DEFLATED) as z:
    z.writestr('root/README.md','# Configured Release\n')
    z.writestr('root/src/app.txt','configured\n')
PYCFGZIP
printf 'asset\n' > "$C/App_Windows.zip"
cat > "$C/gitmake.json" <<'JSONCFG'
{"repo":{"name":"ConfiguredRelease","visibility":"private"},"source":{"zip":"Source.zip","strip_root":true},"git":{"branch":"main"},"release":{"enabled":true,"tag":"v1.0.0","notes":"compat","assets":["App_Windows.zip"]}}
JSONCFG
(cd "$C" && "$BIN" > "$TMP/configured.out")
grep -Eq 'Repository created|✓ Published ConfiguredRelease' "$TMP/configured.out"
grep -q 'Released.*v1.0.0' "$TMP/configured.out"
test -f "$C/gitmake.json"

# 7) MCP prepare remains zero-config by default even with write access; persistence is explicit.
M="$TMP/MCPZero"; mkdir -p "$M/src"
printf '# MCP Zero\n' > "$M/README.md"
printf 'print("mcp")\n' > "$M/src/main.py"
python - "$BIN" "$M" <<'PY'
import json,os,subprocess,sys
bin,cwd=sys.argv[1:]
def call(args):
    req={"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"gitmake_prepare","arguments":args}}
    p=subprocess.run([bin,'mcp','--allow-write'],cwd=cwd,env=os.environ.copy(),input=json.dumps(req)+'\n',text=True,capture_output=True)
    if p.returncode: raise SystemExit(p.stderr)
    r=json.loads(p.stdout); assert r['result']['isError'] is False,r
    return r['result']['structuredContent']
a=call({'project_dir':cwd})
assert a['config']['mode']=='in_memory',a
assert a['config']['persisted'] is False,a
assert not os.path.exists(os.path.join(cwd,'gitmake.json'))
b=call({'project_dir':cwd,'persist_config':True})
assert b['config']['persisted'] is True,b
assert os.path.isfile(os.path.join(cwd,'gitmake.json'))
PY

echo V090_SIMPLE_ZERO_CONFIG_E2E_PASS
