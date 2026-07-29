package cli

import (
	"fmt"
	"regexp"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
)

// dottedReviewOperation matches an internal operation identifier such as
// "review.status" or "review.recover". These are wire tokens for the negotiated
// contract; typing one into a shell does nothing, so no human-surface refusal
// may contain one.
var dottedReviewOperation = regexp.MustCompile(`review\.[a-z_-]+`)

func assertNamesNoDottedOperation(t *testing.T, message string) {
	t.Helper()
	if match := dottedReviewOperation.FindString(message); match != "" {
		t.Fatalf("human refusal names the internal operation identifier %q, which is not runnable: %q", match, message)
	}
}

// TestReviewGateDeniedErrorNamesItsContinuation is the human-surface contract:
// every denial either names a command an operator can actually type, or states
// the situation the product already typed in its JSON envelope. It may never
// name a dotted operation identifier, and it may never substitute the
// "requires explicit maintainer action" placeholder for a reason the envelope
// already carries.
func TestReviewGateDeniedErrorNamesItsContinuation(t *testing.T) {
	bareMessage := func(result reviewtransaction.GateResult) string {
		return fmt.Sprintf("review lifecycle gate denied: %s", result)
	}

	t.Run("scope-changed with recovery diagnostics names a runnable recover command", func(t *testing.T) {
		denied := ReviewGateDeniedError{
			Result: reviewtransaction.GateScopeChanged,
			Context: reviewtransaction.GateContext{
				Gate: reviewtransaction.GatePrePush,
				ScopeChange: &reviewtransaction.GateScopeChangeDiagnostics{
					RecoveryOperation: "review.recover",
					RecoveryRequiredInputs: []string{
						"predecessor_lineage_id", "expected_predecessor_revision", "successor_lineage_id",
						"disposition", "reason", "actor",
					},
				},
			},
		}
		got := denied.Error()
		if got == bareMessage(reviewtransaction.GateScopeChanged) {
			t.Fatalf("scope-changed Error() is still the bare mute block: %q", got)
		}
		assertNamesNoDottedOperation(t, got)
		if !strings.Contains(got, "gentle-ai review recover") {
			t.Fatalf("scope-changed Error() = %q, want it to name the runnable gentle-ai review recover", got)
		}
		for _, input := range denied.Context.ScopeChange.RecoveryRequiredInputs {
			if !strings.Contains(got, input) {
				t.Fatalf("scope-changed Error() = %q, want it to name required input %q", got, input)
			}
		}
	})

	t.Run("scope-changed without diagnostics states the reason the envelope carries", func(t *testing.T) {
		const reason = "terminal review receipts do not exactly match the live gate target: reviewed delivery base commit is missing or ambiguous in publication range"
		denied := ReviewGateDeniedError{
			Result:  reviewtransaction.GateScopeChanged,
			Reason:  reason,
			Context: reviewtransaction.GateContext{Gate: reviewtransaction.GatePrePush},
		}
		got := denied.Error()
		assertNamesNoDottedOperation(t, got)
		if !strings.Contains(got, reason) {
			t.Fatalf("scope-changed (no diagnostics) Error() = %q, want the reason the JSON envelope already carries", got)
		}
		if strings.Contains(got, "explicit maintainer action") {
			t.Fatalf("scope-changed (no diagnostics) Error() = %q, must not defer to a maintainer while a specific reason exists", got)
		}
		if strings.Contains(got, "gentle-ai review recover") {
			t.Fatalf("scope-changed (no diagnostics) Error() = %q, must not fabricate a recovery without diagnostics", got)
		}
	})

	t.Run("invalidated states the reason the envelope carries", func(t *testing.T) {
		const reason = "current repository target does not retain the authoritative intended-untracked paths"
		denied := ReviewGateDeniedError{Result: reviewtransaction.GateInvalidated, Reason: reason}
		got := denied.Error()
		assertNamesNoDottedOperation(t, got)
		if !strings.Contains(got, reason) {
			t.Fatalf("invalidated Error() = %q, want the reason the JSON envelope already carries", got)
		}
		if strings.Contains(got, "explicit maintainer action") {
			t.Fatalf("invalidated Error() = %q, must not defer to a maintainer while a specific reason exists", got)
		}
	})

	t.Run("a denial carrying no reason at all keeps the honest terminal precondition", func(t *testing.T) {
		denied := ReviewGateDeniedError{Result: reviewtransaction.GateInvalidated}
		got := denied.Error()
		assertNamesNoDottedOperation(t, got)
		if got == bareMessage(reviewtransaction.GateInvalidated) {
			t.Fatalf("invalidated Error() is still the bare mute block: %q", got)
		}
		if !strings.Contains(got, "explicit maintainer action") {
			t.Fatalf("reasonless invalidated Error() = %q, want the honest terminal precondition", got)
		}
	})

	t.Run("escalated names change-the-candidate-then-recover with the values it already holds", func(t *testing.T) {
		const lineage = "review-escalated-lineage"
		revision := "sha256:" + strings.Repeat("c", 64)
		denied := ReviewGateDeniedError{
			Result: reviewtransaction.GateEscalated,
			Reason: "compact review authority is escalated (correction_budget_exceeded): spent 4, remaining 0, total 2 correction lines",
			Context: reviewtransaction.GateContext{
				Gate: reviewtransaction.GatePreCommit, LineageID: lineage, StoreRevision: revision,
			},
		}
		got := denied.Error()
		assertNamesNoDottedOperation(t, got)
		if got == bareMessage(reviewtransaction.GateEscalated) {
			t.Fatalf("escalated Error() is still the bare mute block: %q", got)
		}
		if !strings.Contains(got, denied.Reason) {
			t.Fatalf("escalated Error() = %q, want the escalation reason the envelope carries", got)
		}
		// `review status` only DESCRIBES an escalated lineage: with the
		// candidate unchanged it reports action "stop". Naming it would be
		// naming a dead end, so the message must name the only thing that
		// actually moves this operator forward.
		for _, want := range []string{
			"change the candidate",
			"gentle-ai review recover",
			"--predecessor-lineage " + lineage,
			"--expected-predecessor-revision " + revision,
			"--disposition escalated",
		} {
			if !strings.Contains(got, want) {
				t.Fatalf("escalated Error() = %q, want it to contain %q", got, want)
			}
		}
	})

	t.Run("escalated without a bound lineage names where to read the inputs", func(t *testing.T) {
		denied := ReviewGateDeniedError{Result: reviewtransaction.GateEscalated}
		got := denied.Error()
		assertNamesNoDottedOperation(t, got)
		if !strings.Contains(got, "gentle-ai review recover") || !strings.Contains(got, "gentle-ai review status") {
			t.Fatalf("escalated Error() without a lineage = %q, want the recover command plus where to read its inputs", got)
		}
	})

	t.Run("Result and prefix are untouched: same denial, same result value", func(t *testing.T) {
		for _, result := range []reviewtransaction.GateResult{
			reviewtransaction.GateScopeChanged, reviewtransaction.GateInvalidated, reviewtransaction.GateEscalated,
		} {
			denied := ReviewGateDeniedError{Result: result}
			if !strings.HasPrefix(denied.Error(), bareMessage(result)) {
				t.Fatalf("Error() = %q, want it to still start with %q (message-only change, no condition change)", denied.Error(), bareMessage(result))
			}
		}
	})
}

