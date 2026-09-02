# Shared GitHub CLI stub for the E2E suites.
#
# Every suite used to embed its own `gh` shell script. Two problems followed
# from that. The stubs were extensionless files, which Go's exec.LookPath
# cannot resolve on Windows, so GitMake silently fell through to the real,
# authenticated gh and published live repositories during a test run. And
# sixteen near-copies drifted apart, so a suite could pass because its own stub
# happened to be lenient.
#
# install_fake_gh compiles internal/testsupport/fakegh instead: a real
# executable, found the same way on every platform, with one implementation.
#
# Usage, after $TMP exists and $BIN has been built:
#
#   source "$(dirname "${BASH_SOURCE[0]}")/fakegh.sh"
#   install_fake_gh "$TMP/bin" "$TMP/remotes"
#   require_fake_gh "$BIN"

# install_fake_gh <bin dir> <state dir>
#
# Builds the stub into <bin dir>, prepends that directory to PATH, and exports
# FAKE_GH_ROOT=<state dir>. Bare repositories live under <state dir>; releases
# live in a sibling `releases` directory, so a suite that sets
# FAKE_GH_ROOT="$TMP/remotes" finds releases under "$TMP/releases".
install_fake_gh() {
  local bindir="$1"
  local state="$2"
  local repo_root
  repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

  if [ -z "$bindir" ] || [ -z "$state" ]; then
    echo "install_fake_gh requires a bin directory and a state directory" >&2
    return 64
  fi

  local name="gh"
  case "$(uname -s 2>/dev/null || echo unknown)" in
    MINGW* | MSYS* | CYGWIN* | Windows_NT) name="gh.exe" ;;
  esac

  mkdir -p "$bindir" "$state"
  if ! (cd "$repo_root" && go build -o "$bindir/$name" ./internal/testsupport/fakegh); then
    echo "could not build the fake GitHub CLI" >&2
    return 65
  fi

  export PATH="$bindir:$PATH"
  export FAKE_GH_ROOT="$state"
}

# require_fake_gh <path to built gitmake>
#
# Refuses to continue unless GitMake itself resolves the stub. Without this a
# misconfigured suite runs against a real GitHub account, which has already
# happened once.
require_fake_gh() {
  local bin="$1"
  if [ -z "${FAKE_GH_ROOT:-}" ]; then
    echo "REFUSING TO RUN: FAKE_GH_ROOT is not set; call install_fake_gh first." >&2
    return 71
  fi

  # Capture the report rather than piping it. `gitmake doctor` exits non-zero
  # whenever it finds any issue -- a git identity that is configured later in
  # the suite, for instance -- and under `set -o pipefail` that would fail the
  # check even though the stub was resolved correctly.
  local report
  report="$("$bin" doctor 2>/dev/null || true)"
  case "$report" in
    *"gh version 2.fake"*) return 0 ;;
  esac

  echo "REFUSING TO RUN: GitMake does not resolve the fake GitHub CLI." >&2
  echo "Continuing would run this suite against a real GitHub account." >&2
  echo "gitmake doctor reported:" >&2
  echo "$report" >&2
  return 71
}
