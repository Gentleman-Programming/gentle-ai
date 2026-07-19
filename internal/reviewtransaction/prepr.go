package reviewtransaction

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"
)

const prePRCIAttestationSchema = "gentle-ai.pre-pr-ci-attestation/v1"

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

type prePRCIAttestation struct {
	Schema     string `json:"schema"`
	Issuer     string `json:"issuer"`
	MergedTree string `json:"merged_tree"`
	Status     string `json:"status"`
	Signature  string `json:"signature"`
}

type prePRCITrust struct {
	Issuer           string `json:"issuer"`
	Ed25519PublicKey string `json:"ed25519_public_key"`
}

const (
	baseAdvanceCompatibleStatus            = "base-advanced-compatible"
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
	case currentChangesBoundaryCompatibleStatus:
		return core && proof.CIAttestationArtifactHash == "" && proof.CIAttestationIssuer == "" &&
			proof.CIStatus == currentChangesBoundaryCIStatus
	default:
		return false
	}
}

func deriveBaseAdvanceCompatibility(ctx context.Context, repo string, receipt Receipt, request GateRequest, snapshot Snapshot, refs *resolvedPrePRRefs, preimages gateArtifactPreimages) (BaseAdvanceCompatibility, error) {
	if refs == nil {
		return BaseAdvanceCompatibility{}, errors.New("resolved pre-PR refs are missing")
	}
	if request.ExternalEvidence != ExternalEvidenceNone {
		return BaseAdvanceCompatibility{}, errors.New("external evidence invalidates or escalates compatibility")
	}
	if request.PrePR == nil || strings.TrimSpace(request.PrePR.CIAttestationArtifact) == "" {
		return BaseAdvanceCompatibility{}, errors.New("trusted CI attestation is required")
	}
	mergeBase, err := runGit(ctx, repo, nil, nil, "merge-base", refs.Selection.Commit, refs.HeadCommit)
	if err != nil {
		return BaseAdvanceCompatibility{}, fmt.Errorf("derive original merge-base: %w", err)
	}
	mergeBaseTree, err := (SnapshotBuilder{Repo: repo}).resolveTree(ctx, strings.TrimSpace(string(mergeBase)))
	if err != nil || mergeBaseTree != receipt.BaseTree {
		return BaseAdvanceCompatibility{}, errors.New("original reviewed merge-base tree is not preserved")
	}

	builder := SnapshotBuilder{Repo: repo}
	originalPaths, err := builder.changedPaths(ctx, receipt.BaseTree, receipt.FinalCandidateTree)
	if err != nil {
		return BaseAdvanceCompatibility{}, err
	}
	currentPaths, err := builder.changedPaths(ctx, receipt.BaseTree, snapshot.CandidateTree)
	if err != nil {
		return BaseAdvanceCompatibility{}, err
	}
	if digestPaths(originalPaths) != receipt.PathsDigest || digestPaths(currentPaths) != receipt.PathsDigest {
		return BaseAdvanceCompatibility{}, errors.New("delivered path identity changed")
	}
	originalPatch, err := patchIdentity(ctx, repo, receipt.BaseTree, receipt.FinalCandidateTree)
	if err != nil {
		return BaseAdvanceCompatibility{}, err
	}
	currentPatch, err := patchIdentity(ctx, repo, receipt.BaseTree, snapshot.CandidateTree)
	if err != nil || originalPatch != currentPatch {
		return BaseAdvanceCompatibility{}, errors.New("delivered patch identity changed")
	}
	basePaths, err := builder.changedPaths(ctx, receipt.BaseTree, snapshot.BaseTree)
	if err != nil {
		return BaseAdvanceCompatibility{}, err
	}
	if !disjointPaths(originalPaths, basePaths) {
		return BaseAdvanceCompatibility{}, errors.New("base advance overlaps delivered paths")
	}
	mergedOutput, err := runGit(ctx, repo, nil, nil, "merge-tree", "--write-tree", refs.Selection.Commit, refs.HeadCommit)
	if err != nil {
		return BaseAdvanceCompatibility{}, errors.New("merge against new base is not conflict-free")
	}
	mergedFields := strings.Fields(string(mergedOutput))
	if len(mergedFields) == 0 || !validGitTree(mergedFields[0]) {
		return BaseAdvanceCompatibility{}, errors.New("merged result tree cannot be derived")
	}
	attestationHash, issuer, err := verifyPrePRCIAttestation(preimages.policy, preimages.ciAttestation, mergedFields[0])
	if err != nil {
		return BaseAdvanceCompatibility{}, err
	}
	selector := ""
	if refs.Selection.Source == PrePRBoundaryExplicit {
		selector = refs.Selection.Selector
	}
	selectionNow, err := selectPrePRBoundary(ctx, repo, selector)
	if err != nil || selectionNow != refs.Selection {
		return BaseAdvanceCompatibility{}, errors.New("pre-PR base ref advanced during validation")
	}
	headNow, err := resolveCommit(ctx, repo, "HEAD")
	if err != nil || headNow != refs.HeadCommit {
		return BaseAdvanceCompatibility{}, errors.New("HEAD advanced during validation")
	}
	proof := BaseAdvanceCompatibility{
		Status: baseAdvanceCompatibleStatus, Compatible: true, OriginalMergeBaseTree: receipt.BaseTree, NewBaseTree: snapshot.BaseTree,
		OriginalPatchIdentity: originalPatch, DeliveredPatchIdentity: currentPatch,
		DeliveredPathsDigest: receipt.PathsDigest, BaseAdvancePathsDigest: digestPaths(basePaths), PathsDisjoint: true,
		MergedResultTree: mergedFields[0], CIAttestationArtifactHash: attestationHash,
		CIAttestationIssuer: issuer, CIStatus: "success",
	}
	if !proof.valid() {
		return BaseAdvanceCompatibility{}, errors.New("compatible base advance proof is incomplete")
	}
	return proof, nil
}

