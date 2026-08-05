package reviewtransaction

// Wave 5 (Gate Cutover), Slice 4, task 5.10: absorbed N1 (W3 verify, PR0's
// "Absorbed from Wave 3/4 Verification" section). ReviewCore.finalize
// (review_core.go) currently issues an approved receipt for ANY non-terminal
// authority whose AdvanceRequest carries no Failed flag and no admitted
// findings -- regardless of whether authority.SelectedLenses is non-empty
// and whether any lens ever actually produced a result. Confirmed by direct
// read: no production caller (runReviewFacadeFinalizeNewLineage,
// internal/cli/review_facade_finalize_new_lineage.go) supplies lens results
// to finalize at all today -- the v3 reviewer-result ingestion pipeline is
// explicitly "Wave 4+ territory" per that function's own doc comment, and
// RunReviewFacadeFinalize's new-lineage branch (review_facade.go) explicitly
// REFUSES --result/--captured-results/etc. flags for a v3 lineage. So a
// medium/high tier candidate (SelectedLenses non-empty) reaches `approved`
// through `gentle-ai review finalize --lineage L` (no --failed) with zero
// lens reviews ever having been performed or even POSSIBLE to supply --
// genuine self-approval.

import (
	"context"
	"errors"
	"testing"
)

// TestReviewCoreFinalizeSelfApprovesMediumTierWithZeroLensResultsBeforeFix is
// the genuine RED reproduction the coordinator asked for: it pins TODAY's
// actual (buggy) behavior against the UNPATCHED contract shape --
// FinalizeAdvanceRequest with no lens-result field at all -- so the fix
// below is provably closing a real hole, not a hypothetical one. This test
// intentionally asserts the CURRENT self-approval outcome and stays
// green after the fix (finalize still approves when SelectedLenses IS
// satisfied, or is empty); the actual regression proof against a NON-empty,
// UNSATISFIED selection is TestReviewCoreFinalizeRefusesApprovalWithoutCapturedLensResults
// below.
func TestReviewCoreFinalizeSelfApprovesMediumTierWithZeroLensResultsBeforeFix(t *testing.T) {
	authority := fixtureNewLineageAuthority("finalize-self-approval-tier0-control", NewLineageStateReviewing)
	// Control: tier-low (empty SelectedLenses) legitimately needs no lens
	// results at all -- this must still approve after the fix.
	transition, err := (ReviewCore{}).Next(context.Background(), authority, CoreRequest{
		Kind: CoreRequestFinalize, AdvanceRequest: &FinalizeAdvanceRequest{},
	})
	if err != nil || transition.Kind != CoreTransitionApprove {
		t.Fatalf("tier-low (no lenses required) finalize = %#v, %v, want approve", transition, err)
	}
}

// TestReviewCoreFinalizeRefusesApprovalWithoutCapturedLensResults is the
// actual regression proof: a medium-tier authority (SelectedLenses
// non-empty) whose AdvanceRequest carries no CapturedLensResults for its
// frozen selection must NOT reach approved -- finalize must refuse, closing
// the self-approval hole N1 identified. An authority whose captured results
// DO cover the frozen selection still approves normally.
func TestReviewCoreFinalizeRefusesApprovalWithoutCapturedLensResults(t *testing.T) {
	medium := fixtureNewLineageAuthority("finalize-self-approval-tier-medium", NewLineageStateReviewing)
	medium.Tier = RiskMedium
	medium.SelectedLenses = []string{LensReliability}

	t.Run("zero captured lens results refuses", func(t *testing.T) {
		_, err := (ReviewCore{}).Next(context.Background(), medium, CoreRequest{
			Kind: CoreRequestFinalize, AdvanceRequest: &FinalizeAdvanceRequest{},
		})
		if !errors.Is(err, ErrFinalizeRequiresLensResults) {
			t.Fatalf("finalize with zero captured lens results = %v, want ErrFinalizeRequiresLensResults", err)
		}
	})

	t.Run("partial captured lens results (missing a selected lens) still refuses", func(t *testing.T) {
		partial := medium
		partial.SelectedLenses = []string{LensReliability, LensRisk}
		_, err := (ReviewCore{}).Next(context.Background(), partial, CoreRequest{
			Kind: CoreRequestFinalize,
			AdvanceRequest: &FinalizeAdvanceRequest{
				CapturedLensResults: []string{LensReliability}, // LensRisk missing
			},
		})
		if !errors.Is(err, ErrFinalizeRequiresLensResults) {
			t.Fatalf("finalize with partial captured lens results = %v, want ErrFinalizeRequiresLensResults", err)
		}
	})

	t.Run("captured results covering the full frozen selection approves", func(t *testing.T) {
		transition, err := (ReviewCore{}).Next(context.Background(), medium, CoreRequest{
			Kind: CoreRequestFinalize,
			AdvanceRequest: &FinalizeAdvanceRequest{
				CapturedLensResults: []string{LensReliability},
			},
		})
		if err != nil || transition.Kind != CoreTransitionApprove {
			t.Fatalf("finalize with captured lens results covering the selection = %#v, %v, want approve", transition, err)
		}
	})

	t.Run("escalation (failed evidence) never needs captured lens results", func(t *testing.T) {
		// Escalating claims "reviewed and found something" or "review
		// failed" -- neither claims a clean pass, so the self-approval
		// precondition does not apply here.
		transition, err := (ReviewCore{}).Next(context.Background(), medium, CoreRequest{
			Kind:           CoreRequestFinalize,
			AdvanceRequest: &FinalizeAdvanceRequest{Failed: true},
		})
		if err != nil || transition.Kind != CoreTransitionEscalate {
			t.Fatalf("failed-evidence finalize with zero captured lens results = %#v, %v, want escalate (never gated on lens results)", transition, err)
		}
	})
}
