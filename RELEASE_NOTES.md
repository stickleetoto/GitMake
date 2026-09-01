# GitMake v1.2.6 — Self-Upgrade Actually Replaces the Executable

GitMake v1.2.6 fixes `gitmake upgrade` on Windows. It does not change the publishing safety model or approval semantics.

## Root cause

The deferred replacement helper introduced in v1.2.3 was launched with the `DETACHED_PROCESS` creation flag. `powershell.exe` is a console subsystem application: started without a console it exits immediately with status `0` **without executing the `-File` script at all**. `cmd.Start()` returned successfully, the exit code looked clean, and GitMake printed `✓ Upgrade staged` — while the helper had never run a single line.

Every staged replacement from v1.2.3 through v1.2.5 was a silent no-op. The retry loops, exact-path process eviction, and respawn-race hardening added across those releases never executed once. The synchronous installer path did not set that flag, which is exactly why installing from a downloaded copy worked while self-upgrade never did.

The existing tests asserted only on the *text* of the generated PowerShell script, so they could not see any of this. Every one of them passed.

## Fixed

### Replacement happens, in process, and is verified

`gitmake upgrade` no longer defers replacement to a detached helper in the normal case. Windows refuses to delete or overwrite the file backing a running image, but it does permit renaming it, so GitMake renames the current executable aside, moves the verified new executable into the canonical path, and only then removes the displaced file. The outcome is confirmed on disk before the command returns.

No process has to be stopped for an upgrade to succeed, including the one performing it. MCP stdio servers still running the old build keep running from the renamed file until they exit, so an MCP host that auto-respawns can no longer block replacement.

### Replacement is never destructive

The current executable is no longer deleted before the new one is in place, and a failed attempt restores it. The previous helper ran `Remove-Item $dst` before `Move-Item`, which could leave the install directory with no `gitmake.exe` at all.

### Non-ASCII install paths

Generated PowerShell helper scripts are now written with a UTF-8 BOM. Windows PowerShell 5.1 decodes a BOM-less `-File` script using the system ANSI code page, so on a Korean (or any non-Latin) Windows install the embedded paths were corrupted before the script ran and the helper failed with `Illegal characters in path`.

### Truthful reporting

GitMake prints `✓ Installed <tag>` only for a replacement that has actually completed. When a deferred helper is genuinely required it prints `· Replacement scheduled after this process exits` with the log path and a verification command, and the helper must prove it started before that is reported at all.

`gitmake upgrade` replaces the executable that was invoked and now names it. When that is not the installed copy on PATH, the output says so instead of letting the user assume the PATH command was updated.

### Other updater fixes

- The download directory is removed once replacement completes, instead of leaking roughly 10 MB per attempt into `%TEMP%` permanently.
- `gitmake upgrade` works on Linux and macOS. The updater previously refused to run anywhere but Windows x64 despite those release builds being published.
- GitHub failures distinguish anonymous rate limiting, proxy blocking, missing releases, and outages instead of surfacing a bare `GitHub returned HTTP 403`.
- Brief sharing violations from antivirus or the search indexer are absorbed by a short bounded rename retry.

## Verification

- `go build ./...`
- `go test ./...`
- `go vet ./...`
- v1.0 guided/tokenless stability E2E
- v1.1 chat approval E2E
- v1.2 one-shot publish E2E
- v1.2.1 protocol routing E2E
- v1.2.3 locked executable recovery E2E
- v1.2.4 respawn-safe replacement E2E
- v1.2.5 real-world workflow E2E
- v1.2.6 updater lifecycle E2E
- GitMake source-tree self secret scan

New process-level coverage replaces the string-only helper assertions: the staged helper is launched for real and must produce observable evidence that it ran, and a live executable is replaced while a process is still running it. The regression guard was confirmed to fail against the `DETACHED_PROCESS` flag that caused this bug.

No repository was published during these local regression checks.

## Upgrading from v1.2.5 or earlier

Because self-upgrade never worked on Windows, an installation at v1.2.5 or earlier cannot upgrade itself to v1.2.6. Install once from the release package:

```powershell
.\gitmake.exe install
```

`gitmake upgrade` works normally from v1.2.6 onward.
