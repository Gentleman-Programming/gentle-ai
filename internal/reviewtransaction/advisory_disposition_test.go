package reviewtransaction

import (
	"reflect"
	"testing"
)

// TestAdvisoryFindingsDeclareNonBlockingRouting pins the disposition every
// non-blocking frozen finding already carries internally (Outcomes[id] and its
// absence from FixFindingIDs) as an explicit, named value. The field incident
// this covers: a consumer read a WARNING off an approved receipt, inferred
// from the severity string alone that it had to act, and burned a review cycle
// re-running an already-approved candidate.
func TestAdvisoryFindingsDeclareNonBlockingRouting(t *testing.T) {
	state := CompactState{
		State: StateApproved,
		Findings: []Finding{
			{ID: "R3-001", Lens: "reliability", Location: "feature.go:3", Severity: "WARNING", Claim: "no covering assertion"},
			{ID: "R3-002", Lens: "reliability", Location: "feature.go:5", Severity: "SUGGESTION", Claim: "could reuse the helper"},
			{ID: "R1-003", Lens: "risk", Location: "legacy.go:9", Severity: "CRITICAL", Claim: "pre-existing injection"},
			{ID: "R1-004", Lens: "risk", Location: "feature.go:7", Severity: "BLOCKER", Claim: "inferential claim the refuter knocked down"},
		},
		Outcomes: map[string]EvidenceOutcome{
			"R3-001": OutcomeInfo,
			"R3-002": OutcomeInfo,
			"R1-003": OutcomeInfo,
			"R1-004": OutcomeRefuted,
		},
		FixFindingIDs: []string{},
	}
	want := []AdvisoryFinding{
		{ID: "R1-003", Lens: "risk", Location: "legacy.go:9", Severity: "CRITICAL", Disposition: AdvisoryFollowUp},
		{ID: "R1-004", Lens: "risk", Location: "feature.go:7", Severity: "BLOCKER", Disposition: AdvisoryRefuted},
		{ID: "R3-001", Lens: "reliability", Location: "feature.go:3", Severity: "WARNING", Disposition: AdvisoryInformational},
		{ID: "R3-002", Lens: "reliability", Location: "feature.go:5", Severity: "SUGGESTION", Disposition: AdvisoryInformational},
	}
	got := AdvisoryFindings(state)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("AdvisoryFindings(approved) = %#v, want %#v", got, want)
	}
}

// TestAdvisoryFindingsExcludeCorrectedAndUnapprovedLineages keeps the new
// projection strictly descriptive: it never claims a corrected (blocking)
// finding was advisory, and it says nothing at all about a lineage that is not
// terminally approved.
func TestAdvisoryFindingsExcludeCorrectedAndUnapprovedLineages(t *testing.T) {
	corrected := CompactState{
		State: StateApproved,
		Findings: []Finding{
			{ID: "R1-001", Lens: "risk", Location: "feature.go:3", Severity: "BLOCKER", Claim: "candidate-caused"},
			{ID: "R3-002", Lens: "reliability", Location: "feature.go:5", Severity: "WARNING", Claim: "advisory only"},
		},
		Outcomes:      map[string]EvidenceOutcome{"R1-001": OutcomeCorroborated, "R3-002": OutcomeInfo},
		FixFindingIDs: []string{"R1-001"},
	}
	got := AdvisoryFindings(corrected)
	if len(got) != 1 || got[0].ID != "R3-002" || got[0].Disposition != AdvisoryInformational {
		t.Fatalf("AdvisoryFindings(corrected approved) = %#v, want only the informational finding", got)
	}

	reviewing := corrected
	reviewing.State = StateReviewing
	if got := AdvisoryFindings(reviewing); len(got) != 0 {
		t.Fatalf("AdvisoryFindings(reviewing) = %#v, want none: only a terminal approval settles disposition", got)
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
