package sddstatus

import (
	"context"
	"strings"
	"testing"
)

func TestRuntimeLedgerRejectsMalformedPersistedInterruptedEvidence(t *testing.T) {
	store := mustRuntimeStore(t, initRuntimeLedgerRepo(t), "malformed-interrupted")
	started, err := store.Begin(context.Background(), BeginAttemptRequest{
		RequestID: "malformed-begin", WorkUnit: "runtime evidence", EvidenceGoal: "replay safely",
		MaxAttempts: 2, MaxChangedLines: 20,
	})
	if err != nil {
		t.Fatal(err)
	}
	settled, err := store.Finish(context.Background(), FinishAttemptRequest{
		ExpectedRevision: started.Revision, RequestID: "malformed-finish", Outcome: AttemptInterrupted,
		Diagnosis: "the interrupted attempt was recorded", HarnessDisposition: HarnessInvalidated,
		CleanupEvidence: "cleanup completed", ProcessEvidence: "no surviving processes",
	})
	if err != nil {
		t.Fatal(err)
	}

	record, err := store.loadRecord(settled.Revision)
	if err != nil {
		t.Fatal(err)
	}
	record.Finish.EvidenceRevision = "sha256:" + strings.Repeat("G", 64)
	record.RequestDigest = runtimeValueHash("gentle-ai.sdd-runtime-finish-request/v1", FinishAttemptRequest{
		ExpectedRevision: record.PreviousRevision, RequestID: record.RequestID, Outcome: record.Finish.Outcome,
		EvidenceRevision: record.Finish.EvidenceRevision, Diagnosis: record.Finish.Diagnosis,
		HarnessDisposition: record.Finish.HarnessDisposition, CleanupEvidence: record.Finish.CleanupEvidence,
		ProcessEvidence: record.Finish.ProcessEvidence,
	})
	revision, payload, err := runtimeRecordRevision(record)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.publishRecord(revision, payload); err != nil {
		t.Fatal(err)
	}
	if err := store.publishHead(revision); err != nil {
		t.Fatal(err)
	}

	if _, err := store.Status(); !isRuntimeRecordRejection(err, "invalid_finish_event") {
		t.Fatalf("malformed persisted interrupted evidence status error = %v, want fail-closed replay refusal", err)
	}
}
