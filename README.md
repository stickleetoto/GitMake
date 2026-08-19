# GitMake v0.1.3

`project.zip + gitmake.json + gitmake.exe` -> automatically create or update a GitHub repository.

GitMake treats the ZIP as the latest repository snapshot. If the GitHub repository does not exist, it creates one and pushes an initial commit. If it already exists, GitMake clones it into a temporary workspace, preserves `.git`, mirrors the ZIP snapshot, commits changes, and pushes.

## Requirements

- Git installed and available as `git`
- GitHub CLI installed and available as `gh`
- One-time authentication: `gh auth login`
- Git commit identity configured when a commit is needed:
  - `git config --global user.name "Your Name"`
  - `git config --global user.email "you@example.com"`

`gitmake.exe` itself does not require Go or another runtime.

## Fastest Windows workflow

Put the project ZIP beside `gitmake.exe` and double-click it.

```text
work-folder/
├─ gitmake.exe
└─ ContextDiet_v0.1.0.zip
```

If `gitmake.json` does not exist, GitMake creates it. When exactly one ZIP is present, GitMake automatically selects the ZIP, derives a repository name such as `ContextDiet`, and continues in the same run.

If no ZIP is present yet, GitMake creates the starter JSON and shows an onboarding message instead of failing. Add one ZIP and run it again; placeholder values are repaired automatically.

For explicit control, provide `gitmake.json` yourself:

```json
{
  "schema_version": 1,
  "repo": {
    "name": "ContextDiet",
    "description": "Fast file context optimizer",
    "visibility": "private"
  },
  "source": {
    "zip": "ContextDiet_v0.1.0.zip",
    "strip_root": true
  },
  "git": {
    "branch": "main",
    "initial_commit_message": "Initial commit",
    "commit_message": "Update ContextDiet"
  }
}
```

UTF-8 JSON with or without a UTF-8 BOM is accepted. UTF-16 JSON is rejected with a clear encoding error.

## Automatic recovery

v0.1.3 repairs common first-run problems automatically:

- `YOUR_PROJECT.zip` remains in a starter config and one ZIP is later added -> selects that ZIP and updates the JSON.
- `YOUR_REPOSITORY` remains -> derives a repository name from the ZIP.
- configured ZIP was renamed/removed and exactly one ZIP remains -> rebinds `source.zip` to that ZIP.
- multiple possible ZIPs -> stops and lists the candidates instead of guessing.
- existing GitHub repository is empty -> creates the configured branch, commits the ZIP, and pushes.
- generated config says `main` but an existing legacy repository only has another default branch such as `master` -> uses the existing default branch and reports the fallback.

## Double-click behavior

When launched directly from Explorer, GitMake switches to the directory containing `gitmake.exe`, so the JSON and ZIP beside it are found reliably. The console waits for Enter before closing.

`RUN_GITMAKE.bat` is also included and always pauses after execution.

## Dry run

```powershell
.\gitmake.exe --dry-run
```

A dry run performs discovery, validation, repository detection and staging, but does not create a remote repository, commit, or push.

## Flags

```text
--config PATH       Config path (default: gitmake.json)
--dry-run           Do not create/commit/push
--verbose           Print external commands
--keep-temp         Keep temporary workspace
--create-only       Fail if repository already exists
--update-only       Fail if repository does not exist
--version           Print version
```

## Snapshot update semantics

The ZIP is authoritative for repository files.

```text
existing repo clone
      ↓
preserve .git
      ↓
remove old working-tree files
      ↓
copy ZIP snapshot
      ↓
git add -A
      ↓
commit + push only if changed
```

A file that exists in GitHub but is absent from the new ZIP is deleted in the update commit.

## ZIP safety and Windows compatibility

GitMake rejects archives containing:

- absolute paths or drive-absolute paths
- any `..` traversal component, including embedded paths such as `a/../b`
- any `.git` path, case-insensitive
- symlinks or unsupported special file types
- Windows-reserved device names such as `CON`, `NUL`, `COM1`, `LPT1`
- Windows-invalid characters or components ending in a dot/space
- case-colliding paths such as `A.txt` and `a.txt`
- file/directory path conflicts
- invalid UTF-8 path names
- more than 100,000 ZIP entries
- more than 8 GiB declared or actually extracted content

The source ZIP and JSON are never modified except when GitMake intentionally repairs its own starter JSON fields. Git operations run in a temporary workspace.

## `strip_root`

`"strip_root": true` is the default.

```text
ZIP:
ContextDiet/
├─ README.md
└─ src/

Repository:
README.md
src/
```

If the ZIP already has files at its root, GitMake keeps that layout instead of stripping a directory incorrectly.

## Build and test

```powershell
.\build.ps1
```

Or:

```text
go test ./...
go vet ./...
go build -o gitmake.exe ./cmd/gitmake
```

A Linux regression harness for CREATE/UPDATE and fake-GitHub E2E testing is included at `scripts/e2e.sh`.

## Safety scope

GitMake v0.1.x intentionally does not implement repository deletion, force push, history rewrite, PR management, Issues, Releases, Actions management, or multi-ZIP merging.
