## v1.2.4 — Respawn-Safe Replacement

- Fixed a Windows replacement race where an MCP host could automatically respawn the installed GitMake after the helper stopped it, relocking `gitmake.exe` before the next retry.
- Exact-path GitMake process eviction now runs before every replacement attempt rather than only once.
- Manual `install`/setup launched from a downloaded executable now performs locked-target replacement synchronously, so success is reported only after the installed file has actually been replaced.
- Added a short `Wait-Process` synchronization after forced termination so Windows image/file handles can close before remove/move.
- Increased retry cadence to 250 ms while keeping the overall recovery window at roughly one minute.
- Preserved the exact-path safety boundary: GitMake instances running from Downloads or any other path are not terminated.
- Added regression coverage proving process eviction occurs inside the retry loop before each move attempt.

## v1.2.3 — Locked-Executable Recovery

- Fixed Windows install/update replacement when the installed `gitmake.exe` is held open by a long-lived GitMake process such as an MCP stdio server.
- Added a detached replacement helper that waits for the invoking GitMake process to exit, stops only GitMake processes whose executable path exactly matches the installed target, then retries replacement for transient antivirus/indexer locks.
- Installer results now distinguish immediate replacement from staged replacement and expose the helper log path.
- `gitmake upgrade` now uses the same exact-path replacement helper, preventing a successful download from silently failing to replace a locked executable.
- Added unit coverage for parent-process waiting, exact-path process scoping, safe PowerShell path quoting, and Windows cross-compilation of installer/upgrader/helper test binaries.

## v1.2.2 — Authless Self-Upgrade

### Fixed

- `gitmake upgrade` no longer requires GitHub CLI authentication for public GitMake releases.
- Self-upgrade now discovers the latest release through the public GitHub Releases API and downloads release assets over HTTPS.
- Added strict GitHub/GitHubusercontent HTTPS host validation for updater asset URLs.
- Added semantic version comparison so a newer local build is never downgraded to an older published release.
- Improved no-op upgrade output to show both the current version and the latest published tag.

### Safety

- Existing SHA-256 verification remains mandatory before replacement is staged.
- Publishing authentication and safety boundaries are unchanged; only the public self-updater was decoupled from `gh auth`.

### Verification

- Added anonymous public-release client tests.
- Added downgrade-refusal coverage.
- Existing v1.0/v1.1/v1.2/v1.2.1 regression suites remain green.

## v1.2.1 — Protocol Routing Hardening

- Fixed modern MCP 2026-07-28 sessions being able to accidentally arm the legacy held-open `elicitation/create` path after `initialize`.
- Legacy stdio elicitation is now enabled only for the 2025-11-25 protocol; modern publishing stays on MRTR `input_required` / `inputResponses`.
- Tightened signed MCP approval state validation so the `purpose` binding is mandatory, not optional.
- Added `e2e_v121.sh` to prove that a modern client which still sends `initialize` receives an MRTR `input_required` result rather than a legacy server-to-client request.
- Updated LLM/Claude guidance so `gitmake_publish` is consistently documented as the normal publishing entry point, with prepare → terminal approve → apply only as the compatibility fallback.
- Relaxed two historical E2E version assertions so they verify the stable v1 version/schema contract instead of incorrectly pinning the old `1.0.0` patch number.
- No config, plan, project, CLI, safety, or lower-level MCP compatibility break.

## v1.2.0 — One-shot Publish Orchestrator

- Added `gitmake_publish`, the primary high-level MCP publishing tool.
- `gitmake_publish` performs prepare → reviewed plan → client-controlled human approval → exact-plan revalidation → apply → final result in one interactive MCP operation.
- Claude/LLMs no longer need to end a turn after planning just to ask the user to approve before a separate apply call.
- Added both modern MCP 2026-07-28 MRTR (`input_required` / `inputResponses`) and legacy 2025-11-25 stdio elicitation support for the one-shot publish flow.
- Added purpose-bound signed approval request state so a `gitmake_publish` approval state cannot be replayed as a standalone `gitmake_apply` approval state.
- Preserved `gitmake_prepare`, `gitmake_apply`, terminal `gitmake approve`, tokenless one-shot grants, destructive confirmation, and all v1 safety invariants as stable expert/fallback paths.
- Clients without elicitation are never bypassed: GitMake directs them to the stable prepare → terminal approve → apply fallback.

## v1.1.0 — MCP Chat Approval

