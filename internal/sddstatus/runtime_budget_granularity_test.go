package sddstatus

import (
	"context"
	"fmt"
	"testing"
)

// #3815: RuntimeAttempt was simultaneously one provider call, one unit of
// budget, and one unit of work. A work unit that legitimately needs several
// calls therefore exhausted its objective by ACCOUNTING rather than by
// failure: each begin charged an attempt, so with the default budget of two,
// two calls ended the objective even when both delivered real increment. That
// is #3808, where two calls produced zero delivered production and
// decision_required.
//
// The rule: an interrupted settlement that delivered measurable increment
// advances the objective instead of discharging an attempt against it. A call
// that delivered nothing is still spent, so max_attempts keeps bounding calls
// that produce nothing, and cumulative changed lines keep bounding the total —
// a refund always costs delivered lines, and those are capped.

func beginRuntimeAttempt(t *testing.T, store RuntimeStore, expected, requestID string) RuntimeStatus {
	t.Helper()
	status, err := store.Begin(context.Background(), BeginAttemptRequest{
		ExpectedRevision: expected, RequestID: requestID, WorkUnit: "large-atomic-unit",
		EvidenceGoal: "deliver one atomic cutover across several calls",
		MaxAttempts:  2, MaxChangedLines: 400,
	})
	if err != nil {
		t.Fatalf("Begin(%s): %v", requestID, err)
	}
	return status
}

func interruptRuntimeAttempt(t *testing.T, store RuntimeStore, expected, requestID string) RuntimeStatus {
	t.Helper()
	status, err := store.Finish(context.Background(), FinishAttemptRequest{
		ExpectedRevision: expected, RequestID: requestID, Outcome: AttemptInterrupted,
		Diagnosis:          "the call ended before the atomic unit was complete",
		HarnessDisposition: HarnessReused,
		CleanupEvidence:    "workspace left intact for the successor call",
		ProcessEvidence:    "no descendants remained after the call",
	})
	if err != nil {
		t.Fatalf("Finish(%s): %v", requestID, err)
	}
	return status
}

// TestInterruptedCallThatDeliveredIncrementDoesNotSpendTheObjectiveBudget is
// the #3808 shape: two calls, both delivering, must not exhaust a two-attempt
// objective.
func TestInterruptedCallThatDeliveredIncrementDoesNotSpendTheObjectiveBudget(t *testing.T) {
	repo := initRuntimeLedgerRepo(t)
	store, err := OpenRuntimeStore(context.Background(), repo, "budget-granularity")
	if err != nil {
		t.Fatal(err)
	}

	status := beginRuntimeAttempt(t, store, "", "granularity-begin-1")
	appendRuntimeLedgerFile(t, repo, "first slice of the cutover\n")
	status = interruptRuntimeAttempt(t, store, status.Revision, "granularity-finish-1")

	if status.CumulativeAttempts != 0 {
		t.Errorf("CumulativeAttempts = %d after an interrupted call that delivered increment, want 0", status.CumulativeAttempts)
	}
	if status.CumulativeChangedLines == 0 {
		t.Error("CumulativeChangedLines = 0; the delivered increment was not charged")
	}
	if status.DecisionRequired {
		t.Error("DecisionRequired after one delivering call")
	}

	status = beginRuntimeAttempt(t, store, status.Revision, "granularity-begin-2")
	appendRuntimeLedgerFile(t, repo, "second slice of the cutover\n")
	status = interruptRuntimeAttempt(t, store, status.Revision, "granularity-finish-2")

	if status.DecisionRequired {
		t.Errorf("DecisionRequired after two delivering calls on a %d-attempt objective; the unit exhausted by accounting", 2)
	}
	if status.NextAction != RuntimeActionBegin {
		t.Errorf("NextAction = %q after a delivering call, want %q", status.NextAction, RuntimeActionBegin)
	}
}

// TestInterruptedCallThatDeliveredNothingStillSpendsTheBudget pins the other
// half: a refund is earned by delivering, never granted for free.
func TestInterruptedCallThatDeliveredNothingStillSpendsTheBudget(t *testing.T) {
	repo := initRuntimeLedgerRepo(t)
	store, err := OpenRuntimeStore(context.Background(), repo, "budget-granularity-empty")
	if err != nil {
		t.Fatal(err)
	}

	status := beginRuntimeAttempt(t, store, "", "empty-begin-1")
	status = interruptRuntimeAttempt(t, store, status.Revision, "empty-finish-1")

	if status.CumulativeAttempts != 1 {
		t.Errorf("CumulativeAttempts = %d after an interrupted call that delivered nothing, want 1", status.CumulativeAttempts)
	}
	if status.LifetimeAttempts != 1 {
		t.Errorf("LifetimeAttempts = %d, want 1; the lifetime counter is never refunded", status.LifetimeAttempts)
	}
}

