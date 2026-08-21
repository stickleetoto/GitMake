# GitMake v0.7.3 Test Report

Status: **PASS** for the implemented v0.7.3 scope.

## Static / unit verification

- `go test ./...` — PASS
- `go vet ./...` — PASS
- `go test -race ./...` — PASS
- MCP default/read-only tool exposure — PASS
- forced `persist_config` rejection in read-only MCP — PASS
- existing v0.7.2 project-identity / destructive-risk / approval tests — PASS

## Regression suites re-run

- base E2E — PASS
- v0.3 E2E — PASS
- v0.4 E2E — PASS
- v0.5 E2E — PASS
- v0.5.1 E2E — PASS
- v0.5.2 E2E — PASS
- v0.6 MCP E2E — PASS
- v0.6.1 AI setup E2E — PASS
- v0.7 safety E2E — PASS
- v0.7.2 safety E2E — PASS
- v0.7.3 high-level prepare E2E — PASS

## v0.7.3 high-level prepare E2E

`scripts/e2e_v073.sh` — PASS

Verified:

1. `gitmake_prepare` is exposed in the default read-only MCP toolset.
2. A folder containing only one source ZIP reaches a reviewed CREATE plan through one MCP call.
3. Read-only prepare does not create `gitmake.json`.
4. Read-only prepare reports config mode `in_memory`, `project_config_mutated=false`, and `github_mutated=false`.
5. The inferred config is strictly validated before planning.
6. The returned reviewed plan includes repository, mode, source, changes, identity/risk data, and a plan ID.
7. With explicit `--allow-write`, the same tool persists the missing config through GitMake's guarded atomic writer.
8. Write-enabled prepare still stops at `ready_for_approval` and does not mutate GitHub.
9. Existing v0.7.2 destructive token, project identity, source mismatch, visibility mismatch, and replay-rejection tests continue to pass.

## Cross-platform builds

- Windows amd64 `gitmake.exe` — PASS
- Windows amd64 `GitMake-Setup.exe` — PASS
- Linux amd64 — PASS
- macOS amd64 — PASS
- macOS arm64 — PASS

## Safety invariants

- no force push
- no history rewrite
- no repository deletion
- read-only prepare performs no project/GitHub mutation
- MCP cannot mint approval tokens
- destructive MCP apply cannot use a normal approval token
- source/config/remote changes invalidate reviewed plans
