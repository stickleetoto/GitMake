#!/usr/bin/env bash
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
BIN="$ROOT/dist/gitmake-e2e-v03"
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT
mkdir -p "$TMP/bin" "$TMP/remotes" "$TMP/releases"
go build -o "$BIN" "$ROOT/cmd/gitmake"
cat > "$TMP/bin/gh" <<'GH'
#!/usr/bin/env bash
set -euo pipefail
r="${FAKE_GH_ROOT:?}"
if [[ "${1:-}" == "--version" ]]; then echo "gh version 2.fake"; exit 0; fi
if [[ "${1:-}" == "auth" && "${2:-}" == "status" ]]; then echo ok; exit 0; fi
if [[ "${1:-}" == "api" && "${2:-}" == "user" ]]; then echo testuser; exit 0; fi
if [[ "${1:-}" == "repo" && "${2:-}" == "view" ]]; then
  t="$3"; o="${t%%/*}"; n="${t#*/}"; d="$r/$o/$n.git"
  [[ -d "$d" ]] || { echo "HTTP 404 not found" >&2; exit 1; }
  h="$(git --git-dir="$d" symbolic-ref --short HEAD 2>/dev/null || true)"
  if [[ " $* " == *" --jq .url "* ]]; then echo "https://example/$t"; exit 0; fi
  if [[ -n "$h" ]]; then printf '{"nameWithOwner":"%s","url":"https://example/%s","defaultBranchRef":{"name":"%s"}}\n' "$t" "$t" "$h"; else printf '{"nameWithOwner":"%s","url":"https://example/%s","defaultBranchRef":null}\n' "$t" "$t"; fi
  exit 0
fi
if [[ "${1:-}" == "repo" && "${2:-}" == "clone" ]]; then
  t="$3"; d="$4"; o="${t%%/*}"; n="${t#*/}"; git clone "$r/$o/$n.git" "$d" >/dev/null 2>&1; exit 0
fi
if [[ "${1:-}" == "repo" && "${2:-}" == "create" ]]; then
  t="$3"; shift 3; src=""
  while (($#)); do case "$1" in --source) src="$2"; shift 2;; --remote|--description) shift 2;; --push|--private|--public|--internal) shift;; *) shift;; esac; done
  o="${t%%/*}"; n="${t#*/}"; d="$r/$o/$n.git"; mkdir -p "$(dirname "$d")"; git init --bare "$d" >/dev/null 2>&1
  b="$(git -C "$src" branch --show-current)"; git -C "$src" remote add origin "$d"; git -C "$src" push -u origin "$b" >/dev/null 2>&1; git --git-dir="$d" symbolic-ref HEAD "refs/heads/$b"; echo "https://example/$t"; exit 0
fi
if [[ "${1:-}" == "release" && "${2:-}" == "view" ]]; then
  # Upgrade-style no-tag latest query is not part of this local E2E.
  if [[ "${3:-}" == "--repo" ]]; then echo "v0.3.0"; exit 0; fi
  tag="$3"; shift 3; t=""; while (($#)); do case "$1" in --repo) t="$2"; shift 2;; --json|--jq) shift 2;; *) shift;; esac; done
  d="$(dirname "$r")/releases/$t/$tag"; [[ -d "$d" ]] || { echo "release not found" >&2; exit 1; }; printf '{"url":"https://example/%s/releases/%s","tagName":"%s","isDraft":false,"isPrerelease":false}\n' "$t" "$tag" "$tag"; exit 0
fi
if [[ "${1:-}" == "release" && "${2:-}" == "create" ]]; then
  tag="$3"; shift 3; t=""; assets=(); while (($#)); do case "$1" in --repo) t="$2"; shift 2;; --target|--title|--notes|--notes-file) shift 2;; --generate-notes|--draft|--prerelease|--latest=*) shift;; --*) exit 2;; *) assets+=("$1"); shift;; esac; done
  d="$(dirname "$r")/releases/$t/$tag"; mkdir -p "$d/assets"; for a in "${assets[@]}"; do cp "$a" "$d/assets/$(basename "$a")"; done; echo "https://example/$t/releases/$tag"; exit 0
fi
exit 2
GH
chmod +x "$TMP/bin/gh"
export PATH="$TMP/bin:$PATH" FAKE_GH_ROOT="$TMP/remotes" GIT_CONFIG_GLOBAL="$TMP/gitconfig"
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
P="$TMP/nozip"; mkdir -p "$P"; (cd "$P" && "$BIN" >o 2>&1); grep -q 'No project ZIP found' "$P/o"; test ! -e "$P/gitmake.json"

# one ZIP: config + CREATE in one run
P="$TMP/demo"; mkdir -p "$P"; makezip "$P/Demo_v1.0.0.zip" 'Demo/a.txt=a'; (cd "$P" && "$BIN" >o 2>&1); grep -q 'Repository created' "$P/o"; test -f "$P/gitmake.json"; test "$(git --git-dir="$TMP/remotes/testuser/Demo.git" rev-list --count HEAD)" = 1

# update + concise change counts
makezip "$P/Demo_v1.0.0.zip" 'Demo/a.txt=aa' 'Demo/b.txt=b'; (cd "$P" && "$BIN" >u 2>&1); grep -q 'Repository updated' "$P/u"; grep -q '+1 ~1 -0' "$P/u"; test "$(git --git-dir="$TMP/remotes/testuser/Demo.git" rev-list --count HEAD)" = 2

# no change
(cd "$P" && "$BIN" >n 2>&1); grep -q 'already up to date' "$P/n"; test "$(git --git-dir="$TMP/remotes/testuser/Demo.git" rev-list --count HEAD)" = 2

# multiple ZIPs gives explicit syntax hint
Q="$TMP/multi"; mkdir -p "$Q"; makezip "$Q/A.zip" 'a=x'; makezip "$Q/B.zip" 'b=x'; if (cd "$Q" && "$BIN" >m 2>&1); then exit 10; fi; grep -q 'gitmake YourProject.zip' "$Q/m"

# positional ZIP disambiguates and creates config
R="$TMP/positional"; mkdir -p "$R"; makezip "$R/Chosen_v3.0.zip" 'Chosen/c.txt=c'; makezip "$R/asset.zip" 'asset.txt=x'; (cd "$R" && "$BIN" Chosen_v3.0.zip >p 2>&1); grep -q 'Repository created' "$R/p"; grep -q 'Chosen_v3.0.zip' "$R/gitmake.json"

# init does not publish
I="$TMP/init"; mkdir -p "$I"; makezip "$I/Init_v1.0.zip" 'Init/x=x'; (cd "$I" && "$BIN" init >i 2>&1); grep -q 'Run `gitmake` to publish' "$I/i"; test ! -d "$TMP/remotes/testuser/Init.git"

# release create
S="$TMP/release"; mkdir -p "$S"; makezip "$S/src.zip" 'root/x=x'; echo asset > "$S/win.bin"; cat > "$S/gitmake.json" <<JSON
{"repo":{"name":"Rel"},"source":{"zip":"src.zip"},"git":{},"release":{"enabled":true,"tag":"v1.0.0","notes":"ok","assets":["win.bin"]}}
JSON
(cd "$S" && "$BIN" >r 2>&1); grep -q 'Released.*v1.0.0' "$S/r"; test -f "$TMP/releases/testuser/Rel/v1.0.0/assets/win.bin"

echo V03_E2E_PASS
