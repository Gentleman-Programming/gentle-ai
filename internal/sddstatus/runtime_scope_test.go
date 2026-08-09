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
	idempotent, err := store.AdmitSliceProof(context.Background(), report, "slice", passed.Objective.ID)
	if err != nil || !idempotent.Valid {
		t.Fatalf("identical slice A proof retry = %#v err=%v", idempotent, err)
	}
	afterRetry, err := store.Status()
	if err != nil || afterRetry.Revision != reloaded.Revision || len(afterRetry.SliceProofs) != 1 {
		t.Fatalf("identical retry mutated proof state = %#v err=%v", afterRetry, err)
	}
	for _, tt := range []struct {
		name      string
		report    string
		sliceID   string
		wantErr   string
		wantValid bool
	}{
		{"missing scope metadata", testVerifyEnvelope("pass", 0, 0, "1/1", "1/1", 0, 0), passed.Objective.ID, "", false},
		{"partial scope metadata", strings.TrimSuffix(report, "slice_id: "+passed.Objective.ID+"\n```") + "```", passed.Objective.ID, "", false},
		{"mismatched scope metadata", strings.Replace(report, passed.Objective.ID, runtimeTestHash('b'), 1), passed.Objective.ID, "", false},
		{"mismatched requested slice", report, runtimeTestHash('b'), "completed scoped runtime objective", false},
		{"conflicting proof", strings.Replace(report, "go test ./internal/example", "go test ./internal/other", 1), passed.Objective.ID, "conflicts", false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			admission, err := store.AdmitSliceProof(context.Background(), tt.report, "slice", tt.sliceID)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("admit slice proof error = %v", err)
				}
			} else if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("admit slice proof error = %v, want %q", err, tt.wantErr)
			}
			if admission.Valid != tt.wantValid {
				t.Fatalf("admission = %#v, want valid=%v", admission, tt.wantValid)
			}
			status, err := store.Status()
			if err != nil || status.Revision != reloaded.Revision || len(status.SliceProofs) != 1 {
				t.Fatalf("refused proof changed accepted state = %#v err=%v", status, err)
			}
		})
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
	report := strings.TrimSuffix(testVerifyEnvelope("pass_with_warnings", 0, 0, "0/0", "0/0", 0, 0), "```") + "scope: slice\nslice_id: " + passed.Objective.ID + "\n```"
	if admission, err := store.AdmitSliceProof(context.Background(), report, "slice", passed.Objective.ID); err != nil || !admission.Valid {
		t.Fatalf("foundational proof = %#v err=%v", admission, err)
	}
}

func TestRuntimeSliceProofRejectsValidFailReportWithoutMutation(t *testing.T) {
	repo := initRuntimeLedgerRepo(t)
	store, err := OpenRuntimeStore(context.Background(), repo, "slice-proof-fail")
	if err != nil {
		t.Fatal(err)
	}
	started, err := store.Begin(context.Background(), BeginAttemptRequest{
		RequestID: "slice-proof-fail-begin", WorkUnit: "slice-fail", EvidenceGoal: "prove failing slice report", MaxAttempts: 1, MaxChangedLines: 20,
		Scope: &RuntimeScope{Tasks: []string{"1.1"}, Requirements: []string{"REQ-A"}, Scenarios: []string{"scenario-a"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	appendRuntimeLedgerFile(t, repo, "slice-proof-fail\n")
	completed, err := store.Finish(context.Background(), FinishAttemptRequest{
		ExpectedRevision: started.Revision, RequestID: "slice-proof-fail-finish", Outcome: AttemptPassed, EvidenceRevision: runtimeTestHash('a'),
		Diagnosis: "slice completed", HarnessDisposition: HarnessReused, CleanupEvidence: "cleanup complete", ProcessEvidence: "processes stopped",
	})
	if err != nil {
		t.Fatal(err)
	}
	report := strings.TrimSuffix(testVerifyEnvelope("fail", 0, 0, "1/1", "1/1", 1, 0), "```") + "scope: slice\nslice_id: " + completed.Objective.ID + "\n```"
	if general := ValidateVerifyReportAdmission(report, SpecCounts{}, *completed.Objective); !general.Valid || general.Verdict != "fail" {
		t.Fatalf("valid fail report = %#v", general)
	}
	admission, err := store.AdmitSliceProof(context.Background(), report, "slice", completed.Objective.ID)
	if err != nil || admission.Valid || admission.Reason != "slice proof requires a complete passing report" {
		t.Fatalf("failing slice proof = %#v err=%v", admission, err)
	}
	status, err := store.Status()
	if err != nil || status.Revision != completed.Revision || len(status.SliceProofs) != 0 {
		t.Fatalf("failing slice proof mutated state = %#v err=%v", status, err)
	}
	if _, err := store.Begin(context.Background(), BeginAttemptRequest{
		ExpectedRevision: status.Revision, RequestID: "slice-proof-fail-successor", WorkUnit: "slice-successor", EvidenceGoal: "prove successor", MaxAttempts: 1, MaxChangedLines: 20,
		Scope: &RuntimeScope{Tasks: []string{"1.2"}, Requirements: []string{"REQ-B"}, Scenarios: []string{"scenario-b"}},
	}); !errors.Is(err, ErrRuntimeObjectiveDone) {
		t.Fatalf("advance after failing slice proof = %v, want ErrRuntimeObjectiveDone", err)
	}
}
