# GitMake v0.10.0 Test Report

Date: 2026-08-22

## Result

**PASS** for the v0.10.0 Guided UX + Trust & Recovery release gate.

## Static / unit / race

- `go test ./...` — PASS
- `go vet ./...` — PASS
- `go test -race ./...` — PASS
- repository self secret-scan regression — PASS

## v0.10.0 focused E2E

`scripts/e2e_v0100.sh` — **V0100_GUIDED_UX_E2E_PASS**

Covered:

1. Reviewed plan JSON carries supported decision notes.
2. Low-risk interactive Simple Mode shows `Why`, accepts ordinary confirmation, publishes, and emits the compact success result.
3. `--yes` does not bypass a medium-risk visibility-mismatch plan; exact interactive `PUBLISH` is required.
4. A destructive update requires the dynamic plan-bound `DELETE-XXXXXX` phrase before Simple Mode may continue.
5. A likely secret finding is blocked with `SECRET_DETECTED` and guided exclusion/recovery hints.
6. Windows `GitMake-Setup.exe` cross-compiles with the new first-run readiness flow.

## Regression suites

The following suites were rerun successfully after rebasing historical assertions that intentionally conflicted with the v0.9 zero-config/help behavior:

- `scripts/e2e.sh` — PASS
- `scripts/e2e_v03.sh` — PASS
- `scripts/e2e_v04.sh` — PASS
- `scripts/e2e_v05.sh` — PASS
- `scripts/e2e_v051.sh` — PASS
- `scripts/e2e_v052.sh` — PASS
- `scripts/e2e_v06.sh` — PASS
- `scripts/e2e_v061.sh` — PASS
- `scripts/e2e_v07.sh` — PASS
- `scripts/e2e_v072.sh` — PASS
- `scripts/e2e_v073.sh` — PASS
- `scripts/e2e_v080.sh` — PASS
- `scripts/e2e_v090.sh` — PASS

The rebased assertions preserve the tested functional semantics while accepting two intentional product changes from v0.9: normal publishing no longer writes a missing `gitmake.json`, and default help is deliberately the small Simple Mode surface.

## Safety invariants retained

- `--yes` cannot bypass medium/high-risk review
- no force push
- no Git history rewrite
- no repository deletion
- managed sync / protected paths
- secret scan + large-file/LFS preflight
- project identity binding
- branch protection / tag conflict checks
- immutable plan stale revalidation
- one-shot human MCP approval
- destructive mass-deletion gate
- folder snapshot symlink / unsafe-path / case-collision defenses