// deriveCurrentChangesBoundaryCompatibility reconciles an approved
// current-changes receipt with a pre-PR publication boundary that advanced
// past the frozen base (issue #1376). It is derived gate evidence only and
// never mutates the receipt. It allows only when the approved bytes provably
// reach the publication boundary unchanged:
//   - the reviewed genesis scope is non-empty (an empty self-diff review can
//     never authorize a non-empty publication),
//   - the publication head tree is byte-identical to the approved candidate
//     tree,
//   - the frozen review base tree is exactly the publication merge-base tree,
//   - the delivered paths are non-empty, stay inside the immutable genesis
//     scope, and are disjoint from the boundary advance,
//   - every path touched by the publication range stays inside the genesis
//     scope (nothing unreviewed rides along in intermediate commits), and
//   - the merge against the advanced boundary is conflict-free.
func deriveCurrentChangesBoundaryCompatibility(ctx context.Context, repo string, state CompactState, request GateRequest, snapshot Snapshot, refs *resolvedPrePRRefs) (BaseAdvanceCompatibility, error) {
	if refs == nil {
		return BaseAdvanceCompatibility{}, errors.New("resolved pre-PR refs are missing")
	}
	if request.ExternalEvidence != ExternalEvidenceNone {
		return BaseAdvanceCompatibility{}, errors.New("external evidence invalidates or escalates compatibility")
	}
	if state.InitialSnapshot.Kind != TargetCurrentChanges {
		return BaseAdvanceCompatibility{}, errors.New("boundary reconciliation is limited to current-changes receipts")
	}
	if refs.DeliveredCommitCount != 1 {
		return BaseAdvanceCompatibility{}, errors.New("boundary reconciliation requires exactly one delivery commit")
	}
	if len(state.GenesisPaths) == 0 {
		return BaseAdvanceCompatibility{}, errors.New("an empty reviewed scope cannot authorize a publication")
	}
	frozenBaseTree := state.InitialSnapshot.BaseTree
	if snapshot.CandidateTree != state.CurrentSnapshot.CandidateTree {
		return BaseAdvanceCompatibility{}, errors.New("publication head does not match the approved candidate tree")
	}
	mergeBase, err := runGit(ctx, repo, nil, nil, "merge-base", refs.Selection.Commit, refs.HeadCommit)
	if err != nil {
		return BaseAdvanceCompatibility{}, fmt.Errorf("derive publication merge-base: %w", err)
	}
	builder := SnapshotBuilder{Repo: repo}
	mergeBaseTree, err := builder.resolveTree(ctx, strings.TrimSpace(string(mergeBase)))
	if err != nil || mergeBaseTree != frozenBaseTree {
		return BaseAdvanceCompatibility{}, errors.New("approved review base is not the publication merge-base")
	}
	deliveredPaths, err := builder.changedPaths(ctx, frozenBaseTree, snapshot.CandidateTree)
	if err != nil {
		return BaseAdvanceCompatibility{}, err
	}
	if len(deliveredPaths) == 0 || pathsAreSubset(deliveredPaths, state.GenesisPaths) != nil {
		return BaseAdvanceCompatibility{}, errors.New("delivered paths do not stay inside the reviewed genesis scope")
	}
	patch, err := patchIdentity(ctx, repo, frozenBaseTree, snapshot.CandidateTree)
	if err != nil {
		return BaseAdvanceCompatibility{}, err
	}
	basePaths, err := builder.changedPaths(ctx, frozenBaseTree, snapshot.BaseTree)
	if err != nil {
		return BaseAdvanceCompatibility{}, err
	}
	if !disjointPaths(deliveredPaths, basePaths) {
		return BaseAdvanceCompatibility{}, errors.New("boundary advance overlaps delivered paths")
	}
	if err := validateReviewedPublicationRange(ctx, repo, state.GenesisPaths, refs); err != nil {
		return BaseAdvanceCompatibility{}, err
	}
	mergedOutput, err := runGit(ctx, repo, nil, nil, "merge-tree", "--write-tree", refs.Selection.Commit, refs.HeadCommit)
	if err != nil {
		return BaseAdvanceCompatibility{}, errors.New("merge against the advanced boundary is not conflict-free")
	}
	mergedFields := strings.Fields(string(mergedOutput))
	if len(mergedFields) == 0 || !validGitTree(mergedFields[0]) {
		return BaseAdvanceCompatibility{}, errors.New("merged result tree cannot be derived")
	}
	selector := ""
	if refs.Selection.Source == PrePRBoundaryExplicit {
		selector = refs.Selection.Selector
	}
	selectionNow, err := selectPrePRBoundary(ctx, repo, selector)
	if err != nil || selectionNow != refs.Selection {
		return BaseAdvanceCompatibility{}, errors.New("pre-PR base ref advanced during validation")
	}
	headNow, err := resolveCommit(ctx, repo, "HEAD")
	if err != nil || headNow != refs.HeadCommit {
		return BaseAdvanceCompatibility{}, errors.New("HEAD advanced during validation")
	}
	proof := BaseAdvanceCompatibility{
		Status: currentChangesBoundaryCompatibleStatus, Compatible: true,
		OriginalMergeBaseTree: frozenBaseTree, NewBaseTree: snapshot.BaseTree,
		OriginalPatchIdentity: patch, DeliveredPatchIdentity: patch,
		DeliveredPathsDigest: digestPaths(deliveredPaths), BaseAdvancePathsDigest: digestPaths(basePaths), PathsDisjoint: true,
		MergedResultTree: mergedFields[0], CIStatus: currentChangesBoundaryCIStatus,
	}
	if !proof.valid() {
		return BaseAdvanceCompatibility{}, errors.New("current-changes boundary proof is incomplete")
	}
	return proof, nil
}

