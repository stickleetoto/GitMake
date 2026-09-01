# Shared safety gate for E2E suites that stub the GitHub CLI.
#
# Those suites put an extensionless `gh` shell script on PATH and assume
# GitMake will call it. Go's exec.LookPath resolves commands through PATHEXT on
# Windows, so it never finds an extensionless file: GitMake falls through to the
# REAL, authenticated `gh` and publishes to a real GitHub account. That is
# exactly what happened during a Windows run of this suite, which created live
# repositories before the script's first assertion failed.
#
# Fail closed rather than guess. Source this file and call require_fake_gh with
# the built gitmake binary AFTER the stub is on PATH.
require_fake_gh() {
  local bin="$1"
  case "$(uname -s 2>/dev/null || echo unknown)" in
    MINGW* | MSYS* | CYGWIN* | Windows_NT)
      echo "REFUSING TO RUN: this E2E stubs the GitHub CLI with an extensionless shell script." >&2
      echo "Go cannot resolve that on Windows, so GitMake would call the real, authenticated gh" >&2
      echo "and publish to a real GitHub account. Run this suite under Linux, macOS, or WSL." >&2
      return 70
      ;;
  esac
  # Positive proof: ask GitMake itself which gh it resolves.
  if ! "$bin" doctor 2>/dev/null | grep -q 'gh version 2.fake'; then
    echo "REFUSING TO RUN: GitMake does not resolve the fake gh stub." >&2
    echo "Continuing would run this suite against a real GitHub account." >&2
    return 71
  fi
}
