#!/usr/bin/env bash
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
BIN="$ROOT/dist/gitmake-e2e-v03"
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT
mkdir -p "$TMP/bin" "$TMP/remotes" "$TMP/releases"
go build -o "$BIN" "$ROOT/cmd/gitmake"
source "$(dirname "${BASH_SOURCE[0]}")/fakegh.sh"
install_fake_gh "$TMP/bin" "$TMP/remotes"
export GIT_CONFIG_GLOBAL="$TMP/gitconfig"
require_fake_gh "$BIN"
git config --global user.name "GitMake Test"
git config --global user.email "gitmake@test.invalid"
makezip(){ python - "$@" <<'PY'
import sys,zipfile
p=sys.argv[1]
with zipfile.ZipFile(p,'w',zipfile.ZIP_DEFLATED) as z:
 for x in sys.argv[2:]:
  n,b=x.split('=',1); z.writestr(n,b)
PY
}

# no ZIP: onboarding and no placeholder mutation
# A folder with nothing publishable must be refused, and must not leave a
# config behind. The suite asserted a v0.3 message and forgot that the refusal
# is a non-zero exit, so under `set -e` it could never reach any later check.
P="$TMP/nozip"; mkdir -p "$P"; (cd "$P" && ! "$BIN" >o 2>&1); grep -q 'no project source found' "$P/o"; test ! -e "$P/gitmake.json"

# one ZIP: zero-config CREATE in one run
P="$TMP/demo"; mkdir -p "$P"; makezip "$P/Demo_v1.0.0.zip" 'Demo/a.txt=a'; (cd "$P" && "$BIN" >o 2>&1); grep -q 'Repository created' "$P/o"; test ! -f "$P/gitmake.json"; test "$(git --git-dir="$TMP/remotes/testuser/Demo.git" rev-list --count HEAD)" = 1

# update + concise change counts
makezip "$P/Demo_v1.0.0.zip" 'Demo/a.txt=aa' 'Demo/b.txt=b'; (cd "$P" && "$BIN" >u 2>&1); grep -q 'Repository updated' "$P/u"; grep -q '+1 ~1 -0' "$P/u"; test "$(git --git-dir="$TMP/remotes/testuser/Demo.git" rev-list --count HEAD)" = 2

# no change
(cd "$P" && "$BIN" >n 2>&1); grep -q 'already up to date' "$P/n"; test "$(git --git-dir="$TMP/remotes/testuser/Demo.git" rev-list --count HEAD)" = 2

# multiple ZIPs gives explicit syntax hint
Q="$TMP/multi"; mkdir -p "$Q"; makezip "$Q/A.zip" 'a=x'; makezip "$Q/B.zip" 'b=x'; if (cd "$Q" && "$BIN" >m 2>&1); then exit 10; fi; grep -q 'Project.zip' "$Q/m"

# positional ZIP disambiguates without persisting config
R="$TMP/positional"; mkdir -p "$R"; makezip "$R/Chosen_v3.0.zip" 'Chosen/c.txt=c'; makezip "$R/asset.zip" 'asset.txt=x'; (cd "$R" && "$BIN" Chosen_v3.0.zip >p 2>&1); grep -q 'Repository created' "$R/p"; test ! -f "$R/gitmake.json"

# init does not publish
I="$TMP/init"; mkdir -p "$I"; makezip "$I/Init_v1.0.zip" 'Init/x=x'; (cd "$I" && "$BIN" init --yes >i 2>&1); grep -q 'Run `gitmake` to publish' "$I/i"; test ! -d "$TMP/remotes/testuser/Init.git"

# release create
S="$TMP/release"; mkdir -p "$S"; makezip "$S/src.zip" 'root/x=x'; echo asset > "$S/win.bin"; cat > "$S/gitmake.json" <<JSON
{"repo":{"name":"Rel"},"source":{"zip":"src.zip"},"git":{},"release":{"enabled":true,"tag":"v1.0.0","notes":"ok","assets":["win.bin"]}}
JSON
(cd "$S" && "$BIN" >r 2>&1); grep -q 'Released.*v1.0.0' "$S/r"; test -f "$TMP/releases/testuser/Rel/v1.0.0/assets/win.bin"

echo V03_E2E_PASS
