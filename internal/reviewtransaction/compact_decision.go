package reviewtransaction

import (
	"errors"
	"fmt"
	"sort"
	"strings"
)

// The decision_required pause (#1380 WU1): when a completed compact review
// leaves unresolved findings, the engine pauses for a human decision instead
// of escalating on its own. This file owns the decision vocabulary, the frozen
// pause evidence, and the transition-time cause attribution shared with the
// escalated rendering.

const (
	// StateDecisionRequired pauses a completed review whose evidence is
	// inconclusive. It is an active, non-terminal authority whose only
	// sanctioned exit is `review decide` (continue|stop).
	StateDecisionRequired State = "decision_required"

	// CompactDecisionPause is the engine-authored decision entry written when
	// a review completion pauses for a decision. CompactDecisionContinue
	// resumes the reviewing authority; CompactDecisionStop escalates it. The
	// vocabulary is closed.
	CompactDecisionPause    = "pause"
	CompactDecisionContinue = "continue"
	CompactDecisionStop     = "stop"

	// CompactDecisionReasonEvidenceInconclusive is the only reason a review
	// completion pauses today: admitted lens evidence that could not resolve
	// every finding.
	CompactDecisionReasonEvidenceInconclusive = "evidence_inconclusive"

	// CompactDecisionSystemActor attributes the engine-authored pause entry to
	// the operation that wrote it, matching the operation string the
	// committing Replace journals in the store trace.
	CompactDecisionSystemActor = "review/complete-review"
)

// CompactDecisionEvidence is the frozen question written at the pause
// transition. Every field is derived from the admitted review view at that
// transition, so the escalated rendering after a stop classifies the same
// cause the pause did.
type CompactDecisionEvidence struct {
	Reason            string   `json:"reason"`
	Cause             string   `json:"cause"`
	FindingIDs        []string `json:"finding_ids"`
	AttemptedEvidence []string `json:"attempted_evidence"`
	MissingEvidence   []string `json:"missing_evidence"`
	Recommendation    string   `json:"recommendation"`
	NextAction        string   `json:"next_action"`
	BoundedCost       string   `json:"bounded_cost"`
}

// CompactDecisionEntry is one journaled decision. The pause entry is authored
// by the engine and attributed to the authoring operation; decide entries
// carry the human actor and reason that made the call, bound to the exact
// store revision the decision was issued against.
type CompactDecisionEntry struct {
	Decision string `json:"decision"`
	Actor    string `json:"actor"`
	Reason   string `json:"reason"`
	Revision string `json:"revision"`
}

func validCompactDecisionVocabulary(value string) bool {
	return value == CompactDecisionPause || value == CompactDecisionContinue || value == CompactDecisionStop
}

// deriveCompactDecisionEvidence freezes the pause question from the admitted
// review view at the transition. The cause classification deliberately shares
// one helper with the escalated rendering so both can never disagree.
func deriveCompactDecisionEvidence(view CompactReviewView) *CompactDecisionEvidence {
	decision := &CompactDecisionEvidence{Reason: CompactDecisionReasonEvidenceInconclusive}
	cause, unresolvedIDs, refuterOutcomes, found := classifyCompactUnresolvedFindings(view)
	if !found {
		decision.Cause = "unresolved_severe_findings"
		decision.FindingIDs = append([]string{}, view.FixFindingIDs...)
		decision.AttemptedEvidence = []string{"admitted review lens results"}
		decision.MissingEvidence = []string{"conclusive outcomes for the severe findings"}
	} else {
		decision.Cause = cause
		decision.FindingIDs = unresolvedIDs
		attempted := []string{}
		missing := []string{}
		fixSet := make(map[string]struct{}, len(view.FixFindingIDs))
		for _, id := range view.FixFindingIDs {
			fixSet[id] = struct{}{}
		}
		for _, id := range unresolvedIDs {
			if _, found := view.Classifications[id]; found {
				attempted = append(attempted, "classification:"+id)
			}
			switch cause {
			case "missing_refuter_outcome":
				if _, refuted := refuterOutcomesByID(refuterOutcomes)[id]; !refuted {
					missing = append(missing, "refuter_outcome:"+id)
				}
			case "insufficient_evidence":
				missing = append(missing, "concrete_evidence:"+id)
			case "unknown_causality":
				missing = append(missing, "causal_disposition:"+id)
			default:
				if _, fixing := fixSet[id]; !fixing {
					missing = append(missing, "conclusive_outcome:"+id)
				}
			}
			if _, refuted := refuterOutcomesByID(refuterOutcomes)[id]; refuted {
				attempted = append(attempted, "refuter_outcome:"+id)
			}
		}
		decision.AttemptedEvidence = attempted
		decision.MissingEvidence = missing
	}
	if len(decision.AttemptedEvidence) == 0 {
		decision.AttemptedEvidence = []string{"admitted review lens results"}
	}
	if len(decision.MissingEvidence) == 0 {
		decision.MissingEvidence = []string{"conclusive outcomes for the unresolved findings"}
	}
	decision.Recommendation = "A human decides whether this review continues or stops."
	decision.NextAction = "review decide (continue|stop) with the current lineage and expected revision"
	decision.BoundedCost = "one decide invocation; no reviewer recapture and no additional reviewer work"
	return decision
}

