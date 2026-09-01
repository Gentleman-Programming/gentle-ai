package sddstatus

import (
	"context"
	"strings"
	"testing"
)

// #2564 (slice 1 of #1974): remediation intent becomes visible at acquire
// through RemediatesEvidenceRevision, and an intent whose settlement is
// structurally unsatisfiable earns a typed refusal BEFORE any token is issued
// and any correction work runs. Satisfiability follows the chain-derived
// binding (#2565): the immutable attempt chain must hold the declared failed
// evidence unremediated. An audited reset does not sever that binding, so a
// post-reset correction acquire stays legitimate; a chain with nothing failed,
// an already-corrected failure, or a different declared revision refuses.

// seedFailedVerificationLedger drives the shared ledger fixture: a disabled
// store whose first verification attempt settled as failed with maxAttempts
// bounding the objective. It returns the store, the failed evidence revision,
// and the runtime status after the finish.
func seedFailedVerificationLedger(t *testing.T, repo, change string, maxAttempts int) (RuntimeStore, string, RuntimeStatus) {
	t.Helper()
	store := mustRuntimeStore(t, repo, change)
	store.ReviewDisabled = true
	begun, err := store.Begin(context.Background(), BeginAttemptRequest{
		RequestID: change + "-begin-verification", WorkUnit: "verify",
		EvidenceGoal: "independent verification", MaxAttempts: maxAttempts, MaxChangedLines: 20,
	})
	if err != nil {
		t.Fatal(err)
	}
	failedEvidence := runtimeTestHash('a')
	failed, err := store.Finish(context.Background(), FinishAttemptRequest{
		ExpectedRevision: begun.Revision, RequestID: change + "-finish-verification", Outcome: AttemptFailed,
		EvidenceRevision: failedEvidence, Diagnosis: "independent verification found a correctable defect",
		HarnessDisposition: HarnessReused, CleanupEvidence: "verification cleanup completed",
		ProcessEvidence: "verification process scan completed",
	})
	if err != nil {
		t.Fatal(err)
	}
	return store, failedEvidence, failed
}

// TestCompactAcquireRemediationIntentSurvivesAuditedReset pins the slice 1 /
// slice 2 reconciliation: the reporter's recipe (disabled review, failed
// verification, audited reset) followed by an acquire that declares the
// chain's failed evidence must issue a token, because the immutable chain
// still binds that evidence even though the reset wiped the live pointer.
func TestCompactAcquireRemediationIntentSurvivesAuditedReset(t *testing.T) {
	repo := initRuntimeLedgerRepo(t)
	store, failedEvidence, failed := seedFailedVerificationLedger(t, repo, "intent-after-reset", 1)
	if !failed.DecisionRequired {
		t.Fatalf("exhausted failed verification did not require a decision: %#v", failed)
	}
	reset, err := store.Reset(context.Background(), ResetObjectiveRequest{
		ExpectedRevision: failed.Revision, RequestID: "intent-reset",
		Reason: "maintainer authorized bounded remediation", Actor: "maintainer",
	})
	if err != nil {
		t.Fatal(err)
	}
	if reset.Objective != nil || reset.EvidenceRevision != "" {
		t.Fatalf("audited reset kept a live objective or evidence binding: %#v", reset)
	}

	// A stale declared evidence still fails fast: the chain binds 'a', not 'c'.
	before := countRuntimeRecords(t, store.Dir)
	stale, err := store.Acquire(context.Background(), CompactAcquireRequest{
		BeginAttemptRequest: BeginAttemptRequest{
			RequestID: "intent-acquire-stale", WorkUnit: "remediation",
			EvidenceGoal: "bounded correction", MaxAttempts: 1, MaxChangedLines: 200,
		},
		RemediatesEvidenceRevision: runtimeTestHash('c'),
	})
	if err != nil {
		t.Fatal(err)
	}
	if stale.State != CompactStateBlocked || stale.Reason != CompactBlockRemediationUnsatisfiable || stale.Token != "" {
		t.Fatalf("stale post-reset remediation-intent acquire = %#v, want blocked/%s", stale, CompactBlockRemediationUnsatisfiable)
	}
	if !strings.Contains(stale.Exit, "gentle-ai sdd-attempt status") || stale.Detail != stale.Exit {
		t.Fatalf("remediation-intent refusal broke the Exit/Detail discipline: %#v", stale)
	}
	if countRuntimeRecords(t, store.Dir) != before {
		t.Fatalf("refused remediation-intent acquire mutated the ledger: before=%d after=%d", before, countRuntimeRecords(t, store.Dir))
	}

	// The chain-bound declaration survives the audited reset and issues the token.
	correction, err := store.Acquire(context.Background(), CompactAcquireRequest{
		BeginAttemptRequest: BeginAttemptRequest{
			RequestID: "intent-acquire-correction", WorkUnit: "remediation",
			EvidenceGoal: "bounded correction", MaxAttempts: 1, MaxChangedLines: 200,
		},
		RemediatesEvidenceRevision: failedEvidence,
	})
	if err != nil {
		t.Fatal(err)
	}
	if correction.State != CompactStateProceed || correction.Token == "" {
		t.Fatalf("post-reset chain-bound remediation-intent acquire = %#v, want proceed", correction)
	}
}

