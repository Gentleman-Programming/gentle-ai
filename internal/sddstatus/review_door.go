package sddstatus

import (
	"context"
)

// review_door.go in INC-18 is decoupled from reviewtransaction.
// SDD status no longer creates review offers or calls OfferReviewAfterVerify.
var reviewEntryHook = func() {}

var reviewEntryHookCallCount int

func reviewEntryHookCallCountForTest() int {
	return reviewEntryHookCallCount
}

func resetReviewEntryHookCallCountForTest() {
	reviewEntryHookCallCount = 0
}

// reviewOfferForVerify is preserved as a no-op stub.
func reviewOfferForVerify(ctx context.Context, repo string) (bool, error) {
	reviewEntryHook()
	reviewEntryHookCallCount++
	return false, nil
}

