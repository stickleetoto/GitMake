# GitMake v1.3.0

GitMake turns a project **folder or ZIP snapshot** into a GitHub repository and optional GitHub Release with one command. It deliberately owns a **small publishing workflow**, not all of GitHub.

```text
project folder OR ZIP
   ↓
snapshot/discover + security preflight
   ↓
reviewed plan
   ↓
create/update repository
   ↓
commit + normal push
   ↓
optional Release + assets
```

v1.2.0 adds **One-shot Publish Orchestration** on top of the frozen v1 workflow. In elicitation-capable MCP clients, the new `gitmake_publish` tool performs prepare → reviewed plan → human approval → exact-plan revalidation → apply → final result as one interactive MCP operation. Agents no longer need to stop the chat between `gitmake_prepare` and `gitmake_apply`. Existing `gitmake_prepare`, `gitmake_apply`, terminal `gitmake approve`, schemas, safety gates, and approval semantics remain backward compatible.

v1.2.1 hardened protocol routing and approval-state validation. v1.2.2 hardened self-upgrade by using public GitHub Releases over anonymous HTTPS, preserving SHA-256 verification, and refusing accidental downgrades. v1.2.3 introduced Windows locked-executable recovery. v1.2.4 closed the MCP auto-respawn race during Windows replacement. v1.2.5 is a real-world workflow hardening patch: root `--stdin` configuration is now authoritative and fail-closed, security scan output aggregates every supported finding, MCP tool failures preserve the structured CLI error payload, and `gitmake preview` now returns actionable guidance instead of being mistaken for a source path.