// TestCompactAcquireRemediationIntentStillIssuesTokenForDirectCorrection holds
// the direct path end to end, then proves the anti-laundering fail-fast: once
// the failure's one correction settled, a later acquire declaring the same
// evidence is refused before issuing a token.
func TestCompactAcquireRemediationIntentStillIssuesTokenForDirectCorrection(t *testing.T) {
	repo := initRuntimeLedgerRepo(t)
	store, failedEvidence, _ := seedFailedVerificationLedger(t, repo, "intent-direct-correction", 2)

	correction, err := store.Acquire(context.Background(), CompactAcquireRequest{
		BeginAttemptRequest: BeginAttemptRequest{
			RequestID: "direct-acquire-correction", WorkUnit: "verify",
			EvidenceGoal: "independent verification", MaxAttempts: 2, MaxChangedLines: 20,
		},
		RemediatesEvidenceRevision: failedEvidence,
	})
	if err != nil {
		t.Fatal(err)
	}
	if correction.State != CompactStateProceed || correction.Token == "" {
		t.Fatalf("direct correction acquire with intent = %#v, want proceed", correction)
	}

	appendRuntimeLedgerFile(t, repo, "bounded unmanaged correction\n")
	settled, err := store.Settle(context.Background(), CompactSettleRequest{
		Token: correction.Token, RequestID: "direct-settle-correction", Outcome: AttemptPassed,
		EvidenceRevision: runtimeTestHash('b'), Diagnosis: "bounded correction passed focused checks",
		HarnessDisposition: HarnessReused, CleanupEvidence: "correction cleanup completed",
		ProcessEvidence: "correction process scan completed", RemediatesEvidenceRevision: failedEvidence,
	})
	if err != nil {
		t.Fatal(err)
	}
	if settled.State != CompactStateComplete {
		t.Fatalf("direct correction settle = %#v, want complete", settled)
	}
	status, err := store.Status()
	if err != nil {
		t.Fatal(err)
	}
	last := status.Attempts[len(status.Attempts)-1]
	if !status.Complete || last.RemediatesEvidenceRevision != failedEvidence {
		t.Fatalf("direct correction did not bind the failed evidence: %#v", status)
	}

	// Anti-laundering fail-fast: the passed settlement severed the chain
	// binding, so a fresh scope declaring the same evidence refuses at
	// acquire, with zero mutation.
	reset, err := store.Reset(context.Background(), ResetObjectiveRequest{
		ExpectedRevision: status.Revision, RequestID: "direct-reset-after-completion",
		Reason: "attempt to reopen the settled correction", Actor: "maintainer",
	})
	if err != nil {
		t.Fatal(err)
	}
	before := countRuntimeRecords(t, store.Dir)
	laundered, err := store.Acquire(context.Background(), CompactAcquireRequest{
		BeginAttemptRequest: BeginAttemptRequest{
			RequestID: "direct-acquire-laundered", WorkUnit: "remediation",
			EvidenceGoal: "bounded correction", MaxAttempts: 1, MaxChangedLines: 200,
		},
		RemediatesEvidenceRevision: failedEvidence,
	})
	if err != nil {
		t.Fatal(err)
	}
	if laundered.State != CompactStateBlocked || laundered.Reason != CompactBlockRemediationUnsatisfiable ||
		laundered.Exit == "" || countRuntimeRecords(t, store.Dir) != before {
		t.Fatalf("second correction acquire against settled evidence = %#v records=%d, want blocked without mutation (reset revision %s)", laundered, countRuntimeRecords(t, store.Dir), reset.Revision)
	}
}

// TestCompactAcquireRemediationIntentFailsFastWithNothingToRemediate refuses a
// declared correction when the chain holds no failed evidence at all.
func TestCompactAcquireRemediationIntentFailsFastWithNothingToRemediate(t *testing.T) {
	repo := initRuntimeLedgerRepo(t)
	store := mustRuntimeStore(t, repo, "intent-nothing-failed")
	store.ReviewDisabled = true
	if err := store.ensureDirectories(); err != nil {
		t.Fatal(err)
	}
	result, err := store.Acquire(context.Background(), CompactAcquireRequest{
		BeginAttemptRequest: BeginAttemptRequest{
			RequestID: "nothing-acquire", WorkUnit: "remediation",
			EvidenceGoal: "bounded correction", MaxAttempts: 1, MaxChangedLines: 200,
		},
		RemediatesEvidenceRevision: runtimeTestHash('a'),
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.State != CompactStateBlocked || result.Reason != CompactBlockRemediationUnsatisfiable || result.Token != "" {
		t.Fatalf("nothing-failed remediation-intent acquire = %#v, want blocked/%s", result, CompactBlockRemediationUnsatisfiable)
	}
	if countRuntimeRecords(t, store.Dir) != 0 {
		t.Fatal("refused remediation-intent acquire mutated an empty ledger")
	}
}

// TestCompactAcquireRemediationIntentValidatesRevisionShape mirrors settle's
// shape validation at the new acquire boundary.
func TestCompactAcquireRemediationIntentValidatesRevisionShape(t *testing.T) {
	repo := initRuntimeLedgerRepo(t)
	store := mustRuntimeStore(t, repo, "intent-shape")
	store.ReviewDisabled = true
	_, err := store.Acquire(context.Background(), CompactAcquireRequest{
		BeginAttemptRequest: BeginAttemptRequest{
			RequestID: "shape-acquire", WorkUnit: "verify",
			EvidenceGoal: "independent verification", MaxAttempts: 1, MaxChangedLines: 20,
		},
		RemediatesEvidenceRevision: "not-a-revision",
	})
	if err == nil || !strings.Contains(err.Error(), "--remediates-evidence-revision") {
		t.Fatalf("malformed remediation intent = %v, want a shape refusal naming the flag", err)
	}
	if countRuntimeRecords(t, store.Dir) != 0 {
		t.Fatalf("malformed remediation intent mutated the ledger")
	}
}
