# Changelog

## v0.5.0 — Agent Interface

### Added

- `gitmake ai describe` for built-in human-readable AI/tool discovery.
- `gitmake ai describe --json` with stable `gitmake.ai/v1` capability manifest.
- `gitmake ai install` to install a managed GitMake section into `AGENTS.md` and write `.gitmake/ai.json`.
- Idempotent AGENTS.md updates that preserve existing user-authored instructions.
- Global `--json` machine-readable output.
- Structured publish pipeline data including mode, repository, branch, file count, diff counts, release metadata, dry-run/read-only state, and pipeline stage.
- Global `--read-only` safety mode for agent inspection.
- Stable documented exit codes: 0 success, 1 runtime/workflow error, 2 usage error.
- High-level pipeline stage model: DISCOVER → PLAN → PREPARE → VALIDATE → GIT → PUSH → RELEASE → REPORT.

### Safety

- `--read-only` blocks `init`, `install`, `upgrade`, and `ai install`.
- Read-only publish requires `--dry-run`.
- Read-only publish refuses to auto-create `gitmake.json`.
- AI capability manifest explicitly advertises that force-push, history rewriting, and repository deletion are unavailable.

### Compatibility

- Existing v0.4 publish, Release, install, upgrade, doctor, and init workflows remain supported.
- Human-readable output remains the default; JSON is opt-in.
