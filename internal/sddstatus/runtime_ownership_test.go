package sddstatus

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"
)

func TestRuntimeOwnershipMaterializesLegacyCompatibilityFields(t *testing.T) {
	ctx := context.Background()
	repo := initRuntimeLedgerRepo(t)
	store := mustRuntimeStore(t, repo, "ownership-foundation")
	started, err := store.Begin(ctx, BeginAttemptRequest{
		RequestID: "ownership-begin", WorkUnit: "apply", EvidenceGoal: "prove ownership projection",
		MaxAttempts: 2, MaxChangedLines: 20,
	})
	if err != nil {
		t.Fatal(err)
	}
	replay, err := store.load()
	if err != nil {
		t.Fatal(err)
	}
	active := replay.Status.runtimeActiveAttempt()
	objective := replay.Status.runtimeObjective()
	if active == nil || objective == nil || replay.Status.ActiveAttempt == active || replay.Status.Objective == objective ||
		!reflect.DeepEqual(replay.Status.ActiveAttempt, active) || !reflect.DeepEqual(replay.Status.Objective, objective) {
		t.Fatalf("ownership did not materialize legacy fields: %#v", replay.Status)
	}
	if replay.Status.ownership.active[active.Ordinal] != objective.ID {
		t.Fatalf("active ordinal %d is not indexed by objective: %#v", active.Ordinal, replay.Status.ownership)
	}

	// The compatibility fields are projections, not a second authority. A stale
	// in-memory projection cannot make readiness admit a second attempt.
	replay.Status.ActiveAttempt = nil
	replay.Status.Objective = nil
	result, terminal := runtimeReadiness(runtimeReadinessInput{Status: replay.Status, AttemptTokens: replay.AttemptTokens})
	if !terminal || result.Reason != CompactBlockActiveAttempt || result.Token != started.Revision {
		t.Fatalf("readiness trusted compatibility fields over canonical ownership: %#v", result)
	}
	replay.Status.materializeRuntimeOwnership()
	if replay.Status.ActiveAttempt == active || replay.Status.Objective == objective ||
		!reflect.DeepEqual(replay.Status.ActiveAttempt, active) || !reflect.DeepEqual(replay.Status.Objective, objective) {
		t.Fatalf("materialized compatibility fields = %#v", replay.Status)
	}
}

func TestRuntimeOwnershipCompatibilityMutationCannotChangeCanonicalOwnership(t *testing.T) {
	status := RuntimeStatus{ownership: newRuntimeOwnership()}
	objective := &RuntimeObjective{ID: "objective", WorkUnit: "apply", ItemEditRoots: []string{"one", "two"}}
	status.setRuntimeObjective(objective)
	attempt := &RuntimeAttempt{
		Ordinal: 1, ObjectiveID: objective.ID, WorkUnit: "apply", ItemEditRoots: []string{"three", "four"},
		Handoff: &RuntimeHandoff{Ordinal: 1, SourceWorktree: "source", DestinationWorktree: "destination"},
	}
	status.setRuntimeActiveAttempt(attempt)
	wantObjective := *objective
	wantObjective.ItemEditRoots = append([]string(nil), objective.ItemEditRoots...)
	wantAttempt := *attempt
	wantAttempt.ItemEditRoots = append([]string(nil), attempt.ItemEditRoots...)
	wantHandoff := *attempt.Handoff
	wantAttempt.Handoff = &wantHandoff
	wantJSON, err := json.Marshal(RuntimeStatus{Objective: objective, ActiveAttempt: attempt})
	if err != nil {
		t.Fatal(err)
	}
	gotJSON, err := json.Marshal(status)
	if err != nil || string(gotJSON) != string(wantJSON) {
		t.Fatalf("compatibility JSON = %s, want %s, err=%v", gotJSON, wantJSON, err)
	}

	status.Objective.ItemEditRoots[0] = "mutated-root"
	status.Objective.ItemEditRoots = append(status.Objective.ItemEditRoots, "extra-root")
	status.ActiveAttempt.ItemEditRoots[0] = "mutated-attempt-root"
	status.ActiveAttempt.ItemEditRoots = append(status.ActiveAttempt.ItemEditRoots, "extra-attempt-root")
	status.ActiveAttempt.Handoff.SourceWorktree = "mutated-source"
	*status.Objective = RuntimeObjective{ID: "replaced-objective", ItemEditRoots: []string{"replacement"}}
	*status.ActiveAttempt = RuntimeAttempt{Ordinal: 2, ObjectiveID: "replaced-objective", ItemEditRoots: []string{"replacement"}, Handoff: &RuntimeHandoff{SourceWorktree: "replacement"}}

	if got := status.runtimeObjective(); got == status.Objective || !reflect.DeepEqual(got, &wantObjective) {
		t.Fatalf("canonical objective changed through compatibility projection: %#v", got)
	}
	if got := status.runtimeActiveAttempt(); got == status.ActiveAttempt || !reflect.DeepEqual(got, &wantAttempt) {
		t.Fatalf("canonical active attempt changed through compatibility projection: %#v", got)
	}
}

