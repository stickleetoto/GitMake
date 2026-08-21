## v0.7.2 — Project Identity + Destructive Change Gate

- Added protected repository identity metadata in `.gitmake/project.json` and validation before updates.
- Added `PROJECT_IDENTITY_MISMATCH` hard-stop behavior for conflicting repository bindings.
- Stopped automatic retargeting of a real repository config when its configured ZIP is missing and an unrelated lone ZIP is present (`PROJECT_SOURCE_MISMATCH`). Starter placeholder configs still self-heal.
- Plans now surface working directory, config path, source path, target repository, remote visibility, project identity, change counts, deletion ratio, and risk classification.
- Added destructive-change classification when at least 10 managed files and at least 30% of the prior managed baseline would be deleted.
- Direct publish and ordinary apply are blocked for destructive plans. Local apply requires `--destructive`.
- MCP destructive apply requires a separately human-minted `gitmake approve <plan_id> --destructive` one-shot token; normal approval tokens are rejected.
- Added explicit config-vs-remote visibility mismatch reporting without changing existing repository visibility.
- Added ZIP-only MCP E2E coverage: inspect → config suggest → config write → plan with no hand-authored config.
- Added v0.7.2 safety E2E coverage for stale-source retarget refusal, destructive plan gating, destructive token replay rejection, identity tamper detection, and plan provenance.

## v0.7.1

- Fix self-publish false positives caused by secret-scanner unit-test fixtures containing literal private-key/token signatures.
- Add a regression test so scanner test fixtures cannot silently reintroduce signatures that block GitMake source publication.

# Changelog

## v0.7.1 — Safety Gate + Cross-Agent Hardening

- Added **managed sync** with `.gitmake/managed.json`: GitMake preserves remote-only files and deletes only files it previously managed; `.github/**` and `.gitmake/**` are protected by default.
- Added security preflight for likely secret files/content, large direct-Git files, and Git LFS requirements.
- Added `sync` and `security` configuration sections while keeping config schema version 1 compatible.
- Added `gitmake approve <plan_id>` and one-shot expiring approval tokens. MCP `gitmake_apply` now requires a human-generated approval token in addition to the reviewed plan ID.
- Added `gitmake inspect` / MCP project inspection and in-memory config suggestion so agents can remain inside the GitMake tool surface.
- Hardened multi-ZIP selection: content evidence is weighted over names, obvious binary archives are not auto-selected as source, and close candidates require input.
- Added GitHub preflight for required-PR branch protection, pre-existing bare release tags, stale remote state, and large-file/LFS constraints.
- Added generic MCP registration output with `gitmake ai setup --client generic --json`.
- Added Linux/macOS per-user installation to `~/.local/bin` and PATH management.
- Preserved all existing safety invariants: no force push, history rewrite, or repository deletion.

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
