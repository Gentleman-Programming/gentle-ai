package reviewtransaction

import (
	"reflect"
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

func TestNonBlockingStatementNeverSaysApproved(t *testing.T) {
	if containsFold(NonBlockingStatement, "approved") || !containsFold(NonBlockingStatement, "non-blocking") || !containsFold(NonBlockingStatement, "refuted") {
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
