package sddstatus

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
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

func TestRuntimeReplayRejectsForgedFailSliceProof(t *testing.T) {
	repo := initRuntimeLedgerRepo(t)
	store, err := OpenRuntimeStore(context.Background(), repo, "slice-proof-forged-fail")
	if err != nil {
		t.Fatal(err)
	}
	started, err := store.Begin(context.Background(), BeginAttemptRequest{
		RequestID: "slice-proof-forged-fail-begin", WorkUnit: "slice-fail", EvidenceGoal: "prove forged failure rejection", MaxAttempts: 1, MaxChangedLines: 20,
		Scope: &RuntimeScope{Tasks: []string{"1.1"}, Requirements: []string{"REQ-A"}, Scenarios: []string{"scenario-a"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	appendRuntimeLedgerFile(t, repo, "slice-proof-forged-fail\n")
	completed, err := store.Finish(context.Background(), FinishAttemptRequest{
		ExpectedRevision: started.Revision, RequestID: "slice-proof-forged-fail-finish", Outcome: AttemptPassed, EvidenceRevision: runtimeTestHash('a'),
		Diagnosis: "slice completed", HarnessDisposition: HarnessReused, CleanupEvidence: "cleanup complete", ProcessEvidence: "processes stopped",
	})
	if err != nil {
		t.Fatal(err)
	}
	report := strings.TrimSuffix(testVerifyEnvelope("fail", 0, 0, "1/1", "1/1", 1, 0), "```") + "scope: slice\nslice_id: " + completed.Objective.ID + "\n```"
	event := &runtimeSliceProofEvent{ObjectiveID: completed.Objective.ID, SliceID: completed.Objective.ID, EvidenceRevision: completed.EvidenceRevision, Report: report}
	requestDigest := runtimeValueHash("gentle-ai.sdd-runtime-slice-proof-request/v1", struct{ Scope, SliceID, Report string }{"slice", event.SliceID, event.Report})
	record := runtimeRecord{
		Schema: runtimeRecordSchema, Change: store.Change, PreviousRevision: completed.Revision, Operation: runtimeOperationSliceProof,
		RequestID: "slice-proof-" + strings.TrimPrefix(requestDigest, "sha256:")[:32], RequestDigest: requestDigest, SliceProof: event,
	}
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
	if _, err := store.Status(); err == nil || !strings.Contains(err.Error(), "invalid SDD runtime slice proof report") {
		t.Fatalf("forged fail slice proof replay error = %v", err)
	}
}

func TestRuntimeSliceProofConcurrentIdenticalAdmissionIsIdempotent(t *testing.T) {
	repo := initRuntimeLedgerRepo(t)
	store, err := OpenRuntimeStore(context.Background(), repo, "slice-proof-concurrent")
	if err != nil {
		t.Fatal(err)
	}
	scope := &RuntimeScope{Tasks: []string{"1.1"}, Requirements: []string{"REQ-A"}, Scenarios: []string{"scenario-a"}}
	started, err := store.Begin(context.Background(), BeginAttemptRequest{
		RequestID: "slice-proof-concurrent-begin", WorkUnit: "slice-a", EvidenceGoal: "prove concurrent slice admission", MaxAttempts: 1, MaxChangedLines: 20, Scope: scope,
	})
	if err != nil {
		t.Fatal(err)
	}
	appendRuntimeLedgerFile(t, repo, "slice-proof-concurrent\n")
	completed, err := store.Finish(context.Background(), FinishAttemptRequest{
		ExpectedRevision: started.Revision, RequestID: "slice-proof-concurrent-finish", Outcome: AttemptPassed, EvidenceRevision: runtimeTestHash('a'),
		Diagnosis: "slice completed", HarnessDisposition: HarnessReused, CleanupEvidence: "cleanup complete", ProcessEvidence: "processes stopped",
	})
	if err != nil {
		t.Fatal(err)
	}
	report := strings.TrimSuffix(testVerifyEnvelope("pass", 0, 0, "1/1", "1/1", 0, 0), "```") + "scope: slice\nslice_id: " + completed.Objective.ID + "\n```"
	requestDigest := runtimeValueHash("gentle-ai.sdd-runtime-slice-proof-request/v1", struct{ Scope, SliceID, Report string }{
		Scope: "slice", SliceID: completed.Objective.ID, Report: report,
	})
	requestID := "slice-proof-" + strings.TrimPrefix(requestDigest, "sha256:")[:32]

	// Inject the other writer after AdmitSliceProof's precheck but before its
	// locked mutation, proving the exact immutable request receipt converges
	// without publishing a second proof.
	originalAcquire := runtimeAcquireAuthorityFileLock
	injected := false
	var injectedErr error
	runtimeAcquireAuthorityFileLock = func(path string) (*reviewtransaction.AuthorityFileLock, error) {
		if !injected {
			injected = true
			runtimeAcquireAuthorityFileLock = originalAcquire
			_, injectedErr = store.mutate(context.Background(), completed.Revision, requestID, requestDigest, func(runtimeReplay) (runtimeRecord, error) {
				return runtimeRecord{Operation: runtimeOperationSliceProof, SliceProof: &runtimeSliceProofEvent{
					ObjectiveID: completed.Objective.ID, SliceID: completed.Objective.ID, EvidenceRevision: completed.EvidenceRevision, Report: report,
				}}, nil
			})
			if injectedErr != nil {
				return nil, injectedErr
			}
		}
		return originalAcquire(path)
	}
	t.Cleanup(func() { runtimeAcquireAuthorityFileLock = originalAcquire })

	admission, err := store.AdmitSliceProof(context.Background(), report, "slice", completed.Objective.ID)
	if injectedErr != nil || err != nil || !admission.Valid {
		t.Fatalf("concurrent identical proof = %#v err=%v injected=%v", admission, err, injectedErr)
	}
	status, err := store.Status()
	if err != nil || len(status.SliceProofs) != 1 || countRuntimeRecords(t, store.Dir) != 3 {
		t.Fatalf("concurrent identical proof changed immutable state = %#v err=%v records=%d", status, err, countRuntimeRecords(t, store.Dir))
	}
}

func TestRuntimeSliceProofMutationDuplicateCheckUsesProofEvidence(t *testing.T) {
	const objectiveID = "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	const evidenceRevision = "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	const reportDigest = "sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"
	if err := runtimeSliceProofDuplicateError(nil, objectiveID, evidenceRevision, reportDigest); err != nil {
		t.Fatalf("precheck before concurrent proof = %v", err)
	}
	proofs := []SliceProof{{ObjectiveID: objectiveID, EvidenceRevision: evidenceRevision, ReportDigest: reportDigest}}
	if err := runtimeSliceProofDuplicateError(proofs, objectiveID, evidenceRevision, reportDigest); !errors.Is(err, errRuntimeSliceProofAlreadyRecorded) {
		t.Fatalf("identical proof in mutation state = %v, want exact-match sentinel", err)
	}
	if err := runtimeSliceProofDuplicateError([]SliceProof{{ObjectiveID: objectiveID, EvidenceRevision: evidenceRevision, ReportDigest: "sha256:dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd"}}, objectiveID, evidenceRevision, reportDigest); err == nil || errors.Is(err, errRuntimeSliceProofAlreadyRecorded) {
		t.Fatalf("conflicting proof in mutation state = %v, want conflict", err)
	}
}

func TestRuntimeScopedRescopePreservesScopeAcrossReplay(t *testing.T) {
	repo := initRuntimeLedgerRepo(t)
	store, err := OpenRuntimeStore(context.Background(), repo, "scoped-rescope")
	if err != nil {
		t.Fatal(err)
	}
	scope := &RuntimeScope{Tasks: []string{"1.1"}, Requirements: []string{"REQ-A"}, Scenarios: []string{"scenario-a"}}
	started, err := store.Begin(context.Background(), BeginAttemptRequest{
		RequestID: "scoped-rescope-begin", WorkUnit: "oversized-slice", EvidenceGoal: "prove scoped rescope", MaxAttempts: 2, MaxChangedLines: 40, Scope: scope,
	})
	if err != nil {
		t.Fatal(err)
	}
	interrupted, err := store.Finish(context.Background(), FinishAttemptRequest{
		ExpectedRevision: started.Revision, RequestID: "scoped-rescope-finish", Outcome: AttemptInterrupted, EvidenceRevision: runtimeTestHash('a'),
		Diagnosis: "slice needs a narrower changed-line budget", HarnessDisposition: HarnessReused, CleanupEvidence: "cleanup complete", ProcessEvidence: "processes stopped",
	})
	if err != nil {
		t.Fatal(err)
	}
	rescoped, err := store.Rescope(context.Background(), RescopeObjectiveRequest{
		ExpectedRevision: interrupted.Revision, RequestID: "scoped-rescope", WorkUnit: "narrowed-slice", EvidenceGoal: "prove narrowed scoped rescope", MaxAttempts: 2, MaxChangedLines: 20,
		Reason: "narrow the completed slice budget", Actor: "maintainer",
	})
	if err != nil {
		t.Fatal(err)
	}
	if rescoped.Objective == nil {
		t.Fatalf("rescoped scoped objective = %#v", rescoped.Objective)
	}
	wantID := runtimeObjectiveID(store.Change, "narrowed-slice", "prove narrowed scoped rescope", rescoped.Objective.InitialCandidateIdentity, rescoped.Objective.Generation, scope)
	if rescoped.Objective.ID != wantID || !runtimeScopeEqual(rescoped.Objective.Scope, scope) {
		t.Fatalf("rescoped scoped objective = %#v, want identity %s and scope %#v", rescoped.Objective, wantID, scope)
	}
	replay, err := store.load()
	if err != nil || replay.Status.Objective == nil || replay.Status.Objective.ID != wantID || !runtimeScopeEqual(replay.Status.Objective.Scope, scope) || len(replay.Scopes) != 1 {
		t.Fatalf("replayed scoped rescope = %#v scopes=%#v err=%v", replay.Status.Objective, replay.Scopes, err)
	}
	if _, err := store.Begin(context.Background(), BeginAttemptRequest{
		ExpectedRevision: rescoped.Revision, RequestID: "scoped-rescope-successor-begin", WorkUnit: "narrowed-slice", EvidenceGoal: "prove narrowed scoped rescope", MaxAttempts: 2, MaxChangedLines: 20, Scope: scope,
	}); err != nil {
		t.Fatalf("next permitted scoped begin after rescope = %v", err)
	}
}

func TestRuntimeLegacyScopedRescopeWithoutScopeReplaysPredecessorAssignment(t *testing.T) {
	repo := initRuntimeLedgerRepo(t)
	store, err := OpenRuntimeStore(context.Background(), repo, "legacy-scoped-rescope")
	if err != nil {
		t.Fatal(err)
	}
	scope := &RuntimeScope{Tasks: []string{"1.1"}, Requirements: []string{"REQ-A"}, Scenarios: []string{"scenario-a"}}
	started, err := store.Begin(context.Background(), BeginAttemptRequest{
		RequestID: "legacy-scoped-rescope-begin", WorkUnit: "original-slice", EvidenceGoal: "prove legacy replay", MaxAttempts: 2, MaxChangedLines: 40, Scope: scope,
	})
	if err != nil {
		t.Fatal(err)
	}
	interrupted, err := store.Finish(context.Background(), FinishAttemptRequest{
		ExpectedRevision: started.Revision, RequestID: "legacy-scoped-rescope-finish", Outcome: AttemptInterrupted, EvidenceRevision: runtimeTestHash('a'),
		Diagnosis: "slice needs a narrower changed-line budget", HarnessDisposition: HarnessReused, CleanupEvidence: "cleanup complete", ProcessEvidence: "processes stopped",
	})
	if err != nil {
		t.Fatal(err)
	}
	last := interrupted.Attempts[len(interrupted.Attempts)-1]
	request := RescopeObjectiveRequest{
		ExpectedRevision: interrupted.Revision, RequestID: "legacy-scoped-rescope", WorkUnit: "narrowed-slice", EvidenceGoal: "prove legacy narrowed replay", MaxAttempts: 2, MaxChangedLines: 20,
		Reason: "narrow the legacy slice budget", Actor: "maintainer",
	}
	generation := interrupted.ObjectiveGeneration + 1
	record := runtimeRecord{
		Schema: runtimeRecordSchema, Change: store.Change, PreviousRevision: interrupted.Revision, Operation: runtimeOperationRescope,
		RequestID: request.RequestID, RequestDigest: runtimeValueHash("gentle-ai.sdd-runtime-rescope-request/v1", request),
		Rescope: &runtimeRescopeEvent{
			PreviousObjectiveID: interrupted.Objective.ID, PreviousGeneration: interrupted.Objective.Generation,
			PreviousMaxAttempts: interrupted.Objective.MaxAttempts, PreviousMaxChangedLines: interrupted.Objective.MaxChangedLines,
			RescopeCandidateIdentity: interrupted.Objective.InitialCandidateIdentity, RescopeCandidateTree: last.FinishCandidateTree,
			ObjectiveID: runtimeObjectiveID(store.Change, request.WorkUnit, request.EvidenceGoal, interrupted.Objective.InitialCandidateIdentity, generation, nil), ObjectiveGeneration: generation,
			WorkUnit: request.WorkUnit, EvidenceGoal: request.EvidenceGoal, MaxAttempts: request.MaxAttempts, MaxChangedLines: request.MaxChangedLines, Reason: request.Reason, Actor: request.Actor,
		},
	}
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
	replayed, err := store.Status()
	if err != nil || replayed.Objective == nil || !runtimeScopeEqual(replayed.Objective.Scope, scope) {
		t.Fatalf("legacy scoped rescope replay = %#v err=%v", replayed.Objective, err)
	}
}

func TestRuntimeScopedRescopeReplayRejectsMismatchedPersistedScope(t *testing.T) {
	repo := initRuntimeLedgerRepo(t)
	store, err := OpenRuntimeStore(context.Background(), repo, "scoped-rescope-mismatch")
	if err != nil {
		t.Fatal(err)
	}
	scope := &RuntimeScope{Tasks: []string{"1.1"}, Requirements: []string{"REQ-A"}, Scenarios: []string{"scenario-a"}}
	started, err := store.Begin(context.Background(), BeginAttemptRequest{
		RequestID: "scoped-rescope-mismatch-begin", WorkUnit: "original-slice", EvidenceGoal: "prove mismatched scope replay", MaxAttempts: 2, MaxChangedLines: 40, Scope: scope,
	})
	if err != nil {
		t.Fatal(err)
	}
	interrupted, err := store.Finish(context.Background(), FinishAttemptRequest{
		ExpectedRevision: started.Revision, RequestID: "scoped-rescope-mismatch-finish", Outcome: AttemptInterrupted, EvidenceRevision: runtimeTestHash('a'),
		Diagnosis: "slice needs a narrower changed-line budget", HarnessDisposition: HarnessReused, CleanupEvidence: "cleanup complete", ProcessEvidence: "processes stopped",
	})
	if err != nil {
		t.Fatal(err)
	}
	last := interrupted.Attempts[len(interrupted.Attempts)-1]
	request := RescopeObjectiveRequest{
		ExpectedRevision: interrupted.Revision, RequestID: "scoped-rescope-mismatch", WorkUnit: "narrowed-slice", EvidenceGoal: "prove mismatched persisted scope", MaxAttempts: 2, MaxChangedLines: 20,
		Reason: "narrow the persisted slice budget", Actor: "maintainer",
	}
	mismatchedScope := &RuntimeScope{Tasks: []string{"1.2"}, Requirements: []string{"REQ-B"}, Scenarios: []string{"scenario-b"}}
	generation := interrupted.ObjectiveGeneration + 1
	record := runtimeRecord{
		Schema: runtimeRecordSchema, Change: store.Change, PreviousRevision: interrupted.Revision, Operation: runtimeOperationRescope,
		RequestID: request.RequestID, RequestDigest: runtimeValueHash("gentle-ai.sdd-runtime-rescope-request/v1", request),
		Rescope: &runtimeRescopeEvent{
			PreviousObjectiveID: interrupted.Objective.ID, PreviousGeneration: interrupted.Objective.Generation,
			PreviousMaxAttempts: interrupted.Objective.MaxAttempts, PreviousMaxChangedLines: interrupted.Objective.MaxChangedLines,
			RescopeCandidateIdentity: interrupted.Objective.InitialCandidateIdentity, RescopeCandidateTree: last.FinishCandidateTree,
			ObjectiveID: runtimeObjectiveID(store.Change, request.WorkUnit, request.EvidenceGoal, interrupted.Objective.InitialCandidateIdentity, generation, mismatchedScope), ObjectiveGeneration: generation,
			WorkUnit: request.WorkUnit, EvidenceGoal: request.EvidenceGoal, MaxAttempts: request.MaxAttempts, MaxChangedLines: request.MaxChangedLines, Scope: mismatchedScope, Reason: request.Reason, Actor: request.Actor,
		},
	}
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
	if _, err := store.Status(); err == nil || !strings.Contains(err.Error(), "does not match the terminal objective") {
		t.Fatalf("mismatched persisted scope replay error = %v", err)
	}
}

func TestRuntimeObjectiveIDExplicitScopePreservesHashes(t *testing.T) {
	const change, workUnit, evidenceGoal, candidate = "slice-change", "slice-work", "prove slice", "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	const generation = 3
	scope := &RuntimeScope{Tasks: []string{"1.1"}, Requirements: []string{"REQ-A"}, Scenarios: []string{"scenario-a"}}

	const unscopedWant = "sha256:a2c425cb9b8e698b9dedb543524552b1b7f542291af99fedb77f06b5b2bec45e"
	if got := runtimeObjectiveID(change, workUnit, evidenceGoal, candidate, generation, nil); got != unscopedWant {
		t.Fatalf("unscoped objective ID = %s, want existing hash %s", got, unscopedWant)
	}

	scopedWant := runtimeValueHash(runtimeObjectiveSchemaV2, struct {
		Change            string        `json:"change"`
		WorkUnit          string        `json:"work_unit"`
		EvidenceGoal      string        `json:"evidence_goal"`
		CandidateIdentity string        `json:"candidate_identity"`
		Generation        int           `json:"generation"`
		Scope             *RuntimeScope `json:"scope"`
	}{change, workUnit, evidenceGoal, candidate, generation, scope})
	if got := runtimeObjectiveID(change, workUnit, evidenceGoal, candidate, generation, scope); got != scopedWant {
		t.Fatalf("scoped objective ID = %s, want existing hash %s", got, scopedWant)
	}
}
