# GitMake v0.5.2 — Agent Hardening

v0.5.2 makes GitMake safer and easier for terminal-capable AI agents to operate.

Highlights:

- LLMs can read the authoritative `gitmake.json` schema with `gitmake config schema --json`.
- Full configs and partial patches can be supplied through stdin, strictly validated, previewed, and safely written.
- `gitmake plan` / `gitmake apply` binds user approval to SHA-256 fingerprints and the current remote state.
- multi-ZIP discovery now exposes confidence and evidence for its source choice.
- structured error codes help agents recover without parsing prose.
- `release.on_existing="resume"` recovers missing release assets.
- `gitmake history` records recent publish/apply outcomes.
- self-upgrade now verifies the downloaded Windows package against a published SHA-256 checksum before replacement.

GitMake still intentionally stays small: it handles project snapshot → repository → optional release publishing and leaves general GitHub administration to Git/GitHub CLI or dedicated tools.
