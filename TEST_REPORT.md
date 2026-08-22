# GitMake v0.8.0 Test Report

Date: 2026-08-21  
Status: **PASS** for the implemented v0.8.0 Folder Mode scope.

## Scope

v0.8.0 adds live project-folder input alongside the existing ZIP workflow while preserving the reviewed plan, security, managed-sync, project-identity, approval-token, and MCP apply gates.

## Static / Go tests

- `go test ./...` — PASS
- `go vet ./...` — PASS
- `go test -race ./...` — PASS

## Regression suites

- Base E2E — PASS (`ALL_E2E_PASS`)
- v0.3 E2E — PASS (`V03_E2E_PASS`)
- v0.4 E2E — PASS (`V04_E2E_PASS`)
- v0.5 E2E — PASS (`V05_E2E_PASS`)
- v0.5.1 E2E — PASS (`V051_E2E_PASS`)
- v0.5.2 E2E — PASS (`V052_E2E_PASS`)
- v0.6 MCP E2E — PASS (`V06_MCP_E2E_PASS`)
- v0.6.1 AI setup E2E — PASS (`V061_AI_SETUP_E2E_PASS`)
- v0.7 safety E2E — PASS (`V07_E2E_PASS`)
- v0.7.2 safety E2E — PASS (`V072_SAFETY_E2E_PASS`)
- v0.7.3 high-level prepare / safety E2E — PASS (`V073_SAFETY_E2E_PASS`)
- v0.8.0 folder E2E — PASS (`V080_FOLDER_E2E_PASS`)

## v0.8.0 Folder Mode coverage

### Direct folder publish

A project tree can be published directly with `gitmake .` / `source.folder`.

Verified:
- folder input is detected and normalized;
- `gitmake.json` is excluded from the source snapshot;
- `.git/`, `.gitmake/`, `.env`, common local caches, and ignored files do not enter the snapshot;
- common root/nested `.gitignore` rules and root `.gitmakeignore` rules are applied;
- selected files pass through the existing secret scan and Git/GitHub preflight;
- managed sync, project identity, commit, and push use the same existing pipeline as ZIP mode.

### Deterministic snapshot / plan binding

Verified:
- folder plans record `source_mode: folder` and an absolute `source_path`;
- ignored-file changes do **not** stale a reviewed plan;
- included-file changes **do** stale a reviewed plan and block apply;
- the snapshot hashes the exact bytes copied into the temporary snapshot, closing the plan-hash/copy TOCTOU gap;
- symlinks, unsupported special files, unsafe relative paths, and case-colliding paths are rejected.

### Source selection safety

Verified:
- a clear live project folder can be selected without first creating a ZIP;
- an explicit folder or ZIP path is the strongest selection signal;
- existing `source.zip` configs keep their ZIP semantics;
- ambiguous/suspicious multi-ZIP selection is an error and is never converted into a successful no-op;
- the user/agent can resolve ambiguity explicitly with `gitmake .` or `gitmake Project.zip`.

### MCP high-level prepare

Verified in a folder containing no project ZIP and no config:
- `gitmake_prepare` detects Folder Mode;
- it can infer config in memory under read-only MCP;
- it produces a reviewed CREATE plan;
- it does not create `gitmake.json` in read-only mode;
- the result carries folder source provenance through the normal plan/risk pipeline.

## Compatibility

- Existing ZIP configs continue to use `source.zip` + optional `strip_root`.
- Folder configs use `source.folder` and reject `strip_root`.
- Exactly one of `source.zip` or `source.folder` is required.
- Config schema remains `schema_version: 1` because the new source alternative is additive and old valid configs remain valid.

## Known boundary

The built-in ignore matcher intentionally targets common `.gitignore` patterns rather than claiming complete parity with every exotic Git ignore grammar edge case. Explicit `.gitmakeignore` remains available for publish-specific exclusions.
