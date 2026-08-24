# GitMake v1.1.0 Test Report

Date: 2026-08-25

## Result

**PASS** for the v1.1.0 release gate covering MCP Chat Approval, the frozen v1 tokenless approval fallback, replay resistance, current v1/v0.10/v0.9/v0.8 behavior, and multi-platform builds.

## Static / unit / race

- `gofmt -l .` — PASS (no unformatted files)
- `go test ./...` — PASS
- `go vet ./...` — PASS
- `go test -race ./...` — PASS
- repository self secret-scan regression — PASS through the security scanner test suite

## v1.1.0 MCP Chat Approval E2E

`scripts/e2e_v110.sh` — **V110_CHAT_APPROVAL_E2E_PASS**

Covered:

1. Modern MCP 2026-07-28 Multi Round-Trip flow returns `input_required` when `gitmake_apply` needs human approval.
2. The approval request uses client-controlled `elicitation/create` input and a signed, expiring `requestState` bound to the reviewed plan.
3. An accepted response creates the same short-lived local one-shot approval grant used by the v1 terminal workflow, then applies the exact reviewed plan.
4. Replaying the same accepted approval after the grant has been consumed is rejected.
5. Destructive/high-risk approval requires the plan-specific `DELETE-XXXXXX` phrase; medium-risk approval requires `PUBLISH`.
6. Legacy MCP 2025-11-25 stdio clients that advertise elicitation receive an `elicitation/create` request while the tool call is pending.
7. Legacy clients without elicitation support fall back to the stable terminal `gitmake approve` workflow.
8. `server/discover` advertises the modern protocol/tool capability surface.

## Current compatibility regressions

- `scripts/e2e_v100.sh` — **V100_TOKENLESS_STABILITY_E2E_PASS**
- `scripts/e2e_v0100.sh` — **V0100_GUIDED_UX_E2E_PASS**
- `scripts/e2e_v090.sh` — **V090_SIMPLE_ZERO_CONFIG_E2E_PASS**
- `scripts/e2e_v080.sh` — **V080_FOLDER_E2E_PASS** and **V073_SAFETY_E2E_PASS**

Historical pre-v1 fixtures that explicitly assert superseded token-copy approval wording are not used as current compatibility gates. The frozen v1 contract is covered by the v1.0+ regression suites above.

## Approval / trust model

v1.1 adds a new **approval transport**, not a weaker approval class:

- Preferred: approval inside an elicitation-capable MCP client.
- Fallback: terminal `gitmake approve`.
- Both paths create a short-lived, local, plan-bound, single-use grant.
- The model cannot mint a GitMake grant through a normal GitMake tool.
- A consumed grant cannot be re-minted for the same reviewed plan; another mutation requires a fresh plan.
- Destructive operations retain stronger typed confirmation.

Client-side automation is part of the trust boundary: if the MCP client is configured with an `Elicitation` hook that auto-accepts requests, that client configuration can intentionally skip the visible human dialog. GitMake does not treat the model itself as approval, but it must trust the MCP client's elicitation result.

## Concurrency / recovery hardening retained from v1

Unit coverage passes for:

- plan/repository operation locking
- lock replay/reacquisition behavior
- apply operation journal creation/finish/recovery records
- approval expiry, binding, one-shot behavior, consumed-grant replay protection, and cleanup
- expired plan/approval/lock/journal housekeeping

Apply retries revalidate the reviewed plan before mutation instead of blindly resuming stale state.

## Performance regression guard

`BenchmarkHashThousandSmallFiles` (`go test ./internal/foldersource -run '^$' -bench '^BenchmarkHashThousandSmallFiles$' -benchtime=3x -count=1`):

- Linux amd64 test environment
- 1,000 small files
- approximately **21.0 ms/op** in this run

This is a regression guard, not a product performance guarantee. Results can vary with filesystem cache and host load; v1.1 does not introduce speculative performance changes.

## Release build target matrix

Release packaging targets:

- Windows amd64 — `gitmake.exe` + `GitMake-Setup.exe`
- Linux amd64
- Linux arm64 (Raspberry Pi 4/5 and other 64-bit ARM Linux)
- macOS amd64
- macOS arm64

## Stable safety invariants

v1.1 preserves the frozen v1 safety contract:

- no force push
- no Git history rewrite
- no repository deletion
- managed sync / protected paths
- secret scan + large-file/LFS preflight
- project identity binding
- branch protection / tag conflict checks
- immutable plan stale revalidation
- local, single-use human approval grants
- destructive mass-deletion gate
- folder snapshot symlink / unsafe-path / case-collision defenses
- same-plan / same-repository concurrent mutation locks

## Build verification

Cross-builds completed successfully for all release targets. The native Linux amd64 binary reports `gitmake 1.1.0`; cross-built binaries were verified by executable format inspection.
