# GitMake v1.2.4 Test Report

**PASS** for the Respawn-Safe Replacement patch.

## Core regression

- `go test ./...` — PASS
- `go vet ./...` — PASS
- `go test -race ./...` — PASS

## Publishing / MCP regression

- v1.0 guided/stability E2E — PASS
- v1.1 chat approval E2E — PASS
- v1.2 one-shot publish E2E — PASS
- v1.2.1 protocol routing E2E — PASS
- v1.2.3 locked-executable recovery E2E — PASS
- v1.2.4 respawn-safe replacement E2E — PASS

## v1.2.4 replacement coverage

- exact-target process eviction occurs inside the replacement retry loop;
- process termination is still scoped to exact executable-path equality;
- `Wait-Process` synchronization runs before replacement to allow handle release;
- downloaded/manual install uses the synchronous replacement path while installed-copy self-upgrade retains detached replacement;
- broad image-name termination (`taskkill /IM gitmake.exe`) remains explicitly absent;
- installer, upgrader, and replacement-helper Windows amd64 test binaries cross-compile successfully.

A live Windows lock/respawn execution test cannot run in the Linux packaging environment; the target-machine smoke test remains the final platform check.
