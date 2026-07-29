package cli

import (
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
)

// TestReviewGateDeniedBaseMismatchNamesTheExpectedBase is the human half of the
// community report. The operator who was misled read a terminal, not JSON: the
// refusal stated only that the base "no longer matches compact authority" while
// the envelope beside it published the live base and nothing else. Whatever the
// machine surface carries, the terminal must state both the base this gate
// required and the base it found.
func TestReviewGateDeniedBaseMismatchNamesTheExpectedBase(t *testing.T) {
	const (
		expected = "0f772cc7071f9d47c11999222e4912e813ae3b46"
		actual   = "1344f5dbc3cffc7fbbc21cd056e491d97b1a46f7"
	)
	denied := ReviewGateDeniedError{
		Result: reviewtransaction.GateInvalidated,
		Reason: "current repository base no longer matches compact authority",
		Context: reviewtransaction.GateContext{
			Gate:            reviewtransaction.GatePrePR,
			BaseTree:        actual,
			ReceiptBaseTree: expected,
			Denial:          &reviewtransaction.GateDenial{Stage: "receipt-binding", Code: "base-mismatch"},
			BaseMismatch:    &reviewtransaction.GateBaseMismatchDiagnostics{Expected: expected, Actual: actual},
		},
	}

	got := denied.Error()
	assertNamesNoDottedOperation(t, got)
	if !strings.Contains(got, denied.Reason) {
		t.Fatalf("base-mismatch Error() = %q, want it to keep the published reason", got)
	}
	if !strings.Contains(got, expected) {
		t.Fatalf("base-mismatch Error() = %q, want it to name the expected base %q", got, expected)
	}
	if !strings.Contains(got, actual) {
		t.Fatalf("base-mismatch Error() = %q, want it to name the base it found %q", got, actual)
	}
}

// TestReviewGateDeniedBaseMismatchWithoutDiagnosticsStaysHonest keeps the
// fallback intact: a denial that carries no derived diagnostics says exactly
// what it did before rather than inventing an expectation.
func TestReviewGateDeniedBaseMismatchWithoutDiagnosticsStaysHonest(t *testing.T) {
	denied := ReviewGateDeniedError{
		Result: reviewtransaction.GateInvalidated,
		Reason: "current repository base no longer matches compact authority",
		Context: reviewtransaction.GateContext{
			Gate:   reviewtransaction.GatePrePR,
			Denial: &reviewtransaction.GateDenial{Stage: "receipt-binding", Code: "base-mismatch"},
		},
	}
	got := denied.Error()
	if got != "review lifecycle gate denied: invalidated: "+denied.Reason {
		t.Fatalf("base-mismatch fallback Error() = %q", got)
	}
}
