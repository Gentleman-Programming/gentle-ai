package reviewtransaction

import (
	"reflect"
	"testing"
)

func TestCompactRecoveredEvidenceOwnsOnlyAccountingReferences(t *testing.T) {
	typeOfEvidence := reflect.TypeOf(CompactRecoveredEvidence{})
	for _, field := range []string{
		"SourceCorrectionAttempt",
		"TargetedValidationRequest",
		"ReviewEvidenceHash",
		"SuccessorTargetIdentity",
	} {
		if _, found := typeOfEvidence.FieldByName(field); found {
			t.Fatalf("recovered evidence still owns forbidden copied field %q", field)
		}
	}
	for _, field := range []string{"PredecessorTargetIdentity", "AdmittedRoleReferences"} {
		if _, found := typeOfEvidence.FieldByName(field); !found {
			t.Fatalf("recovered evidence is missing accounting field %q", field)
		}
	}
	typeOfReference := reflect.TypeOf(CompactRecoveredEvidenceReference{})
	for _, field := range []string{"Role", "Lens", "SelectedOrder", "TargetIdentity", "CapturePhaseRevision", "RequestHash", "ArtifactDigest"} {
		if _, found := typeOfReference.FieldByName(field); !found {
			t.Fatalf("recovered evidence reference is missing binding field %q", field)
		}
	}
	for _, field := range []string{"Value", "Path", "Digest", "ResultHash"} {
		if _, found := typeOfReference.FieldByName(field); found {
			t.Fatalf("recovered evidence reference owns forbidden payload or duplicate digest field %q", field)
		}
	}
}

func TestCompactRecoveredEvidenceReferencesRejectNonCanonicalBindings(t *testing.T) {
	predecessor, evidence, request := accountingRecoveryReferenceFixture(t)
	if err := validateCompactRecoveredEvidenceReferences(predecessor, evidence); err != nil {
		t.Fatalf("valid accounting references: %v", err)
	}
	rebuilt, err := rebuildCompactRecoveredTargetedValidationRequest(predecessor, evidence)
	if err != nil {
		t.Fatalf("rebuild targeted validation request: %v", err)
	}
	if !reflect.DeepEqual(rebuilt, request) || rebuilt.ExpectedRevision != predecessor.CapturePhaseRevision {
		t.Fatalf("recovered request = %#v, want frozen Pn-bound request %#v", rebuilt, request)
	}
	liveRevision, err := CompactRevisionForState(predecessor)
	if err != nil || rebuilt.ExpectedRevision == liveRevision {
		t.Fatalf("recovered request bound live Rn %q instead of frozen Pn %q: %v", liveRevision, predecessor.CapturePhaseRevision, err)
	}

	cases := map[string]func(*CompactRecoveredEvidence){
		"duplicate": func(value *CompactRecoveredEvidence) {
			value.AdmittedRoleReferences = append(value.AdmittedRoleReferences, value.AdmittedRoleReferences[0])
		},
		"unordered": func(value *CompactRecoveredEvidence) {
			value.AdmittedRoleReferences[0], value.AdmittedRoleReferences[1] = value.AdmittedRoleReferences[1], value.AdmittedRoleReferences[0]
		},
		"unknown digest": func(value *CompactRecoveredEvidence) {
			value.AdmittedRoleReferences[0].ArtifactDigest = hash("a")
		},
		"stale predecessor target": func(value *CompactRecoveredEvidence) {
			value.PredecessorTargetIdentity = hash("b")
		},
		"stale target": func(value *CompactRecoveredEvidence) {
			value.AdmittedRoleReferences[0].TargetIdentity = hash("b")
		},
		"stale phase": func(value *CompactRecoveredEvidence) {
			value.AdmittedRoleReferences[0].CapturePhaseRevision = hash("c")
		},
		"stale validator phase": func(value *CompactRecoveredEvidence) {
			value.AdmittedRoleReferences[len(value.AdmittedRoleReferences)-1].CapturePhaseRevision = hash("c")
		},
		"missing admitted entry": func(value *CompactRecoveredEvidence) {
			value.AdmittedRoleReferences = value.AdmittedRoleReferences[:len(value.AdmittedRoleReferences)-1]
		},
		"stale validator request": func(value *CompactRecoveredEvidence) {
			value.AdmittedRoleReferences[len(value.AdmittedRoleReferences)-1].RequestHash = hash("d")
		},
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			changed := cloneCompactRecoveredEvidence(evidence)
			mutate(&changed)
			if err := validateCompactRecoveredEvidenceReferences(predecessor, changed); err == nil {
				t.Fatalf("accepted %s accounting references: %#v", name, changed)
			}
			if _, err := rebuildCompactRecoveredTargetedValidationRequest(predecessor, changed); err == nil {
				t.Fatalf("rebuilt targeted request from %s accounting references", name)
			}
		})
	}
}

func TestCompactRecoveredEvidenceReferencesAreNotActiveCaptureSlots(t *testing.T) {
	predecessor, evidence, _ := accountingRecoveryReferenceFixture(t)
	successor := newCompactTestState(t, initSnapshotRepo(t), "accounting-reference-classification")
	successor.Recovery = &CompactRecoveryProvenance{Evidence: &evidence}
	for _, admitted := range predecessor.AdmittedRoleResults {
		if !compactAdmittedRoleResultIsAccountingOnly(successor, admitted) {
			t.Fatalf("accounting reference did not classify admitted role result: %#v", admitted)
		}
		if compactAdmittedRoleResultCanSatisfyActiveCapture(successor, admitted) {
			t.Fatalf("accounting reference satisfied an active capture slot: %#v", admitted)
		}
	}
	fresh := predecessor.AdmittedRoleResults[0]
	fresh.ArtifactDigest = hash("e")
	if compactAdmittedRoleResultIsAccountingOnly(successor, fresh) || !compactAdmittedRoleResultCanSatisfyActiveCapture(successor, fresh) {
		t.Fatalf("unreferenced role result classification = accounting=%v active=%v", compactAdmittedRoleResultIsAccountingOnly(successor, fresh), compactAdmittedRoleResultCanSatisfyActiveCapture(successor, fresh))
	}
}

