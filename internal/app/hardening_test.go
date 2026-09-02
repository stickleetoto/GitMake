package app

import (
	"errors"
	"fmt"
	"testing"

	"gitmake/internal/gmerr"
)

// The `--json` error codes are a frozen v1 contract, and until now nothing
// tested them: they were recovered by matching substrings of human-readable
// messages, so rewording any error anywhere could silently reclassify it.
// These tests pin the contract.

// TestEveryDocumentedCodeIsReported walks every code GitMake publishes and
// requires the classifier to report it with its documented guidance. A code
// that loses its guidance, or is renamed, fails here.
func TestEveryDocumentedCodeIsReported(t *testing.T) {
	codes := gmerr.Codes()
	if len(codes) == 0 {
		t.Fatal("no documented error codes")
	}
	for _, code := range codes {
		t.Run(string(code), func(t *testing.T) {
			wantRecoverable, wantAction, ok := gmerr.Guidance(code)
			if !ok {
				t.Fatalf("code %s has no guidance", code)
			}
			got := classifyMachineError(gmerr.New(code, "something went wrong"), nil)
			if got == nil {
				t.Fatal("classifier returned nothing")
			}
			if got.Code != string(code) {
				t.Fatalf("code = %q, want %q", got.Code, code)
			}
			if got.Recoverable != wantRecoverable {
				t.Fatalf("recoverable = %v, want %v", got.Recoverable, wantRecoverable)
			}
			if got.SuggestedAction != wantAction {
				t.Fatalf("suggested action = %q, want %q", got.SuggestedAction, wantAction)
			}
		})
	}
}

// TestCodeSurvivesWrapping matters because errors are wrapped as they travel up
// the pipeline; the code has to reach the JSON layer through those wrappers.
func TestCodeSurvivesWrapping(t *testing.T) {
	base := gmerr.New(gmerr.SecretDetected, "potential secrets detected")
	wrapped := fmt.Errorf("publish failed: %w", fmt.Errorf("safety gate: %w", base))

	got := classifyMachineError(wrapped, nil)
	if got.Code != "SECRET_DETECTED" {
		t.Fatalf("code = %q after two levels of wrapping, want SECRET_DETECTED", got.Code)
	}
	if got.Message != wrapped.Error() {
		t.Fatalf("message should stay the full chain, got %q", got.Message)
	}
}

// TestCodedErrorBeatsMessageMatching is the point of the whole change: the code
// wins even when the text looks like something else entirely.
func TestCodedErrorBeatsMessageMatching(t *testing.T) {
	err := gmerr.New(gmerr.PlanStale, "gitmake.json schema_version is unsupported")
	got := classifyMachineError(err, nil)
	if got.Code != "PLAN_STALE" {
		t.Fatalf("code = %q, want PLAN_STALE; message matching overrode the declared code", got.Code)
	}
}

// TestLegacyMessagesStillClassify covers the sites not yet converted. Each
// entry is a message shape the pipeline still produces, so this fails the
// moment one of them is reworded without being converted first.
func TestLegacyMessagesStillClassify(t *testing.T) {
	cases := []struct {
		message string
		want    string
	}{
		{"a large file exceeds the safe direct-git threshold", "LARGE_FILE_BLOCKED"},
		{"asset.bin is marked for Git LFS", "GIT_LFS_REQUIRED"},
		{"branch main requires pull requests", "BRANCH_REQUIRES_PR"},
		{"release tag v1.0.0 already exists", "TAG_CONFLICT"},
		{"failed to push: non-fast-forward", "REMOTE_MOVED"},
		{"destructive change blocked", "DESTRUCTIVE_CHANGE_BLOCKED"},
		{"refusing to retarget repository", "PROJECT_SOURCE_MISMATCH"},
		{"one-shot approval is required", "APPROVAL_REQUIRED"},
		{"GitHub authentication is not ready; run 'gh auth login'", "GH_AUTH_REQUIRED"},
		{"GitHub CLI (gh) not found", "GH_CLI_NOT_FOUND"},
		{"git not found", "GIT_NOT_FOUND"},
		{"plan gm_abc not found", "PLAN_NOT_FOUND"},
		{"SHA-256 mismatch for GitMake_v1.2.6_Windows_x64.zip", "UPGRADE_INTEGRITY_FAILED"},
		{"parse config: unexpected end of JSON input", "CONFIG_INVALID"},
		{"unsupported schema_version 2", "CONFIG_INVALID"},
	}
	for _, tc := range cases {
		t.Run(tc.want, func(t *testing.T) {
			got := classifyMachineError(errors.New(tc.message), nil)
			if got.Code != tc.want {
				t.Fatalf("%q classified as %s, want %s", tc.message, got.Code, tc.want)
			}
		})
	}
}

// TestUnrelatedFailuresAreNotMisreported guards the regression that motivated
// narrowing the config pattern. Reporting an I/O error or a usage refusal as
// CONFIG_INVALID sends the user to edit a file that is perfectly fine.
func TestUnrelatedFailuresAreNotMisreported(t *testing.T) {
	for _, message := range []string{
		"read-only mode blocks `gitmake config write`",
		"hash config: permission denied",
		"check config: input/output error",
		"gitmake config write requires --stdin",
	} {
		got := classifyMachineError(errors.New(message), nil)
		if got.Code == "CONFIG_INVALID" {
			t.Fatalf("%q was reported as CONFIG_INVALID; it is not a malformed configuration", message)
		}
		if got.Code != "RUNTIME_ERROR" {
			t.Fatalf("%q classified as %s, want RUNTIME_ERROR", message, got.Code)
		}
	}
}

func TestNilErrorClassifiesToNothing(t *testing.T) {
	if got := classifyMachineError(nil, nil); got != nil {
		t.Fatalf("nil error classified as %+v", got)
	}
}