- Added human approval inside elicitation-capable MCP clients. `gitmake_apply` now requests a client-controlled form confirmation when no valid local approval grant exists.
- Claude Code can surface the approval dialog directly during the tool call; terminal `gitmake approve` remains the compatibility fallback.
- Added risk-adaptive chat confirmation: low risk uses the client Accept/Decline action, medium risk requires `PUBLISH`, and destructive/high risk requires the plan-specific `DELETE-XXXXXX` phrase.
- Added MCP 2026-07-28 Multi Round-Trip Request support (`input_required` / `inputResponses`) plus `server/discover`, while preserving legacy 2025-11-25 stdio `elicitation/create` compatibility.
- Added signed/expiring MRTR `requestState` binding to the reviewed plan fingerprint.
- Hardened one-shot approval replay: a consumed grant cannot be re-minted for the same reviewed plan; another mutation requires a fresh plan.
- Preserved all v1.0 CLI/config/plan/MCP tool names and the terminal approval fallback.

## v1.0.0 — Stable / Tokenless Approval

- Removed approval-token copy/paste from the normal MCP workflow. `gitmake approve` stores a local short-lived single-use grant bound to the exact reviewed plan; `gitmake_apply` needs only the plan ID.
- `gitmake approve` with no ID selects the newest reviewed plan for the current project directory.
- Added plan-level and repository-level mutation locks.
- Added apply operation journaling and interrupted-run revalidation.
- Added cache housekeeping for expired approvals, stale locks, old plans, and operation journals.
- Added an official Linux arm64 build for Raspberry Pi 4/5 and other ARM64 Linux systems.
- Added a folder-hashing performance benchmark and kept optimization measurement-driven.
- Froze the v1 CLI/config/plan/MCP/safety compatibility surface in `STABILITY.md`.
- Legacy pre-1.0 approval tokens remain accepted as an optional compatibility path.

## v0.10.0 — Guided UX + Trust & Recovery

- Added risk-adaptive Simple Mode confirmation: low risk uses `[Y/n]`, medium risk requires exact `PUBLISH`, and high/destructive plans require a plan-specific `DELETE-XXXXXX` phrase.
- Restricted `--yes` to low-risk Simple Mode plans; it cannot bypass medium/high-risk human review.
- Added `decision_notes` to reviewed plans and a human-readable `Why` section so automatic source/config/visibility/identity choices expose their actual evidence.
- Added a compact Simple Mode success result with repository, branch, change counts, release, elapsed time, and repository URL; detailed pipeline logs remain available with `--verbose`.
- Added guided error recovery with stable machine error codes plus actionable `Recommended` next steps for secrets, stale plans/remotes, ambiguous sources, GitHub auth, Git LFS, identity mismatches, and destructive-change blocks.
- Redesigned `GitMake-Setup.exe` as a first-run readiness flow: install/PATH, Git, GitHub CLI, GitHub login, optional Claude Code detection, read-only MCP connection, and clear next actions.
- Preserved v0.9 zero-config, project-memory, folder/ZIP ambiguity, Simple/Expert split, one-shot MCP approval, managed sync, secret scanning, and all existing safety invariants.
- Added v0.10 unit/E2E coverage for low/medium/high confirmation behavior, decision explanations, compact success output, guided recovery, setup build, and regression compatibility.

## v0.9.0 — Simple Mode + Zero-Config

- Made missing `gitmake.json` a true zero-config path: inferred settings stay in memory and are no longer written as a side effect of normal publish/plan/apply.
- Added interactive Simple Mode for `gitmake` / `gitmake Project.zip`: create a reviewed plan, show target/source/changes/risk/release, ask once, then apply the exact plan.
- Kept non-interactive automation non-blocking; `--yes` explicitly accepts safe Simple Mode confirmation while destructive plans still require the expert `--destructive` workflow.
- Added local folder project memory in `.gitmake/project.json`; a renamed project folder continues to target the original owner/repository without requiring config.
- Added project-memory mismatch hard stops instead of silently retargeting a folder that was previously bound to another repository.
- Added folder-vs-ZIP ambiguity detection. Interactive Simple Mode asks which source to use; machine/MCP flows return `SOURCE_AMBIGUOUS` instead of guessing.
- Split CLI documentation into a tiny default `gitmake help` surface and `gitmake help --expert` for config/plan/MCP/diagnostic commands.
- Changed `gitmake_prepare` so zero-config remains the default even with MCP write access; persistent config now requires explicit `persist_config: true`.
- Added v0.9 E2E coverage for zero-config folder publish, project memory after local rename, source ambiguity, interactive Simple Mode, help-surface split, and explicit MCP config persistence.

