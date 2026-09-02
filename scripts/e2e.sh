#!/usr/bin/env bash
set -euo pipefail
ROOT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
BIN="$ROOT_DIR/dist/gitmake-e2e"
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT
mkdir -p "$ROOT_DIR/dist" "$TMP/bin" "$TMP/remotes"

go build -o "$BIN" "$ROOT_DIR/cmd/gitmake"

source "$(dirname "${BASH_SOURCE[0]}")/fakegh.sh"
install_fake_gh "$TMP/bin" "$TMP/remotes"
require_fake_gh "$BIN"
export FAKE_GH_ROOT="$TMP/remotes"
export GIT_CONFIG_GLOBAL="$TMP/gitconfig"
git config --global user.name "GitMake E2E"
git config --global user.email "gitmake@example.test"

makezip() {
  local zip="$1"; shift
  python - "$zip" "$@" <<'PY'
import sys, zipfile
with zipfile.ZipFile(sys.argv[1], 'w', zipfile.ZIP_DEFLATED) as z:
    for item in sys.argv[2:]:
        name, body = item.split('=', 1)
        z.writestr(name, body)
PY
}

# A. First run, no config, one ZIP, Unicode path: create in one invocation.
A="$TMP/한글 경로/첫 생성"; mkdir -p "$A"
makezip "$A/Demo_v1.0.0.zip" 'Demo/README.md=hello' 'Demo/old.txt=old'
(cd "$A" && "$BIN" >create.log 2>&1)
test ! -e "$A/gitmake.json"
test "$(git --git-dir="$TMP/remotes/testuser/Demo.git" rev-list --count HEAD)" = 1

# B. Update mirror: modify/add/delete and preserve history.
makezip "$A/Demo_v1.0.0.zip" 'Demo/README.md=hello-v2' 'Demo/new.txt=new'
(cd "$A" && "$BIN" >update.log 2>&1)
test "$(git --git-dir="$TMP/remotes/testuser/Demo.git" rev-list --count HEAD)" = 2
! git --git-dir="$TMP/remotes/testuser/Demo.git" ls-tree -r --name-only HEAD | grep -qx old.txt
git --git-dir="$TMP/remotes/testuser/Demo.git" ls-tree -r --name-only HEAD | grep -qx new.txt

# C. No-change update: no empty commit.
(cd "$A" && "$BIN" >nochange.log 2>&1)
test "$(git --git-dir="$TMP/remotes/testuser/Demo.git" rev-list --count HEAD)" = 2
grep -q 'already up to date' "$A/nochange.log"

# D. No ZIP is onboarding, not a crash/error exit.
D="$TMP/nozip"; mkdir -p "$D"
(cd "$D" && "$BIN" >nozip.log 2>&1)
grep -q 'No project source could be selected in this folder' "$D/nozip.log"

# E. Placeholder starter repairs itself when one ZIP is added later.
makezip "$D/Recover_v2.0.zip" 'Recover/file.txt=ok'
(cd "$D" && "$BIN" >recover.log 2>&1)
test ! -e "$D/gitmake.json"
test -d "$TMP/remotes/testuser/Recover.git"

# F. Multiple ZIP ambiguity lists candidates and does not mutate GitHub.
F="$TMP/multi"; mkdir -p "$F"
printf '%s\n' '{"repo":{"name":"Multi"},"source":{"zip":"missing.zip"},"git":{}}' > "$F/gitmake.json"
makezip "$F/a.zip" 'a.txt=a'; makezip "$F/b.zip" 'b.txt=b'
if (cd "$F" && "$BIN" >multi.log 2>&1); then exit 10; fi
grep -q a.zip "$F/multi.log"; grep -q b.zip "$F/multi.log"
test ! -d "$TMP/remotes/testuser/Multi.git"

# G. Existing empty remote repository is populated.
G="$TMP/empty"; mkdir -p "$G" "$TMP/remotes/testuser"
git init --bare "$TMP/remotes/testuser/EmptyRepo.git" >/dev/null 2>&1
printf '%s\n' '{"repo":{"name":"EmptyRepo"},"source":{"zip":"payload.zip"},"git":{}}' > "$G/gitmake.json"
makezip "$G/payload.zip" 'root/hello.txt=populated'
(cd "$G" && "$BIN" >empty.log 2>&1)
test "$(git --git-dir="$TMP/remotes/testuser/EmptyRepo.git" rev-list --count refs/heads/main)" = 1

