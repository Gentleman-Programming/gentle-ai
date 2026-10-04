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
