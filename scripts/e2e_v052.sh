#!/usr/bin/env bash
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT
BIN="$TMP/gitmake"
(cd "$ROOT" && go build -trimpath -o "$BIN" ./cmd/gitmake)

# Config schema is directly machine-readable.
"$BIN" config schema --json > "$TMP/schema.json"
python3 - "$TMP/schema.json" <<'PY'
import json,sys
x=json.load(open(sys.argv[1],encoding='utf-8'))
assert x['$id']=='gitmake.config/v1', x
assert x['additionalProperties'] is False, x
assert set(x['properties']['repo']['properties']['visibility']['enum'])=={'private','public','internal'}
PY

# Agent-authored full config write and strict validation.
C="$TMP/config"; mkdir -p "$C"
python3 - "$C/Project_Source.zip" <<'PY'
import zipfile,sys
with zipfile.ZipFile(sys.argv[1],'w',zipfile.ZIP_DEFLATED) as z:
    z.writestr('Project/README.md','# Project\n')
PY
(
  cd "$C"
  printf '%s\n' '{"repo":{"name":"AgentProject","visibility":"public"},"source":{"zip":"Project_Source.zip"},"git":{"branch":"main"}}' \
    | "$BIN" config write --stdin --json > write.json
)
python3 - "$C/write.json" "$C/gitmake.json" <<'PY'
import json,sys,os
x=json.load(open(sys.argv[1],encoding='utf-8'))
assert x['schema']=='gitmake.config-write/v1' and x['written'] is True, x
c=json.load(open(sys.argv[2],encoding='utf-8'))
assert c['repo']['visibility']=='public' and c['git']['branch']=='main', c
PY
(cd "$C" && "$BIN" config validate --json > validate.json)
python3 - "$C/validate.json" <<'PY'
import json,sys
x=json.load(open(sys.argv[1],encoding='utf-8'))
assert x['schema']=='gitmake.config-validation/v1' and x['ok'] is True, x
PY

# Patch merges recursively, keeps unrelated settings, and validates before writing.
(
  cd "$C"
  printf '%s\n' '{"repo":{"visibility":"private"},"git":{"commit_message":"Agent update"}}' \
    | "$BIN" config patch --stdin --json > patch.json
)
python3 - "$C/gitmake.json" <<'PY'
import json,sys
c=json.load(open(sys.argv[1],encoding='utf-8'))
assert c['repo']['name']=='AgentProject'
assert c['repo']['visibility']=='private'
assert c['git']['commit_message']=='Agent update'
PY

# Dry-run write validates without touching existing config.
cp "$C/gitmake.json" "$TMP/config-before"
(
  cd "$C"
  printf '%s\n' '{"repo":{"name":"Other"},"source":{"zip":"Project_Source.zip"}}' \
    | "$BIN" config write --stdin --dry-run --json > drywrite.json
)
cmp "$C/gitmake.json" "$TMP/config-before"
python3 - "$C/drywrite.json" <<'PY'
import json,sys
x=json.load(open(sys.argv[1],encoding='utf-8'))
assert x['dry_run'] is True and x['written'] is False, x
PY

# Unknown fields fail with a stable config error code.
if (
  cd "$C"
  printf '%s\n' '{"repo":{"name":"Bad","invented":true},"source":{"zip":"Project_Source.zip"}}' \
    | "$BIN" config write --stdin --json > bad.json
); then exit 51; fi
python3 - "$C/bad.json" <<'PY'
import json,sys
x=json.load(open(sys.argv[1],encoding='utf-8'))
assert x['ok'] is False and x['error']['code']=='CONFIG_INVALID', x
PY

# Read-only blocks config mutation.
if (
  cd "$C"
  printf '%s\n' '{"repo":{"visibility":"public"}}' \
    | "$BIN" config patch --stdin --read-only --json > readonly.json
); then exit 52; fi
python3 - "$C/readonly.json" <<'PY'
import json,sys
x=json.load(open(sys.argv[1],encoding='utf-8'))
assert x['ok'] is False and 'read-only mode blocks' in x['error']['message'], x
PY

