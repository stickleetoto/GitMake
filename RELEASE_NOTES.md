# GitMake v0.7.2 — Project Identity + Destructive Change Gate

v0.7.2 closes the project-context failure found during real Claude MCP testing: a plan was created against the wrong working directory/config/source combination and would have removed most of an existing repository if approved. The AI warned about the deletion count, but GitMake itself now enforces the safety boundary.

## What changed

### Project identity binding

GitMake commits protected metadata at `.gitmake/project.json` containing a deterministic project ID and the bound `owner/repository`. Updates validate that identity before synchronization. A conflicting valid identity fails with `PROJECT_IDENTITY_MISMATCH`; GitMake does not auto-rebind it.

### No stale-config ZIP retargeting

If a real repository config references a ZIP that no longer exists, GitMake no longer replaces `source.zip` simply because one other ZIP happens to be in the folder. The operation stops with `PROJECT_SOURCE_MISMATCH` and asks for an explicit config correction. Placeholder starter configs keep their convenient self-healing behavior.

### Destructive mass-deletion gate

A reviewed update becomes destructive when both conditions are true:

- at least 10 previously managed files would be deleted, and
- at least 30% of the previous managed baseline would disappear.

Plan creation is still allowed so the user can inspect the exact proposal, but mutation is blocked by default.

Local explicit apply:

```powershell
gitmake apply <plan_id> --destructive
```

MCP explicit approval:

```powershell
gitmake approve <plan_id> --destructive
```

The destructive approval requires an interactive human confirmation string, is plan-bound, expires, is single-use, and is rejected on replay. MCP cannot mint it.

### Stronger plan provenance

Plans now expose the fields that must be checked before approval:

- working directory
- config path
- source ZIP path and digest
- target repository
- configured visibility and actual remote visibility
- project identity status and ID
- change counts
- managed deletion baseline/ratio
- risk level and reasons

The MCP plan tool explicitly instructs agents to surface those values to the user.

### Remote visibility mismatch reporting

When updating an existing repository, GitMake reports if `repo.visibility` in the config differs from the actual GitHub repository visibility. Update mode does not silently change repository visibility.

## ZIP-only AI workflow

The intended v0.7.2 agent flow works from a folder containing only a project ZIP:

```text
MCP inspect
→ discovery
→ config suggest
→ guarded config write
→ validate/plan
→ provenance + risk review
→ human approval
→ apply
```

No hand-authored `gitmake.json` is required.

## Compatibility

- Config schema remains `gitmake.config/v1` / `schema_version: 1`.
- Existing v0.7.1 managed manifests remain valid.
- Existing repositories adopt `.gitmake/project.json` on their first successful v0.7.2 update.
- No force push, history rewrite, repository deletion, or unreviewed MCP publish capability was added.
