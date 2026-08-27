package sddstatus

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Issue #3073 (residual shape, Flow C): the "changed correction candidate"
// judgment compared the settling snapshot only against the correction
// attempt's BEGIN snapshot. When the correction bytes land AFTER the audited
// reset but BEFORE the correction attempt acquires, the begin snapshot
// already contains them, so a candidate genuinely changed relative to the
// state that FAILED was refused as unchanged — and the attempt stuck with no
// recovery transition. The judgment must compare against the remediated
// failed evidence's candidate snapshot: the state the failure was recorded
// over is the laundering baseline, not whatever the retry happened to begin
// from.
func TestRuntimeRemediationSettlesWhenCorrectionPredatesAcquire(t *testing.T) {
	repo := initRuntimeLedgerRepo(t)
	store := mustRuntimeStore(t, repo, "correction-predates-acquire")
	store.ReviewDisabled = true
	first, err := store.Begin(context.Background(), BeginAttemptRequest{
		ExpectedRevision: "", RequestID: "predates-begin-verification", WorkUnit: "verify",
		EvidenceGoal: "independent verification", MaxAttempts: 1, MaxChangedLines: 0,
	})
	if err != nil {
		t.Fatal(err)
	}
	failedEvidence := runtimeTestHash('a')
	failed, err := store.Finish(context.Background(), FinishAttemptRequest{
		ExpectedRevision: first.Revision, RequestID: "predates-finish-verification", Outcome: AttemptFailed,
		EvidenceRevision: failedEvidence, Diagnosis: "independent verification found a correctable defect",
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
		ExpectedRevision: failed.Revision, RequestID: "predates-audited-reset",
		Reason: "maintainer decision: authorize a focused remediation of the failed verification",
		Actor:  "maintainer",
	})
	if err != nil {
		t.Fatal(err)
	}
	// THE TRIGGER: the real tracked correction is applied after the reset and
	// before the correction attempt acquires, so the begin snapshot already
	// carries the corrected bytes.
	appendRuntimeLedgerFile(t, repo, "expect(emptyState).toBeVisible()\n")
	active, err := store.Begin(context.Background(), BeginAttemptRequest{
		ExpectedRevision: reset.Revision, RequestID: "predates-begin-correction", WorkUnit: "correction",
		EvidenceGoal: "focused remediation", MaxAttempts: 1, MaxChangedLines: 0,
	})
	if err != nil {
		t.Fatal(err)
	}
	completed, err := store.Finish(context.Background(), FinishAttemptRequest{
		ExpectedRevision: active.Revision, RequestID: "predates-finish-correction", Outcome: AttemptPassed,
		EvidenceRevision: runtimeTestHash('b'), Diagnosis: "focused and full verification passed against the corrected candidate",
		HarnessDisposition: HarnessReused, CleanupEvidence: "correction cleanup completed",
		ProcessEvidence: "correction process scan completed", RemediatesEvidenceRevision: failedEvidence,
	})
	if err != nil {
		t.Fatalf("correction changed against the failed evidence was refused: %v", err)
	}
	if !completed.Complete || completed.ActiveAttempt != nil {
		t.Fatalf("pre-acquire correction settle = %#v", completed)
	}
	last := completed.Attempts[len(completed.Attempts)-1]
	if last.RemediatesEvidenceRevision != failedEvidence {
		t.Fatalf("correction did not link the failed evidence: %#v", last)
	}
	// The candidate really did change relative to the state that failed, even
	// though it never changed during the correction attempt itself.
	if last.FinishCandidateTree != last.BeginCandidateTree ||
		last.FinishCandidateTree == completed.Attempts[0].FinishCandidateTree {
		t.Fatalf("pre-acquire correction candidate provenance = %#v vs failed %#v", last, completed.Attempts[0])
	}
	// The replay twin must admit exactly what the write guard admitted.
	reopened := mustRuntimeStore(t, repo, "correction-predates-acquire")
	replayed, err := reopened.Status()
	if err != nil || replayed.Revision != completed.Revision || !replayed.Complete {
		t.Fatalf("full-chain replay of the pre-acquire correction = %#v err=%v", replayed, err)
	}
}

