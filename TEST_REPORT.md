# GitMake v1.2.9 Test Report

Verified on Windows 11 x64 locally, and on Linux, Windows and macOS in CI.

## Gates

| Gate | Result |
| --- | --- |
| `go build ./...` | PASS |
| `go vet ./...` | PASS |
| `go test ./...` | PASS |
| `go test ./...` a second time | PASS (the suite is repeatable) |
| `go test -race ./...` | PASS on Linux and macOS in CI |
| Windows E2E: v05, v051, v052, v121, v123, v124, v126 | PASS locally and in CI |
| Linux E2E: all remaining suites | PASS |
| `package` job: build, verify manifest, rebuild from the source ZIP | PASS |

CI on the release commit: 8 of 8 green.

## Coverage

| Package | v1.2.8 | v1.2.9 |
| --- | --- | --- |
| `internal/securityscan` | 83.1% | 87.3% |
| `internal/app` | 41.9% | 41.9% |
| `internal/planstore` | 82.6% | 82.6% |
| `internal/discovery` | 81.8% | 81.8% |
| `internal/github` | 69.3% | 69.3% |
| `internal/gmerr` | 100% | 100% |

## What is proven that was not before

The secret scan was made roughly a hundred times faster. Everything below
exists so that speed cannot have cost detection, which is the failure this
change could plausibly have introduced and which no existing test would have
caught: a literal gate that is too narrow stops finding a credential without
failing, and a parallel scan can make findings depend on which worker finished
first.

- Every content rule returns the identical offset gated and ungated, over
  samples covering every branch of every alternation: all five GitHub token
  prefixes, both AWS prefixes, all five Slack ones, both Stripe ones, every
  `-----BEGIN` variant, Discord's canary and ptb hosts, and OpenAI's three
  account prefixes.
- Those samples are proven to be real positives first, so the agreement above
  cannot be satisfied by inputs that match nothing.
- Every rule declares a literal gate, and every rule has a sample. A rule added
  later without either is a test failure, not a silent regression.
- A rule that declares no literals runs its regex unchanged, so the fallback
  costs speed rather than detection.
- The parallel scan equals the sequential scan of the same tree at 2, 3, 4, 8
  and 16 workers, twenty runs each, over nineteen kinds of secret in sixty
  files — two kinds in one file, secrets three thousand lines deep, binary
  files, a secret-by-name `.env`, and a large file. Reports must be deeply
  equal, order included.
- The same tree scanned repeatedly at the default worker count gives the
  identical report every time.
- An unreadable file produces the same error on every run, so the gate cannot
  sometimes pass and sometimes fail.

The equivalence test was verified to fail rather than assumed to work.
Collecting results in completion order — the classic form of this bug — makes
the scanner attribute secrets to the wrong files, and the test catches it on
the first run.

The whole scanner was also compared against v1.2.8 end to end. A 125-file
fixture covering all nineteen rules, binary files, `.env`, a documentation
placeholder that must not block, a large file, an empty file and an empty
directory produced 97 findings on both versions and byte-identical report JSON.

## Benchmarks

Committed as `BenchmarkScanTree` and `BenchmarkScanContentOneFile`, so a later
change to the rule table cannot quietly make the publish floor worse. Trees are
warmed before timing: on Windows the first read of a freshly written file also
pays for the virus scanner, which would otherwise swamp the measurement.

| tree | v1.2.8 | v1.2.9 |
| --- | --- | --- |
| 100 files × 4 KiB | 118.7 ms | 3.4 ms |
| 500 files × 16 KiB | 3,958 ms | 14.2 ms |
| 2000 files × 16 KiB | 17,271 ms | 175 ms |

Allocation on the last: 2.17 GB to 11.5 MB.

## Not verified here

- The race detector on the development machine, which has no C compiler. The
  CI race jobs on Linux and macOS cover it, and both pass.
- Linux and macOS self-upgrade against a real GitHub release. The replacement
  is platform-neutral Go and is unit-tested, and the platform asset mapping is
  tested against the published release names, but no such machine was used.
