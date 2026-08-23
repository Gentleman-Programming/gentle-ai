package reviewtransaction

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"reflect"
)

const (
	TargetedValidationRequestSchema   = "gentle-ai.review-targeted-validation-request/v1"
	TargetedValidationRequestSchemaID = "https://gentle-ai.dev/contracts/review-integration/v1/schemas/targeted-validation-request.schema.json"
)

// TargetedValidationRequest is the complete provider-owned input for a scoped
// validator. Consumers never need to inspect compact authority files.
type TargetedValidationRequest struct {
	Schema           string   `json:"schema"`
	RequestHash      string   `json:"request_hash"`
	LineageID        string   `json:"lineage_id"`
	ExpectedRevision string   `json:"expected_revision"`
	TargetIdentity   string   `json:"target_identity"`
	FixFindingIDs    []string `json:"fix_finding_ids"`
	// PolicyContent is the exact policy text frozen with the active compact
	// authority. It may be empty, but is never reconstructed from a live file.
	PolicyContent string `json:"policy_content"`
	// FixFindings and FixClassifications carry only the causal evidence bound to
	// FixFindingIDs, in that exact order, for the scoped validator prompt.
	FixFindings              []Finding         `json:"fix_findings"`
	FixClassifications       []FindingEvidence `json:"fix_classifications"`
	Projection               Projection        `json:"projection"`
	CorrectionCandidateTree  string            `json:"correction_candidate_tree"`
	CorrectionTargetIdentity string            `json:"correction_target_identity"`
	CorrectionPaths          []string          `json:"correction_paths"`
	CorrectionPathsDigest    string            `json:"correction_paths_digest"`
}

// BuildTargetedValidationRequest derives a corrected target from live Git and
// binds the exact validator request to current compact authority.
func BuildTargetedValidationRequest(ctx context.Context, repo string, state CompactState, revision string) (TargetedValidationRequest, error) {
	return buildTargetedValidationRequest(ctx, repo, state, revision, nil)
}

// BuildTargetedValidationRequestFromSnapshot derives the correction boundary
// from a previously captured live snapshot. It never rereads mutable worktree
// content, so status projection and routing can share one candidate tree.
func BuildTargetedValidationRequestFromSnapshot(ctx context.Context, repo string, state CompactState, revision string, live Snapshot) (TargetedValidationRequest, error) {
	return buildTargetedValidationRequest(ctx, repo, state, revision, &live)
}

func buildTargetedValidationRequest(ctx context.Context, repo string, state CompactState, revision string, captured *Snapshot) (TargetedValidationRequest, error) {
	if err := ctx.Err(); err != nil {
		return TargetedValidationRequest{}, err
	}
	if err := state.Validate(); err != nil {
		return TargetedValidationRequest{}, err
	}
	if state.State == StateCorrectionRequired && state.CorrectionAttemptConsumed() {
		return TargetedValidationRequest{}, ErrCompactCorrectionConsumed
	}
	wantRevision, err := CompactRevisionForState(state)
	if err != nil || revision != wantRevision {
		return TargetedValidationRequest{}, errors.New("targeted validation request requires the exact compact authority revision")
	}
	store, err := CompactAuthoritativeStore(ctx, repo, state.LineageID)
	if err != nil {
		return TargetedValidationRequest{}, err
	}
	live, err := store.Load()
	if err != nil || live.Revision != revision {
		return TargetedValidationRequest{}, errors.New("targeted validation request requires the current compact authority revision")
	}
	if state.State != StateCorrectionRequired || state.ProposedCorrectionLines == nil || len(state.FixFindingIDs) == 0 {
		return TargetedValidationRequest{}, errors.New("targeted validation request requires a forecasted compact correction")
	}
	projection := state.InitialSnapshot.Projection
	if projection == "" {
		projection = ProjectionWorkspace
	}
	var fix Snapshot
	if captured == nil {
		fix, err = (SnapshotBuilder{Repo: repo}).Build(ctx, Target{
			Kind: TargetFixDiff, Projection: projection,
			BaseRef: state.CurrentSnapshot.CandidateTree, IntendedUntracked: state.InitialSnapshot.IntendedUntracked,
			LedgerIDs: state.FixFindingIDs,
		})
	} else {
		fix, err = targetedValidationFixFromSnapshot(ctx, repo, state, projection, *captured)
	}
	if err != nil {
		return TargetedValidationRequest{}, err
	}
	if fix.CandidateTree == state.CurrentSnapshot.CandidateTree || fix.Identity == state.CurrentSnapshot.Identity {
		return TargetedValidationRequest{}, errors.New("targeted validation request requires a changed correction candidate")
	}
	if _, err := admitCorrectionScope(fix.Paths, state.GenesisPaths); err != nil {
		return TargetedValidationRequest{}, err
	}
	return targetedValidationRequestForCorrection(state, revision, fix)
}

