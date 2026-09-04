# GitMake v1.2.8 Test Report

Verified on Windows 11 x64 locally, and on Linux, Windows and macOS in CI.

## Gates

| Gate | Result |
| --- | --- |
| `go build ./...` | PASS |
| `go vet ./...` | PASS |
| `go test ./...` | PASS |
| `go test ./...` a second time | PASS (the suite is repeatable) |
| `go test -race ./...` | PASS on Linux and macOS in CI |
| Windows E2E: v05, v051, v052, v121, v123, v124, v126 | PASS |
| Linux E2E: all remaining suites | PASS |
| `package` job: build, verify manifest, rebuild from the source ZIP | PASS |

Every extraction step in this release was verified against the full suite and
the E2E gates before the next one began.

## Coverage

| Package | v1.2.7 | v1.2.8 |
| --- | --- | --- |
| `internal/app` | 26.2% | 41.9% |
| `internal/github` | 69.3% | 69.3% |
| `internal/planstore` | 82.6% | 82.6% |
| `internal/securityscan` | 83.1% | 83.1% |
| `internal/gmerr` | 100% | 100% |

## What is proven that was not before

The confirmation rules `STABILITY.md` promises now have tests: the full
risk table, that `--yes` cannot answer for a person above low risk even at a
terminal, and that a destructive phrase minted for one plan cannot confirm
another.

The publish pipeline is driven end to end against a stubbed GitHub CLI with a
bare repository on disk, so results are inspected rather than inferred:

- a repository is created with the expected files and one commit
- a second publish reports UPDATE with exactly one modification
- republishing an unchanged source adds no commit
- a dry run creates nothing
- a configured release is created with its assets; a duplicate tag is refused
- a bound folder published elsewhere stops with PROJECT_IDENTITY_MISMATCH
- managed sync leaves remote-only files alone, and removes only what GitMake published
- a mass deletion is classified destructive and blocked before any mutation
- an ordinary five-file cleanup is not
- an update reports a visibility mismatch and never changes the remote
- a detected secret blocks the publish while its findings survive for the user

## Retired

`e2e_v03`, `e2e_v07`, `e2e_v072` and `e2e_v073` are removed. Each ended in the
pre-v1.0 approval flow that printed a copyable token, which v1.0 removed, so
none could run. Everything they asserted that GitMake still does is listed
above; the rest was already covered in `internal/github`, `internal/discovery`,
`internal/securityscan` and `mcp_test`.

## Not verified here

- Linux and macOS self-upgrade against a real GitHub release. The replacement
  is platform-neutral Go and is unit-tested, and the platform asset mapping is
  tested against the published release names, but no such machine was used.
