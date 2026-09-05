package sddstatus

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestRuntimeReviewDisabledRemediationConsumesTheOnlyRemainingAttempt(t *testing.T) {
	repo := initRuntimeLedgerRepo(t)
	store := mustRuntimeStore(t, repo, "review-disabled-remediation")
	store.ReviewDisabled = true
	first, err := store.Begin(context.Background(), BeginAttemptRequest{
		ExpectedRevision: "", RequestID: "review-disabled-begin-verification", WorkUnit: "verify",
		EvidenceGoal: "independent verification", MaxAttempts: 2, MaxChangedLines: 20,
	})
	if err != nil {
		t.Fatal(err)
	}
	failedEvidence := runtimeTestHash('a')
	failed, err := store.Finish(context.Background(), FinishAttemptRequest{
		ExpectedRevision: first.Revision, RequestID: "review-disabled-finish-verification", Outcome: AttemptFailed,
		EvidenceRevision: failedEvidence, Diagnosis: "independent verification found a correctable defect",
		HarnessDisposition: HarnessReused, CleanupEvidence: "verification cleanup completed",
		ProcessEvidence: "verification process scan completed",
	})
	if err != nil {
		t.Fatal(err)
	}
	active, err := store.Begin(context.Background(), BeginAttemptRequest{
		ExpectedRevision: failed.Revision, RequestID: "review-disabled-begin-correction", WorkUnit: "verify",
		EvidenceGoal: "independent verification", MaxAttempts: 2, MaxChangedLines: 20,
	})
	if err != nil {
		t.Fatal(err)
	}
	request := FinishAttemptRequest{
		ExpectedRevision: active.Revision, RequestID: "review-disabled-finish-correction", Outcome: AttemptPassed,
		EvidenceRevision: runtimeTestHash('b'), Diagnosis: "bounded correction passed focused checks",
		HarnessDisposition: HarnessReused, CleanupEvidence: "correction cleanup completed",
		ProcessEvidence: "correction process scan completed", RemediatesEvidenceRevision: failedEvidence,
	}
	before := countRuntimeRecords(t, store.Dir)
	if _, err := store.Finish(context.Background(), request); err == nil {
		t.Fatal("unchanged candidate satisfied review-disabled remediation")
	}
	if status, err := store.Status(); err != nil || status.Revision != active.Revision || status.ActiveAttempt == nil || countRuntimeRecords(t, store.Dir) != before {
		t.Fatalf("unchanged remediation refusal mutated runtime: status=%#v err=%v records=%d", status, err, countRuntimeRecords(t, store.Dir))
	}
	appendRuntimeLedgerFile(t, repo, "bounded review-disabled correction\n")
	missingEvidence := request
	missingEvidence.RequestID = "review-disabled-finish-missing-evidence"
	missingEvidence.RemediatesEvidenceRevision = ""
	if _, err := store.Finish(context.Background(), missingEvidence); err == nil {
		t.Fatal("generic passing finish satisfied disabled remediation")
	}
	wrongEvidence := request
	wrongEvidence.RequestID = "review-disabled-finish-wrong-evidence"
	wrongEvidence.RemediatesEvidenceRevision = runtimeTestHash('c')
	if _, err := store.Finish(context.Background(), wrongEvidence); err == nil {
		t.Fatal("wrong failed evidence satisfied review-disabled remediation")
	}
	completed, err := store.Finish(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if !completed.Complete || completed.ActiveAttempt != nil || len(completed.Attempts) != 2 {
		t.Fatalf("review-disabled remediation completion = %#v", completed)
	}
	last := completed.Attempts[len(completed.Attempts)-1]
	if last.RemediatesEvidenceRevision != failedEvidence || last.FinishCandidateIdentity == last.BeginCandidateIdentity || last.FinishCandidateTree == last.BeginCandidateTree {
		t.Fatalf("review-disabled remediation did not link failed evidence to a changed candidate: %#v", last)
	}
	if replay, err := store.Finish(context.Background(), request); err != nil || replay.Revision != completed.Revision {
		t.Fatalf("exact remediation replay = %#v err=%v", replay, err)
	}
	result, err := store.Acquire(context.Background(), CompactAcquireRequest{BeginAttemptRequest: BeginAttemptRequest{
		RequestID: "review-disabled-replay-correction", WorkUnit: "verify", EvidenceGoal: "independent verification", MaxAttempts: 2, MaxChangedLines: 20,
	}})
	if err != nil || result.State != CompactStateComplete {
		t.Fatalf("subsequent correction = %#v err=%v", result, err)
	}
}

func TestRuntimeReviewDisabledEngramRemediationNeedsNoOpenSpecRoot(t *testing.T) {
	repo := initRuntimeLedgerRepo(t)
	if _, err := os.Stat(filepath.Join(repo, "openspec")); !os.IsNotExist(err) {
		t.Fatalf("pure Engram fixture unexpectedly has an OpenSpec root: %v", err)
	}
	store := mustRuntimeStore(t, repo, "engram-remediation")
	store.ReviewDisabled = true
	first, err := store.Begin(context.Background(), BeginAttemptRequest{
		ExpectedRevision: "", RequestID: "engram-begin-failed-verification", WorkUnit: "verify",
		EvidenceGoal: "independent verification", MaxAttempts: 2, MaxChangedLines: 20,
	})
	if err != nil {
		t.Fatal(err)
	}
	failedEvidence := runtimeTestHash('d')
	failed, err := store.Finish(context.Background(), FinishAttemptRequest{
		ExpectedRevision: first.Revision, RequestID: "engram-finish-failed-verification", Outcome: AttemptFailed,
		EvidenceRevision: failedEvidence, Diagnosis: "independent verification found a correctable defect",
		HarnessDisposition: HarnessReused, CleanupEvidence: "verification cleanup completed",
		ProcessEvidence: "verification process scan completed",
	})
	if err != nil {
		t.Fatal(err)
	}
	active, err := store.Begin(context.Background(), BeginAttemptRequest{
		ExpectedRevision: failed.Revision, RequestID: "engram-begin-correction", WorkUnit: "verify",
		EvidenceGoal: "independent verification", MaxAttempts: 2, MaxChangedLines: 20,
	})
	if err != nil {
		t.Fatal(err)
	}
	appendRuntimeLedgerFile(t, repo, "bounded Engram correction\n")
	request := FinishAttemptRequest{
		ExpectedRevision: active.Revision, RequestID: "engram-finish-correction", Outcome: AttemptPassed,
		EvidenceRevision: runtimeTestHash('e'), Diagnosis: "corrected candidate passed focused checks",
		HarnessDisposition: HarnessReused, CleanupEvidence: "correction cleanup completed",
		ProcessEvidence: "correction process scan completed", RemediatesEvidenceRevision: failedEvidence,
	}
	completed, err := store.Finish(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if !completed.Complete || completed.ActiveAttempt != nil || len(completed.Attempts) != 2 {
		t.Fatalf("pure Engram remediation completion = %#v", completed)
	}
	last := completed.Attempts[len(completed.Attempts)-1]
	if last.RemediatesEvidenceRevision != failedEvidence || last.FinishCandidateTree == completed.Attempts[0].FinishCandidateTree {
		t.Fatalf("pure Engram remediation did not bind failed evidence to changed candidate: %#v", last)
	}
	if _, err := os.Stat(filepath.Join(repo, "openspec")); !os.IsNotExist(err) {
		t.Fatalf("runtime remediation created or required an OpenSpec root: %v", err)
	}
	replayed, err := store.Finish(context.Background(), request)
	if err != nil || replayed.Revision != completed.Revision {
		t.Fatalf("exact pure Engram remediation replay = %#v err=%v", replayed, err)
	}
	reopened := mustRuntimeStore(t, repo, "engram-remediation")
	status, err := reopened.Status()
	if err != nil || status.Revision != completed.Revision || !status.Complete {
		t.Fatalf("pure Engram remediation chain replay = %#v err=%v", status, err)
	}
}