# H. Legacy master default branch falls back from generated main.
H="$TMP/legacy"; S="$TMP/seed"; mkdir -p "$H" "$S"
git -C "$S" init >/dev/null 2>&1; git -C "$S" branch -M master
echo old > "$S/legacy.txt"; git -C "$S" add -A; git -C "$S" commit -m seed >/dev/null
git init --bare "$TMP/remotes/testuser/Legacy.git" >/dev/null 2>&1
git -C "$S" remote add origin "$TMP/remotes/testuser/Legacy.git"; git -C "$S" push -u origin master >/dev/null 2>&1
git --git-dir="$TMP/remotes/testuser/Legacy.git" symbolic-ref HEAD refs/heads/master
printf '%s\n' '{"repo":{"name":"Legacy"},"source":{"zip":"payload.zip"},"git":{"branch":"main"}}' > "$H/gitmake.json"
makezip "$H/payload.zip" 'root/legacy.txt=new'
(cd "$H" && "$BIN" >legacy.log 2>&1)
grep -q 'Branch fallback' "$H/legacy.log"
test "$(git --git-dir="$TMP/remotes/testuser/Legacy.git" rev-list --count refs/heads/master)" = 2

# I. Dry-run create does not create remote.
I="$TMP/drycreate"; mkdir -p "$I"
printf '%s\n' '{"repo":{"name":"DryCreate"},"source":{"zip":"payload.zip"},"git":{}}' > "$I/gitmake.json"
makezip "$I/payload.zip" 'root/x.txt=x'
(cd "$I" && "$BIN" --dry-run >dry.log 2>&1)
test ! -d "$TMP/remotes/testuser/DryCreate.git"

# J. Dry-run update does not add commit.
count_before="$(git --git-dir="$TMP/remotes/testuser/Demo.git" rev-list --count HEAD)"
makezip "$A/Demo_v1.0.0.zip" 'Demo/README.md=dry-change'
(cd "$A" && "$BIN" --dry-run >dryupdate.log 2>&1)
test "$(git --git-dir="$TMP/remotes/testuser/Demo.git" rev-list --count HEAD)" = "$count_before"

# K. create-only/update-only guards.
if (cd "$A" && "$BIN" --create-only >guard1.log 2>&1); then exit 11; fi
grep -q -- '--create-only' "$A/guard1.log"
K="$TMP/updatemissing"; mkdir -p "$K"
printf '%s\n' '{"repo":{"name":"Missing"},"source":{"zip":"payload.zip"},"git":{}}' > "$K/gitmake.json"
makezip "$K/payload.zip" 'x.txt=x'
if (cd "$K" && "$BIN" --update-only >guard2.log 2>&1); then exit 12; fi
grep -q -- '--update-only' "$K/guard2.log"

# L. Authentication failure is clear and does not create a remote.
L="$TMP/authfail"; mkdir -p "$L"
printf '%s\n' '{"repo":{"name":"AuthFail"},"source":{"zip":"payload.zip"},"git":{}}' > "$L/gitmake.json"
makezip "$L/payload.zip" 'x.txt=x'
if (cd "$L" && FAKE_GH_AUTH_FAIL=1 "$BIN" >auth.log 2>&1); then exit 13; fi
grep -q 'gh auth login' "$L/auth.log"
test ! -d "$TMP/remotes/testuser/AuthFail.git"

# M. ZIP invalid on Windows is rejected before GitHub mutation.
M="$TMP/badzip"; mkdir -p "$M"
printf '%s\n' '{"repo":{"name":"BadZip"},"source":{"zip":"bad.zip"},"git":{}}' > "$M/gitmake.json"
makezip "$M/bad.zip" 'root/CON.txt=x'
if (cd "$M" && "$BIN" >bad.log 2>&1); then exit 14; fi
grep -q 'reserved Windows device name' "$M/bad.log"
test ! -d "$TMP/remotes/testuser/BadZip.git"

# N. Case-colliding ZIP paths are rejected.
N="$TMP/collision"; mkdir -p "$N"
printf '%s\n' '{"repo":{"name":"Collision"},"source":{"zip":"bad.zip"},"git":{}}' > "$N/gitmake.json"
makezip "$N/bad.zip" 'root/A.txt=A' 'root/a.txt=a'
if (cd "$N" && "$BIN" >collision.log 2>&1); then exit 15; fi
grep -q 'case-colliding' "$N/collision.log"

# O. CREATE can publish a GitHub release and upload assets.
O="$TMP/release"; mkdir -p "$O"
makezip "$O/payload.zip" 'root/app.txt=v1'
echo binary > "$O/app-win.zip"
printf '%s\n' '{"repo":{"name":"ReleaseRepo"},"source":{"zip":"payload.zip"},"git":{},"release":{"enabled":true,"tag":"v1.0.0","title":"ReleaseRepo v1.0.0","notes":"first release","assets":["app-win.zip"]}}' > "$O/gitmake.json"
(cd "$O" && "$BIN" >release-create.log 2>&1)
test -d "$TMP/releases/testuser/ReleaseRepo/v1.0.0"
test -f "$TMP/releases/testuser/ReleaseRepo/v1.0.0/assets/app-win.zip"
grep -q 'Released.*v1.0.0' "$O/release-create.log"

