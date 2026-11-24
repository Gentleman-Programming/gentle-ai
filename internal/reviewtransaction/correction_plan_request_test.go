package reviewtransaction

import (
	"reflect"
	"strings"
	"testing"
)

// persistMixedOutcomeCorrectionAuthority builds a persisted
// correction_required authority whose fix set is produced by admitted captures
// alone. The refuted finding stays out of the fix set; the inconclusive
// refuter outcome stays in it exactly as compact authority produces it
// (issue #5888).
func persistMixedOutcomeCorrectionAuthority(t *testing.T, repo, lineage string) (CompactState, CompactStore) {
	t.Helper()
	writeSnapshotFile(t, repo, "tracked.txt", "base\none\ntwo\nthree\nwrong\n")
	state := newCompactTestState(t, repo, lineage)
	if len(state.SelectedLenses) == 0 {
		t.Fatal("mixed-outcome fixture unexpectedly selected no lenses")
	}
	state, store := startReviewingCompactAuthority(t, repo, state)
	deterministic := Finding{
		ID: "R3-001", Lens: state.SelectedLenses[0], Location: "tracked.txt:2", Severity: "CRITICAL",
		Claim: "candidate exposes the wrong behavior", ProofRefs: []string{"reproduced candidate failure"},
		EvidenceClass: EvidenceDeterministic, CausalDisposition: CausalIntroduced,
	}
	inconclusive := Finding{
		ID: "R3-002", Lens: state.SelectedLenses[0], Location: "tracked.txt:3", Severity: "CRITICAL",
		Claim: "candidate worsens the behavior", ProofRefs: []string{"candidate-specific interpretation"},
		EvidenceClass: EvidenceInferential, CausalDisposition: CausalIntroduced,
	}
	refuted := Finding{
		ID: "R3-003", Lens: state.SelectedLenses[0], Location: "tracked.txt:4", Severity: "CRITICAL",
		Claim: "candidate breaks the behavior", ProofRefs: []string{"candidate-specific interpretation"},
		EvidenceClass: EvidenceInferential, CausalDisposition: CausalIntroduced,
	}
	next, record := captureAndCompleteCompactReview(t, store, state, CompactReviewInput{
		LensResults: []LensResult{{
			Lens: state.SelectedLenses[0], Findings: []Finding{deterministic, inconclusive, refuted},
			Evidence: []string{"inspected the complete frozen candidate scope"},
		}},
		Classifications: []FindingEvidence{
			{FindingID: deterministic.ID, Class: EvidenceDeterministic, Causality: CausalIntroduced, Proof: "reproduced candidate failure"},
			{FindingID: inconclusive.ID, Class: EvidenceInferential, Causality: CausalIntroduced, Proof: "candidate-specific interpretation"},
			{FindingID: refuted.ID, Class: EvidenceInferential, Causality: CausalIntroduced, Proof: "candidate-specific interpretation"},
		},
		RefuterOutcomes: []EvidenceResult{
			{FindingID: inconclusive.ID, Outcome: OutcomeInconclusive, Proof: "could not disprove"},
			{FindingID: refuted.ID, Outcome: OutcomeRefuted, Proof: "disproved on the base tree"},
		},
	})
	if next.State != StateCorrectionRequired {
		t.Fatalf("mixed-outcome fixture state = %q, want %q", next.State, StateCorrectionRequired)
	}
	if !reflect.DeepEqual(next.FixFindingIDs, []string{"R3-001", "R3-002"}) {
		t.Fatalf("mixed-outcome fix set = %v, want the corroborated and inconclusive findings only", next.FixFindingIDs)
	}
	if _, err := store.Replace(record.Revision, "review/complete-review", next); err != nil {
		t.Fatal(err)
	}
	return next, store
}

