package reviewtransaction

import "errors"

// Typed errors for the v2 decision-required state machine. The 6 sentinels
// below are the single source of truth for every caller that needs to
// classify a failure mode with errors.Is — consumers and the CLI both rely
// on stable sentinels so retries and rollbacks can be distinguished from
// genuine user errors.

var (
	// ErrAuthorityRevisionMismatch: --expected-revision does not match the
	// current CompactStore authority revision (CAS failure).
	ErrAuthorityRevisionMismatch = errors.New("review/decide: authority revision does not match expected")

	// ErrIllegalStateForDecide: lineage is not in StateDecisionRequired when
	// `review decide` is invoked.
	ErrIllegalStateForDecide = errors.New("review/decide: lineage is not in decision_required state")

	// ErrDecisionConflict: replay with a conflicting decision (e.g., first
	// --decision stop then --decision continue for the same revision).
	ErrDecisionConflict = errors.New("review/decide: conflicting decision already recorded")

	// ErrInvalidDecisionFlag: --decision value is neither "continue" nor "stop".
	ErrInvalidDecisionFlag = errors.New("review/decide: --decision must be continue or stop")

	// ErrIllegalStateForAdjudicate: review/decision-adjudicate-batch invoked
	// on a lineage that is not in StateDecisionCarryOn.
	ErrIllegalStateForAdjudicate = errors.New("review/decision-adjudicate-batch: lineage is not in decision_carry_on state")

	// ErrAdjudicatorUnavailable: the bounded adjudication invocation failed
	// with a retriable provider error; the lineage remains in StateDecisionCarryOn.
	ErrAdjudicatorUnavailable = errors.New("review/decision-adjudicate-batch: adjudicator provider unavailable")
)
