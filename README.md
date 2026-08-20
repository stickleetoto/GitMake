# GitMake v0.5.0

GitMake turns a project ZIP into a GitHub repository and optional GitHub Release with one command.

v0.5.0 adds the **Agent Interface**: machine-readable output, built-in AI capability discovery, repository-local AI instructions, and a read-only preview mode for agents.

```powershell
gitmake
```

## What GitMake owns

GitMake intentionally handles a small workflow well:

```text
project ZIP
   ↓
validate snapshot
   ↓
create/update GitHub repository
   ↓
commit + push
   ↓
optional GitHub Release + assets
```

It does not try to replace GitHub CLI, Git, pull requests, issues, Actions, or general repository administration.

## Install on Windows

Unzip the Windows package and double-click:

```text
GitMake-Setup.exe
```

The setup program installs `gitmake.exe` to:

```text
%LOCALAPPDATA%\Programs\GitMake\gitmake.exe
```

and registers the directory in the current user's PATH without requiring administrator privileges.

You can also install from the portable binary:

```powershell
.\gitmake.exe install
```

Then open a new terminal and verify:

```powershell
gitmake --version
gitmake doctor
```

## Requirements

- Git available as `git`
- GitHub CLI available as `gh`
- GitHub login completed with `gh auth login`
- Git `user.name` and `user.email` configured

GitMake itself is a single native executable and does not require Go at runtime.

## Daily workflow

Put one project ZIP in a folder and run:

```powershell
gitmake
```

If `gitmake.json` is missing and there is exactly one ZIP, GitMake creates a safe starter configuration and continues in the same invocation.

For explicit setup:

```powershell
gitmake init
```

For a specific ZIP:

```powershell
gitmake Project_v1.2.3.zip
```

## Agent Interface

### Discover GitMake from an AI agent

```powershell
gitmake ai describe --json
```

This returns a stable manifest (`gitmake.ai/v1`) describing GitMake's purpose, commands, capabilities, safety boundaries, recommended agent workflow, and exit codes.

Example shape:

```json
{
  "schema": "gitmake.ai/v1",
  "name": "gitmake",
  "version": "0.5.0",
  "purpose": "...",
  "commands": {
    "preview": {
      "command": "gitmake --dry-run --read-only --json"
    },
    "publish": {
      "command": "gitmake --json"
    }
  },
  "safety": {
    "force_push": false,
    "rewrite_history": false,
    "delete_repositories": false
  }
}
```

Human-readable discovery is also available:

```powershell
gitmake ai describe
```

### Install repository-local AI instructions

Run inside a project:

```powershell
gitmake ai install
```

GitMake creates/updates only its managed section in:

```text
AGENTS.md
```

and writes the full machine-readable manifest to:

```text
.gitmake/ai.json
```

Existing user-authored `AGENTS.md` content is preserved. Re-running the command is idempotent.

### Safe AI preview

For an AI that should inspect the publish plan but not mutate the project or GitHub:

```powershell
gitmake --dry-run --read-only --json
```

`--read-only` blocks `init`, `install`, `upgrade`, `ai install`, and any real publish. A publish in read-only mode is only allowed together with `--dry-run` and an already existing `gitmake.json`.

## JSON output

Use `--json` on normal commands:

```powershell
gitmake --json
gitmake doctor --json
gitmake help --json
gitmake --version --json
```

Publish results use `gitmake.result/v1` and include a structured pipeline summary:

```json
{
  "schema": "gitmake.result/v1",
  "ok": true,
  "version": "0.5.0",
  "command": "publish",
  "exit_code": 0,
  "pipeline": {
    "stage": "REPORT",
    "mode": "UPDATE",
    "repository": "owner/project",
    "branch": "main",
    "files": 37,
    "changes": {
      "added": 2,
      "modified": 4,
      "deleted": 1
    },
    "dry_run": true,
    "read_only": true
  }
}
```

Human console output remains the default when `--json` is not supplied.

## Pipeline stages

The Agent Interface exposes a stable high-level execution model:

```text
DISCOVER
→ PLAN
→ PREPARE
→ VALIDATE
→ GIT
→ PUSH
→ RELEASE
→ REPORT
```

Some stages are skipped when they are not applicable, such as `PUSH` during a dry run.

## CLI

```text
gitmake                     Publish/update current project
gitmake Project.zip         Publish using a specific source ZIP
gitmake init [Project.zip]  Create gitmake.json
gitmake doctor              Diagnose Git/GitHub/install state
gitmake install             Install GitMake for the current Windows user
gitmake upgrade             Upgrade from the latest GitMake Release
gitmake ai describe         Describe capabilities to AI agents
gitmake ai install          Install repository-local AI guidance
gitmake help                Show help
```

Common flags:

```text
--dry-run       Preview without modifying GitHub
--read-only     Block mutations; use with --dry-run for agent previews
--json          Emit machine-readable JSON
--no-release    Skip configured Release creation
--verbose       Print external git/gh commands
--yes           Accept safe init defaults
--keep-temp     Keep temporary workspace for debugging
--create-only   Refuse to update an existing repository
--update-only   Refuse to create a missing repository
--config PATH   Use another JSON config
--version       Print GitMake version
```

## Exit codes

```text
0  success
1  runtime/environment/workflow error
2  CLI usage error
```

These exit codes are also declared by `gitmake ai describe --json`.

## Configuration

Minimal example:

```json
{
  "schema_version": 1,
  "repo": {
    "name": "ContextDiet",
    "visibility": "private"
  },
  "source": {
    "zip": "ContextDiet_v1.0.0.zip",
    "strip_root": true
  },
  "git": {
    "branch": "main",
    "initial_commit_message": "Initial commit",
    "commit_message": "Update repository"
  }
}
```

Optional GitHub Release:

```json
{
  "release": {
    "enabled": true,
    "tag": "v1.0.0",
    "title": "ContextDiet v1.0.0",
    "generate_notes": true,
    "assets": [
      "ContextDiet_v1.0.0_Windows_x64.zip",
      "ContextDiet_v1.0.0_Source.zip"
    ],
    "on_existing": "error"
  }
}
```

## Safety

GitMake deliberately does not provide:

- force push
- Git history rewriting
- repository deletion

ZIP extraction also rejects path traversal, embedded `.git`, symlinks, Windows-invalid names, case-colliding paths, and extraction-limit violations.

## Build from source

```powershell
go test ./...
go vet ./...
go build ./cmd/gitmake
```

Windows build helpers are included as `build.ps1` and `build.bat`.

## License

MIT. See `LICENSE`.
