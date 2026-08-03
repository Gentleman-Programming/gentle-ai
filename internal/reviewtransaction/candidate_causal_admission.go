// Package reviewtransaction — candidate-causal finding admission (Wave 3
// remediation, task C2). Spec rdd-review-core-transitions, "Candidate-Causal
// Admission Only": only a finding caused by the frozen candidate may block;
// a pre-existing or base-only finding must become a follow-up, never a
// blocker. This file owns exactly the pure classification decision;
// internal/cli's new-lineage finalize routing (task C1) is the production
// caller that persists AdmitCandidateCausalFindings' admitted IDs into
// NewLineageAuthority.AdmittedFindingIDs (review-state.json) via
// AuthorityStore.Mutate — S5's own recorded decision 6.3: admitted-finding
// references belong in review-state.json, not the receipt.
package reviewtransaction

// AdmitCandidateCausalFindings classifies findings by their already-computed
// CausalDisposition (spec rdd-review-core-transitions, "Candidate-Causal
// Admission Only"):
//
//   - CausalIntroduced, CausalBehaviorActivated, CausalWorsened trace to
//     paths changed by the frozen candidate — admitted, and only these ever
//     populate admittedIDs.
//   - CausalPreExisting, CausalBaseOnly trace to the base tree before the
//     candidate existed — a follow-up, never a blocker.
//   - CausalUnknown (and the zero value, an unset disposition) has no
//     confirmed candidate cause to admit on; it defaults to the same
//     follow-up routing as pre-existing/base-only rather than silently
//     admitting an unproven cause.
//
// followUpIDs is returned for caller visibility only — AuthorityStore's
// NewLineageAuthority schema persists no follow-up field, so a finding's
// mere absence from admittedIDs IS its "cannot authorize a correction"
// property: nothing downstream ever reads followUpIDs as authority.
func AdmitCandidateCausalFindings(findings []FindingEvidence) (admittedIDs []string, followUpIDs []string) {
	for _, finding := range findings {
		switch finding.Causality {
		case CausalIntroduced, CausalBehaviorActivated, CausalWorsened:
			admittedIDs = append(admittedIDs, finding.FindingID)
		default: // CausalPreExisting, CausalBaseOnly, CausalUnknown, ""
			followUpIDs = append(followUpIDs, finding.FindingID)
		}
	}
	return admittedIDs, followUpIDs
}
