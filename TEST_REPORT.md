# GitMake v0.3.1 Test Report

Date: 2026-08-21

## Result

**PASS** for all test/build gates available in this environment.

## Regression target

v0.3.1 specifically fixes the Windows doctor/install inconsistency where `gitmake --version` could resolve successfully while `gitmake doctor` still reported that GitMake was not installed on PATH.

The new diagnostic model independently records:

- installed binary at `%LOCALAPPDATA%\\Programs\\GitMake\\gitmake.exe`
- actual `gitmake` command resolution
- current-process PATH registration
- persisted per-user PATH registration
- whether the running executable is the standard installed copy

Doctor treats a persisted user PATH registration as healthy even when the current shell has not refreshed yet, and manually scans PATH as a fallback when `exec.LookPath`/`PATHEXT` resolution disagrees.

## Automated gates

- `go test ./...` — PASS
- `go vet ./...` — PASS
- `go test -race ./...` — PASS
- `scripts/e2e.sh` — `ALL_E2E_PASS`
- `scripts/e2e_v03.sh` — `V03_E2E_PASS`
- Windows amd64 cross-build — PASS

## New regression tests

`internal/installer/status_test.go` covers:

- case-insensitive Windows PATH matching
- trailing-slash normalization
- rejecting prefix-only PATH matches
- healthy state when the installed binary is present and the persisted user PATH is registered even if live command resolution is temporarily unavailable
- healthy state for resolved/process-PATH/current-installed-copy signals
- unhealthy state for a binary that exists but has no command/PATH signal

## Existing E2E coverage retained

- first-run onboarding
- CREATE from one ZIP
- UPDATE with history preservation
- no-change update
- Unicode paths
- ambiguous ZIP handling
- positional ZIP selection
- empty GitHub repository population
- legacy `master` branch fallback
- dry-run create/update
- create-only/update-only guards
- authentication failure
- Windows-invalid ZIP rejection
- case-colliding ZIP rejection
- GitHub Release creation and asset upload
- no-change Release creation
- duplicate Release protection
- `on_existing=skip`

## Windows limitation of this build environment

The binaries are cross-compiled from Linux. Windows-specific registry/PATH mutation and PowerShell command resolution cannot be executed natively here. Those code paths are compile-validated, the decision logic is isolated into cross-platform unit tests, and the Windows executable format was verified after build.

Recommended real-machine acceptance check after installing v0.3.1:

```powershell
gitmake --version
gitmake doctor
Get-Command gitmake | Format-List Source,Path,CommandType
where.exe gitmake
```

Expected result: doctor reports both `GitMake install` and `CLI command` as healthy and exits with code 0.
