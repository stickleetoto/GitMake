# GitMake v1.2.3 Test Report

**PASS** for the Locked-Executable Recovery patch.

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

## v1.2.3 replacement coverage

- replacement script waits for the creating parent PID before touching the installed executable;
- process termination is scoped to exact executable-path equality;
- broad image-name termination (`taskkill /IM gitmake.exe`) is explicitly absent;
- PowerShell single-quote escaping is unit-tested;
- installer, upgrader, and replacement-helper Windows amd64 test binaries cross-compile successfully.

A live Windows file-lock execution test cannot run in the Linux packaging environment; the Windows-specific path is validated by cross-compilation plus script-generation unit tests and should be smoke-tested on the target Windows machine before publishing.
