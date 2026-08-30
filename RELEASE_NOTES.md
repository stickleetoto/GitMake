# GitMake v1.2.1 — Protocol Routing Hardening

GitMake v1.2.1 is a focused hardening patch for the v1.2 one-shot publish orchestrator. The public workflow remains the same:

```text
gitmake_publish
→ reviewed immutable plan
→ client-controlled human approval
→ exact-plan revalidation
→ apply
→ final repository/release result
```

## Fixed

A modern MCP 2026-07-28 client that performed `initialize` with elicitation support could leave the server's legacy elicitation flag enabled. On stdio, that could route a later `gitmake_publish` call into the old held-open `elicitation/create` path instead of returning the modern MRTR `input_required` result.

v1.2.1 now enables the legacy held-open path only for protocol `2025-11-25`. Modern `2026-07-28` requests remain on the MRTR flow.

Signed approval request state is also stricter: its `purpose` binding is now mandatory. A purpose-less state is rejected, and publish/apply states remain isolated from each other.

## Documentation and test cleanup

- `GITMAKE_FOR_LLM.md`, Quick Start, and Claude MCP setup now consistently describe `gitmake_publish` as the normal AI publishing entry point.
- prepare → terminal approve → apply is documented as the compatibility fallback, not the default path.
- Added a v1.2.1 protocol-routing E2E regression test.
- Historical v0.4/v0.5 E2E checks no longer pin the obsolete `1.0.0` patch version; they validate the stable v1 version/schema contract instead.

## Compatibility

No v1 public interface is removed or repurposed. `gitmake_prepare`, `gitmake_apply`, terminal `gitmake approve`, v1 schemas, risk-adaptive confirmation, managed sync, project identity, stale-plan checks, mutation locks, and the no-force-push/no-history-rewrite/no-repository-delete safety model remain unchanged.