# A small fake GitHub backend exercises reviewed plan/apply and stale-input rejection.
mkdir -p "$TMP/fakebin" "$TMP/remotes"
cat > "$TMP/fakebin/gh" <<'GH'
#!/usr/bin/env bash
set -euo pipefail
root="${FAKE_GH_ROOT:?}"
if [[ "${1:-}" == "--version" ]]; then echo "gh version 2.fake"; exit 0; fi
if [[ "${1:-}" == "auth" && "${2:-}" == "status" ]]; then echo "Logged in"; exit 0; fi
if [[ "${1:-}" == "api" && "${2:-}" == "user" ]]; then echo "testuser"; exit 0; fi
if [[ "${1:-}" == "repo" && "${2:-}" == "view" ]]; then
  target="$3"; remote="$root/${target}.git"
  if [[ ! -d "$remote" ]]; then echo "repository not found (HTTP 404)" >&2; exit 1; fi
  head="$(git --git-dir="$remote" symbolic-ref --short HEAD 2>/dev/null || true)"
  printf '{"nameWithOwner":"%s","url":"https://example.test/%s","defaultBranchRef":{"name":"%s"}}\n' "$target" "$target" "$head"
  exit 0
fi
if [[ "${1:-}" == "repo" && "${2:-}" == "clone" ]]; then
  target="$3"; dest="$4"; remote="$root/${target}.git"
  git clone "$remote" "$dest" >/dev/null 2>&1
  exit 0
fi
if [[ "${1:-}" == "release" && "${2:-}" == "view" ]]; then
  tag="$3"; shift 3; target=""
  while (($#)); do
    case "$1" in
      --repo) target="$2"; shift 2;;
      --json) shift 2;;
      --jq) shift 2;;
      *) shift;;
    esac
  done
  rel="$(dirname "$root")/releases/$target/$tag"
  if [[ ! -d "$rel" ]]; then echo "release not found (HTTP 404)" >&2; exit 1; fi
  python3 - "$rel" "$target" "$tag" <<'PYGH'
import json,sys,os
rel,target,tag=sys.argv[1:]
assets=[]
ad=os.path.join(rel,'assets')
if os.path.isdir(ad):
    assets=[{'name':n} for n in sorted(os.listdir(ad))]
print(json.dumps({'url':f'https://example.test/{target}/releases/tag/{tag}','tagName':tag,'isDraft':False,'isPrerelease':False,'assets':assets}))
PYGH
  exit 0
fi
if [[ "${1:-}" == "release" && "${2:-}" == "create" ]]; then
  tag="$3"; shift 3; target=""; assets=()
  while (($#)); do
    case "$1" in
      --repo) target="$2"; shift 2;;
      --target|--title|--notes|--notes-file) shift 2;;
      --generate-notes|--draft|--prerelease|--latest=*) shift;;
      --*) shift;;
      *) assets+=("$1"); shift;;
    esac
  done
  rel="$(dirname "$root")/releases/$target/$tag"
  mkdir -p "$rel/assets"
  for a in "${assets[@]}"; do cp "$a" "$rel/assets/$(basename "$a")"; done
  echo "https://example.test/$target/releases/tag/$tag"
  exit 0
fi
if [[ "${1:-}" == "release" && "${2:-}" == "upload" ]]; then
  tag="$3"; shift 3; target=""; assets=()
  while (($#)); do
    case "$1" in
      --repo) target="$2"; shift 2;;
      *) assets+=("$1"); shift;;
    esac
  done
  rel="$(dirname "$root")/releases/$target/$tag"
  test -d "$rel"
  for a in "${assets[@]}"; do cp "$a" "$rel/assets/$(basename "$a")"; done
  exit 0
fi
if [[ "${1:-}" == "repo" && "${2:-}" == "create" ]]; then
  target="$3"; shift 3; source=""
  while (($#)); do
    case "$1" in
      --source) source="$2"; shift 2;;
      --remote|--description) shift 2;;
      --push|--private|--public|--internal) shift;;
      *) shift;;
    esac
  done
  remote="$root/${target}.git"; mkdir -p "$(dirname "$remote")"
  git init --bare "$remote" >/dev/null 2>&1
  branch="$(git -C "$source" branch --show-current)"
  git -C "$source" remote add origin "$remote"
  git -C "$source" push -u origin "$branch" >/dev/null 2>&1
  git --git-dir="$remote" symbolic-ref HEAD "refs/heads/$branch"
  echo "https://example.test/$target"
  exit 0
