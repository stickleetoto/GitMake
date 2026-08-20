# GitMake v0.5.0 Test Report

Date: 2026-08-21

## Summary

GitMake v0.5.0 Agent Interface passed unit, static, race, legacy E2E, new AI/JSON/read-only E2E, and Windows x64 cross-build validation.

## Automated checks

| Check | Result |
|---|---|
| `go test ./...` | PASS |
| `go vet ./...` | PASS |
| `go test -race ./...` | PASS |
| Base create/update/release E2E | PASS (`ALL_E2E_PASS`) |
| v0.3 install/CLI regression E2E | PASS (`V03_E2E_PASS`) |
| v0.4 init UX regression E2E | PASS (`V04_E2E_PASS`) |
| v0.5 Agent Interface E2E | PASS (`V05_E2E_PASS`) |
| Windows amd64 `gitmake.exe` cross-build | PASS |
| Windows amd64 `GitMake-Setup.exe` cross-build | PASS |
| PE32+ x86-64 verification | PASS |

## v0.5 Agent Interface coverage

Validated:

- `gitmake ai describe --json` emits valid `gitmake.ai/v1` JSON.
- AI manifest declares capabilities, safety boundaries, recommended workflow, and stable exit codes.
- `gitmake ai install` preserves pre-existing `AGENTS.md` user content.
- Re-running `gitmake ai install` is idempotent and maintains exactly one managed GitMake section.
- `.gitmake/ai.json` is written as valid machine-readable capability metadata.
- `--read-only` blocks `init`, `install`, `upgrade`, and `ai install` mutations.
- Read-only publish without `--dry-run` is rejected.
- Read-only dry-run refuses to create a missing `gitmake.json`.
- Read-only source resolution refuses to auto-repair stale `source.zip` and leaves config bytes unchanged.
- `--version --json` emits the stable `gitmake.version/v1` schema.
- Generic `--json` output emits the stable `gitmake.result/v1` envelope.
- AI-safe publish preview (`--dry-run --read-only --json`) returns structured repository mode, target, file count, diff counts, safety flags, and final pipeline stage without GitHub mutation.

## Legacy regression coverage retained

The base/v0.3/v0.4 suites still cover:

- CREATE and UPDATE repository flows.
- Git history preservation.
- Snapshot add/modify/delete mirroring.
- no-change updates without empty commits.
- Unicode paths.
- ambiguous/missing ZIP handling.
- empty remote repositories.
- non-`main` default-branch fallback.
- dry-run create/update.
- create-only/update-only guards.
- authentication errors.
- Windows-invalid ZIP paths and case collisions.
- GitHub Release creation, assets, duplicate handling, no-change Releases, missing assets, and `--no-release`.
- interactive/non-interactive `gitmake init` behavior.

## Windows build artifacts

Cross-compiled with:

```text
GOOS=windows
GOARCH=amd64
CGO_ENABLED=0
```

Verified file type:

```text
gitmake.exe       PE32+ executable, x86-64
GitMake-Setup.exe PE32+ executable, x86-64
```

Binary SHA-256 at build time:

```text
14a3c79d0734e88ce0589bc36788dda5d3cc2c266f515f291e3f81865fa8f25e  gitmake.exe
df678bbb6301770d9ca5fd57010e06b4b747872632837023a87707cead90ab8a  GitMake-Setup.exe
```

## Platform note

The Linux CI/container environment can execute the portable Go logic and E2E harnesses, but cannot directly exercise Windows Explorer launch behavior, Windows registry/PATH mutation, or in-place executable replacement. Those Windows-specific mechanisms are unchanged from the v0.3.1 line that was previously validated on a real Windows host; v0.5.0's new Agent Interface logic is platform-neutral and covered by automated tests.
