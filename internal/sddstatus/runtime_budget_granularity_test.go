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
