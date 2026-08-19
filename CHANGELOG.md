# Changelog

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