// TestRefundsAreCappedAtTheConfiguredAttemptCeiling pins the bound: an
// objective earns back at most MaxAttempts calls, so it spends at most twice
// what the operator configured and max_attempts still escalates.
func TestRefundsAreCappedAtTheConfiguredAttemptCeiling(t *testing.T) {
	repo := initRuntimeLedgerRepo(t)
	store, err := OpenRuntimeStore(context.Background(), repo, "budget-refund-cap")
	if err != nil {
		t.Fatal(err)
	}

	status := RuntimeStatus{}
	expected := ""
	for call := 1; call <= 4; call++ {
		status = beginRuntimeAttempt(t, store, expected, fmt.Sprintf("cap-begin-%d", call))
		appendRuntimeLedgerFile(t, repo, fmt.Sprintf("slice %d\n", call))
		status = interruptRuntimeAttempt(t, store, status.Revision, fmt.Sprintf("cap-finish-%d", call))
		expected = status.Revision
		if call < 4 && status.DecisionRequired {
			t.Fatalf("call %d reached decision-required before the 2x ceiling", call)
		}
	}

	if !status.DecisionRequired {
		t.Errorf("four delivering calls on a 2-attempt objective did not reach decision-required; max_attempts no longer escalates")
	}
	if status.LifetimeAttempts != 4 {
		t.Errorf("LifetimeAttempts = %d, want 4; every call that ran must stay recorded", status.LifetimeAttempts)
	}
}

// failRuntimeAttempt closes the active attempt with outcome failed, the
// disposition and evidence revision the caller declares.
func failRuntimeAttempt(t *testing.T, store RuntimeStore, expected, requestID string, disposition HarnessDisposition, evidenceRevision string) RuntimeStatus {
	t.Helper()
	status, err := store.Finish(context.Background(), FinishAttemptRequest{
		ExpectedRevision: expected, RequestID: requestID, Outcome: AttemptFailed,
		EvidenceRevision:   evidenceRevision,
		Diagnosis:          "independent verification rejected the candidate",
		HarnessDisposition: disposition,
		CleanupEvidence:    "workspace left intact for the successor call",
		ProcessEvidence:    "no descendants remained after the call",
	})
	if err != nil {
		t.Fatalf("Finish(%s): %v", requestID, err)
	}
	return status
}

// TestFailedInvalidatedZeroLineSettlementRefundsTheAttempt is the RED
// reproduction for #3152: a failed settlement the actor itself marked
// HarnessInvalidated, with zero native changed lines, means the intended
// acceptance work never ran at all -- the harness could not be constructed,
// not the candidate. Charging it identically to a real product failure
// consumes the acceptance budget before any acceptance attempt has actually
// executed. This mirrors the already-established interrupted-with-increment
// refund (#3815/#3839) with the complementary failure-side shape the design
// decision on #3152 selected, and reuses the exact same accounting fields
// (outcome, harness_disposition, changed_lines) status already serializes
// per attempt -- no new field is needed for a reader to see why.
func TestFailedInvalidatedZeroLineSettlementRefundsTheAttempt(t *testing.T) {
	repo := initRuntimeLedgerRepo(t)
	store, err := OpenRuntimeStore(context.Background(), repo, "invalidated-harness-refund")
	if err != nil {
		t.Fatal(err)
	}

	status := beginRuntimeAttempt(t, store, "", "invalidated-begin-1")
	status = failRuntimeAttempt(t, store, status.Revision, "invalidated-finish-1", HarnessInvalidated, runtimeTestHash('4'))

	if status.CumulativeAttempts != 0 {
		t.Errorf("CumulativeAttempts = %d after a failed/invalidated/zero-line settlement, want 0 (refunded)", status.CumulativeAttempts)
	}
	if status.LifetimeAttempts != 1 {
		t.Errorf("LifetimeAttempts = %d, want 1; the lifetime counter is never refunded", status.LifetimeAttempts)
	}
	if status.DecisionRequired {
		t.Error("DecisionRequired after one refunded harness-invalidated failure on a 2-attempt objective")
	}
	if status.NextAction != RuntimeActionBegin {
		t.Errorf("NextAction = %q after a refunded failure, want %q", status.NextAction, RuntimeActionBegin)
	}
	if len(status.Attempts) != 1 || status.Attempts[0].Outcome != AttemptFailed ||
		status.Attempts[0].HarnessDisposition != HarnessInvalidated || status.Attempts[0].ChangedLines != 0 {
		t.Fatalf("recorded attempt = %#v, want the truthful failed/invalidated/zero-line record status already shows", status.Attempts)
	}

	// A SECOND harness-invalidated zero-line failure still refunds: the
	// design decision caps refunds at MaxAttempts (2 here), so this call is
	// also earned back.
	status = beginRuntimeAttempt(t, store, status.Revision, "invalidated-begin-2")
	status = failRuntimeAttempt(t, store, status.Revision, "invalidated-finish-2", HarnessInvalidated, runtimeTestHash('5'))
	if status.CumulativeAttempts != 0 {
		t.Errorf("CumulativeAttempts = %d after two refunded harness-invalidated failures, want 0", status.CumulativeAttempts)
	}
	if status.DecisionRequired {
		t.Error("DecisionRequired after two refunded harness-invalidated failures on a 2-attempt objective")
	}

	// A THIRD and FOURTH failure exhaust the refund cap (MaxAttempts=2,
	// #2804/#3815, the same shape TestRefundsAreCappedAtTheConfiguredAttempt-
	// Ceiling pins for the interrupted side): the objective must still reach
	// decision-required eventually, so max_attempts keeps escalating instead
	// of refunding without bound.
	status = beginRuntimeAttempt(t, store, status.Revision, "invalidated-begin-3")
	status = failRuntimeAttempt(t, store, status.Revision, "invalidated-finish-3", HarnessInvalidated, runtimeTestHash('6'))
	if status.DecisionRequired {
		t.Error("DecisionRequired before the refund cap was actually exhausted")
	}
	status = beginRuntimeAttempt(t, store, status.Revision, "invalidated-begin-4")
	status = failRuntimeAttempt(t, store, status.Revision, "invalidated-finish-4", HarnessInvalidated, runtimeTestHash('7'))
	if !status.DecisionRequired {
		t.Error("four harness-invalidated failures on a 2-attempt objective did not reach decision-required; the refund cap did not hold")
	}
	if status.LifetimeAttempts != 4 {
		t.Errorf("LifetimeAttempts = %d, want 4; every call that ran must stay recorded even when refunded", status.LifetimeAttempts)
	}
}

