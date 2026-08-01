package sddstatus

import (
	"context"
	"errors"
	"testing"
)

// TestRuntimeFinishDoesNotDemandAReviewSuccessorWhileReviewIsDisabled is the
// second instance the reporter raised, and it is the same principle as the
// gate: with receipt-driven development switched off, receipt-driven development
// does not exist, so it must have no implications.
//
// The deadlock it removes is real and closed. A clone earns a review binding,
// the operator switches reviews off, work continues, and the attempt changes
// the candidate tree. Closing that passing attempt used to demand an atomic
// approved recovery successor — a successor the operator cannot produce,
// because `review start` is refused while the switch is off. The only way out
// was to turn reviews back on for the sole purpose of satisfying a system that
// was supposed to be inert.
//
// Nothing is fabricated here: no binding is advanced, no approval is minted,
// and the attempt closes as an ordinary finish. Turning reviews back on
// re-validates from the current state — the binding still refers to the
// candidate it was approved for, and the next enforcement point rediscovers
// that on its own.
func TestRuntimeFinishDoesNotDemandAReviewSuccessorWhileReviewIsDisabled(t *testing.T) {
	fixture := newRuntimeRemediationFixture(t, true)
	request := fixture.finishRequest("finish-while-review-disabled")
	request.ExpectedBindingRevision = ""
	request.SuccessorLineageID = ""
	request.RemediatesEvidenceRevision = ""

	store := fixture.store
	store.ReviewDisabled = true
	before := countRuntimeRecords(t, store.Dir)

	status, err := store.Finish(context.Background(), request)
	if err != nil {
		t.Fatalf("closing a passing attempt while reviews are off demanded a review obligation: %T %v", err, err)
	}
	if status.ActiveAttempt != nil {
		t.Fatalf("attempt did not close while reviews are off: %#v", status.ActiveAttempt)
	}
	if countRuntimeRecords(t, store.Dir) != before+1 {
		t.Fatalf("disabled finish records = %d, want %d", countRuntimeRecords(t, store.Dir), before+1)
	}

	// It closed as an ORDINARY finish: no remediation record, and the binding
	// is exactly the one that already existed. A disabled switch never advances
	// review authority any more than it approves.
	record, err := store.loadRecord(status.Revision)
	if err != nil {
		t.Fatal(err)
	}
	if record.Operation != runtimeOperationFinish || record.Binding != nil {
		t.Fatalf("disabled finish record = %#v", record)
	}
	if status.Binding == nil || status.Binding.Revision != fixture.predecessorBinding.Revision {
		t.Fatalf("disabled finish mutated the review binding: %#v", status.Binding)
	}
}

// TestRuntimeFinishStillDemandsAReviewSuccessorWhileReviewIsEnabled is the
// regression that matters most: with the switch on, the identical attempt still
// refuses to close without an approved recovery successor.
func TestRuntimeFinishStillDemandsAReviewSuccessorWhileReviewIsEnabled(t *testing.T) {
	fixture := newRuntimeRemediationFixture(t, true)
	request := fixture.finishRequest("finish-while-review-enabled")
	request.ExpectedBindingRevision = ""
	request.SuccessorLineageID = ""
	request.RemediatesEvidenceRevision = ""

	if fixture.store.ReviewDisabled {
		t.Fatal("the default RuntimeStore must enforce review obligations")
	}
	before := countRuntimeRecords(t, fixture.store.Dir)
	if _, err := fixture.store.Finish(context.Background(), request); !errors.Is(err, ErrRuntimeRemediationSuccessorRequired) {
		t.Fatalf("enabled finish without a successor = %T %v", err, err)
	}
	assertRuntimeRemediationUnchanged(t, fixture, before)
}

// TestRuntimeFinishStillValidatesAnExplicitSuccessorWhileReviewIsDisabled holds
// the other edge: the switch removes the IMPLICIT demand, never the checks on
// an explicit request. An operator who deliberately passes a remediation
// successor while reviews are off asked for receipt-driven development to act,
// so it still validates that successor rather than trusting it.
func TestRuntimeFinishStillValidatesAnExplicitSuccessorWhileReviewIsDisabled(t *testing.T) {
	fixture := newRuntimeRemediationFixture(t, true)
	request := fixture.finishRequest("finish-explicit-stale-successor-while-disabled")
	request.ExpectedBindingRevision = runtimeTestHash('9')

	store := fixture.store
	store.ReviewDisabled = true
	before := countRuntimeRecords(t, store.Dir)

	_, err := store.Finish(context.Background(), request)
	var conflict *BindingRevisionConflictError
	if !errors.As(err, &conflict) || conflict.Current != fixture.predecessorBinding.Revision {
		t.Fatalf("explicit stale successor while disabled = %T %#v", err, err)
	}
	assertRuntimeRemediationUnchanged(t, fixture, before)
}
