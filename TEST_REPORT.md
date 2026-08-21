# GitMake v0.5.2 Test Report

## Result

**PASS**

v0.5.2 passed unit, static, race, legacy end-to-end, Agent Interface, configless/multi-ZIP, LLM config-authoring, plan/apply, release-resume, upgrade-integrity, and Windows x64 cross-build validation.

## Automated validation

```text
go test ./...          PASS
go vet ./...           PASS
go test -race ./...    PASS

scripts/e2e.sh         ALL_E2E_PASS
scripts/e2e_v03.sh     V03_E2E_PASS
scripts/e2e_v04.sh     V04_E2E_PASS
scripts/e2e_v05.sh     V05_E2E_PASS
scripts/e2e_v051.sh    V051_E2E_PASS
scripts/e2e_v052.sh    V052_E2E_PASS
```

The full legacy+new E2E set was also run in sequence; the first long combined command reached the environment timeout after v0.5, then v0.5.1 and v0.5.2 were rerun separately and passed.

## v0.5.2 agent/config coverage

Validated:

- `gitmake config schema --json` returns strict `gitmake.config/v1` JSON Schema.
- full config supplied through stdin is strictly parsed, defaulted, validated, and written.
- unknown fields are rejected and surface `CONFIG_INVALID` in machine output.
- `config patch` recursively merges object fields while preserving unrelated config.
- `null` patch semantics delete a field before normal config defaults/validation are applied.
- config `--dry-run` validates and returns the normalized result without writing.
- `--read-only` blocks config write/patch.
- config replacement preserves the previous file until a validated replacement is ready.
- flags after positional arguments work (`gitmake apply <plan_id> --json`, `gitmake Project.zip --dry-run`).

## Plan / Apply coverage

Validated:

- `gitmake plan --json` stores a `gitmake.plan/v1` plan in user cache.
- plans contain source SHA-256, config SHA-256 when persisted, repository/mode/branch, remote baseline, changes, release digests, and fingerprint.
- `gitmake apply <plan_id> --json` successfully creates the reviewed repository when state is unchanged.
- modifying the source ZIP after plan creation causes apply to fail with `PLAN_STALE` before GitHub mutation.
- remote/config/release state is recomputed immediately before apply through the same read-only planning path.

## Recovery / integrity coverage

Validated:

- `release.on_existing="resume"` detects assets already present on an existing release and uploads only missing configured assets.
- operation history includes successful and failed publish/apply records and plan IDs.
- self-upgrade checksum parser verifies the Windows package against the matching published SHA-256 line.
- tampered package bytes are rejected by checksum validation.

## Existing safety/regression coverage

Retained and revalidated:

- CREATE / UPDATE / no-change behavior
- Git history preservation
- empty remote repositories
- default-branch fallback
- GitHub auth failure
- dry-run and create/update-only guards
- GitHub Release create/skip/no-change flows
- ZIP traversal defense
- protected `.git` rejection
- symlink rejection
- Windows reserved names
- Windows case-colliding paths
- configless read-only planning
- multi-ZIP source/release-asset classification
- ambiguous source candidates are never guessed
- AGENTS.md managed section preservation/idempotency

## Windows build

Cross-built with:

```text
GOOS=windows
GOARCH=amd64
CGO_ENABLED=0
```

Verified artifacts:

```text
gitmake.exe       PE32+ x86-64 Windows console executable
GitMake-Setup.exe PE32+ x86-64 Windows console executable
```

Windows registry/PATH installation code is unchanged from the v0.3.1 line previously validated on a real Windows host. The new v0.5.2 features are platform-neutral except self-replacement, which remains Windows-specific and now receives only a checksum-verified package.
