# GitMake v0.6.1

GitMake v0.6.1 turns Claude Code MCP setup into a one-command flow.

## Highlights

- `gitmake ai setup` detects Claude Code and registers GitMake MCP automatically.
- Read-only access is the default.
- `gitmake ai setup --write` explicitly enables only validated config write/patch and reviewed plan apply tools.
- `gitmake ai status` shows registration, access mode, command path, scope, and connection health.
- `gitmake ai remove` cleanly removes the GitMake-managed user-scope registration.
- Windows AI setup uses the stable installed GitMake executable instead of a temporary portable path.
- `GitMake-Setup.exe` automatically connects Claude Code read-only when available.
- Existing correct registrations are left untouched, and GitMake refuses to remove/replace same-named project/local MCP servers.

Manual `gitmake mcp` remains available for other MCP clients and advanced setups.
