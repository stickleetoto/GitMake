# GitMake v0.7.1

- Fix self-publish false positives caused by secret-scanner unit-test fixtures containing literal private-key/token signatures.
- Add a regression test so scanner test fixtures cannot silently reintroduce signatures that block GitMake source publication.

# GitMake v0.7.1 — Safety Gate + Cross-Agent Hardening

v0.7.1 hardens GitMake for real repositories and AI-driven workflows. The focus is not more GitHub surface area; it is making the small publish/update/release workflow safer, more explainable, and more portable.

## Seven problems addressed

1. **Remote-only file loss** — default sync is now `managed`. GitMake records the files it owns in `.gitmake/managed.json`, deletes only previously managed files that disappear from the next source ZIP, and preserves unrelated repository files. `.github/**` and `.gitmake/**` are protected by default.
2. **Secret leakage** — a security preflight scans high-risk filenames and common secret patterns before Git commits or GitHub writes. Likely private keys, GitHub/AWS tokens, `.env` secrets, and similar findings block publication unless explicitly allow-listed.
3. **Broad AI write access** — MCP apply now requires a one-shot, expiring human approval token created interactively with `gitmake approve <plan_id>`. The token is bound to one reviewed plan and is consumed after a successful apply.
4. **MCP gaps** — new MCP/CLI inspection and config-suggestion capabilities let agents inspect a project, discover ZIPs, obtain the authoritative schema, suggest config in memory, validate, plan, and apply without falling back to raw Git/GitHub workflows.
5. **Multi-ZIP guess risk** — discovery is more conservative. Content evidence outweighs filenames, binary-looking single archives are not treated as source automatically, and close source candidates return `needs_input` instead of being guessed.
6. **GitHub edge cases** — preflight checks large files, Git LFS availability, required-PR branch protection, existing bare tags, and stale remote state. GitMake still never force-pushes, so races after planning fail safely as non-fast-forward updates.
7. **Claude/Windows bias** — GitMake now supports per-user installation on Linux/macOS (`~/.local/bin`) and emits a generic stdio MCP descriptor with `gitmake ai setup --client generic --json`. Claude Code remains the one-click client integration.

## Managed sync migration

v0.7.1 changes the default update behavior from mirror-style deletion to managed ownership. On the first v0.7 adoption of an existing repository, files that exist only on the remote working tree are preserved. GitMake then records its managed source files in `.gitmake/managed.json`; future updates may remove a file only if GitMake previously managed it and it has disappeared from the new source ZIP.

For legacy exact-snapshot behavior, set:

```json
{
  "sync": {
    "mode": "snapshot"
  }
}
```

Protected paths are still honored in snapshot mode.

## Security configuration

```json
{
  "sync": {
    "mode": "managed",
    "protected_paths": [".github/**", ".gitmake/**"]
  },
  "security": {
    "secret_scan": true,
    "allow_secret_paths": [],
    "warn_file_bytes": 52428800,
    "max_git_file_bytes": 99614720
  }
}
```

Files beyond the direct-Git limit are blocked unless covered by Git LFS attributes and a working `git lfs` installation.

## Human-approved MCP apply

Plan first:

```powershell
gitmake plan --json
```

The human creates a short-lived token interactively:

```powershell
gitmake approve gm_0123456789abcdef
```

The agent supplies both the reviewed `plan_id` and approval token to the MCP apply tool. GitMake revalidates source/config/remote/release state before mutation and consumes the token after successful use.

## Generic MCP clients

```powershell
gitmake ai setup --client generic --json
```

This prints the portable stdio registration descriptor without editing an unknown client's configuration. Claude Code still supports:

```powershell
gitmake ai setup
```

## Compatibility

- Config schema remains `gitmake.config/v1` / `schema_version: 1`.
- Existing v0.6.x configs continue to work with the new safe defaults.
- No force push, history rewrite, or repository deletion capability was added.
