# GitMake v1.0.0 — Stable

GitMake v1.0.0 freezes the public interface after the 0.x development line.

The final pre-1.0 change is **tokenless human approval**: after an AI creates a reviewed plan, the user runs `gitmake approve`. GitMake stores a short-lived local single-use grant bound to the exact plan; there is no `gma_...` token to copy back into Claude or another MCP client. The AI can then call `gitmake_apply` with the plan ID only.

## Final v1 hardening

- Tokenless local approval grants, bound to plan fingerprint, source hash, config hash, and target repository.
- `gitmake approve` with no plan ID automatically selects the newest reviewed plan for the current project directory.
- Low-risk approval is `[Y/n]`; medium risk requires `PUBLISH`; destructive risk requires a plan-specific `DELETE-XXXXXX` phrase plus `--destructive`.
- Plan-level and repository-level mutation locks prevent overlapping publishes/applies.
- Apply operation journal records interrupted runs and forces full plan revalidation before a retry.
- Expired approval/plan/lock/journal cache housekeeping.
- Linux arm64 release build for Raspberry Pi 4/5 and other ARM64 Linux systems.
- Folder hashing benchmark added as a performance-regression guard; no speculative optimization was introduced.
- Pre-1.0 token-based MCP approval remains accepted as an optional compatibility path, but is deprecated.

## v1 stable surface

The v1 stability contract covers the everyday CLI, config schema v1, plan schema v1, project identity, managed-sync semantics, MCP tool names, exit-code classes, and core safety invariants. See `STABILITY.md`.

GitMake remains intentionally narrow: project folder/ZIP → reviewed plan → human approval → safe GitHub repository/release publish.
