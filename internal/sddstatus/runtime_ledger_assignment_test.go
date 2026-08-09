package sddstatus

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
)

func TestNormalizeRejectsNoneSentinelID(t *testing.T) {
	base := BeginAttemptRequest{
		RequestID: "normalize-none", WorkUnit: "assignment-normalization",
		EvidenceGoal: "prove assignment IDs are unambiguous", MaxAttempts: 2, MaxChangedLines: 20,
	}
	for _, test := range []struct {
		name string
		set  func(*BeginAttemptRequest)
	}{
		{name: "requirement", set: func(request *BeginAttemptRequest) {
			request.ObligationAssignmentExplicit = true
			request.AssignedRequirementIDs = []string{"none"}
		}},
		{name: "scenario", set: func(request *BeginAttemptRequest) {
			request.ObligationAssignmentExplicit = true
			request.AssignedScenarioIDs = []string{"none"}
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			request := base
			test.set(&request)
			if _, err := normalizeBeginAttemptRequest(request); err == nil || !strings.Contains(err.Error(), "none") {
				t.Fatalf("normalize(%s) error = %v, want the reserved none sentinel refusal", test.name, err)
			}
		})
	}
}

func TestLegacyLedgerReplaysByteIdentical(t *testing.T) {
	repo := initRuntimeLedgerRepo(t)
	store := mustRuntimeStore(t, repo, "legacy-assignment-replay")
	const evidenceGoal = "prove legacy replay"
	request := BeginAttemptRequest{RequestID: "legacy-begin", WorkUnit: "legacy-work-unit", EvidenceGoal: evidenceGoal, MaxAttempts: 2, MaxChangedLines: 20}
	record := runtimeRecord{
		Schema: runtimeRecordSchema, Change: store.Change, Operation: runtimeOperationBegin,
		RequestID: request.RequestID, RequestDigest: runtimeValueHash("gentle-ai.sdd-runtime-begin-request/v1", request),
		Begin: &runtimeBeginEvent{
			ObjectiveID: legacyRuntimeObjectiveID(store.Change, evidenceGoal), WorkUnit: request.WorkUnit,
			EvidenceGoal: request.EvidenceGoal, MaxAttempts: request.MaxAttempts, MaxChangedLines: request.MaxChangedLines, Ordinal: 1,
			BeginCandidateIdentity: runtimeTestHash('a'), BeginCandidateTree: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		},
	}
	revision, payload, err := runtimeRecordRevision(record)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"obligation_assignment_explicit", "assigned_requirement_ids", "assigned_scenario_ids"} {
		if bytes.Contains(payload, []byte(name)) {
			t.Fatalf("legacy record leaked the new field %q: %s", name, payload)
		}
	}
	if err := store.ensureDirectories(); err != nil {
		t.Fatal(err)
	}
	if err := store.publishRecord(revision, payload); err != nil {
		t.Fatal(err)
	}
	if err := store.publishHead(revision); err != nil {
		t.Fatal(err)
	}
	status, err := store.Status()
	if err != nil {
		t.Fatalf("legacy record did not replay: %v", err)
	}
	if status.Revision != revision || status.Objective == nil || status.Objective.ObligationAssignmentExplicit ||
		len(status.Objective.AssignedRequirementIDs) != 0 || len(status.Objective.AssignedScenarioIDs) != 0 {
		t.Fatalf("legacy replay projected an assignment: %#v", status.Objective)
	}
}

