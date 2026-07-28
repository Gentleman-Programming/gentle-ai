package sddstatus

import (
	"strings"
	"testing"
)

// TestReviewGateEmptyReceiptReasonNamesBothSelectors proves issue #1924:
// the empty-candidate receipt rerun hint must name both --base-ref and
// --committed-only, matching the negotiated base-diff START validation in
// validateReviewTransitionSelectorFlagCounts (review_facade.go:1680-1692)
// and the recovery operation builder in ReviewGateDeniedError.Error()
// (review.go:238-240).
func TestReviewGateEmptyReceiptReasonNamesBothSelectors(t *testing.T) {
	// The hint must name --base-ref <commit> AND --committed-only in that order.
	// Without --committed-only the re-run lands on a stale_target_identity
	// rejection even with the workspace unchanged.
	if !strings.Contains(reviewGateEmptyReceiptReason, "--base-ref <commit>") {
		t.Fatalf("reviewGateEmptyReceiptReason = %q, want it to contain %q", reviewGateEmptyReceiptReason, "--base-ref <commit>")
	}
	if !strings.Contains(reviewGateEmptyReceiptReason, "--committed-only") {
		t.Fatalf("reviewGateEmptyReceiptReason = %q, want it to contain %q", reviewGateEmptyReceiptReason, "--committed-only")
	}
	// Verify order: --base-ref must appear before --committed-only.
	baseRefIdx := strings.Index(reviewGateEmptyReceiptReason, "--base-ref <commit>")
	committedIdx := strings.Index(reviewGateEmptyReceiptReason, "--committed-only")
	if baseRefIdx == -1 || committedIdx == -1 {
		t.Fatalf("reviewGateEmptyReceiptReason = %q, both selectors must be present", reviewGateEmptyReceiptReason)
	}
	if baseRefIdx > committedIdx {
		t.Fatalf("reviewGateEmptyReceiptReason = %q, --base-ref must appear before --committed-only", reviewGateEmptyReceiptReason)
	}
}