func refuterOutcomesByID(outcomes []EvidenceResult) map[string]EvidenceResult {
	byID := make(map[string]EvidenceResult, len(outcomes))
	for _, outcome := range outcomes {
		byID[outcome.FindingID] = outcome
	}
	return byID
}

// CompactDecisionQuestionSchema is the typed, runnable decision question the
// native STATUS projection carries for a paused decision_required authority.
const CompactDecisionQuestionSchema = "gentle-ai.review-decision/v1"

// CompactDecisionChoice is one allowed answer with its exact runnable decide
// invocation bound to the current expected revision.
type CompactDecisionChoice struct {
	Answer     string `json:"answer"`
	Label      string `json:"label"`
	Effect     string `json:"effect"`
	Invocation string `json:"invocation"`
}

// CompactDecisionQuestion is the core-owned decision question: what paused,
// the frozen evidence, and the two complete choices with their runnable
// invocations. The native STATUS projection carries it; the negotiated
// envelopes stay unchanged (their stop rows name the decide command).
type CompactDecisionQuestion struct {
	Schema           string                   `json:"schema"`
	LineageID        string                   `json:"lineage_id"`
	ExpectedRevision string                   `json:"expected_revision"`
	State            State                    `json:"state"`
	Headline         string                   `json:"headline"`
	Reason           string                   `json:"reason"`
	Evidence         *CompactDecisionEvidence `json:"evidence"`
	Choices          []CompactDecisionChoice  `json:"choices"`
}

// DecisionQuestion projects the typed question for a paused authority and
// returns nil for every other state, mirroring EscalationEvidence's coupling.
func (state CompactState) DecisionQuestion(revision string) *CompactDecisionQuestion {
	if state.State != StateDecisionRequired || state.Decision == nil {
		return nil
	}
	return &CompactDecisionQuestion{
		Schema: CompactDecisionQuestionSchema, LineageID: state.LineageID, ExpectedRevision: revision,
		State:    state.State,
		Headline: "This review needs a human decision.",
		Reason:   "The admitted review evidence could not resolve every finding, so the engine paused instead of escalating on its own.",
		Evidence: state.Decision,
		Choices: []CompactDecisionChoice{
			{
				Answer: CompactDecisionContinue, Label: "Continue the review",
				Effect:     "Resumes this lineage as reviewing with the answered question cleared; the admitted captures stay bound.",
				Invocation: compactDecisionInvocation(state.LineageID, revision, CompactDecisionContinue),
			},
			{
				Answer: CompactDecisionStop, Label: "Stop this review",
				Effect:     "Escalates this lineage with the frozen question and decision journal retained; delivery then follows ordinary repository policy.",
				Invocation: compactDecisionInvocation(state.LineageID, revision, CompactDecisionStop),
			},
		},
	}
}

func compactDecisionInvocation(lineage, revision, decision string) string {
	return "gentle-ai review decide --cwd <repo> --lineage " + lineage + " --expected-revision " + revision +
		" --decision " + decision + " --actor <actor> --reason <reason>"
}

// Validate enforces the identity half of the decision-question contract; the
// shape mirrors the shared consent-envelope completeness discipline.
func (question *CompactDecisionQuestion) Validate() error {
	if question == nil {
		// refusal:by-design operator-knowledge: the question is projected by the engine itself; a nil projection is a caller bug, not an operator input
		return errors.New("decision question is absent")
	}
	if question.Schema != CompactDecisionQuestionSchema {
		// refusal:by-design world-action: the schema identity is provider-owned contract bytes; only a provider code fix can change it
		return errors.New("unsupported decision question schema")
	}
	if validateLineageID(question.LineageID) != nil || !validSHA256(question.ExpectedRevision) || question.State != StateDecisionRequired {
		// refusal:by-design world-action: the binding fields are projected from the authority itself; a mismatch requires a provider code fix
		return errors.New("decision question binding is incomplete")
	}
	if strings.TrimSpace(question.Headline) == "" || strings.TrimSpace(question.Reason) == "" {
		// refusal:by-design world-action: the envelope is built and validated by its producer; the exit is a code fix, not a command
		return errors.New("decision question must state why input is required")
	}
	if question.Evidence == nil {
		// refusal:by-design world-action: the evidence block is projected from the frozen pause; its absence requires a provider code fix
		return errors.New("decision question must carry its frozen evidence")
	}
	if len(question.Choices) != 2 || question.Choices[0].Answer != CompactDecisionContinue || question.Choices[1].Answer != CompactDecisionStop {
		// refusal:by-design world-action: the choice set is the closed continue/stop vocabulary; only a provider code fix can change it
		return errors.New("decision question requires exactly the continue and stop choices")
	}
	for _, choice := range question.Choices {
		if strings.TrimSpace(choice.Label) == "" || strings.TrimSpace(choice.Effect) == "" {
			// refusal:by-design world-action: labels and effects are provider-authored contract text; the exit is a code fix, not a command
			return fmt.Errorf("decision choice %q is incomplete", choice.Answer)
		}
		if choice.Invocation != compactDecisionInvocation(question.LineageID, question.ExpectedRevision, choice.Answer) {
			// refusal:by-design world-action: the invocation is derived from the authority binding; a drift requires a provider code fix
			return fmt.Errorf("decision choice %q does not carry its runnable decide invocation", choice.Answer)
		}
	}
	return nil
}

