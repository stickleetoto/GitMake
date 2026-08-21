#!/usr/bin/env bash
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT
BIN="$ROOT/dist/gitmake-e2e-v061"
mkdir -p "$ROOT/dist" "$TMP/bin" "$TMP/state"
(cd "$ROOT" && go build -o "$BIN" ./cmd/gitmake)

cat > "$TMP/bin/claude" <<'SH'
#!/usr/bin/env bash
set -euo pipefail
STATE="${FAKE_CLAUDE_STATE:?}"
mkdir -p "$STATE"
if [[ "${1:-}" == "--version" ]]; then
  echo "2.1.230 (Claude Code)"
  exit 0
fi
if [[ "${1:-}" != "mcp" ]]; then
  echo "unsupported" >&2; exit 2
fi
shift
case "${1:-}" in
  get)
    if [[ ! -f "$STATE/registered" ]]; then echo "No MCP server found with name: ${2:-}" >&2; exit 1; fi
    echo "gitmake:"
    echo "  Scope: User config"
    echo "  Type: stdio"
    echo "  Command: $(cat "$STATE/command")"
    echo "  Args: $(cat "$STATE/args")"
    echo "  Status: ✔ Connected"
    ;;
  list)
    if [[ ! -f "$STATE/registered" ]]; then echo "No MCP servers configured."; exit 0; fi
    echo "gitmake: $(cat "$STATE/command") $(cat "$STATE/args") (stdio) - ✔ Connected"
    ;;
  remove)
    if [[ ! -f "$STATE/registered" ]]; then echo "No MCP server found" >&2; exit 1; fi
    rm -f "$STATE/registered" "$STATE/command" "$STATE/args"
    echo "Removed MCP server gitmake from user config"
    ;;
  add)
    # Expected: add --transport stdio --scope user gitmake -- /path/gitmake mcp [--allow-write]
    shift
    while [[ $# -gt 0 && "$1" != "--" ]]; do shift; done
    [[ $# -gt 0 ]] || { echo "missing --" >&2; exit 2; }
    shift
    [[ $# -ge 2 ]] || { echo "missing command" >&2; exit 2; }
    printf '%s' "$1" > "$STATE/command"
    shift
    printf '%s' "$*" > "$STATE/args"
    touch "$STATE/registered"
    echo "Added stdio MCP server gitmake to user config"
    ;;
  *) echo "unsupported mcp command ${1:-}" >&2; exit 2;;
esac
SH
chmod +x "$TMP/bin/claude"
export PATH="$TMP/bin:$PATH"
export FAKE_CLAUDE_STATE="$TMP/state"

# Fresh read-only setup.
OUT="$($BIN ai setup --json)"
echo "$OUT" | grep -q '"registered": true'
echo "$OUT" | grep -q '"access": "read-only"'
[[ "$(cat "$TMP/state/args")" == "mcp" ]]

# Idempotent setup stays read-only.
OUT="$($BIN ai setup --json)"
echo "$OUT" | grep -q '"changed": false'

# Status reports the connection.
OUT="$($BIN ai status --json)"
echo "$OUT" | grep -q '"health": "connected"'
echo "$OUT" | grep -q '"access": "read-only"'

# Explicit write setup replaces registration.
OUT="$($BIN ai setup --write --yes --json)"
echo "$OUT" | grep -q '"access": "write"'
[[ "$(cat "$TMP/state/args")" == "mcp --allow-write" ]]

# Remove is idempotent.
OUT="$($BIN ai remove --json)"
echo "$OUT" | grep -q '"removed": true'
OUT="$($BIN ai remove --json)"
echo "$OUT" | grep -q '"removed": false'

# Status remains successful and descriptive when not configured.
OUT="$($BIN ai status --json)"
echo "$OUT" | grep -q '"registered": false'

echo "V061_AI_SETUP_E2E_PASS"
