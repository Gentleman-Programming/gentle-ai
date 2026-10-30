package cli

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/gentleman-programming/gentle-ai/v4/internal/reviewtransaction"
)

// reviewEscalationFault names which rule an --escalate-item/--escalate-reason
// pair broke, so assess and START share one validation while each keeps its
// own runnable refusal wording.
type reviewEscalationFault int

const (
	reviewEscalationValid reviewEscalationFault = iota
	reviewEscalationUnpaired
	reviewEscalationBadItem
	reviewEscalationBadReason
)

// reviewEscalationFromFlags validates the optional --escalate-item and
// --escalate-reason pair: both or neither, an item of the shared high-risk
// list (reviewAssessHighRiskItems), and a non-empty reason of at most
// reviewtransaction.AgentEscalationReasonMax characters. It returns nil
// without either flag.
func reviewEscalationFromFlags(args []string, item, reason string) (*reviewtransaction.CompactAgentEscalation, reviewEscalationFault) {
	itemGiven := reviewFlagProvided(args, "--escalate-item") || strings.TrimSpace(item) != ""
	reasonGiven := reviewFlagProvided(args, "--escalate-reason") || reason != ""
	if !itemGiven && !reasonGiven {
		return nil, reviewEscalationValid
	}
	if !itemGiven || !reasonGiven {
		return nil, reviewEscalationUnpaired
	}
	number, err := strconv.Atoi(strings.TrimSpace(item))
	if _, known := reviewAssessHighRiskItems[number]; err != nil || !known {
		return nil, reviewEscalationBadItem
	}
	if strings.TrimSpace(reason) == "" || len(reason) > reviewtransaction.AgentEscalationReasonMax {
		return nil, reviewEscalationBadReason
	}
	return &reviewtransaction.CompactAgentEscalation{Item: number, Reason: reason}, reviewEscalationValid
}

// parseReviewStartEscalation is START's form of the assess validation (S14).
func parseReviewStartEscalation(args []string, item, reason string) (*reviewtransaction.CompactAgentEscalation, error) {
	escalation, fault := reviewEscalationFromFlags(args, item, reason)
	switch fault {
	case reviewEscalationUnpaired:
		return nil, errors.New("review start --escalate-item and --escalate-reason must be passed together; rerun `gentle-ai review start --escalate-item <1-6> --escalate-reason <text>`")
	case reviewEscalationBadItem:
		return nil, fmt.Errorf("review start --escalate-item %q must be an integer from 1 to 6 naming a high-risk item; rerun `gentle-ai review start --escalate-item <1-6> --escalate-reason <text>`", item)
	case reviewEscalationBadReason:
		return nil, fmt.Errorf("review start --escalate-reason must be non-empty and at most %d characters; rerun `gentle-ai review start --escalate-item <1-6> --escalate-reason <text>` with a one-line reason", reviewtransaction.AgentEscalationReasonMax)
	}
	return escalation, nil
}

// escalateReviewStartAssessment applies an agent escalation to the
// classifier's assessment exactly as assess does: passive and medium become
// high, a tier is never lowered, and the agent_escalation reason is added. It
// sorts first, so the reasons stay in canonical START order.
func escalateReviewStartAssessment(assessment reviewtransaction.RiskAssessment, escalation *reviewtransaction.CompactAgentEscalation) reviewtransaction.RiskAssessment {
	if escalation == nil {
		return assessment
	}
	assessment.Level, assessment.DominantLens = reviewtransaction.RiskHigh, ""
	assessment.Reasons = append([]reviewtransaction.RiskReason{{
		Code: reviewtransaction.RiskReasonAgentEscalation, Signal: reviewtransaction.SignalAgentEscalation,
	}}, assessment.Reasons...)
	return assessment
}