// TestFailedSettlementKeepsChargedWhenNotBothInvalidatedAndZeroLine pins the
// narrow scope the design decision selected (#3152): only failed AND
// HarnessInvalidated AND zero changed lines refunds. A reused-harness
// failure, and an invalidated-harness failure that DID change lines, must
// both remain charged exactly as before.
func TestFailedSettlementKeepsChargedWhenNotBothInvalidatedAndZeroLine(t *testing.T) {
	t.Run("reused harness failure stays charged", func(t *testing.T) {
		repo := initRuntimeLedgerRepo(t)
		store, err := OpenRuntimeStore(context.Background(), repo, "reused-harness-charged")
		if err != nil {
			t.Fatal(err)
		}
		status := beginRuntimeAttempt(t, store, "", "reused-begin-1")
		status = failRuntimeAttempt(t, store, status.Revision, "reused-finish-1", HarnessReused, runtimeTestHash('7'))
		if status.CumulativeAttempts != 1 {
			t.Errorf("CumulativeAttempts = %d after a reused-harness failure, want 1 (charged)", status.CumulativeAttempts)
		}
	})

	t.Run("invalidated harness failure with changed lines stays charged", func(t *testing.T) {
		repo := initRuntimeLedgerRepo(t)
		store, err := OpenRuntimeStore(context.Background(), repo, "invalidated-lines-charged")
		if err != nil {
			t.Fatal(err)
		}
		status := beginRuntimeAttempt(t, store, "", "invalidated-lines-begin-1")
		appendRuntimeLedgerFile(t, repo, "partial candidate change before the harness was invalidated\n")
		status = failRuntimeAttempt(t, store, status.Revision, "invalidated-lines-finish-1", HarnessInvalidated, runtimeTestHash('8'))
		if status.CumulativeAttempts != 1 {
			t.Errorf("CumulativeAttempts = %d after an invalidated-harness failure with changed lines, want 1 (charged)", status.CumulativeAttempts)
		}
	})
}

// commitRawFinishRecord publishes a finish record directly, bypassing
// Finish's own write-time computation of InvalidatedWithoutDelivery, so a
// test can pin the exact on-disk shape an older (or newer) binary would have
// committed rather than the shape today's Finish would compute.
func commitRawFinishRecord(t *testing.T, store RuntimeStore, previousRevision, requestID string, event *runtimeFinishEvent) {
	t.Helper()
	record := runtimeRecord{
		Schema: runtimeRecordSchema, Change: store.Change, PreviousRevision: previousRevision,
		Operation: runtimeOperationFinish, RequestID: requestID, Finish: event,
	}
	record.RequestDigest = runtimeValueHash("gentle-ai.sdd-runtime-finish-request/v1", FinishAttemptRequest{
		ExpectedRevision: previousRevision, RequestID: requestID, Outcome: event.Outcome,
		EvidenceRevision: event.EvidenceRevision, Diagnosis: event.Diagnosis,
		HarnessDisposition: event.HarnessDisposition, CleanupEvidence: event.CleanupEvidence,
		ProcessEvidence: event.ProcessEvidence,
	})
	revision, payload, err := runtimeRecordRevision(record)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.publishRecord(revision, payload); err != nil {
		t.Fatal(err)
	}
	if err := store.publishHead(revision); err != nil {
		t.Fatal(err)
	}
}

