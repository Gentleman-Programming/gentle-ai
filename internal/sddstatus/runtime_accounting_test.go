package sddstatus

import "testing"

func TestRuntimeAccountingTransitions(t *testing.T) {
	tests := []struct {
		name  string
		apply func(*runtimeAccounting)
		want  runtimeAccounting
	}{
		{"begin charges current and lifetime", func(a *runtimeAccounting) { a.nextOrdinal = 1; a.begin() }, runtimeAccounting{attempts: 1, lifetimeAttempts: 1, nextOrdinal: 2}},
		{"finish charges lines once", func(a *runtimeAccounting) { a.finish(7) }, runtimeAccounting{lines: 7, lifetimeLines: 7}},
		{"reset keeps lifetime and ordinal", func(a *runtimeAccounting) {
			a.attempts, a.lines, a.lifetimeAttempts, a.lifetimeLines, a.nextOrdinal = 2, 7, 4, 9, 5
			a.reset()
		}, runtimeAccounting{lifetimeAttempts: 4, lifetimeLines: 9, nextOrdinal: 5}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got runtimeAccounting
			tt.apply(&got)
			if got != tt.want {
				t.Fatalf("accounting = %#v, want %#v", got, tt.want)
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
	accounting := runtimeAccounting{attempts: 2, lines: 7, lifetimeAttempts: 4, lifetimeLines: 9, nextOrdinal: 5}
	var status RuntimeStatus
	accounting.materialize(&status)
	if status.CumulativeAttempts != 2 || status.CumulativeChangedLines != 7 || status.LifetimeAttempts != 4 || status.LifetimeChangedLines != 9 || status.NextOrdinal != 5 {
		t.Fatalf("legacy accounting projection = %#v", status)
	}
}
