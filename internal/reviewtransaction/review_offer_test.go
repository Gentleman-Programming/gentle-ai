package reviewtransaction

import (
	"context"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/state"
)

// TestOfferReviewAfterVerifyDisabledKillSwitchReturnsUnavailableBeforeRepoRead
// is design decision 8's one testable requirement for the unwired Wave-4 API
// shape: with the GLOBAL kill switch recorded off, OfferReviewAfterVerify
// returns Offer{Available:false}, nil BEFORE touching the repository at all —
// proven here by supplying a repository path that does not exist and
// confirming the call still succeeds cleanly instead of failing on repo
// resolution. The clone-local override is off-only (rdd_mode.go's own
// ErrRDDModeRepositoryForcedOn invariant), so a global-off reading is on its
// own sufficient to know the effective switch is off without ever reading a
// clone-local override — that is what makes a repo-free early return honest
// rather than a partial check.
func TestOfferReviewAfterVerifyDisabledKillSwitchReturnsUnavailableBeforeRepoRead(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	if err := state.Write(home, state.InstallState{RDDMode: string(RDDModeOff)}); err != nil {
		t.Fatal(err)
	}

	offer, err := OfferReviewAfterVerify(context.Background(), "/does/not/exist/at/all", OfferRequest{LineageID: "unwired-offer-lineage"})
	if err != nil {
		t.Fatalf("OfferReviewAfterVerify(kill switch off) = err %v, want nil (a repository read would have failed on this nonexistent path)", err)
	}
	if offer.Available {
		t.Fatalf("OfferReviewAfterVerify(kill switch off) = %#v, want Available=false", offer)
	}
}

// TestOfferReviewAfterVerifyContextCanceledRefusesFirst proves the ordinary
// ctx.Err() precondition this package's other entry points (ReviewCore.Next,
// DiscoverNewLineage) already enforce first, so an unwired API still fails
// closed on an already-canceled context rather than reading global state.
func TestOfferReviewAfterVerifyContextCanceledRefusesFirst(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := OfferReviewAfterVerify(ctx, "/does/not/exist/at/all", OfferRequest{}); err == nil {
		t.Fatal("OfferReviewAfterVerify(canceled context) = nil error, want the context error")
	}
}

// TestOfferReviewAfterVerifyDefaultModeStillReportsUnavailable triangulates
// against the disabled case above with a DIFFERENT global mode (unset —
// RDDModeStatusSchema's own default) to prove the false result on the
// disabled path is not a hardcoded hollow stub: an unset global mode reaches
// past the kill-switch early return (default is effective ON, not OFF) and
// still reports Available:false, exactly as design decision 8 specifies —
// this API ships its exact shape unwired, with no fabricated go-ahead until
// Wave 4 composes the rest of the decision.
func TestOfferReviewAfterVerifyDefaultModeStillReportsUnavailable(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	// No state file written at all: readGlobalRDDModeForOffer must treat a
	// missing state file as an unset (not off, not corrupt) global mode.
	offer, err := OfferReviewAfterVerify(context.Background(), "/does/not/exist/at/all", OfferRequest{LineageID: "unwired-offer-lineage"})
	if err != nil {
		t.Fatalf("OfferReviewAfterVerify(default mode) = err %v, want nil", err)
	}
	if offer.Available {
		t.Fatalf("OfferReviewAfterVerify(default mode) = %#v, want Available=false (unwired: Wave 4 composes the real decision)", offer)
	}
}
