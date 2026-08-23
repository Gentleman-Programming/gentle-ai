package reviewtransaction

import (
	"context"
	"testing"
	"time"

	"github.com/gentleman-programming/gentle-ai/v2/internal/state"
)

// TestOfferReviewAfterVerifyDisabledKillSwitchReturnsUnavailableBeforeRepoRead
// proves that the global kill switch returns Offer{Available:false} before
// repository resolution. The nonexistent repository path makes a read visible.
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
// ctx.Err() precondition this package's other entry points enforce first, so
// an unwired API fails closed before reading global state.
func TestOfferReviewAfterVerifyContextCanceledRefusesFirst(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := OfferReviewAfterVerify(ctx, "/does/not/exist/at/all", OfferRequest{}); err == nil {
		t.Fatal("OfferReviewAfterVerify(canceled context) = nil error, want the context error")
	}
}

// TestOfferReviewAfterVerifyEnabledModeWithNoReceiptIsAvailable keeps the
// state-free enabled absence control: when no receipt is supplied, an enabled
// review workflow can offer a review without reading repository authority.
func TestOfferReviewAfterVerifyEnabledModeWithNoReceiptIsAvailable(t *testing.T) {
	enableGlobalRDDModeForOfferTest(t)
	offer, err := OfferReviewAfterVerify(context.Background(), "/does/not/exist/at/all", OfferRequest{LineageID: "unwired-offer-lineage"})
	if err != nil {
		t.Fatalf("OfferReviewAfterVerify(enabled mode) = err %v, want nil", err)
	}
	if !offer.Available {
		t.Fatalf("OfferReviewAfterVerify(enabled mode, no receipt) = %#v, want Available=true (nothing governs this candidate yet)", offer)
	}
}

// TestOfferReviewAfterVerifyUnsetModeOffersNothing proves that an unset mode
// preserves the opt-in default: without an explicit enable, no offer exists.
func TestOfferReviewAfterVerifyUnsetModeOffersNothing(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	offer, err := OfferReviewAfterVerify(context.Background(), "/does/not/exist/at/all", OfferRequest{LineageID: "unwired-offer-lineage"})
	if err != nil {
		t.Fatalf("OfferReviewAfterVerify(unset mode) = err %v, want nil", err)
	}
	if offer.Available {
		t.Fatalf("OfferReviewAfterVerify(unset mode) = %#v, want Available=false — nobody opted in", offer)
	}
}

// enableGlobalRDDModeForOfferTest gives the test an isolated home carrying the
// same explicit global "on" that `gentle-ai review mode enable` persists.
func enableGlobalRDDModeForOfferTest(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	recordedAt := time.Now().UTC()
	if err := state.Write(home, state.InstallState{
		RDDMode:           string(RDDModeOn),
		RDDModeRecordedAt: &recordedAt,
	}); err != nil {
		t.Fatalf("enable global review mode for this test: %v", err)
	}
	return home
}
