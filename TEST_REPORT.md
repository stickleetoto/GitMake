# GitMake v0.7.2 Test Report

Status: **PASS** for the implemented v0.7.2 scope.

## Static / unit verification

- `go test ./...` — PASS
- `go vet ./...` — PASS
- `go test -race ./...` — PASS
- project identity unit tests — PASS
- destructive risk classification unit tests — PASS
- destructive approval record unit test — PASS
- stale real-config source retarget refusal test — PASS

## Regression suites

- base E2E — PASS
- v0.3 E2E — PASS
- v0.4 E2E — PASS
- v0.5 E2E — PASS
- v0.5.1 E2E — PASS
- v0.5.2 E2E — PASS
- v0.6 MCP E2E — PASS
- v0.6.1 AI setup E2E — PASS
- v0.7 safety E2E — PASS

## v0.7.2 safety E2E

`scripts/e2e_v072.sh` — PASS

Verified:

1. A non-placeholder repository config is not auto-retargeted from a missing configured ZIP to an unrelated lone ZIP.
2. New repositories receive `.gitmake/project.json`.
3. Plan provenance reports exact working directory, config path, source path, target repository, remote visibility, project identity, and risk.
4. Config-private / remote-public mismatch is reported while remote visibility remains unchanged.
5. Deleting more than 30% and at least 10 files from the managed baseline is classified destructive.
6. Direct publish is blocked for destructive changes.
7. Ordinary `gitmake apply` is blocked for a destructive plan.
8. Ordinary approval cannot mint a token for a destructive plan.
9. Interactive `gitmake approve <id> --destructive` creates a destructive-class one-shot token.
10. MCP apply accepts that token once and rejects replay.
11. Tampering project identity to another valid repository binding produces `PROJECT_IDENTITY_MISMATCH`.
12. ZIP-only MCP authoring works end-to-end through inspect → config suggest → config write → plan without a hand-authored config.

## Safety invariants

- no force push
- no history rewrite
- no repository deletion
- MCP cannot mint approval tokens
- destructive MCP apply cannot use a normal approval token
- source/config/remote changes invalidate reviewed plans
