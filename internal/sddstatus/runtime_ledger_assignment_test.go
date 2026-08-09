package sddstatus

import (
	"bytes"
	"context"
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
	// omitempty must keep these new fields out of the wire bytes when their
	// values are zero. A leaked key (with a null or empty value) is the only
	// shape that would still round-trip through replay but break byte-identity
	// for pre-#2268 readers.
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
