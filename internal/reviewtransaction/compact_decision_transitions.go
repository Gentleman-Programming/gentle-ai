package reviewtransaction

// Canonical reference for the seven allowed and four forbidden transitions
// of the v2 decision-required state machine. Cross-reference: engram obs
// #21219. The validateCompactSuccessor switch in compact_store.go is the
// source of truth for actual operational validation; the table here is the
// human-readable documentation and the importable helper that the
// operation arms consult to keep the truth table and the executor in lock
// step.

const (
	// CompactDecideOperation drives edges 2, 3, 4 from StateDecisionRequired.
	CompactDecideOperation = "review/decide"

	// CompactDecisionAdjudicateBatchOperation drives edges 5, 6, 7 from
	// StateDecisionCarryOn. The self-loop is forbidden because the bounded
	// action is single-shot.
	CompactDecisionAdjudicateBatchOperation = "review/decision-adjudicate-batch"
)

// allowedDecisionEdge describes one cell of the truth table.
type allowedDecisionEdge struct {
	From      State
	To        State
	Operation string
}

// allowedDecisionEdges is the canonical table of the seven allowed edges
// from the canonical truth table (engram obs #21219); order is stable so
// the table can be diffed against the truth table without reordering noise.
var allowedDecisionEdges = []allowedDecisionEdge{
	// Edge 1: Reviewing -> DecisionRequired via review/complete-review.
	// PR #3 extends the existing review/complete-review arm to admit this
	// target when the feature flag is enabled.
	{From: StateReviewing, To: StateDecisionRequired, Operation: "review/complete-review"},
	// Edge 2: DecisionRequired -> DecisionRequired (idempotent re-apply).
	{From: StateDecisionRequired, To: StateDecisionRequired, Operation: CompactDecideOperation},
	// Edge 3: DecisionRequired -> Escalated (--decision stop).
	{From: StateDecisionRequired, To: StateEscalated, Operation: CompactDecideOperation},
	// Edge 4: DecisionRequired -> DecisionCarryOn (--decision continue).
	{From: StateDecisionRequired, To: StateDecisionCarryOn, Operation: CompactDecideOperation},
	// Edge 5: DecisionCarryOn -> Validating (unresolved remain).
	{From: StateDecisionCarryOn, To: StateValidating, Operation: CompactDecisionAdjudicateBatchOperation},
	// Edge 6: DecisionCarryOn -> Approved (all severe resolved).
	{From: StateDecisionCarryOn, To: StateApproved, Operation: CompactDecisionAdjudicateBatchOperation},
	// Edge 7: DecisionCarryOn -> Escalated (adjudicator unavailable).
	{From: StateDecisionCarryOn, To: StateEscalated, Operation: CompactDecisionAdjudicateBatchOperation},
}

// AllowedDecisionTransitions returns a copy of the truth table for callers
// that need to enumerate the allowed edges (auditors, the truth-table
// regression test, the next-transition resolver). The slice is defensive:
// callers must not mutate the package-level table.
func AllowedDecisionTransitions() []allowedDecisionEdge {
	out := make([]allowedDecisionEdge, len(allowedDecisionEdges))
	copy(out, allowedDecisionEdges)
	return out
}

// legalDecisionStateTransition is the helper consulted by the new
// validateCompactSuccessor arms. It returns true when the (from, to) pair
// is allowed for the given operation string.
func legalDecisionStateTransition(from, to State, operation string) bool {
	for _, edge := range allowedDecisionEdges {
		if edge.From == from && edge.To == to && edge.Operation == operation {
			return true
		}
	}
	return false
}