// TestCorrectionPlanRequestAcceptsCompactAuthorizedInconclusiveFixFindings
// pins issue #5888: compact authority admits both corroborated and
// inconclusive refuter outcomes into the fix set, so the correction-plan
// request builder must project the exact authorized set without rewriting any
// outcome, spending budget, or consuming an attempt.
func TestCorrectionPlanRequestAcceptsCompactAuthorizedInconclusiveFixFindings(t *testing.T) {
	t.Run("mixed corroborated and inconclusive findings", func(t *testing.T) {
		state, store := persistMixedOutcomeCorrectionAuthority(t, initSnapshotRepo(t), "correction-plan-mixed")
		request, err := BuildCorrectionPlanRequest(state, state.CapturePhaseRevision)
		if err != nil {
			t.Fatalf("mixed-outcome correction plan request: %v", err)
		}
		if !reflect.DeepEqual(request.FixFindingIDs, []string{"R3-001", "R3-002"}) || len(request.Findings) != 2 ||
			request.Findings[0].ID != "R3-001" || request.Findings[1].ID != "R3-002" ||
			request.Findings[1].EvidenceClass != EvidenceInferential || request.Findings[1].CausalDisposition != CausalIntroduced {
			t.Fatalf("mixed-outcome correction request = %#v", request)
		}
		if ValidateCorrectionPlanRequest(request) != nil {
			t.Fatal("mixed-outcome correction request failed its own contract validation")
		}
		// The original inconclusive outcome must remain inconclusive; bounded
		// correction never fabricates corroboration.
		view, err := state.CompactReviewView()
		if err != nil {
			t.Fatal(err)
		}
		if view.Outcomes["R3-002"] != OutcomeInconclusive || view.Outcomes["R3-001"] != OutcomeCorroborated {
			t.Fatalf("outcomes after correction planning = %#v", view.Outcomes)
		}
		reloaded, err := store.Load()
		if err != nil {
			t.Fatal(err)
		}
		reloadedRequest, err := BuildCorrectionPlanRequest(reloaded.State, reloaded.State.CapturePhaseRevision)
		if err != nil {
			t.Fatalf("fresh-load mixed authority correction plan request: %v", err)
		}
		if !reflect.DeepEqual(reloadedRequest, request) {
			t.Fatalf("fresh-load correction request changed: %#v vs %#v", reloadedRequest, request)
		}
		if len(reloaded.State.CorrectionAttempts) != 0 || reloaded.State.CumulativeCorrectionLines != 0 {
			t.Fatalf("correction planning consumed authority accounting: %#v", reloaded.State)
		}
	})

	t.Run("inconclusive-only findings keep bounded correction available", func(t *testing.T) {
		repo := initSnapshotRepo(t)
		writeSnapshotFile(t, repo, "tracked.txt", "base\none\ntwo\nthree\nwrong\n")
		state := newCompactTestState(t, repo, "correction-plan-inconclusive-only")
		state, store := startReviewingCompactAuthority(t, repo, state)
		finding := Finding{
			ID: "R3-002", Lens: state.SelectedLenses[0], Location: "tracked.txt:2", Severity: "CRITICAL",
			Claim: "candidate worsens the behavior", ProofRefs: []string{"candidate-specific interpretation"},
			EvidenceClass: EvidenceInferential, CausalDisposition: CausalIntroduced,
		}
		next, _ := captureAndCompleteCompactReview(t, store, state, CompactReviewInput{
			LensResults: []LensResult{{
				Lens: state.SelectedLenses[0], Findings: []Finding{finding},
				Evidence: []string{"inspected the complete frozen candidate scope"},
			}},
			Classifications: []FindingEvidence{
				{FindingID: finding.ID, Class: EvidenceInferential, Causality: CausalIntroduced, Proof: "candidate-specific interpretation"},
			},
			RefuterOutcomes: []EvidenceResult{
				{FindingID: finding.ID, Outcome: OutcomeInconclusive, Proof: "could not disprove"},
			},
		})
		if next.State != StateCorrectionRequired {
			t.Fatalf("inconclusive-only fixture state = %q, want %q", next.State, StateCorrectionRequired)
		}
		request, err := BuildCorrectionPlanRequest(next, next.CapturePhaseRevision)
		if err != nil {
			t.Fatalf("inconclusive-only correction plan request: %v", err)
		}
		if !reflect.DeepEqual(request.FixFindingIDs, []string{"R3-002"}) || len(request.Findings) != 1 {
			t.Fatalf("inconclusive-only correction request = %#v", request)
		}
		view, err := next.CompactReviewView()
		if err != nil {
			t.Fatal(err)
		}
		if view.Outcomes["R3-002"] != OutcomeInconclusive {
			t.Fatalf("inconclusive outcome was rewritten: %#v", view.Outcomes)
		}
	})

	t.Run("corroborated-only findings keep existing behavior", func(t *testing.T) {
		repo := initSnapshotRepo(t)
		writeSnapshotFile(t, repo, "tracked.txt", "base\none\ntwo\nthree\nwrong\n")
		state := newCompactTestState(t, repo, "correction-plan-corroborated-only")
		state, store := startReviewingCompactAuthority(t, repo, state)
		finding := Finding{
			ID: "R3-001", Lens: state.SelectedLenses[0], Location: "tracked.txt:2", Severity: "CRITICAL",
			Claim: "candidate exposes the wrong behavior", ProofRefs: []string{"reproduced candidate failure"},
			EvidenceClass: EvidenceDeterministic, CausalDisposition: CausalIntroduced,
		}
		next, _ := captureAndCompleteCompactReview(t, store, state, CompactReviewInput{
			LensResults: []LensResult{{
				Lens: state.SelectedLenses[0], Findings: []Finding{finding},
				Evidence: []string{"inspected the complete frozen candidate scope"},
			}},
			Classifications: []FindingEvidence{
				{FindingID: finding.ID, Class: EvidenceDeterministic, Causality: CausalIntroduced, Proof: "reproduced candidate failure"},
			},
			RefuterOutcomes: []EvidenceResult{},
		})
		if next.State != StateCorrectionRequired {
			t.Fatalf("corroborated-only fixture state = %q, want %q", next.State, StateCorrectionRequired)
		}
		request, err := BuildCorrectionPlanRequest(next, next.CapturePhaseRevision)
		if err != nil {
			t.Fatalf("corroborated-only correction plan request: %v", err)
		}
		if !reflect.DeepEqual(request.FixFindingIDs, []string{"R3-001"}) || len(request.Findings) != 1 {
			t.Fatalf("corroborated-only correction request = %#v", request)
		}
	})

	t.Run("refuted findings stay excluded and injected IDs are rejected", func(t *testing.T) {
		state, _ := persistMixedOutcomeCorrectionAuthority(t, initSnapshotRepo(t), "correction-plan-refuted-excluded")
		for _, injected := range []struct {
			name string
			id   string
		}{
			{name: "refuted finding id", id: "R3-003"},
			{name: "unknown finding id", id: "R3-999"},
		} {
			tampered := state
			tampered.FixFindingIDs = append(append([]string(nil), state.FixFindingIDs...), injected.id)
			if _, err := BuildCorrectionPlanRequest(tampered, tampered.CapturePhaseRevision); err == nil ||
				!strings.Contains(err.Error(), "not satisfied by admitted compact authority") {
				t.Fatalf("injected %s was accepted: %v", injected.name, err)
			}
		}
	})
}
