# GitMake v0.5.0 — Agent Interface

v0.5.0 makes GitMake directly discoverable and usable by AI coding agents while keeping the normal human CLI workflow unchanged.

## New

- `gitmake ai describe --json` exposes a stable machine-readable capability manifest.
- `gitmake ai install` adds a managed GitMake section to `AGENTS.md` and writes `.gitmake/ai.json`.
- `--json` produces machine-readable command results.
- `--read-only` provides a non-mutating agent mode.
- `gitmake --dry-run --read-only --json` is now the recommended safe agent preview workflow.
- Publish JSON includes pipeline state, repository mode, branch, file count, diff counts, Release metadata, and safety flags.
- Stable exit codes are declared for agent/tool integrations.

## Safety

Read-only mode refuses project/config mutations and real GitHub publishing. GitMake continues to omit force-push, history rewriting, and repository deletion entirely.

## Existing behavior retained

One-command ZIP publishing, automatic repository create/update, Git history preservation, GitHub Release assets, project setup, doctor, per-user installation, and self-upgrade remain supported.
