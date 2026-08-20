# GitMake v0.4.0 Test Report

Date: 2026-08-21

## Result

PASS for the tested development environment and Windows cross-build.

## Validation

- `go test ./...` — PASS
- `go vet ./...` — PASS
- `go test -race ./...` — PASS
- Base create/update/release E2E — `ALL_E2E_PASS`
- v0.3 compatibility E2E — `V03_E2E_PASS`
- v0.4 init UX E2E — `V04_E2E_PASS`
- Windows amd64 `gitmake.exe` cross-build — PASS
- Windows amd64 `GitMake-Setup.exe` cross-build — PASS
- PE32+ x86-64 executable identification — PASS

## v0.4 setup cases covered

- one-ZIP interactive initialization
- repository name suggestion from versioned ZIP filename
- public/private visibility selection
- optional description
- default branch acceptance
- final confirmation before writing config
- multiple-ZIP numbered selection
- `gitmake init --yes` safe defaults
- no-ZIP init leaves no placeholder config
- existing config is not overwritten
- version/help surfaces updated to 0.4.0

## Lightweight optimization audit

Optimization is not currently a release blocker.

Measured on the Linux development container (not a Windows performance guarantee):

- stripped Windows x64 `gitmake.exe`: about 2.9 MiB
- stripped Windows x64 Setup: about 1.8 MiB
- local process startup (`--version`, 100 runs): median ~0.8 ms, p95 ~1.1 ms
- synthetic 5,000-file / 1 KiB-per-file ZIP: safe extraction ~120 ms
- mirror of the same 5,000 files: ~94 ms

These local costs are small compared with the normal external work performed by GitMake (`git`, `gh`, cloning, pushing, and network round trips). The main future optimization candidate is snapshot mirroring: updates currently replace the working-tree snapshot before Git computes the diff, which is simple and reliable but causes avoidable disk I/O for very large repositories.

Recommendation: keep v0.4 focused on UX and correctness. Only introduce differential-copy/hash caching after a benchmark demonstrates that large projects make local snapshot mirroring a meaningful part of total publish time.

## Platform note

Windows binaries were cross-compiled and structurally validated in the development environment. The v0.3.1 install/PATH/upgrade path was previously confirmed on a real Windows machine; v0.4 changes are concentrated in project initialization and do not replace that installer layer.
