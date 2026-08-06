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
	}
	admitted, followUps, unresolved := AdmitCandidateCausalFindings(findings)
	wantAdmitted := []string{"introduced-finding", "behavior-activated-finding", "worsened-finding"}
	wantFollowUps := []string{"pre-existing-finding", "base-only-finding"}
	if !equalStrings(admitted, wantAdmitted) {
		t.Fatalf("admitted = %v, want %v", admitted, wantAdmitted)
	}
	if !equalStrings(followUps, wantFollowUps) {
		t.Fatalf("followUps = %v, want %v", followUps, wantFollowUps)
	}
	if len(unresolved) != 0 {
		t.Fatalf("unresolved = %v, want none (no unknown/unset-disposition finding in this table)", unresolved)
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
	admitted, followUps, unresolved := AdmitCandidateCausalFindings(nil)
	if len(admitted) != 0 || len(followUps) != 0 || len(unresolved) != 0 {
		t.Fatalf("AdmitCandidateCausalFindings(nil) = (%v, %v, %v), want (nil, nil, nil)", admitted, followUps, unresolved)
	}
}

// TestAdmitCandidateCausalFindingsUnknownDispositionIsUnresolvedNotFollowUp
// is W-9's direct unit proof (Wave 7 S1, design decision 3c): a finding
// whose causal_disposition is CausalUnknown, or unset entirely (the zero
// value), traces to no confirmed candidate cause -- unlike pre-existing/
// base-only, which trace to a KNOWN non-candidate cause -- so it belongs in
// its own unresolvedIDs bucket, not silently folded into followUpIDs where
// v3's finalize (review_facade_finalize_new_lineage.go) previously had no
// way to distinguish "known non-candidate" from "never determined" and so
// treated both as equally safe to let approve. Only v3 finalize consumes
// this bucket (to escalate); every existing 2-bucket caller stays additive.
func TestAdmitCandidateCausalFindingsUnknownDispositionIsUnresolvedNotFollowUp(t *testing.T) {
	findings := []FindingEvidence{
		{FindingID: "explicit-unknown", Causality: CausalUnknown},
		{FindingID: "unset-disposition"},
	}
	admitted, followUps, unresolved := AdmitCandidateCausalFindings(findings)
	if len(admitted) != 0 {
		t.Fatalf("admitted = %v, want none: an unresolved cause must never authorize a correction", admitted)
	}
	if len(followUps) != 0 {
		t.Fatalf("followUps = %v, want none: unknown/unset disposition is unresolved, not a known non-candidate follow-up", followUps)
	}
	wantUnresolved := []string{"explicit-unknown", "unset-disposition"}
	if !equalStrings(unresolved, wantUnresolved) {
		t.Fatalf("unresolved = %v, want %v", unresolved, wantUnresolved)
	}
}

// TestSevereFindingEvidenceFiltersOutNonSevereFindings is W-11's direct unit
// proof (design decision 3b): SevereFindingEvidence -- the filter v3
// finalize applies to the captured-results channel BEFORE
// AdmitCandidateCausalFindings -- keeps only BLOCKER/CRITICAL findings,
// dropping WARNING even when it carries a candidate-causal disposition that
// would otherwise admit it. AdmitCandidateCausalFindings itself stays
// untouched (byte-identical) -- the filter runs strictly before it, never
// inside it, so the shared --admission-findings channel (whose
// FindingEvidence carries no severity at all) is never affected.
func TestSevereFindingEvidenceFiltersOutNonSevereFindings(t *testing.T) {
	findings := []FindingEvidence{
		{FindingID: "blocker-introduced", Severity: "BLOCKER", Causality: CausalIntroduced},
		{FindingID: "critical-worsened", Severity: "CRITICAL", Causality: CausalWorsened},
		{FindingID: "warning-introduced", Severity: "WARNING", Causality: CausalIntroduced},
		{FindingID: "suggestion-introduced", Severity: "SUGGESTION", Causality: CausalIntroduced},
		{FindingID: "unset-severity-introduced", Causality: CausalIntroduced},
	}
	severe := SevereFindingEvidence(findings)
	wantIDs := []string{"blocker-introduced", "critical-worsened"}
	var gotIDs []string
	for _, finding := range severe {
		gotIDs = append(gotIDs, finding.FindingID)
	}
	if !equalStrings(gotIDs, wantIDs) {
		t.Fatalf("SevereFindingEvidence(findings) ids = %v, want %v", gotIDs, wantIDs)
	}

	admitted, _, _ := AdmitCandidateCausalFindings(severe)
	if !equalStrings(admitted, wantIDs) {
		t.Fatalf("AdmitCandidateCausalFindings(SevereFindingEvidence(findings)) admitted = %v, want %v -- a WARNING candidate-causal finding must never block", admitted, wantIDs)
	}

	admittedUnfiltered, _, _ := AdmitCandidateCausalFindings(findings)
	if len(admittedUnfiltered) != len(findings) {
		t.Fatalf("sanity check failed: AdmitCandidateCausalFindings ignores severity by design (that gate would fail-open the --admission-findings channel, decision 3b); unfiltered admitted = %v", admittedUnfiltered)
	}
}
