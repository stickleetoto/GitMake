# Changelog

## v0.6.1 — One-click AI Setup

- Added `gitmake ai setup` to detect Claude Code and register GitMake MCP automatically at user scope.
- Read-only MCP access remains the default.
- Added `gitmake ai setup --write` for explicitly enabling reviewed config/apply tools, with confirmation unless `--yes` is supplied.
- Added `gitmake ai status [--json]` for Claude detection, MCP registration, scope, access level, command path, and connection health.
- Added idempotent `gitmake ai remove [--json]` for the GitMake-managed user-scope Claude MCP registration.
- `gitmake ai setup` repairs/installs the stable per-user GitMake binary on Windows before registering MCP, avoiding temporary/portable executable paths.
- `GitMake-Setup.exe` now automatically connects Claude Code in read-only mode when Claude is detected; setup remains successful when Claude is absent.
- Existing matching MCP registration is preserved instead of rewritten; permission changes safely replace only the user-scoped GitMake registration.
- GitMake refuses to overwrite/remove a same-named MCP server in project/local scope.
- Added dedicated AI setup unit tests and end-to-end coverage for setup, idempotency, read→write transition, status, and removal.

## v0.6.0

- Added local MCP stdio server via `gitmake mcp`.
- Default MCP toolset is read-only; mutating tools require explicit `--allow-write`.
- Added MCP tools for capability discovery, doctor, multi-ZIP discovery, config schema/validation, read-only preview, reviewed plan creation, and history.
- Added guarded MCP config write/patch and plan apply tools when write mode is explicitly enabled.
- MCP config authoring routes through GitMake's own schema validation and atomic writer rather than direct file editing.
- No direct MCP force-push, repository deletion, history rewrite, or unreviewed publish tool.
- Added `GITMAKE_PROJECT_DIR` and Claude Code `CLAUDE_PROJECT_DIR` project-root fallback.
- Added compatibility for modern direct `tools/list` clients and legacy MCP `initialize` clients.
- Strengthened AI guidance to prefer `gitmake config write --stdin --json` over direct `gitmake.json` editing.
- Added MCP unit/E2E coverage and kept all previous regression suites.

## v0.5.2 — Agent Hardening + LLM Config Authoring

### Added

- `gitmake config schema --json` authoritative local JSON Schema for `gitmake.json`.
- `gitmake config validate --json` strict config validation and normalized interpretation.
- `gitmake config write --stdin` for validated full-config writes from an LLM or automation.
- `gitmake config patch --stdin` with recursive JSON object merge and validation before replacement.
- `--dry-run` support for config writes/patches; `--read-only` blocks both mutation commands.
- `gitmake plan --json` reviewed plans containing source/config/release hashes and remote baseline.
- `gitmake apply <plan_id>` stale-plan revalidation before any mutation.
- `gitmake history [--json]` operation audit records.
- stable high-level machine error codes and suggested recovery actions.
- source-selection confidence score + evidence in multi-ZIP discovery.
- `release.on_existing="resume"` to upload only missing assets to an existing release.
- SHA-256 verification of downloaded self-upgrade packages using the release checksum asset.
- flags can now appear after positional arguments, e.g. `gitmake apply <id> --json`.

### Safety

- LLMs can discover the config schema locally instead of guessing fields.
- unknown config fields remain rejected.
- config write/patch validates before guarded file replacement.
- plan/apply binds approval to the reviewed ZIP/config/remote/release state.
- tampered or changed inputs fail with `PLAN_STALE`.
- self-upgrade refuses a package whose SHA-256 does not match the published checksum.

### Compatibility

- existing `gitmake.ai/v1`, `gitmake.result/v1`, and `gitmake.discovery/v1` consumers remain compatible.
- existing v0.5.1 configless planning and multi-ZIP behavior is retained.
- human `gitmake` one-command workflow is unchanged.
