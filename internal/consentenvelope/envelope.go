// Package consentenvelope carries the generic core of a typed blocking
// consent question: the headline/reason/value triple that states WHY input is
// required, the evidence list, exactly two token choices each carrying a
// human label, the effect of choosing it, and one runnable follow-up
// invocation, plus the documented off path outside the choice set.
//
// It was extracted from the review consent contract (#2554, S4a of #2540) so
// the SDD edit-authority consent question reuses the same discipline instead
// of hand-copying it. The package owns only the COMPLETENESS half of
// validation; every consumer keeps its own identity half (schema, operation,
// frozen target, invocation shape) next to its builder. The JSON field names
// on Choice and OffPath are shipped contract bytes: the review consent
// fixtures pin them byte-for-byte, so they never change here.
package consentenvelope

import (
	"errors"
	"fmt"
)

// Choice is one allowed answer: its token, the human label the interactive
// prompt uses, what choosing it does, and the exact runnable follow-up
// invocation scoped to the one blocked subject.
type Choice struct {
	Answer     string `json:"answer"`
	Label      string `json:"label"`
	Effect     string `json:"effect"`
	Invocation string `json:"invocation"`
}

// OffPath names the documented deliberate exit outside the choice set. It
// exists so the envelope never hides the alternative, and so taking it costs
// more than answering a relayed question in a hurry.
type OffPath struct {
	Note    string `json:"note"`
	Command string `json:"command"`
}

// Core is the generic completeness view of one blocking consent envelope.
// Consumers project their typed result into it and delegate the completeness
// half of Validate here; the zero value is deliberately invalid.
type Core struct {
	Headline string
	Reason   string
	Value    string
	// Evidence may legitimately be empty, but never nil: an empty list must
	// encode as [] rather than null, so nil is a construction bug.
	Evidence []string
	Choices  []Choice
	OffPath  OffPath
}

// ValidateCompleteness enforces the generic half of the consent-envelope
// contract: the envelope states why input is required, offers exactly the
// grant and decline tokens in that order, and every choice is answerable and
// runnable. Identity validation stays with each consumer.
func (core Core) ValidateCompleteness(grantedAnswer, declinedAnswer string) error {
	if core.Headline == "" || core.Reason == "" || core.Value == "" || core.Evidence == nil {
		return errors.New("consent question must state why input is required") // refusal:by-design world-action: the envelope is built and validated by its producer; the exit is a code fix, not a command
	}
	if len(core.Choices) != 2 ||
		core.Choices[0].Answer != grantedAnswer ||
		core.Choices[1].Answer != declinedAnswer {
		return errors.New("consent question requires exactly the granted and declined choices") // refusal:by-design world-action: the envelope is built and validated by its producer; the exit is a code fix, not a command
	}
	for _, choice := range core.Choices {
		if choice.Label == "" || choice.Effect == "" {
			return fmt.Errorf("consent choice %q is incomplete", choice.Answer) // refusal:by-design world-action: the envelope is built and validated by its producer; the exit is a code fix, not a command
		}
		if choice.Invocation == "" {
			return fmt.Errorf("consent choice %q does not name a runnable invocation", choice.Answer) // refusal:by-design world-action: the envelope is built and validated by its producer; the exit is a code fix, not a command
		}
	}
	if core.OffPath.Note == "" || core.OffPath.Command == "" {
		return errors.New("consent question must document the deliberate off path") // refusal:by-design world-action: the envelope is built and validated by its producer; the exit is a code fix, not a command
	}
	return nil
}
