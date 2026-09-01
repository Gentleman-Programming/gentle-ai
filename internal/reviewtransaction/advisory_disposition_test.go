package reviewtransaction

import (
	"encoding/json"
	"reflect"
	"testing"
)

func admittedAdvisoryState(t *testing.T, lineage string, findings []Finding) CompactState {
	t.Helper()
	fixture := newCompactReviewerCaptureFixture(t, lineage)
	request := fixture.request
	request.Result = LensResult{
		Lens: LensReliability, Findings: findings,
		Evidence: []string{"inspected the complete frozen candidate scope"},
	}
	raw, err := json.Marshal(compactProviderReviewerResult{
		SubjectHash: request.ArtifactSubject.SubjectHash, Inspection: request.Inspection,
		Lens: request.Result.Lens, Findings: request.Result.Findings, Evidence: request.Result.Evidence,
	})
	if err != nil {
		t.Fatal(err)
	}
	request.RawPayload = append(raw, '\n')
	if _, err := fixture.store.CaptureAdmittedReviewerResult(t.Context(), request); err != nil {
		t.Fatal(err)
	}
	record, err := fixture.store.Load()
	if err != nil {
		t.Fatal(err)
	}
	state := record.State
	state.State = StateApproved
	return state
}

// TestAdvisoryFindingsDeclareNonBlockingRouting reads both the informational
// and separately actionable outcomes from the admitted reviewer value.
func TestAdvisoryFindingsDeclareNonBlockingRouting(t *testing.T) {
	state := admittedAdvisoryState(t, "advisory-disposition", []Finding{
		{ID: "R3-001", Lens: LensReliability, Location: "internal/a.go:3", Severity: "WARNING", Claim: "no covering assertion", ProofRefs: []string{"inspection found no covering assertion"}},
		{ID: "R3-003", Lens: LensReliability, Location: "internal/b.go:5", Severity: "CRITICAL", Claim: "pre-existing injection", ProofRefs: []string{"base and candidate share the behavior"}, EvidenceClass: EvidenceDeterministic, CausalDisposition: CausalPreExisting},
	})
	want := []AdvisoryFinding{
		{ID: "R3-001", Lens: "reliability", Location: "internal/a.go:3", Severity: "WARNING", Disposition: AdvisoryInformational},
		{ID: "R3-003", Lens: "reliability", Location: "internal/b.go:5", Severity: "CRITICAL", Disposition: AdvisoryFollowUp},
	}
	if got := AdvisoryFindings(state); !reflect.DeepEqual(got, want) {
		t.Fatalf("AdvisoryFindings(approved) = %#v, want %#v", got, want)
	}
}

// TestAdvisoryFindingsUseAdmittedValuesInsteadOfTamperedProjections proves an
// approved advisory payload cannot be redirected by compatibility projections.
func TestAdvisoryFindingsUseAdmittedValuesInsteadOfTamperedProjections(t *testing.T) {
	admitted := Finding{ID: "R3-admitted", Lens: LensReliability, Location: "internal/a.go:3", Severity: "WARNING", Claim: "admitted warning", ProofRefs: []string{"admitted reviewer evidence"}}
	state := admittedAdvisoryState(t, "advisory-tampered-projection", []Finding{admitted})
	// CompactState no longer permits retired projection values. Advisory semantics
	// remain a pure read of the admitted capture.
	want := []AdvisoryFinding{{ID: admitted.ID, Lens: "reliability", Location: admitted.Location, Severity: admitted.Severity, Disposition: AdvisoryInformational}}
	if got := AdvisoryFindings(state); !reflect.DeepEqual(got, want) {
		t.Fatalf("AdvisoryFindings(tampered projections) = %#v, want admitted values %#v", got, want)
	}
}

// TestAdvisoryFindingsRequireAdmittedEvidence keeps the projection descriptive:
// persisted compatibility fields alone never announce an advisory outcome.
func TestAdvisoryFindingsRequireAdmittedEvidence(t *testing.T) {
	if got := AdvisoryFindings(CompactState{State: StateApproved}); len(got) != 0 {
		t.Fatalf("AdvisoryFindings(without admitted evidence) = %#v, want none", got)
	}

	reviewing := admittedAdvisoryState(t, "advisory-reviewing", []Finding{{ID: "R3-001", Lens: LensReliability, Location: "internal/a.go:3", Severity: "WARNING", Claim: "advisory only", ProofRefs: []string{"admitted reviewer evidence"}}})
	reviewing.State = StateReviewing
	if got := AdvisoryFindings(reviewing); len(got) != 0 {
		t.Fatalf("AdvisoryFindings(reviewing) = %#v, want none: only terminal approval settles disposition", got)
	}
}

// TestAdvisoryStatementSaysApprovalStands makes the sentence itself testable:
// a consumer must not have to infer "nothing to do" from a severity string.
func TestAdvisoryStatementSaysApprovalStands(t *testing.T) {
	for _, want := range []string{"approved", "non-blocking", "no correction"} {
		if !containsFold(AdvisoryStatement, want) {
			t.Fatalf("AdvisoryStatement %q must state %q", AdvisoryStatement, want)
		}
	}
}

func containsFold(haystack, needle string) bool {
	return len(needle) == 0 || indexFold(haystack, needle) >= 0
}

func indexFold(haystack, needle string) int {
	lower := func(value string) string {
		out := []rune(value)
		for index, char := range out {
			if char >= 'A' && char <= 'Z' {
				out[index] = char + ('a' - 'A')
			}
		}
		return string(out)
	}
	lowerHaystack, lowerNeedle := lower(haystack), lower(needle)
	for index := 0; index+len(lowerNeedle) <= len(lowerHaystack); index++ {
		if lowerHaystack[index:index+len(lowerNeedle)] == lowerNeedle {
			return index
		}
	}
	return -1
}