func TestRuntimeOwnershipRetainsSingleActiveAdmission(t *testing.T) {
	ctx := context.Background()
	repo := initRuntimeLedgerRepo(t)
	store := mustRuntimeStore(t, repo, "ownership-single-active")
	first, err := store.Acquire(ctx, CompactAcquireRequest{BeginAttemptRequest: BeginAttemptRequest{
		RequestID: "ownership-first", WorkUnit: "first", EvidenceGoal: "prove serialized admission",
		MaxAttempts: 2, MaxChangedLines: 20,
	}})
	if err != nil || first.State != CompactStateProceed {
		t.Fatalf("first acquire = %#v, %v", first, err)
	}
	second, err := store.Acquire(ctx, CompactAcquireRequest{BeginAttemptRequest: BeginAttemptRequest{
		RequestID: "ownership-second", WorkUnit: "second", EvidenceGoal: "must remain blocked",
		MaxAttempts: 2, MaxChangedLines: 20,
	}})
	if err != nil || second.State != CompactStateBlocked || second.Reason != CompactBlockActiveAttempt || second.Token != first.Token {
		t.Fatalf("second acquire enabled concurrency: %#v, %v", second, err)
	}
	status, err := store.Status()
	if err != nil || status.runtimeActiveAttempt() == nil || len(status.Attempts) != 1 || countRuntimeRecords(t, store.Dir) != 1 {
		t.Fatalf("serialized ownership = %#v, %v", status, err)
	}
}

func TestRuntimeLineageFailedEvidenceScopesAndDischargesObligations(t *testing.T) {
	status := RuntimeStatus{ownership: newRuntimeOwnership()}
	objectiveA := &RuntimeObjective{ID: "objective-a"}
	objectiveB := &RuntimeObjective{ID: "objective-b"}
	status.setRuntimeObjective(objectiveA)
	status.setRuntimeObjective(objectiveB)
	failed := RuntimeAttempt{Ordinal: 1, ObjectiveID: objectiveA.ID, Outcome: AttemptFailed, EvidenceRevision: runtimeTestHash('a')}
	status.Attempts = []RuntimeAttempt{failed}

	for _, tt := range []struct {
		name     string
		parent   string
		attempts []RuntimeAttempt
		want     string
	}{
		{name: "unrelated objective cannot inherit", want: ""},
		{name: "reset successor inherits", parent: objectiveA.ID, want: failed.EvidenceRevision},
		{name: "explicit correction discharges once", parent: objectiveA.ID, attempts: []RuntimeAttempt{{Ordinal: 2, ObjectiveID: objectiveB.ID, Outcome: AttemptPassed, RemediatesEvidenceRevision: failed.EvidenceRevision}}, want: ""},
		{name: "unknown parent fails closed", parent: "missing", want: ""},
	} {
		t.Run(tt.name, func(t *testing.T) {
			delete(status.ownership.parents, objectiveB.ID)
			status.ownership.roots[objectiveB.ID] = true
			if tt.parent != "" {
				status.ownership.parents[objectiveB.ID] = tt.parent
				delete(status.ownership.roots, objectiveB.ID)
			}
			status.Attempts = append([]RuntimeAttempt{failed}, tt.attempts...)
			got, ok := runtimeLineageFailedEvidence(status)
			if got != tt.want || ok != (tt.want != "") {
				t.Fatalf("lineage failed evidence = %q, %v; want %q", got, ok, tt.want)
			}
			if tt.parent == "" && unmanagedRemediationSettleable(status, failed.EvidenceRevision) {
				t.Fatal("unrelated objective accepted predecessor remediation evidence")
			}
		})
	}
	status.ownership.parents[objectiveB.ID] = objectiveA.ID
	delete(status.ownership.roots, objectiveB.ID)
	status.Attempts = []RuntimeAttempt{failed, {Ordinal: 2, ObjectiveID: objectiveB.ID, Outcome: AttemptPassed, RemediatesEvidenceRevision: failed.EvidenceRevision}}
	if evidence, ordinal, ok := runtimeLineageDischargedFailure(status, failed.EvidenceRevision); !ok || evidence != failed.EvidenceRevision || ordinal != 2 {
		t.Fatalf("discharged lineage failure = %q, %d, %v", evidence, ordinal, ok)
	}
}

