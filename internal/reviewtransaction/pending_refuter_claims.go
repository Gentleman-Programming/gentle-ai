package reviewtransaction

// PendingRefuterClaims reports the inferential severe candidate-causal
// findings that compact finalization cannot resolve without a refuter result.
func PendingRefuterClaims(state CompactState, input CompactReviewInput) []RefuterClaim {
	if len(input.LensResults) != len(state.SelectedLenses) {
		return nil
	}
	classifications := make(map[string]FindingEvidence, len(input.Classifications))
	for _, item := range input.Classifications {
		classifications[item.FindingID] = item
	}
	resolved := make(map[string]bool, len(input.RefuterOutcomes))
	for _, result := range input.RefuterOutcomes {
		resolved[result.FindingID] = true
	}
	claims := []RefuterClaim{}
	for index, result := range input.LensResults {
		result.Lens = state.SelectedLenses[index]
		canonical, err := CanonicalCompactLensResult(result)
		if err != nil {
			return nil
		}
		for _, finding := range canonical.Findings {
			item, ok := classifications[finding.ID]
			if !ok || resolved[finding.ID] || !isSevereSeverity(finding.Severity) || item.Class != EvidenceInferential {
				continue
			}
			switch effectiveCompactCausality(finding, item.Causality, state.GenesisPaths) {
			case CausalIntroduced, CausalBehaviorActivated, CausalWorsened:
				claims = append(claims, RefuterClaim{
					FindingID: finding.ID, SnapshotIdentity: state.InitialSnapshot.Identity,
					Claim: finding.Claim, Location: finding.Location, Proof: item.Proof,
				})
			}
		}
	}
	return claims
}

func effectiveCompactCausality(finding Finding, declared CausalDisposition, genesisPaths []string) CausalDisposition {
	switch declared {
	case CausalIntroduced, CausalBehaviorActivated, CausalWorsened:
		if !findingLocationInGenesis(finding.Location, genesisPaths) {
			return CausalUnknown
		}
	}
	return declared
}