// targetedValidationRequestForCorrection binds a repository-validated fix
// snapshot to an exact authority revision. Recovery uses the same constructor
// to attest imported historical validator evidence without inventing a second
// correction-subject format.
func targetedValidationRequestForCorrection(state CompactState, revision string, fix Snapshot) (TargetedValidationRequest, error) {
	snapshotProjection, err := canonicalProjection(state.InitialSnapshot.Projection)
	if err != nil || !validSHA256(revision) || fix.Kind != TargetFixDiff || fix.Projection != snapshotProjection ||
		!equalStrings(fix.LedgerIDs, state.FixFindingIDs) || correctionScopeRefused(fix.Paths, state.GenesisPaths) {
		return TargetedValidationRequest{}, errors.New("targeted validation request correction binding is invalid")
	}
	projection := snapshotProjection
	if projection == "" {
		projection = ProjectionWorkspace
	}
	policyContent, err := state.FrozenPolicyForTargetedValidation()
	if err != nil {
		return TargetedValidationRequest{}, err
	}
	findings, classifications, err := targetedValidationCausalEvidence(state)
	if err != nil {
		return TargetedValidationRequest{}, err
	}
	request := TargetedValidationRequest{
		Schema: TargetedValidationRequestSchema, LineageID: state.LineageID, ExpectedRevision: revision,
		TargetIdentity: state.InitialSnapshot.Identity, FixFindingIDs: append([]string(nil), state.FixFindingIDs...),
		PolicyContent: policyContent, FixFindings: findings, FixClassifications: classifications,
		Projection: projection, CorrectionCandidateTree: fix.CandidateTree,
		CorrectionTargetIdentity: fix.Identity, CorrectionPaths: append([]string(nil), fix.Paths...),
		CorrectionPathsDigest: fix.PathsDigest,
	}
	request.RequestHash = targetedValidationRequestHash(request)
	if err := ValidateTargetedValidationRequest(request); err != nil {
		return TargetedValidationRequest{}, err
	}
	return request, nil
}

func targetedValidationFixFromSnapshot(ctx context.Context, repo string, state CompactState, projection Projection, live Snapshot) (Snapshot, error) {
	snapshotProjection, err := canonicalProjection(projection)
	if err != nil {
		return Snapshot{}, err
	}
	if live.Projection != snapshotProjection {
		return Snapshot{}, errors.New("targeted validation live snapshot uses a different correction projection")
	}
	if !equalStrings(live.IntendedUntracked, state.InitialSnapshot.IntendedUntracked) {
		return Snapshot{}, errors.New("targeted validation live snapshot uses a different intended-untracked set")
	}
	builder := SnapshotBuilder{Repo: repo}
	if err := builder.ValidateEvidence(ctx, live); err != nil {
		return Snapshot{}, err
	}
	paths, err := builder.changedPaths(ctx, state.CurrentSnapshot.CandidateTree, live.CandidateTree)
	if err != nil {
		return Snapshot{}, err
	}
	pathsDigest := digestPaths(paths)
	intended := append([]string(nil), state.InitialSnapshot.IntendedUntracked...)
	ledgerIDs := append([]string(nil), state.FixFindingIDs...)
	fix := Snapshot{
		Kind: TargetFixDiff, Projection: snapshotProjection, UnbornHead: live.UnbornHead,
		BaseTree: state.CurrentSnapshot.CandidateTree, CandidateTree: live.CandidateTree,
		PathsDigest: pathsDigest, Paths: paths,
		IntendedUntracked: intended, IntendedUntrackedProof: live.IntendedUntrackedProof,
		LedgerIDs: ledgerIDs,
	}
	fix.Identity = snapshotIdentityForProjection(
		fix.Kind, fix.Projection, fix.BaseTree, fix.CandidateTree, fix.PathsDigest,
		fix.IntendedUntrackedProof, fix.IntendedUntracked, fix.LedgerIDs,
	)
	if err := builder.ValidateEvidence(ctx, fix); err != nil {
		return Snapshot{}, err
	}
	return fix, nil
}