func TestRuntimeLineageRejectsIncompleteAndCyclicOwnership(t *testing.T) {
	for _, mutate := range []func(*RuntimeStatus){
		func(status *RuntimeStatus) { delete(status.ownership.roots, "objective-b") },
		func(status *RuntimeStatus) {
			status.ownership.parents["objective-a"] = "objective-b"
			delete(status.ownership.roots, "objective-a")
			status.ownership.parents["objective-b"] = "objective-a"
			delete(status.ownership.roots, "objective-b")
		},
	} {
		status := RuntimeStatus{ownership: newRuntimeOwnership()}
		status.setRuntimeObjective(&RuntimeObjective{ID: "objective-a"})
		status.setRuntimeObjective(&RuntimeObjective{ID: "objective-b"})
		mutate(&status)
		if err := status.validateRuntimeLineage(); err == nil {
			t.Fatal("corrupt runtime lineage was accepted")
		}
		if result, terminal := runtimeReadiness(runtimeReadinessInput{Status: status}); !terminal || result.Reason != CompactBlockCorruptAuthority {
			t.Fatalf("corrupt lineage readiness = %#v, %v", result, terminal)
		}
	}
}

func TestRuntimeLineageDischargeAndCompletionIgnoreUnrelatedAttempts(t *testing.T) {
	status := RuntimeStatus{ownership: newRuntimeOwnership()}
	for _, id := range []string{"objective-a", "objective-b", "objective-c"} {
		status.setRuntimeObjective(&RuntimeObjective{ID: id})
	}
	status.setRuntimeObjectiveWithParent(&RuntimeObjective{ID: "objective-b"}, "objective-a")
	failed := RuntimeAttempt{Ordinal: 1, ObjectiveID: "objective-a", Outcome: AttemptFailed, EvidenceRevision: runtimeTestHash('a')}
	unrelated := RuntimeAttempt{Ordinal: 2, ObjectiveID: "objective-c", Outcome: AttemptPassed, RemediatesEvidenceRevision: failed.EvidenceRevision}
	status.Attempts = []RuntimeAttempt{failed, unrelated}
	if _, _, ok := runtimeLineageDischargedFailure(status, failed.EvidenceRevision); ok {
		t.Fatal("unrelated passed remediation discharged current-lineage failure")
	}
	if got, ok := runtimeLineageFailedEvidence(status); !ok || got != failed.EvidenceRevision {
		t.Fatalf("unrelated remediation hid current failure: %q, %v", got, ok)
	}

	correction := RuntimeAttempt{Ordinal: 3, ObjectiveID: "objective-b", Outcome: AttemptPassed,
		RemediatesEvidenceRevision: failed.EvidenceRevision, EvidenceRevision: runtimeTestHash('b'),
		BeginCandidateIdentity: "before", FinishCandidateIdentity: "after", BeginCandidateTree: "before", FinishCandidateTree: "after"}
	status.Attempts = append(status.Attempts, correction)
	status.EvidenceRevision = correction.EvidenceRevision
	if evidence, ordinal, ok := runtimeLineageDischargedFailure(status, failed.EvidenceRevision); !ok || evidence != failed.EvidenceRevision || ordinal != correction.Ordinal {
		t.Fatalf("lineage correction did not discharge exactly once: %q, %d, %v", evidence, ordinal, ok)
	}
	if !nativeRuntimeCompletedUnmanagedCorrection(&status) {
		t.Fatal("lineage correction with unrelated intervening attempt was not recognized")
	}
}
