# GitMake v1.3.0

**Safe GitHub publishing for humans and AI agents.**

GitMake turns a project **folder or ZIP snapshot** into a reviewed GitHub repository update — and optionally a GitHub Release — with one command.

It deliberately owns one small workflow well: **inspect → review → publish → verify**.

```text
project folder / ZIP
        ↓
 discover + snapshot
        ↓
 security preflight
        ↓
  reviewed plan
        ↓
 human approval when needed
        ↓
 commit + normal push
        ↓
 optional GitHub Release
        ↓
 remote verification
```

## Why GitMake

| Capability | What it means |
|---|---|
| **Zero-config workflow** | Safe defaults are inferred in memory; a config file is optional. |
| **Security preflight** | Secrets, unsafe paths, large-file/LFS issues, branch policy conflicts and stale state can block publishing before mutation. |
| **Risk-aware approval** | Low-risk work stays simple; medium and destructive plans require stronger confirmation. |
| **Agent-safe MCP** | AI agents can prepare and publish through a compact MCP workflow without gaining unreviewed destructive Git access. |
| **Undo without history rewrite** | `gitmake undo` restores the previous content with a new commit — no reset or force-push. |
| **Post-publish verification** | GitMake asks GitHub what actually landed instead of trusting only the local command result. |

### v1.3.0 highlights

- `gitmake undo` for safe publish recovery
- post-publish remote branch verification
- release asset size verification
- one-call `gitmake_publish` orchestration for elicitation-capable MCP clients
- continuous integration across Linux, Windows and macOS
- security scanning optimized from seconds to milliseconds on typical source trees while preserving verdicts

For the v1 compatibility promise, see [`STABILITY.md`](STABILITY.md).

## Quick start

```text
gitmake                         review + publish the current project
gitmake Project.zip             review + publish a specific ZIP
gitmake undo                    revert the last publish
gitmake upgrade                 update GitMake
```

GitMake keeps the default human-facing surface intentionally small. Advanced configuration, planning, diagnostics, MCP and automation commands remain available through:

```text
gitmake help --expert
```

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

Publishing requires Git, GitHub CLI `gh`, `gh auth login`, and a configured Git identity.

## Daily workflow

Run GitMake inside a project folder:

```text
MyProject/
├─ README.md
├─ src/
├─ tests/
└─ pyproject.toml

> gitmake
```

Or publish an explicit ZIP snapshot:

```text
gitmake Project.zip
```

Interactive Simple Mode shows the target, source mode, change counts, risk and release state before publishing:

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
  ↳ Existing repository visibility is preserved.

Publish update? [Y/n]:
```

A successful publish collapses the internal pipeline into a compact result unless `--verbose` is requested:

```text
✓ Published GambleLM

