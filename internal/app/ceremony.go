package app

import (
	"fmt"
	"strings"

	"gitmake/internal/planstore"
)

// Confirmation friction is a safety contract, not a UX preference. STABILITY.md
// promises that `--yes` accepts low-risk plans only, that a medium-risk plan
// requires a typed PUBLISH, and that a destructive one requires a phrase bound
// to that specific plan. Until now that promise lived entirely inside one
// function that printed to stdout and read from a terminal, so nothing tested
// it: the rules and the conversation were the same code.
//
// The rules are separated here as pure values, and the conversation is reached
// through prompter. What GitMake demands for a given risk is now decidable
// without a terminal, and provable in a table.

// Ceremony is how much a human has to do before a plan may be applied.
type Ceremony int

const (
	// CeremonyYesNo is an ordinary [Y/n] question. Only this level may be
	// satisfied by --yes.
	CeremonyYesNo Ceremony = iota
	// CeremonyTypedPublish requires the word PUBLISH, typed by a person.
	CeremonyTypedPublish
	// CeremonyTypedDelete requires a phrase carrying the plan's own suffix, so
	// a confirmation cannot be copied between plans.
	CeremonyTypedDelete
)

func (c Ceremony) String() string {
	switch c {
	case CeremonyTypedPublish:
		return "typed-publish"
	case CeremonyTypedDelete:
		return "typed-delete"
	default:
		return "yes-no"
	}
}

// RequiresHuman reports whether this level must be answered at a terminal.
// --yes cannot stand in for a person here, and neither can an agent.
func (c Ceremony) RequiresHuman() bool { return c != CeremonyYesNo }

// requiredCeremony maps a reviewed risk to the friction GitMake demands.
// An unset level is treated as low, matching the plan schema's default.
func requiredCeremony(risk planstore.Risk) Ceremony {
	level := strings.ToLower(strings.TrimSpace(risk.Level))
	switch {
	case risk.Destructive || level == "high":
		return CeremonyTypedDelete
	case level == "medium":
		return CeremonyTypedPublish
	default:
		return CeremonyYesNo
	}
}

// expectedPhrase is the exact answer that confirms a typed ceremony, or "" when
// the ceremony is not typed.
func expectedPhrase(c Ceremony, planID string) string {
	switch c {
	case CeremonyTypedDelete:
		return "DELETE-" + confirmationCode(planID)
	case CeremonyTypedPublish:
		return "PUBLISH"
	default:
		return ""
	}
}

// ceremonyPrompt is what the user is asked.
func ceremonyPrompt(c Ceremony, p planstore.Plan) string {
	switch c {
	case CeremonyTypedDelete:
		return fmt.Sprintf("\nHigh-risk change. Type %s to confirm: ", expectedPhrase(c, p.ID))
	case CeremonyTypedPublish:
		return "\nRisk needs review. Type PUBLISH to continue: "
	default:
		if p.Mode == "UPDATE" {
			return "\nPublish update? [Y/n]: "
		}
		return "\nPublish? [Y/n]: "
	}
}

// nonInteractiveError explains why a plan cannot be confirmed here.
func nonInteractiveError(c Ceremony, planID string) error {
	switch c {
	case CeremonyTypedDelete:
		return fmt.Errorf("high-risk plan requires interactive confirmation; review plan %s in a terminal", planID)
	case CeremonyTypedPublish:
		return fmt.Errorf("medium-risk plan requires interactive confirmation; review plan %s in a terminal", planID)
	default:
		return nil
	}
}

// answerConfirms reports whether a typed answer satisfies the ceremony.
// The delete phrase is compared exactly: it carries the plan id, and accepting
// a near miss would defeat the point of binding it to one plan.
func answerConfirms(c Ceremony, planID, answer string) bool {
	answer = strings.TrimSpace(answer)
	switch c {
	case CeremonyTypedDelete:
		return answer == expectedPhrase(c, planID)
	case CeremonyTypedPublish:
		return strings.EqualFold(answer, "PUBLISH")
	default:
		answer = strings.ToLower(answer)
		return answer == "" || answer == "y" || answer == "yes"
	}
}

// prompter is the conversation with the person at the keyboard. Tests supply
// their own; production uses the terminal.
type prompter interface {
	// Interactive reports whether a human can actually answer.
	Interactive() bool
	// Ask prints prompt and returns the typed line.
	Ask(prompt string) (string, error)
}

type terminalPrompter struct{}

func (terminalPrompter) Interactive() bool { return stdinInteractive() }

func (terminalPrompter) Ask(prompt string) (string, error) {
	fmt.Print(prompt)
	return readSimpleLine()
}

// confirmPlan applies the ceremony rules to a reviewed plan.
//
// It reports whether the plan was confirmed and whether the confirmation was a
// destructive one, so the caller can carry that distinction into the approval
// grant.
func confirmPlan(p planstore.Plan, assumeYes bool, ask prompter) (confirmed bool, destructive bool, err error) {
	ceremony := requiredCeremony(p.Risk)

	if ceremony.RequiresHuman() && !ask.Interactive() {
		return false, false, nonInteractiveError(ceremony, p.ID)
	}
	// --yes is deliberately powerless above the lowest level.
	if !ceremony.RequiresHuman() && assumeYes {
		return true, false, nil
	}

	answer, err := ask.Ask(ceremonyPrompt(ceremony, p))
	if err != nil {
		return false, false, err
	}
	if !answerConfirms(ceremony, p.ID, answer) {
		return false, false, nil
	}
	// The destructive flag travels into the approval grant, so it is reported
	// only for a confirmation that actually happened.
	return true, ceremony == CeremonyTypedDelete, nil
}
