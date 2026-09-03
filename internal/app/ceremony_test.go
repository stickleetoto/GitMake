package app

import (
	"errors"
	"strings"
	"testing"

	"gitmake/internal/planstore"
)

// STABILITY.md promises specific confirmation friction per risk level. Nothing
// tested it: the rules lived inside a function that printed to stdout and read
// a terminal, so the promise and the code could drift apart unnoticed. These
// tests are the contract.

// scriptedPrompter answers as a person would, and records what it was asked.
type scriptedPrompter struct {
	interactive bool
	answer      string
	err         error
	asked       []string
}

func (s *scriptedPrompter) Interactive() bool { return s.interactive }

func (s *scriptedPrompter) Ask(prompt string) (string, error) {
	s.asked = append(s.asked, prompt)
	return s.answer, s.err
}

func planWithRisk(level string, destructive bool) planstore.Plan {
	return planstore.Plan{
		ID:   "gm_1234567890abcdef",
		Mode: "UPDATE",
		Risk: planstore.Risk{Level: level, Destructive: destructive},
	}
}

func TestRequiredCeremonyPerRisk(t *testing.T) {
	cases := []struct {
		name        string
		risk        planstore.Risk
		want        Ceremony
		needsHuman  bool
		phrasePart  string
		description string
	}{
		{name: "low", risk: planstore.Risk{Level: "low"}, want: CeremonyYesNo, needsHuman: false},
		{name: "empty level defaults to low", risk: planstore.Risk{}, want: CeremonyYesNo, needsHuman: false},
		{name: "mixed case low", risk: planstore.Risk{Level: " LOW "}, want: CeremonyYesNo, needsHuman: false},
		{name: "medium", risk: planstore.Risk{Level: "medium"}, want: CeremonyTypedPublish, needsHuman: true, phrasePart: "PUBLISH"},
		{name: "high", risk: planstore.Risk{Level: "high"}, want: CeremonyTypedDelete, needsHuman: true, phrasePart: "DELETE-"},
		{name: "destructive outranks a low level", risk: planstore.Risk{Level: "low", Destructive: true}, want: CeremonyTypedDelete, needsHuman: true, phrasePart: "DELETE-"},
		{name: "destructive with no level", risk: planstore.Risk{Destructive: true}, want: CeremonyTypedDelete, needsHuman: true, phrasePart: "DELETE-"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := requiredCeremony(tc.risk)
			if got != tc.want {
				t.Fatalf("ceremony = %v, want %v", got, tc.want)
			}
			if got.RequiresHuman() != tc.needsHuman {
				t.Fatalf("RequiresHuman = %v, want %v", got.RequiresHuman(), tc.needsHuman)
			}
			if tc.phrasePart != "" && !strings.Contains(expectedPhrase(got, "gm_1234567890abcdef"), tc.phrasePart) {
				t.Fatalf("expected phrase %q does not contain %q", expectedPhrase(got, "gm_1234567890abcdef"), tc.phrasePart)
			}
		})
	}
}

// TestYesCannotPassAboveLowRisk is the invariant. --yes exists so an agent or a
// script can accept a routine change; it must never be able to accept a plan a
// human was supposed to look at.
func TestYesCannotPassAboveLowRisk(t *testing.T) {
	for _, risk := range []planstore.Risk{
		{Level: "medium"},
		{Level: "high"},
		{Level: "low", Destructive: true},
		{Destructive: true},
	} {
		t.Run(risk.Level+destructiveSuffix(risk), func(t *testing.T) {
			// Not a terminal: --yes must not substitute for the person.
			ask := &scriptedPrompter{interactive: false, answer: "PUBLISH"}
			confirmed, _, err := confirmPlan(planWithRisk(risk.Level, risk.Destructive), true, ask)
			if err == nil {
				t.Fatal("--yes was accepted for a plan that requires a human")
			}
			if confirmed {
				t.Fatal("plan reported confirmed without a human")
			}
			if len(ask.asked) != 0 {
				t.Fatalf("no prompt should be issued when confirmation is impossible: %v", ask.asked)
			}
			if !strings.Contains(err.Error(), "interactive confirmation") {
				t.Fatalf("error should explain the requirement, got %v", err)
			}

			// At a terminal, --yes still does not answer for the person: the
			// prompt is issued and the typed answer decides.
			ask = &scriptedPrompter{interactive: true, answer: ""}
			confirmed, _, err = confirmPlan(planWithRisk(risk.Level, risk.Destructive), true, ask)
			if err != nil {
				t.Fatal(err)
			}
			if confirmed {
				t.Fatal("an empty answer confirmed a plan that requires a typed phrase")
			}
			if len(ask.asked) != 1 {
				t.Fatalf("expected exactly one prompt, got %v", ask.asked)
			}
		})
	}
}

func destructiveSuffix(r planstore.Risk) string {
	if r.Destructive {
		return "-destructive"
	}
	return ""
}

func TestYesAcceptsLowRiskWithoutPrompting(t *testing.T) {
	ask := &scriptedPrompter{interactive: false}
	confirmed, destructive, err := confirmPlan(planWithRisk("low", false), true, ask)
	if err != nil {
		t.Fatal(err)
	}
	if !confirmed || destructive {
		t.Fatalf("confirmed=%v destructive=%v; --yes should accept a low-risk plan", confirmed, destructive)
	}
	if len(ask.asked) != 0 {
		t.Fatalf("--yes should not prompt for a low-risk plan: %v", ask.asked)
	}
}

