# GitMake v1.2.8 — The Publish Pipeline, Taken Apart

v1.2.7 gave GitMake continuous integration. This release uses it: the publishing pipeline is separated into stages that can be tested individually, and the safety rules it enforces are pinned by tests for the first time.

Nothing about publishing changes. No config, plan, approval, project-identity or MCP interface moves. The E2E suites are the contract, and they were run at every step.

## A safety promise that nothing was checking

`STABILITY.md` promises specific confirmation friction for each reviewed risk: `--yes` accepts low-risk plans only, a medium-risk plan requires a typed `PUBLISH`, and a destructive one requires a phrase carrying that plan's own suffix.

Nothing tested it. The rules lived inside one function that printed to stdout and read a terminal in the same breath, so the promise and the code could drift apart with nobody noticing. The rules are now separate from the conversation, and the contract is a table:

| Reviewed risk | Confirmation | `--yes` |
| --- | --- | --- |
| low or unset | `[Y/n]` | accepted |
| medium | typed `PUBLISH` | refused |
| high | typed `DELETE-XXXXXX` | refused |
| destructive | typed `DELETE-XXXXXX` | refused, whatever the level says |

The destructive phrase carries the plan id and is compared exactly, so a confirmation cannot be moved between plans.

## runPublish: 295 lines to 47

The publish already had five stages — it announced `DISCOVER`, `PLAN`, `PREPARE`, `SECURITY` and `VALIDATE` as it ran — but all five shared one scope, so none could be read or tested on its own. They are separate now, and `runPublish` reads as what it always was: discover, plan, snapshot, report, apply.

Two things had to be handled rather than moved. The workspace cleanup and the repository lock belong to the whole publish, not to the stage that creates them, so those stages return their cleanup for `runPublish` to defer. And `VALIDATE` turned out to reject nothing — by the time it runs, the snapshot has matched the reviewed hash and the security gate has passed — so it is now named for what it does: report.

`app.go` drops from 1,320 lines to 1,070.

## Tests where a bug would be worst

`internal/app` holds the publishing pipeline and had 26% coverage, because reaching any part of it meant reaching all of it, with a real GitHub account and a terminal. With the stages separate they can be driven against the stubbed GitHub CLI. Coverage is now 42%.

The suite now proves, on Windows as well as Linux:

- a repository is created with the expected files and one commit; a second publish reports `UPDATE` with exactly one modification; republishing an unchanged source adds no commit
- a dry run creates nothing
- a configured release is created with its assets, and a duplicate tag is refused
- publishing a folder bound to one repository into a different one stops with `PROJECT_IDENTITY_MISMATCH`
- managed sync leaves remote-only files alone and removes only what GitMake published
- a mass deletion is classified destructive and blocked **before** any mutation, while an ordinary five-file cleanup is not
- an update reports a visibility mismatch and never changes the remote
- a detected secret blocks the publish while its findings survive, so the user can see what to fix

## Four E2E suites retired, none of their checks lost

v1.2.7 quarantined `e2e_v03`, `v07`, `v072` and `v073`. Each ended in the pre-v1.0 approval flow that printed a copyable token, which v1.0 removed, so they could not run at all.

Calling them obsolete was too quick: only their tail was. Everything before it asserted behaviour GitMake still has, and quarantining the suites took those checks out of service. They are ported to Go above — where they also run on Windows, which the pty-driven originals never could — and the suites are removed along with the CI quarantine step. Nothing is left permanently red.

## Also fixed

The Windows test job went red on `main` immediately after v1.2.7, having passed locally and on two pull-request runs. Staged replacements keyed their helper script and log on the process id alone, so every replacement in one process shared them and timing decided whether the suite passed. The same collision applied outside tests: two GitMake processes replacing at once shared both files. Each replacement now gets its own.

## Verification

- `go build`, `go vet`, `go test` — and `go test` again, to prove the suite is repeatable
- Full CI on Linux, Windows and macOS, including the race detector and the packaging job
- Windows E2E: `e2e_v05`, `e2e_v051`, `e2e_v052`, `e2e_v121`, `e2e_v123`, `e2e_v124`, `e2e_v126`
- Every extraction step verified against the full suite and the E2E gates before the next began

## Upgrading

`gitmake upgrade` installs this release from v1.2.6 or later.

From v1.2.5 or earlier, self-upgrade cannot move you — staged replacement never ran in those builds. Extract the platform package and install once:

```powershell
.\gitmake.exe install
```

```bash
./gitmake install    # Linux / macOS
```
