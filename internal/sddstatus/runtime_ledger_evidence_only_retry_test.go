package sddstatus

import (
	"context"
	"strings"
	"testing"
)

// Issue #2621: a verification failure is not always candidate-caused. When the
// authoritative suite fails on a transient defect unrelated to the candidate,
// the honest correction is to rerun the verification, not to edit bytes that
// were never wrong. The ledger demanded a changed correction candidate for
// every unmanaged remediation, so the retry could not settle at all: the
// disabled-review guard demanded --remediates-evidence-revision, and supplying
// it then hit "unmanaged remediation requires a changed correction candidate".
//
// The maintainer-audited reset already carries the authority that resolves it.
// Reset records the actor, the reason, and the exact candidate tree it was
// taken against, so a reset that terminated the failing objective while the
// candidate stood at these bytes IS an explicit authorization for one
// evidence-only retry of those bytes. Nothing else relaxes: review stays
// disabled with no binding, the failed evidence stays linked through the
// chain, and the corrected evidence must still be fresh and distinct.
func TestRuntimeEvidenceOnlyRetrySettlesOnAuditedResetAuthority(t *testing.T) {
	repo := initRuntimeLedgerRepo(t)
	store := mustRuntimeStore(t, repo, "evidence-only-retry")
	store.ReviewDisabled = true
	first, err := store.Begin(context.Background(), BeginAttemptRequest{
		ExpectedRevision: "", RequestID: "retry-begin-verification", WorkUnit: "verify",
		EvidenceGoal: "independent verification", MaxAttempts: 1, MaxChangedLines: 0,
	})
	if err != nil {
		t.Fatal(err)
	}
	failedEvidence := runtimeTestHash('a')
	failed, err := store.Finish(context.Background(), FinishAttemptRequest{
		ExpectedRevision: first.Revision, RequestID: "retry-finish-verification", Outcome: AttemptFailed,
		EvidenceRevision: failedEvidence, Diagnosis: "authoritative suite failed one transient test unrelated to the candidate",
		HarnessDisposition: HarnessReused, CleanupEvidence: "verification cleanup completed",
		ProcessEvidence: "verification process scan completed",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !failed.DecisionRequired || failed.NextAction != RuntimeActionReset {
		t.Fatalf("exhausted failed verification = %#v", failed)
	}
	reset, err := store.Reset(context.Background(), ResetObjectiveRequest{
		ExpectedRevision: failed.Revision, RequestID: "retry-audited-reset",
		Reason: "maintainer decision: the failure was a transient unrelated test, authorize one evidence-only retry",
		Actor:  "maintainer",
	})
	if err != nil {
		t.Fatal(err)
	}
	active, err := store.Begin(context.Background(), BeginAttemptRequest{
		ExpectedRevision: reset.Revision, RequestID: "retry-begin-reverification", WorkUnit: "verify",
		EvidenceGoal: "independent verification", MaxAttempts: 1, MaxChangedLines: 0,
	})
	if err != nil {
		t.Fatal(err)
	}
	// No appendRuntimeLedgerFile: the candidate is deliberately untouched --
	// that is the whole point of an evidence-only retry.
	request := FinishAttemptRequest{
		ExpectedRevision: active.Revision, RequestID: "retry-finish-reverification", Outcome: AttemptPassed,
		EvidenceRevision: runtimeTestHash('b'), Diagnosis: "full and focused suites passed against the unchanged candidate",
		HarnessDisposition: HarnessReused, CleanupEvidence: "retry cleanup completed",
		ProcessEvidence: "retry process scan completed", RemediatesEvidenceRevision: failedEvidence,
	}
	completed, err := store.Finish(context.Background(), request)
	if err != nil {
		t.Fatalf("authorized evidence-only retry was refused: %v", err)
	}
	if !completed.Complete || completed.ActiveAttempt != nil || completed.Binding != nil {
		t.Fatalf("authorized evidence-only retry = %#v", completed)
	}
	// The failed evidence stays preserved and linked; no approval is invented.
	last := completed.Attempts[len(completed.Attempts)-1]
	if last.RemediatesEvidenceRevision != failedEvidence || last.FinishCandidateTree != last.BeginCandidateTree {
		t.Fatalf("evidence-only retry did not link the failed evidence to the unchanged candidate: %#v", last)
	}
	if completed.Attempts[0].Outcome != AttemptFailed || completed.Attempts[0].EvidenceRevision != failedEvidence {
		t.Fatalf("evidence-only retry rewrote the original failed evidence: %#v", completed.Attempts[0])
	}
	// The replay twin must admit exactly what the write guard admitted.
	reopened := mustRuntimeStore(t, repo, "evidence-only-retry")
	replayed, err := reopened.Status()
	if err != nil || replayed.Revision != completed.Revision || !replayed.Complete {
		t.Fatalf("full-chain replay of the evidence-only retry = %#v err=%v", replayed, err)
	}
}

// The waiver is scoped to the audited reset that authorized it. A failure with
// attempts still available needs no reset, so an unchanged-candidate pass there
// is the laundering the original demand exists to refuse, and it must stay
// refused with its exact message.
func TestRuntimeUnchangedRetryWithoutAuditedResetStaysRefused(t *testing.T) {
	repo := initRuntimeLedgerRepo(t)
	store := mustRuntimeStore(t, repo, "unauthorized-retry")
	store.ReviewDisabled = true
	objective := BeginAttemptRequest{
		ExpectedRevision: "", RequestID: "unauthorized-begin-verification", WorkUnit: "verify",
		EvidenceGoal: "independent verification", MaxAttempts: 2, MaxChangedLines: 20,
	}
	first, err := store.Begin(context.Background(), objective)
	if err != nil {
		t.Fatal(err)
	}
	failedEvidence := runtimeTestHash('a')
	failed, err := store.Finish(context.Background(), FinishAttemptRequest{
		ExpectedRevision: first.Revision, RequestID: "unauthorized-finish-verification", Outcome: AttemptFailed,
		EvidenceRevision: failedEvidence, Diagnosis: "independent verification found a correctable defect",
		HarnessDisposition: HarnessReused, CleanupEvidence: "verification cleanup completed",
		ProcessEvidence: "verification process scan completed",
	})
	if err != nil {
		t.Fatal(err)
	}
	retry := objective
	retry.ExpectedRevision = failed.Revision
	retry.RequestID = "unauthorized-begin-retry"
	active, err := store.Begin(context.Background(), retry)
	if err != nil {
		t.Fatal(err)
	}
	before := countRuntimeRecords(t, store.Dir)
	_, err = store.Finish(context.Background(), FinishAttemptRequest{
		ExpectedRevision: active.Revision, RequestID: "unauthorized-finish-retry", Outcome: AttemptPassed,
		EvidenceRevision: runtimeTestHash('b'), Diagnosis: "unchanged candidate claims a correction with no authorization",
		HarnessDisposition: HarnessReused, CleanupEvidence: "retry cleanup completed",
		ProcessEvidence: "retry process scan completed", RemediatesEvidenceRevision: failedEvidence,
	})
	if err == nil || !strings.Contains(err.Error(), "unmanaged remediation requires a changed correction candidate") {
		t.Fatalf("unauthorized unchanged retry = %v, want the changed-candidate refusal", err)
	}
	status, statusErr := store.Status()
	if statusErr != nil || status.Revision != active.Revision || status.ActiveAttempt == nil ||
		countRuntimeRecords(t, store.Dir) != before {
		t.Fatalf("unauthorized refusal mutated runtime: status=%#v err=%v records=%d", status, statusErr, countRuntimeRecords(t, store.Dir))
	}
}

func TestRuntimeEvidenceOnlyRetrySettlesOnAuditedRescopeAuthority(t *testing.T) {
	repo := initRuntimeLedgerRepo(t)
	store := mustRuntimeStore(t, repo, "rescope-evidence-only-retry")
	store.ReviewDisabled = true

	first, err := store.Begin(context.Background(), BeginAttemptRequest{
		ExpectedRevision: "", RequestID: "rescope-retry-begin-a", WorkUnit: "verify-objective-a",
		EvidenceGoal: "independent verification of objective A", MaxAttempts: 2, MaxChangedLines: 20,
	})
	if err != nil {
		t.Fatal(err)
	}
	failedEvidence := runtimeTestHash('c')
	failed, err := store.Finish(context.Background(), FinishAttemptRequest{
		ExpectedRevision: first.Revision, RequestID: "rescope-retry-finish-a", Outcome: AttemptFailed,
		EvidenceRevision: failedEvidence, Diagnosis: "objective A verification failed with the candidate unchanged",
		HarnessDisposition: HarnessReused, CleanupEvidence: "verification cleanup completed",
		ProcessEvidence: "verification process scan completed",
	})
	if err != nil {
		t.Fatal(err)
	}
	if failed.DecisionRequired || failed.NextAction != RuntimeActionBegin || failed.CumulativeAttempts != 1 {
		t.Fatalf("failed objective A did not retain unused capacity: %#v", failed)
	}
	failedObjective := *failed.Objective
	failedCandidateTree := failed.Attempts[0].FinishCandidateTree

	rescoped, err := store.Rescope(context.Background(), RescopeObjectiveRequest{
		ExpectedRevision: failed.Revision, RequestID: "rescope-retry-a-to-b",
		WorkUnit: "runtime-proof-objective-b", EvidenceGoal: "rerun evidence for objective A",
		MaxAttempts: 2, MaxChangedLines: 10,
		Reason: "maintainer authorized a narrower runtime proof for the unchanged candidate", Actor: "maintainer",
	})
	if err != nil {
		t.Fatal(err)
	}
	if rescoped.LastRescope == nil || rescoped.LastRescope.PreviousObjectiveID != failedObjective.ID ||
		rescoped.LastRescope.PreviousGeneration != failedObjective.Generation ||
		rescoped.LastRescope.RescopeCandidateTree != failedCandidateTree {
		t.Fatalf("audited A-to-B rescope did not preserve the failed authority facts: %#v", rescoped.LastRescope)
	}

	active, err := store.Begin(context.Background(), BeginAttemptRequest{
		ExpectedRevision: rescoped.Revision, RequestID: "rescope-retry-begin-b", WorkUnit: "runtime-proof-objective-b",
		EvidenceGoal: "rerun evidence for objective A", MaxAttempts: 2, MaxChangedLines: 10,
	})
	if err != nil {
		t.Fatal(err)
	}
	completed, err := store.Finish(context.Background(), FinishAttemptRequest{
		ExpectedRevision: active.Revision, RequestID: "rescope-retry-finish-b", Outcome: AttemptPassed,
		EvidenceRevision: runtimeTestHash('d'), Diagnosis: "fresh runtime proof passed with the candidate unchanged",
		HarnessDisposition: HarnessReused, CleanupEvidence: "runtime proof cleanup completed",
		ProcessEvidence: "runtime proof process scan completed", RemediatesEvidenceRevision: failedEvidence,
	})
	if err != nil {
		t.Fatalf("rescope-authorized evidence-only retry was refused: %v", err)
	}
	if !completed.Complete || completed.ActiveAttempt != nil || completed.Binding != nil {
		t.Fatalf("rescope-authorized evidence-only retry = %#v", completed)
	}
	last := completed.Attempts[len(completed.Attempts)-1]
	if last.RemediatesEvidenceRevision != failedEvidence || last.BeginCandidateTree != failedCandidateTree ||
		last.FinishCandidateTree != failedCandidateTree {
		t.Fatalf("rescope-authorized retry did not preserve the evidence link and candidate tree: %#v", last)
	}

	reopened := mustRuntimeStore(t, repo, "rescope-evidence-only-retry")
	replayed, err := reopened.Status()
	if err != nil || replayed.Revision != completed.Revision || !replayed.Complete || replayed.LastRescope == nil {
		t.Fatalf("full-chain replay of the rescope-authorized retry = %#v err=%v", replayed, err)
	}
}

func TestRuntimeRescopeBeforeObjectiveBFailureDoesNotAuthorizeEvidenceOnlyRetry(t *testing.T) {
	repo := initRuntimeLedgerRepo(t)
	store := mustRuntimeStore(t, repo, "rescope-precedes-failure")
	store.ReviewDisabled = true

	first, err := store.Begin(context.Background(), BeginAttemptRequest{
		ExpectedRevision: "", RequestID: "precedes-begin-a", WorkUnit: "verify-objective-a",
		EvidenceGoal: "independent verification of objective A", MaxAttempts: 3, MaxChangedLines: 20,
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = store.Finish(context.Background(), FinishAttemptRequest{
		ExpectedRevision: first.Revision, RequestID: "precedes-finish-a", Outcome: AttemptFailed,
		EvidenceRevision: runtimeTestHash('e'), Diagnosis: "objective A verification failed with the candidate unchanged",
		HarnessDisposition: HarnessReused, CleanupEvidence: "verification cleanup completed",
		ProcessEvidence: "verification process scan completed",
	})
	if err != nil {
		t.Fatal(err)
	}
	failedA, err := store.Status()
	if err != nil {
		t.Fatal(err)
	}
	rescoped, err := store.Rescope(context.Background(), RescopeObjectiveRequest{
		ExpectedRevision: failedA.Revision, RequestID: "precedes-a-to-b",
		WorkUnit: "verify-objective-b", EvidenceGoal: "independent verification of objective B",
		MaxAttempts: 3, MaxChangedLines: 10,
		Reason: "maintainer narrowed the objective after objective A", Actor: "maintainer",
	})
	if err != nil {
		t.Fatal(err)
	}
	second, err := store.Begin(context.Background(), BeginAttemptRequest{
		ExpectedRevision: rescoped.Revision, RequestID: "precedes-begin-b-failure", WorkUnit: "verify-objective-b",
		EvidenceGoal: "independent verification of objective B", MaxAttempts: 3, MaxChangedLines: 10,
	})
	if err != nil {
		t.Fatal(err)
	}
	failedBEvidence := runtimeTestHash('f')
	failedB, err := store.Finish(context.Background(), FinishAttemptRequest{
		ExpectedRevision: second.Revision, RequestID: "precedes-finish-b-failure", Outcome: AttemptFailed,
		EvidenceRevision: failedBEvidence, Diagnosis: "objective B verification failed with the candidate unchanged",
		HarnessDisposition: HarnessReused, CleanupEvidence: "verification cleanup completed",
		ProcessEvidence: "verification process scan completed",
	})
	if err != nil {
		t.Fatal(err)
	}
	retry, err := store.Begin(context.Background(), BeginAttemptRequest{
		ExpectedRevision: failedB.Revision, RequestID: "precedes-begin-b-retry", WorkUnit: "verify-objective-b",
		EvidenceGoal: "independent verification of objective B", MaxAttempts: 3, MaxChangedLines: 10,
	})
	if err != nil {
		t.Fatal(err)
	}
	before := countRuntimeRecords(t, store.Dir)
	_, err = store.Finish(context.Background(), FinishAttemptRequest{
		ExpectedRevision: retry.Revision, RequestID: "precedes-finish-b-retry", Outcome: AttemptPassed,
		EvidenceRevision: runtimeTestHash('a'), Diagnosis: "unchanged objective B retry claims a correction from the older rescope",
		HarnessDisposition: HarnessReused, CleanupEvidence: "retry cleanup completed",
		ProcessEvidence: "retry process scan completed", RemediatesEvidenceRevision: failedBEvidence,
	})
	if err == nil || !strings.Contains(err.Error(), "unmanaged remediation requires a changed correction candidate") {
		t.Fatalf("older rescope authorized objective B retry = %v, want the changed-candidate refusal", err)
	}
	status, statusErr := store.Status()
	if statusErr != nil || status.Revision != retry.Revision || status.ActiveAttempt == nil ||
		countRuntimeRecords(t, store.Dir) != before {
		t.Fatalf("older-rescope refusal mutated or damaged runtime state: status=%#v err=%v records=%d", status, statusErr, countRuntimeRecords(t, store.Dir))
	}
}
