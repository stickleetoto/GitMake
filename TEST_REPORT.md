# GitMake v0.2.0 Test Report

Date: 2026-08-19

## Final result

PASS

## Static / unit validation

- `go test ./...` — PASS
- `go vet ./...` — PASS
- `go test -race ./...` — PASS
- Windows x64 cross-build — PASS
- Binary type — `PE32+ executable, x86-64, console`

## End-to-end regression

The E2E harness uses real Git repositories plus a fake `gh` adapter that emulates the GitHub CLI commands GitMake invokes.

Covered flows:

1. First-run CREATE with no config and one ZIP
2. Unicode / Korean working path
3. Snapshot UPDATE preserving Git history
4. Add / modify / delete mirror semantics
5. No-change update avoids empty commit
6. No-ZIP onboarding
7. Placeholder config self-repair
8. Multiple-ZIP ambiguity rejection
9. Existing empty remote repository
10. Legacy default-branch fallback (`main` -> repository default)
11. CREATE dry-run
12. UPDATE dry-run
13. `--create-only` guard
14. `--update-only` guard
15. GitHub authentication failure
16. Windows-reserved ZIP path rejection
17. Case-colliding ZIP path rejection
18. CREATE + GitHub Release + asset upload
19. New release with no repository file changes
20. Duplicate release early failure
21. `release.on_existing = skip`
22. Missing release asset fails before source mutation
23. Release dry-run
24. `--no-release`
25. Invalid Git tag rejection via `git check-ref-format`

Result: `ALL_E2E_PASS`

## Release safety properties verified

- Release configuration is optional and backward-compatible with v0.1.x JSON.
- Missing release assets are detected before commit/push.
- Invalid release tags are detected before commit/push.
- Existing release conflicts are detected before commit/push when `on_existing` is `error`.
- A failed release can be retried later without requiring a new repository commit if the tag/release does not yet exist.
- Existing releases are never deleted or overwritten by GitMake v0.2.0.
- `--dry-run` creates neither repository changes nor releases.
- `--no-release` keeps repository publishing functional while suppressing release creation.
