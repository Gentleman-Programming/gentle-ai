package cli

import (
	"errors"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
)

// TestUnclassifiedProgressFailureStillCarriesItsCause pins the diagnostic half
// of the post-progress classification.
//
// The classification itself is deliberate and stays untouched: issue 1861
// established that a failure arriving after a committed native transition must
// keep reporting an unknown mutation outcome, because a caller that retries a
// maybe-committed mutation can double-apply it. See
// TestLockContentionAfterACommittedTransitionStillReportsUnknown.
//
// What is not deliberate is discarding the reason. The progress branch matches
// four typed causes and returns early, so any other cause leaves the envelope
// on its struct-literal default and skips the assignment that would have
// preserved the native reason. The caller then reads "failed without
// authoritative mutation evidence" with an empty cause and has nothing to act
// on or report, for a state whose reason the tool already held.
func TestUnclassifiedProgressFailureStillCarriesItsCause(t *testing.T) {
	t.Parallel()

	const lineage = "unclassified-progress-cause"
	native := errors.New("authority store rejected the terminal successor")
	progressed := &reviewFacadeOperationProgressError{
		LineageID:            lineage,
		StoreRevision:        "sha256:" + strings.Repeat("b", 64),
		CommittedTransitions: 1,
		Cause:                native,
	}

	failure := newReviewIntegrationFailure(
		ReviewIntegrationOperationFinalize, []string{"--lineage", lineage}, progressed,
	)

	// The 1861 classification is preserved exactly.
	if failure.Phase != "native_committed" || failure.MutationOutcome != ReviewMutationUnknown ||
		failure.RetrySafe || failure.Replayability != reviewtransaction.ReplayabilityStatusRequired ||
		failure.NextAction != "review.status" || failure.LineageID != lineage {
		t.Fatalf("post-progress classification was altered: %#v", failure)
	}

	// The reason survives instead of being discarded.
	if strings.TrimSpace(failure.Cause) == "" {
		t.Fatalf("unclassified post-progress failure carries no cause: %#v", failure)
	}
	if !strings.Contains(failure.Cause, "authority store rejected the terminal successor") {
		t.Fatalf("cause does not name the native reason: %q", failure.Cause)
	}
}
