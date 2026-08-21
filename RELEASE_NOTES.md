# GitMake v0.7.3 — High-level MCP Prepare

v0.7.3 closes the remaining MCP-only workflow gap found in real Claude Code testing. Claude could discover the ZIP, infer a config, validate it, and create a safe plan through GitMake — but it still used its host `Write(gitmake.json)` tool between those steps.

GitMake now exposes one high-level MCP tool: **`gitmake_prepare`**.

## One call from ZIP to reviewed plan

For a folder containing only a project ZIP, an agent can call:

```text
gitmake_prepare
```

GitMake then performs the full preparation pipeline itself:

```text
discover source ZIP
→ infer validated config
→ security scan / large-file / LFS checks
→ Git + GitHub preflight
→ repository existence / visibility check
→ project identity validation
→ managed-sync change calculation
→ destructive-risk classification
→ immutable reviewed plan
→ stop before apply
```

The structured response uses `gitmake.prepare/v1` and includes the reviewed plan, config state, MCP access state, and the next required human-approval action.

## Read-only stays genuinely read-only

The default `gitmake ai setup` registration remains read-only. If no `gitmake.json` exists, `gitmake_prepare` infers and validates the config **in memory** and still creates the reviewed plan. It does not write the project and does not mutate GitHub.

This means an AI no longer needs to escape to its own filesystem Write/Edit tool just to reach the plan stage.

## Write-enabled mode uses GitMake's writer

If the user explicitly enabled:

```text
gitmake ai setup --write
```

then `gitmake_prepare` may persist a missing config. The write is routed through GitMake's existing strict schema validation and atomic replacement path. The high-level tool still does **not** publish or apply the plan.

## Agent guidance hardened

`gitmake ai describe --json` and the MCP tool description now explicitly tell agents:

- prefer `gitmake_prepare` for ZIP-only/unconfigured projects;
- do not create or edit `gitmake.json` with host filesystem Write/Edit tools when GitMake authoring tools are available;
- surface plan provenance, changes, and risk to the user;
- wait for explicit approval;
- MCP apply still requires a human-minted one-shot approval token.

## Safety inherited from v0.7.2

The high-level tool does not bypass any safety gates. Project identity binding, stale-source retarget refusal, managed-sync preservation, secret scanning, large-file/LFS checks, branch/tag preflight, stale-plan rejection, destructive deletion gates, and one-shot approval remain enforced by the same underlying GitMake pipeline.

No force push, history rewrite, repository deletion, or unreviewed MCP publish capability was added.
