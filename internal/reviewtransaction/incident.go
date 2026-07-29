package reviewtransaction

// IncidentTarget freezes every delivery boundary that operational recovery is
// allowed to observe but not change. A target change belongs to a new review
// lineage rather than a separate incident transaction.
type IncidentTarget struct {
	CodeTree               string `json:"code_tree"`
	ConfigHash             string `json:"config_hash"`
	GeneratedArtifactsHash string `json:"generated_artifacts_hash"`
	ProvenanceHash         string `json:"provenance_hash"`
}

func CanUseSeparateIncident(receipt Receipt, frozen, current IncidentTarget) bool {
	if validateReceiptStructure(receipt) != nil || receipt.TerminalState != TerminalApproved {
		return false
	}
	if !validIncidentTarget(frozen) || !validIncidentTarget(current) {
		return false
	}
	return frozen == current && frozen.CodeTree == receipt.FinalCandidateTree
}

func validIncidentTarget(target IncidentTarget) bool {
	return validGitTree(target.CodeTree) && validSHA256(target.ConfigHash) &&
		validSHA256(target.GeneratedArtifactsHash) && validSHA256(target.ProvenanceHash)
}
