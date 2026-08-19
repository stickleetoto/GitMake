# GitMake v0.1.3 Regression Report

## Static / unit validation

- `go test ./...`: PASS
- `go vet ./...`: PASS
- `go test -race ./...`: PASS

## End-to-end matrix

The E2E suite uses the real Git executable and a deterministic fake `gh` adapter backed by real bare Git repositories.

- First run with no config + one ZIP: PASS
- Unicode / Korean working-directory path: PASS
- CREATE initial repository and push: PASS
- UPDATE with modified file: PASS
- UPDATE with added file: PASS
- UPDATE with deleted file: PASS
- Git history preservation: PASS
- No-change re-run creates no empty commit: PASS
- First run with no ZIP gives onboarding and zero exit status: PASS
- `YOUR_PROJECT.zip` / `YOUR_REPOSITORY` self-repair: PASS
- Multiple ZIP ambiguity lists candidates and makes no remote mutation: PASS
- Existing empty remote repository receives first commit: PASS
- Existing `master` default branch fallback from generated `main`: PASS
- CREATE dry-run makes no remote repository: PASS
- UPDATE dry-run makes no commit: PASS
- `--create-only` guard: PASS
- `--update-only` guard: PASS
- Authentication failure gives `gh auth login` guidance and makes no remote mutation: PASS
- Windows reserved ZIP filename rejection: PASS
- Windows case-colliding ZIP path rejection: PASS

## Archive unit coverage

- top-level directory stripping: PASS
- root-file preservation: PASS
- ZIP Slip / traversal rejection: PASS
- embedded `..` rejection: PASS
- `.git` path rejection: PASS
- Windows reserved name rejection: PASS
- trailing-dot rejection: PASS
- case-collision rejection: PASS
- file/directory conflict rejection: PASS

## Config unit coverage

- default values: PASS
- unknown-field rejection: PASS
- unsafe repository name rejection: PASS
- trailing JSON rejection: PASS
- UTF-8 BOM acceptance: PASS
- UTF-16 rejection: PASS
- starter creation without ZIP: PASS
- single-ZIP starter detection: PASS
- existing config preservation: PASS
- placeholder repair: PASS
- stale single-ZIP repair: PASS
- multiple-ZIP diagnostic: PASS
