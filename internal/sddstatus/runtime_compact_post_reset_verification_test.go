package sddstatus

import (
	"context"
	"testing"
)

// TestCompactSettlePassesFinalVerificationAfterResetAndDischargedRemediation
// is the #3547 shape end to end at the compact surface: a failed attempt, a
// passed remediation that discharges it, an authorized reset opening a fresh
// independent-verification objective, then one verification attempt settled
// as passed WITHOUT a remediation binding. The final settlement must record
// the passed outcome instead of blocking with invalid_continuation.
func TestCompactSettlePassesFinalVerificationAfterResetAndDischargedRemediation(t *testing.T) {
	repo := initRuntimeLedgerRepo(t)
	store, err := OpenRuntimeStore(context.Background(), repo, "post-reset-verification")
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()

	acquire := func(requestID, workUnit, goal, remediates string) CompactAttemptResult {
		t.Helper()
		result, err := store.Acquire(ctx, CompactAcquireRequest{
			BeginAttemptRequest: BeginAttemptRequest{
				RequestID: requestID, WorkUnit: workUnit, EvidenceGoal: goal,
				MaxAttempts: 3, MaxChangedLines: 400,
			},
			RemediatesEvidenceRevision: remediates,
		})
		if err != nil {
			t.Fatalf("acquire %s: %v", requestID, err)
		}
		if result.State != CompactStateProceed {
			t.Fatalf("acquire %s = %#v, want proceed", requestID, result)
		}
		return result
	}
	settle := func(requestID, token string, outcome AttemptOutcome, evidence, remediates string) CompactAttemptResult {
		t.Helper()
		result, err := store.Settle(ctx, CompactSettleRequest{
			RequestID: requestID, Token: token, Outcome: outcome,
			EvidenceRevision: evidence, Diagnosis: "settlement for " + requestID,
			HarnessDisposition: HarnessReused, CleanupEvidence: "workspace cleanup completed",
			ProcessEvidence:            "process scan found no descendants",
			RemediatesEvidenceRevision: remediates,
		})
		if err != nil {
			t.Fatalf("settle %s: %v", requestID, err)
		}
		return result
	}

	// 1. Ordinary failed attempt records the chain's failed evidence.
	first := acquire("post-reset-apply-acquire", "payments-apply", "implement payments", "")
	appendRuntimeLedgerFile(t, repo, "implementation that failed verification\n")
	failed := settle("post-reset-apply-settle", first.Token, AttemptFailed, runtimeTestHash('f'), "")
	if failed.State != CompactStateProceed && failed.State != CompactStateBlocked {
		t.Fatalf("failed settle = %#v", failed)
	}

	// 2. A passed remediation discharges that failure.
	remediation := acquire("post-reset-remediation-acquire", "payments-apply", "implement payments", runtimeTestHash('f'))
	appendRuntimeLedgerFile(t, repo, "correction that converged\n")
	discharged := settle("post-reset-remediation-settle", remediation.Token, AttemptPassed, runtimeTestHash('b'), runtimeTestHash('f'))
	if discharged.State != CompactStateComplete && discharged.State != CompactStateProceed {
		t.Fatalf("remediation settle = %#v", discharged)
	}

	// 3. Authorized reset opens a fresh final independent-verification scope.
	status, err := store.Status()
	if err != nil {
		t.Fatal(err)
	}
	reset, err := store.Reset(ctx, ResetObjectiveRequest{
		ExpectedRevision: status.Revision, RequestID: "post-reset-authorization",
		Reason: "maintainer authorized a fresh final independent verification", Actor: "maintainer",
	})
	if err != nil {
		t.Fatalf("authorized reset: %v", err)
	}
	if reset.Objective != nil || reset.Complete {
		t.Fatalf("reset state = %#v", reset)
	}

	// 4. One verification attempt without any remediation binding.
	verification := acquire("post-reset-verify-acquire", "final-verification", "independently verify the change", "")
	appendRuntimeLedgerFile(t, repo, "conventional verification report\n")

	// 5. The truthful passed settlement of that verification must be admitted.
	final := settle("post-reset-verify-settle", verification.Token, AttemptPassed, runtimeTestHash('c'), "")
	if final.State == CompactStateBlocked {
		t.Fatalf("final verification settle blocked: reason=%q exit=%q", final.Reason, final.Exit)
	}
	if final.State != CompactStateComplete && final.State != CompactStateProceed {
		t.Fatalf("final verification settle = %#v", final)
	}

	verified, err := store.Status()
	if err != nil {
		t.Fatal(err)
	}
	last := verified.Attempts[len(verified.Attempts)-1]
	if last.Outcome != AttemptPassed || last.EvidenceRevision != runtimeTestHash('c') || last.RemediatesEvidenceRevision != "" {
		t.Fatalf("final verification record = %#v", last)
	}
	if !verified.Complete {
		t.Fatalf("final verification did not complete its objective: %#v", verified)
	}
}
