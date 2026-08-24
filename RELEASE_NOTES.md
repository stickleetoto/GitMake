# GitMake v1.1.0 — MCP Chat Approval

GitMake v1.1.0 is a backward-compatible v1 release that removes the last common approval-context switch for AI users.

When a connected MCP client supports **elicitation**, `gitmake_apply` can now ask the human for approval directly inside the client UI. Claude Code supports MCP elicitation dialogs. The model cannot create an approval through a GitMake tool; it triggers the client-controlled request and GitMake only proceeds after an accepted response.

## Approval flow

```text
project folder / ZIP
→ gitmake_prepare
→ reviewed plan
→ gitmake_apply(plan_id)
→ human approval dialog in MCP client
→ exact-plan revalidation
→ GitHub publish
```

Risk remains adaptive:

- low: Accept / Decline
- medium: human must enter `PUBLISH`
- high/destructive: human must enter the plan-specific `DELETE-XXXXXX` phrase

If the client does not support elicitation, the frozen v1 fallback remains:

```text
gitmake approve
```

No approval token copy/paste is used.

## Protocol support

- MCP 2026-07-28 Multi Round-Trip Requests (`input_required`, `inputResponses`, signed expiring `requestState`)
- MCP `server/discover`
- Legacy 2025-11-25 stdio `elicitation/create` for older/stateful clients
- Existing direct tools/list and v1 tool names remain compatible

## Safety hardening

- Chat approval binds to the exact reviewed plan fingerprint, source/config hashes, and repository.
- Request state is signed and expires.
- Accepted approval becomes the same short-lived single-use local grant used by terminal approval.
- A consumed grant cannot be re-minted for the same reviewed plan, preventing elicitation replay from authorizing a second mutation.
- Existing plan/repository locks, stale-plan checks, secret scanning, managed sync, project identity, destructive gates, and no-force-push/no-history-rewrite/no-repo-delete invariants remain.

## Compatibility

The v1 stability contract is preserved. `gitmake approve` is still supported and remains the fallback for clients without elicitation.
