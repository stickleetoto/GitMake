# GitMake v0.2.0

`project.zip + gitmake.json + gitmake.exe` -> create or update a GitHub repository, then optionally publish a GitHub Release with assets.

GitMake treats the source ZIP as the latest repository snapshot. If the GitHub repository does not exist, it creates one and pushes an initial commit. If it already exists, GitMake clones it into a temporary workspace, preserves `.git`, mirrors the ZIP snapshot, commits changes, and pushes. v0.2.0 can then create a tagged GitHub Release and upload local files such as Windows builds and source archives.

## Requirements

- Git installed and available as `git`
- GitHub CLI installed and available as `gh`
- One-time authentication: `gh auth login`
- Git commit identity configured when a commit is needed:
  - `git config --global user.name "Your Name"`
  - `git config --global user.email "you@example.com"`

`gitmake.exe` itself does not require Go or another runtime.

## Fastest Windows workflow

Put one project ZIP beside `gitmake.exe` and double-click it.

```text
work-folder/
├─ gitmake.exe
└─ ContextDiet_v0.2.0_Source.zip
```

If `gitmake.json` does not exist, GitMake creates it. When exactly one ZIP is present, GitMake automatically selects the ZIP, derives a repository name such as `ContextDiet`, and continues in the same run.

If no ZIP is present yet, GitMake creates a starter JSON and shows an onboarding message instead of failing. Add one ZIP and run it again; placeholder values are repaired automatically.

A minimal explicit config:

```json
{
  "schema_version": 1,
  "repo": {
    "name": "ContextDiet",
    "description": "Fast file context optimizer",
    "visibility": "private"
  },
  "source": {
    "zip": "ContextDiet_v0.2.0_Source.zip",
    "strip_root": true
  },
  "git": {
    "branch": "main",
    "initial_commit_message": "Initial commit",
    "commit_message": "Update ContextDiet v0.2.0"
  }
}
```

UTF-8 JSON with or without a UTF-8 BOM is accepted. UTF-16 JSON is rejected with a clear encoding error.

## Automatic GitHub Releases

Add a `release` block:

```json
{
  "schema_version": 1,
  "repo": {
    "name": "ContextDiet",
    "visibility": "public"
  },
  "source": {
    "zip": "ContextDiet_v0.2.0_Source.zip",
    "strip_root": true
  },
  "git": {
    "branch": "main",
    "commit_message": "Release v0.2.0"
  },
  "release": {
    "enabled": true,
    "tag": "v0.2.0",
    "title": "ContextDiet v0.2.0",
    "generate_notes": true,
    "assets": [
      "ContextDiet_v0.2.0_Windows_x64.zip",
      "ContextDiet_v0.2.0_Source.zip"
    ],
    "on_existing": "error"
  }
}
```

Then one run performs:

```text
validate source + release files
        ↓
CREATE or UPDATE repository
        ↓
commit + push
        ↓
create tag/release
        ↓
upload configured assets
```

Release creation happens only after the repository push succeeds. GitMake validates release assets and a notes file before mutating the repository, so a missing local artifact fails early.

A release can still be created when the repository snapshot has no new changes. This is useful when the Git commit is already present but the release publication step previously failed or was intentionally delayed.

### Release fields

| Field | Behavior |
|---|---|
| `enabled` | Enables release publication |
| `tag` | Required Git tag, e.g. `v0.2.0` |
| `title` | Optional release title |
| `notes` | Inline release notes |
| `notes_file` | Release notes file relative to `gitmake.json` |
| `generate_notes` | Ask GitHub to generate release notes; defaults to `true` when no notes are supplied |
| `assets` | Local files to upload; relative paths are resolved beside `gitmake.json` |
| `draft` | Create a draft release |
| `prerelease` | Mark as prerelease |
| `latest` | Optional explicit `true`/`false` Latest setting |
| `on_existing` | `error` (default) or `skip` when the release tag already exists |

`notes` and `notes_file` are mutually exclusive. Generated notes can also be explicitly enabled alongside `notes` or `notes_file`. If generation is explicitly disabled, one of those note sources is required so the CLI stays non-interactive.

GitMake rejects duplicate release asset basenames and asset paths containing `#`, because GitHub CLI uses `#` as an asset label separator.

### Duplicate releases

The safe default is:

```json
"on_existing": "error"
```

If `v0.2.0` already exists, GitMake stops before committing or pushing a new source update. This helps catch the common mistake of changing code without bumping the release tag.

If you intentionally want repeat runs to ignore an existing release:

```json
"on_existing": "skip"
```

The repository may still be updated, but the existing release is left untouched.

### Skip release for one run

```powershell
.\gitmake.exe --no-release
```

This keeps the `release` config intact but suppresses release publication for the current run.

## Automatic recovery

GitMake repairs common first-run problems automatically:

- `YOUR_PROJECT.zip` remains in a starter config and one ZIP is later added -> selects that ZIP and updates the JSON.
- `YOUR_REPOSITORY` remains -> derives a repository name from the ZIP.
- configured ZIP was renamed/removed and exactly one ZIP remains -> rebinds `source.zip` to that ZIP.
- multiple possible ZIPs -> stops and lists the candidates instead of guessing.
- existing GitHub repository is empty -> creates the configured branch, commits the ZIP, and pushes.
- generated config says `main` but an existing legacy repository only has another default branch such as `master` -> uses the existing default branch and reports the fallback.

When a release is enabled and multiple ZIP files exist beside GitMake, set `source.zip` explicitly. Release asset ZIP files count as ZIP candidates during first-run auto-discovery.

## Double-click behavior

When launched directly from Explorer, GitMake switches to the directory containing `gitmake.exe`, so the JSON and files beside it are found reliably. The console waits for Enter before closing.

`RUN_GITMAKE.bat` is also included and always pauses after execution.

## Dry run

```powershell
.\gitmake.exe --dry-run
```

A dry run performs discovery, validation, repository detection, staging, release conflict checks, and release-file validation, but does not create a remote repository, commit, push, tag, release, or upload assets.

## Flags

```text
--config PATH       Config path (default: gitmake.json)
--dry-run           Do not create/commit/push/release
--verbose           Print external commands
--keep-temp         Keep temporary workspace
--create-only       Fail if repository already exists
--update-only       Fail if repository does not exist
--no-release        Skip configured release creation for this run
--version           Print version
```

## Snapshot update semantics

The source ZIP is authoritative for repository files.

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

A file that exists in GitHub but is absent from the new source ZIP is deleted in the update commit.

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

A regression harness for CREATE/UPDATE/release flows with a fake GitHub CLI is included at `scripts/e2e.sh`.

## Safety scope

GitMake v0.2.0 intentionally does not implement repository deletion, force push, history rewrite, PR management, Issues, Actions management, release deletion, or release asset overwrite. Existing releases are either rejected or skipped according to explicit config.
