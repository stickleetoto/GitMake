package app

import (
	"errors"
	"io"
	"os"
	"strings"
	"testing"

	"gitmake/internal/planstore"
)

func TestDecisionNotesExplainZeroConfigAndVisibility(t *testing.T) {
	s := &PipelineState{
		Mode:       "CREATE",
		Visibility: "private",
		SourceMode: "folder",
		Config:     &ConfigState{Source: "inferred"},
	}
	notes := decisionNotesFromState(s)
	joined := strings.Join(notes, "\n")
	for _, want := range []string{"inferred in memory", "project folder", "Private visibility"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("missing %q in %q", want, joined)
		}
	}
}

func TestConfirmationCodeUsesPlanSuffix(t *testing.T) {
	if got := confirmationCode("gm_56ba4fcc81bcc066"); got != "BCC066" {
		t.Fatalf("confirmationCode=%q", got)
	}
}

func TestPlanCarriesDecisionNotes(t *testing.T) {
	s := &PipelineState{
		Mode:         "CREATE",
		Repository:   "owner/demo",
		Visibility:   "private",
		Branch:       "main",
		SourceMode:   "folder",
		SourcePath:   "/tmp/demo",
		SourceSHA256: strings.Repeat("a", 64),
		Changes:      &ChangeCounts{Added: 2},
		Risk:         &RiskState{Level: "low"},
		Config:       &ConfigState{Source: "inferred"},
		Identity:     &IdentityState{Status: "created", Repository: "owner/demo"},
	}
	p, err := planFromState("gm_test", "/tmp/demo", s)
	if err != nil {
		t.Fatal(err)
	}
	if len(p.DecisionNotes) == 0 {
		t.Fatal("expected decision notes")
	}
}

func TestSimpleSuccessIsCompact(t *testing.T) {
	p := planstore.Plan{Repository: "owner/demo", Branch: "main", Changes: planstore.ChangeCounts{Added: 3}}
	state := &PipelineState{Repository: "owner/demo", RepositoryURL: "https://example.test/owner/demo", Branch: "main", Changes: &ChangeCounts{Added: 3}}
	out := captureStdoutForTest(t, func() { printSimpleSuccess(p, state, 0) })
	for _, want := range []string{"✓ Published demo", "Repository  owner/demo", "Changes     +3 ~0 -0", "https://example.test/owner/demo"} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q in %q", want, out)
		}
	}
	if strings.Contains(out, "DISCOVER") || strings.Contains(out, "PREPARE") {
		t.Fatalf("success screen leaked pipeline detail: %q", out)
	}
}

func TestFriendlyErrorIncludesGuidedRecovery(t *testing.T) {
	old := os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stderr = w
	printFriendlyError(errors.New("potential secrets detected; publish blocked: .env (env_file)"))
	_ = w.Close()
	os.Stderr = old
	b, _ := io.ReadAll(r)
	_ = r.Close()
	text := string(b)
	for _, want := range []string{"SECRET_DETECTED", "Recommended", ".gitmakeignore", "Nothing was published"} {
		if !strings.Contains(text, want) {
			t.Fatalf("missing %q in %q", want, text)
		}
	}
}

func captureStdoutForTest(t *testing.T, fn func()) string {
	t.Helper()
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w
	fn()
	_ = w.Close()
	os.Stdout = old
	b, _ := io.ReadAll(r)
	_ = r.Close()
	return string(b)
}
