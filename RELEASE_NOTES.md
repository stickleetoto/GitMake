# GitMake v0.10.0 — Guided UX + Trust & Recovery

v0.10.0 is the second half of the UX pass started in v0.9. It does not broaden GitMake into a general GitHub client. Instead it makes the existing zero-config folder/ZIP publishing flow easier to trust, easier to confirm, and easier to recover when GitMake intentionally blocks.

## 1. Risk-adaptive confirmation

Simple Mode now changes confirmation friction according to the reviewed plan:

```text
low       → Publish? [Y/n]
medium    → Type PUBLISH to continue
high      → Type DELETE-XXXXXX to confirm
```

`--yes` is deliberately limited to low-risk plans and cannot bypass medium/high-risk confirmation. Destructive plans still retain the existing expert and one-shot human approval controls.

## 2. Explain automatic decisions

Reviewed plans now carry `decision_notes`, shown to humans as a compact `Why` section. GitMake only explains facts that were actually used by the resolved pipeline: zero-config inference, project-memory restoration, folder/ZIP source selection evidence, private zero-config default, preserved remote visibility, and verified/first-adoption project identity.

## 3. Compact success result

Normal Simple Mode hides low-level pipeline chatter after success and shows the result that matters:

```text
✓ Published GambleLM

Repository  testuser/GambleLM
Branch      main
Changes     +2 ~4 -0
Release     none
Time        5.8s

https://github.com/testuser/GambleLM
```

Use `--verbose` when the pipeline details are useful.

## 4. Guided error recovery

Friendly errors now include a stable code when available plus a `Recommended` section with the safest next action. GitMake can guide source disambiguation, GitHub login, Git LFS setup, exclusion of files that should never be published, and rebuilding stale plans. It intentionally does not auto-override project-identity mismatches or destructive safety gates.

## 5. First-run readiness UX

`GitMake-Setup.exe` now checks the full path from installation to first publish:

- GitMake CLI + user PATH
- Git
- GitHub CLI
- GitHub login
- optional Claude Code
- optional read-only GitMake MCP connection

When publishing prerequisites are ready, Setup finishes with a simple next action: open a project folder and run `gitmake`, or ask Claude to publish the project when MCP is connected.

## Compatibility and safety

The v0.9 zero-config contract remains intact. Folder and ZIP mode, project memory, source ambiguity handling, reviewed plans, managed sync, protected paths, secret/large-file/LFS preflight, branch protection checks, project identity, stale-plan revalidation, one-shot MCP approvals, and no-force-push/no-history-rewrite/no-repo-delete invariants are preserved.
