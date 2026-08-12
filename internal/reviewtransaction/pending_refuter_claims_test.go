package reviewtransaction

import "testing"

func TestPendingRefuterClaimsIncludesFindingClaimAndLocation(t *testing.T) {
	finding := Finding{
		ID: "R3-001", Lens: "reliability", Location: "tracked.txt:1", Severity: "CRITICAL",
		Claim: "the candidate depends on an external contract", ProofRefs: []string{"tracked.txt:1"},
	}
	claims := PendingRefuterClaims(CompactState{
		SelectedLenses:  []string{LensReliability},
		GenesisPaths:    []string{"tracked.txt"},
		InitialSnapshot: Snapshot{Identity: "sha256:snapshot"},
	}, CompactReviewInput{
		LensResults:     []LensResult{{Lens: LensReliability, Findings: []Finding{finding}, Evidence: []string{"reviewed exact candidate"}}},
		Classifications: []FindingEvidence{{FindingID: finding.ID, Class: EvidenceInferential, Causality: CausalIntroduced, Proof: "candidate trace requires interpretation"}},
	})

	if len(claims) != 1 {
		t.Fatalf("PendingRefuterClaims() = %#v, want one claim", claims)
	}
	if claims[0].Claim != finding.Claim || claims[0].Location != finding.Location {
		t.Fatalf("PendingRefuterClaims()[0] = %#v, want claim %q at %q", claims[0], finding.Claim, finding.Location)
	}
}