// classifyCompactUnresolvedFindings derives the canonical escalation cause and
// evidence for the unresolved inconclusive findings of a completed review.
// found reports whether any unresolved finding exists; the cause precedence
// (unknown_causality, insufficient_evidence, missing_refuter_outcome) matches
// the escalated rendering exactly.
func classifyCompactUnresolvedFindings(view CompactReviewView) (cause string, unresolvedIDs []string, refuterOutcomes []EvidenceResult, found bool) {
	unresolvedIDs = []string{}
	cause = "unresolved_severe_findings"
	hasUnknownCausality := false
	hasInsufficientEvidence := false
	hasMissingRefuter := false

	refuterMap := make(map[string]EvidenceResult, len(view.RefuterOutcomes))
	for _, r := range view.RefuterOutcomes {
		refuterMap[r.FindingID] = r
	}
	fixSet := make(map[string]struct{}, len(view.FixFindingIDs))
	for _, id := range view.FixFindingIDs {
		fixSet[id] = struct{}{}
	}

	for id, outcome := range view.Outcomes {
		if outcome != OutcomeInconclusive {
			continue
		}
		if _, fixing := fixSet[id]; fixing {
			continue
		}
		unresolvedIDs = append(unresolvedIDs, id)
		if class, found := view.Classifications[id]; found {
			if class.Causality == CausalUnknown {
				hasUnknownCausality = true
			}
			if class.Class == EvidenceInsufficient {
				hasInsufficientEvidence = true
			}
			if class.Class == EvidenceInferential {
				if _, inRefuter := refuterMap[id]; !inRefuter {
					hasMissingRefuter = true
				}
			}
		}
	}
	if len(unresolvedIDs) == 0 {
		return "", nil, nil, false
	}
	sort.Strings(unresolvedIDs)
	if hasUnknownCausality {
		cause = "unknown_causality"
	} else if hasInsufficientEvidence {
		cause = "insufficient_evidence"
	} else if hasMissingRefuter {
		cause = "missing_refuter_outcome"
	}
	var refuter []EvidenceResult
	if len(view.RefuterOutcomes) > 0 {
		refuter = append([]EvidenceResult(nil), view.RefuterOutcomes...)
	}
	return cause, unresolvedIDs, refuter, true
}

// validateCompactDecisionHistory enforces the decision journal contract:
// entries use the closed vocabulary, the engine-authored pause entries carry
// the system attribution, decide entries carry a human actor and reason, and
// the epoch never trails the journal it counts.
func validateCompactDecisionHistory(epoch int, history []CompactDecisionEntry) error {
	for index, entry := range history {
		if !validCompactDecisionVocabulary(entry.Decision) {
			return errors.New("compact decision history entry is outside the closed vocabulary") // refusal:by-design world-action: a forged journal entry requires authority replacement, not an operator command
		}
		if strings.TrimSpace(entry.Actor) == "" || strings.TrimSpace(entry.Reason) == "" {
			return errors.New("compact decision history entry lacks its actor or reason") // refusal:by-design world-action: an unattributed decision cannot be audited
		}
		if entry.Decision == CompactDecisionPause {
			if entry.Actor != CompactDecisionSystemActor || entry.Reason != CompactDecisionReasonEvidenceInconclusive {
				return errors.New("compact pause entries are engine-authored") // refusal:by-design world-action: a hand-written pause entry would misattribute the engine's decision
			}
		}
		if index == 0 && entry.Decision != CompactDecisionPause {
			return errors.New("compact decision history must begin with the engine pause") // refusal:by-design world-action: a decision journal that starts mid-story cannot be audited
		}
	}
	if epoch < len(history) {
		return errors.New("compact decision epoch must count the journaled decisions") // refusal:by-design world-action: contradictory persisted accounting requires code or storage repair
	}
	return nil
}
