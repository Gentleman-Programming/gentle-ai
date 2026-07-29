package cli

import (
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
)

// Coming back after switching reviews off, the operator sees "scope-changed"
// and a recovery command. Both are accurate and neither says what happened:
// that this candidate carries work no receipt covers. The count is already
// derived and frozen in the diagnostics, so naming the situation costs nothing
// and turns a state name into something an operator recognises.
func TestScopeChangedDenialNamesTheUncoveredScopeBeforeTheMechanism(t *testing.T) {
	err := ReviewGateDeniedError{
		Result: reviewtransaction.GateScopeChanged,
		Context: reviewtransaction.GateContext{
			ScopeChange: &reviewtransaction.GateScopeChangeDiagnostics{
				DifferingPathCount:     2,
				RecoveryOperation:      "review.recover",
				RecoveryRequiredInputs: []string{"predecessor_lineage_id"},
			},
		},
	}
	message := err.Error()

	if !strings.Contains(message, "2 path") {
		t.Errorf("denial never says how much is uncovered.\n got: %s", message)
	}
	if !strings.Contains(message, "no terminal receipt covers") {
		t.Errorf("denial names the mechanism but not the situation.\n got: %s", message)
	}
	if !strings.Contains(message, "gentle-ai review recover") {
		t.Errorf("denial lost its recovery.\n got: %s", message)
	}
	assertNamesNoDottedOperation(t, message)
}

// With no derived count there is nothing truthful to say about scope, and a
// fabricated "0 paths" would be worse than silence.
func TestScopeChangedDenialSaysNothingAboutScopeWithoutADerivedCount(t *testing.T) {
	err := ReviewGateDeniedError{
		Result: reviewtransaction.GateScopeChanged,
		Context: reviewtransaction.GateContext{
			ScopeChange: &reviewtransaction.GateScopeChangeDiagnostics{
				RecoveryOperation: "review.recover",
			},
		},
	}
	if message := err.Error(); strings.Contains(message, "path") {
		t.Fatalf("denial invented a scope it never derived: %s", message)
	}
}
