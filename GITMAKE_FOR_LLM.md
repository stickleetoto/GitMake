# GitMake — LLM Usage Guide

> This file is for AI coding agents and LLMs working inside a project that uses GitMake.
> Read this file before creating/updating GitHub repositories, publishing project folders/ZIPs, or making releases.

## What GitMake is

GitMake is a high-level GitHub publishing workflow.

It accepts either:

- the current project folder, or
- a project ZIP

and turns it into a reviewed GitHub repository create/update and, optionally, a GitHub Release.

Preferred normal AI workflow:

```text
Project folder / ZIP
        ↓
gitmake_publish
        ↓
reviewed plan + MCP human approval
        ↓
exact-plan apply
        ↓
GitHub
```

GitMake owns the publishing workflow. Do not bypass it with raw `git`, `gh`, or direct GitHub API calls when GitMake can perform the requested operation.

---

## Core rule for LLMs

When the user says something like:

- "Upload this to GitHub"
- "Publish this project"
- "Make a new GitHub repo"
- "Update this repo"
- "Create a release"
- "Prepare this ZIP for GitHub"

**Use GitMake first.**

If GitMake MCP tools are available, prefer them over shell commands.

For a normal request to **actually publish/upload/create/update** a repository, use the highest-level tool first:

```text
gitmake_publish
```

`gitmake_publish` is the primary publishing orchestrator. It keeps prepare, reviewed plan creation, client-controlled human approval, exact-plan revalidation, apply, and the final result inside one interactive MCP operation. **Do not stop the chat merely to ask the user for approval** when this tool is available; GitMake requests approval through the MCP client UI.

Use `gitmake_prepare` when the user explicitly asks for a plan only, wants to inspect changes before deciding whether to publish, or when `gitmake_publish` reports that expert intervention/input is required.

---

## Preferred MCP workflow

### Normal publish request

Call:

```text
gitmake_publish
```

GitMake will:

1. infer folder vs ZIP and zero-config settings
2. run security/GitHub/project-identity preflight
3. create the reviewed immutable plan
4. open a client-controlled MCP approval prompt
5. require stronger typed confirmation for medium/destructive risk
6. revalidate the exact plan after approval
7. apply the repository/release mutation
8. return the final real result

Do not answer, auto-accept, or simulate the approval on the user's behalf.

If the user declines, report that nothing was published.

If the MCP client does not support elicitation, use the compatibility workflow:

```text
gitmake_prepare
→ show reviewed plan
→ user runs `gitmake approve`
→ gitmake_apply(plan_id)
```

### Failure payloads

When a GitMake MCP tool fails, read the returned `structuredContent` before falling back to shell/CLI diagnostics. v1.2.5 and later preserve the original GitMake machine error there, including fields such as `error.code`, `error.stage`, `error.recoverable`, `error.suggested_action`, and pipeline/security findings. Do not reduce a structured failure to a generic exit code.

### Plan-only request

When the user explicitly says “prepare only”, “show me the plan”, or “do not publish yet”, call:

```text
gitmake_prepare
```

and stop before mutation.

---

## Folder mode

GitMake supports publishing the current project folder directly.

Example:

```text
MyProject/
├─ README.md
├─ src/
├─ tests/
├─ pyproject.toml
└─ ...
```

The user can simply say:

> Upload this project to GitHub.

GitMake can snapshot the folder and route it through the same reviewed publishing pipeline used for ZIP files.

Do not create a ZIP manually unless GitMake specifically requires one.

GitMake may use:

```text
.gitignore
.gitmakeignore
```

to exclude files from folder snapshots.

---

## ZIP mode

GitMake also supports project ZIPs.

Example:

```text
Project_v1.0.0.zip
```

If only one plausible project ZIP exists, GitMake may select it automatically.

If multiple plausible sources exist, do not guess. Respect GitMake's ambiguity result and ask the user which source is intended.

For nested publish bundles, distinguish between:

- the actual source ZIP
- release assets
- documentation
- GitMake config

Never assume an outer bundle ZIP itself is the source project if GitMake reports ambiguity.

---

## Zero-config behavior

A `gitmake.json` file is **not required** for normal use.

GitMake can infer a safe configuration in memory.

For shell automation that intentionally supplies a one-run complete config, `gitmake --stdin` is an ephemeral publish/preview config path. Treat malformed stdin as a hard failure; GitMake must not fall back to inference. For reviewed plan/apply workflows, persist with `gitmake config write --stdin` first.

Do not create or edit `gitmake.json` with generic filesystem Write/Edit tools unless the user explicitly wants a persistent config and GitMake does not provide a config-writing tool.

If GitMake config tools are available, use them instead.

Safe defaults may include:

```text
branch      main
visibility  private for new repos unless explicitly requested otherwise
sync        managed
security    secret scan enabled
```

Always show inferred values to the user in the plan before apply.

---

## Project memory

After a successful publish, GitMake may store project identity in:

```text
.gitmake/project.json
```

This allows GitMake to remember the repository target even if the local folder is renamed.

Do not manually rewrite project identity files.

If GitMake reports a project identity mismatch, stop and ask the user to review it.

---

## Managed sync

GitMake uses managed synchronization to reduce accidental deletion of remote-only files.

Important behavior:

- Files previously managed by GitMake may be updated or removed.
- Remote-only files that GitMake never managed should normally be preserved.
- Protected paths may be preserved.
- Large deletion ratios may be classified as destructive.

