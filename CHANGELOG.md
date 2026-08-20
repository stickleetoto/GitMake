# Changelog

## v0.4.0

- Added interactive `gitmake init` wizard.
- Added numbered source selection for folders with multiple ZIPs.
- Added repository name, visibility, description, and branch prompts.
- Added final confirmation before config creation.
- Added `--yes` safe-default non-interactive setup.
- `init` with no ZIP no longer creates placeholder config.
- Setup cancellation leaves the project directory unchanged.
- Existing config remains protected from accidental overwrite.

## v0.3.1

- Fixed a Windows `gitmake doctor` false negative where `gitmake --version` worked but doctor still reported PATH as missing.
- Doctor now checks the installed binary, actual command resolution, current-process PATH, and persisted per-user PATH independently.
- Added a manual PATH scan fallback for `exec.LookPath` / `PATHEXT` edge cases.
- Doctor now reports the resolved `gitmake.exe` path and distinguishes a stale shell from a broken installation.
- `gitmake install` no longer tries to overwrite its own currently-running installed executable; it safely verifies/repairs PATH instead.
- Hardened installer staging cleanup and replacement error reporting.
- Added regression tests for Windows-style PATH matching and doctor health classification.

## v0.3.0

### Installable CLI
- Added `gitmake install` for per-user Windows installation under `%LOCALAPPDATA%\Programs\GitMake`.
- Added automatic user-PATH registration without requiring administrator privileges.
- Added `GitMake-Setup.exe` for a double-click installer workflow.
- Added `INSTALL_GITMAKE.bat` as a persistent-console fallback.

### CLI UX
- Reworked normal output into compact project/result summaries instead of numbered internal steps.
- Added actionable error presentation with dependency/auth/Git-identity suggestions.
- Added `gitmake help`.
- Added positional source ZIP support: `gitmake Project.zip`.
- Default run no longer writes a placeholder config when there is no ZIP; it gives an onboarding instruction instead.
- One unambiguous ZIP still creates configuration and publishes in a single invocation.
- Multiple ZIPs are never guessed; candidates are listed and the explicit ZIP syntax is suggested.
- Added compact change counts (`+added ~modified -deleted`).
- Preserved Explorer double-click pause behavior.

### Diagnostics
- Added `gitmake doctor` checks for Git, GitHub CLI, GitHub authentication, Git identity, PATH installation, and local project config presence.

### Self update
- Added `gitmake upgrade` using the latest public `stickleetoto/GitMake` GitHub Release.
- Windows upgrades download the matching Windows x64 ZIP and replace the running executable after process exit.

### Existing v0.2 functionality retained
- CREATE/UPDATE auto-detection.
- Snapshot mirroring with Git-history preservation.
- Safe ZIP extraction and Windows path validation.
- Dry run / create-only / update-only.
- Optional GitHub Release creation and asset upload.
- Existing-release protection.

## v0.2.0
- Added GitHub Release creation, tags, notes, assets, duplicate-release handling, and `--no-release`.

## v0.1.3
- Expanded first-run, empty-repository, legacy-branch, Unicode/BOM, ZIP safety, and Windows compatibility handling.