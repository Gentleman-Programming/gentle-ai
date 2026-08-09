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

func TestRuntimeEvidenceOnlyRetrySettlesOnAuditedRescopeAuthority(t *testing.T) {
	repo := initRuntimeLedgerRepo(t)
	store := mustRuntimeStore(t, repo, "evidence-only-rescope-retry")
	store.ReviewDisabled = true
	first, err := store.Begin(context.Background(), BeginAttemptRequest{
		ExpectedRevision: "", RequestID: "rescope-retry-begin-verification", WorkUnit: "verify",
		EvidenceGoal: "independent verification", MaxAttempts: 3, MaxChangedLines: 20,
	})
	if err != nil {
		t.Fatal(err)
	}
	failedEvidence := runtimeTestHash('c')
	failed, err := store.Finish(context.Background(), FinishAttemptRequest{
		ExpectedRevision: first.Revision, RequestID: "rescope-retry-finish-verification", Outcome: AttemptFailed,
		EvidenceRevision: failedEvidence, Diagnosis: "authoritative suite failed a transient test unrelated to the candidate",
		HarnessDisposition: HarnessReused, CleanupEvidence: "verification cleanup completed",
		ProcessEvidence: "verification process scan completed",
	})
	if err != nil {
		t.Fatal(err)
	}
	if failed.DecisionRequired || failed.CumulativeAttempts != 1 || failed.NextAction != RuntimeActionBegin {
		t.Fatalf("failed verification with remaining budget = %#v", failed)
	}
	rescoped, err := store.Rescope(context.Background(), RescopeObjectiveRequest{
		ExpectedRevision: failed.Revision, RequestID: "rescope-retry-audited-rescope",
		WorkUnit: "narrower-verification", EvidenceGoal: "rerun the focused independent verification",
		MaxAttempts: 2, MaxChangedLines: 10,
		Reason: "maintainer narrowed the transient verification failure to one evidence-only retry", Actor: "maintainer",
	})
	if err != nil {
		t.Fatal(err)
	}
	active, err := store.Begin(context.Background(), BeginAttemptRequest{
		ExpectedRevision: rescoped.Revision, RequestID: "rescope-retry-begin-reverification",
		WorkUnit: "narrower-verification", EvidenceGoal: "rerun the focused independent verification",
		MaxAttempts: 2, MaxChangedLines: 10,
	})
	if err != nil {
		t.Fatal(err)
	}
	completed, err := store.Finish(context.Background(), FinishAttemptRequest{
		ExpectedRevision: active.Revision, RequestID: "rescope-retry-finish-reverification", Outcome: AttemptPassed,
		EvidenceRevision: runtimeTestHash('d'), Diagnosis: "focused and full suites passed against the unchanged candidate",
		HarnessDisposition: HarnessReused, CleanupEvidence: "retry cleanup completed",
		ProcessEvidence: "retry process scan completed", RemediatesEvidenceRevision: failedEvidence,
	})
	if err != nil {
		t.Fatalf("rescope-authorized evidence-only retry was refused: %v", err)
	}
	last := completed.Attempts[len(completed.Attempts)-1]
	if !completed.Complete || completed.CumulativeAttempts != 2 || completed.CumulativeChangedLines != 0 ||
		last.RemediatesEvidenceRevision != failedEvidence || last.FinishCandidateTree != last.BeginCandidateTree {
		t.Fatalf("rescope-authorized evidence-only retry = %#v", completed)
	}
	reopened := mustRuntimeStore(t, repo, "evidence-only-rescope-retry")
	reopened.ReviewDisabled = true
	replayed, err := reopened.Status()
	if err != nil || replayed.Revision != completed.Revision || !replayed.Complete {
		t.Fatalf("full-chain replay of rescope-authorized retry = %#v err=%v", replayed, err)
	}
}

func TestRuntimeEvidenceOnlyRetryRescopeAuthorityMustMatchFailure(t *testing.T) {
	failed := RuntimeAttempt{ObjectiveID: "failed-objective", ObjectiveGeneration: 2}
	matching := &RuntimeRescope{
		Actor: "maintainer", Reason: "authorize retry", PreviousObjectiveID: failed.ObjectiveID,
		PreviousGeneration: failed.ObjectiveGeneration, RescopeCandidateTree: "candidate-tree",
	}
	tests := []struct {
		name      string
		authority *RuntimeRescope
		tree      string
	}{
		{name: "missing authority", authority: nil, tree: "candidate-tree"},
		{name: "stale objective", authority: func() *RuntimeRescope {
			value := *matching
			value.PreviousObjectiveID = "older-objective"
			return &value
		}(), tree: "candidate-tree"},
		{name: "stale generation", authority: func() *RuntimeRescope { value := *matching; value.PreviousGeneration--; return &value }(), tree: "candidate-tree"},
		{name: "mismatched candidate tree", authority: matching, tree: "other-tree"},
		{name: "missing actor", authority: func() *RuntimeRescope { value := *matching; value.Actor = ""; return &value }(), tree: "candidate-tree"},
		{name: "missing reason", authority: func() *RuntimeRescope { value := *matching; value.Reason = ""; return &value }(), tree: "candidate-tree"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if runtimeEvidenceOnlyRetryAuthorized(RuntimeStatus{LastRescope: tt.authority}, failed, tt.tree) {
				t.Fatal("mismatched rescope authority authorized an evidence-only retry")
			}
		})
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
