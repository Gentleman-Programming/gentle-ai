package reviewtransaction

import (
	"bytes"
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

func prePRBoundaryAdvanced(refs *resolvedPrePRRefs) bool {
	return refs != nil && refs.Selection.Commit != refs.Selection.MergeBase
}

func prePRAttestationRequested(request GateRequest) bool {
	return request.PrePR != nil && strings.TrimSpace(request.PrePR.CIAttestationArtifact) != ""
}

// deriveBaseAdvanceCompatibility verifies the shared content and merge proof.
func deriveBaseAdvanceCompatibility(ctx context.Context, repo string, receipt Receipt, request GateRequest, snapshot Snapshot, refs *resolvedPrePRRefs, preimages gateArtifactPreimages, requireAttestation bool) (BaseAdvanceCompatibility, error) {
	if refs == nil {
		return BaseAdvanceCompatibility{}, errors.New("resolved pre-PR refs are missing")
	}
	if request.ExternalEvidence != ExternalEvidenceNone {
		return BaseAdvanceCompatibility{}, errors.New("external evidence invalidates or escalates compatibility")
	}
	if requireAttestation && (request.PrePR == nil || strings.TrimSpace(request.PrePR.CIAttestationArtifact) == "") {
		return BaseAdvanceCompatibility{}, errors.New("trusted CI attestation is required")
	}
	builder := SnapshotBuilder{Repo: repo}
	reviewedHead, err := reviewedBaseAdvanceHead(ctx, repo, receipt.FinalCandidateTree, refs.HeadCommit)
	if err != nil {
		return BaseAdvanceCompatibility{}, err
	}
	mergeBase, err := runGit(ctx, repo, nil, nil, "merge-base", "--all", refs.Selection.Commit, reviewedHead)
	if err != nil || len(strings.Fields(string(mergeBase))) != 1 {
		return BaseAdvanceCompatibility{}, errors.New("reviewed base is not the unique merge-base of the advanced parent and candidate") // refusal:by-design world-action: only a new merge or reviewed candidate can establish one unambiguous ancestry proof
	}
	mergeBaseTree, err := builder.resolveTree(ctx, strings.TrimSpace(string(mergeBase)))
	if err != nil || mergeBaseTree != receipt.BaseTree {
		return BaseAdvanceCompatibility{}, errors.New("original reviewed merge-base tree is not preserved")
	}
	advertisedBaseTree, err := builder.resolveTree(ctx, refs.Selection.Commit)
	if err != nil {
		return BaseAdvanceCompatibility{}, errors.New("advertised pre-PR base tree cannot be derived") // refusal:by-design world-action: only Git object recovery can restore a missing advertised base tree
	}

	originalPaths, err := builder.changedPaths(ctx, receipt.BaseTree, receipt.FinalCandidateTree)
	if err != nil {
		return BaseAdvanceCompatibility{}, err
	}
	deliveredBaseTree := snapshot.BaseTree
	if deliveredBaseTree != receipt.BaseTree && deliveredBaseTree != advertisedBaseTree {
		return BaseAdvanceCompatibility{}, errors.New("delivered target base is neither the reviewed nor advanced parent base") // refusal:by-design world-action: only changing the delivery target or reviewing a new candidate can establish an allowed base
	}
	currentPaths, err := builder.changedPaths(ctx, deliveredBaseTree, snapshot.CandidateTree)
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
	currentPatch, err := patchIdentity(ctx, repo, deliveredBaseTree, snapshot.CandidateTree)
	if err != nil || originalPatch != currentPatch {
		return BaseAdvanceCompatibility{}, errors.New("delivered patch identity changed")
	}
	basePaths, err := builder.changedPaths(ctx, receipt.BaseTree, advertisedBaseTree)
	if err != nil {
		return BaseAdvanceCompatibility{}, err
	}
	if !disjointPaths(originalPaths, basePaths) {
		return BaseAdvanceCompatibility{}, errors.New("base advance overlaps delivered paths")
	}
	mergedOutput, err := runGit(ctx, repo, nil, nil, "merge-tree", "--write-tree", refs.Selection.Commit, reviewedHead)
	if err != nil {
		return BaseAdvanceCompatibility{}, fmt.Errorf("merge against new base is not conflict-free: %w", err)
	}
	mergedFields := strings.Fields(string(mergedOutput))
	if len(mergedFields) == 0 || !validGitTree(mergedFields[0]) {
		return BaseAdvanceCompatibility{}, errors.New("merged result tree cannot be derived")
	}
	approvedEntries, err := listTreeEntries(ctx, repo, receipt.FinalCandidateTree)
	if err != nil {
		return BaseAdvanceCompatibility{}, err
	}
	mergedEntries, err := listTreeEntries(ctx, repo, mergedFields[0])
	if err != nil {
		return BaseAdvanceCompatibility{}, err
	}
	for _, path := range originalPaths {
		approved, approvedPresent := approvedEntries[path]
		merged, mergedPresent := mergedEntries[path]
		if approvedPresent != mergedPresent || !bytes.Equal(approved, merged) {
			return BaseAdvanceCompatibility{}, errors.New("merge result changed reviewed projection") // refusal:by-design world-action: only changing the merge result and reviewing the new candidate can restore this projection
		}
	}
	var attestationHash, issuer string
	if requireAttestation {
		attestationHash, issuer, err = verifyPrePRCIAttestation(preimages.policy, preimages.ciAttestation, mergedFields[0])
		if err != nil {
			return BaseAdvanceCompatibility{}, err
		}
	}
	selector := ""
	if refs.Selection.Source == PrePRBoundaryExplicit {
		selector = refs.Selection.Selector
	}
	var selectionNow PrePRBoundarySelection
	if request.Gate == GatePreCommit || request.Gate == GatePrePush {
		selectionNow, err = selectExplicitBaseAdvanceBoundary(ctx, repo, selector)
	} else {
		selectionNow, err = reselectBoundaryForGate(ctx, repo, request.Gate, selector)
	}
	if err != nil || selectionNow != refs.Selection {
		return BaseAdvanceCompatibility{}, errors.New("pre-PR base ref advanced during validation")
	}
	headNow, err := resolveCommit(ctx, repo, "HEAD")
	if err != nil || headNow != refs.HeadCommit {
		return BaseAdvanceCompatibility{}, errors.New("HEAD advanced during validation")
	}
	status, ciStatus := baseAdvanceCompatibleLocalStatus, currentChangesBoundaryCIStatus
	if requireAttestation {
		status, ciStatus = baseAdvanceCompatibleStatus, "success"
	}
	proof := BaseAdvanceCompatibility{
		Status: status, Compatible: true, OriginalMergeBaseTree: receipt.BaseTree, NewBaseTree: advertisedBaseTree,
		OriginalPatchIdentity: originalPatch, DeliveredPatchIdentity: currentPatch,
		DeliveredPathsDigest: receipt.PathsDigest, BaseAdvancePathsDigest: digestPaths(basePaths), PathsDisjoint: true,
		MergedResultTree: mergedFields[0], CIAttestationArtifactHash: attestationHash,
		CIAttestationIssuer: issuer, CIStatus: ciStatus,
	}
	if !proof.valid() {
		return BaseAdvanceCompatibility{}, errors.New("compatible base advance proof is incomplete")
	}
	return proof, nil
}

// reviewedBaseAdvanceHead binds a committed merge back to its reviewed parent
// without consulting MERGE_HEAD. A staged merge still has HEAD at C0; a
// committed merge must retain a parent with C0's exact tree.
func reviewedBaseAdvanceHead(ctx context.Context, repo, reviewedTree, head string) (string, error) {
	builder := SnapshotBuilder{Repo: repo}
	headTree, err := builder.resolveTree(ctx, head)
	if err != nil {
		return "", err
	}
	if headTree == reviewedTree {
		return head, nil
	}
	parents, err := runGit(ctx, repo, nil, nil, "rev-list", "--parents", "-n", "1", head)
	if err != nil {
		return "", err
	}
	for _, parent := range strings.Fields(string(parents))[1:] {
		tree, treeErr := builder.resolveTree(ctx, parent)
		if treeErr == nil && tree == reviewedTree {
			return parent, nil
		}
	}
	return "", errors.New("committed merge does not retain the reviewed candidate parent") // refusal:by-design world-action: only recreating the merge with the reviewed parent can restore the proof
}

func deriveExplicitBaseAdvanceCompatibility(ctx context.Context, repo string, receipt Receipt, request GateRequest, snapshot Snapshot, preimages gateArtifactPreimages) (BaseAdvanceCompatibility, error) {
	selector := strings.TrimSpace(request.Target.BaseRef)
	if selector == "" {
		return BaseAdvanceCompatibility{}, errors.New("compatible local base advance requires an explicit base ref") // refusal:by-design operator-knowledge: only the caller can choose the intended reviewed base
	}
	selection, err := selectExplicitBaseAdvanceBoundary(ctx, repo, selector)
	if err != nil {
		return BaseAdvanceCompatibility{}, err
	}
	head, err := resolveCommit(ctx, repo, "HEAD")
	if err != nil {
		return BaseAdvanceCompatibility{}, err
	}
	return deriveBaseAdvanceCompatibility(ctx, repo, receipt, request, snapshot,
		&resolvedPrePRRefs{Selection: selection, HeadCommit: head}, preimages, false)
}

func selectExplicitBaseAdvanceBoundary(ctx context.Context, repo, selector string) (PrePRBoundarySelection, error) {
	if strings.TrimSpace(selector) == "" {
		return PrePRBoundarySelection{}, errors.New("compatible local base advance requires an explicit base ref") // refusal:by-design operator-knowledge: only the caller can choose the intended reviewed base
	}
	return selectPrePushBoundary(ctx, repo, selector)
}

// reselectBoundaryForGate re-derives the boundary selector using exactly the
// same resolution algorithm the target's own gate used the first time
// (gate.go's prePRBoundaryForRequest vs. buildPushTarget/selectPrePushBoundary
// diverge on how an empty/default selector resolves), so the freshness check
// below compares like with like instead of risking a false "advanced during
// validation" -- or worse, a false pass -- from mixing the two boundary
// resolvers.
func reselectBoundaryForGate(ctx context.Context, repo string, gate GateKind, selector string) (PrePRBoundarySelection, error) {
	if gate == GatePrePush {
		return selectPrePushBoundary(ctx, repo, selector)
	}
	return selectPrePRBoundary(ctx, repo, selector)
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
	if len(state.CorrectionScopePaths()) == 0 {
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
	if len(deliveredPaths) == 0 || pathsAreSubset(deliveredPaths, state.CorrectionScopePaths()) != nil {
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
	if err := validateReviewedPublicationRange(ctx, repo, state.CorrectionScopePaths(), refs); err != nil {
		return BaseAdvanceCompatibility{}, err
	}
	mergedOutput, err := runGit(ctx, repo, nil, nil, "merge-tree", "--write-tree", refs.Selection.Commit, refs.HeadCommit)
	if err != nil {
		return BaseAdvanceCompatibility{}, fmt.Errorf("merge against the advanced boundary is not conflict-free: %w", err)
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