## v0.8.0 — Folder or ZIP, Same Safe Workflow

- Added first-class `source.folder` alongside the existing `source.zip` mode; exactly one source mode is required.
- Added direct folder publishing with `gitmake .` and project-folder inference for normal source trees.
- Folder mode builds a deterministic temporary snapshot and reuses the same GitMake security/identity/managed-sync/plan/apply pipeline instead of committing the live working tree.
- Added root/nested `.gitignore` support plus root `.gitmakeignore` for publish-only exclusions.
- Hard-excluded `.git/`, `.gitmake/`, `gitmake.json`, `.env`, common dependency/cache directories, and platform junk from folder snapshots.
- Added folder-source safety checks for symlinks, unsupported entries, unsafe paths, and case-colliding paths.
- Added deterministic folder hashing: ignored-file changes do not stale a plan, included-file changes do.
- Plans and machine output now expose `source_mode` together with `source_path` and source hash.
- Expanded `gitmake_prepare` MCP to prefer one high-level folder-or-ZIP preparation workflow.
- Added v0.8 folder E2E coverage and reran all prior regression suites.

## v0.7.3 — High-level MCP Prepare

- Added `gitmake_prepare`, a high-level MCP tool that turns a ZIP-only project into a reviewed GitMake plan in one call.
- `gitmake_prepare` owns source discovery, config inference/validation, security and GitHub preflight, project-identity checks, managed-sync planning, risk classification, and immutable plan creation.
- Default read-only MCP mode keeps a missing config in memory and performs no project/GitHub mutation.
- Write-enabled MCP mode may persist a missing config only through GitMake's validated atomic config writer; it still stops before apply.
- Added explicit MCP guidance: when `gitmake_prepare` or `gitmake_config_write` exists, agents must not use host filesystem Write/Edit tools to author `gitmake.json`.
- Added structured `gitmake.prepare/v1` output with config persistence state, reviewed plan, access state, and next-action guidance.
- Added focused E2E coverage for read-only ZIP-only preparation and write-enabled atomic config persistence.
- Re-ran base, v0.3, v0.4, v0.5, v0.5.1, v0.5.2, v0.6, v0.6.1, v0.7, v0.7.2, and v0.7.3 regression suites.

## v0.7.2 — Project Identity + Destructive Change Gate

- Added protected repository identity metadata in `.gitmake/project.json` and validation before updates.
- Added `PROJECT_IDENTITY_MISMATCH` hard-stop behavior for conflicting repository bindings.
- Stopped automatic retargeting of a real repository config when its configured ZIP is missing and an unrelated lone ZIP is present (`PROJECT_SOURCE_MISMATCH`). Starter placeholder configs still self-heal.
- Plans now surface working directory, config path, source path, target repository, remote visibility, project identity, change counts, deletion ratio, and risk classification.
- Added destructive-change classification when at least 10 managed files and at least 30% of the prior managed baseline would be deleted.
- Direct publish and ordinary apply are blocked for destructive plans. Local apply requires `--destructive`.
- MCP destructive apply requires a separately human-minted `gitmake approve <plan_id> --destructive` one-shot token; normal approval tokens are rejected.
- Added explicit config-vs-remote visibility mismatch reporting without changing existing repository visibility.
- Added ZIP-only MCP E2E coverage: inspect → config suggest → config write → plan with no hand-authored config.
- Added v0.7.3 safety E2E coverage for stale-source retarget refusal, destructive plan gating, destructive token replay rejection, identity tamper detection, and plan provenance.

## v0.7.1

- Fix self-publish false positives caused by secret-scanner unit-test fixtures containing literal private-key/token signatures.
- Add a regression test so scanner test fixtures cannot silently reintroduce signatures that block GitMake source publication.

# Changelog

## v0.7.1 — Safety Gate + Cross-Agent Hardening

