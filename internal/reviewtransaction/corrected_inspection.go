package reviewtransaction

import (
	"context"
	"errors"
	"reflect"
	"strings"
)

// CorrectedCandidateInspectionRepositoryContextError marks opaque repository
// context resolution failure during corrected inspection.
type CorrectedCandidateInspectionRepositoryContextError struct{ cause error }

func (err *CorrectedCandidateInspectionRepositoryContextError) Error() string {
	return err.cause.Error()
}
func (err *CorrectedCandidateInspectionRepositoryContextError) Unwrap() error { return err.cause }

// ResolveCorrectedCandidateInspection resolves the immutable correction snapshot
// for a captured targeted-validation request without rebuilding live workspace
// content. The returned snapshot can be passed to SnapshotBuilder.InspectCandidate.
func ResolveCorrectedCandidateInspection(ctx context.Context, repo string, repositoryContextHandle string, request TargetedValidationRequest) (Snapshot, error) {
	if err := ValidateTargetedValidationRequest(request); err != nil {
		return Snapshot{}, errors.New("corrected candidate inspection request is invalid") // refusal:by-design operator-knowledge: only a fresh native transition can provide an exact provider-issued request
	}
	var binding ReviewRepositoryContextBinding
	var err error
	if strings.HasPrefix(repositoryContextHandle, reviewRepositoryContextV2HandlePrefix) {
		repo, binding, err = resolveReviewRepositoryContextV2TokenForCorrectedInspection(ctx, repo, repositoryContextHandle, ReviewRepositoryContextBinding{
			LineageID: request.LineageID, TargetIdentity: request.CorrectionTargetIdentity, Revision: request.ExpectedRevision,
		})
	} else {
		repo, binding, err = resolveOpaqueReviewRepositoryContext(ctx, repositoryContextHandle)
	}
	if err != nil {
		return Snapshot{}, &CorrectedCandidateInspectionRepositoryContextError{cause: err}
	}
	if binding.LineageID != request.LineageID || binding.Revision != request.ExpectedRevision ||
		binding.TargetIdentity != request.CorrectionTargetIdentity {
		return Snapshot{}, errors.New("corrected candidate inspection context does not match request") // refusal:by-design operator-knowledge: the opaque locator commits the exact binding and cannot be corrected by caller input
	}
	store, err := CompactAuthoritativeStore(ctx, repo, request.LineageID)
	if err != nil {
		return Snapshot{}, err
	}
	record, err := store.Load()
	if err != nil {
		return Snapshot{}, err
	}
	state := record.State
	if state.CapturePhaseRevision != request.ExpectedRevision || state.LineageID != request.LineageID ||
		state.State != StateCorrectionRequired || state.ProposedCorrectionLines == nil || state.CorrectionAttemptConsumed() {
		return Snapshot{}, errors.New("corrected candidate inspection requires current unconsumed correction authority") // refusal:by-design operator-knowledge: only current compact authority can identify an open correction transaction
	}
	projection, err := canonicalProjection(state.InitialSnapshot.Projection)
	if err != nil {
		return Snapshot{}, err
	}
	correction := Snapshot{
		Kind: TargetFixDiff, Projection: projection, UnbornHead: state.CurrentSnapshot.UnbornHead,
		BaseTree: state.CurrentSnapshot.CandidateTree, CandidateTree: request.CorrectionCandidateTree,
		PathsDigest: request.CorrectionPathsDigest, Paths: append([]string(nil), request.CorrectionPaths...),
		IntendedUntracked: append([]string(nil), state.InitialSnapshot.IntendedUntracked...),
		LedgerIDs:         append([]string(nil), state.FixFindingIDs...),
	}
	builder := SnapshotBuilder{Repo: repo}
	correction.IntendedUntrackedProof, err = builder.untrackedProof(ctx, correction.CandidateTree, correction.IntendedUntracked)
	if err != nil {
		return Snapshot{}, err
	}
	correction.Identity = snapshotIdentityForProjection(correction.Kind, correction.Projection, correction.BaseTree,
		correction.CandidateTree, correction.PathsDigest, correction.IntendedUntrackedProof, correction.IntendedUntracked, correction.LedgerIDs)
	if err := builder.ValidateEvidence(ctx, correction); err != nil || correction.Identity != request.CorrectionTargetIdentity {
		return Snapshot{}, errors.New("corrected candidate inspection tree evidence does not match request") // refusal:by-design world-action: missing or mismatched Git objects require a fresh correction capture
	}
	expected, err := targetedValidationRequestForCorrection(state, state.CapturePhaseRevision, correction)
	if err != nil || !reflect.DeepEqual(request, expected) {
		return Snapshot{}, errors.New("corrected candidate inspection request does not match authority") // refusal:by-design operator-knowledge: only native authority can derive the exact targeted-validation request
	}
	return correction, nil
}

