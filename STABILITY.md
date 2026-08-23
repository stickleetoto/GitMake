# GitMake v1 Stability Contract

GitMake v1.0.0 freezes the public publishing interface after the 0.x development line.

## Stable through v1.x

GitMake v1.x will preserve these interfaces unless a security issue makes a breaking change unavoidable:

- Everyday CLI entry points: `gitmake`, `gitmake <source>`, `gitmake upgrade`, `gitmake approve`, `gitmake ai setup`.
- Exit-code classes: `0` success, `1` runtime/safety failure, `2` usage error.
- Config schema: `gitmake.config/v1` / `schema_version: 1`.
- Reviewed-plan schema: `gitmake.plan/v1`.
- Project identity: `gitmake.project/v1`.
- Managed-sync metadata semantics and protected-path behavior.
- MCP tool names already shipped in v1.0, especially `gitmake_prepare` and `gitmake_apply`.
- `gitmake_apply` requires only `plan_id` in the v1 tokenless approval flow. A legacy pre-1.0 `approval_token` remains optional for compatibility.
- Safety invariants: no force push, no history rewriting, no repository deletion, no silent project retargeting, and no automatic bypass of protected branches or destructive-change gates.

## Approval semantics

`gitmake approve` creates a short-lived local, single-use approval grant bound to the exact reviewed plan fingerprint, source hash, config hash (when persisted), and target repository. The user does not copy an approval token into the AI.

Normal approvals expire after 10 minutes and are consumed only after a successful MCP apply. Destructive plans require `gitmake approve --destructive` and the stronger deletion confirmation phrase.

## Compatible evolution

v1.x may add optional JSON fields, new MCP tools, new platforms, new diagnostics, or performance improvements. Existing required fields and command meanings will not be repurposed.

## Recovery and concurrency

GitMake serializes concurrent mutations for the same plan/repository. Apply operations are journaled before mutation. If an apply is interrupted, the next attempt revalidates the complete reviewed plan against local source/config state and the GitHub remote before doing anything. GitMake never blindly resumes a stale write.

Release asset uploads can use the existing `release.on_existing: "resume"` behavior when a release exists but is incomplete.
