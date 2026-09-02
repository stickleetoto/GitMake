package gmerr

import (
	"errors"
	"fmt"
	"testing"
)

func TestWrapOnNilStaysNil(t *testing.T) {
	// Call sites wrap unconditionally, so a nil cause must not become a
	// non-nil error that reports a failure which never happened.
	if err := Wrap(ConfigInvalid, nil, "validate"); err != nil {
		t.Fatalf("Wrap(nil) = %v, want nil", err)
	}
}

func TestCodeOfFindsTheCodeThroughWrapping(t *testing.T) {
	base := New(PlanStale, "plan is stale")
	wrapped := fmt.Errorf("apply: %w", fmt.Errorf("revalidate: %w", base))
	if got := CodeOf(wrapped); got != PlanStale {
		t.Fatalf("CodeOf = %q, want %q", got, PlanStale)
	}
	if got := CodeOf(errors.New("plain")); got != "" {
		t.Fatalf("CodeOf on an uncoded error = %q, want empty", got)
	}
	if got := CodeOf(nil); got != "" {
		t.Fatalf("CodeOf(nil) = %q, want empty", got)
	}
}

func TestErrorPreservesTheCauseForIsAndAs(t *testing.T) {
	cause := errors.New("disk is full")
	err := Wrap(ConfigInvalid, cause, "write config")

	if !errors.Is(err, cause) {
		t.Fatal("the wrapped cause must remain reachable through errors.Is")
	}
	if got := err.Error(); got != "write config: disk is full" {
		t.Fatalf("message = %q", got)
	}

	// Wrapping with no message of its own should not introduce a stray colon.
	bare := Wrap(ConfigInvalid, cause, "")
	if got := bare.Error(); got != "disk is full" {
		t.Fatalf("bare wrap message = %q, want the cause verbatim", got)
	}
}

func TestEveryCodeHasGuidance(t *testing.T) {
	codes := Codes()
	if len(codes) < 20 {
		t.Fatalf("only %d documented codes; the v1 contract lists more", len(codes))
	}
	seen := map[Code]bool{}
	for _, code := range codes {
		if seen[code] {
			t.Fatalf("duplicate code %q", code)
		}
		seen[code] = true

		_, action, ok := Guidance(code)
		if !ok {
			t.Fatalf("code %q has no guidance", code)
		}
		if action == "" {
			t.Fatalf("code %q has no suggested action; every blocked user needs a next step", code)
		}
	}
	if _, _, ok := Guidance(Code("NOT_A_REAL_CODE")); ok {
		t.Fatal("an unknown code must not report guidance")
	}
}
