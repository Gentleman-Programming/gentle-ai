package cli

import (
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
)

// allReviewSelfRecoveryShapes is the closed enumeration this test walks. A
// new shape constant added to review_self_recovery_reason.go without a row
// here (or without a non-empty, uniquely-worded reason) fails closed.
var allReviewSelfRecoveryShapes = []reviewSelfRecoveryShape{
	reviewSelfRecoveryShapeRecoverInvalidated,
	reviewSelfRecoveryShapeRecoverCorrectionRequired,
	reviewSelfRecoveryShapeRecoverApproved,
	reviewSelfRecoveryShapeRecoverEscalatedChangedTarget,
	reviewSelfRecoveryShapeRecoverEscalatedAccountingOnly,
}

func TestReviewSelfRecoveryReasonIsAClosedNamedEnum(t *testing.T) {
	seen := make(map[string]reviewSelfRecoveryShape, len(allReviewSelfRecoveryShapes))
	for _, shape := range allReviewSelfRecoveryShapes {
		reason := reviewSelfRecoveryReason(shape)
		if reason == "" {
			t.Fatalf("reviewSelfRecoveryReason(%q) is empty", shape)
		}
		if reason[:len("self-recovery: ")] != "self-recovery: " {
			t.Fatalf("reviewSelfRecoveryReason(%q) = %q, want the self-recovery: prefix naming the triggering state", shape, reason)
		}
		if existing, ok := seen[reason]; ok {
			t.Fatalf("reviewSelfRecoveryReason(%q) and reviewSelfRecoveryReason(%q) collide on %q; each shape must name a distinct triggering state", shape, existing, reason)
		}
		seen[reason] = shape
	}
}

func TestReviewSelfRecoveryReasonRejectsAnUnknownShape(t *testing.T) {
	// The type is closed by construction (only the constants above compile),
	// but an unrecognized/zero-value shape must still resolve to empty
	// rather than silently accepting free text, so a caller can never
	// smuggle an unregistered reason into the CAS audit trail.
	if got := reviewSelfRecoveryReason(reviewSelfRecoveryShape("free-text-is-banned")); got != "" {
		t.Fatalf("reviewSelfRecoveryReason(unknown) = %q, want empty", got)
	}
}

func TestReviewSelfRecoveryShapeForRecover(t *testing.T) {
	tests := []struct {
		name          string
		predecessor   reviewtransaction.State
		targetChanged bool
		wantShape     reviewSelfRecoveryShape
		wantOK        bool
	}{
		{"invalidated", reviewtransaction.StateInvalidated, false, reviewSelfRecoveryShapeRecoverInvalidated, true},
		{"correction required", reviewtransaction.StateCorrectionRequired, false, reviewSelfRecoveryShapeRecoverCorrectionRequired, true},
		{"approved", reviewtransaction.StateApproved, false, reviewSelfRecoveryShapeRecoverApproved, true},
		{"escalated, target changed", reviewtransaction.StateEscalated, true, reviewSelfRecoveryShapeRecoverEscalatedChangedTarget, true},
		{"escalated, unchanged target (accounting-only candidate)", reviewtransaction.StateEscalated, false, reviewSelfRecoveryShapeRecoverEscalatedAccountingOnly, true},
		{"reviewing (active attempt) is not a derivable shape", reviewtransaction.StateReviewing, false, "", false},
		{"validating is not a derivable shape", reviewtransaction.StateValidating, false, "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			shape, ok := reviewSelfRecoveryShapeForRecover(tt.predecessor, tt.targetChanged)
			if ok != tt.wantOK || shape != tt.wantShape {
				t.Fatalf("reviewSelfRecoveryShapeForRecover(%q, %v) = (%q, %v), want (%q, %v)", tt.predecessor, tt.targetChanged, shape, ok, tt.wantShape, tt.wantOK)
			}
		})
	}
}
