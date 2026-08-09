package sddstatus

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestRuntimeSliceProofPersistsBeforeAdvancingDistinctWorkUnit(t *testing.T) {
	repo := initRuntimeLedgerRepo(t)
	store, err := OpenRuntimeStore(context.Background(), repo, "slice-proof")
	if err != nil {
		t.Fatal(err)
	}
	scopeA := &RuntimeScope{Tasks: []string{"1.1"}, Requirements: []string{"REQ-A"}, Scenarios: []string{"scenario-a"}}
	started, err := store.Begin(context.Background(), BeginAttemptRequest{
		RequestID: "slice-a-begin", WorkUnit: "slice-a", EvidenceGoal: "prove slice A", MaxAttempts: 2, MaxChangedLines: 20, Scope: scopeA,
	})
	if err != nil {
		t.Fatal(err)
	}
	appendRuntimeLedgerFile(t, repo, "slice-a\n")
	passed, err := store.Finish(context.Background(), FinishAttemptRequest{
		ExpectedRevision: started.Revision, RequestID: "slice-a-finish", Outcome: AttemptPassed, EvidenceRevision: runtimeTestHash('a'),
		Diagnosis: "slice A passed", HarnessDisposition: HarnessReused, CleanupEvidence: "cleanup complete", ProcessEvidence: "processes stopped",
	})
	if err != nil {
		t.Fatal(err)
	}
	scopeB := &RuntimeScope{Tasks: []string{"1.2"}, Requirements: []string{"REQ-B"}, Scenarios: []string{"scenario-b"}}
	advance := BeginAttemptRequest{ExpectedRevision: passed.Revision, RequestID: "slice-b-begin", WorkUnit: "slice-b", EvidenceGoal: "prove slice B", MaxAttempts: 2, MaxChangedLines: 20, Scope: scopeB}
	if _, err := store.Begin(context.Background(), advance); !errors.Is(err, ErrRuntimeObjectiveDone) {
		t.Fatalf("advance before slice proof = %v, want ErrRuntimeObjectiveDone", err)
	}
	report := strings.TrimSuffix(testVerifyEnvelope("pass", 0, 0, "1/1", "1/1", 0, 0), "```") + "scope: slice\nslice_id: " + passed.Objective.ID + "\n```"
	admission, err := store.AdmitSliceProof(context.Background(), report, "slice", passed.Objective.ID)
	if err != nil || !admission.Valid {
		t.Fatalf("slice A proof = %#v err=%v", admission, err)
	}
	reloaded, err := store.Status()
	if err != nil || len(reloaded.SliceProofs) != 1 || reloaded.SliceProofs[0].ObjectiveID != passed.Objective.ID {
		t.Fatalf("reloaded proof = %#v err=%v", reloaded.SliceProofs, err)
	}
	overlap := advance
	overlap.RequestID = "slice-overlap"
	overlap.ExpectedRevision = reloaded.Revision
	overlap.Scope = &RuntimeScope{Tasks: []string{"1.2"}, Requirements: []string{"REQ-A"}, Scenarios: []string{"scenario-b"}}
	if _, err := store.Begin(context.Background(), overlap); err == nil || !strings.Contains(err.Error(), "overlaps") {
		t.Fatalf("overlapping successor = %v, want overlap refusal", err)
	}
	advance.ExpectedRevision = reloaded.Revision
	advanced, err := store.Begin(context.Background(), advance)
	if err != nil || advanced.Objective == nil || advanced.Objective.WorkUnit != "slice-b" || advanced.Complete {
		t.Fatalf("advance after slice proof = %#v err=%v", advanced, err)
	}
	if global := parseVerifyResult(testVerifyEnvelope("pass", 0, 0, "1/2", "1/2", 0, 0), SpecCounts{Requirements: 2, Scenarios: 2}); global.Passing {
		t.Fatal("slice proof made whole-change verification pass")
	}
}

func TestRuntimeScopeFailsClosedForOverlapAndInvalidFoundation(t *testing.T) {
	for _, tt := range []struct {
		name  string
		scope *RuntimeScope
		want  string
	}{
		{"taskless foundation", &RuntimeScope{}, "at least one assigned task"},
		{"duplicate assignment", &RuntimeScope{Tasks: []string{"1.1", "1.1"}}, "duplicate"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			repo := initRuntimeLedgerRepo(t)
			store, err := OpenRuntimeStore(context.Background(), repo, "scope-invalid")
			if err != nil {
				t.Fatal(err)
			}
			_, err = store.Begin(context.Background(), BeginAttemptRequest{RequestID: "scope-invalid", WorkUnit: "foundation", EvidenceGoal: "prove foundation", Scope: tt.scope})
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("begin error = %v, want %q", err, tt.want)
			}
		})
	}
}

func TestRuntimeTaskOnlyFoundationSliceAdmitsZeroTotals(t *testing.T) {
	repo := initRuntimeLedgerRepo(t)
	store, err := OpenRuntimeStore(context.Background(), repo, "foundation-slice")
	if err != nil {
		t.Fatal(err)
	}
	started, err := store.Begin(context.Background(), BeginAttemptRequest{
		RequestID: "foundation-begin", WorkUnit: "foundation", EvidenceGoal: "prove runtime foundation", Scope: &RuntimeScope{Tasks: []string{"1.0"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	appendRuntimeLedgerFile(t, repo, "foundation\n")
	passed, err := store.Finish(context.Background(), FinishAttemptRequest{
		ExpectedRevision: started.Revision, RequestID: "foundation-finish", Outcome: AttemptPassed, EvidenceRevision: runtimeTestHash('a'),
		Diagnosis: "foundation passed", HarnessDisposition: HarnessReused, CleanupEvidence: "cleanup complete", ProcessEvidence: "processes stopped",
	})
	if err != nil {
		t.Fatal(err)
	}
	report := strings.TrimSuffix(testVerifyEnvelope("pass", 0, 0, "0/0", "0/0", 0, 0), "```") + "scope: slice\nslice_id: " + passed.Objective.ID + "\n```"
	if admission, err := store.AdmitSliceProof(context.Background(), report, "slice", passed.Objective.ID); err != nil || !admission.Valid {
		t.Fatalf("foundational proof = %#v err=%v", admission, err)
	}
}
