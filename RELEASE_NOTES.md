# GitMake v1.3.0 — A Way Back, and a Way to Be Sure

GitMake could publish but never return. `gitmake history` said what had happened and left you to work out the rest by hand. And a publish reported success on the strength of the commands it had just run, never on what GitHub actually held.

This release fixes both, and they turn out to be the same fix: a publish now records the commit it created. That is what makes verification possible, and it is what gives an undo something to revert.

Nothing about publishing itself changes. No config, plan, approval, project-identity or MCP interface moves.

## `gitmake undo`

Reverts the most recent GitMake publish by **adding** a commit.

```text
gitmake undo --dry-run    show what would be reverted, change nothing
gitmake undo              revert it
```

Not a reset, not a force push, not a deletion. The safety contract forbids all three, and none of them would achieve more — removing a commit from a branch does not remove it from GitHub.

It stops rather than guessing in three cases:

- **The branch has moved on.** The published commit must still be the tip. Reverting under somebody else's later push would undo a state that no longer exists, and deciding what they meant is not GitMake's call to make.
- **The publish created the repository.** There is no earlier state to return to. GitMake does not delete repositories, so removing it is yours to do on GitHub.
- **That publish was already undone.**

Undo is confirmed with the same ceremony a publish of the same risk would need, computed against the managed-file baseline the previous run recorded, so `--yes` cannot accept a destructive one. Releases and tags are left in place: GitMake still has no code that deletes anything on GitHub, and this release does not add any.

## Undo does not unpublish, and says so

This is the part that decides whether the command is worth having at all.

Reverting adds a commit. It removes nothing: the previous contents stay reachable by SHA, through the GitHub API, and in every fork, clone and CI log that already read them. A user who believes an undo removed a leaked credential is worse off than one who never ran it. So GitMake prints this on every undo, not only when it thinks it saw a secret:

```text
! What was published stays published.
  The reverted contents remain reachable in Git history and through the
  GitHub API, and in any fork, clone or CI log that already has them.
  If a credential was published, undoing does not unpublish it: rotate it.
```

## Every publish is now confirmed against GitHub

A publish used to end by reporting what it had asked GitHub to do. Nothing asked GitHub what it did.

That is the exact shape of failure this project keeps finding. The staged upgrade helper reported success for three releases while doing nothing at all, because a zero exit code was read as proof of work. So the last thing a publish does now is ask:

- the remote branch must point at the commit that was pushed;
- a release this run created must exist, with every asset present at the size it was uploaded with.

```text
✓ Verified              remote at 4a91c7f0b2de · 7 assets
```

A check that runs and disagrees is a failure, and the command exits non-zero. A check that cannot run — a network error while reading back a push that already succeeded — is reported as "not verified" and does not fail the publish. Those are different events, and GitMake reports them differently.

Simple Mode's summary carries a `Verified` line too. Whether GitHub holds what was just approved is not a detail to leave the reader to assume.

## Three defects found while building this

Each was found by the new tests, not in use.

**Simple Mode publishes recorded useless history.** The path a person actually takes ran the apply on its own pipeline state and never copied it back, so every history entry written from it had no repository, no branch and no commit. Nothing noticed while history was only ever read by a human — but `gitmake undo` could not find the publish it was meant to return. Simple Mode has been recording empty entries since history was introduced.

**Undo could never be classified destructive.** Risk here is a ratio, and the first version passed no managed-file baseline, which left the denominator at zero and the destructive rule unreachable. An undo removing every file in a repository would have been offered as an ordinary `[Y/n]` question that `--yes` could answer.

**History entries overwrote each other.** File names were derived from `time.Now()`, and Windows clock granularity is around 15 ms, so entries written back to back shared a name. Twenty-five consecutive writes stored fourteen. This is the same defect, for the same reason, as the one fixed in the upgrade helper in v1.2.8, and it is fixed the same way.

## Verification

- `go build`, `go vet`, `go test` — and `go test` again, to prove the suite is repeatable
- Full CI on Linux, Windows and macOS, including both race jobs and the packaging job
- Windows E2E: `e2e_v05`, `e2e_v051`, `e2e_v052`, `e2e_v121`, `e2e_v123`, `e2e_v124`, `e2e_v126`, and the new `e2e_v130`
- Every new guard was verified to fail against the defect it describes, rather than assumed to work
- Coverage: `internal/history` 0% to 79%, `internal/gitops` 28% to 50%, `internal/app` 42% to 49%

## Upgrading

`gitmake upgrade` installs this release from v1.2.6 or later.

From v1.2.5 or earlier, self-upgrade cannot move you — staged replacement never ran in those builds. Extract the platform package and install once:

```powershell
.\gitmake.exe install
```

```bash
./gitmake install    # Linux / macOS
```
