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
//   - CausalPreExisting, CausalBaseOnly trace to a KNOWN cause outside the
//     frozen candidate (the base tree, before the candidate existed) — a
//     follow-up, never a blocker.
//   - CausalUnknown (and the zero value, an unset disposition) has no
//     confirmed candidate cause at all — unlike pre-existing/base-only,
//     which are a KNOWN non-candidate cause, "unknown" means the cause was
//     never determined. W-9 (Wave 7 S1, design decision 3c): this is its own
//     unresolvedIDs bucket, not silently folded into followUpIDs — only v3
//     finalize (review_facade_finalize_new_lineage.go) consumes it, and
//     escalates rather than approves, matching v2's own transaction.go
//     validate logic ("unknown-causality severe finding must terminally
//     escalate as inconclusive").
//
// followUpIDs and unresolvedIDs are returned for caller visibility only —
// AuthorityStore's NewLineageAuthority schema persists no follow-up or
// unresolved field, so a finding's mere absence from admittedIDs IS its
// "cannot authorize a correction" property: nothing downstream reads either
// as authority except v3 finalize's own escalation decision on unresolvedIDs.
// This third return is purely additive: every existing 2-bucket caller that
// only read admittedIDs/followUpIDs is otherwise byte-identical, including
// the shared --admission-findings channel (artifact_admission.go), which
// this function never gates on severity (decision 3b) or calls at all.
func AdmitCandidateCausalFindings(findings []FindingEvidence) (admittedIDs []string, followUpIDs []string, unresolvedIDs []string) {
	for _, finding := range findings {
		switch finding.Causality {
		case CausalIntroduced, CausalBehaviorActivated, CausalWorsened:
			admittedIDs = append(admittedIDs, finding.FindingID)
		case CausalPreExisting, CausalBaseOnly:
			followUpIDs = append(followUpIDs, finding.FindingID)
		default: // CausalUnknown, ""
			unresolvedIDs = append(unresolvedIDs, finding.FindingID)
		}
	}
	return admittedIDs, followUpIDs, unresolvedIDs
}

// SevereFindingEvidence filters findings to only those the shared
// isSevereSeverity vocabulary (BLOCKER/CRITICAL) treats as severe — W-11's
// filter (Wave 7 S1, design decision 3b): v3 finalize applies this to the
// captured-results channel's CapturedFindingEvidence() BEFORE calling
// AdmitCandidateCausalFindings, so a WARNING-severity candidate-causal
// finding stays non-blocking (matching v2's artifact_admission.go, which
// only requires evidence_class/causal_disposition — and therefore only ever
// considers admitting — severe findings in the first place). This filter is
// applied by the CALLER, never inside AdmitCandidateCausalFindings itself:
// that function is also the shared --admission-findings channel's admission
// decision, whose FindingEvidence carries no Severity at all (zero value,
// i.e. never severe) — gating inside AdmitCandidateCausalFindings would
// therefore fail that channel open completely (decision 3b's rejected
// alternative).
func SevereFindingEvidence(findings []FindingEvidence) []FindingEvidence {
	var severe []FindingEvidence
	for _, finding := range findings {
		if isSevereSeverity(finding.Severity) {
			severe = append(severe, finding)
		}
	}
	return severe
}
