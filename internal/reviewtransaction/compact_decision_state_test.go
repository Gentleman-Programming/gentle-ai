package reviewtransaction

import (
	"errors"
	"strings"
	"testing"
)

// TestStateDecisionRequired_IsConstant locks the literal value of StateDecisionRequired.
// The literal is the externally-visible contract used by `review inspect-authority`
// and the journal; mutating it would silently shift every previously persisted
// lineage's state.
func TestStateDecisionRequired_IsConstant(t *testing.T) {
	if string(StateDecisionRequired) != "decision_required" {
		t.Fatalf("StateDecisionRequired literal = %q, want %q", string(StateDecisionRequired), "decision_required")
	}
}

// TestStateDecisionCarryOn_IsConstant locks the literal value of StateDecisionCarryOn.
func TestStateDecisionCarryOn_IsConstant(t *testing.T) {
	if string(StateDecisionCarryOn) != "decision_carry_on" {
		t.Fatalf("StateDecisionCarryOn literal = %q, want %q", string(StateDecisionCarryOn), "decision_carry_on")
	}
}

// TestCompactStateValidate_DecisionStatesAreAccepted proves that the compact
// state machine accepts the two new states without extra validation (they
// are pause-only no-op cases, mirroring `case StateEscalated:`).
func TestCompactStateValidate_DecisionStatesAreAccepted(t *testing.T) {
	repo := initSnapshotRepo(t)
	base := newCompactTestState(t, repo, "decision-state-validate")
	for _, testCase := range []struct {
		name  string
		state State
	}{
		{name: "decision_required", state: StateDecisionRequired},
		{name: "decision_carry_on", state: StateDecisionCarryOn},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			probe := base
			probe.State = testCase.state
			if err := probe.Validate(); err != nil {
				t.Fatalf("Validate() rejected %s: %v", testCase.name, err)
			}
		})
	}
}

// TestCompactStateValidate_UnknownStateRejected is the safety regression: a
// state literal that is not in the validate switch must be rejected with a
// clear, formatted error mentioning the bad literal.
func TestCompactStateValidate_UnknownStateRejected(t *testing.T) {
	repo := initSnapshotRepo(t)
	state := newCompactTestState(t, repo, "unknown-state-validate")
	state.State = State("utterly-unknown")
	err := state.Validate()
	if err == nil {
		t.Fatalf("Validate() accepted unknown state, want error")
	}
	if !strings.Contains(err.Error(), "invalid compact review state") {
		t.Fatalf("Validate() error = %q, want one containing %q", err.Error(), "invalid compact review state")
	}
	if !strings.Contains(err.Error(), "utterly-unknown") {
		t.Fatalf("Validate() error = %q, want it to mention the bad literal", err.Error())
	}
}

// TestDecisionPayloadErrors_AreDistinctSentinels pins the six typed errors
// declared in compact_decision_state.go as distinct sentinels so callers
// using errors.Is can trust each one maps to a single decision failure mode.
func TestDecisionPayloadErrors_AreDistinctSentinels(t *testing.T) {
	sentinels := map[string]error{
		"ErrAuthorityRevisionMismatch": ErrAuthorityRevisionMismatch,
		"ErrIllegalStateForDecide":     ErrIllegalStateForDecide,
		"ErrDecisionConflict":          ErrDecisionConflict,
		"ErrInvalidDecisionFlag":       ErrInvalidDecisionFlag,
		"ErrIllegalStateForAdjudicate": ErrIllegalStateForAdjudicate,
		"ErrAdjudicatorUnavailable":    ErrAdjudicatorUnavailable,
	}
	seen := map[string]string{}
	for name, err := range sentinels {
		if err == nil {
			t.Fatalf("nil sentinel for %s", name)
		}
		if err.Error() == "" {
			t.Fatalf("empty message for %s", name)
		}
		for other, otherErr := range sentinels {
			if other == name {
				continue
			}
			if errors.Is(err, otherErr) {
				t.Fatalf("sentinel %s collides with %s", name, other)
			}
		}
		if previous, ok := seen[err.Error()]; ok {
			t.Fatalf("sentinels %s and %s share message %q", previous, name, err.Error())
		}
		seen[err.Error()] = name
	}
}
