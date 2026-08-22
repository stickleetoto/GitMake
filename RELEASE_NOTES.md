# GitMake v0.8.0 — Folder or ZIP, Same Safe Workflow

v0.8.0 adds direct **project folder publishing** without removing or weakening GitMake's established ZIP workflow.

## Two first-class source modes

ZIP mode remains unchanged:

```text
gitmake Project.zip
```

Folder mode can now publish a live source tree:

```text
cd MyProject
gitmake .
```

A normal project directory can also be inferred by `gitmake` / `gitmake_prepare` when strong project markers are present.

## Folder snapshots, not direct working-tree commits

GitMake does not commit the live folder directly. It builds a deterministic temporary snapshot, then runs the same security, identity, managed-sync, planning, approval, and apply pipeline already used for ZIP sources.

Folder mode:

- honors common root/nested `.gitignore` rules;
- supports a root `.gitmakeignore` for publish-only exclusions;
- always excludes `.git/`, `.gitmake/`, `gitmake.json`, `.env`, common dependency/cache directories, and platform junk;
- rejects symlinks, special files, unsafe relative paths, and case-colliding paths;
- hashes only the selected publishable snapshot, so ignored-file changes do not stale a reviewed plan;
- invalidates a reviewed plan when an included source file changes.

## Config schema

`source` now accepts exactly one mode:

```json
{"source":{"folder":"."}}
```

or:

```json
{"source":{"zip":"Project.zip","strip_root":true}}
```

Existing ZIP configs remain compatible with schema version 1.

## MCP / LLM workflow

`gitmake_prepare` is now the primary high-level entry point for **both folders and ZIPs**. It infers the source mode, creates a safe snapshot, validates or authors config through GitMake, runs preflight checks, and returns the immutable reviewed plan before any GitHub mutation.

Plans now explicitly carry `source_mode` (`folder` or `zip`) alongside `source_path` and the deterministic source SHA-256.

## Regression coverage

v0.8.0 adds E2E coverage for:

- direct folder publishing;
- `.gitignore` / `.gitmakeignore` and hard default exclusions;
- exclusion of `gitmake.json` and local secret/cache files from the repository;
- folder-mode plan provenance;
- ignored-file changes remaining plan-safe;
- included-file changes making a plan stale;
- MCP `gitmake_prepare` inferring a folder source with no ZIP and no config.

All prior ZIP, MCP, safety-gate, project-identity, destructive-approval, AI setup, and release regression suites were rerun after the change.
