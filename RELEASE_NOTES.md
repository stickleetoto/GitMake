# GitMake v1.2.2 — Authless Self-Upgrade

GitMake v1.2.2 is a focused reliability patch for `gitmake upgrade`.

Publishing still uses Git/GitHub CLI authentication as before. Self-upgrade no longer does.

## Fixed

Previously `gitmake upgrade` called the normal GitHub publishing preflight first. That meant a public GitMake release could not be downloaded unless `gh` was installed and `gh auth status` succeeded.

v1.2.2 separates those concerns:

- latest-version discovery uses the public GitHub Releases API over HTTPS;
- release assets download anonymously from GitHub release URLs;
- no `gh auth login` is required for self-upgrade;
- the downloaded platform ZIP is still verified against the published SHA-256 file before replacement;
- release download URLs are restricted to HTTPS GitHub/GitHubusercontent hosts;
- a newer local GitMake build is never replaced by an older published release.

## Compatibility

The publishing workflow and MCP interfaces are unchanged:

```text
gitmake_publish
→ reviewed immutable plan
→ client-controlled human approval
→ exact-plan revalidation
→ apply
→ final repository/release result
```

Windows x64 remains the supported in-place self-replacement platform in v1.2.2. Other release binaries remain available for manual installation.

## Verification

- `go test ./...` PASS
- `go vet ./...` PASS
- `go test -race ./...` PASS
- v1.0 tokenless stability E2E PASS
- v1.1 chat approval E2E PASS
- v1.2 one-shot publish E2E PASS
- v1.2.1 protocol routing E2E PASS
- new anonymous updater and downgrade-refusal unit tests PASS