Repository  testuser/GambleLM
Branch      main
Changes     +2 ~4 -0
Release     none
Time        5.8s
```

When GitMake blocks, it follows the error with an actionable `Recommended` section rather than silently guessing a fix.

## Safety model

GitMake is designed around a few hard boundaries:

- no force push
- no Git history rewrite
- no repository deletion
- no unreviewed destructive MCP publish
- `.github/**` and `.gitmake/**` protected by default
- ambiguous folder/ZIP source candidates are not guessed
- ZIP traversal, `.git` injection, symlink and invalid-path defenses
- secret and large-file/LFS preflight before mutation
- required-PR branch policy is not bypassed
- bare tag conflicts are not silently reused
- reviewed plans reject stale source, config and remote state
- self-upgrade verifies SHA-256 before replacement

### Risk-aware confirmation

Confirmation friction follows the reviewed plan risk:

- **low risk** — normal confirmation; `--yes` may accept Simple Mode plans
- **medium risk** — requires typed `PUBLISH`
- **destructive/high risk** — requires a plan-specific `DELETE-XXXXXX` phrase or explicit expert destructive approval

GitMake never turns a normal `--yes` into permission for destructive work.

## Undo

```text
gitmake undo --dry-run
gitmake undo
```

Undo adds a new commit that restores the content from before the previous GitMake publish. It does **not** reset, force-push or delete history.

GitMake stops instead of guessing when the branch moved after the publish, when that publish created the repository, or when the publish was already undone.

Releases and tags are left alone. Undo also does not erase already-published credentials from Git history, forks, API caches or CI logs — rotate leaked credentials instead.

## Zero-config by default

A missing `gitmake.json` is inferred **in memory** and is not written as a side effect of publishing.

Safe defaults include:

- `main`
- `private` for a new repository
- managed sync
- secret scanning
- no release

Persist configuration only when you actually need stable advanced settings:

```text
gitmake init .
gitmake init Project.zip
```

For automation, `--stdin` accepts a complete one-run config without writing `gitmake.json`:

```bash
printf '%s' '{"repo":{"name":"demo","visibility":"public"},"source":{"folder":"."},"git":{"branch":"develop"}}' \
  | gitmake --stdin --dry-run --read-only --json
```

The stdin config is authoritative for that invocation and invalid, empty, trailing or unknown-field JSON fails closed.

## Project memory

After a successful folder publish, GitMake stores the folder→repository binding in:

```text
.gitmake/project.json
```

That path is excluded from source snapshots. Renaming the local folder does not silently retarget the repository, and a conflicting identity stops with `PROJECT_IDENTITY_MISMATCH`.

## Managed sync

The default `sync.mode` is `managed`.

On first adoption of an existing repository, GitMake preserves files that exist only on the remote side. It records the source files it owns in `.gitmake/managed.json`; later, a file is deleted only if GitMake previously managed it and the new source snapshot no longer contains it.

Protected by default:

```text
.github/**
.gitmake/**
```

Exact legacy mirror behavior remains available with `sync.mode: "snapshot"`, while protected paths stay protected.

## Folder and ZIP handling

Folder Mode creates a deterministic temporary snapshot and reuses the same publishing pipeline as ZIP Mode. It never commits the working tree directly.

Default exclusions include `.git/`, `.gitmake/`, local dependency/cache directories, `.env`, platform junk and GitMake's own config metadata where appropriate.

GitMake honors root and nested `.gitignore` rules plus an optional root `.gitmakeignore`:

```text
# .gitmakeignore
dataset/
checkpoints/
*.ckpt
private/**
```

Only included files contribute to the reviewed source hash. Symlinks and unsafe or case-colliding paths are rejected.

When several ZIPs are present, GitMake uses content as primary evidence and names as supporting evidence. Close candidates become `needs_input` rather than an arbitrary selection.

## Security preflight

Before commit, push or release mutation, GitMake checks for issues including:

- real `.env` files and private-key paths
- GitHub, AWS, Anthropic, OpenAI, Hugging Face, Google/GCP, Stripe, SendGrid, npm, Azure, Slack and Discord credential patterns
- structural credentials such as passwords embedded in connection strings and JWTs
- oversized direct-Git files
- Git LFS markings and `git lfs` availability
- required-PR branch protection
- pre-existing bare tags that would make a release ambiguous
- stale remote state

Findings fail closed and report the file, kind, line and confidence. File contents are scanned in full, and all supported findings are aggregated in one pass.

GitMake never force-pushes to escape a race or branch policy.

## Plan → Apply

Expert and automation workflows can create a reviewed immutable plan:

```text
gitmake plan --json
```

A plan binds source/config provenance, project identity, visibility, digests, target repository, remote baseline, visible change counts, deletion risk, release inputs and a fingerprint.

Apply it with:

```text
gitmake apply gm_0123456789abcdef --json
```

If the source, config, remote or release state changed after review, apply fails with `PLAN_STALE` instead of publishing a different state.

## AI / MCP

GitMake can expose the publishing workflow to LLM agents without exposing unrestricted GitHub mutation.

For normal MCP publishing, agents should prefer **`gitmake_publish`**:

```text
source/config inference
        ↓
security + GitHub preflight
        ↓
reviewed immutable plan
        ↓
client-controlled human approval
        ↓
exact-plan revalidation
        ↓
publish
        ↓
remote verification
```

Use `gitmake_prepare` when plan-only review is desired.

On clients with MCP elicitation support, GitMake can request human approval inside the client UI. The model must never answer that approval on the user's behalf.

Clients without elicitation can use the terminal fallback:

```text
gitmake approve
gitmake approve gm_0123456789abcdef
gitmake approve --destructive
```

Approvals are short-lived, plan-bound and single-use.

### Claude Code setup

```text
gitmake ai setup
gitmake ai status
```

The registration is read-only by default. To expose guarded config write/patch and approved publishing:

```text
gitmake ai setup --write
```

### Generic MCP clients

```text
gitmake ai setup --client generic --json
```

Raw server modes:

```text
gitmake mcp
gitmake mcp --allow-write
```

There is no MCP force-push, repository-delete, history-rewrite or unreviewed direct-publish tool.

## Configuration for humans and agents

Inspect the authoritative schema instead of guessing it:

```text
gitmake config schema --json
gitmake config validate --json
```

Agents can author config through GitMake itself:

```text
gitmake config write --stdin --json
gitmake config patch --stdin --json
```

Unknown fields are rejected and the complete normalized config is validated before replacement. `--dry-run` previews writes and `--read-only` blocks them.

## Release recovery

A release configuration can use `on_existing: "resume"` so GitMake uploads only missing configured assets when the release already exists.

A bare pre-existing tag without the reviewed release is treated as a conflict rather than silently reused.

## History

```text
gitmake history
gitmake history --json
```

Audit records include success/failure, repository, mode, change counts, plan ID, release tag, and dry-run/read-only state.

## Self-upgrade integrity

```text
gitmake upgrade
```

Release discovery and downloads use the public GitHub Release API over HTTPS. Packages are checked against the matching SHA-256 asset before disk replacement, and GitMake refuses accidental downgrade to an older release.

> **Coming from v1.2.5 or earlier?** Those builds cannot self-upgrade because their staged Windows replacement path was defective. Install a newer platform package once; from v1.2.6 onward normal `gitmake upgrade` works.

On Windows, modern builds replace the running executable through a verified rename-aside sequence and never print `Installed` for a replacement that has not actually happened.

## Machine-readable errors

`--json` failures use stable high-level codes, including:

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

## CLI reference

Simple surface:

```text
gitmake                         Review + publish current project
gitmake Project.zip             Review + publish explicit ZIP
gitmake undo                    Revert the last GitMake publish
gitmake upgrade                 Upgrade GitMake
gitmake help                    Everyday help
gitmake --version               Version
```

Expert surface:

```text
gitmake init [source]
gitmake doctor
gitmake inspect
gitmake discover
gitmake plan [source]
gitmake approve
gitmake apply <plan_id>
gitmake history

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

## Why GitMake exists

GitMake is intentionally smaller than GitHub CLI, GitHub MCP or a general release framework.

Its job is to make one repetitive workflow — **turning a project snapshot into a safely reviewed repository update and release** — simple enough for both humans and agents to use consistently.
