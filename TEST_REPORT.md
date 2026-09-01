# GitMake v1.2.5 Test Report

## Core verification

- `go test ./...` — PASS
- `go vet ./...` — PASS
- `go test -race ./...` — PASS
- repository source-tree secret self-scan — PASS

## Compatibility / E2E

- v1.0 guided/tokenless stability E2E — PASS
- v1.1 chat approval E2E — PASS
- v1.2 one-shot publish E2E — PASS
- v1.2.1 modern/legacy protocol routing E2E — PASS
- v1.2.3 locked executable recovery E2E — PASS
- v1.2.4 respawn-safe Windows replacement E2E — PASS
- v1.2.5 real-world workflow E2E — PASS

## v1.2.5 regression coverage

1. Root `--stdin` config overrides inference for repo name, visibility, branch, and source without writing `gitmake.json`.
2. Invalid stdin JSON exits non-zero with `CONFIG_INVALID`; no fallback to inferred config occurs.
3. `--stdin` on unrelated commands is rejected; `plan --stdin` is rejected with persistence guidance.
4. Multiple secret kinds in one file and across files are all surfaced; Slack high-confidence tokens are detected.
5. MCP tool failure preserves the CLI machine result, including `SECRET_DETECTED` / `CONFIG_INVALID`, stage, recoverability, suggested action, and pipeline details.
6. `gitmake preview` emits usage guidance instead of a source-path filesystem error.

No publish/apply mutation was required for the v1.2.5 regression suite.