func TestExplicitEmptyVsAbsentAssignment(t *testing.T) {
	base := BeginAttemptRequest{
		RequestID: "explicit-empty", WorkUnit: "zero-obligation-unit",
		EvidenceGoal: "prove the zero-obligation harness", MaxAttempts: 2, MaxChangedLines: 20,
	}
	unbound, err := normalizeBeginAttemptRequest(base)
	if err != nil {
		t.Fatalf("unbound legacy request rejected: %v", err)
	}
	if unbound.ObligationAssignmentExplicit || unbound.AssignedRequirementIDs != nil || unbound.AssignedScenarioIDs != nil {
		t.Fatalf("unbound request was normalized as an assignment: %#v", unbound)
	}

	base.ObligationAssignmentExplicit = true
	base.AssignedRequirementIDs = []string{}
	base.AssignedScenarioIDs = []string{}
	explicit, err := normalizeBeginAttemptRequest(base)
	if err != nil {
		t.Fatalf("explicit empty assignment rejected: %v", err)
	}
	if !explicit.ObligationAssignmentExplicit || len(explicit.AssignedRequirementIDs) != 0 || len(explicit.AssignedScenarioIDs) != 0 {
		t.Fatalf("explicit empty assignment was not preserved: %#v", explicit)
	}

	invalid := base
	invalid.ObligationAssignmentExplicit = false
	invalid.AssignedRequirementIDs = []string{"REQ-1"}
	if _, err := normalizeBeginAttemptRequest(invalid); err == nil {
		t.Fatal("unbound request with non-empty requirement IDs was accepted")
	}

	repo := initRuntimeLedgerRepo(t)
	store := mustRuntimeStore(t, repo, "explicit-empty")
	status, err := store.Begin(context.Background(), explicit)
	if err != nil {
		t.Fatal(err)
	}
	if status.Objective == nil || !status.Objective.ObligationAssignmentExplicit ||
		len(status.Objective.AssignedRequirementIDs) != 0 || len(status.Objective.AssignedScenarioIDs) != 0 {
		t.Fatalf("explicit empty assignment was not bound to objective: %#v", status.Objective)
	}
}

// A continuing attempt must present the exact same assignment the recorded
// objective carries (Requirement: Immutable Obligation ID Assignment /
// Scenario: Continuing attempt presents the identical assignment + Scenario:
// Report ID lists mismatched or assignment altered, begin-validation half):
// the assignment is part of the objective's immutable scope, so an altered or
// silently-unbound continuing attempt is a changed objective, refused with
// the same ErrRuntimeObjectiveChange that already routes a caller to reset or
// rescope.
func TestContinuingAttemptAssignmentMatch(t *testing.T) {
	repo := initRuntimeLedgerRepo(t)
	store := mustRuntimeStore(t, repo, "assignment-continuing")

	bound := BeginAttemptRequest{
		RequestID: "bound-begin", WorkUnit: "objective-scope",
		EvidenceGoal: "prove assignment continuity", MaxAttempts: 4, MaxChangedLines: 20,
		ObligationAssignmentExplicit: true, AssignedRequirementIDs: []string{"REQ-1"}, AssignedScenarioIDs: []string{"S1"},
	}
	started, err := store.Begin(context.Background(), bound)
	if err != nil {
		t.Fatalf("initial bound begin refused: %v", err)
	}
	boundObjectiveID := started.Objective.ID

	intermediate, err := store.Finish(context.Background(), FinishAttemptRequest{
		ExpectedRevision: started.Revision, RequestID: "bound-finish", Outcome: AttemptFailed,
		EvidenceRevision: runtimeTestHash('a'), Diagnosis: "intermediate failed",
		HarnessDisposition: HarnessReused, CleanupEvidence: "cleanup completed",
		ProcessEvidence: "process scan found no descendants",
	})
	if err != nil {
		t.Fatal(err)
	}

	identical := bound
	identical.ExpectedRevision = intermediate.Revision
	identical.RequestID = "identical-begin"
	continued, err := store.Begin(context.Background(), identical)
	if err != nil {
		t.Fatalf("identical continuing attempt was refused: %v", err)
	}
	if continued.Objective == nil || continued.Objective.ID != boundObjectiveID {
		t.Fatalf("identical continuing attempt opened a new objective: %#v", continued.Objective)
	}

	mid, err := store.Finish(context.Background(), FinishAttemptRequest{
		ExpectedRevision: continued.Revision, RequestID: "identical-finish", Outcome: AttemptFailed,
		EvidenceRevision: runtimeTestHash('b'), Diagnosis: "mid failed",
		HarnessDisposition: HarnessReused, CleanupEvidence: "cleanup completed",
		ProcessEvidence: "process scan found no descendants",
	})
	if err != nil {
		t.Fatal(err)
	}

	altered := identical
	altered.ExpectedRevision = mid.Revision
	altered.RequestID = "altered-begin"
	altered.AssignedRequirementIDs = []string{"REQ-1", "REQ-2"}
	if _, err := store.Begin(context.Background(), altered); !errors.Is(err, ErrRuntimeObjectiveChange) {
		t.Fatalf("altered continuing attempt error = %v, want ErrRuntimeObjectiveChange", err)
	}

	alteredScenario := identical
	alteredScenario.ExpectedRevision = mid.Revision
	alteredScenario.RequestID = "altered-scenario-begin"
	alteredScenario.AssignedScenarioIDs = []string{"S1", "S2"}
	if _, err := store.Begin(context.Background(), alteredScenario); !errors.Is(err, ErrRuntimeObjectiveChange) {
		t.Fatalf("altered scenario continuing attempt error = %v, want ErrRuntimeObjectiveChange", err)
	}

	unbound := identical
	unbound.ExpectedRevision = mid.Revision
	unbound.RequestID = "unbound-begin"
	unbound.ObligationAssignmentExplicit = false
	unbound.AssignedRequirementIDs = nil
	unbound.AssignedScenarioIDs = nil
	if _, err := store.Begin(context.Background(), unbound); !errors.Is(err, ErrRuntimeObjectiveChange) {
		t.Fatalf("silent un-bind error = %v, want ErrRuntimeObjectiveChange", err)
	}

	after, err := store.Status()
	if err != nil {
		t.Fatal(err)
	}
	if after.Revision != mid.Revision {
		t.Fatalf("refused attempts mutated the ledger: revision=%q, want %q", after.Revision, mid.Revision)
	}
}