fi
echo "unexpected gh args: $*" >&2
exit 2
GH
chmod +x "$TMP/fakebin/gh"
export PATH="$TMP/fakebin:$PATH"
export FAKE_GH_ROOT="$TMP/remotes"
export XDG_CACHE_HOME="$TMP/cache"
export GIT_CONFIG_GLOBAL="$TMP/gitconfig"
git config --global user.name "GitMake E2E"
git config --global user.email "gitmake@example.test"

P="$TMP/plan"; mkdir -p "$P"
python3 - "$P/PlanProject_Source.zip" <<'PY'
import zipfile,sys
with zipfile.ZipFile(sys.argv[1],'w',zipfile.ZIP_DEFLATED) as z:
    z.writestr('PlanProject/README.md','v1\n')
PY
cat > "$P/gitmake.json" <<'JSON'
{"repo":{"name":"PlanProject","visibility":"private"},"source":{"zip":"PlanProject_Source.zip"},"git":{"branch":"main"}}
JSON
(cd "$P" && "$BIN" plan --json > plan.json)
PLAN_ID="$(python3 - "$P/plan.json" <<'PY'
import json,sys
x=json.load(open(sys.argv[1],encoding='utf-8')); print(x['plan_id'])
PY
)"
# Flags after a positional argument are intentionally supported.
(cd "$P" && "$BIN" apply "$PLAN_ID" --json > applied.json)
python3 - "$P/applied.json" <<'PY'
import json,sys
x=json.load(open(sys.argv[1],encoding='utf-8'))
assert x['ok'] is True and x['pipeline']['plan_id'].startswith('gm_'), x
assert x['pipeline']['mode']=='CREATE', x
PY
test -d "$TMP/remotes/testuser/PlanProject.git"

# New plan becomes stale if source changes before approval/apply.
python3 - "$P/PlanProject_Source.zip" <<'PY'
import zipfile,sys
with zipfile.ZipFile(sys.argv[1],'w',zipfile.ZIP_DEFLATED) as z: z.writestr('PlanProject/README.md','v2\n')
PY
(cd "$P" && "$BIN" plan --json > plan2.json)
PLAN2="$(python3 - "$P/plan2.json" <<'PY'
import json,sys; print(json.load(open(sys.argv[1]))['plan_id'])
PY
)"
python3 - "$P/PlanProject_Source.zip" <<'PY'
import zipfile,sys
with zipfile.ZipFile(sys.argv[1],'w',zipfile.ZIP_DEFLATED) as z: z.writestr('PlanProject/README.md','tampered\n')
PY
if (cd "$P" && "$BIN" apply "$PLAN2" --json > stale.json); then exit 53; fi
python3 - "$P/stale.json" <<'PY'
import json,sys
x=json.load(open(sys.argv[1],encoding='utf-8'))
assert x['error']['code']=='PLAN_STALE', x
PY

# Release resume uploads only assets missing from an already-created release.
RR="$TMP/resume"; mkdir -p "$RR"
python3 - "$RR/ResumeProject_Source.zip" <<'PY'
import zipfile,sys
with zipfile.ZipFile(sys.argv[1],'w',zipfile.ZIP_DEFLATED) as z: z.writestr('ResumeProject/README.md','same\n')
PY
echo one > "$RR/a.zip"; echo two > "$RR/b.zip"
cat > "$RR/gitmake.json" <<'JSON'
{"repo":{"name":"ResumeProject","visibility":"private"},"source":{"zip":"ResumeProject_Source.zip"},"git":{"branch":"main"},"release":{"enabled":true,"tag":"v1.0.0","notes":"resume test","assets":["a.zip","b.zip"],"on_existing":"resume"}}
JSON
(cd "$RR" && "$BIN" --json > first-release.json)
REL="$TMP/releases/testuser/ResumeProject/v1.0.0/assets"
test -f "$REL/a.zip" && test -f "$REL/b.zip"
rm "$REL/b.zip"
(cd "$RR" && "$BIN" --json > resumed-release.json)
test -f "$REL/b.zip"
python3 - "$RR/resumed-release.json" <<'PY'
import json,sys
x=json.load(open(sys.argv[1],encoding='utf-8'))
assert x['ok'] is True, x
r=x['pipeline']['release']
assert r['resumed'] is True and r['assets']==1, r
PY

(cd "$P" && "$BIN" history --json > history.json)
python3 - "$P/history.json" <<'PY'
import json,sys
x=json.load(open(sys.argv[1],encoding='utf-8'))
assert x['schema']=='gitmake.history/v1' and len(x['entries'])>=2, x
PY

echo V052_E2E_PASS
