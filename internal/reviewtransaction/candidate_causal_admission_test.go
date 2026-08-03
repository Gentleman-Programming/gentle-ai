package reviewtransaction

import "testing"

// TestAdmitCandidateCausalFindingsBlocksOnlyCandidateCaused is the direct
// unit-level RED test for spec rdd-review-core-transitions,
// "Candidate-Causal Admission Only" (task C2) — both scenarios in one
// table-driven assertion, so a mutation collapsing the switch to "admit
// everything" or "admit nothing" fails at least one row:
//
//   - "Candidate-caused finding blocks": a finding whose evidence traces
//     only to paths changed by the frozen candidate (introduced,
//     behavior-activated, worsened) is admitted.
//   - "Pre-existing finding becomes a follow-up": a finding present in the
//     base tree before the candidate existed (pre-existing, base-only) —
//     and the unresolved unknown disposition, which by definition traces to
//     no confirmed candidate cause — becomes a follow-up, never admitted.
func TestAdmitCandidateCausalFindingsBlocksOnlyCandidateCaused(t *testing.T) {
	findings := []FindingEvidence{
		{FindingID: "introduced-finding", Causality: CausalIntroduced},
		{FindingID: "behavior-activated-finding", Causality: CausalBehaviorActivated},
		{FindingID: "worsened-finding", Causality: CausalWorsened},
		{FindingID: "pre-existing-finding", Causality: CausalPreExisting},
		{FindingID: "base-only-finding", Causality: CausalBaseOnly},
		{FindingID: "unknown-cause-finding", Causality: CausalUnknown},
	}
	admitted, followUps := AdmitCandidateCausalFindings(findings)
	wantAdmitted := []string{"introduced-finding", "behavior-activated-finding", "worsened-finding"}
	wantFollowUps := []string{"pre-existing-finding", "base-only-finding", "unknown-cause-finding"}
	if !equalStrings(admitted, wantAdmitted) {
		t.Fatalf("admitted = %v, want %v", admitted, wantAdmitted)
	}
	if !equalStrings(followUps, wantFollowUps) {
		t.Fatalf("followUps = %v, want %v", followUps, wantFollowUps)
	}
	for _, id := range wantFollowUps {
		if stringIndex(admitted, id) >= 0 {
			t.Fatalf("follow-up finding %q must never appear in admittedIDs (cannot authorize a correction)", id)
		}
	}
}

// TestAdmitCandidateCausalFindingsEmptyInputAdmitsNothing proves the
// no-findings case (the tier-0 minimal finalize path, task C1) never
// fabricates a blocker.
func TestAdmitCandidateCausalFindingsEmptyInputAdmitsNothing(t *testing.T) {
	admitted, followUps := AdmitCandidateCausalFindings(nil)
	if len(admitted) != 0 || len(followUps) != 0 {
		t.Fatalf("AdmitCandidateCausalFindings(nil) = (%v, %v), want (nil, nil)", admitted, followUps)
	}
}
