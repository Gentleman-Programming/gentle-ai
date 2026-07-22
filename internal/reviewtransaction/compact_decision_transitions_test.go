package reviewtransaction

import (
	"errors"
	"strings"
	"testing"
)

// TestLegalStateTransition_AllowedEdges covers the seven allowed edges from
// the canonical truth table (engram obs #21219). Every allowed edge must
// be accepted by validateCompactSuccessor for the correct operation string.
func TestLegalStateTransition_AllowedEdges(t *testing.T) {
	allowed := []struct {
		name      string
		previous  State
		next      State
		operation string
	}{
		{name: "reviewing_to_decision_required", previous: StateReviewing, next: StateDecisionRequired, operation: "review/complete-review"},
		{name: "decision_required_to_decision_required", previous: StateDecisionRequired, next: StateDecisionRequired, operation: "review/decide"},
		{name: "decision_required_to_escalated", previous: StateDecisionRequired, next: StateEscalated, operation: "review/decide"},
		{name: "decision_required_to_decision_carry_on", previous: StateDecisionRequired, next: StateDecisionCarryOn, operation: "review/decide"},
		{name: "decision_carry_on_to_validating", previous: StateDecisionCarryOn, next: StateValidating, operation: "review/decision-adjudicate-batch"},
		{name: "decision_carry_on_to_approved", previous: StateDecisionCarryOn, next: StateApproved, operation: "review/decision-adjudicate-batch"},
		{name: "decision_carry_on_to_escalated", previous: StateDecisionCarryOn, next: StateEscalated, operation: "review/decision-adjudicate-batch"},
	}
	for _, edge := range allowed {
		t.Run(edge.name, func(t *testing.T) {
			previous := newDecisionEdgeFixture(edge.previous)
			next := previous
			next.State = edge.next
			if err := validateCompactSuccessor("sha256:edge", previous, next, edge.operation); err != nil {
				t.Fatalf("validateCompactSuccessor rejected allowed edge %s (%s -> %s via %s): %v",
					edge.name, edge.previous, edge.next, edge.operation, err)
			}
		})
	}
}

// TestLegalStateTransition_ForbiddenEdges covers the four edges that the
// canonical truth table explicitly forbids. A regression that accidentally
// admits one of these edges would orphan the lineage (the #1433 defect).
func TestLegalStateTransition_ForbiddenEdges(t *testing.T) {
	forbidden := []struct {
		name      string
		previous  State
		next      State
		operation string
	}{
		{name: "decision_required_to_approved", previous: StateDecisionRequired, next: StateApproved, operation: "review/decide"},
		{name: "decision_required_to_validating", previous: StateDecisionRequired, next: StateValidating, operation: "review/decide"},
		{name: "decision_carry_on_to_decision_required", previous: StateDecisionCarryOn, next: StateDecisionRequired, operation: "review/decision-adjudicate-batch"},
		{name: "decision_carry_on_self_loop", previous: StateDecisionCarryOn, next: StateDecisionCarryOn, operation: "review/decision-adjudicate-batch"},
	}
	for _, edge := range forbidden {
		t.Run(edge.name, func(t *testing.T) {
			previous := newDecisionEdgeFixture(edge.previous)
			next := previous
			next.State = edge.next
			err := validateCompactSuccessor("sha256:edge", previous, next, edge.operation)
			if err == nil {
				t.Fatalf("admitted forbidden edge %s (%s -> %s via %s); want ErrInvalidSuccessor",
					edge.name, edge.previous, edge.next, edge.operation)
			}
			if !errors.Is(err, ErrInvalidSuccessor) {
				t.Fatalf("rejected %s with %v, want ErrInvalidSuccessor", edge.name, err)
			}
		})
	}
}

// TestLegalStateTransition_UnsupportedOperationRejected ensures that an
// unrecognized operation string falls through to the default arm and is
// rejected with the "unsupported compact operation" error. The regression
// guard prevents the #1433 defect (a missing edge that silently orphans
// the lineage) from being re-introduced.
func TestLegalStateTransition_UnsupportedOperationRejected(t *testing.T) {
	previous := newDecisionEdgeFixture(StateReviewing)
	next := previous
	next.State = StateDecisionRequired
	err := validateCompactSuccessor("sha256:edge", previous, next, "review/decision-typo")
	if err == nil {
		t.Fatalf("unrecognized operation string admitted; want ErrInvalidSuccessor")
	}
	if !errors.Is(err, ErrInvalidSuccessor) {
		t.Fatalf("rejection error = %v, want ErrInvalidSuccessor", err)
	}
	if !strings.Contains(err.Error(), "unsupported compact operation") {
		t.Fatalf("rejection error = %q, want one mentioning unsupported operation", err.Error())
	}
}

// TestLegalStateTransition_TruthTableEnumeratesEveryAllowedEdge is the
// regression guard referenced in the canonical truth table (engram obs
// #21219): the package-level truth table must enumerate every allowed edge
// or the executor would silently reject them.
func TestLegalStateTransition_TruthTableEnumeratesEveryAllowedEdge(t *testing.T) {
	rows := AllowedDecisionTransitions()
	if len(rows) != 7 {
		t.Fatalf("allowedDecisionEdges length = %d, want 7 (edges 1..7 from obs #21219)", len(rows))
	}
}

// newDecisionEdgeFixture builds a minimal CompactState that satisfies
// validateCompactSuccessor's scope/tier/budget immutability guard. FixDeltaHash
// is set to EmptyFixDeltaHash so the review/complete-review arm (edge 1)
// accepts the successor; the absence of any other mutable field is what makes
// the "unrelated state" guard also pass.
func newDecisionEdgeFixture(state State) CompactState {
	return CompactState{
		State:                state,
		LineageID:            "edge-fixture",
		Generation:           1,
		PolicyHash:           "sha256:" + strings.Repeat("0", 64),
		RiskLevel:            RiskHigh,
		SelectedLenses:       []string{"review-readability"},
		OriginalChangedLines: 1,
		CorrectionBudget:     50,
		FixDeltaHash:         EmptyFixDeltaHash,
	}
}
