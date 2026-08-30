# GitMake v1.2.1 Test Report

## Release gate

**PASS** for the v1.2.1 Protocol Routing Hardening patch.

The patch changes only MCP protocol routing, approval-state purpose validation, documentation, and stale historical test assertions. The v1.2 one-shot publish workflow and frozen v1 safety model remain intact.

## Core Go validation

- `go test ./...` — PASS
- `go vet ./...` — PASS
- `go test -race ./...` — PASS

New unit coverage verifies:

- modern `2026-07-28` initialization does not activate the legacy held-open elicitation path
- approval request state without an exact purpose binding is rejected
- publish approval state cannot be reused as apply approval state

## Current E2E release gates

- `scripts/e2e.sh` — PASS (`ALL_E2E_PASS`)
- `scripts/e2e_v0100.sh` — PASS (`V0100_GUIDED_UX_E2E_PASS`)
- `scripts/e2e_v100.sh` — PASS (`V100_TOKENLESS_STABILITY_E2E_PASS`)
- `scripts/e2e_v110.sh` — PASS (`V110_CHAT_APPROVAL_E2E_PASS`)
- `scripts/e2e_v120.sh` — PASS (`V120_ONE_SHOT_PUBLISH_E2E_PASS`)
- `scripts/e2e_v121.sh` — PASS (`V121_PROTOCOL_ROUTING_E2E_PASS`)

The v1.2.1 E2E specifically initializes a client as MCP `2026-07-28` with elicitation capability and then calls `gitmake_publish`. The response must be the modern `input_required` result tied to the original tool call; a legacy `elicitation/create` server request fails the test.

## Historical harness cleanup

`e2e_v04.sh` and `e2e_v05.sh` contained obsolete assertions that required the executable version to be exactly `1.0.0`, which made those historical behavior suites fail on every later valid v1 release. They now test the stable v1 version/schema shape instead. Both suites pass after the cleanup.

## Safety invariants retained

- reviewed immutable plan before mutation
- client-controlled human approval
- medium risk requires `PUBLISH`
- destructive/high risk requires plan-specific `DELETE-XXXXXX`
- exact source/config/remote revalidation
- short-lived single-use approval grants
- project identity and managed-sync protections
- plan/repository mutation locks
- no force push
- no history rewrite
- no repository deletion

## Cross-platform build targets

Release packaging targets:

- Windows amd64 (`gitmake.exe` + `GitMake-Setup.exe`)
- Linux amd64
- Linux arm64
- macOS amd64
- macOS arm64

All are built with `CGO_ENABLED=0` for the release bundle.
