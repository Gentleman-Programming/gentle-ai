package sddstatus

import "testing"

func TestRuntimeAccountingTransitions(t *testing.T) {
	tests := []struct {
		name  string
		apply func(*runtimeAccounting, *RuntimeObjective, *RuntimeObjective) error
		want  runtimeObjectiveAccounting
	}{
		{"begin and finish charge the current key", func(a *runtimeAccounting, current, _ *RuntimeObjective) error {
			if err := a.fresh(current); err != nil {
				return err
			}
			if err := a.begin(current); err != nil {
				return err
			}
			return a.finish(current, 7)
		}, runtimeObjectiveAccounting{attempts: 1, lines: 7}},
		{"generation isolates a reused objective ID", func(a *runtimeAccounting, current, other *RuntimeObjective) error {
			if err := a.fresh(current); err != nil {
				return err
			}
			if err := a.begin(current); err != nil {
				return err
			}
			return a.fresh(other)
		}, runtimeObjectiveAccounting{}},
		{"rescope carries while fresh successors do not", func(a *runtimeAccounting, current, other *RuntimeObjective) error {
			if err := a.fresh(current); err != nil {
				return err
			}
			if err := a.begin(current); err != nil {
				return err
			}
			if err := a.finish(current, 7); err != nil {
				return err
			}
			return a.carry(other, current)
		}, runtimeObjectiveAccounting{attempts: 1, lines: 7}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			current := &RuntimeObjective{ID: "objective", Generation: 1}
			other := &RuntimeObjective{ID: "objective", Generation: 2}
			accounting := runtimeAccounting{nextOrdinal: 1}
			if err := tt.apply(&accounting, current, other); err != nil {
				t.Fatal(err)
			}
			got, err := accounting.current(other)
			if tt.name == "begin and finish charge the current key" {
				got, err = accounting.current(current)
			}
			if err != nil || got != tt.want {
				t.Fatalf("accounting = %#v, %v; want %#v", got, err, tt.want)
			}
		})
	}
}

func TestRuntimeReplayProjectsNextOrdinalAfterFirstBegin(t *testing.T) {
	repo := initRuntimeLedgerRepo(t)
	store := mustRuntimeStore(t, repo, "accounting-first-begin")
	status, err := store.Begin(t.Context(), BeginAttemptRequest{RequestID: "accounting-first", WorkUnit: "apply", EvidenceGoal: "prove ordinal projection", MaxAttempts: 1, MaxChangedLines: 20})
	if err != nil || status.NextOrdinal != 2 {
		t.Fatalf("first begin status = %#v, %v", status, err)
	}
}

func TestRuntimeAccountingMaterializesLegacyFields(t *testing.T) {
	objective := &RuntimeObjective{ID: "objective", Generation: 1}
	accounting := runtimeAccounting{buckets: map[runtimeObjectiveAccountingKey]runtimeObjectiveAccounting{{"objective", 1}: {attempts: 2, lines: 7}}, lifetimeAttempts: 4, lifetimeLines: 9, nextOrdinal: 5}
	status := RuntimeStatus{Objective: objective}
	if err := accounting.materialize(&status); err != nil {
		t.Fatal(err)
	}
	if status.CumulativeAttempts != 2 || status.CumulativeChangedLines != 7 || status.LifetimeAttempts != 4 || status.LifetimeChangedLines != 9 || status.NextOrdinal != 5 {
		t.Fatalf("legacy accounting projection = %#v", status)
	}
	status.CumulativeAttempts, status.CumulativeChangedLines = 99, 99
	if err := accounting.materialize(&status); err != nil || status.CumulativeAttempts != 2 || status.CumulativeChangedLines != 7 {
		t.Fatalf("compatibility projection mutated accounting: %#v, %v", status, err)
	}
}

func TestRuntimeAccountingFailsClosedForMissingKeyAndSecondActiveBegin(t *testing.T) {
	objective := &RuntimeObjective{ID: "objective", Generation: 1}
	if err := (&runtimeAccounting{}).finish(objective, 1); err == nil {
		t.Fatal("finish accepted a missing accounting key")
	}
	replay := runtimeReplay{Status: RuntimeStatus{ownership: newRuntimeOwnership()}}
	replay.Status.setRuntimeObjective(objective)
	active := RuntimeAttempt{Ordinal: 1, ObjectiveID: objective.ID, ObjectiveGeneration: objective.Generation, Outcome: AttemptRunning}
	replay.Status.Attempts = []RuntimeAttempt{active}
	replay.Status.setRuntimeActiveAttempt(&active)
	if err := applyRuntimeBeginEvent(&replay, "crafted", runtimeRecord{Begin: &runtimeBeginEvent{}}); err == nil {
		t.Fatal("replay accepted a crafted second active begin")
	}
}