// The failed-evidence baseline cuts both ways: a candidate reverted to the
// exact bytes that failed is unchanged relative to the failure no matter how
// much it differs from the correction attempt's begin snapshot, and the
// audited reset taken over DIFFERENT bytes does not authorize it either. The
// begin-relative judgment used to accept this laundering shape.
func TestRuntimeRemediationRevertToFailedBytesStaysRefused(t *testing.T) {
	repo := initRuntimeLedgerRepo(t)
	store := mustRuntimeStore(t, repo, "revert-to-failed-bytes")
	store.ReviewDisabled = true
	first, err := store.Begin(context.Background(), BeginAttemptRequest{
		ExpectedRevision: "", RequestID: "revert-begin-verification", WorkUnit: "verify",
		EvidenceGoal: "independent verification", MaxAttempts: 1, MaxChangedLines: 0,
	})
	if err != nil {
		t.Fatal(err)
	}
	failedEvidence := runtimeTestHash('a')
	failed, err := store.Finish(context.Background(), FinishAttemptRequest{
		ExpectedRevision: first.Revision, RequestID: "revert-finish-verification", Outcome: AttemptFailed,
		EvidenceRevision: failedEvidence, Diagnosis: "independent verification found a correctable defect",
		HarnessDisposition: HarnessReused, CleanupEvidence: "verification cleanup completed",
		ProcessEvidence: "verification process scan completed",
	})
	if err != nil {
		t.Fatal(err)
	}
	// Drift the candidate BEFORE the reset so the reset's audited authority is
	// bound to different bytes than the ones that failed.
	appendRuntimeLedgerFile(t, repo, "unrelated drift\n")
	reset, err := store.Reset(context.Background(), ResetObjectiveRequest{
		ExpectedRevision: failed.Revision, RequestID: "revert-audited-reset",
		Reason: "maintainer decision: authorize a focused remediation of the failed verification",
		Actor:  "maintainer",
	})
	if err != nil {
		t.Fatal(err)
	}
	active, err := store.Begin(context.Background(), BeginAttemptRequest{
		ExpectedRevision: reset.Revision, RequestID: "revert-begin-correction", WorkUnit: "correction",
		EvidenceGoal: "focused remediation", MaxAttempts: 1, MaxChangedLines: 20,
	})
	if err != nil {
		t.Fatal(err)
	}
	// Revert to the exact bytes the failed verification was recorded over.
	if err := os.WriteFile(filepath.Join(repo, "tracked.txt"), []byte("base\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	before := countRuntimeRecords(t, store.Dir)
	_, err = store.Finish(context.Background(), FinishAttemptRequest{
		ExpectedRevision: active.Revision, RequestID: "revert-finish-correction", Outcome: AttemptPassed,
		EvidenceRevision: runtimeTestHash('b'), Diagnosis: "reverted candidate claims a correction of the bytes that failed",
		HarnessDisposition: HarnessReused, CleanupEvidence: "correction cleanup completed",
		ProcessEvidence: "correction process scan completed", RemediatesEvidenceRevision: failedEvidence,
	})
	if err == nil || !strings.Contains(err.Error(), "failed-evidence remediation requires a changed correction candidate") {
		t.Fatalf("revert to the failed bytes = %v, want the changed-candidate refusal", err)
	}
	status, statusErr := store.Status()
	if statusErr != nil || status.Revision != active.Revision || status.ActiveAttempt == nil ||
		countRuntimeRecords(t, store.Dir) != before {
		t.Fatalf("revert refusal mutated runtime: status=%#v err=%v records=%d", status, statusErr, countRuntimeRecords(t, store.Dir))
	}
}

// The replay mirror decides the exact write-guard predicate, and every shape
// the write guard admits replays cleanly (write-to-replay closure). A
// two-baseline AND accepted unchanged-vs-failed records the write path refuses.
func TestRuntimeRemediationReplayDecidesWriteGuardPredicate(t *testing.T) {
	failedTree, beginTree, failedEvidence := "failedtree", "begintree", runtimeTestHash('a')
	cases := []struct {
		name, finishTree string
		reset            *RuntimeReset
		wantRefused      bool
	}{
		{"unchanged vs failed baseline refuses like the write guard", failedTree, nil, true},
		{"post-acquire change replays", "correctedtree", nil, false},
		{"pre-acquire change with finish equal to begin replays", beginTree, nil, false},
		{"evidence-only waiver shape replays unchanged bytes", failedTree, &RuntimeReset{Actor: "maintainer", Reason: "authorized evidence-only retry", PreviousObjectiveID: "objective", PreviousGeneration: 1, ResetCandidateTree: failedTree}, false},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			replay := &runtimeReplay{Status: RuntimeStatus{
				Objective: &RuntimeObjective{ID: "objective", Generation: 1, MaxAttempts: 3, MaxChangedLines: 10},
				Attempts: []RuntimeAttempt{
					{ObjectiveID: "objective", ObjectiveGeneration: 1, Outcome: AttemptFailed, EvidenceRevision: failedEvidence,
						FinishCandidateIdentity: runtimeTestHash('f'), FinishCandidateTree: failedTree},
					{Outcome: AttemptRunning},
				},
				ActiveAttempt: &RuntimeAttempt{Ordinal: 2, BeginCandidateIdentity: runtimeTestHash('0'), BeginCandidateTree: beginTree},
				LastReset:     testCase.reset,
			}}
			err := applyRuntimeFinishEvent(replay, &runtimeFinishEvent{
				Ordinal: 2, FinishCandidateIdentity: runtimeTestHash('1'), FinishCandidateTree: testCase.finishTree,
				Outcome: AttemptPassed, EvidenceRevision: runtimeTestHash('b'), RemediatesEvidenceRevision: failedEvidence,
			}, true)
			if (err != nil) != testCase.wantRefused {
				t.Fatalf("replay refusal = %v, want refused %v", err, testCase.wantRefused)
			}
		})
	}
}

