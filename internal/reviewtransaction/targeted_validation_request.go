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
	Schema                   string     `json:"schema"`
	RequestHash              string     `json:"request_hash"`
	LineageID                string     `json:"lineage_id"`
	ExpectedRevision         string     `json:"expected_revision"`
	TargetIdentity           string     `json:"target_identity"`
	FixFindingIDs            []string   `json:"fix_finding_ids"`
	Projection               Projection `json:"projection"`
	CorrectionCandidateTree  string     `json:"correction_candidate_tree"`
	CorrectionTargetIdentity string     `json:"correction_target_identity"`
	CorrectionPaths          []string   `json:"correction_paths"`
	CorrectionPathsDigest    string     `json:"correction_paths_digest"`
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
	if err := pathsAreSubset(fix.Paths, state.GenesisPaths); err != nil {
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
		!equalStrings(fix.LedgerIDs, state.FixFindingIDs) || pathsAreSubset(fix.Paths, state.GenesisPaths) != nil {
		return TargetedValidationRequest{}, errors.New("targeted validation request correction binding is invalid")
	}
	projection := snapshotProjection
	if projection == "" {
		projection = ProjectionWorkspace
	}
	request := TargetedValidationRequest{
		Schema: TargetedValidationRequestSchema, LineageID: state.LineageID, ExpectedRevision: revision,
		TargetIdentity: state.InitialSnapshot.Identity, FixFindingIDs: append([]string(nil), state.FixFindingIDs...),
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
	if targetedValidationRequestHash(request) != request.RequestHash {
		return errors.New("targeted validation request hash does not match its binding")
	}
	return nil
}

func targetedValidationRequestHash(request TargetedValidationRequest) string {
	preimage := struct {
		Schema                   string     `json:"schema"`
		LineageID                string     `json:"lineage_id"`
		ExpectedRevision         string     `json:"expected_revision"`
		TargetIdentity           string     `json:"target_identity"`
		FixFindingIDs            []string   `json:"fix_finding_ids"`
		Projection               Projection `json:"projection"`
		CorrectionCandidateTree  string     `json:"correction_candidate_tree"`
		CorrectionTargetIdentity string     `json:"correction_target_identity"`
		CorrectionPaths          []string   `json:"correction_paths"`
		CorrectionPathsDigest    string     `json:"correction_paths_digest"`
	}{
		Schema: request.Schema, LineageID: request.LineageID, ExpectedRevision: request.ExpectedRevision,
		TargetIdentity: request.TargetIdentity, FixFindingIDs: request.FixFindingIDs, Projection: request.Projection,
		CorrectionCandidateTree:  request.CorrectionCandidateTree,
		CorrectionTargetIdentity: request.CorrectionTargetIdentity, CorrectionPaths: request.CorrectionPaths,
		CorrectionPathsDigest: request.CorrectionPathsDigest,
	}
	payload, _ := json.Marshal(preimage)
	sum := sha256.Sum256(append([]byte("gentle-ai.review-targeted-validation-request/v1\x00"), payload...))
	return "sha256:" + hex.EncodeToString(sum[:])
}
