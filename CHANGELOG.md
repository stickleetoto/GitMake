# Changelog

## v0.2.0
- Added automatic GitHub Release creation after a successful repository create/update.
- Added release configuration: tag, title, inline notes, notes file, generated notes, assets, draft, prerelease, latest, and duplicate-tag policy.
- Release assets and notes files are validated before any repository mutation, preventing a missing local artifact from causing a half-finished update.
- Existing release tags fail early by default; `release.on_existing: "skip"` provides explicit idempotent skip behavior.
- Added exact Git tag validation through `git check-ref-format`.
- New releases can be created even when the repository snapshot itself has no changes.
- Added `--no-release` to update/create the repository without publishing the configured release.
- `--dry-run` now previews release creation and asset upload as well as Git changes.
- Release target follows the actual branch used by GitMake, including legacy default-branch fallback.
- Added release regression coverage for CREATE, no-change release, duplicate tags, skip behavior, missing assets, dry-run, `--no-release`, and invalid tags.

## v0.1.3
- Fixed the v0.1.2 `YOUR_PROJECT.zip` trap: starter placeholders now self-repair when a ZIP is added later.
- First run with exactly one ZIP now creates the config and continues in the same invocation.
- First run with no ZIP is treated as onboarding rather than a fatal file-not-found error.
- A stale configured ZIP automatically rebinds when exactly one replacement ZIP exists.
- Multiple ZIP ambiguity now lists every candidate instead of guessing.
- Existing empty GitHub repositories can now receive their first commit.
- Existing repositories whose default branch is not `main` are handled for the common generated-config compatibility case.
- Added UTF-8 BOM support and a clear UTF-16 config rejection.
- Hardened ZIP validation for embedded `..`, Windows device names, invalid Windows filename characters, trailing dot/space names, case collisions, file/directory conflicts, invalid UTF-8 names, and actual extraction-size overflow.
- Snapshot cleanup now makes read-only files writable before replacement when needed on Windows.
- GitHub authentication check is scoped to `github.com`.
- Added broader unit, race, vet and end-to-end regression coverage.
- Windows distribution ZIP is packaged flat so extracting it does not create an unnecessary duplicate folder level.

## v0.1.2
- Missing `gitmake.json` no longer fails with a raw file-not-found error.
- First run creates a starter `gitmake.json` automatically.
- If exactly one ZIP is present, `source.zip` is selected automatically.
- Repository name is derived from common versioned ZIP names such as `ContextDiet_v0.1.0.zip` -> `ContextDiet`.
- Existing configuration files are never overwritten.
- Keeps the v0.1.1 Explorer double-click pause behavior.

## v0.1.1
- Explorer double-click mode uses the executable directory as the working directory.
- Console stays open until Enter so errors can be read.