// D4 obligation assignment: rescope may carry the same assignment forward, may
// narrow to a strict subset, and must fail closed on a widened / altered
// assignment, a silent un-bind of a bound previous, or a silent bind of an
// unbound previous. The D4 checks sit beside the budget-widening checks and
// route through the same ErrRuntimeObjectiveChange refusal. Each branch uses
// its own store so a passing branch's records cannot influence the next
// branch's drift check.
func TestRescopeCarryReassignWidenRefusal(t *testing.T) {
	for _, test := range []struct {
		name    string
		request RescopeObjectiveRequest
		wantErr bool
		check   func(t *testing.T, status RuntimeStatus, request RescopeObjectiveRequest)
	}{
		{
			name: "carry-forward",
			request: RescopeObjectiveRequest{
				WorkUnit: "carry-scope", EvidenceGoal: "d4 carry-forward",
				MaxAttempts: 2, MaxChangedLines: 20, Reason: "d4 carry-forward", Actor: "maintainer",
				ObligationAssignmentExplicit: true,
				AssignedRequirementIDs:       []string{"REQ-1", "REQ-2"},
				AssignedScenarioIDs:          []string{"S1", "S2"},
			},
			check: func(t *testing.T, status RuntimeStatus, _ RescopeObjectiveRequest) {
				if !status.Objective.ObligationAssignmentExplicit ||
					len(status.Objective.AssignedRequirementIDs) != 2 ||
					len(status.Objective.AssignedScenarioIDs) != 2 {
					t.Fatalf("carry-forward did not preserve the assignment: %#v", status.Objective)
				}
			},
		},
		{
			name: "narrowing-subset",
			request: RescopeObjectiveRequest{
				WorkUnit: "narrow-scope", EvidenceGoal: "d4 narrower subset",
				MaxAttempts: 2, MaxChangedLines: 20, Reason: "d4 narrowing", Actor: "maintainer",
				ObligationAssignmentExplicit: true,
				AssignedRequirementIDs:       []string{"REQ-1"},
				AssignedScenarioIDs:          []string{"S1"},
			},
			check: func(t *testing.T, status RuntimeStatus, _ RescopeObjectiveRequest) {
				if len(status.Objective.AssignedRequirementIDs) != 1 ||
					status.Objective.AssignedRequirementIDs[0] != "REQ-1" {
					t.Fatalf("narrowing reassign did not narrow: %#v", status.Objective)
				}
			},
		},
		{
			name: "altered",
			request: RescopeObjectiveRequest{
				WorkUnit: "altered-scope", EvidenceGoal: "d4 altered",
				MaxAttempts: 2, MaxChangedLines: 20, Reason: "d4 altered", Actor: "maintainer",
				ObligationAssignmentExplicit: true,
				AssignedRequirementIDs:       []string{"REQ-1", "REQ-3"},
				AssignedScenarioIDs:          []string{"S1", "S2"},
			},
			wantErr: true,
		},
		{
			name: "silent-unbind",
			request: RescopeObjectiveRequest{
				WorkUnit: "unbind-scope", EvidenceGoal: "d4 unbind",
				MaxAttempts: 2, MaxChangedLines: 20, Reason: "d4 unbind", Actor: "maintainer",
				ObligationAssignmentExplicit: false,
			},
			wantErr: true,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			repo := initRuntimeLedgerRepo(t)
			store := mustRuntimeStore(t, repo, "rescope-d4-"+test.name)
			begin, err := store.Begin(context.Background(), BeginAttemptRequest{
				RequestID: "d4-begin", WorkUnit: "d4-scope", EvidenceGoal: "d4 prelude",
				MaxAttempts: 2, MaxChangedLines: 20,
				ObligationAssignmentExplicit: true,
				AssignedRequirementIDs:       []string{"REQ-1", "REQ-2"},
				AssignedScenarioIDs:          []string{"S1", "S2"},
			})
			if err != nil {
				t.Fatal(err)
			}
			failed, err := store.Finish(context.Background(), FinishAttemptRequest{
				ExpectedRevision: begin.Revision, RequestID: "d4-finish", Outcome: AttemptFailed,
				EvidenceRevision: runtimeTestHash('a'), Diagnosis: "d4 prelude",
				HarnessDisposition: HarnessReused, CleanupEvidence: "cleanup completed",
				ProcessEvidence: "process scan found no descendants",
			})
			if err != nil {
				t.Fatal(err)
			}
			test.request.ExpectedRevision = failed.Revision
			test.request.RequestID = "d4-rescope-" + test.name
			rescoped, err := store.Rescope(context.Background(), test.request)
			if test.wantErr {
				if !errors.Is(err, ErrRuntimeObjectiveChange) {
					t.Fatalf("rescope %s error = %v, want ErrRuntimeObjectiveChange", test.name, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("rescope %s refused: %v", test.name, err)
			}
			if test.check != nil {
				test.check(t, rescoped, test.request)
			}
		})
	}
}

// runtimeRescopeAssignmentAdmissibleUnbound covers the unbound → present
// failure closed by D4. A predecessor that did not bind an assignment cannot
// be silently bound by a rescope -- the slice identity stays unbound.
func TestRescopeAssignmentUnboundToBoundRefused(t *testing.T) {
	repo := initRuntimeLedgerRepo(t)
	store := mustRuntimeStore(t, repo, "rescope-unbound-bind")

	started, err := store.Begin(context.Background(), BeginAttemptRequest{
		RequestID: "unbound-begin", WorkUnit: "u-scope", EvidenceGoal: "unbound prelude",
		MaxAttempts: 2, MaxChangedLines: 20,
	})
	if err != nil {
		t.Fatal(err)
	}
	failed, err := store.Finish(context.Background(), FinishAttemptRequest{
		ExpectedRevision: started.Revision, RequestID: "unbound-finish", Outcome: AttemptFailed,
		EvidenceRevision: runtimeTestHash('c'), Diagnosis: "unbound prelude",
		HarnessDisposition: HarnessReused, CleanupEvidence: "cleanup completed",
		ProcessEvidence: "process scan found no descendants",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Rescope(context.Background(), RescopeObjectiveRequest{
		ExpectedRevision: failed.Revision, RequestID: "unbound-rescope-bind",
		WorkUnit: "u-scope-narrower", EvidenceGoal: "u scope narrower",
		MaxAttempts: 2, MaxChangedLines: 20, Reason: "u scope narrower", Actor: "maintainer",
		ObligationAssignmentExplicit: true,
		AssignedRequirementIDs:       []string{"REQ-1"},
		AssignedScenarioIDs:          []string{"S1"},
	}); !errors.Is(err, ErrRuntimeObjectiveChange) {
		t.Fatalf("unbound → bound rescope error = %v, want ErrRuntimeObjectiveChange", err)
	}
}

// SliceAssignments projection: excludes unbound objectives by definition, and
// applies the D6 supersede rule (rescope ancestors and reset predecessors
// excluded; advance predecessors retained). The runtime ledger never reorders
// the chain, so the projection is stable as long as the chain is.
func TestSliceAssignmentsProjectionExcludesUnbound(t *testing.T) {
	repo := initRuntimeLedgerRepo(t)
	store := mustRuntimeStore(t, repo, "slice-assignments")

	boundBegin, err := store.Begin(context.Background(), BeginAttemptRequest{
		RequestID: "sa-bound-begin", WorkUnit: "bound-scope", EvidenceGoal: "bound slice",
		MaxAttempts: 2, MaxChangedLines: 20,
		ObligationAssignmentExplicit: true,
		AssignedRequirementIDs:       []string{"REQ-1"},
		AssignedScenarioIDs:          []string{"S1"},
	})
	if err != nil {
		t.Fatal(err)
	}

	assignments, err := store.SliceAssignments()
	if err != nil {
		t.Fatal(err)
	}
	if len(assignments) != 1 {
		t.Fatalf("projection leaked an unbound objective: assignments=%#v", assignments)
	}
	if assignments[0].SliceID != boundBegin.Objective.ID ||
		len(assignments[0].RequirementIDs) != 1 || assignments[0].RequirementIDs[0] != "REQ-1" {
		t.Fatalf("projection returned the wrong bound slice: %#v", assignments[0])
	}

	failed, err := store.Finish(context.Background(), FinishAttemptRequest{
		ExpectedRevision: boundBegin.Revision, RequestID: "sa-finish", Outcome: AttemptFailed,
		EvidenceRevision: runtimeTestHash('d'), Diagnosis: "sa prelude",
		HarnessDisposition: HarnessReused, CleanupEvidence: "cleanup completed",
		ProcessEvidence: "process scan found no descendants",
	})
	if err != nil {
		t.Fatal(err)
	}
	narrow, err := store.Rescope(context.Background(), RescopeObjectiveRequest{
		ExpectedRevision: failed.Revision, RequestID: "sa-rescope",
		WorkUnit: "narrower-scope", EvidenceGoal: "narrower slice",
		MaxAttempts: 2, MaxChangedLines: 20, Reason: "narrower", Actor: "maintainer",
		ObligationAssignmentExplicit: true,
		AssignedRequirementIDs:       []string{"REQ-1"},
		AssignedScenarioIDs:          []string{"S1"},
	})
	if err != nil {
		t.Fatal(err)
	}

	assignments, err = store.SliceAssignments()
	if err != nil {
		t.Fatal(err)
	}
	if len(assignments) != 1 {
		t.Fatalf("rescoped predecessor was not excluded: assignments=%#v", assignments)
	}
	if assignments[0].SliceID != narrow.Objective.ID {
		t.Fatalf("projection returned the wrong post-rescope slice: %#v", assignments[0])
	}

	passed, err := store.Begin(context.Background(), BeginAttemptRequest{
		ExpectedRevision: narrow.Revision, RequestID: "sa-pass-begin", WorkUnit: "narrower-scope",
		EvidenceGoal: "narrower slice", MaxAttempts: 2, MaxChangedLines: 20,
		ObligationAssignmentExplicit: true,
		AssignedRequirementIDs:       []string{"REQ-1"},
		AssignedScenarioIDs:          []string{"S1"},
	})
	if err != nil {
		t.Fatal(err)
	}
	passedStatus, err := store.Finish(context.Background(), FinishAttemptRequest{
		ExpectedRevision: passed.Revision, RequestID: "sa-pass-finish", Outcome: AttemptPassed,
		EvidenceRevision: runtimeTestHash('e'), Diagnosis: "narrower passed",
		HarnessDisposition: HarnessReused, CleanupEvidence: "cleanup completed",
		ProcessEvidence: "process scan found no descendants",
	})
	if err != nil {
		t.Fatal(err)
	}
	advanced, err := store.Begin(context.Background(), BeginAttemptRequest{
		ExpectedRevision: passedStatus.Revision, RequestID: "sa-advance-begin", WorkUnit: "next-scope",
		EvidenceGoal: "next slice after the bound predecessor",
		MaxAttempts:  2, MaxChangedLines: 20,
		ObligationAssignmentExplicit: true,
		AssignedRequirementIDs:       []string{"REQ-2"},
		AssignedScenarioIDs:          []string{"S2"},
	})
	if err != nil {
		t.Fatal(err)
	}
	assignments, err = store.SliceAssignments()
	if err != nil {
		t.Fatal(err)
	}
	if len(assignments) != 2 {
		t.Fatalf("advance-predecessor was not retained: assignments=%#v", assignments)
	}
	gotIDs := map[string]bool{}
	for _, entry := range assignments {
		gotIDs[entry.SliceID] = true
	}
	if !gotIDs[narrow.Objective.ID] || !gotIDs[advanced.Objective.ID] {
		t.Fatalf("projection lost the advance-predecessor or the current slice: %v", gotIDs)
	}
}

// Idempotent acquire replay: an Acquire that re-issues the same begin request
// (including its bound assignment) must hit the same record and return the
// same ownership token without publishing a new attempt.
func TestIdempotentAcquireReplayWithAssignment(t *testing.T) {
	repo := initRuntimeLedgerRepo(t)
	store := mustRuntimeStore(t, repo, "acquire-idempotent-assign")

	request := CompactAcquireRequest{
		BeginAttemptRequest: BeginAttemptRequest{
			RequestID: "acquire-assign", WorkUnit: "acquire-scope", EvidenceGoal: "acquire idempotence",
			MaxAttempts: 2, MaxChangedLines: 20,
			ObligationAssignmentExplicit: true,
			AssignedRequirementIDs:       []string{"REQ-1"},
			AssignedScenarioIDs:          []string{"S1"},
		},
	}
	first, err := store.Acquire(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if first.State != CompactStateProceed || first.Token == "" {
		t.Fatalf("initial acquire = %#v", first)
	}
	before := countRuntimeRecords(t, store.Dir)

	replayed, err := store.Acquire(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if replayed != first {
		t.Fatalf("acquire replay diverged: got=%#v want=%#v", replayed, first)
	}
	if countRuntimeRecords(t, store.Dir) != before {
		t.Fatalf("acquire replay wrote a new record: records=%d, want %d",
			countRuntimeRecords(t, store.Dir), before)
	}
}
