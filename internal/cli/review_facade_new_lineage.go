package cli

import (
	"context"
	"fmt"

	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
)

// prepareReviewFacadeCompactAtomicStart freezes compact state and its immutable
// worktree-bound START binding without opening authority storage. The caller
// performs context preflight and live snapshot revalidation before the exact
// CompactStore.CreateOrReplayAtomicStart boundary.
func prepareReviewFacadeCompactAtomicStart(
	ctx context.Context, root, explicitLineage, policySource string,
	target reviewtransaction.Target, snapshot reviewtransaction.Snapshot,
	assessment reviewtransaction.RiskAssessment, changedLines int, lenses []string,
) (reviewtransaction.CompactAtomicStartRequest, error) {
	lineage := explicitLineage
	if lineage == "" {
		derived, err := reviewAtomicStartLineage(ctx, root, snapshot.Identity)
		if err != nil {
			return reviewtransaction.CompactAtomicStartRequest{}, fmt.Errorf("derive compact atomic START lineage: %w", err)
		}
		lineage = derived
	}
	policy, err := facadePolicyBytes(policySource)
	if err != nil {
		return reviewtransaction.CompactAtomicStartRequest{}, err
	}
	policyHash := facadePayloadHash(policy)
	policyContent := string(policy)
	state, err := reviewtransaction.NewCompactState(reviewtransaction.Start{
		LineageID: lineage, Mode: reviewtransaction.ModeOrdinaryBounded, Generation: 1,
		Snapshot: snapshot, PolicyHash: policyHash, PolicyContent: &policyContent, RiskLevel: assessment.Level,
		SelectedLenses: append([]string(nil), lenses...), OriginalChangedLines: &changedLines,
	})
	if err != nil {
		return reviewtransaction.CompactAtomicStartRequest{}, fmt.Errorf("build compact atomic START state: %w", err)
	}
	lease, err := reviewtransaction.OpenRepositoryIdentityLease(ctx, root)
	if err != nil {
		return reviewtransaction.CompactAtomicStartRequest{}, fmt.Errorf("resolve compact atomic START worktree: %w", err)
	}
	selector := target
	if selector.Kind == reviewtransaction.TargetBaseDiff || selector.Kind == reviewtransaction.TargetBaseWorkspaceOverlay {
		selector.BaseRef = snapshot.BaseTree
	}
	if selector.Projection == reviewtransaction.ProjectionWorkspace {
		selector.Projection = ""
	}
	binding := reviewtransaction.CompactAtomicStartBinding{
		LineageID: lineage, WorktreeIdentity: lease.Identity().RepositoryRef,
		TargetIdentity: snapshot.Identity, Selector: selector, PolicyHash: policyHash,
		Tier: assessment.Level, SelectedLenses: append([]string(nil), lenses...),
		OriginalChangedLines: changedLines, CorrectionBudget: state.CorrectionBudget,
		CorrectionBudgetPolicy: state.CorrectionBudgetPolicy,
	}
	if err := binding.Validate(); err != nil {
		return reviewtransaction.CompactAtomicStartRequest{}, fmt.Errorf("validate compact atomic START binding: %w", err)
	}
	return reviewtransaction.CompactAtomicStartRequest{State: state, Binding: binding}, nil
}

// runReviewFacadeCompactAtomicStart is the only shipped START write boundary.
// It operates on the exact compact lineage only; discovery, recovery, reuse,
// stale-burn, legacy START, and v3 creation are intentionally absent.
func runReviewFacadeCompactAtomicStart(ctx context.Context, root string, request reviewtransaction.CompactAtomicStartRequest) (reviewtransaction.CompactAtomicStartResult, error) {
	store, err := reviewtransaction.CompactAuthoritativeStore(ctx, root, request.Binding.LineageID)
	if err != nil {
		return reviewtransaction.CompactAtomicStartResult{}, err
	}
	return store.CreateOrReplayAtomicStart(ctx, request)
}