// reviewEscalationFollowUpArguments renders the escalate words a relayed
// consent answer must repeat, so answering consent never reruns START at the
// classifier's lower tier.
func reviewEscalationFollowUpArguments(escalation *reviewtransaction.CompactAgentEscalation) string {
	if escalation == nil {
		return ""
	}
	return " --escalate-item " + strconv.Itoa(escalation.Item) + " --escalate-reason " + reviewTransitionShellWord(escalation.Reason)
}

// reviewLensSelection is the agent's START --lenses choice and its reason
// (verify-always-rdd-high S8): the 4R lenses pertinent to what it touched and
// how, in canonical lens names.
type reviewLensSelection struct {
	Lenses []string
	Reason string
}

const reviewLensSelectionRerun = "rerun `gentle-ai review start --lenses <risk,resilience,readability,reliability> --lenses-reason <text>`"

// parseReviewStartLensSelection validates the optional --lenses and
// --lenses-reason pair: both or neither, known lens names without repeats, a
// bounded non-empty reason, and never together with --focus. It returns nil
// without either flag, so START keeps the tier default (phase A).
func parseReviewStartLensSelection(args []string, lenses, reason string) (*reviewLensSelection, error) {
	lensesGiven := reviewFlagProvided(args, "--lenses") || strings.TrimSpace(lenses) != ""
	reasonGiven := reviewFlagProvided(args, "--lenses-reason") || reason != ""
	if !lensesGiven && !reasonGiven {
		return nil, nil
	}
	if !lensesGiven || !reasonGiven {
		return nil, errors.New("review start --lenses and --lenses-reason must be passed together; " + reviewLensSelectionRerun)
	}
	if reviewFlagProvided(args, "--focus") {
		return nil, errors.New("review start --lenses replaces --focus; omit --focus and " + reviewLensSelectionRerun)
	}
	seen := map[string]bool{}
	selection := &reviewLensSelection{Reason: reason}
	for _, name := range strings.Split(lenses, ",") {
		lens, ok := reviewtransaction.ReviewLensFor(name)
		if !ok {
			return nil, fmt.Errorf("review start --lenses names unknown lens %q; %s", strings.TrimSpace(name), reviewLensSelectionRerun)
		}
		if seen[lens] {
			return nil, fmt.Errorf("review start --lenses repeats lens %q; %s", lens, reviewLensSelectionRerun)
		}
		seen[lens] = true
		selection.Lenses = append(selection.Lenses, lens)
	}
	selection.Lenses = reviewCanonicalLensOrder(selection.Lenses)
	if err := reviewtransaction.ValidateLensSelectionReason(reason); err != nil {
		return nil, fmt.Errorf("review start --lenses-reason must be non-empty and at most %d characters; %s with a one-line reason", reviewtransaction.LensSelectionReasonMax, reviewLensSelectionRerun)
	}
	return selection, nil
}

// reviewLensSelectionFollowUpArguments renders the selection words a relayed
// consent answer must repeat, so answering consent never reruns START with the
// tier default instead of the agent's lenses.
func reviewLensSelectionFollowUpArguments(selection *reviewLensSelection) string {
	if selection == nil {
		return ""
	}
	return " --lenses " + reviewTransitionShellWord(strings.Join(selection.Lenses, ",")) +
		" --lenses-reason " + reviewTransitionShellWord(selection.Reason)
}

// reviewLensSelectionReason returns the reason START freezes with the
// selection, or "" when the tier default chose the lenses.
func reviewLensSelectionReason(selection *reviewLensSelection) string {
	if selection == nil {
		return ""
	}
	return selection.Reason
}

// reviewCanonicalLensOrder orders selected lenses as the 4R lens set is
// ordered, so a selection renders and freezes the same however it was typed.
func reviewCanonicalLensOrder(lenses []string) []string {
	ordered := make([]string, 0, len(lenses))
	for _, lens := range []string{reviewtransaction.LensRisk, reviewtransaction.LensResilience, reviewtransaction.LensReadability, reviewtransaction.LensReliability} {
		for _, selected := range lenses {
			if selected == lens {
				ordered = append(ordered, lens)
			}
		}
	}
	return ordered
}
