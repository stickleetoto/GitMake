# GitMake v0.7.1 Test Report

Status: **PASS** for the implemented v0.7.1 scope.

## Focus

v0.7.1 closes seven safety/portability gaps: managed sync ownership, secret scanning, one-shot AI approval, self-contained MCP inspection/config planning, conservative multi-ZIP discovery, GitHub preflight constraints, and cross-platform/generic MCP support.

## Regression suites

The release candidate is validated with:

```text
go test ./...
go vet ./...
go test -race ./...
scripts/e2e.sh
scripts/e2e_v03.sh
scripts/e2e_v04.sh
scripts/e2e_v05.sh
scripts/e2e_v051.sh
scripts/e2e_v052.sh
scripts/e2e_v06.sh
scripts/e2e_v061.sh
scripts/e2e_v07.sh
```

## v0.7-specific E2E coverage

1. Managed sync first adoption preserves remote-only files.
2. Later managed updates remove files GitMake previously owned but no longer receives.
3. Protected paths survive snapshot-style updates.
4. Likely secrets are blocked before a remote repository is created or modified.
5. Configurable direct-Git file limits block oversized files.
6. LFS-marked oversized files require `git lfs` availability.
7. Required-PR branch protection blocks the direct-push workflow without creating an extra commit.
8. An existing bare Git tag blocks accidental release creation against an unreviewed tag.
9. An obvious binary/release-only ZIP is not silently selected as project source.
10. Generic MCP registration descriptor is emitted without modifying unknown client configuration.
11. Unix install uses `~/.local/bin/gitmake` and idempotently manages the shell PATH snippet.
12. A real PTY-backed human approval creates a one-shot token; MCP apply succeeds once and token replay is rejected.
13. Existing plan/apply stale-input and remote-baseline checks remain enforced.
14. Existing v0.1–v0.6.1 workflows remain covered by regression suites.

## Safety invariants

- no force push
- no history rewrite
- no repository deletion
- source ZIP may not inject `.git`
- ZIP traversal/symlink/Windows-invalid-name defenses remain active
- secret and large-file preflight occurs before Git/GitHub mutation
- managed sync does not claim arbitrary remote-only files on first adoption
- branch protection requiring PRs is not bypassed
- bare tag conflicts are not silently reused
- MCP cannot mint its own approval token
- approval tokens expire, are plan-bound, and are single-use
- apply revalidates reviewed state before mutation

## Platform build targets

Release packaging builds/tests the Go source on the host and cross-compiles CLI binaries for Windows amd64, Linux amd64, macOS amd64, and macOS arm64. `GitMake-Setup.exe` remains the Windows convenience installer; Unix-like systems use `gitmake install`.

Native UI execution for every target OS is not available in the build container; platform-specific install logic is unit/E2E tested where possible and cross-compilation is verified.