func accountingRecoveryReferenceFixture(t *testing.T) (CompactState, CompactRecoveredEvidence, TargetedValidationRequest) {
	t.Helper()
	repo := initSnapshotRepo(t)
	writeSnapshotFile(t, repo, "tracked.txt", "base\nwrong\n")
	predecessor := newCompactTestState(t, repo, "accounting-reference-predecessor")
	policy := "frozen accounting recovery policy"
	predecessor.FrozenPolicyContent, predecessor.PolicyHash = &policy, compactPolicyContentHash(policy)
	predecessor.CorrectionBudget, predecessor.CorrectionBudgetPolicy = 1, ""
	phase, err := deriveCompactCapturePhaseRevision(predecessor)
	if err != nil {
		t.Fatal(err)
	}
	predecessor.CapturePhaseRevision = phase
	store, err := CompactAuthoritativeStore(t.Context(), repo, predecessor.LineageID)
	if err != nil {
		t.Fatal(err)
	}
	record := writeCompactFixtureRecord(t, store, predecessor)
	finding := Finding{
		ID: "R3-001", Lens: "reliability", Location: "tracked.txt:2", Severity: "CRITICAL",
		Claim: "wrong value", ProofRefs: []string{"changed hunk causes failure"},
		EvidenceClass: EvidenceDeterministic, CausalDisposition: CausalIntroduced,
	}
	captureAdmittedCorrectionFinding(t, store, predecessor, finding)
	record, err = store.Load()
	if err != nil {
		t.Fatal(err)
	}
	predecessor = record.State
	view, err := predecessor.CompactReviewView()
	if err != nil {
		t.Fatal(err)
	}
	if err := predecessor.CompleteReview(CompactReviewInput{
		LensResults: view.LensResults, Classifications: []FindingEvidence{view.Classifications[finding.ID]},
	}); err != nil {
		t.Fatal(err)
	}
	record.Revision, err = store.Replace(record.Revision, "review/complete-review", predecessor)
	if err != nil {
		t.Fatal(err)
	}
	if err := predecessor.BeginCorrection(1); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Replace(record.Revision, "review/begin-fix", predecessor); err != nil {
		t.Fatal(err)
	}
	writeSnapshotFile(t, repo, "tracked.txt", "base\nfixed\n")
	fix, err := (SnapshotBuilder{Repo: repo}).Build(t.Context(), Target{
		Kind: TargetFixDiff, Projection: predecessor.InitialSnapshot.Projection,
		BaseRef: predecessor.CurrentSnapshot.CandidateTree, IntendedUntracked: predecessor.InitialSnapshot.IntendedUntracked,
		LedgerIDs: predecessor.FixFindingIDs,
	})
	if err != nil {
		t.Fatal(err)
	}
	request, err := BuildTargetedValidationRequestFromSnapshot(t.Context(), repo, predecessor, predecessor.CapturePhaseRevision, fix)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.CaptureAdmittedTargetedValidatorResult(t.Context(), CompactAdmittedTargetedValidatorResultRequest{
		ExpectedRequest: request, Payload: []byte(`{"outcome":"passed"}`),
	}); err != nil {
		t.Fatal(err)
	}
	record, err = store.Load()
	if err != nil {
		t.Fatal(err)
	}
	predecessor = record.State
	fixHash := FixDeltaHashForSnapshot(fix)
	validation := ScopedValidationResult{
		LedgerIDs: predecessor.FixFindingIDs, FixCausedFindings: []Finding{}, FollowUps: []FollowUp{},
		OriginalCriteria:              ValidationCheck{EvidenceHash: hash("2"), FixDeltaHash: fixHash, Passed: true},
		CorrectionRegression:          ValidationCheck{EvidenceHash: hash("3"), FixDeltaHash: fixHash, Passed: true},
		TargetedValidationRequestHash: request.RequestHash, CorrectionTargetIdentity: fix.Identity,
	}
	if err := predecessor.CompleteCorrection(fix, 1, validation); err != nil {
		t.Fatal(err)
	}
	actual := 2
	predecessor.State = StateEscalated
	predecessor.CorrectionAttempts[0].ActualLines = actual
	predecessor.CumulativeCorrectionLines, predecessor.ActualCorrectionLines = actual, &actual
	if err := predecessor.Validate(); err != nil {
		t.Fatalf("validate accounting predecessor: %v", err)
	}
	evidence := CompactRecoveredEvidence{
		Schema:                    CompactRecoveredEvidenceSchema,
		Relation:                  string(compactTargetChangedScope),
		PathRelation:              string(compactPathsSame),
		PredecessorTargetIdentity: predecessor.InitialSnapshot.Identity,
		NativeCorrectionLines:     1,
		AdmittedRoleReferences:    compactRecoveredEvidenceReferences(predecessor),
	}
	return predecessor, evidence, request
}

func cloneCompactRecoveredEvidence(value CompactRecoveredEvidence) CompactRecoveredEvidence {
	value.AdmittedRoleReferences = append([]CompactRecoveredEvidenceReference(nil), value.AdmittedRoleReferences...)
	return value
}
