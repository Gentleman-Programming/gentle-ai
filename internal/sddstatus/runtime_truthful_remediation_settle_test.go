package sddstatus

import (
	"context"
	"strings"
	"testing"
)

const (
	truthfulWorkUnit    = "payments-retry-implementation"
	truthfulGoal        = "implement bounded payment retries"
	truthfulMaxAttempts = 4
	truthfulMaxLines    = 400
)

// newTruthfulRemediationChain settles one ordinary failed attempt so every
// test starts from the exact state #3422 reproduces: an immutable chain whose
// unremediated head is the failed evidence a bounded remediation binds.
func newTruthfulRemediationChain(t *testing.T) (string, RuntimeStore, RuntimeStatus) {
	t.Helper()
	repo := initRuntimeLedgerRepo(t)
	store, err := OpenRuntimeStore(context.Background(), repo, "truthful-remediation")
	if err != nil {
		t.Fatal(err)
	}
	started, err := store.Begin(context.Background(), BeginAttemptRequest{
		ExpectedRevision: "", RequestID: "truthful-apply-begin", WorkUnit: truthfulWorkUnit,
		EvidenceGoal: truthfulGoal, MaxAttempts: truthfulMaxAttempts, MaxChangedLines: truthfulMaxLines,
	})
	if err != nil {
		t.Fatal(err)
	}
	appendRuntimeLedgerFile(t, repo, "first implementation slice\n")
	failed, err := store.Finish(context.Background(), FinishAttemptRequest{
		ExpectedRevision: started.Revision, RequestID: "truthful-apply-finish", Outcome: AttemptFailed,
		EvidenceRevision: runtimeTestHash('f'), Diagnosis: "verification failed on retry backoff",
		HarnessDisposition: HarnessReused, CleanupEvidence: "workspace cleanup completed",
		ProcessEvidence: "process scan found no descendants",
	})
	if err != nil {
		t.Fatal(err)
	}
	if failed.Complete || failed.ActiveAttempt != nil {
		t.Fatalf("failed predecessor settlement = %#v", failed)
	}
	return repo, store, failed
}

func beginTruthfulRemediation(t *testing.T, store RuntimeStore, previous RuntimeStatus, requestID string) RuntimeStatus {
	t.Helper()
	started, err := store.Begin(context.Background(), BeginAttemptRequest{
		ExpectedRevision: previous.Revision, RequestID: requestID, WorkUnit: truthfulWorkUnit,
		EvidenceGoal: truthfulGoal, MaxAttempts: truthfulMaxAttempts, MaxChangedLines: truthfulMaxLines,
	})
	if err != nil {
		t.Fatal(err)
	}
	return started
}

// TestRuntimeFinishRecordsTruthfulFailedRemediation is #3422: a bounded
// remediation whose verification genuinely still fails must settle with a
// truthful failed outcome. Refusing it leaves the consumer with no terminal
// transition at all — the attempt stays active while verification and archive
// stay blocked.
func TestRuntimeFinishRecordsTruthfulFailedRemediation(t *testing.T) {
	repo, store, failed := newTruthfulRemediationChain(t)
	started := beginTruthfulRemediation(t, store, failed, "truthful-remediation-begin")
	appendRuntimeLedgerFile(t, repo, "attempted correction that did not converge\n")

	settled, err := store.Finish(context.Background(), FinishAttemptRequest{
		ExpectedRevision: started.Revision, RequestID: "truthful-remediation-finish", Outcome: AttemptFailed,
		EvidenceRevision: runtimeTestHash('b'), Diagnosis: "runtime environment blocker persisted",
		HarnessDisposition: HarnessInvalidated, CleanupEvidence: "workspace cleanup completed",
		ProcessEvidence:            "process scan found no descendants",
		RemediatesEvidenceRevision: runtimeTestHash('f'),
	})
	if err != nil {
		t.Fatalf("truthful failed remediation settle refused: %v", err)
	}
	if settled.ActiveAttempt != nil {
		t.Fatalf("failed remediation did not close the attempt: %#v", settled.ActiveAttempt)
	}
	if settled.Complete {
		t.Fatalf("failed remediation marked the objective complete: %#v", settled)
	}
	last := settled.Attempts[len(settled.Attempts)-1]
	if last.Outcome != AttemptFailed || last.EvidenceRevision != runtimeTestHash('b') ||
		last.RemediatesEvidenceRevision != runtimeTestHash('f') {
		t.Fatalf("failed remediation record = %#v", last)
	}

	// The chain's unremediated head is now the remediation's own failure, so
	// the next correction binds the new evidence, not the discharged original.
	if !failedEvidenceRemediationSettleable(settled, runtimeTestHash('b')) {
		t.Fatalf("next remediation cannot bind the new failed evidence: %#v", settled.Attempts)
	}
	if failedEvidenceRemediationSettleable(settled, runtimeTestHash('f')) {
		t.Fatal("original failed evidence stayed bindable past its failed correction")
	}

	// The persisted chain must replay: a committed truthful record that the
	// replay twin rejects would make the whole ledger unreadable.
	reopened, err := OpenRuntimeStore(context.Background(), repo, "truthful-remediation")
	if err != nil {
		t.Fatal(err)
	}
	replayed, err := reopened.Status()
	if err != nil {
		t.Fatalf("chain with a truthful failed remediation does not replay: %v", err)
	}
	if len(replayed.Attempts) != len(settled.Attempts) {
		t.Fatalf("replayed attempts = %d, want %d", len(replayed.Attempts), len(settled.Attempts))
	}

	// A later passed correction of the NEW failure still works: the truthful
	// failed record is an ordinary predecessor, not a dead end.
	restarted := beginTruthfulRemediation(t, store, settled, "truthful-second-remediation-begin")
	appendRuntimeLedgerFile(t, repo, "second correction that converged\n")
	passed, err := store.Finish(context.Background(), FinishAttemptRequest{
		ExpectedRevision: restarted.Revision, RequestID: "truthful-second-remediation-finish", Outcome: AttemptPassed,
		EvidenceRevision: runtimeTestHash('c'), Diagnosis: "verification passed after correction",
		HarnessDisposition: HarnessReused, CleanupEvidence: "workspace cleanup completed",
		ProcessEvidence:            "process scan found no descendants",
		RemediatesEvidenceRevision: runtimeTestHash('b'),
	})
	if err != nil {
		t.Fatalf("passed correction after a truthful failed remediation refused: %v", err)
	}
	if !passed.Complete {
		t.Fatalf("passed correction did not complete the objective: %#v", passed)
	}
}