// Failed attempts recorded before the finish candidate snapshot existed carry
// no baseline of their own, so the judgment falls back to the correction
// attempt's begin snapshot — the pre-#3073 comparison — instead of judging
// every candidate as changed.
func TestRuntimeRemediationCandidateBaselineFallsBackToBeginSnapshot(t *testing.T) {
	failedTree, failedIdentity := "failedtree", runtimeTestHash('f')
	beginTree, beginIdentity := "begintree", runtimeTestHash('0')
	active := RuntimeAttempt{BeginCandidateIdentity: beginIdentity, BeginCandidateTree: beginTree}
	snapshotFailed := RuntimeAttempt{FinishCandidateIdentity: failedIdentity, FinishCandidateTree: failedTree}
	legacyFailed := RuntimeAttempt{}
	cases := []struct {
		name           string
		failed         RuntimeAttempt
		identity, tree string
		wantUnchanged  bool
	}{
		{"matches the failed baseline", snapshotFailed, failedIdentity, failedTree, true},
		{"failed tree alone marks unchanged", snapshotFailed, runtimeTestHash('1'), failedTree, true},
		{"matches only the begin snapshot", snapshotFailed, beginIdentity, beginTree, false},
		{"differs from the failed baseline", snapshotFailed, runtimeTestHash('1'), "othertree", false},
		{"legacy failed record falls back to begin", legacyFailed, beginIdentity, beginTree, true},
		{"legacy fallback admits a changed candidate", legacyFailed, runtimeTestHash('1'), "othertree", false},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			got := runtimeRemediationCandidateUnchanged(testCase.failed, active, testCase.identity, testCase.tree)
			if got != testCase.wantUnchanged {
				t.Fatalf("runtimeRemediationCandidateUnchanged = %v, want %v", got, testCase.wantUnchanged)
			}
		})
	}
}