func TestTypedDeletePhraseIsBoundToThePlan(t *testing.T) {
	plan := planWithRisk("high", true)
	other := planWithRisk("high", true)
	other.ID = "gm_fedcba0987654321"

	// The phrase from a different plan must not confirm this one.
	ask := &scriptedPrompter{interactive: true, answer: expectedPhrase(CeremonyTypedDelete, other.ID)}
	confirmed, _, err := confirmPlan(plan, false, ask)
	if err != nil {
		t.Fatal(err)
	}
	if confirmed {
		t.Fatal("a confirmation phrase from another plan was accepted")
	}

	// Its own phrase confirms, and reports the confirmation as destructive.
	ask = &scriptedPrompter{interactive: true, answer: expectedPhrase(CeremonyTypedDelete, plan.ID)}
	confirmed, destructive, err := confirmPlan(plan, false, ask)
	if err != nil {
		t.Fatal(err)
	}
	if !confirmed || !destructive {
		t.Fatalf("confirmed=%v destructive=%v for the plan's own phrase", confirmed, destructive)
	}
}

// TestDeletePhraseIsCaseSensitive matters because the phrase is the whole
// ceremony: accepting a near miss would defeat binding it to one plan.
func TestDeletePhraseIsCaseSensitive(t *testing.T) {
	plan := planWithRisk("high", true)
	phrase := expectedPhrase(CeremonyTypedDelete, plan.ID)

	ask := &scriptedPrompter{interactive: true, answer: strings.ToLower(phrase)}
	confirmed, _, err := confirmPlan(plan, false, ask)
	if err != nil {
		t.Fatal(err)
	}
	if confirmed {
		t.Fatalf("a lowercase %q was accepted", phrase)
	}
}

func TestTypedPublishAcceptsAnyCasing(t *testing.T) {
	// PUBLISH is a word the user is asked to type, not a plan-bound secret, so
	// casing is forgiving here where the delete phrase is not.
	for _, answer := range []string{"PUBLISH", "publish", " Publish "} {
		ask := &scriptedPrompter{interactive: true, answer: answer}
		confirmed, destructive, err := confirmPlan(planWithRisk("medium", false), false, ask)
		if err != nil {
			t.Fatal(err)
		}
		if !confirmed {
			t.Fatalf("answer %q should confirm a medium-risk plan", answer)
		}
		if destructive {
			t.Fatal("a medium-risk confirmation is not destructive")
		}
	}
	for _, answer := range []string{"", "y", "yes", "PUBLIS", "PUBLISH now"} {
		ask := &scriptedPrompter{interactive: true, answer: answer}
		confirmed, _, err := confirmPlan(planWithRisk("medium", false), false, ask)
		if err != nil {
			t.Fatal(err)
		}
		if confirmed {
			t.Fatalf("answer %q must not confirm a medium-risk plan", answer)
		}
	}
}

func TestLowRiskAnswers(t *testing.T) {
	yes := []string{"", "y", "Y", "yes", "YES", " y "}
	no := []string{"n", "no", "nope", "x"}

	for _, answer := range yes {
		ask := &scriptedPrompter{interactive: true, answer: answer}
		confirmed, _, err := confirmPlan(planWithRisk("low", false), false, ask)
		if err != nil {
			t.Fatal(err)
		}
		if !confirmed {
			t.Fatalf("answer %q should confirm; [Y/n] defaults to yes", answer)
		}
	}
	for _, answer := range no {
		ask := &scriptedPrompter{interactive: true, answer: answer}
		confirmed, _, err := confirmPlan(planWithRisk("low", false), false, ask)
		if err != nil {
			t.Fatal(err)
		}
		if confirmed {
			t.Fatalf("answer %q must not confirm", answer)
		}
	}
}

func TestPromptTextMatchesTheCeremony(t *testing.T) {
	update := planWithRisk("low", false)
	create := planWithRisk("low", false)
	create.Mode = "CREATE"

	if got := ceremonyPrompt(CeremonyYesNo, update); !strings.Contains(got, "Publish update?") {
		t.Fatalf("update prompt = %q", got)
	}
	if got := ceremonyPrompt(CeremonyYesNo, create); !strings.Contains(got, "Publish?") || strings.Contains(got, "update") {
		t.Fatalf("create prompt = %q", got)
	}
	if got := ceremonyPrompt(CeremonyTypedPublish, update); !strings.Contains(got, "PUBLISH") {
		t.Fatalf("medium prompt = %q", got)
	}
	if got := ceremonyPrompt(CeremonyTypedDelete, update); !strings.Contains(got, expectedPhrase(CeremonyTypedDelete, update.ID)) {
		t.Fatalf("destructive prompt must name the plan-bound phrase, got %q", got)
	}
}

// TestDeclinedConfirmationIsNeverDestructive keeps the destructive flag honest:
// it travels into the approval grant, so it must describe a confirmation that
// actually happened.
func TestDeclinedConfirmationIsNeverDestructive(t *testing.T) {
	ask := &scriptedPrompter{interactive: true, answer: "no thanks"}
	confirmed, destructive, err := confirmPlan(planWithRisk("high", true), false, ask)
	if err != nil {
		t.Fatal(err)
	}
	if confirmed {
		t.Fatal("plan should not be confirmed")
	}
	if destructive {
		t.Fatal("a declined confirmation must not be reported as destructive")
	}
}

func TestPrompterErrorIsReportedNotSwallowed(t *testing.T) {
	want := errors.New("stdin closed")
	ask := &scriptedPrompter{interactive: true, err: want}
	confirmed, _, err := confirmPlan(planWithRisk("low", false), false, ask)
	if !errors.Is(err, want) {
		t.Fatalf("error = %v, want %v", err, want)
	}
	if confirmed {
		t.Fatal("a failed prompt must not confirm")
	}
}
