# GitMake v1.2.5 — Real-World Workflow Hardening

GitMake v1.2.5 is a focused stability patch based on real project usage after the v1.2.4 freeze candidate. It does not change the publishing safety model or approval semantics.

## Fixed

### Authoritative `--stdin` config

Root `gitmake --stdin ...` previously accepted the flag but the publish pipeline ignored the supplied JSON and continued with file/inferred config. This could make a caller believe `repo.name`, `visibility`, or `git.branch` had been applied when they had not.

v1.2.5 strictly parses a complete config from stdin, validates it with the same schema/default rules as `gitmake.json`, uses it for the invocation, and reports `config.source: "stdin"`. Invalid or empty input fails closed. Explicit stdin config is not silently rewritten by project-memory defaults.

`gitmake plan --stdin` is rejected because an ephemeral config cannot be reproduced for later exact-plan apply. Use `gitmake config write --stdin` first when a persisted plan/apply workflow is required.

### Complete security findings

The scanner previously stopped after the first matching secret kind inside each file, and Slack tokens were not part of the high-confidence rules. It now aggregates every supported kind per file and across files, with deterministic path/kind ordering, and includes a Slack token rule. The block still occurs before mutation.

### MCP error transparency

CLI failures already carried structured machine errors, but the MCP adapter discarded that payload and returned only a generic exit-code message. MCP error results now preserve the original GitMake machine result in `structuredContent` and text content, including error code, stage, recovery guidance, and security findings.

### CLI guidance

`gitmake preview` is not a real subcommand in the flag-oriented CLI. v1.2.5 detects that common mistake and points users to `gitmake --dry-run --read-only` rather than trying to open a path named `preview`.

## Verification

- `go test ./...`
- `go vet ./...`
- `go test -race ./...`
- v1.0 guided/tokenless stability E2E
- v1.1 chat approval E2E
- v1.2 one-shot publish E2E
- v1.2.1 protocol routing E2E
- v1.2.3 locked executable recovery E2E
- v1.2.4 respawn-safe replacement E2E
- v1.2.5 real-world workflow E2E
- GitMake source-tree self secret scan

No repository was published during these local regression checks.
