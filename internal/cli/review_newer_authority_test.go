package cli

import (
	"fmt"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
)

// TestNewerAuthorityIsNotReportedAsARefreshableContextFailure is #2461's
// reproduced defect, at the boundary where the reporter met it.
//
// A v2.3.0 binary resolving a repository context whose authority rc.6 wrote
// fails inside validateLiveReviewRepositoryContext's store.Load(). Every such
// failure used to be classified as repository_context_unavailable with the
// instruction "refresh the exact native next_transition before retrying". That
// instruction is unreachable for this cause: the reading binary can never
// parse those bytes, so no transition refresh changes the outcome. @heriobb04
// followed it exactly, relaunched all four reoffered slots, and got an
// identical failure each time.
func TestNewerAuthorityIsNotReportedAsARefreshableContextFailure(t *testing.T) {
	cause := fmt.Errorf("load compact authority: %w: json: unknown field %q",
		reviewtransaction.ErrCompactAuthorityFromNewerRelease, "correction_budget_policy")

	message := reviewRepositoryContextResolutionFailure(cause).Error()

	if strings.Contains(message, "refresh the exact native next_transition") {
		t.Fatalf("refusal still tells the caller to refresh a transition that cannot help: %q", message)
	}
	if strings.Contains(message, "repository_context_unavailable") {
		t.Fatalf("refusal reuses the generic code, so this cause stays indistinguishable from a stale context: %q", message)
	}
	if !strings.Contains(message, reviewAuthorityNewerReleaseCode) {
		t.Fatalf("refusal does not carry its own typed code: %q", message)
	}
	// The action has to name what actually resolves it, and the PATH shape
	// because that is how the stale binary got invoked in the first place.
	for _, want := range []string{"upgrade", "which -a gentle-ai"} {
		if !strings.Contains(message, want) {
			t.Fatalf("refusal does not name %q: %q", want, message)
		}
	}
}

// TestOrdinaryContextFailuresKeepTheirCode proves the new branch is narrow: an
// unrelated resolution failure must still reach the caller as the generic
// refreshable context refusal, which for most causes is the right advice.
func TestOrdinaryContextFailuresKeepTheirCode(t *testing.T) {
	message := reviewRepositoryContextResolutionFailure(
		fmt.Errorf("review repository context is stale or has no live matching authority"),
	).Error()

	if !strings.Contains(message, "repository_context_unavailable") ||
		!strings.Contains(message, "refresh the exact native next_transition") {
		t.Fatalf("ordinary context failure lost its classification: %q", message)
	}
}
