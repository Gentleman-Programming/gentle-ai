package sddstatus

import (
	"context"

	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
)

// review_door.go is internal/sddstatus's one door into reviewtransaction's
// offer/ReviewCore symbols (design.md decision 4). It exists so the Wave 4
// S3 absence guard (review_offer_absence_guard_test.go) can name exactly one
// exempt file rather than allowing offer/ReviewCore references anywhere in
// the package.
//
// reviewOfferForVerify is this door's one real call (wired 2026-08-03 per
// the orchestrator's amendment to design.md decision 3): status.go calls it
// exactly when verify has passed and the kill switch is on
// (applyReviewOfferRouting), never otherwise. Modelled on
// internal/reviewtransaction/gate.go's
// finalGateAuthorizationHook/artifactPreimagesReadHook trip-wire pattern and
// internal/reviewtransaction/shadow_observer.go's
// shadowObserverCallCountForTest — reviewEntryHook fires once per call, and
// reviewEntryHookCallCountForTest gives tests and the OFF-mode absence
// journey a way to prove it fired exactly zero times without needing to
// override the hook itself.
var reviewEntryHook = func() {}

// reviewEntryHookCallCount is incremented once per reviewOfferForVerify
// call, independent of whatever reviewEntryHook itself is currently
// overridden to (tests that override reviewEntryHook, e.g.
// TestReviewEntryHookIsTheOneDoor, do not affect this counter). Package
// tests are not run with -parallel against this file, so no mutex is used
// here — matching the precedent shadowObserverCallCount predates before it
// added concurrency support for its own, different, concurrent use.
var reviewEntryHookCallCount int

func reviewEntryHookCallCountForTest() int {
	return reviewEntryHookCallCount
}

func resetReviewEntryHookCallCountForTest() {
	reviewEntryHookCallCount = 0
}

// reviewOfferForVerify is internal/sddstatus's one call into
// reviewtransaction's post-verify offer API. It always fires reviewEntryHook
// first, so every real call is provable by the counter above regardless of
// what OfferReviewAfterVerify itself does.
//
// Offers are informational and mode-only. They do not read runtime authority.
func reviewOfferForVerify(ctx context.Context, repo string) (reviewtransaction.Offer, error) {
	reviewEntryHook()
	reviewEntryHookCallCount++
	return reviewtransaction.OfferReviewAfterVerify(ctx, repo)
}
