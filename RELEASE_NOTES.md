# GitMake v0.3.1

This is a focused Windows installation/doctor reliability patch.

## Fixes

- Fixes the false `PATH not installed` result that could appear even when `gitmake --version` already worked.
- `gitmake doctor` now verifies four independent signals: installed binary, resolved command path, current-process PATH, and persisted user PATH.
- Shows the actual resolved `gitmake.exe` path in diagnostics.
- Correctly treats a freshly registered user PATH as healthy even if the current shell has not refreshed yet.
- Adds a fallback manual PATH scan for Windows `PATHEXT` / `exec.LookPath` edge cases.
- Running `gitmake install` from the already-installed copy no longer attempts to overwrite the executable currently in use.

## Validation

- Unit tests
- Go vet
- Race detector
- Existing CREATE/UPDATE/Release E2E suite
- Windows amd64 cross-build
