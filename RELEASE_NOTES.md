# GitMake v0.4.0

GitMake v0.4.0 focuses on first-run project setup UX.

## New

- Interactive `gitmake init` wizard.
- Automatic ZIP discovery when exactly one project ZIP is present.
- Numbered ZIP selection when multiple archives are present.
- Repository-name suggestion derived from the ZIP filename.
- Visibility selection with safe `private` default.
- Optional repository description and branch selection.
- Final confirmation before writing `gitmake.json`.
- `gitmake init --yes` for safe non-interactive defaults.

## UX / safety

- `gitmake init` no longer writes placeholder configuration when there is no ZIP.
- Cancelling setup leaves the directory unchanged.
- Existing `gitmake.json` is never overwritten by normal `init`.
- Daily publishing remains one command: `gitmake`.

## Existing features retained

Repository create/update, snapshot mirroring, Release assets, `doctor`, per-user install, and self-upgrade remain supported.