// TestReviewGateDeniedNegotiatedEnvelopeUnchangedByHumanMessage is the
// byte-identical proof (non-negotiable #3): enriching the human-surface
// Error() string must never change the negotiated JSON failure envelope
// fields that already carry the routing knowledge.
func TestReviewGateDeniedNegotiatedEnvelopeUnchangedByHumanMessage(t *testing.T) {
	tree := strings.Repeat("a", 40)
	sha := "sha256:" + strings.Repeat("b", 64)
	denied := ReviewGateDeniedError{
		Result: reviewtransaction.GateScopeChanged,
		Reason: "terminal review receipts do not exactly match the live gate target",
		Context: reviewtransaction.GateContext{
			Gate: reviewtransaction.GatePrePush,
			ScopeChange: &reviewtransaction.GateScopeChangeDiagnostics{
				Expected:             reviewtransaction.GateTargetEvidence{CandidateTree: tree, PathsDigest: sha},
				Actual:               reviewtransaction.GateTargetEvidence{CandidateTree: tree, PathsDigest: sha},
				DifferingPathsDigest: sha,
				PredecessorLineageID: "predecessor-lineage",
				PredecessorRevision:  sha,
				RecoveryOperation:    "review.recover",
				RecoveryRequiredInputs: []string{
					"predecessor_lineage_id", "expected_predecessor_revision", "successor_lineage_id",
					"disposition", "reason", "actor",
				},
			},
		},
	}
	failure := newReviewIntegrationFailure(ReviewIntegrationOperationValidate, nil, denied)
	if failure.Message != "The review delivery gate denied the current target." {
		t.Fatalf("negotiated failure Message changed = %q, want the existing byte-identical envelope text", failure.Message)
	}
	if failure.Code != "gate_scope_changed" || failure.NextAction != reviewGateAction(reviewtransaction.GateScopeChanged) {
		t.Fatalf("negotiated failure Code/NextAction changed = code=%q next_action=%q", failure.Code, failure.NextAction)
	}
	if failure.Context == nil || failure.Context.ScopeChange == nil ||
		failure.Context.ScopeChange.RecoveryOperation != "review.recover" ||
		!strings.Contains(strings.Join(failure.Context.ScopeChange.RecoveryRequiredInputs, ","), "predecessor_lineage_id") {
		t.Fatalf("negotiated failure scope-change context changed = %#v", failure.Context)
	}
	if err := failure.Validate(); err != nil {
		t.Fatalf("negotiated failure with untouched routing fields must still validate: %v", err)
	}
}
