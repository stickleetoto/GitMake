# GitMake v0.6.1 Test Report

## Result

PASS for the implemented v0.6.1 scope.

## v0.6.1 focus

- `gitmake ai setup` auto-detects Claude Code and registers user-scope GitMake MCP.
- Default Claude MCP access is read-only.
- Re-running setup is idempotent when the registration already matches.
- `gitmake ai setup --write --yes` safely replaces the managed user-scope registration with the guarded write toolset.
- Same-named non-user MCP registrations are refused rather than overwritten.
- `gitmake ai status` reports Claude version, registration, scope, access, command path, and health.
- `gitmake ai remove` is idempotent.
- Windows AI setup targets the stable per-user GitMake installation path.
- `GitMake-Setup.exe` is wired to auto-connect Claude Code read-only when Claude is detected.

## Automated verification

- `go test ./...` — PASS
- `go vet ./...` — PASS
- `go test -race ./...` — PASS
- base E2E — `ALL_E2E_PASS`
- v0.3 regression — `V03_E2E_PASS`
- v0.4 regression — `V04_E2E_PASS`
- v0.5 Agent Interface — `V05_E2E_PASS`
- v0.5.1 Multi-ZIP/configless planning — `V051_E2E_PASS`
- v0.5.2 Agent Hardening/plan/config authoring — `V052_E2E_PASS`
- v0.6 MCP regression — `V06_MCP_E2E_PASS`
- v0.6.1 AI Setup E2E — `V061_AI_SETUP_E2E_PASS`

## v0.6.1 AI setup E2E cases

1. fresh read-only Claude registration
2. repeated/idempotent read-only setup
3. status reports connected + read-only
4. explicit read-only → write transition
5. write registration uses `mcp --allow-write`
6. removal succeeds
7. second removal is a safe no-op
8. status remains machine-readable when no registration exists

Unit tests additionally cover refusal to replace a same-named project-scoped server.

## Windows build

Cross-compiled with:

```text
GOOS=windows
GOARCH=amd64
CGO_ENABLED=0
```

Artifacts identified as:

```text
gitmake.exe       PE32+ x86-64 Windows console executable
GitMake-Setup.exe PE32+ x86-64 Windows console executable
```

The current build environment is Linux, so native Windows execution of the installer UI/PATH/Claude discovery cannot be performed here. The underlying Claude registration manager is covered by unit + E2E tests, and both Windows executables compile successfully.

## Safety checks retained

- no force push
- no history rewrite
- no repository deletion
- read-only MCP is default
- mutating MCP tools require explicit write setup
- apply still requires a reviewed plan and rejects stale plans
- GitMake only manages its user-scoped Claude MCP registration
- non-user same-name registrations are not silently deleted or replaced