// TestReplayHonorsTheStoredInvalidatedWithoutDeliveryMarker is #3152's R3
// fix: the refund ground for a failed/invalidated/zero-line settlement must
// be the marker recorded ONCE at settle time, never a predicate recomputed
// against historical outcome/harness_disposition/changed_lines on every
// replay. Recomputing it would retroactively refund -- and silently reopen a
// maintainer-consent gate on -- a chain a pre-#3152 binary already charged
// and closed.
func TestReplayHonorsTheStoredInvalidatedWithoutDeliveryMarker(t *testing.T) {
	t.Run("a pre-#3152 record without the marker keeps its old charge and closed gate", func(t *testing.T) {
		repo := initRuntimeLedgerRepo(t)
		store, err := OpenRuntimeStore(context.Background(), repo, "legacy-invalidated-no-marker")
		if err != nil {
			t.Fatal(err)
		}
		started, err := store.Begin(context.Background(), BeginAttemptRequest{
			RequestID: "legacy-begin-1", WorkUnit: "acceptance", EvidenceGoal: "run acceptance work",
			MaxAttempts: 1, MaxChangedLines: 400,
		})
		if err != nil {
			t.Fatal(err)
		}
		beginRecord, err := store.loadRecord(started.Revision)
		if err != nil {
			t.Fatal(err)
		}
		// No InvalidatedWithoutDelivery field at all -- exactly what a binary
		// compiled before this field existed would have committed.
		commitRawFinishRecord(t, store, started.Revision, "legacy-finish-1", &runtimeFinishEvent{
			Ordinal: 1, FinishCandidateIdentity: beginRecord.Begin.BeginCandidateIdentity,
			FinishCandidateTree: beginRecord.Begin.BeginCandidateTree, Outcome: AttemptFailed,
			ChangedLines: 0, EvidenceRevision: runtimeTestHash('e'),
			Diagnosis: "harness crashed before running anything", HarnessDisposition: HarnessInvalidated,
			CleanupEvidence: "none", ProcessEvidence: "none",
		})
		status, err := store.Status()
		if err != nil {
			t.Fatal(err)
		}
		if status.CumulativeAttempts != 1 {
			t.Fatalf("CumulativeAttempts = %d after replaying a pre-#3152 record, want 1 (its original charge, unchanged)", status.CumulativeAttempts)
		}
		if !status.DecisionRequired {
			t.Fatal("replay silently released a maintainer-consent gate a pre-#3152 binary already closed")
		}
	})

	t.Run("a record carrying the marker refunds and the gate stays open", func(t *testing.T) {
		repo := initRuntimeLedgerRepo(t)
		store, err := OpenRuntimeStore(context.Background(), repo, "marked-invalidated-refunds")
		if err != nil {
			t.Fatal(err)
		}
		started, err := store.Begin(context.Background(), BeginAttemptRequest{
			RequestID: "marked-begin-1", WorkUnit: "acceptance", EvidenceGoal: "run acceptance work",
			MaxAttempts: 1, MaxChangedLines: 400,
		})
		if err != nil {
			t.Fatal(err)
		}
		beginRecord, err := store.loadRecord(started.Revision)
		if err != nil {
			t.Fatal(err)
		}
		commitRawFinishRecord(t, store, started.Revision, "marked-finish-1", &runtimeFinishEvent{
			Ordinal: 1, FinishCandidateIdentity: beginRecord.Begin.BeginCandidateIdentity,
			FinishCandidateTree: beginRecord.Begin.BeginCandidateTree, Outcome: AttemptFailed,
			ChangedLines: 0, EvidenceRevision: runtimeTestHash('f'),
			Diagnosis: "harness crashed before running anything", HarnessDisposition: HarnessInvalidated,
			CleanupEvidence: "none", ProcessEvidence: "none",
			InvalidatedWithoutDelivery: true,
		})
		status, err := store.Status()
		if err != nil {
			t.Fatal(err)
		}
		if status.CumulativeAttempts != 0 {
			t.Fatalf("CumulativeAttempts = %d after replaying a marked record, want 0 (refunded)", status.CumulativeAttempts)
		}
		if status.DecisionRequired {
			t.Fatal("a refunded attempt must not close the gate on a 1-attempt objective")
		}
	})
}
