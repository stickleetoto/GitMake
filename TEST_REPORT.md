# GitMake v1.3.0 Test Report

Verified on Windows 11 x64 locally, and on Linux, Windows and macOS in CI.

## Gates

| Gate | Result |
| --- | --- |
| `go build ./...` | PASS |
| `go vet ./...` | PASS |
| `go test ./...` | PASS |
| `go test ./...` a second time | PASS (the suite is repeatable) |
| `go test -race ./...` | PASS on Linux and macOS in CI |
| Windows E2E: v05, v051, v052, v121, v123, v124, v126, v130 | PASS locally and in CI |
| Linux E2E: all remaining suites | PASS |
| `package` job: build, verify manifest, rebuild from the source ZIP | PASS |

## Coverage

| Package | v1.2.9 | v1.3.0 |
| --- | --- | --- |
| `internal/history` | 0% | 79.2% |
| `internal/gitops` | 28.1% | 49.7% |
| `internal/app` | 41.9% | 49.3% |
| `internal/securityscan` | 87.3% | 87.3% |
| `internal/planstore` | 82.6% | 82.6% |
| `internal/gmerr` | 100% | 100% |

`internal/history` had no tests at all. It was decoration until this release
made it the thing `gitmake undo` reads to decide what to revert.

## What is proven that was not before

### Undo reverts without removing

Driven end to end against the stubbed GitHub CLI with a bare repository on
disk, so the remote is inspected rather than inferred:

- a revert takes the remote from two commits to three; it never removes one
- the reverted commit stays reachable afterwards, because undo is not deletion
- content returns to the previous publish, and a file that publish added is gone
- the same operation against real Git, in `internal/gitops`: the tree after a
  revert equals the tree before the publish

### Undo refuses rather than guesses

- a branch that moved on since the publish is refused, with `REMOTE_MOVED`, and
  the remote is unchanged afterwards
- a publish that created the repository is refused, saying there is no earlier
  state and that repository deletion is the user's to do
- a second undo of the same publish is refused with `NOTHING_TO_UNDO`
- `--dry-run` changes nothing and does not consume the entry
- the selection rules are also tested directly against ten history shapes,
  because the end-to-end tests cannot easily produce every one

### Undo is not a way around the confirmation table

`--yes` cannot accept an undo whose revert would delete most of a repository.
The risk is computed against the managed-file baseline the previous run
recorded; the prompter is injected so the result does not depend on whatever
stdin the test runner was handed.

### Undo tells the truth about what it did not do

The output must say that the reverted contents remain published and that a
published credential has to be rotated. Asserted in the Go tests and again in
`e2e_v130` against the real binary.

### Publishes are confirmed against GitHub

- a publish records a full commit SHA, and the remote really points at it
- a release is checked for existence and for every asset at the size uploaded
- a missing asset and a truncated one are both reported
- an unreported size is not treated as a mismatch
- a dry run produces no verification, because it has nothing to confirm

## Guards verified to fail

Every new guard was run against the defect it describes rather than assumed to
work:

| Guard | Sabotage | Result |
| --- | --- | --- |
| Simple Mode publishes are undoable | remove the state copy-back | FAIL: "left no repository" |
| Undo respects the destructive ceremony | drop the managed baseline | FAIL: "--yes accepted an undo that deletes most of the repository" |
| History entries do not collide | replace the counter with a constant | FAIL: 25 writes stored 14 |
| Publishes are verified | return early from the VERIFY stage | FAIL: "no verification at all" |

## Not verified here

- The race detector on the development machine, which has no C compiler. The
  CI race jobs on Linux and macOS cover it.
- `gitmake undo` against a real GitHub repository. It is driven end to end
  against the stubbed CLI and real Git, and the operations it performs
  (`ls-remote`, `revert`, `push`) are exercised against real repositories in
  `internal/gitops`, but no live repository was reverted.
- Linux and macOS self-upgrade against a real GitHub release.
