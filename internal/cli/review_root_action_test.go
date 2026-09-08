package cli

import (
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
)

// The root `action` of a negotiated STATUS envelope must agree with its
// `next_transition`: while a live transaction mandates a collect or an
// execute, "stop" is not an honest root action (#3928).
func TestReviewRootActionFollowsMandatedTransition(t *testing.T) {
	collect := &ReviewNextTransition{Kind: reviewNextTransitionCollect, ReasonCode: "reviewer_results_required"}
	execute := &ReviewNextTransition{Kind: reviewNextTransitionExecute, ReasonCode: "approved_acknowledgement_required"}
	stop := &ReviewNextTransition{Kind: reviewNextTransitionStop, ReasonCode: "native_stop_required"}
	cases := []struct {
		name       string
		action     reviewtransaction.TargetStatusAction
		transition *ReviewNextTransition
		want       reviewtransaction.TargetStatusAction
	}{
		{"reviewing lineage awaiting captures", reviewtransaction.TargetStatusActionStop, collect, reviewtransaction.TargetStatusActionCollect},
		{"approved lineage awaiting acknowledgement", reviewtransaction.TargetStatusActionStop, execute, reviewtransaction.TargetStatusActionExecute},
		{"genuine terminal stop", reviewtransaction.TargetStatusActionStop, stop, reviewtransaction.TargetStatusActionStop},
		{"no transition computed", reviewtransaction.TargetStatusActionStop, nil, reviewtransaction.TargetStatusActionStop},
		{"preflight start stays start", reviewtransaction.TargetStatusActionStart, execute, reviewtransaction.TargetStatusActionStart},
		{"recovery stays recover", reviewtransaction.TargetStatusActionRecover, execute, reviewtransaction.TargetStatusActionRecover},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := reviewRootActionForTransition(tc.action, tc.transition); got != tc.want {
				t.Fatalf("reviewRootActionForTransition(%q, %v) = %q, want %q", tc.action, tc.transition, got, tc.want)
			}
		})
	}
}
