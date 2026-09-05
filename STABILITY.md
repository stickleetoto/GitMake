# GitMake v1 Stability Contract

GitMake v1.0.0 froze the public publishing interface after the 0.x development line. v1.1.0 added MCP chat approval as an optional, backward-compatible approval transport. v1.2.0 added `gitmake_publish` as a new high-level orchestration tool without changing the frozen lower-level interfaces. v1.2.1 hardens protocol routing and approval-state validation without changing those interfaces. v1.2.2 moves self-upgrade discovery/download to anonymous public GitHub HTTPS, preserves checksum verification, and adds downgrade refusal without changing the publishing interfaces. v1.2.3 adds Windows locked-executable recovery through an exact-path, detached replacement helper without changing publishing interfaces. v1.2.4 makes that helper resilient to MCP host auto-respawn by repeating exact-path eviction before every replacement attempt, again without changing publishing interfaces. v1.2.5 hardens real-world input/error observability: stdin config now fails closed and is actually applied, security findings are aggregated, and MCP preserves existing structured CLI failures. v1.2.6 makes `gitmake upgrade` actually replace the installed executable: replacement moved from a detached helper that never ran to an in-process rename-aside that is verified before the command returns, is non-destructive, requires stopping no process, and is reported truthfully. v1.2.7 adds continuous integration, coded machine errors, broader secret detection, and read-only machine surfaces for `doctor`, `install`, and `upgrade`. v1.2.8 separates the publish pipeline into testable stages and pins the confirmation rules with tests. v1.2.9 speeds up the secret scan by roughly a hundred times and parallelises it, with no change to what it detects: the scan is held to byte-identical reports against v1.2.8, and every rule is proven to reach the same verdict gated and ungated. v1.3.0 adds `gitmake undo` and confirms every publish against GitHub before reporting success. Through v1.2.9 these were all bug fixes, additive diagnostics and internal structure, not a new publishing surface; `gitmake undo` is the first command added since v1.0 that changes a repository, and the section below is its contract.

## Stable through v1.x

GitMake v1.x will preserve these interfaces unless a security issue makes a breaking change unavoidable:

- Everyday CLI entry points: `gitmake`, `gitmake <source>`, `gitmake upgrade`, `gitmake approve`, `gitmake ai setup`.
- Exit-code classes: `0` success, `1` runtime/safety failure, `2` usage error.
- Config schema: `gitmake.config/v1` / `schema_version: 1`.
- Reviewed-plan schema: `gitmake.plan/v1`.
- Project identity: `gitmake.project/v1`.
- Managed-sync metadata semantics and protected-path behavior.
- MCP tool names already shipped in v1.0, especially `gitmake_prepare` and `gitmake_apply`.
- `gitmake_publish` is additive in v1.2 and composes the frozen prepare/approval/apply semantics rather than replacing them.
- The `gitmake.doctor/v1`, `gitmake.install/v1`, and `gitmake.upgrade/v1` JSON reports added in v1.2.7 are read-only diagnostics. They do not change any command's behaviour and carry no approval authority.
- `gitmake_apply` requires only `plan_id` in the v1 tokenless approval flow. A legacy pre-1.0 `approval_token` remains optional for compatibility.
- Safety invariants: no force push, no history rewriting, no repository deletion, no silent project retargeting, and no automatic bypass of protected branches or destructive-change gates.

## Approval semantics

Since v1.2.8 the confirmation rules are enforced by a tested table: `--yes` is accepted for low-risk plans only, a medium-risk plan requires a typed `PUBLISH`, and a destructive one requires a plan-bound `DELETE-XXXXXX` compared exactly. `--yes` cannot answer for a person at any level above low, and a phrase minted for one plan cannot confirm another.

`gitmake approve` continues to create a short-lived local, single-use approval grant bound to the exact reviewed plan fingerprint, source hash, config hash (when persisted), and target repository. v1.1 may create the same class of grant from a **client-controlled MCP elicitation response** when the connected client supports elicitation. This does not change the required `plan_id` input of `gitmake_apply`.

Normal approvals expire after 10 minutes and are consumed only after a successful MCP apply. Chat approval requires explicit client acceptance; medium/destructive risk still requires stronger typed confirmation. Destructive terminal fallback remains `gitmake approve --destructive`.

## Compatible evolution

v1.x may add optional JSON fields, new MCP tools, new platforms, new diagnostics, or performance improvements. Existing required fields and command meanings will not be repurposed.

## Self-upgrade contract

`gitmake upgrade` verifies the SHA-256 of the downloaded package before anything on disk is touched, refuses to install an older published release over a newer local build, and never leaves the install path without an executable: the current one is renamed aside rather than deleted, and it is restored if the replacement cannot be completed.

GitMake reports an upgrade as installed only after the replacement exists on disk. When replacement must be deferred to a helper, the helper has to prove it started, and the CLI says the replacement is scheduled rather than done. `gitmake upgrade` replaces the executable that was invoked and reports that path.

## Undo (added v1.3.0)

`gitmake undo` reverts the most recent GitMake publish by adding a commit. It is bound by the same safety invariants as everything else, and by four of its own:

- **It never removes a commit.** Undo is `git revert`, not a reset and not a force push. History is only ever appended to.
- **It refuses when the branch has moved.** The published commit must still be the branch tip. Reverting under somebody else's later push would undo a state that no longer exists, and GitMake will not decide what was meant.
- **It cannot undo a publish that created the repository.** There is no earlier state to return to, and GitMake does not delete repositories.
- **It does not unpublish.** Reverting adds a commit; the previous contents stay reachable by SHA, through the GitHub API, and in any fork, clone or CI log that already read them. GitMake says so on every undo. A published credential must be rotated; undoing is not remediation.

Releases and tags are left in place. GitMake has no code that deletes anything on GitHub, and v1.3.0 does not add any.

An undo is confirmed with the same ceremony a publish of the same risk would need, computed against the managed-file baseline the previous run recorded. `--yes` cannot accept a destructive one.

`gitmake undo` is a terminal command. It is not exposed as an MCP tool in v1.3.0.

## Publish verification (added v1.3.0)

Every non-dry-run publish now ends by asking GitHub what it actually holds, rather than by trusting the commands that just ran: the remote branch must point at the commit that was pushed, and a release this run created must exist with every asset present at the size it was uploaded with.

A check that runs and disagrees is a failure, and the command exits non-zero. A check that cannot run -- a network error while reading back a push that already succeeded -- is reported as "not verified" and does not fail the publish. The distinction is deliberate.

The `verification` object in `--json` pipeline output is additive and read-only.

## Recovery and concurrency

GitMake serializes concurrent mutations for the same plan/repository. Apply operations are journaled before mutation. If an apply is interrupted, the next attempt revalidates the complete reviewed plan against local source/config state and the GitHub remote before doing anything. GitMake never blindly resumes a stale write.

Release asset uploads can use the existing `release.on_existing: "resume"` behavior when a release exists but is incomplete.
