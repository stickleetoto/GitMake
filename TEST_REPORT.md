# GitMake v0.3.0 Test Report

Date: 2026-08-21

## Result

**PASS for the v0.3.0 source and Windows x64 cross-build gates used in this environment.**

## Automated checks

- `go test ./...` — PASS
- `go vet ./...` — PASS
- `go test -race ./...` — PASS
- `scripts/e2e_v03.sh` — `V03_E2E_PASS`
- Windows x64 `gitmake.exe` cross-build — PASS
- Windows x64 `GitMake-Setup.exe` cross-build — PASS
- `file` verification — both binaries are PE32+ x86-64 Windows console executables

## v0.3 focused E2E coverage

The v0.3 harness uses real Git plus a fake `gh` adapter and validates:

1. No-ZIP onboarding without writing a placeholder config.
2. One-ZIP first run creates config and repository in one invocation.
3. Existing-repository snapshot UPDATE preserves history.
4. Change summary counts additions/modifications/deletions.
5. No-change rerun does not create an empty commit.
6. Multiple ZIP ambiguity refuses to guess and suggests explicit positional syntax.
7. `gitmake Project.zip` disambiguates the source.
8. `gitmake init` creates configuration without publishing.
9. Configured GitHub Release creation and local asset upload.

The existing unit suite additionally covers ZIP traversal/path safety, Windows-invalid archive names, case collisions, config validation/BOM behavior, Git branch/update logic, and snapshot mirroring.

## Windows-specific note

The binaries were cross-compiled for Windows from the available Linux build environment. `gitmake install`, PATH registration through Windows PowerShell, Setup execution, and the post-exit self-replacement used by `gitmake upgrade` cannot be executed natively in this environment; they are compile-validated and isolated behind Windows build-tag implementations. The portable publish/update path is also covered by the cross-platform E2E tests.