func patchIdentity(ctx context.Context, repo, baseTree, candidateTree string) (string, error) {
	payload, err := runGit(ctx, repo, nil, nil, "diff", "--binary", "--full-index", "--no-ext-diff", baseTree, candidateTree, "--")
	if err != nil {
		return "", err
	}
	hash := sha256.New()
	hash.Write([]byte("gentle-ai.delivered-patch/v1\x00"))
	hash.Write(payload)
	return "sha256:" + hex.EncodeToString(hash.Sum(nil)), nil
}

func disjointPaths(left, right []string) bool {
	left = append([]string(nil), left...)
	right = append([]string(nil), right...)
	sort.Strings(left)
	sort.Strings(right)
	for i, j := 0, 0; i < len(left) && j < len(right); {
		if left[i] == right[j] {
			return false
		}
		if left[i] < right[j] {
			i++
		} else {
			j++
		}
	}
	return true
}

func verifyPrePRCIAttestation(policy, attestationPayload []byte, mergedTree string) (string, string, error) {
	trust, err := parsePrePRCITrust(policy)
	if err != nil {
		return "", "", err
	}
	if len(attestationPayload) == 0 {
		return "", "", errors.New("trusted CI attestation is required")
	}
	var attestation prePRCIAttestation
	decoder := json.NewDecoder(strings.NewReader(string(attestationPayload)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&attestation); err != nil {
		return "", "", fmt.Errorf("parse CI attestation: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return "", "", errors.New("CI attestation contains multiple JSON values")
	}
	if attestation.Schema != prePRCIAttestationSchema || attestation.Status != "success" || attestation.MergedTree != mergedTree || attestation.Issuer != trust.Issuer {
		return "", "", errors.New("CI attestation is not successful for the exact merged result")
	}
	publicKey, err := base64.StdEncoding.DecodeString(trust.Ed25519PublicKey)
	if err != nil || len(publicKey) != ed25519.PublicKeySize {
		return "", "", errors.New("receipt-bound PRE-PR CI public key is invalid")
	}
	signature, err := base64.StdEncoding.DecodeString(attestation.Signature)
	if err != nil || !ed25519.Verify(ed25519.PublicKey(publicKey), prePRCIAttestationPreimage(attestation), signature) {
		return "", "", errors.New("CI attestation signature is invalid")
	}
	sum := sha256.Sum256(attestationPayload)
	return "sha256:" + hex.EncodeToString(sum[:]), attestation.Issuer, nil
}

func parsePrePRCITrust(policy []byte) (prePRCITrust, error) {
	var envelope struct {
		PrePRCITrust *prePRCITrust `json:"pre_pr_ci_trust"`
	}
	if json.Unmarshal(policy, &envelope) == nil && envelope.PrePRCITrust != nil {
		return *envelope.PrePRCITrust, nil
	}
	var trust prePRCITrust
	for _, line := range strings.Split(string(policy), "\n") {
		key, value, found := strings.Cut(strings.TrimSpace(line), ":")
		if !found {
			continue
		}
		switch strings.TrimSpace(key) {
		case "pre_pr_ci_issuer":
			trust.Issuer = strings.TrimSpace(value)
		case "pre_pr_ci_ed25519_public_key":
			trust.Ed25519PublicKey = strings.TrimSpace(value)
		}
	}
	if trust.Issuer == "" || trust.Ed25519PublicKey == "" {
		return prePRCITrust{}, errors.New("receipt-bound policy does not declare a PRE-PR CI trust root")
	}
	return trust, nil
}

func prePRCIAttestationPreimage(attestation prePRCIAttestation) []byte {
	return []byte(attestation.Schema + "\x00" + attestation.Issuer + "\x00" + attestation.MergedTree + "\x00" + attestation.Status + "\x00")
}