// TestRuntimeFinishRecordsTruthfulInterruptedRemediation holds the same
// principle for the third outcome: an interrupted remediation settles
// truthfully, discharges nothing, and leaves the original failed evidence as
// the chain's bindable head.
func TestRuntimeFinishRecordsTruthfulInterruptedRemediation(t *testing.T) {
	repo, store, failed := newTruthfulRemediationChain(t)
	started := beginTruthfulRemediation(t, store, failed, "truthful-interrupted-begin")
	appendRuntimeLedgerFile(t, repo, "partial correction before interruption\n")

	settled, err := store.Finish(context.Background(), FinishAttemptRequest{
		ExpectedRevision: started.Revision, RequestID: "truthful-interrupted-finish", Outcome: AttemptInterrupted,
		Diagnosis:          "operator interrupted the correction run",
		HarnessDisposition: HarnessInvalidated, CleanupEvidence: "workspace cleanup completed",
		ProcessEvidence:            "process scan found no descendants",
		RemediatesEvidenceRevision: runtimeTestHash('f'),
	})
	if err != nil {
		t.Fatalf("truthful interrupted remediation settle refused: %v", err)
	}
	if settled.ActiveAttempt != nil || settled.Complete {
		t.Fatalf("interrupted remediation state = %#v", settled)
	}
	if !failedEvidenceRemediationSettleable(settled, runtimeTestHash('f')) {
		t.Fatal("interrupted remediation discharged the failed evidence it never repaired")
	}
}

// TestRuntimeFinishKeepsPassedRemediationGuards proves the fix loosens nothing
// for passing corrections: the changed-candidate demand, the fresh-evidence
// demand, and the exact chain binding all still refuse.
func TestRuntimeFinishKeepsPassedRemediationGuards(t *testing.T) {
	t.Run("unchanged candidate still refused", func(t *testing.T) {
		_, store, failed := newTruthfulRemediationChain(t)
		started := beginTruthfulRemediation(t, store, failed, "guard-unchanged-begin")
		_, err := store.Finish(context.Background(), FinishAttemptRequest{
			ExpectedRevision: started.Revision, RequestID: "guard-unchanged-finish", Outcome: AttemptPassed,
			EvidenceRevision: runtimeTestHash('b'), Diagnosis: "claims to pass without changes",
			HarnessDisposition: HarnessReused, CleanupEvidence: "workspace cleanup completed",
			ProcessEvidence:            "process scan found no descendants",
			RemediatesEvidenceRevision: runtimeTestHash('f'),
		})
		if err == nil || !strings.Contains(err.Error(), "changed correction candidate") {
			t.Fatalf("unchanged passing correction = %v, want changed-candidate refusal", err)
		}
	})

	t.Run("stale evidence still refused", func(t *testing.T) {
		repo, store, failed := newTruthfulRemediationChain(t)
		started := beginTruthfulRemediation(t, store, failed, "guard-stale-begin")
		appendRuntimeLedgerFile(t, repo, "changed candidate with stale evidence\n")
		_, err := store.Finish(context.Background(), FinishAttemptRequest{
			ExpectedRevision: started.Revision, RequestID: "guard-stale-finish", Outcome: AttemptPassed,
			EvidenceRevision: runtimeTestHash('f'), Diagnosis: "reuses the failed evidence",
			HarnessDisposition: HarnessReused, CleanupEvidence: "workspace cleanup completed",
			ProcessEvidence:            "process scan found no descendants",
			RemediatesEvidenceRevision: runtimeTestHash('f'),
		})
		if err == nil || !strings.Contains(err.Error(), "fresh corrected evidence") {
			t.Fatalf("stale passing evidence = %v, want fresh-evidence refusal", err)
		}
	})

	t.Run("wrong binding still refused for a failed outcome", func(t *testing.T) {
		repo, store, failed := newTruthfulRemediationChain(t)
		started := beginTruthfulRemediation(t, store, failed, "guard-binding-begin")
		appendRuntimeLedgerFile(t, repo, "correction bound to the wrong failure\n")
		_, err := store.Finish(context.Background(), FinishAttemptRequest{
			ExpectedRevision: started.Revision, RequestID: "guard-binding-finish", Outcome: AttemptFailed,
			EvidenceRevision: runtimeTestHash('b'), Diagnosis: "still failing",
			HarnessDisposition: HarnessReused, CleanupEvidence: "workspace cleanup completed",
			ProcessEvidence:            "process scan found no descendants",
			RemediatesEvidenceRevision: runtimeTestHash('d'),
		})
		if err == nil || !strings.Contains(err.Error(), "unremediated failure") {
			t.Fatalf("wrong failed-outcome binding = %v, want exact-binding refusal", err)
		}
	})
}
