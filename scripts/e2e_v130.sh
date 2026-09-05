#!/usr/bin/env bash
# v1.3.0 end-to-end: `gitmake undo` and post-publish verification, driven
# through the real binary rather than through the package API.
#
# The Go tests call runUndo directly, so nothing there exercises argument
# parsing, the read-only gate, or the exit codes. This does.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

bin="$tmp/bin"
mkdir -p "$bin"
gh_name="gh"
gm_name="gitmake"
case "$(uname -s 2>/dev/null || echo unknown)" in
  MINGW* | MSYS* | CYGWIN*) gh_name="gh.exe"; gm_name="gitmake.exe" ;;
esac
go build -o "$bin/$gh_name" ./internal/testsupport/fakegh
go build -o "$tmp/$gm_name" ./cmd/gitmake
gm="$tmp/$gm_name"

export PATH="$bin:$PATH"
export FAKE_GH_ROOT="$tmp/remotes"
export XDG_CACHE_HOME="$tmp/cache"
export LocalAppData="$tmp/cache"
export HOME="$tmp/home"
export GIT_CONFIG_GLOBAL="$tmp/gitconfig"
mkdir -p "$HOME"
git config --global user.name "GitMake E2E" >/dev/null
git config --global user.email "e2e@example.test" >/dev/null
git config --global init.defaultBranch main >/dev/null

project="$tmp/project"
mkdir -p "$project/src"
cd "$project"
printf '# Demo\n' > README.md
printf 'package main\n' > src/main.go
cat > gitmake.json <<'JSON'
{
  "schema_version": 1,
  "repo": { "name": "UndoDemo", "visibility": "public" },
  "source": { "folder": "." },
  "git": { "branch": "main" }
}
JSON

remote="$FAKE_GH_ROOT/testuser/UndoDemo.git"

# --- create, then one ordinary update -------------------------------------
"$gm" --yes >/dev/null
printf '# Demo\n\nsecond revision\n' > README.md
printf 'added\n' > extra.txt
publish_out="$("$gm" --yes)"

# The publish must confirm itself against the remote rather than assume.
echo "$publish_out" | grep -q 'Verified' || {
  echo "publish did not verify the remote:" >&2; echo "$publish_out" >&2; exit 1; }

count_before="$(git --git-dir="$remote" rev-list --count HEAD)"
[ "$count_before" = "2" ] || { echo "expected 2 commits, got $count_before" >&2; exit 1; }

# --- read-only must block a mutating command ------------------------------
if "$gm" undo --read-only >/dev/null 2>&1; then
  echo "read-only mode did not block undo" >&2
  exit 1
fi

# --- positional arguments are rejected ------------------------------------
if "$gm" undo something >/dev/null 2>&1; then
  echo "undo accepted a positional argument" >&2
  exit 1
fi

# --- dry run changes nothing ----------------------------------------------
dry_out="$("$gm" undo --dry-run)"
echo "$dry_out" | grep -q 'Dry run' || { echo "dry run did not say so" >&2; exit 1; }
count_after_dry="$(git --git-dir="$remote" rev-list --count HEAD)"
[ "$count_after_dry" = "$count_before" ] || {
  echo "dry run changed the remote: $count_before -> $count_after_dry" >&2; exit 1; }

# --- the undo itself -------------------------------------------------------
undo_out="$("$gm" undo --yes)"
echo "$undo_out" | grep -q 'Reverted' || { echo "undo did not report a revert" >&2; exit 1; }

# It must never let the user believe the previous contents are gone.
echo "$undo_out" | grep -qi 'stays published' || {
  echo "undo did not warn that published content remains:" >&2; echo "$undo_out" >&2; exit 1; }
echo "$undo_out" | grep -qi 'rotate it' || {
  echo "undo did not tell the user to rotate a published credential" >&2; exit 1; }

# A revert adds a commit; it never removes one.
count_after="$(git --git-dir="$remote" rev-list --count HEAD)"
[ "$count_after" = "3" ] || { echo "expected 3 commits after undo, got $count_after" >&2; exit 1; }

# And the reverted publish is still reachable: undo is not deletion.
git --git-dir="$remote" cat-file -e "$(git --git-dir="$remote" rev-parse HEAD~1)^{commit}"

# Content is back where it was.
if git --git-dir="$remote" show HEAD:README.md | grep -q 'second revision'; then
  echo "README was not reverted" >&2
  exit 1
fi
if git --git-dir="$remote" show HEAD:extra.txt >/dev/null 2>&1; then
  echo "a file added by the undone publish is still on the remote" >&2
  exit 1
fi

# --- a second undo refuses, with the documented machine code ---------------
if "$gm" undo --yes >/dev/null 2>&1; then
  echo "a second undo of the same publish must fail" >&2
  exit 1
fi
# The command is expected to fail here, and `set -o pipefail` would turn that
# into a script failure if its output were read straight out of a pipeline.
json_out="$("$gm" undo --yes --json 2>/dev/null || true)"
code="$(printf '%s' "$json_out" | tr -d ' \n' | sed -n 's/.*"code":"\([A-Z_]*\)".*/\1/p')"
[ "$code" = "NOTHING_TO_UNDO" ] || { echo "expected NOTHING_TO_UNDO, got '$code'" >&2; exit 1; }

count_final="$(git --git-dir="$remote" rev-list --count HEAD)"
[ "$count_final" = "3" ] || {
  echo "a refused undo changed the remote: 3 -> $count_final" >&2; exit 1; }

echo V130_UNDO_AND_VERIFY_E2E_PASS
