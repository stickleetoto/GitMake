# GitMake v1.2.6 Test Report

Verified on Windows 11 x64, Go 1.26.5.

## Core verification

Run against the published `GitMake_v1.2.6_Source.zip`, extracted to a clean directory:

- `go build ./...` — PASS
- `go test ./...` — PASS
- `go test ./...` a second time — PASS (see below)
- `go vet ./...` — PASS
- `gitmake --version` — `gitmake 1.2.6`
- `sha256sum -c GitMake_v1.2.6_SHA256.txt` — all 6 assets OK

### The suite is now repeatable

`go test ./...` previously passed exactly once per machine. The approval tests isolated their store with `XDG_CACHE_HOME`, which `os.UserCacheDir` honours only on Linux, so on Windows and macOS they wrote consumed-grant markers into the real store under fixed plan ids and every later run failed with `approval for plan gm_tokenless_test was already used`. Go's test cache masked it in between. Reproduced on both v1.2.5 and this branch before fixing.
- `go test -race ./...` — NOT RUN. The race detector requires cgo and no C compiler is installed on this machine (`cgo: C compiler "gcc" not found`). This must be run on a host with a C toolchain before release.

## Root-cause regression guard

The defect fixed in v1.2.6 was that the staged replacement helper never executed, because `DETACHED_PROCESS` makes `powershell.exe` exit 0 without running the script. The new guard was confirmed to detect it:

- With `CREATE_NO_WINDOW` (the fix): `TestStagedHelperActuallyRunsTheScript` — PASS
- With `DETACHED_PROCESS` (the defect, temporarily restored): `TestStagedHelperActuallyRunsTheScript` — FAIL, `replacement helper did not start within 10s (no log at ...)`

The entire v1.2.5 suite passed against the defect, which is why it shipped three times.

## Process-level coverage

| Test | Proves |
| --- | --- |
| `TestReplaceExecutableReplacesAnImageThatIsStillRunning` | A real executable held open by a live process is replaced, and a fresh invocation of the canonical path reports the new version while the holder keeps running. |
| `TestStagedHelperActuallyRunsTheScript` | The fallback helper is really launched and really executes. |
| `TestStagedHelperReplacesInsideANonASCIIPath` | The helper works against a Korean install directory. |
| `TestUpgradeInstallsTheNewExecutable` | The full download → checksum → extract → replace pipeline installs a new version against a locally built release package, and leaves no temporary download directory. |
| `TestUpgradeFailsClosedOnChecksumMismatch` | A package failing SHA-256 never reaches the install target. |
| `TestUpgradeReportsMissingAssetWithoutTouchingTarget` | A release missing its platform asset fails without modifying the target. |
| `TestReplaceExecutableNeverLeavesTargetMissing` | A failed replacement leaves the original executable in place. |
| `TestReplacementNeverDeletesTheTargetFirst` | The helper renames aside instead of deleting, and restores on failure. |
| `TestHelperScriptIsWrittenWithABOM` | Non-ASCII paths survive Windows PowerShell's ANSI default. |
| `TestReplaceExecutableHandlesSpacesAndNonASCIIPaths` | Install paths with spaces and Korean characters. |
| `TestStageRejectsAStaleLogAsProofOfStart` | The start handshake cannot be satisfied by an old log. |
| `TestDescribeHTTPFailureIsActionable` | Rate limiting, proxy blocking, missing releases, and outages are distinguished. |
| `TestReplaceOrStageInstallsWithoutDeletingTheTargetFirst` | The installer no longer deletes the target before replacing it. |

## E2E suites

| Suite | Result |
| --- | --- |
| `e2e_v123.sh` locked executable recovery | PASS |
| `e2e_v124.sh` respawn-safe replacement | PASS (assertions updated to the contract rather than the old statement order) |
| `e2e_v126.sh` updater lifecycle | PASS |
| `e2e.sh`, `e2e_v03/v05/v051/v052/v07/v072/v073/v080/v090/v0100/v100/v110/v120/v121/v125` | NOT RUN on Windows — refused by the new `require_fake_gh.sh` gate, exit 70. Must be run under Linux, macOS, or WSL. |

### Why those suites are refused

Those suites stub the GitHub CLI with an extensionless `gh` shell script. Go's `exec.LookPath` resolves commands through `PATHEXT` on Windows and never finds an extensionless file, so GitMake fell through to the real, authenticated `gh`. During this work a Windows run of `e2e_v100.sh` and `e2e_v110.sh` created two live private repositories on the signed-in account before their first assertion failed. The gate now refuses to start instead.

This means the v1.0 / v1.1 / v1.2 / v1.2.5 suites were never a valid Windows gate, and the "Windows-verified" claims in earlier test reports overstated what had actually been exercised on Windows.

## Not verified here

- `go test -race ./...` (no C compiler on this host).
- Linux and macOS self-upgrade. The rename-aside replacement is platform-neutral Go and is unit-tested, and the platform asset mapping is tested against the published release names, but no macOS or Linux machine was available to run the real upgrade.
- The GitHub-CLI-dependent E2E suites, for the reason above.

No repository was published by GitMake during the v1.2.6 verification work.
