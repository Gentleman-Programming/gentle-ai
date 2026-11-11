package reviewtransaction

import (
	"reflect"
	"strings"
	"testing"
)

// nonBlockingFixtureFindings admits one finding of every routing a stopped
// review can hold: informational, follow-up, a finding to correct, and an
// unresolved severe finding.
func nonBlockingFixtureFindings() []Finding {
	return []Finding{
		{ID: "R3-001", Lens: LensReliability, Location: "internal/a.go:3", Severity: "WARNING", Claim: "no covering assertion", ProofRefs: []string{"inspection found no covering assertion"}},
		{ID: "R3-002", Lens: LensReliability, Location: "internal/a.go:7", Severity: "CRITICAL", Claim: "introduced nil dereference", ProofRefs: []string{"the changed line dereferences a nil session"}, EvidenceClass: EvidenceDeterministic, CausalDisposition: CausalIntroduced},
		{ID: "R3-003", Lens: LensReliability, Location: "internal/b.go:5", Severity: "CRITICAL", Claim: "pre-existing injection", ProofRefs: []string{"base and candidate share the behavior"}, EvidenceClass: EvidenceDeterministic, CausalDisposition: CausalPreExisting},
		{ID: "R3-004", Lens: LensReliability, Location: "internal/b.go:9", Severity: "CRITICAL", Claim: "possible race", ProofRefs: []string{"the lock order could not be established"}, EvidenceClass: EvidenceInsufficient, CausalDisposition: CausalUnknown},
	}
}

func TestNonBlockingFindingsOfStoppedReviewCarryClaimsAndNeverAFindingToCorrect(t *testing.T) {
	want := []NonBlockingFinding{
		{ID: "R3-001", Lens: "reliability", Location: "internal/a.go:3", Severity: "WARNING", Disposition: AdvisoryInformational, Claim: "no covering assertion"},
		{ID: "R3-003", Lens: "reliability", Location: "internal/b.go:5", Severity: "CRITICAL", Disposition: AdvisoryFollowUp, Claim: "pre-existing injection"},
	}
	for _, state := range []State{StateCorrectionRequired, StateEscalated} {
		admitted := admittedAdvisoryState(t, "non-blocking-"+strings.ReplaceAll(string(state), "_", "-"), nonBlockingFixtureFindings())
		admitted.State = state
		view, err := admitted.CompactReviewView()
		if err != nil {
			t.Fatal(err)
		}
		if view.Outcomes["R3-002"] != OutcomeInconclusive || view.Outcomes["R3-004"] != OutcomeInconclusive {
			t.Fatalf("fixture view outcomes = %v, want R3-002 and R3-004 unresolved", view.Outcomes)
		}
		got := NonBlockingFindingSetFor(admitted)
		if got == nil || got.Statement != NonBlockingStatement || !reflect.DeepEqual(got.Findings, want) {
			t.Fatalf("NonBlockingFindingSetFor(%s) = %#v, want %#v", state, got, want)
		}
	}
}

// The selection never lists a finding to correct, a corroborated or
// inconclusive outcome, or an outcome it does not recognize.
func TestNonBlockingSelectionExcludesFindingsToCorrectAndUnknownOutcomes(t *testing.T) {
	view := CompactReviewView{
		Findings: []Finding{
			{ID: "F-fix-info", Severity: "WARNING", Claim: "listed for correction"},
			{ID: "F-corroborated", Severity: "CRITICAL", Claim: "corroborated"},
			{ID: "F-inconclusive", Severity: "CRITICAL", Claim: "inconclusive"},
			{ID: "F-unknown", Severity: "CRITICAL", Claim: "unrecognized outcome"},
			{ID: "F-refuted", Lens: "risk", Location: "a.go:1", Severity: "CRITICAL", Claim: "knocked down"},
			{ID: "F-info", Severity: "SUGGESTION", Claim: "style"},
		},
		Outcomes: map[string]EvidenceOutcome{
			"F-fix-info": OutcomeInfo, "F-corroborated": OutcomeCorroborated, "F-inconclusive": OutcomeInconclusive,
			"F-unknown": EvidenceOutcome("future"), "F-refuted": OutcomeRefuted, "F-info": OutcomeInfo,
		},
		FixFindingIDs: []string{"F-fix-info"},
	}
	want := []NonBlockingFinding{
		{ID: "F-info", Severity: "SUGGESTION", Disposition: AdvisoryInformational, Claim: "style"},
		{ID: "F-refuted", Lens: "risk", Location: "a.go:1", Severity: "CRITICAL", Disposition: AdvisoryRefuted, Claim: "knocked down"},
	}
	if got := nonBlockingFindingsOf(view); !reflect.DeepEqual(got, want) {
		t.Fatalf("nonBlockingFindingsOf = %#v, want %#v", got, want)
	}
}

func TestNonBlockingFindingsAreEmptyNotAbsentWhenEveryFindingBlocks(t *testing.T) {
	admitted := admittedAdvisoryState(t, "non-blocking-empty", nonBlockingFixtureFindings()[1:2])
	admitted.State = StateCorrectionRequired
	got := NonBlockingFindingSetFor(admitted)
	if got == nil || got.Findings == nil || len(got.Findings) != 0 {
		t.Fatalf("NonBlockingFindingSetFor(only a finding to correct) = %#v, want an explicit empty set", got)
	}
}

func TestNonBlockingFindingsAreOnlyForStoppedReviews(t *testing.T) {
	for _, state := range []State{StateApproved, StateReviewing, StateValidating} {
		admitted := admittedAdvisoryState(t, "non-blocking-other-"+string(state), nonBlockingFixtureFindings())
		admitted.State = state
		if got := NonBlockingFindingSetFor(admitted); got != nil {
			t.Fatalf("NonBlockingFindingSetFor(%s) = %#v, want nil", state, got)
		}
	}
}

func TestNonBlockingStatementNeverSaysApproved(t *testing.T) {
	if containsFold(NonBlockingStatement, "approved") || !containsFold(NonBlockingStatement, "non-blocking") {
		t.Fatalf("NonBlockingStatement %q must say non-blocking and never approved", NonBlockingStatement)
	}
}

// PRESERVE: the approved advisory projection shares the selection and keeps
// excluding the finding to correct and the unresolved one, without a claim.
func TestAdvisoryFindingsUnchangedBySharedSelection(t *testing.T) {
	admitted := admittedAdvisoryState(t, "advisory-shared-selection", nonBlockingFixtureFindings())
	want := []AdvisoryFinding{
		{ID: "R3-001", Lens: "reliability", Location: "internal/a.go:3", Severity: "WARNING", Disposition: AdvisoryInformational},
		{ID: "R3-003", Lens: "reliability", Location: "internal/b.go:5", Severity: "CRITICAL", Disposition: AdvisoryFollowUp},
	}
	if got := AdvisoryFindings(admitted); !reflect.DeepEqual(got, want) {
		t.Fatalf("AdvisoryFindings(approved) = %#v, want %#v", got, want)
	}
}
