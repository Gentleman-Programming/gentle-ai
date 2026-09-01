package sddstatus

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestRuntimeLedgerRepairsPublishedConsecutiveRescope(t *testing.T) {
	repo := initRuntimeLedgerRepo(t)
	store := mustRuntimeStore(t, repo, "repair-2839")
	started, err := store.Begin(context.Background(), BeginAttemptRequest{RequestID: "begin-a", WorkUnit: "objective-a", EvidenceGoal: "prove A", MaxAttempts: 2, MaxChangedLines: 20})
	if err != nil {
		t.Fatal(err)
	}
	failed, err := store.Finish(context.Background(), FinishAttemptRequest{ExpectedRevision: started.Revision, RequestID: "finish-a", Outcome: AttemptFailed, EvidenceRevision: runtimeTestHash('a'), Diagnosis: "failed without drift", HarnessDisposition: HarnessReused, CleanupEvidence: "clean", ProcessEvidence: "reproduced"})
	if err != nil {
		t.Fatal(err)
	}
	// The historical v2.4.0-rc.1 writer published successor B already
	// exhausted (max_attempts equal to the carried cumulative_attempts).
	// Today's writer refuses that shape before mutation (#2804), so the
	// historical chain is reproduced the way it actually exists in the
	// wild: as an already-published record.
	lastA := failed.Attempts[0]
	generationB := failed.ObjectiveGeneration + 1
	requestB := RescopeObjectiveRequest{ExpectedRevision: failed.Revision, RequestID: "rescope-a-b", WorkUnit: "objective-b", EvidenceGoal: "prove B", MaxAttempts: 1, MaxChangedLines: 10, Reason: "narrow A to B", Actor: "maintainer"}
	recordB := runtimeRecord{Schema: runtimeRecordSchema, Change: store.Change, PreviousRevision: failed.Revision, Operation: runtimeOperationRescope, RequestID: requestB.RequestID, RequestDigest: runtimeValueHash("gentle-ai.sdd-runtime-rescope-request/v1", requestB), Rescope: &runtimeRescopeEvent{PreviousObjectiveID: failed.Objective.ID, PreviousGeneration: failed.Objective.Generation, PreviousMaxAttempts: failed.Objective.MaxAttempts, PreviousMaxChangedLines: failed.Objective.MaxChangedLines, RescopeCandidateIdentity: lastA.FinishCandidateIdentity, RescopeCandidateTree: lastA.FinishCandidateTree, ObjectiveID: runtimeObjectiveID(store.Change, requestB.WorkUnit, requestB.EvidenceGoal, lastA.FinishCandidateIdentity, generationB), ObjectiveGeneration: generationB, WorkUnit: requestB.WorkUnit, EvidenceGoal: requestB.EvidenceGoal, MaxAttempts: requestB.MaxAttempts, MaxChangedLines: requestB.MaxChangedLines, Reason: requestB.Reason, Actor: requestB.Actor}}
	revisionB, payloadB, err := runtimeRecordRevision(recordB)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.publishRecord(revisionB, payloadB); err != nil {
		t.Fatal(err)
	}
	if err := store.publishHead(revisionB); err != nil {
		t.Fatal(err)
	}
	b, err := store.Status()
	if err != nil {
		t.Fatal(err)
	}
	last, generation := b.Attempts[0], b.ObjectiveGeneration+1
	request := RescopeObjectiveRequest{ExpectedRevision: b.Revision, RequestID: "rescope-b-c", WorkUnit: "objective-c", EvidenceGoal: "prove C", MaxAttempts: 1, MaxChangedLines: 5, Reason: "narrow B to C", Actor: "maintainer"}
	poison := runtimeRecord{Schema: runtimeRecordSchema, Change: store.Change, PreviousRevision: b.Revision, Operation: runtimeOperationRescope, RequestID: request.RequestID, RequestDigest: runtimeValueHash("gentle-ai.sdd-runtime-rescope-request/v1", request), Rescope: &runtimeRescopeEvent{PreviousObjectiveID: b.Objective.ID, PreviousGeneration: b.Objective.Generation, PreviousMaxAttempts: b.Objective.MaxAttempts, PreviousMaxChangedLines: b.Objective.MaxChangedLines, RescopeCandidateIdentity: b.Objective.InitialCandidateIdentity, RescopeCandidateTree: last.FinishCandidateTree, ObjectiveID: runtimeObjectiveID(store.Change, request.WorkUnit, request.EvidenceGoal, b.Objective.InitialCandidateIdentity, generation), ObjectiveGeneration: generation, WorkUnit: request.WorkUnit, EvidenceGoal: request.EvidenceGoal, MaxAttempts: request.MaxAttempts, MaxChangedLines: request.MaxChangedLines, Reason: request.Reason, Actor: request.Actor}}
	poisonRevision, payload, err := runtimeRecordRevision(poison)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.publishRecord(poisonRevision, payload); err != nil {
		t.Fatal(err)
	}
	if err := store.publishHead(poisonRevision); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(store.Dir, "records", poisonRevision[len("sha256:"):]+".json")
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	repaired, err := store.RepairConsecutiveRescope(context.Background(), RepairConsecutiveRescopeRequest{ExpectedRevision: poisonRevision, RequestID: "repair-poison", Reason: "repair released writer defect", Actor: "maintainer"})
	if err != nil {
		t.Fatal(err)
	}
	after, err := os.ReadFile(path)
	if err != nil || string(after) != string(before) {
		t.Fatalf("poisoned record changed: err=%v", err)
	}
	if repaired.Objective == nil || repaired.Objective.ID != b.Objective.ID || repaired.LastRepair == nil || repaired.LastRepair.ReplacedRevision != poisonRevision || repaired.LastReset != nil || !repaired.DecisionRequired || repaired.NextAction != RuntimeActionReset || repaired.CumulativeAttempts != 1 {
		t.Fatalf("repaired status = %#v", repaired)
	}
	reset, err := store.Reset(context.Background(), ResetObjectiveRequest{ExpectedRevision: repaired.Revision, RequestID: "reset-b", Reason: "reset repaired exhausted objective", Actor: "maintainer"})
	if err != nil || reset.LastReset == nil || reset.DecisionRequired || reset.NextAction != RuntimeActionBegin {
		t.Fatalf("reset repaired objective = %#v, err=%v", reset, err)
	}
	if _, err := store.Begin(context.Background(), BeginAttemptRequest{ExpectedRevision: reset.Revision, RequestID: "begin-b", WorkUnit: "objective-b", EvidenceGoal: "prove B", MaxAttempts: 1, MaxChangedLines: 10}); err != nil {
		t.Fatalf("begin after repair: %v", err)
	}
}