- Added **managed sync** with `.gitmake/managed.json`: GitMake preserves remote-only files and deletes only files it previously managed; `.github/**` and `.gitmake/**` are protected by default.
- Added security preflight for likely secret files/content, large direct-Git files, and Git LFS requirements.
- Added `sync` and `security` configuration sections while keeping config schema version 1 compatible.
- Added `gitmake approve <plan_id>` and one-shot expiring approval tokens. MCP `gitmake_apply` now requires a human-generated approval token in addition to the reviewed plan ID.
- Added `gitmake inspect` / MCP project inspection and in-memory config suggestion so agents can remain inside the GitMake tool surface.
- Hardened multi-ZIP selection: content evidence is weighted over names, obvious binary archives are not auto-selected as source, and close candidates require input.
- Added GitHub preflight for required-PR branch protection, pre-existing bare release tags, stale remote state, and large-file/LFS constraints.
- Added generic MCP registration output with `gitmake ai setup --client generic --json`.
- Added Linux/macOS per-user installation to `~/.local/bin` and PATH management.
- Preserved all existing safety invariants: no force push, history rewrite, or repository deletion.

## v0.6.1 — One-click AI Setup

- Added `gitmake ai setup` to detect Claude Code and register GitMake MCP automatically at user scope.
- Read-only MCP access remains the default.
- Added `gitmake ai setup --write` for explicitly enabling reviewed config/apply tools, with confirmation unless `--yes` is supplied.
- Added `gitmake ai status [--json]` for Claude detection, MCP registration, scope, access level, command path, and connection health.
- Added idempotent `gitmake ai remove [--json]` for the GitMake-managed user-scope Claude MCP registration.
- `gitmake ai setup` repairs/installs the stable per-user GitMake binary on Windows before registering MCP, avoiding temporary/portable executable paths.
- `GitMake-Setup.exe` now automatically connects Claude Code in read-only mode when Claude is detected; setup remains successful when Claude is absent.
- Existing matching MCP registration is preserved instead of rewritten; permission changes safely replace only the user-scoped GitMake registration.
- GitMake refuses to overwrite/remove a same-named MCP server in project/local scope.
- Added dedicated AI setup unit tests and end-to-end coverage for setup, idempotency, read→write transition, status, and removal.

## v0.6.0

- Added local MCP stdio server via `gitmake mcp`.
- Default MCP toolset is read-only; mutating tools require explicit `--allow-write`.
- Added MCP tools for capability discovery, doctor, multi-ZIP discovery, config schema/validation, read-only preview, reviewed plan creation, and history.
- Added guarded MCP config write/patch and plan apply tools when write mode is explicitly enabled.
- MCP config authoring routes through GitMake's own schema validation and atomic writer rather than direct file editing.
- No direct MCP force-push, repository deletion, history rewrite, or unreviewed publish tool.
- Added `GITMAKE_PROJECT_DIR` and Claude Code `CLAUDE_PROJECT_DIR` project-root fallback.
- Added compatibility for modern direct `tools/list` clients and legacy MCP `initialize` clients.
- Strengthened AI guidance to prefer `gitmake config write --stdin --json` over direct `gitmake.json` editing.
- Added MCP unit/E2E coverage and kept all previous regression suites.

## v0.5.2 — Agent Hardening + LLM Config Authoring

### Added

- `gitmake config schema --json` authoritative local JSON Schema for `gitmake.json`.
- `gitmake config validate --json` strict config validation and normalized interpretation.
- `gitmake config write --stdin` for validated full-config writes from an LLM or automation.
- `gitmake config patch --stdin` with recursive JSON object merge and validation before replacement.
- `--dry-run` support for config writes/patches; `--read-only` blocks both mutation commands.
- `gitmake plan --json` reviewed plans containing source/config/release hashes and remote baseline.
- `gitmake apply <plan_id>` stale-plan revalidation before any mutation.
- `gitmake history [--json]` operation audit records.
- stable high-level machine error codes and suggested recovery actions.
- source-selection confidence score + evidence in multi-ZIP discovery.
- `release.on_existing="resume"` to upload only missing assets to an existing release.
- SHA-256 verification of downloaded self-upgrade packages using the release checksum asset.
- flags can now appear after positional arguments, e.g. `gitmake apply <id> --json`.

### Safety

- LLMs can discover the config schema locally instead of guessing fields.
- unknown config fields remain rejected.
- config write/patch validates before guarded file replacement.
- plan/apply binds approval to the reviewed ZIP/config/remote/release state.
- tampered or changed inputs fail with `PLAN_STALE`.
- self-upgrade refuses a package whose SHA-256 does not match the published checksum.

### Compatibility

- existing `gitmake.ai/v1`, `gitmake.result/v1`, and `gitmake.discovery/v1` consumers remain compatible.
- existing v0.5.1 configless planning and multi-ZIP behavior is retained.
- human `gitmake` one-command workflow is unchanged.