# P. A new release can be created even when the repository snapshot has no changes.
python - "$O/gitmake.json" <<'PY2'
import json, sys
p=sys.argv[1]; d=json.load(open(p)); d['release']['tag']='v1.0.1'; d['release']['title']='ReleaseRepo v1.0.1'; json.dump(d, open(p,'w'), indent=2)
PY2
count_before="$(git --git-dir="$TMP/remotes/testuser/ReleaseRepo.git" rev-list --count HEAD)"
(cd "$O" && "$BIN" >release-nochange.log 2>&1)
test "$(git --git-dir="$TMP/remotes/testuser/ReleaseRepo.git" rev-list --count HEAD)" = "$count_before"
test -d "$TMP/releases/testuser/ReleaseRepo/v1.0.1"
grep -q 'already up to date' "$O/release-nochange.log"

# Q. Duplicate release defaults to an early error and does not push source changes.
makezip "$O/payload.zip" 'root/app.txt=should-not-push'
if (cd "$O" && "$BIN" >release-duplicate.log 2>&1); then exit 16; fi
grep -q 'already exists' "$O/release-duplicate.log"
test "$(git --git-dir="$TMP/remotes/testuser/ReleaseRepo.git" rev-list --count HEAD)" = "$count_before"

# R. on_existing=skip permits a source update while leaving the existing release alone.
python - "$O/gitmake.json" <<'PY2'
import json, sys
p=sys.argv[1]; d=json.load(open(p)); d['release']['on_existing']='skip'; json.dump(d, open(p,'w'), indent=2)
PY2
(cd "$O" && "$BIN" >release-skip.log 2>&1)
test "$(git --git-dir="$TMP/remotes/testuser/ReleaseRepo.git" rev-list --count HEAD)" = $((count_before+1))
grep -q 'already exists' "$O/release-skip.log"

# S. Missing release asset fails before a new source change is committed.
python - "$O/gitmake.json" <<'PY2'
import json, sys
p=sys.argv[1]; d=json.load(open(p)); d['release']['tag']='v1.0.2'; d['release']['on_existing']='error'; d['release']['assets']=['missing.zip']; json.dump(d, open(p,'w'), indent=2)
PY2
makezip "$O/payload.zip" 'root/app.txt=missing-asset-change'
count_before_missing="$(git --git-dir="$TMP/remotes/testuser/ReleaseRepo.git" rev-list --count HEAD)"
if (cd "$O" && "$BIN" >release-missing.log 2>&1); then exit 17; fi
grep -q 'release asset not found' "$O/release-missing.log"
test "$(git --git-dir="$TMP/remotes/testuser/ReleaseRepo.git" rev-list --count HEAD)" = "$count_before_missing"

# T. Dry-run previews a release but creates neither commit nor release.
python - "$O/gitmake.json" <<'PY2'
import json, sys
p=sys.argv[1]; d=json.load(open(p)); d['release']['assets']=['app-win.zip']; json.dump(d, open(p,'w'), indent=2)
PY2
(cd "$O" && "$BIN" --dry-run >release-dry.log 2>&1)
test "$(git --git-dir="$TMP/remotes/testuser/ReleaseRepo.git" rev-list --count HEAD)" = "$count_before_missing"
test ! -d "$TMP/releases/testuser/ReleaseRepo/v1.0.2"
grep -q 'Release plan.*v1.0.2' "$O/release-dry.log"

# U. --no-release updates GitHub but suppresses the configured release.
(cd "$O" && "$BIN" --no-release >release-norelease.log 2>&1)
test "$(git --git-dir="$TMP/remotes/testuser/ReleaseRepo.git" rev-list --count HEAD)" = $((count_before_missing+1))
test ! -d "$TMP/releases/testuser/ReleaseRepo/v1.0.2"
grep -q 'Release.*skipped' "$O/release-norelease.log"

# V. Invalid Git tag is rejected before repository mutation.
python - "$O/gitmake.json" <<'PY2'
import json, sys
p=sys.argv[1]; d=json.load(open(p)); d['release']['tag']='bad tag'; json.dump(d, open(p,'w'), indent=2)
PY2
makezip "$O/payload.zip" 'root/app.txt=invalid-tag-change'
count_before_badtag="$(git --git-dir="$TMP/remotes/testuser/ReleaseRepo.git" rev-list --count HEAD)"
if (cd "$O" && "$BIN" >release-badtag.log 2>&1); then exit 18; fi
grep -q 'not a valid Git tag name' "$O/release-badtag.log"
test "$(git --git-dir="$TMP/remotes/testuser/ReleaseRepo.git" rev-list --count HEAD)" = "$count_before_badtag"

echo "ALL_E2E_PASS"
