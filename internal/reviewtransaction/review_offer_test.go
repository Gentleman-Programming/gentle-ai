package reviewtransaction

import (
	"context"
	"os"
	"testing"
	"time"

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

// TestOfferReviewAfterVerifyEnabledModeWithNoReceiptIsAvailable is Wave 4
// S5b's closed loop (design decision 8, superseding the earlier unwired
// placeholder): with reviews enabled and no receipt supplied at all, no prior
// receipt governs this candidate, so OfferReviewAfterVerify genuinely invites
// a review — Available:true, not a hollow always-false stub.
//
// It used to prove this against an unset global mode, because unset resolved
// to on. Receipt-driven development is opt-in now, so unset resolves to off and
// there is correctly nothing to offer; the invitation is a property of reviews
// being enabled, so the fixture enables them the way a user does.
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

// TestOfferReviewAfterVerifyUnsetModeOffersNothing is the other side of that
// flip, and it is worth its own test: with nobody opted in, reviews are off,
// so an invitation would be an offer the product cannot honor.
func TestOfferReviewAfterVerifyUnsetModeOffersNothing(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	// No state file written at all: readGlobalRDDModeForOffer must treat a
	// missing state file as an unset (not off, not corrupt) global mode, which
	// the resolver then answers with the shipped opt-in default of off.
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

// TestOfferReviewAfterVerifyReceiptStillGoverningReportsUnavailable proves
// the other half of the closed loop: a receipt whose ValidateSDDReceiptRef
// re-validation still resolves GateAllow already governs this exact
// candidate, so there is nothing to offer.
func TestOfferReviewAfterVerifyReceiptStillGoverningReportsUnavailable(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	root := initSnapshotRepo(t)
	ref := writeApprovedCompactFixtureForOfferTest(t, root, "offer-governs-lineage")

	offer, err := OfferReviewAfterVerify(context.Background(), root, OfferRequest{LineageID: ref.Lineage, Receipt: &ref})
	if err != nil {
		t.Fatalf("OfferReviewAfterVerify(governing receipt) error = %v", err)
	}
	if offer.Available {
		t.Fatalf("OfferReviewAfterVerify(governing receipt) = %#v, want Available=false — an allow-resolving receipt already governs this candidate", offer)
	}
}

// TestOfferReviewAfterVerifyReceiptNotAllowReportsAvailable proves a
// SDDReceiptRef that does NOT resolve GateAllow (here: no matching compact
// authority for its lineage) still results in a genuine invitation — the
// same as no receipt at all.
func TestOfferReviewAfterVerifyReceiptNotAllowReportsAvailable(t *testing.T) {
	// Reviews must be enabled for an invitation to mean anything, so this opts
	// in explicitly rather than leaning on a default that no longer exists.
	enableGlobalRDDModeForOfferTest(t)
	root := initSnapshotRepo(t)
	ref := SDDReceiptRef{Lineage: "offer-no-authority-lineage", ReceiptHash: "sha256:" + shaIDForTest("9")}

	offer, err := OfferReviewAfterVerify(context.Background(), root, OfferRequest{LineageID: ref.Lineage, Receipt: &ref})
	if err != nil {
		t.Fatalf("OfferReviewAfterVerify(non-allow receipt) error = %v", err)
	}
	if !offer.Available {
		t.Fatalf("OfferReviewAfterVerify(non-allow receipt) = %#v, want Available=true", offer)
	}
}

// writeApprovedCompactFixtureForOfferTest builds one minimal zero-lens
// approved-and-verified compact authority in root and returns the
// SDDReceiptRef a caller who discovered it in the runtime ledger would
// carry.
func writeApprovedCompactFixtureForOfferTest(t *testing.T, root, lineage string) SDDReceiptRef {
	t.Helper()
	ctx := context.Background()
	snapshot, err := (SnapshotBuilder{Repo: root}).Build(ctx, Target{Kind: TargetCurrentChanges, IntendedUntracked: []string{}})
	if err != nil {
		t.Fatal(err)
	}
	assessment, err := (SnapshotBuilder{Repo: root}).AssessSnapshotRisk(ctx, snapshot)
	if err != nil {
		t.Fatal(err)
	}
	lenses, err := SelectReviewLenses(assessment, "risk")
	if err != nil {
		t.Fatal(err)
	}
	compactState, err := NewCompactState(Start{
		LineageID: lineage, Mode: ModeOrdinaryBounded, Generation: 1,
		Snapshot: snapshot, PolicyHash: "sha256:" + shaIDForTest("1"), RiskLevel: assessment.Level,
		SelectedLenses: lenses, OriginalChangedLines: &assessment.ChangedLines,
	})
	if err != nil {
		t.Fatal(err)
	}
	store, err := CompactAuthoritativeStore(ctx, root, lineage)
	if err != nil {
		t.Fatal(err)
	}
	revision, err := store.Replace("", "review/start", compactState)
	if err != nil {
		t.Fatal(err)
	}
	results := make([]LensResult, len(lenses))
	for index, lens := range lenses {
		results[index] = LensResult{Lens: lens, Findings: []Finding{}, Evidence: []string{"review complete"}}
	}
	if err := compactState.CompleteReview(CompactReviewInput{LensResults: results, Classifications: []FindingEvidence{}, RefuterOutcomes: []EvidenceResult{}}); err != nil {
		t.Fatal(err)
	}
	revision, err = store.Replace(revision, "review/complete-review", compactState)
	if err != nil {
		t.Fatal(err)
	}
	if err := compactState.CompleteVerification([]byte("verification passed\n"), true); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Replace(revision, "review/complete-verification", compactState); err != nil {
		t.Fatal(err)
	}
	receipt, err := compactState.Receipt()
	if err != nil {
		t.Fatal(err)
	}
	if err := WriteCompactReceiptAtomic(store.ReceiptPath(), receipt); err != nil {
		t.Fatal(err)
	}
	payload, err := os.ReadFile(store.ReceiptPath())
	if err != nil {
		t.Fatal(err)
	}
	return SDDReceiptRef{Lineage: lineage, ReceiptHash: receiptRefHash(payload)}
}