v1.2.6 fixes self-upgrade for real. The deferred replacement helper introduced in v1.2.3 was launched with `DETACHED_PROCESS`, which makes `powershell.exe` exit immediately without running the script, so **every staged replacement from v1.2.3 through v1.2.5 was a silent no-op that still reported success**. Replacement is now performed in process with a rename-aside sequence that Windows permits on a running image, is verified before the command returns, never deletes the current executable before the new one is in place, and no longer needs to stop any process. See [Self-upgrade integrity](#self-upgrade-integrity).

v1.2.7 fixes the reasons that defect went unnoticed for three releases: GitMake now has continuous integration across Linux, Windows, and macOS; machine error codes are carried on the error rather than recovered from message text; the two safety-critical packages that had no tests are covered; and the secret scanner recognises the credentials AI-authored projects actually leak. `doctor`, `install`, and `upgrade` gained `--json`, and `gitmake upgrade --check` reports an available update without installing it.

v1.2.9 makes the security gate roughly a hundred times faster on a typical source tree, without changing a single verdict: a tree of two thousand files went from 17.3 seconds to 175 ms. The scan is held to producing byte-identical reports to v1.2.8. v1.2.8 took the publish pipeline apart -- its five stages, discover, plan, snapshot, report and apply, are separate and individually tested, and the confirmation rules GitMake promises are pinned by tests: `--yes` accepts low-risk plans only, a medium-risk plan requires a typed `PUBLISH`, and a destructive one requires a plan-bound phrase. Publishing behaviour is unchanged.

v1.3.0 adds a way back and a way to be sure. `gitmake undo` reverts the last publish by adding a commit -- never by removing one -- and refuses when the branch has moved on or when the publish created the repository. Every publish now ends by asking GitHub what it actually holds instead of trusting the commands that just ran: the remote branch must point at the commit that was pushed, and a new release must carry every asset at the size it was uploaded with.

For the v1 compatibility promise, see [`STABILITY.md`](STABILITY.md).

## Install

### Windows

Extract the Windows package and double-click `GitMake-Setup.exe`, or:

```powershell
.\gitmake.exe install
```

GitMake installs per-user to `%LOCALAPPDATA%\Programs\GitMake` and adds that directory to the user PATH.

### Linux / macOS

Extract the platform package and run:

```bash
./gitmake install
```

GitMake installs to `~/.local/bin/gitmake` and idempotently manages the PATH snippet in the appropriate user shell profile.

Verify:

```text
gitmake --version
gitmake doctor
```

Requirements for publishing: Git, GitHub CLI `gh`, `gh auth login`, and a configured Git identity.

## Daily workflow

For normal human use:

```text
gitmake                         review + publish the current project
gitmake Project.zip             review + publish a specific ZIP
gitmake undo                    revert the last publish
gitmake upgrade                 update GitMake
```

### Undoing a publish

```text
gitmake undo --dry-run          show what would be reverted, change nothing
gitmake undo                    revert it
```

Undo adds a commit that restores the previous content. It does not reset, force-push, or delete, and it stops rather than guessing in three cases: the branch has moved on since the publish, the publish created the repository, or that publish was already undone. Releases and tags are left alone.

It also does not unpublish. The reverted contents stay reachable by SHA, through the GitHub API, and in any fork or CI log that already read them, so GitMake says so every time. **If a credential was published, undoing does not remove it -- rotate it.**

Interactive Simple Mode shows the target, source mode, change counts, risk, and release before asking once:

```text
GitMake 1.3.0

testuser/GambleLM
Update · public

Source     folder
Changes    +2 ~4 -0
Risk       low
Release    none
Plan       gm_...

Why
  ↳ Repository target was restored from .gitmake/project.json project memory.
  ↳ Existing repository visibility is preserved; GitMake does not silently change it during an update.

Publish update? [Y/n]:
```

Confirmation friction now follows the reviewed risk. `--yes` may accept **low-risk** Simple Mode plans only; it never bypasses medium/high-risk review. Medium-risk plans require an interactive `PUBLISH`, while high-risk/destructive plans require a plan-specific `DELETE-XXXXXX` confirmation. Existing expert `apply --destructive` and human-minted destructive MCP approval flows remain available.

### Guided trust and recovery

GitMake explains important automatic choices directly under the plan instead of asking the user to trust an opaque inference:

```text
Why
  ↳ Configuration was inferred in memory; no gitmake.json was required.
  ↳ Private visibility is the zero-config safety default for a new repository.
```

After a successful Simple Mode publish, the noisy internal pipeline is collapsed unless `--verbose` is requested:

```text
✓ Published GambleLM

Repository  testuser/GambleLM
Branch      main
Changes     +2 ~4 -0
Release     none
Time        5.8s

https://github.com/testuser/GambleLM
```

When GitMake blocks, it now follows the error with a `Recommended` section. Examples include choosing an explicit source for `SOURCE_AMBIGUOUS`, running `gh auth login` for missing GitHub auth, excluding files that should never be published after `SECRET_DETECTED`, and rebuilding a fresh plan after stale input/remote state. Safety-critical identity mismatches are never auto-fixed.

### Ephemeral config from stdin

For automation that needs a one-run config without writing `gitmake.json`, `--stdin` now accepts a **complete GitMake config** for publish/preview:

```bash
printf '%s' '{"repo":{"name":"demo","visibility":"public"},"source":{"folder":"."},"git":{"branch":"develop"}}' \
  | gitmake --stdin --dry-run --read-only --json
```

The stdin config is authoritative for that invocation, is reported as `config.source: "stdin"`, and is never silently replaced by inferred defaults. Invalid/empty/trailing/unknown-field JSON fails closed. `gitmake plan --stdin` is intentionally rejected because an ephemeral config cannot be revalidated later during `apply`; persist it first with `gitmake config write --stdin` if you need the plan/apply workflow.

### Zero-config by default

A missing `gitmake.json` is inferred **in memory** and is not written as a side effect of publishing. Safe defaults remain: `main`, `private` for new repositories, managed sync, secret scan, and no release. Persist a config only when you actually need stable advanced settings:

```text
gitmake init .
gitmake init Project.zip
```

### Project memory

After a successful **folder** publish, GitMake stores the folder→repository binding in:

```text
.gitmake/project.json
```

That path is excluded from folder snapshots. If the local project folder is renamed, future zero-config runs still target the original repository. A conflicting binding is a hard `PROJECT_IDENTITY_MISMATCH` stop rather than an automatic retarget.

### Source ambiguity

When both the current folder and one or more ZIPs independently look like plausible project sources, GitMake does not guess. Interactive Simple Mode asks which one to use; machine/MCP callers receive `SOURCE_AMBIGUOUS` / `needs_input` and must provide an explicit source.

### Simple vs expert

The default help surface intentionally shows only everyday commands:

```text
gitmake help
```

Low-level config, plan/apply, diagnostics, MCP, and automation commands remain fully available behind:

```text
gitmake help --expert
```

## Folder Mode

For a live project, run GitMake directly inside the source tree:

```text
MyProject/
├─ README.md
├─ src/
├─ tests/
└─ pyproject.toml

> gitmake .
```

Folder Mode creates a deterministic temporary snapshot and then reuses the same publishing pipeline as ZIP Mode. It never commits the working tree directly. `gitmake.json`, `.git/`, `.gitmake/`, local dependency/cache directories, `.env`, and platform junk files are excluded by default.

GitMake honors common root and nested `.gitignore` rules, plus an optional root `.gitmakeignore` for publish-only exclusions:

```text
# .gitmakeignore
dataset/
checkpoints/
*.ckpt
private/**
```

Only included files contribute to the reviewed source hash. Changing an ignored cache file does not stale a plan; changing an included source file does. Symlinks and unsafe/case-colliding paths are rejected.

Folder config:

```json
{
  "source": {
    "folder": "."
  }
}
```

ZIP config remains supported unchanged:

```json
{
  "source": {
    "zip": "Demo_Source.zip",
    "strip_root": true
  }
}
```

Exactly one of `source.folder` or `source.zip` may be configured.


## One-call AI preparation

When GitMake MCP write access is available, agents should prefer **`gitmake_publish`** for normal requests such as “upload this”, “publish this project”, “create a GitHub repo”, or “update this repo”. `gitmake_publish` owns the full interactive flow in one tool operation:

```text
source/config inference
→ security + GitHub preflight
→ reviewed plan
→ client-controlled human approval
→ exact-plan revalidation
→ apply
→ final repository/release result
```

Use `gitmake_prepare` when the user explicitly asks for plan-only review. `gitmake_prepare` performs:

```text
source discovery
→ config inference + strict validation
→ security / GitHub preflight
→ project identity + managed-sync planning
→ immutable reviewed plan
→ stop for human approval
```

A missing `gitmake.json` stays in memory by default **even when MCP write access is enabled**. Persistence is explicit (`persist_config: true`) and goes through GitMake's validated atomic config writer. `gitmake_prepare` still never publishes; it stops at a reviewed plan. `gitmake_publish` uses the same zero-config preparation logic before requesting human approval.

Agents are explicitly instructed not to create or edit `gitmake.json` with host filesystem Write/Edit tools when `gitmake_publish`, `gitmake_prepare`, or `gitmake_config_write` is available.

## Managed sync: safe by default

The default `sync.mode` is `managed`.

On first adoption of an existing repository GitMake preserves files that exist only in the repository. It records only the source files it owns in `.gitmake/managed.json`. On later updates, a file is deleted only if GitMake previously managed it and the new source snapshot no longer contains it.

Default protected paths:

```text
.github/**
.gitmake/**
```

This prevents a source snapshot from accidentally erasing repository-only workflows/configuration. Exact legacy mirror behavior is still available with:

```json
{
  "sync": {
    "mode": "snapshot"
  }
}
```

Protected paths remain protected in snapshot mode.

## Project identity and destructive-change gate

GitMake now commits a protected repository identity record at `.gitmake/project.json`. On later updates it verifies that the cloned repository is still bound to the target owner/name before synchronization. A conflicting valid identity is a hard stop (`PROJECT_IDENTITY_MISMATCH`).

A stale real ZIP-based `gitmake.json` is also never auto-retargeted to a different lone ZIP beside it. Starter placeholder configs can still self-heal, but an existing repository config with a missing `source.zip` must be fixed explicitly (`PROJECT_SOURCE_MISMATCH`).

Reviewed plans always expose:

```text
working_directory
config_path
source_mode
source_path
repository
remote_visibility
project_identity
changes
risk
```

If at least 10 previously managed files and at least 30% of the managed baseline would be deleted, the plan is classified as destructive. GitMake never accepts that plan through `--yes` or an ordinary approval. Interactive Simple Mode requires the plan-specific `DELETE-XXXXXX` phrase; expert/MCP flows require an explicit destructive opt-in:

```text
gitmake apply <plan_id> --destructive
# or, for MCP:
gitmake approve --destructive
```

The destructive approval remains short-lived, plan-bound, single-use, and cannot be created through MCP.

## Security preflight

Before commit/push/release mutation, GitMake checks for:

- high-risk secret paths such as real `.env` files and private key files
- credential patterns for the services projects actually leak: private keys, GitHub tokens, AWS keys, Anthropic / OpenAI / Hugging Face model keys, Google API keys and OAuth client secrets, GCP service accounts, Stripe, SendGrid, npm, Azure storage keys, and Slack / Discord webhooks
- structural credentials: a password embedded in a connection string, or a JWT
- oversized direct-Git files
- Git LFS markings and `git lfs` availability
- required-PR branch protection
- pre-existing bare tags that would make a release ambiguous
- stale remote state when applying a reviewed plan

Every finding blocks the publish; scanning is fail-closed. Each one reports the file, the kind, the **line**, and a confidence:

```text
× potential secrets detected; publish blocked: src/client.py (anthropic_api_key), .env (secret_file)
```

`high` means an issuer-specific shape that essentially cannot occur by accident. `medium` means a structural match — a password inside a connection string, for example — worth reading before you decide. Documentation placeholders such as `postgres://user:password@localhost/db` are not reported.

File contents are scanned in full rather than up to a size cutoff, so a credential buried in a large log or dump is still found. All findings are reported together: fixing one no longer reveals the next on the following run.

Example configuration:

```json
{
  "security": {
    "secret_scan": true,
    "allow_secret_paths": [],
    "warn_file_bytes": 52428800,
    "max_git_file_bytes": 99614720
  }
}
```

GitMake never force-pushes to escape a race or branch policy.

## Multi-ZIP discovery

```text
gitmake discover --json
```

Selection priority:

```text
explicit ZIP argument
→ gitmake.json
→ one confidently classified source ZIP
→ otherwise needs_input
```

GitMake uses ZIP contents as primary evidence and names as supporting evidence. An obvious binary/release archive is not silently selected as project source. Close or competing source candidates require input.

## Configuration for humans and LLMs

Never guess the config shape:

```text
gitmake config schema --json
```

Validate:

```text
gitmake config validate --json
```

Agents can safely author through GitMake itself:

```text
gitmake config write --stdin --json
gitmake config patch --stdin --json
```

Unknown fields are rejected and the complete normalized config is validated before replacement. `--dry-run` previews writes; `--read-only` blocks them.

A typical ZIP config:

```json
{
  "schema_version": 1,
  "repo": {
    "name": "Demo",
    "visibility": "private"
  },
  "source": {
    "zip": "Demo_Source.zip",
    "strip_root": true
  },
  "git": {
    "branch": "main"
  },
  "sync": {
    "mode": "managed",
    "protected_paths": [".github/**", ".gitmake/**"]
  },
  "security": {
    "secret_scan": true
  },
  "release": {
    "enabled": false
  }
}
```

## Plan → Apply

Create a reviewed immutable plan:

```text
gitmake plan --json
```

A plan binds working-directory/config/source provenance, project identity, configured and remote visibility, source/config digests, target repository, remote baseline, exact user-visible change counts, deletion risk, release inputs, and a fingerprint.

For a local human-controlled CLI apply:

```text
gitmake apply gm_0123456789abcdef --json
```

If source/config/remote/release state changed since review, apply fails with `PLAN_STALE` rather than publishing a different state.

## Human approval for AI/MCP apply

MCP write access is never enough by itself to publish a reviewed plan. GitMake requires a human approval bound to the exact reviewed plan.

### Chat approval (preferred when supported)

On MCP clients with **elicitation** support, `gitmake_apply` asks the human inside the client UI before any GitHub mutation. Claude Code supports MCP elicitation dialogs.

Normal flow:

```text
AI: gitmake_prepare
    ↓
reviewed plan shown to user
    ↓
AI: gitmake_apply(plan_id)
    ↓
GitMake → client-controlled approval dialog
    ↓
human Accept / Decline
    ↓
GitMake revalidates exact plan
    ↓
publish
```

Low-risk plans can be accepted directly in the dialog. Medium-risk plans require `PUBLISH`. High-risk/destructive plans require the plan-specific `DELETE-XXXXXX` phrase. The model must never answer the elicitation on the user's behalf.

The client response creates the same short-lived, single-use local grant used by terminal approval. A consumed grant cannot be re-minted for the same reviewed plan; another mutation requires a fresh plan.

### Terminal fallback

Clients without elicitation support keep the stable v1 flow:

```text
gitmake approve
```

An explicit plan ID is still supported:

```text
gitmake approve gm_0123456789abcdef
```

Destructive fallback approval requires:

```text
gitmake approve --destructive
```

Approval is bound to the plan fingerprint, source/config hashes, and target repository; expires after 10 minutes; and is consumed only after successful apply. Pre-1.0 approval tokens remain accepted only as a deprecated compatibility path.

**Trust note:** MCP chat approval relies on the connected client to surface elicitation to the human. If a client is configured with hooks that auto-answer elicitation, that automation becomes part of the trust boundary. Do not auto-approve GitMake elicitation when manual approval is required.

## AI discovery

```text
gitmake ai describe --json
gitmake ai install
```

The manifest tells agents to use GitMake's authoritative config schema/write tools, run security-aware preview/plan first, stop on ambiguity/security/branch/tag errors, and never manufacture or bypass human approvals.

## Claude Code one-click MCP

```text
gitmake ai setup
gitmake ai status
```

Claude Code is detected and the user-scoped stdio MCP registration is created read-only by default.

To expose guarded config write/patch and approved-plan apply:

```text
gitmake ai setup --write
```

With Claude Code or another elicitation-capable client, `gitmake_publish` can perform the whole prepare/approve/apply lifecycle without ending the assistant turn to ask for approval manually. `gitmake_apply` continues to support direct chat approval for expert flows. If elicitation is unavailable, use `gitmake_prepare` → terminal `gitmake approve` → `gitmake_apply`. No approval token copy/paste is needed.

## Generic MCP clients

GitMake does not guess unknown client config formats. Ask it for a portable stdio descriptor:

```text
gitmake ai setup --client generic --json
```

Then register that descriptor using the client's normal MCP configuration mechanism.

Raw server:

```text
gitmake mcp
gitmake mcp --allow-write
```

Read-only MCP tools include project inspection, describe, doctor, discovery, config schema/validation/suggestion, preview, plan, and history. Guarded write mode adds `gitmake_publish`, config write/patch, and reviewed apply. `gitmake_publish` is the primary normal publishing tool; it still creates a reviewed immutable plan and still requires client-controlled human approval before mutation. There is no MCP force-push, repo-delete, history-rewrite, or unreviewed direct-publish tool.

Project-root resolution supports an explicit tool `project_dir`, `GITMAKE_PROJECT_DIR`, Claude's `CLAUDE_PROJECT_DIR`, then current working directory.

## Release recovery

```json
{
  "release": {
    "enabled": true,
    "tag": "v1.0.0",
    "assets": ["App_Windows.zip", "App_Source.zip"],
    "on_existing": "resume"
  }
}
```

When a release already exists, `resume` uploads only missing configured assets. A bare pre-existing tag without the reviewed release is treated as a conflict rather than silently reused.

## History

```text
gitmake history
gitmake history --json
```

Recent publish/apply audit records include success/failure, repository, mode, change counts, plan ID, release tag, and dry-run/read-only state.

## Self-upgrade integrity

```text
gitmake upgrade
```

> **Coming from v1.2.5 or earlier?** `gitmake upgrade` cannot get you off those builds. Staged replacement never ran in v1.2.3–v1.2.5 — that is the bug v1.2.6 fixed — so they cannot replace themselves. Extract the platform package and run `.\gitmake.exe install` (Windows) or `./gitmake install` (Linux/macOS) once, then check `gitmake --version`. From v1.2.6 onward `gitmake upgrade` works normally.

Upgrade discovery and downloads use the public GitHub Release API over HTTPS and do not require GitHub CLI authentication. Release packages are checked against the matching SHA-256 asset before anything on disk is touched. GitMake also refuses to replace a newer local build with an older published release.

Since v1.2.6 the replacement itself happens **in process, before the command returns**:

```text
✓ Downloaded v1.2.6
✓ SHA-256 verified
✓ Installed v1.2.6
  C:\Users\you\AppData\Local\Programs\GitMake\gitmake.exe

Run: gitmake --version
```

Windows refuses to delete or overwrite the file backing a running image, but it does allow renaming it. GitMake therefore renames the current executable aside, moves the verified new one into the canonical path, and only then removes the displaced file. Nothing is deleted before the replacement is in place, and no process has to be stopped for the upgrade to succeed — including MCP stdio servers still running the old build, which keep running from the renamed file until they exit.

`gitmake upgrade` replaces the executable you actually invoked. If that is not the installed copy on PATH, the result says so rather than letting you assume the PATH command was updated.

In the rare case where the in-process replacement is refused, GitMake falls back to a deferred helper and reports that honestly:

```text
· Replacement scheduled after this process exits
  C:\Users\you\AppData\Local\Programs\GitMake\gitmake.exe
  Log: C:\Users\you\AppData\Local\Temp\gitmake-replace-1234.log

  Verify once this command has closed:
  "C:\Users\you\AppData\Local\Programs\GitMake\gitmake.exe" --version
```

GitMake never prints `Installed` for a replacement that has not happened.

## Machine-readable errors

`--json` failures use stable high-level codes. v0.7 includes codes for security and GitHub safety boundaries such as:

```text
SOURCE_NOT_FOUND
SOURCE_AMBIGUOUS
CONFIG_INVALID
SECRET_DETECTED
LARGE_FILE_BLOCKED
GIT_LFS_REQUIRED
BRANCH_REQUIRES_PR
TAG_CONFLICT
REMOTE_MOVED
PLAN_NOT_FOUND
PLAN_STALE
APPROVAL_REQUIRED
RELEASE_EXISTS
UPGRADE_INTEGRITY_FAILED
```

## CLI

Simple surface:

```text
gitmake                         Review + publish current project
gitmake Project.zip             Review + publish explicit ZIP
gitmake upgrade                 Upgrade GitMake
gitmake help                    Everyday help
gitmake --version               Version
```

Expert surface (`gitmake help --expert`) keeps the full interface:

```text
gitmake init [source]           Persist optional gitmake.json
gitmake doctor                  Diagnose environment/install state
gitmake inspect                 Inspect project/config state
gitmake discover                Classify ZIP candidates
gitmake plan [source]           Store reviewed plan
gitmake approve                 Approve latest reviewed plan locally for one MCP apply
gitmake apply <plan_id>         Revalidate + locally apply plan
gitmake history                 Read audit history

gitmake config schema
gitmake config validate
gitmake config write --stdin
gitmake config patch --stdin

gitmake ai describe
gitmake ai install
gitmake ai setup
gitmake ai setup --write
gitmake ai setup --client generic --json
gitmake ai status
gitmake ai remove
gitmake mcp
gitmake mcp --allow-write
```

## Safety invariants

- no force push
- no Git history rewrite
- no repository deletion
- managed ownership prevents arbitrary remote-only deletion
- `.github/**` and `.gitmake/**` protected by default
- ZIP traversal / `.git` injection / symlink / invalid-path defenses
- secret + large-file/LFS preflight before mutation
- required-PR branch policy is not bypassed
- bare tag conflicts are not silently reused
- ambiguous folder/ZIP source candidates are not guessed
- agent-authored config is schema/semantic validated
- reviewed plans reject stale inputs/remote state
- MCP apply requires a local human one-shot approval (`gitmake approve`); no token copy/paste
- self-upgrade verifies SHA-256

## Why GitMake exists

GitMake is intentionally smaller than GitHub CLI, GitHub MCP, or general release frameworks. Its job is to make one repetitive workflow—**turning a project snapshot into a safely reviewed repository update and release**—simple enough for both humans and agents to use consistently.
