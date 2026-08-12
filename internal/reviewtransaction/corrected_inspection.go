package reviewtransaction

import (
	"context"
	"errors"
	"reflect"
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
func ResolveCorrectedCandidateInspection(ctx context.Context, repositoryContextHandle string, request TargetedValidationRequest) (Snapshot, error) {
	if err := ValidateTargetedValidationRequest(request); err != nil {
		return Snapshot{}, errors.New("corrected candidate inspection request is invalid") // refusal:by-design operator-knowledge: only a fresh native transition can provide an exact provider-issued request
	}
	repo, binding, err := resolveOpaqueReviewRepositoryContext(ctx, repositoryContextHandle)
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
	if record.Revision != request.ExpectedRevision || state.LineageID != request.LineageID ||
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
	expected, err := targetedValidationRequestForCorrection(state, record.Revision, correction)
	if err != nil || !reflect.DeepEqual(request, expected) {
		return Snapshot{}, errors.New("corrected candidate inspection request does not match authority") // refusal:by-design operator-knowledge: only native authority can derive the exact targeted-validation request
	}
	captured, err := ReadCapturedVerificationEvidence(store.Dir, state.LineageID, record.Revision, correction)
	if err != nil || captured.Record.Outcome != VerificationOutcomePassed {
		return Snapshot{}, errors.New("corrected candidate inspection requires passed repository verification evidence") // refusal:by-design operator-knowledge: STATUS supplies the exact candidate-bound evidence capture transition
	}
	return correction, nil
}

// ResolveCorrectedCandidateInspectionBinding verifies a provider-issued targeted inspection binding.
func ResolveCorrectedCandidateInspectionBinding(ctx context.Context, repositoryContextHandle string, binding ReviewRepositoryContextBinding, requestHash string) (SnapshotBuilder, Snapshot, error) {
	repo, contextBinding, err := resolveOpaqueReviewRepositoryContext(ctx, repositoryContextHandle)
	if err != nil {
		return SnapshotBuilder{}, Snapshot{}, &CorrectedCandidateInspectionRepositoryContextError{cause: err}
	}
	if contextBinding != binding {
		return SnapshotBuilder{}, Snapshot{}, errors.New("corrected candidate inspection context does not match binding") // refusal:by-design operator-knowledge: only the opaque provider context commits the exact corrected binding
	}
	store, err := CompactAuthoritativeStore(ctx, repo, binding.LineageID)
	if err != nil {
		return SnapshotBuilder{}, Snapshot{}, err
	}
	record, err := store.Load()
	if err != nil {
		return SnapshotBuilder{}, Snapshot{}, err
	}
	if record.Revision != binding.Revision || record.State.State != StateCorrectionRequired ||
		record.State.ProposedCorrectionLines == nil || record.State.CorrectionAttemptConsumed() {
		return SnapshotBuilder{}, Snapshot{}, errors.New("corrected candidate inspection requires current unconsumed correction authority") // refusal:by-design operator-knowledge: a stale or spent correction must be refreshed through STATUS
	}
	captured, err := ReadCapturedVerificationEvidenceByIdentity(store.Dir, binding.LineageID, binding.Revision, binding.TargetIdentity)
	if err != nil || captured.Record.Outcome != VerificationOutcomePassed {
		return SnapshotBuilder{}, Snapshot{}, errors.New("corrected candidate inspection requires passed repository verification evidence") // refusal:by-design world-action: capture passed evidence for the exact corrected target first
	}
	projection, err := canonicalProjection(record.State.InitialSnapshot.Projection)
	if err != nil {
		return SnapshotBuilder{}, Snapshot{}, err
	}
	correction := Snapshot{
		Kind: TargetFixDiff, Projection: projection, UnbornHead: record.State.CurrentSnapshot.UnbornHead,
		BaseTree: record.State.CurrentSnapshot.CandidateTree, CandidateTree: captured.Record.CandidateTree,
		PathsDigest: captured.Record.PathsDigest, Paths: append([]string(nil), captured.Record.Paths...),
		IntendedUntracked: append([]string(nil), record.State.InitialSnapshot.IntendedUntracked...),
		LedgerIDs:         append([]string(nil), record.State.FixFindingIDs...),
	}
	builder := SnapshotBuilder{Repo: repo}
	correction.IntendedUntrackedProof, err = builder.untrackedProof(ctx, correction.CandidateTree, correction.IntendedUntracked)
	if err != nil {
		return SnapshotBuilder{}, Snapshot{}, err
	}
	correction.Identity = snapshotIdentityForProjection(correction.Kind, correction.Projection, correction.BaseTree,
		correction.CandidateTree, correction.PathsDigest, correction.IntendedUntrackedProof, correction.IntendedUntracked, correction.LedgerIDs)
	request, err := targetedValidationRequestForCorrection(record.State, record.Revision, correction)
	if err != nil || request.RequestHash != requestHash {
		return SnapshotBuilder{}, Snapshot{}, errors.New("corrected candidate inspection request hash does not match authority") // refusal:by-design operator-knowledge: only the native targeted request owns this hash
	}
	// Passed evidence bootstraps correction; the canonical resolver revalidates Git evidence and target identity.
	// The request hash covers that correction target identity, so both passes are required.
	snapshot, err := ResolveCorrectedCandidateInspection(ctx, repositoryContextHandle, request)
	if err != nil {
		return SnapshotBuilder{}, Snapshot{}, err
	}
	return builder, snapshot, nil
}
