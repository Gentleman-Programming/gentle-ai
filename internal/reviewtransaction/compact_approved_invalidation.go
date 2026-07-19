package reviewtransaction

import (
	"context"
	"errors"
	"fmt"
	"os"
	"reflect"
	"strings"
	"time"
)

// CompactApprovedInvalidationRequest identifies one approved compact lineage
// and the lifecycle gate whose live repository evidence must be re-derived.
type CompactApprovedInvalidationRequest struct {
	LineageID        string
	ExpectedRevision string
	Gate             NativeGateRequestInput
}

var finalCompactInvalidationHook = func() {}

// InvalidateApprovedCompactAuthority terminally revokes an approved lineage
// only when its requested lifecycle gate natively re-derives an invalidated
// result while the repository-wide compact authority lock is held. Gate reads
// remain pure; this explicit transition is the sole mutation boundary.
func InvalidateApprovedCompactAuthority(ctx context.Context, repo string, request CompactApprovedInvalidationRequest) (CompactRecord, NativeGateEvaluation, error) {
	if err := ctx.Err(); err != nil {
		return CompactRecord{}, NativeGateEvaluation{}, err
	}
	if err := validateLineageID(request.LineageID); err != nil {
		return CompactRecord{}, NativeGateEvaluation{}, err
	}
	if strings.TrimSpace(request.ExpectedRevision) == "" {
		return CompactRecord{}, NativeGateEvaluation{}, errors.New("approved invalidation requires the exact current store revision")
	}
	if request.Gate.LineageID != "" && request.Gate.LineageID != request.LineageID {
		return CompactRecord{}, NativeGateEvaluation{}, errors.New("approved invalidation gate lineage does not match the requested lineage")
	}
	switch request.Gate.Gate {
	case GatePostApply, GatePreCommit, GatePrePush, GatePrePR, GateRelease:
	default:
		return CompactRecord{}, NativeGateEvaluation{}, fmt.Errorf("approved invalidation requires a supported lifecycle gate, got %q", request.Gate.Gate)
	}
	if request.Gate.Gate == GateRelease {
		for _, path := range []string{
			request.Gate.ReleaseConfiguration, request.Gate.ReleaseGenerated, request.Gate.ReleaseProvenance,
			request.Gate.ReleasePublicationBoundary, request.Gate.ReleaseEvidenceFreshness,
		} {
			if strings.TrimSpace(path) == "" {
				return CompactRecord{}, NativeGateEvaluation{}, errors.New("release gate requires complete independent release evidence")
			}
		}
	}
	request.Gate.LineageID = request.LineageID
	store, err := CompactAuthoritativeStore(ctx, repo, request.LineageID)
	if err != nil {
		return CompactRecord{}, NativeGateEvaluation{}, err
	}
	lock, err := acquireStoreLock(store.lockPath)
	if err != nil {
		return CompactRecord{}, NativeGateEvaluation{}, err
	}
	defer lock.release()

	current, err := store.Load()
	if err != nil {
		return CompactRecord{}, NativeGateEvaluation{}, fmt.Errorf("load approved invalidation target: %w", err)
	}
	if current.HistoricalCompat {
		return CompactRecord{}, NativeGateEvaluation{}, fmt.Errorf("%w: review/invalidate for lineage %q", ErrHistoricalCompatReadOnly, request.LineageID)
	}
	if current.State.State == StateInvalidated && current.State.InvalidationEvidence != nil &&
		current.State.InvalidationEvidence.Context.StoreRevision == request.ExpectedRevision &&
		current.State.InvalidationEvidence.Gate == request.Gate.Gate {
		evidence := current.State.InvalidationEvidence
		evaluation := NativeGateEvaluation{Result: GateInvalidated, Reason: evidence.Reason, Context: evidence.Context}
		if err := os.Remove(store.ReceiptPath()); err != nil && !os.IsNotExist(err) {
			return current, evaluation, fmt.Errorf("remove invalidated compact receipt: %w", err)
		}
		return current, evaluation, nil
	}
	if current.Revision != request.ExpectedRevision {
		return CompactRecord{}, NativeGateEvaluation{}, fmt.Errorf("%w: expected compact revision %q, current %q", ErrConcurrentUpdate, request.ExpectedRevision, current.Revision)
	}
	if current.State.State != StateApproved {
		return CompactRecord{}, NativeGateEvaluation{}, fmt.Errorf("approved invalidation requires an approved authority, got %q", current.State.State)
	}
	receipt, err := current.State.Receipt()
	if err != nil {
		return CompactRecord{}, NativeGateEvaluation{}, err
	}
	published, err := os.ReadFile(store.ReceiptPath())
	if err != nil {
		return CompactRecord{}, NativeGateEvaluation{}, fmt.Errorf("load approved invalidation receipt: %w", err)
	}
	publishedReceipt, err := ParseCompactReceipt(published)
	if err != nil || !compactReceiptEqual(publishedReceipt, receipt) {
		return CompactRecord{}, NativeGateEvaluation{}, errors.New("approved invalidation receipt does not match current authority")
	}

	evaluation := evaluateCompactGate(ctx, store.repo, receipt, request.Gate, true)
	if err := ctx.Err(); err != nil {
		return CompactRecord{}, evaluation, fmt.Errorf("approved invalidation gate infrastructure failure: %w", err)
	}
	if evaluation.Cause != nil {
		return CompactRecord{}, evaluation, fmt.Errorf("approved invalidation gate infrastructure failure: %w", evaluation.Cause)
	}
	if evaluation.Result != GateInvalidated {
		return CompactRecord{}, evaluation, fmt.Errorf("healthy approved authority cannot be invalidated: native gate result is %q", evaluation.Result)
	}
	deniedTarget, deniedUntracked, deniedErr := compactInvalidationTarget(ctx, store.repo, current.State, request.Gate)
	if deniedErr != nil && compactGateInfrastructureFailure(deniedErr) {
		// A genuine git/process/context fault must fail closed and never
		// terminate an approved authority on unreliable evidence.
		return CompactRecord{}, evaluation, fmt.Errorf("derive approved invalidation target: %w", deniedErr)
	}
	finalCompactInvalidationHook()
	finalEvaluation := evaluateCompactGate(ctx, store.repo, receipt, request.Gate, true)
	finalTarget, finalUntracked, finalErr := compactInvalidationTarget(ctx, store.repo, current.State, request.Gate)
	if finalErr != nil && compactGateInfrastructureFailure(finalErr) {
		cause := errors.Join(finalEvaluation.Cause, finalErr, ErrConcurrentUpdate)
		return CompactRecord{}, finalEvaluation, fmt.Errorf("approved invalidation target changed during final derivation: %w", cause)
	}
	// A denial whose cause is an underivable live target — a semantic scope
	// violation such as a now-tracked intended-untracked path (issue #1269) —
	// has no stable snapshot to compare. Its stability is proven instead by
	// both target derivations failing with the identical semantic error and by
	// the bound gate denial below. Infrastructure faults were already rejected
	// above, so any error reaching the stability check here is semantic.
	if finalEvaluation.Cause != nil ||
		!compactInvalidationTargetStable(deniedTarget, deniedUntracked, deniedErr, finalTarget, finalUntracked, finalErr) ||
		!reflect.DeepEqual(finalEvaluation, evaluation) || !compactInvalidationDenialBound(finalEvaluation, current, request) {
		cause := errors.Join(finalEvaluation.Cause, deniedErr, finalErr, ErrConcurrentUpdate)
		return CompactRecord{}, finalEvaluation, fmt.Errorf("approved invalidation target changed during final derivation: %w", cause)
	}
	evaluation = finalEvaluation
	next := current.State
	if err := next.invalidateApproved(evaluation); err != nil {
		return CompactRecord{}, evaluation, err
	}
	record, payload, err := makeCompactRecord(next)
	if err != nil {
		return CompactRecord{}, evaluation, err
	}
	if err := writeAtomic(store.StatePath(), payload, 0o644); err != nil {
		return CompactRecord{}, evaluation, err
	}
	if store.TracePath != "" {
		_ = appendCompactTrace(store.TracePath, CompactTraceEntry{
			Operation: "review/invalidate-approved", PreviousRevision: current.Revision,
			Revision: record.Revision, State: next.State, RecordedAt: time.Now().UTC().Format(time.RFC3339Nano),
		})
	}
	if err := os.Remove(store.ReceiptPath()); err != nil && !os.IsNotExist(err) {
		return record, evaluation, fmt.Errorf("remove invalidated compact receipt: %w", err)
	}
	return record, evaluation, nil
}