Do not bypass a destructive-change block.

If GitMake reports that many managed files would be deleted, show that warning clearly to the user.

---

## Security rules

GitMake may block publishing when it detects:

- `.env` or credentials
- private keys
- tokens
- suspicious secrets
- oversized GitHub files
- Git LFS candidates
- branch protection conflicts
- stale plans
- project identity mismatches
- source/config changes after review
- destructive mass deletion
- concurrent apply attempts

If blocked, do not work around the safety gate with raw `git`, `gh`, GitHub API calls, force pushes, or manual repository mutations.

Explain the reason and follow GitMake's recommended recovery path.

---

## Plan integrity

A GitMake plan is reviewed state, not a suggestion that can be silently changed.

A plan may be bound to:

- source hash
- config hash
- repository
- branch
- remote baseline
- project identity
- plan fingerprint

If the source, config, or remote repository changes after plan creation, GitMake may reject the plan as stale.

When this happens:

1. create a fresh plan
2. show the new changes to the user
3. obtain fresh human approval
4. apply only the new plan

Never reuse an old approval for a new plan.

---

## Release behavior

If release publishing is configured, GitMake may create a GitHub Release after the repository push.

A release can contain:

```text
tag
title
release notes
assets
draft/prerelease state
```

Do not invent release assets.

Use only files selected or validated by GitMake.

If repository publishing succeeds but a release step fails, prefer GitMake's recovery/resume behavior rather than manually uploading assets with another tool.

---

## CLI usage

For humans, the normal CLI is intentionally small.

### Publish current project

```bash
gitmake
```

### Approve latest pending reviewed plan

```bash
gitmake approve
```

### Approve a destructive reviewed plan

```bash
gitmake approve --destructive
```

### Upgrade GitMake

```bash
gitmake upgrade
```

Read the result rather than assuming it succeeded. `✓ Installed <tag>` means the replacement is already on disk. `· Replacement scheduled after this process exits` means it is not yet done: report the printed verification command to the user instead of claiming the upgrade finished.

An installation at v1.2.5 or earlier cannot upgrade itself on Windows — staged replacement never ran in those builds. Direct the user to run `install` from the downloaded release package once.

### Check version

```bash
gitmake --version
```

### Check environment

```bash
gitmake doctor
```

### Connect Claude Code / MCP

Read-only:

```bash
gitmake ai setup
```

Write-capable GitMake MCP:

```bash
gitmake ai setup --write
```

Check connection:

```bash
gitmake ai status
```

Expert commands exist, but normal users should not need to memorize them.

---

## MCP availability

If GitMake MCP is available:

**Prefer MCP.**

Normal AI publishing flow:

```text
gitmake_publish
→ GitMake prepares and surfaces the reviewed plan
→ GitMake requests human approval through the MCP client UI
→ GitMake revalidates the exact reviewed plan
→ GitMake applies the plan and returns the final publish result
```

Use `gitmake_prepare` only for an explicit plan-only request, or as part of the compatibility fallback when interactive elicitation is unavailable. The fallback remains:

```text
gitmake_prepare
→ show plan
→ terminal `gitmake approve`
→ gitmake_apply(plan_id)
→ report result
```

If `gitmake_publish` is missing, the MCP connection may be read-only, stale, or connected to an older GitMake version. If `gitmake_publish` exists but reports that interactive elicitation is unavailable, use the fallback above.

Do not bypass GitMake.

Ask the user to check:

```bash
gitmake --version
gitmake ai setup --write
gitmake ai status
```

and restart the LLM client if its MCP tool list is stale.

---

## Things an LLM must NOT do

Do not:

```text
- force push
- rewrite Git history
- delete repositories
- bypass GitMake approval
- create fake approval
- silently choose an ambiguous source
- manually mutate GitHub when GitMake blocks an operation
- claim CREATE already happened before apply
- reuse consumed approval
- ignore PLAN_STALE
- ignore project identity mismatch
- bypass secret detection
- bypass destructive-change protection
- directly edit GitMake internal state files
```

Do not use raw `git`, `gh`, or GitHub API calls for publishing when the user explicitly asked to use GitMake or GitMake can perform the task.

---

## Recommended LLM response style

Keep GitMake interactions short.

Good:

```text
GitMake prepared the project.

Repository  owner/MyProject
Mode        CREATE
Visibility  private
Changes     +12 ~0 -0
Risk        low
Plan        gm_...

Run `gitmake approve` to approve this reviewed plan.
```

After approval and apply:

```text
Published successfully.

Repository  owner/MyProject
Branch      main
Changes     +12 ~0 -0
Release     none

https://github.com/owner/MyProject
```

Avoid dumping internal implementation details unless the user asks.

---

## If you are unsure

Ask GitMake first.

Prefer:

```text
GitMake → inspect / prepare / validate / plan
```

over guessing.

The product philosophy is:

> **Automate the repetitive GitHub publishing work, but keep risky mutations reviewable and human-approved.**


## GitMake v1.1 chat approval trust boundary

Claude Code and other elicitation-capable MCP clients can present the approval request directly to the human. GitMake trusts the connected MCP client to faithfully collect that response. If the client is configured with an `Elicitation` hook that auto-responds, that hook becomes part of the approval trust boundary. Do not auto-approve GitMake elicitation when manual human review is required.
