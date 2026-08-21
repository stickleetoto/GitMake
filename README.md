# GitMake v0.5.2

GitMake turns a project ZIP into a GitHub repository and optional GitHub Release with one command.

v0.5.2 hardens the Agent Interface so LLMs can discover GitMake, author `gitmake.json` from an authoritative schema, review an immutable publish plan, and apply it only if the reviewed inputs are still current.

```powershell
gitmake
```

## Scope

GitMake intentionally owns one small workflow:

```text
project ZIP
   ↓
discover + validate
   ↓
create/update GitHub repository
   ↓
commit + push
   ↓
optional GitHub Release + assets
```

It is not a replacement for Git, GitHub CLI, pull requests, issues, Actions, or general repository administration.

## Windows install

Unzip the Windows package and double-click `GitMake-Setup.exe`, or run:

```powershell
.\gitmake.exe install
```

GitMake installs per-user to `%LOCALAPPDATA%\Programs\GitMake` and adds that directory to the user PATH. Verify with:

```powershell
gitmake --version
gitmake doctor
```

Requirements: `git`, GitHub CLI `gh`, `gh auth login`, and a configured Git identity.

## Daily workflow

```powershell
gitmake                 # auto create/update from the configured/inferred ZIP
gitmake Project.zip     # explicit source ZIP
gitmake --dry-run       # preview only
gitmake --no-release    # update repo but skip configured release
```

## Multi-ZIP discovery

```powershell
gitmake discover --json
```

Selection priority is deterministic:

```text
explicit ZIP argument
→ gitmake.json
→ one confidently classified source ZIP
→ otherwise needs_input / user selection
```

GitMake uses both archive names and ZIP contents. The JSON report includes confidence, evidence, release-asset candidates, unknown archives, and ambiguity state. If two archives independently look like real source projects, GitMake refuses to guess.

## AI / Agent Interface

Discover capabilities without prior GitMake knowledge:

```powershell
gitmake ai describe --json
```

Install repository-local agent guidance:

```powershell
gitmake ai install
```

This manages only its own section in `AGENTS.md` and writes `.gitmake/ai.json`.

A safe agent preview is:

```powershell
gitmake --dry-run --read-only --json
```

When no `gitmake.json` exists, read-only preview may infer safe defaults in memory. It does not persist a config or mutate GitHub.

## LLM-authored configuration

Agents should never guess GitMake's config shape. Read the authoritative local schema first:

```powershell
gitmake config schema --json
```

Then an LLM can supply a complete configuration through stdin:

```powershell
Get-Content generated.json | gitmake config write --stdin --json
```

Validate the persisted config:

```powershell
gitmake config validate --json
```

Patch only selected fields while preserving the rest:

```powershell
'{"release":{"enabled":true,"tag":"v1.0.0"}}' |
  gitmake config patch --stdin --json
```

Both write and patch are strictly parsed, reject unknown fields, apply documented defaults, validate before replacing the file, and use a guarded replacement flow. Preview an authored config without writing it with `--dry-run`. `--read-only` blocks config writes/patches.

## Plan → Apply approval workflow

For AI or human approval boundaries:

```powershell
gitmake plan --json
```

The stored `gitmake.plan/v1` includes:

- source ZIP SHA-256
- persisted config SHA-256 when present
- repository and mode
- remote base commit for updates
- exact change counts
- release asset and notes digests
- a plan fingerprint

After review:

```powershell
gitmake apply gm_0123456789abcdef --json
```

Before execution GitMake revalidates the source, config, remote repository state, planned diff, and release inputs. If anything changed after review, apply fails with `PLAN_STALE` instead of publishing a different state than the one approved.

## Release recovery

A configured release can use:

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

If the release already exists, GitMake compares uploaded asset names and uploads only missing configured assets. This supports recovery from partial release-asset upload failures without recreating the release.

## Operation history

```powershell
gitmake history
gitmake history --json
```

GitMake stores recent publish/apply audit records in the user's cache, including success/failure, repository, mode, change counts, plan ID, release tag, and dry-run/read-only state. History failure never causes the requested publish operation itself to fail.

## Self-upgrade integrity

```powershell
gitmake upgrade
```

Starting with v0.5.2 releases, upgrade downloads both the Windows package and the matching `GitMake_vX.Y.Z_SHA256.txt` release asset. The package SHA-256 must match before the replacement executable is staged.

## Machine-readable errors

`--json` publish/apply failures use `gitmake.result/v1` and stable high-level codes such as:

```text
SOURCE_NOT_FOUND
SOURCE_AMBIGUOUS
CONFIG_INVALID
GH_AUTH_REQUIRED
GH_CLI_NOT_FOUND
GIT_NOT_FOUND
PLAN_NOT_FOUND
PLAN_STALE
RELEASE_EXISTS
UPGRADE_INTEGRITY_FAILED
```

Errors include stage, recoverability, and a suggested action when applicable.

## Pipeline

```text
DISCOVER → PLAN → PREPARE → VALIDATE → GIT → PUSH → RELEASE → REPORT
```

Dry-run/read-only flows omit mutating stages where appropriate.

## CLI

```text
gitmake                         Publish/update current project
gitmake Project.zip             Use a specific source ZIP
gitmake init [Project.zip]      Create gitmake.json
gitmake doctor                  Diagnose Git/GitHub/install state
gitmake discover                Classify ZIP candidates
gitmake plan [Project.zip]      Store a reviewed immutable plan
gitmake apply <plan_id>         Revalidate and apply the plan
gitmake history                 Show recent operations

gitmake config schema           Print authoritative config JSON Schema
gitmake config validate         Validate gitmake.json
gitmake config write --stdin    Validate + write full config from stdin
gitmake config patch --stdin    Merge + validate a config patch

gitmake ai describe             Describe capabilities for agents
gitmake ai install              Install repository-local AI guidance

gitmake install                 Install for current Windows user
gitmake upgrade                 Upgrade from latest GitMake release
gitmake help
gitmake --version
```

Common flags may appear before or after a positional argument:

```text
--dry-run
--read-only
--json
--no-release
--verbose
--yes
--keep-temp
--create-only
--update-only
--config PATH
```

## Safety invariants

- no force push
- no Git history rewrite
- no repository deletion
- ZIP traversal / `.git` injection / symlink / Windows-invalid path defenses
- read-only publish requires dry-run
- ambiguous source archives are never guessed
- release assets are never silently enabled by discovery
- agent-authored config is strictly schema/semantic validated
- apply refuses stale reviewed plans
- self-upgrade verifies SHA-256 before replacement
