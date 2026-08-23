package sddstatus

import (
	"context"
	"strings"
	"testing"
)

// A settle refusal that withholds the state it evaluated is a dead end with a
// sentence attached. This test covers the SDD-owned failed-evidence accounting
// path and requires the refusal to disclose both the discharged state and a
// runnable continuation.

// TestDischargedFailureRefusalSaysItWasDischargedAndNamesTheOrdinaryExit is
// #2881. The exit here is genuinely simple, which is what makes the silence so
// expensive: this work unit is ordinary work, and it settles by dropping
// --remediates-evidence-revision entirely.
func TestDischargedFailureRefusalSaysItWasDischargedAndNamesTheOrdinaryExit(t *testing.T) {
	ctx := context.Background()
	repo := initRuntimeLedgerRepo(t)
	store := mustRuntimeStore(t, repo, "discharged-failure")
	store.ReviewDisabled = true

	// A failure, then a correction that names it. That correction discharges
	// the binding.
	appendRuntimeLedgerFile(t, repo, "work\n")
	first, err := store.Begin(ctx, BeginAttemptRequest{
		ExpectedRevision: "", RequestID: "df-begin-1", WorkUnit: "format",
		EvidenceGoal: "formatting correction", MaxAttempts: 6, MaxChangedLines: 4000,
	})
	if err != nil {
		t.Fatal(err)
	}
	failedEvidence := runtimeTestHash('a')
	failed, err := store.Finish(ctx, FinishAttemptRequest{
		ExpectedRevision: first.Revision, RequestID: "df-finish-1", Outcome: AttemptFailed,
		EvidenceRevision: failedEvidence, Diagnosis: "formatting verification failed",
		HarnessDisposition: HarnessReused, CleanupEvidence: "cleanup completed", ProcessEvidence: "no descendants",
	})
	if err != nil {
		t.Fatal(err)
	}
	second, err := store.Begin(ctx, BeginAttemptRequest{
		ExpectedRevision: failed.Revision, RequestID: "df-begin-2", WorkUnit: "format",
		EvidenceGoal: "formatting correction", MaxAttempts: 6, MaxChangedLines: 4000,
	})
	if err != nil {
		t.Fatal(err)
	}
	appendRuntimeLedgerFile(t, repo, "first correction slice\n")
	discharged, err := store.Finish(ctx, FinishAttemptRequest{
		ExpectedRevision: second.Revision, RequestID: "df-finish-2", Outcome: AttemptPassed,
		EvidenceRevision: runtimeTestHash('b'), Diagnosis: "first slice passed",
		HarnessDisposition: HarnessReused, CleanupEvidence: "cleanup completed", ProcessEvidence: "no descendants",
		RemediatesEvidenceRevision: failedEvidence,
	})
	if err != nil {
		t.Fatal(err)
	}

	// The reporter's step 4: an explicit audited reset for the next work unit
	// in the same maintainer-approved correction plan.
	reset, err := store.Reset(ctx, ResetObjectiveRequest{
		ExpectedRevision: discharged.Revision, RequestID: "df-reset",
		Reason: "next bounded slice of the same correction plan", Actor: "maintainer",
	})
	if err != nil {
		t.Fatal(err)
	}

	// The next budget-bounded slice, still naming the same failure because it
	// is the same correction plan.
	third, err := store.Begin(ctx, BeginAttemptRequest{
		ExpectedRevision: reset.Revision, RequestID: "df-begin-3", WorkUnit: "format",
		EvidenceGoal: "formatting correction", MaxAttempts: 6, MaxChangedLines: 4000,
	})
	if err != nil {
		t.Fatal(err)
	}
	appendRuntimeLedgerFile(t, repo, "second correction slice\n")
	_, err = store.Finish(ctx, FinishAttemptRequest{
		ExpectedRevision: third.Revision, RequestID: "df-finish-3", Outcome: AttemptPassed,
		EvidenceRevision: runtimeTestHash('c'), Diagnosis: "second slice passed",
		HarnessDisposition: HarnessReused, CleanupEvidence: "cleanup completed", ProcessEvidence: "no descendants",
		RemediatesEvidenceRevision: failedEvidence,
	})
	if err == nil {
		t.Fatal("the second slice settled while claiming to repair a failure that was already discharged")
	}
	message := err.Error()

	// It must say the failure was already repaired, and by which settlement.
	for _, want := range []string{"already", failedEvidence} {
		if !strings.Contains(message, want) {
			t.Fatalf("the refusal does not disclose that the failure was already discharged (missing %q):\n%s", want, message)
		}
	}
	// And it must name the exit, which is to stop claiming a remediation.
	if !strings.Contains(message, "--remediates-evidence-revision") {
		t.Fatalf("the refusal names no continuation; the exit is to settle without --remediates-evidence-revision:\n%s", message)
	}
	// The old text blamed the operator's input for being wrong when it was
	// exactly right, which is what sent #2881's reporter looking for authority
	// to guess at.
	if strings.Contains(message, "requires the current failed evidence and a direct correction attempt") {
		t.Fatalf("the refusal still blames the operator's input rather than naming the discharged state:\n%s", message)
	}
}