// ValidateTargetedValidationRequest verifies canonical fields and the
// provider-owned request hash without reading repository authority.
func ValidateTargetedValidationRequest(request TargetedValidationRequest) error {
	if request.Schema != TargetedValidationRequestSchema || validateLineageID(request.LineageID) != nil {
		return errors.New("targeted validation request identity is incomplete")
	}
	if !validSHA256(request.RequestHash) || !validSHA256(request.ExpectedRevision) ||
		!validSHA256(request.TargetIdentity) || !validSHA256(request.CorrectionTargetIdentity) ||
		!validSHA256(request.CorrectionPathsDigest) {
		return errors.New("targeted validation request content binding is incomplete")
	}
	if !validGitTree(request.CorrectionCandidateTree) {
		return errors.New("targeted validation request candidate tree is invalid")
	}
	if request.Projection != ProjectionWorkspace && request.Projection != ProjectionStaged {
		return errors.New("targeted validation request projection is invalid")
	}
	ids, err := canonicalStrings(request.FixFindingIDs, "fix finding id")
	if err != nil || !reflect.DeepEqual(ids, request.FixFindingIDs) || len(ids) == 0 {
		return errors.New("targeted validation request finding IDs are not canonical")
	}
	paths, err := canonicalStrings(request.CorrectionPaths, "correction path")
	if err != nil || !reflect.DeepEqual(paths, request.CorrectionPaths) || len(paths) == 0 ||
		digestPaths(paths) != request.CorrectionPathsDigest {
		return errors.New("targeted validation request correction paths do not match their digest")
	}
	for _, path := range paths {
		canonical, err := normalizeLogicalPath(path)
		if err != nil || canonical != path {
			return errors.New("targeted validation request correction path is not repository-relative")
		}
	}
	legacySemanticShape := request.PolicyContent == "" && request.FixFindings == nil && request.FixClassifications == nil
	if !legacySemanticShape {
		if err := validateTargetedValidationCausalEvidence(request.FixFindingIDs, request.FixFindings, request.FixClassifications); err != nil {
			return err
		}
	}
	if targetedValidationRequestHash(request) != request.RequestHash {
		return errors.New("targeted validation request hash does not match its binding")
	}
	return nil
}

func targetedValidationCausalEvidence(state CompactState) ([]Finding, []FindingEvidence, error) {
	findings := make(map[string]Finding, len(state.Findings))
	for _, finding := range state.Findings {
		if _, exists := findings[finding.ID]; exists {
			return nil, nil, errors.New("targeted validation causal evidence repeats a frozen finding") // refusal:by-design world-action: a duplicate frozen finding must be repaired in authority, not selected arbitrarily
		}
		findings[finding.ID] = finding
	}
	orderedFindings := make([]Finding, len(state.FixFindingIDs))
	orderedClassifications := make([]FindingEvidence, len(state.FixFindingIDs))
	for index, findingID := range state.FixFindingIDs {
		finding, found := findings[findingID]
		classification, classified := state.Classifications[findingID]
		if !found || !classified || classification.FindingID != findingID {
			return nil, nil, errors.New("targeted validation causal evidence does not match the frozen fix findings") // refusal:by-design world-action: validator semantics require the exact compact causal ledger
		}
		finding.ProofRefs = append([]string(nil), finding.ProofRefs...)
		orderedFindings[index], orderedClassifications[index] = finding, classification
	}
	return orderedFindings, orderedClassifications, nil
}

func validateTargetedValidationCausalEvidence(ids []string, findings []Finding, classifications []FindingEvidence) error {
	if len(findings) != len(ids) || len(classifications) != len(ids) {
		return errors.New("targeted validation request causal evidence does not cover every fix finding") // refusal:by-design operator-knowledge: the validator must receive every frozen causal finding exactly once
	}
	for index, findingID := range ids {
		if findings[index].ID != findingID || classifications[index].FindingID != findingID {
			return errors.New("targeted validation request causal evidence is not ordered by fix finding IDs") // refusal:by-design world-action: request construction must preserve the compact causal order
		}
	}
	return nil
}

func targetedValidationRequestHash(request TargetedValidationRequest) string {
	preimage := struct {
		Schema                   string            `json:"schema"`
		LineageID                string            `json:"lineage_id"`
		ExpectedRevision         string            `json:"expected_revision"`
		TargetIdentity           string            `json:"target_identity"`
		FixFindingIDs            []string          `json:"fix_finding_ids"`
		PolicyContent            string            `json:"policy_content,omitempty"`
		FixFindings              []Finding         `json:"fix_findings,omitempty"`
		FixClassifications       []FindingEvidence `json:"fix_classifications,omitempty"`
		Projection               Projection        `json:"projection"`
		CorrectionCandidateTree  string            `json:"correction_candidate_tree"`
		CorrectionTargetIdentity string            `json:"correction_target_identity"`
		CorrectionPaths          []string          `json:"correction_paths"`
		CorrectionPathsDigest    string            `json:"correction_paths_digest"`
	}{
		Schema: request.Schema, LineageID: request.LineageID, ExpectedRevision: request.ExpectedRevision,
		TargetIdentity: request.TargetIdentity, FixFindingIDs: request.FixFindingIDs,
		PolicyContent: request.PolicyContent, FixFindings: request.FixFindings, FixClassifications: request.FixClassifications,
		Projection: request.Projection, CorrectionCandidateTree: request.CorrectionCandidateTree,
		CorrectionTargetIdentity: request.CorrectionTargetIdentity, CorrectionPaths: request.CorrectionPaths,
		CorrectionPathsDigest: request.CorrectionPathsDigest,
	}
	payload, _ := json.Marshal(preimage)
	sum := sha256.Sum256(append([]byte("gentle-ai.review-targeted-validation-request/v1\x00"), payload...))
	return "sha256:" + hex.EncodeToString(sum[:])
}
