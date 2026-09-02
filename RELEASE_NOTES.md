# GitMake v1.2.7 — Verification, Error Contract, and Secret Coverage

v1.2.6 fixed the updater. This release fixes the reasons that defect went unnoticed through three releases, and closes the largest gap in the safety gate GitMake exists to provide.

Nothing about publishing, approval, plans, config, project identity, or MCP changes. `gitmake upgrade` from v1.2.6 installs this normally.

## Verification infrastructure

The updater bug shipped three times while every gate reported green. The gates were the problem.

- **GitMake now has continuous integration.** There was none at all. Every change runs gofmt, build, vet, and tests on Linux, Windows, and macOS; the race detector where cgo is available; the E2E suites; a cross-build of every published target; and a full packaging job. Tests run **twice** per platform, which is the cheapest possible guard against state that survives a run.
- **Packaging is code, not a procedure.** `scripts/package.py` builds every release artifact, the source ZIP, the checksum manifest, and the self-publish folder from a clean tree. CI then unpacks the produced source ZIP and rebuilds and re-tests GitMake from it. v1.2.5 shipped a stray empty file and a checksum manifest `sha256sum -c` could not read; neither can happen unnoticed now.
- **The GitHub CLI test stub is a real executable.** The sixteen extensionless shell stubs it replaces were invisible to Go's `exec.LookPath` on Windows, so the suites silently fell through to the real authenticated `gh`. Windows E2E coverage goes from three suites to seven, and one suite that appeared to pass had in fact been querying a live account.

## Machine error codes are carried, not guessed

The `--json` error codes are a frozen v1 contract that was reconstructed by matching substrings of human-readable messages, with no test anywhere. Rewording any error could silently reclassify it.

The broadest pattern was actively wrong: every message merely containing the word "config" was reported as `CONFIG_INVALID`, so `read-only mode blocks gitmake config write` and an I/O failure from `hash config: ...` both told the user their `gitmake.json` was malformed.

Errors now carry their own code. Message matching remains for sites not yet converted, but every code it can still produce is pinned by a test: rewording one fails the build instead of changing the contract in silence.

## Secret scanning

The scanner knew five credential shapes. For a tool whose purpose is safe publishing of AI-authored projects, that missed the obvious cases — a model provider key, a cloud service account, or a payment key published cleanly.

- Added Anthropic, OpenAI, and Hugging Face keys; Google API keys, OAuth client secrets, and GCP service accounts; Stripe, SendGrid, npm, and Azure storage keys; Slack and Discord webhooks; PGP and encrypted private key blocks.
- Added two structural rules: a password embedded in a connection string, and a JWT. Documentation placeholders are rejected, so a README example does not block a publish.
- **File contents are scanned in full.** Anything over 2 MiB previously had its contents skipped entirely, so a credential in a log or a database dump was never examined.
- Findings now report a `confidence` and the **line** of the first match. Both confidence levels block — scanning stays fail-closed — but you can see which findings are issuer-specific and where to look.

## Coverage where a bug is worst

`internal/github` performs every remote mutation and `internal/planstore` holds the reviewed plans that approval is bound to. Both had zero tests.

- `internal/github`: 0% → 69%, driven through the compiled fake CLI, so both the arguments GitMake sends and the output it parses are exercised.
- `internal/planstore`: 0% → 83%, covering the substitution guards in particular: a plan file copied under another id, or written by a future schema, must not load.

## Machine surfaces for agents

`doctor`, `install`, and `upgrade` printed only for humans, so an agent had to parse prose to learn whether the environment was usable or whether an upgrade had landed.

```bash
gitmake doctor --json      # every check, its verdict, and the remedies
gitmake install --json     # target, whether replacement completed, PATH state
gitmake upgrade --json     # current and latest version, installed or scheduled
gitmake upgrade --check    # is there a newer release? nothing is downloaded
```

Human output for all three is rendered from the same report the JSON is built from, so the two cannot drift.

## Verification

- `go build ./...`, `go vet ./...`, `go test ./...` — and `go test` a second time, to prove the suite is repeatable
- Windows E2E: `e2e_v05`, `e2e_v051`, `e2e_v052`, `e2e_v121`, `e2e_v123`, `e2e_v124`, `e2e_v126`
- Release artifacts rebuilt, checksum manifest verified, and GitMake rebuilt and re-tested from the published source ZIP
- GitMake's own source tree still passes its own secret scan with all nineteen rules active

`go test -race` and the GitHub-CLI-dependent E2E suites run in CI on Linux and macOS; they were not run on the Windows development machine, which has no C compiler and cannot execute the pty-driven suites.

No repository was published during these checks.

## Upgrading

From v1.2.6, `gitmake upgrade` installs this release normally.

From v1.2.5 or earlier, self-upgrade cannot move you: staged replacement never ran in those builds. Extract the platform package and install once:

```powershell
.\gitmake.exe install
```

```bash
./gitmake install    # Linux / macOS
```
