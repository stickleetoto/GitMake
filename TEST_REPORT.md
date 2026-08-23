# GitMake v1.0.0 Test Report

Date: 2026-08-23

## Result

**PASS** for the v1.0.0 stable release gate covering tokenless approval, concurrency/recovery hardening, the guided v0.10 UX surface, and multi-platform release builds.

## Static / unit / race

- `gofmt` check — PASS
- `go test ./...` — PASS
- `go vet ./...` — PASS
- `go test -race ./...` — PASS
- repository self secret-scan regression — PASS through the security scanner test suite

## v1.0.0 focused E2E

`scripts/e2e_v100.sh` — **V100_TOKENLESS_STABILITY_E2E_PASS**

Covered:

1. A reviewed plan can be approved locally with `gitmake approve` without exposing a `gma_...` token.
2. `gitmake approve` with no plan ID selects the latest reviewed plan for the current project directory.
3. `gitmake_apply` accepts the reviewed `plan_id` after local approval and performs the publish.
4. The local approval grant is single-use; replay is rejected.
5. The existing v0.10 guided confirmation/recovery path remains intact.

## Guided UX regression

`scripts/e2e_v0100.sh` — **V0100_GUIDED_UX_E2E_PASS**

Covered low/medium/destructive confirmation, compact success output, decision explanations, secret-block recovery guidance, and Windows Setup cross-build.

Additional current-version regression checks completed successfully:

- `scripts/e2e_v090.sh` — **V090_SIMPLE_ZERO_CONFIG_E2E_PASS**
- `scripts/e2e_v04.sh` — **V04_E2E_PASS**
- `scripts/e2e_v03.sh` — **V03_E2E_PASS**

The broad base `scripts/e2e.sh` assertion sequence reached **ALL_E2E_PASS** in this container. The surrounding execution harness did not return cleanly after completion during teardown, so that runner result is recorded as an environment/harness cleanup anomaly rather than represented as a clean process-exit PASS.

Historical pre-v1 E2E scripts that explicitly assert the old token-copy approval UI are retained as historical fixtures; their approval assertions are intentionally superseded by the v1 tokenless approval contract and `e2e_v100.sh`.

## Concurrency / recovery hardening

Unit coverage passes for:

- plan/repository operation locking
- lock replay/reacquisition behavior
- apply operation journal creation/finish/recovery records
- approval expiry, binding, use-once behavior, and cleanup
- expired plan/approval/lock/journal housekeeping

Apply retries revalidate the reviewed plan before mutation instead of blindly resuming stale state.

## Performance regression guard

`BenchmarkHashThousandSmallFiles` (`go test ./internal/foldersource -run '^$' -bench '^BenchmarkHashThousandSmallFiles$' -benchtime=3x -count=1`):

- Linux amd64 test environment
- 1,000 small files
- approximately **45.1 ms/op** in this run

This is a regression guard, not a product performance guarantee. v1.0.0 intentionally avoids speculative optimization.

## Release builds

Cross-builds completed successfully for:

- Windows amd64 — `gitmake.exe`
- Windows amd64 — `GitMake-Setup.exe`
- Linux amd64
- Linux arm64 (Raspberry Pi 4/5 and other 64-bit ARM Linux)
- macOS amd64
- macOS arm64

The Linux amd64 release binary reports `gitmake 1.0.0` and the stable `gitmake.version/v1` JSON schema.

## Packaging verification

- Source ZIP integrity test — PASS
- Source ZIP re-extract followed by `go test ./...` — PASS
- Source ZIP re-extract followed by `go vet ./...` — PASS
- Re-extracted source self secret-scan regression — PASS
- Windows/Linux/macOS package ZIP integrity — PASS
- Self-publish ZIP integrity — PASS
- Self-publish `gitmake.json` validation with the v1.0.0 binary — PASS
- Self-publish release assets include Windows x64, Linux x64, Linux arm64, macOS x64, macOS arm64, source, and SHA-256 manifest

## Stable safety invariants

- no force push
- no Git history rewrite
- no repository deletion
- managed sync / protected paths
- secret scan + large-file/LFS preflight
- project identity binding
- branch protection / tag conflict checks
- immutable plan stale revalidation
- tokenless, local, single-use human MCP approval
- destructive mass-deletion gate
- folder snapshot symlink / unsafe-path / case-collision defenses
- same-plan / same-repository concurrent mutation locks
