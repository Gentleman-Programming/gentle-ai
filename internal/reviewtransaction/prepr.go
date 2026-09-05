package reviewtransaction

import "strings"

// BaseAdvanceCompatibility is derived gate evidence. It never mutates or
// extends the review receipt.
type BaseAdvanceCompatibility struct {
	Status                    string `json:"status"`
	Compatible                bool   `json:"compatible"`
	OriginalMergeBaseTree     string `json:"old_base_tree"`
	NewBaseTree               string `json:"new_base_tree"`
	OriginalPatchIdentity     string `json:"original_patch_identity"`
	DeliveredPatchIdentity    string `json:"delivered_patch_identity"`
	DeliveredPathsDigest      string `json:"delivered_paths_digest"`
	BaseAdvancePathsDigest    string `json:"base_advance_paths_digest"`
	PathsDisjoint             bool   `json:"paths_disjoint"`
	MergedResultTree          string `json:"merged_result_tree"`
	CIAttestationArtifactHash string `json:"ci_attestation_artifact_hash"`
	CIAttestationIssuer       string `json:"ci_attestation_issuer"`
	CIStatus                  string `json:"ci_status"`
}

const (
	baseAdvanceCompatibleStatus = "base-advanced-compatible"
	// baseAdvanceCompatibleLocalStatus records native content proof without CI attestation.
	baseAdvanceCompatibleLocalStatus       = "base-advanced-compatible-local"
	currentChangesBoundaryCompatibleStatus = "current-changes-boundary-compatible"
	currentChangesBoundaryCIStatus         = "not-required"
)

func (proof BaseAdvanceCompatibility) valid() bool {
	core := proof.Compatible && validGitTree(proof.OriginalMergeBaseTree) && validGitTree(proof.NewBaseTree) &&
		validSHA256(proof.OriginalPatchIdentity) && proof.OriginalPatchIdentity == proof.DeliveredPatchIdentity &&
		validSHA256(proof.DeliveredPathsDigest) && validSHA256(proof.BaseAdvancePathsDigest) && proof.PathsDisjoint &&
		validGitTree(proof.MergedResultTree)
	switch proof.Status {
	case baseAdvanceCompatibleStatus:
		return core && validSHA256(proof.CIAttestationArtifactHash) &&
			strings.TrimSpace(proof.CIAttestationIssuer) != "" && proof.CIStatus == "success"
	case baseAdvanceCompatibleLocalStatus, currentChangesBoundaryCompatibleStatus:
		return core && proof.CIAttestationArtifactHash == "" && proof.CIAttestationIssuer == "" &&
			proof.CIStatus == currentChangesBoundaryCIStatus
	default:
		return false
	}
}
