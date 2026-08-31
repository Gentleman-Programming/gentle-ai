package sddstatus

import (
	"context"
	"strings"
	"testing"
)

// #3872, #3879, and #3884 share one root class: a compact ledger answer that
// does not name its runnable exit. Settle collapsed five causes into one
// generic invalid_continuation text, and complete carried no exit at all.

func TestSettleWithoutActiveAttemptNamesAcquire(t *testing.T) {
	store := mustRuntimeStore(t, initRuntimeLedgerRepo(t), "settle-nothing-active")
	result, err := store.Settle(context.Background(), compactSettleFixture("idle-settle", runtimeTestHash('1'), AttemptPassed, ""))
	if err != nil {
		t.Fatal(err)
	}
	assertSettleBlockedExit(t, result, "nothing to settle", "`gentle-ai sdd-attempt acquire --cwd <repo> --change <change>")
}

func TestSettlePassedWithoutRemediatesNamesTheChainEvidence(t *testing.T) {
	store := seedUnremediatedFailure(t, "settle-owes-remediation")
	attempt := acquireRegressionAttempt(t, store, "b-acquire", "sdd-remediate", "correct failed verification A", "", 2)
	result, err := store.Settle(context.Background(), compactSettleFixture("b-settle", attempt.Token, AttemptPassed, ""))
	if err != nil {
		t.Fatal(err)
	}
	assertSettleBlockedExit(t, result, "this passed settle was refused", regressionFailedEvidence, "--remediates-evidence-revision")
}

func TestSettleRemediatesMismatchNamesTheChainEvidence(t *testing.T) {
	other := runtimeTestHash('c')
	store := seedUnremediatedFailure(t, "settle-remediates-mismatch")
	attempt := acquireRegressionAttempt(t, store, "b-acquire", "sdd-remediate", "correct failed verification A", "", 2)
	result, err := store.Settle(context.Background(), compactSettleFixture("b-settle", attempt.Token, AttemptPassed, other))
	if err != nil {
		t.Fatal(err)
	}
	assertSettleBlockedExit(t, result, "not "+other, "--remediates-evidence-revision "+regressionFailedEvidence)

	fresh := mustRuntimeStore(t, initRuntimeLedgerRepo(t), "settle-remediates-nothing")
	attempt = acquireRegressionAttempt(t, fresh, "a-acquire", "sdd-apply", "apply the change", "", 2)
	result, err = fresh.Settle(context.Background(), compactSettleFixture("a-settle", attempt.Token, AttemptPassed, other))
	if err != nil {
		t.Fatal(err)
	}
	assertSettleBlockedExit(t, result, "names nothing", other, "without that flag")
}

// The changed-line and older-binary preconditions cannot be reached through
// the CLI (a finish that exceeds its budget never completes), so they are
// driven through the pure readiness predicate on a constructed ledger state.
func TestCompleteExitNamesEachSuccessorPrecondition(t *testing.T) {
	objective := &RuntimeObjective{ID: runtimeTestHash('o'), WorkUnit: "slice-1"}
	passed := RuntimeAttempt{
		ObjectiveID: objective.ID, Outcome: AttemptPassed,
		FinishCandidateIdentity: runtimeTestHash('i'), FinishCandidateTree: strings.Repeat("a", 40),
	}
	exceeded, unbound := passed, passed
	exceeded.ChangedLineBudgetExceeded = true
	unbound.FinishCandidateIdentity, unbound.FinishCandidateTree = "", ""
	for _, tt := range []struct {
		name     string
		last     RuntimeAttempt
		workUnit string
		want     []string
	}{
		{name: "no work unit", last: passed, want: []string{"(slice-1) is complete", "--work-unit \"<a different label>\""}},
		{name: "same label", last: passed, workUnit: "slice-1", want: []string{"--work-unit \"slice-1\" restates the completed objective; choose a different label"}},
		{name: "budget exceeded", last: exceeded, workUnit: "slice-2", want: []string{"exceeded its changed-line budget", "`gentle-ai sdd-attempt reset --cwd <repo> --change <change>"}},
		{name: "older binary", last: unbound, workUnit: "slice-2", want: []string{"no finish candidate identity", "older binary", "`gentle-ai sdd-attempt reset --cwd <repo> --change <change>"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			status := RuntimeStatus{Complete: true, Objective: objective, Attempts: []RuntimeAttempt{tt.last}}
			result, terminal := runtimeReadiness(runtimeReadinessInput{Status: status, Request: BeginAttemptRequest{WorkUnit: tt.workUnit}})
			if !terminal || result.State != CompactStateComplete || result.Exit == "" || result.Detail != result.Exit {
				t.Fatalf("readiness = %#v terminal=%v, want complete with a named exit", result, terminal)
			}
			for _, want := range tt.want {
				if !strings.Contains(result.Exit, want) {
					t.Fatalf("complete exit does not name %q:\n%s", want, result.Exit)
				}
			}
		})
	}
}

func seedUnremediatedFailure(t *testing.T, change string) RuntimeStore {
	t.Helper()
	store := mustRuntimeStore(t, initRuntimeLedgerRepo(t), change)
	failed := acquireRegressionAttempt(t, store, "a-acquire", "sdd-verify", "record failed verification A", "", 1)
	settleRegressionAttempt(t, store, "a-settle", failed.Token, AttemptFailed, regressionFailedEvidence, "")
	status, err := store.Status()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Reset(context.Background(), ResetObjectiveRequest{
		ExpectedRevision: status.Revision, RequestID: "a-reset", Reason: "maintainer authorizes remediation B", Actor: "maintainer",
	}); err != nil {
		t.Fatalf("authorized reset: %v", err)
	}
	return store
}

func compactSettleFixture(requestID, token string, outcome AttemptOutcome, remediates string) CompactSettleRequest {
	return CompactSettleRequest{
		RequestID: requestID, Token: token, Outcome: outcome, EvidenceRevision: regressionPassedEvidence,
		Diagnosis: "settle exit fixture " + requestID, HarnessDisposition: HarnessReused,
		CleanupEvidence: "fixture has no external resources", ProcessEvidence: "fixture process scan found no descendants",
		RemediatesEvidenceRevision: remediates,
	}
}

func assertSettleBlockedExit(t *testing.T, result CompactAttemptResult, wants ...string) {
	t.Helper()
	if result.State != CompactStateBlocked || result.Reason != CompactBlockInvalidContinuation || result.Detail != result.Exit {
		t.Fatalf("settle = %#v, want blocked(invalid_continuation) with detail mirroring exit", result)
	}
	for _, want := range append(wants, "`gentle-ai sdd-attempt status --cwd <repo> --change <change>`") {
		if !strings.Contains(result.Exit, want) {
			t.Fatalf("settle exit does not name %q:\n%s", want, result.Exit)
		}
	}
}
