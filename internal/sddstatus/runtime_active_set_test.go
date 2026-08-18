package sddstatus

import "testing"

func syntheticMultiActiveReplay() runtimeReplay {
	a := &RuntimeObjective{ID: "a", Generation: 1, WorkUnit: "a", MaxAttempts: 2, MaxChangedLines: 20}
	b := &RuntimeObjective{ID: "b", Generation: 2, WorkUnit: "b", MaxAttempts: 2, MaxChangedLines: 20}
	aAttempt := &RuntimeAttempt{Ordinal: 1, ObjectiveID: "a", ObjectiveGeneration: 1, WorkUnit: "a", Outcome: AttemptRunning, BeginCandidateIdentity: "a", BeginCandidateTree: "a"}
	bAttempt := &RuntimeAttempt{Ordinal: 2, ObjectiveID: "b", ObjectiveGeneration: 2, WorkUnit: "b", Outcome: AttemptRunning, BeginCandidateIdentity: "b", BeginCandidateTree: "b"}
	status := RuntimeStatus{Attempts: []RuntimeAttempt{*aAttempt, *bAttempt}, ownership: newRuntimeOwnership()}
	status.ownership.current = "b"
	status.ownership.objectives["a"] = &runtimeObjectiveOwnership{objective: a, active: aAttempt}
	status.ownership.objectives["b"] = &runtimeObjectiveOwnership{objective: b, active: bAttempt}
	status.ownership.active[1], status.ownership.active[2] = "a", "b"
	status.materializeRuntimeOwnership()
	return runtimeReplay{Status: status, Accounting: runtimeAccounting{buckets: map[runtimeObjectiveAccountingKey]runtimeObjectiveAccounting{{"a", 1}: {attempts: 1}, {"b", 2}: {attempts: 1}}, nextOrdinal: 3}}
}

func TestRuntimeActiveSetCompatibilityProjection(t *testing.T) {
	replay := syntheticMultiActiveReplay()
	if replay.Status.runtimeActiveAttempt() != nil || replay.Status.ActiveAttempt == nil || replay.Status.ActiveAttempt.Ordinal != 1 || replay.Status.Objective == nil || replay.Status.Objective.ID != "a" {
		t.Fatalf("active projection = %#v", replay.Status)
	}
	if err := replay.Accounting.materialize(&replay.Status); err != nil || replay.Status.CumulativeAttempts != 1 {
		t.Fatalf("accounting projection = %#v, %v", replay.Status, err)
	}
}

func TestRuntimeFinishEventTargetsExactActiveOwner(t *testing.T) {
	replay := syntheticMultiActiveReplay()
	event := &runtimeFinishEvent{Ordinal: 2, Outcome: AttemptPassed, FinishCandidateIdentity: "done", FinishCandidateTree: "done", EvidenceRevision: runtimeTestHash('b')}
	if err := applyRuntimeFinishEvent(&replay, event, false); err != nil {
		t.Fatal(err)
	}
	if replay.Status.runtimeActiveCount() != 1 || replay.Status.runtimeActiveAttemptForOrdinal(1) == nil || replay.Status.runtimeActiveAttemptForOrdinal(2) != nil || replay.Status.Attempts[0].Outcome != AttemptRunning || replay.Status.Attempts[1].Outcome != AttemptPassed || replay.Status.Complete || replay.Status.NextAction != RuntimeActionFinish {
		t.Fatalf("exact finish = %#v", replay.Status)
	}
	if got := replay.Accounting.buckets[runtimeObjectiveAccountingKey{"b", 2}]; got.lines != 0 || replay.Accounting.buckets[runtimeObjectiveAccountingKey{"a", 1}].lines != 0 {
		t.Fatalf("accounting = %#v", replay.Accounting)
	}
}

func TestRuntimeLifecycleRefusesSyntheticMultiActive(t *testing.T) {
	status := syntheticMultiActiveReplay().Status
	if status.runtimeActiveCount() != 2 || status.runtimeActiveAttempt() != nil {
		t.Fatalf("synthetic state = %#v", status)
	}
}