func frozenCorrectedCandidateInspectionRequest(ctx context.Context, repo string, state CompactState, revision, candidateTree string) (TargetedValidationRequest, error) {
	projection, err := canonicalProjection(state.InitialSnapshot.Projection)
	if err != nil {
		return TargetedValidationRequest{}, err
	}
	builder := SnapshotBuilder{Repo: repo}
	correction := Snapshot{
		Kind: TargetFixDiff, Projection: projection, UnbornHead: state.CurrentSnapshot.UnbornHead,
		BaseTree: state.CurrentSnapshot.CandidateTree, CandidateTree: candidateTree,
		IntendedUntracked: append([]string(nil), state.InitialSnapshot.IntendedUntracked...),
		LedgerIDs:         append([]string(nil), state.FixFindingIDs...),
	}
	correction.Paths, err = builder.changedPaths(ctx, correction.BaseTree, correction.CandidateTree)
	if err != nil {
		return TargetedValidationRequest{}, err
	}
	correction.PathsDigest = digestPaths(correction.Paths)
	correction.IntendedUntrackedProof, err = builder.untrackedProof(ctx, correction.CandidateTree, correction.IntendedUntracked)
	if err != nil {
		return TargetedValidationRequest{}, err
	}
	correction.Identity = snapshotIdentityForProjection(correction.Kind, correction.Projection, correction.BaseTree,
		correction.CandidateTree, correction.PathsDigest, correction.IntendedUntrackedProof, correction.IntendedUntracked, correction.LedgerIDs)
	if err := builder.ValidateEvidence(ctx, correction); err != nil {
		return TargetedValidationRequest{}, err
	}
	return targetedValidationRequestForCorrection(state, revision, correction)
}

// ResolveCorrectedCandidateInspectionBinding verifies a provider-issued targeted inspection binding.
func ResolveCorrectedCandidateInspectionBinding(ctx context.Context, repo string, repositoryContextHandle string, binding ReviewRepositoryContextBinding, requestHash string) (SnapshotBuilder, Snapshot, error) {
	repo, contextBinding, frozen, err := resolveTargetedValidationReviewRepositoryContext(ctx, repo, repositoryContextHandle, binding)
	if err != nil {
		return SnapshotBuilder{}, Snapshot{}, &CorrectedCandidateInspectionRepositoryContextError{cause: err}
	}
	if contextBinding != binding {
		return SnapshotBuilder{}, Snapshot{}, errors.New("corrected candidate inspection context does not match binding") // refusal:by-design operator-knowledge: only the opaque provider context commits the exact corrected binding
	}
	if frozen.RequestHash != requestHash {
		return SnapshotBuilder{}, Snapshot{}, errors.New("corrected candidate inspection request hash does not match authority") // refusal:by-design operator-knowledge: only the native targeted request owns this hash
	}
	store, err := CompactAuthoritativeStore(ctx, repo, binding.LineageID)
	if err != nil {
		return SnapshotBuilder{}, Snapshot{}, err
	}
	record, err := store.Load()
	if err != nil {
		return SnapshotBuilder{}, Snapshot{}, err
	}
	if record.State.CapturePhaseRevision != binding.Revision || record.State.State != StateCorrectionRequired ||
		record.State.ProposedCorrectionLines == nil || record.State.CorrectionAttemptConsumed() {
		return SnapshotBuilder{}, Snapshot{}, errors.New("corrected candidate inspection requires current unconsumed correction authority") // refusal:by-design operator-knowledge: a stale or spent correction must be refreshed through STATUS
	}
	request, err := frozenCorrectedCandidateInspectionRequest(ctx, repo, record.State, record.State.CapturePhaseRevision, frozen.CorrectionCandidateTree)
	if err != nil || request.CorrectionTargetIdentity != binding.TargetIdentity || request.RequestHash != requestHash {
		return SnapshotBuilder{}, Snapshot{}, errors.New("corrected candidate inspection request hash does not match authority") // refusal:by-design operator-knowledge: only the native targeted request owns this hash
	}
	// The frozen correction tree belongs to this exact provider context. The
	// canonical resolver revalidates it without rebuilding the live correction
	// candidate after the validator was issued.
	snapshot, err := ResolveCorrectedCandidateInspection(ctx, repo, repositoryContextHandle, request)
	if err != nil {
		return SnapshotBuilder{}, Snapshot{}, err
	}
	return SnapshotBuilder{Repo: repo}, snapshot, nil
}
