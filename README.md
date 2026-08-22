# GitMake v0.10.0

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

v0.10.0 adds **Guided UX + Trust & Recovery** on top of the v0.9 zero-config/simple-mode foundation. GitMake now adapts confirmation friction to reviewed risk, explains the automatic decisions behind a plan, collapses successful publishes into a compact result card, gives actionable recovery guidance when it blocks, and turns the Windows installer into a readiness/setup experience. `gitmake.json` remains optional advanced configuration.

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
gitmake upgrade                 update GitMake
```

Interactive Simple Mode shows the target, source mode, change counts, risk, and release before asking once:

```text
GitMake 0.10.0

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

When GitMake MCP is available, agents should prefer the high-level `gitmake_prepare` tool for folder, ZIP, or unconfigured projects. It performs:

```text
source discovery
→ config inference + strict validation
→ security / GitHub preflight
→ project identity + managed-sync planning
→ immutable reviewed plan
→ stop for human approval
```

A missing `gitmake.json` stays in memory by default **even when MCP write access is enabled**. Persistence is explicit (`persist_config: true`) and goes through GitMake's validated atomic config writer. `gitmake_prepare` still never publishes; it stops at a reviewed plan.

Agents are explicitly instructed not to create or edit `gitmake.json` with host filesystem Write/Edit tools when `gitmake_prepare` or `gitmake_config_write` is available.

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
gitmake approve <plan_id> --destructive
```

The destructive approval token is still short-lived, plan-bound, single-use, and cannot be minted through MCP.

## Security preflight

Before commit/push/release mutation, GitMake checks for:

- high-risk secret paths such as real `.env` files and private key files
- common private-key, GitHub-token, and AWS credential patterns
- oversized direct-Git files
- Git LFS markings and `git lfs` availability
- required-PR branch protection
- pre-existing bare tags that would make a release ambiguous
- stale remote state when applying a reviewed plan

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

## One-shot approval for AI/MCP apply

MCP write access is no longer enough by itself to publish a reviewed plan. A human must create a short-lived, plan-bound token from an interactive terminal:

```text
gitmake approve gm_0123456789abcdef
```

The token:

- cannot be minted through MCP
- is bound to one plan
- expires
- is consumed after successful apply
- rejects replay

The agent sends both `plan_id` and `approval_token` to `gitmake_apply`. If the plan is destructive, a normal token is rejected; only a human-created `--destructive` token can authorize it.

## AI discovery

```text
gitmake ai describe --json
gitmake ai install
```

The manifest tells agents to use GitMake's authoritative config schema/write tools, run security-aware preview/plan first, stop on ambiguity/security/branch/tag errors, and never manufacture approval tokens.

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

Actual apply still requires the one-shot human approval token above.

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

Read-only MCP tools include project inspection, describe, doctor, discovery, config schema/validation/suggestion, preview, plan, and history. Guarded write mode adds config write/patch and approved apply. There is no MCP force-push, repo-delete, history-rewrite, approval-minting, or unreviewed direct-publish tool.

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

Release packages are checked against the matching SHA-256 asset before replacement is staged.

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
gitmake approve <plan_id>       Human-mint one-shot MCP approval
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
- MCP apply requires a human-minted one-shot approval token
- self-upgrade verifies SHA-256

## Why GitMake exists

GitMake is intentionally smaller than GitHub CLI, GitHub MCP, or general release frameworks. Its job is to make one repetitive workflow—**turning a project snapshot into a safely reviewed repository update and release**—simple enough for both humans and agents to use consistently.