func compactInvalidationTarget(ctx context.Context, repo string, state CompactState, input NativeGateRequestInput) (Snapshot, string, error) {
	request, _, err := buildCompactGateRequest(ctx, repo, state, input)
	if err != nil {
		return Snapshot{}, "", err
	}
	snapshot, _, err := buildCompactLifecycleSnapshot(ctx, repo, request)
	if err != nil {
		return Snapshot{}, "", err
	}
	untracked, err := (SnapshotBuilder{Repo: repo}).DiscoverIntendedUntracked(ctx)
	return snapshot, digestPaths(untracked), err
}

// compactInvalidationTargetStable reports whether two live target derivations
// observed the same denial across the final-derivation window. A derivable
// target must match snapshot identity and untracked digest exactly. An
// underivable target — a semantic scope violation that prevents lifecycle
// snapshot derivation — is stable only when both derivations failed with the
// identical semantic error. Infrastructure faults are rejected by the caller
// before this comparison, so any non-nil error reaching here is semantic.
func compactInvalidationTargetStable(deniedTarget Snapshot, deniedUntracked string, deniedErr error, finalTarget Snapshot, finalUntracked string, finalErr error) bool {
	if deniedErr != nil || finalErr != nil {
		return deniedErr != nil && finalErr != nil && deniedErr.Error() == finalErr.Error()
	}
	return snapshotsEqual(finalTarget, deniedTarget) && finalUntracked == deniedUntracked
}

func compactInvalidationDenialBound(evaluation NativeGateEvaluation, current CompactRecord, request CompactApprovedInvalidationRequest) bool {
	context := evaluation.Context
	return evaluation.Result == GateInvalidated && evaluation.Cause == nil && context.Denial != nil &&
		context.Gate == request.Gate.Gate && context.LineageID == current.State.LineageID &&
		context.Generation == current.State.Generation && context.StoreRevision == current.Revision &&
		context.BaseTree != "" && context.CandidateTree != "" && context.PathsDigest != "" && context.PolicyHash == current.State.PolicyHash &&
		context.LedgerHash == current.State.LedgerHash() && context.EvidenceHash == current.State.EvidenceHash
}
