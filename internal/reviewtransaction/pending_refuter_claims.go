// Package reviewtransaction — pending refuter claims for the compact review
// path. Compact finalize refuses an inferential candidate-causal severe
// finding that carries no refuter outcome ("inferential finding %q requires
// one refuter outcome"). Until this file existed, nothing could answer that
// refusal in advance: the negotiated router had no way to know a captured
// result would demand a refuter, so it offered finalize, finalize refused,
// and the next status offered the same finalize forever (issue #2823).
//
// PendingRefuterClaims answers exactly that question, and nothing else. It
// mutates no state, reads no repository, and applies the same admission rules
// finalize itself applies — it is the question finalize asks, asked one step
// earlier.
package reviewtransaction

// effectiveCausality applies the one repository-derived correction compact
// finalize makes to a reviewer's self-declared causal disposition: a
// candidate-causal claim whose location is not a genesis path was never
// proven to be caused by this candidate, so it degrades to unknown. Both
// finalize's own apply loop and PendingRefuterClaims read the disposition
// through here, so the two can never disagree about which findings are
// candidate-causal.
func effectiveCausality(finding Finding, declared CausalDisposition, genesisPaths []string) CausalDisposition {
	switch declared {
	case CausalIntroduced, CausalBehaviorActivated, CausalWorsened:
		if !findingLocationInGenesis(finding.Location, genesisPaths) {
			return CausalUnknown
		}
	}
	return declared
}

// PendingRefuterClaims reports the severe candidate-causal findings in a
// prepared compact review input whose evidence class is inferential and whose
// refuter outcome has not been supplied yet. An empty result means finalize
// needs no refuter and may be offered directly.
//
// Findings are read through CanonicalCompactLensResult exactly as finalize
// reads them, because a native-assigned finding ID does not exist until that
// canonicalization runs — claiming an un-canonicalized ID here would name a
// finding the caller could never match on submission.
func PendingRefuterClaims(state CompactState, input CompactReviewInput) []RefuterClaim {
	if len(input.LensResults) != len(state.SelectedLenses) {
		return nil
	}
	resolved := make(map[string]bool, len(input.RefuterOutcomes))
	for _, outcome := range input.RefuterOutcomes {
		resolved[outcome.FindingID] = true
	}
	classifications := make(map[string]FindingEvidence, len(input.Classifications))
	for _, item := range input.Classifications {
		classifications[item.FindingID] = item
	}

	claims := []RefuterClaim{}
	for index, result := range input.LensResults {
		result.Lens = state.SelectedLenses[index]
		canonical, err := CanonicalCompactLensResult(result)
		if err != nil {
			// A result finalize itself will reject is not this function's to
			// diagnose: reporting no pending claim lets finalize produce its
			// own exact refusal rather than a second, weaker one here.
			return nil
		}
		for _, finding := range canonical.Findings {
			if !isSevereSeverity(finding.Severity) || resolved[finding.ID] {
				continue
			}
			item, classified := classifications[finding.ID]
			if !classified || item.Class != EvidenceInferential {
				continue
			}
			switch effectiveCausality(finding, item.Causality, state.GenesisPaths) {
			case CausalIntroduced, CausalBehaviorActivated, CausalWorsened:
				claims = append(claims, RefuterClaim{
					FindingID: finding.ID, SnapshotIdentity: state.CurrentSnapshot.Identity, Proof: item.Proof,
				})
			}
		}
	}
	return claims
}
