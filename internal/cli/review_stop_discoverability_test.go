package cli

import (
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
)

// This file is the RED-first proof for organic-dx Phase 3b: in-band route
// enrichment. Every case here changes only a negotiated envelope field or a
// plain error message — never the underlying refusal's conditions.

// TestReceiptDiscoveryFailuresNameTheWayIn is task 3b.1, the highest-value
// fix in the batch: an agent running a delivery gate in a repository with no
// governing review is told bare "stop" today (receipt_missing) or routed
// nowhere (receipt_unrelated), even though the correct continuation
// (review.start / review.status) is already known and documented.
func TestReceiptDiscoveryFailuresNameTheWayIn(t *testing.T) {
	missing := newReviewIntegrationFailure("review.validate", nil, &ReviewReceiptDiscoveryError{Kind: ReviewReceiptMissing})
	if missing.Code != string(ReviewReceiptMissing) || missing.NextAction != "review.start" {
		t.Fatalf("receipt_missing failure = %#v, want next_action %q", missing, "review.start")
	}
	unrelated := newReviewIntegrationFailure("review.validate", nil, &ReviewReceiptDiscoveryError{Kind: ReviewReceiptUnrelated})
	if unrelated.Code != string(ReviewReceiptUnrelated) || unrelated.NextAction != "review.status" {
		t.Fatalf("receipt_unrelated failure = %#v, want next_action %q", unrelated, "review.status")
	}
}

// TestGateEscalatedDenialAndFinalVerificationRetryDenialNameStatus is task
// 3b.3: STATUS already re-derives escalated-gate and retry eligibility, but
// neither denial told the caller so.
func TestGateEscalatedDenialAndFinalVerificationRetryDenialNameStatus(t *testing.T) {
	escalated := newReviewIntegrationFailure("review.validate", nil, ReviewGateDeniedError{Result: reviewtransaction.GateEscalated})
	if escalated.Code != "gate_escalated" || escalated.NextAction != "review.status" {
		t.Fatalf("escalated gate denial failure = %#v, want next_action %q", escalated, "review.status")
	}
	retryDenied := newReviewIntegrationFailure(ReviewIntegrationOperationRetryFinalVerification, nil, &reviewtransaction.FinalVerificationRetryDeniedError{Code: "ineligible", Why: "no matching leaf"})
	if retryDenied.Code != "final_verification_retry_denied" || retryDenied.NextAction != "review.status" {
		t.Fatalf("final_verification_retry_denied failure = %#v, want next_action %q", retryDenied, "review.status")
	}
}

// TestLegacyReadOnlyNegotiatedEnvelopeCarriesTheNonNegotiatedRoute is task
// 3b.4: the non-negotiated twin (review_facade.go:1080) already names
// "choose a new lineage for compact authority"; the negotiated envelope must
// carry that same route instead of a bare stop.
func TestLegacyReadOnlyNegotiatedEnvelopeCarriesTheNonNegotiatedRoute(t *testing.T) {
	typed := reviewtransaction.NewLegacyReadOnlyError("review/start", "legacy-collision")
	failure := newReviewIntegrationFailure("review.start", []string{"--lineage", "legacy-collision"}, typed)
	if failure.NextAction != "review.start" {
		t.Fatalf("legacy_read_only next_action = %q, want %q", failure.NextAction, "review.start")
	}
	if !strings.Contains(failure.Message, "choose a new lineage for compact authority") {
		t.Fatalf("legacy_read_only message = %q, want it to name the same route as the non-negotiated twin", failure.Message)
	}
}

// TestStoreLockUnavailableNamesTheWaitAndRetryRoute is task 3b.5: contrasted
// with authority_lock_timeout, which already routes retry_with_bounded_backoff,
// store_lock_unavailable named nothing.
func TestStoreLockUnavailableNamesTheWaitAndRetryRoute(t *testing.T) {
	failure := newReviewIntegrationFailure("review.start", nil, &reviewtransaction.StoreLockPreAcquisitionError{Err: nil})
	if failure.Code != "store_lock_unavailable" || failure.NextAction != "retry_with_bounded_backoff" {
		t.Fatalf("store_lock_unavailable failure = %#v, want next_action %q", failure, "retry_with_bounded_backoff")
	}
}

// TestNegotiatedDeclineAndDisabledKillSwitchAreTypedNotStarted is task 3b.6:
// today both fall through to the generic operation_outcome_unknown default
// with mutation_outcome: unknown — a false safety claim, since neither
// operation ever began mutating authority (both refusals happen in
// authorizeReviewStart, strictly before any authority is created). This
// corrects a false claim, not wording, so it asserts MutationOutcome
// directly rather than only checking presentation.
func TestNegotiatedDeclineAndDisabledKillSwitchAreTypedNotStarted(t *testing.T) {
	declined := newReviewIntegrationFailure("review.start", nil, errReviewDeclinedForCandidate)
	if declined.MutationOutcome != ReviewMutationNotStarted {
		t.Fatalf("declined failure mutation_outcome = %q, want %q (todays bug: %q)", declined.MutationOutcome, ReviewMutationNotStarted, ReviewMutationUnknown)
	}
	if declined.Code != "review_declined" {
		t.Fatalf("declined failure code = %q, want a typed %q code", declined.Code, "review_declined")
	}

	disabled := newReviewIntegrationFailure("review.start", nil, &reviewtransaction.RDDDisabledError{Operation: reviewtransaction.RDDOperationStart, Source: reviewtransaction.RDDModeSourceGlobal})
	if disabled.MutationOutcome != ReviewMutationNotStarted {
		t.Fatalf("rdd-disabled failure mutation_outcome = %q, want %q (todays bug: %q)", disabled.MutationOutcome, ReviewMutationNotStarted, ReviewMutationUnknown)
	}
	if disabled.Code != "rdd_disabled" {
		t.Fatalf("rdd-disabled failure code = %q, want a typed %q code", disabled.Code, "rdd_disabled")
	}
}
