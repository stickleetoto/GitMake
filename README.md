# GitMake v0.3.1

GitMake turns a project ZIP into a GitHub repository with one command. It creates the repository when it does not exist, mirrors later ZIP snapshots into the existing repository while preserving Git history, and can optionally publish GitHub Releases with assets.

v0.3.1 keeps the installable Windows CLI and fixes/strengthens installation diagnostics:

```powershell
gitmake
```

## Install on Windows

### Easiest

Unzip the Windows package and double-click:

```text
GitMake-Setup.exe
```

The setup program copies `gitmake.exe` to:

```text
%LOCALAPPDATA%\Programs\GitMake\gitmake.exe
```

and adds that directory to the current user's PATH. No administrator permission is required. Open a new PowerShell/Terminal window after installation.

### From the portable binary

```powershell
.\gitmake.exe install
```

Then verify the environment:

```powershell
gitmake doctor
```

## Requirements

GitMake intentionally delegates Git/GitHub authentication to the standard tools:

- Git available as `git`
- GitHub CLI available as `gh`
- GitHub login: `gh auth login`
- Git commit identity (`user.name` and `user.email`)

GitMake itself is a single native executable and does not require Go at runtime.

## Daily workflow

Put a project ZIP in a folder:

```text
ContextDiet-release/
└─ ContextDiet_v0.4.0.zip
```

Open a terminal in that folder and run:

```powershell
gitmake
```

With one ZIP and no config, GitMake automatically creates `gitmake.json`, derives the repository name, validates the ZIP, and continues in the same run.

Later, replace the ZIP contents/version and run `gitmake` again. If the GitHub repository already exists, GitMake clones it, preserves `.git`, mirrors the ZIP snapshot, commits only real changes, and pushes.

### Explicit ZIP

If multiple ZIP files are present, select the source directly:

```powershell
gitmake ContextDiet_v0.4.0_Source.zip
```

GitMake never guesses when multiple ZIPs are ambiguous.

## CLI

```text
gitmake                     Publish/update current project
gitmake Project.zip         Publish using a specific source ZIP
gitmake init [Project.zip]  Create gitmake.json
gitmake doctor              Check Git, gh, login, identity, and PATH
gitmake install             Install GitMake for the current Windows user
gitmake upgrade             Upgrade from the latest GitMake GitHub Release
gitmake help                Show help
```

Common flags:

```text
--dry-run       Preview without modifying GitHub
--no-release    Skip configured GitHub Release creation
--verbose       Print external git/gh commands
--keep-temp     Keep the temporary workspace for debugging
--create-only   Refuse to update an existing repository
--update-only   Refuse to create a missing repository
--config PATH   Use another JSON config
--version       Print version
```

## `gitmake doctor`

Example healthy output:

```text
GitMake Doctor · 0.3.1

✓ Git              git version 2.x
✓ GitHub CLI       gh version 2.x
✓ GitHub login     your-user
✓ Git identity     Your Name <you@example.com>
✓ GitMake install  C:\Users\<you>\AppData\Local\Programs\GitMake\gitmake.exe
✓ CLI command      C:\Users\<you>\AppData\Local\Programs\GitMake\gitmake.exe

Everything looks good.
```

When a dependency is missing, GitMake's normal error output also points to `gitmake doctor` and the relevant fix.

## Configuration

A minimal generated configuration looks like:

```json
{
  "schema_version": 1,
  "repo": {
    "name": "ContextDiet",
    "visibility": "private"
  },
  "source": {
    "zip": "ContextDiet_v0.4.0.zip",
    "strip_root": true
  },
  "git": {
    "branch": "main",
    "initial_commit_message": "Initial commit",
    "commit_message": "Update repository"
  }
}
```

To make a repository public, change:

```json
"visibility": "public"
```

## Automatic GitHub Releases

Add a release block:

```json
{
  "release": {
    "enabled": true,
    "tag": "v0.4.0",
    "title": "ContextDiet v0.4.0",
    "generate_notes": true,
    "assets": [
      "ContextDiet_v0.4.0_Windows_x64.zip",
      "ContextDiet_v0.4.0_Source.zip"
    ],
    "on_existing": "error"
  }
}
```

A normal run then performs repository create/update first and Release publication second. Release assets and release-note files are validated before repository mutation.

`on_existing` can be `error` (safe default) or `skip`.

## Upgrade

Installed or portable Windows builds can ask the public GitMake repository for the latest Release:

```powershell
gitmake upgrade
```

GitMake downloads the matching `GitMake_vX.Y.Z_Windows_x64.zip`, stages the new executable, exits, and lets a small replacement helper swap the executable after the running process releases the file.

## Clean output

Normal operation intentionally avoids verbose step counters:

```text
GitMake 0.3.1

  GitMake
  stickleetoto/GitMake · public

✓ Source validated      34 files
✓ Repository updated    +3 ~7 -1
✓ Pushed                main
✓ Released              v0.3.0 · 2 assets
  https://github.com/stickleetoto/GitMake/releases/tag/v0.3.0

Done in 2.8s
```

Use `--verbose` only when the underlying external commands are needed for debugging.

## Snapshot semantics

The source ZIP is authoritative for repository working-tree files:

```text
existing repository clone
        ↓
preserve .git
        ↓
mirror ZIP snapshot
        ↓
git add -A
        ↓
commit only if changed
        ↓
push
```

Files removed from a new ZIP are therefore removed in the new repository commit instead of lingering forever.

## Safety

GitMake deliberately does **not** implement repository deletion, force push, history rewriting, Release deletion, or destructive remote cleanup.

ZIP validation rejects path traversal, absolute paths, `.git`, symlinks/special files, Windows reserved names, Windows-invalid paths, case-collisions, file/directory conflicts, invalid UTF-8 names, excessive entry counts, and oversized extracted content.

Source archives are processed in temporary workspaces. Git history is preserved on updates.

## Build

```powershell
.\build.ps1
```

or:

```text
go test ./...
go vet ./...
go build -o gitmake.exe ./cmd/gitmake
```

The Windows build scripts create both `gitmake.exe` and `GitMake-Setup.exe`.
