package cli

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
)

// ReviewFacadeStartNewLineageResult is the v3 activation branch's own START
// response shape (Wave 3 Slice 3, design decision 5). It is deliberately not
// ReviewFacadeStartResult: that type's State field is the legacy Mode-keyed
// reviewtransaction.State, which has no member for NewLineageState's
// five-value vocabulary, and reusing it would either fabricate a legacy
// state or silently drop the new one.
type ReviewFacadeStartNewLineageResult struct {
	Operation        string                            `json:"operation"`
	LineageID        string                            `json:"lineage_id"`
	State            reviewtransaction.NewLineageState `json:"state"`
	RiskLevel        reviewtransaction.RiskLevel       `json:"risk_level"`
	SelectedLenses   []string                          `json:"selected_lenses"`
	CorrectionBudget int                               `json:"correction_budget"`
	ChangedLines     int                               `json:"changed_lines"`
	TargetIdentity   string                            `json:"target_identity"`
	BaseTree         string                            `json:"base_tree"`
	CandidateTree    string                            `json:"candidate_tree"`
}

// runReviewFacadeStartNewLineage is the ON side of the Wave 3 Slice 3
// activation branch: ReviewCore.Next decides the frozen authority,
// AuthorityStore.Mutate is the only thing that writes it (v3/, Wave 3 Slice
// 2), and nothing here re-derives tier, lenses, consent, or budget — every
// one of those was already computed by runReviewFacadeStart's shared
// preamble before this branch is reached (authorizeReviewStart already
// returned nil, so consent was granted, or tier 0's carve-out applied).
func runReviewFacadeStartNewLineage(
	ctx context.Context, stdout io.Writer, root, explicitLineage, policySource string,
	snapshot reviewtransaction.Snapshot, assessment reviewtransaction.RiskAssessment,
	changedLines int, lenses []string,
) error {
	lineage := explicitLineage
	if lineage == "" {
		lineage = reviewDerivedStartLineage(snapshot.Identity)
	}
	if lineage == "" {
		return errors.New("review start new-lineage activation requires a derivable or explicit lineage identifier") // refusal:by-design world-action: unreachable in practice -- reviewDerivedStartLineage only returns "" for a target identity digest shorter than reviewDerivedStartLineageDigits, and every resolved Snapshot.Identity is a full sha256 digest; reaching here is a caller-supplied malformed identity, fixed by the caller's construction, not an operator command
	}
	policy, err := facadePolicyBytes(policySource)
	if err != nil {
		return err
	}
	identity, err := reviewtransaction.FreezeCandidateIdentity(ctx, root, snapshot, facadePayloadHash(policy))
	if err != nil {
		return fmt.Errorf("freeze new-lineage candidate identity: %w", err)
	}
	core := reviewtransaction.ReviewCore{}
	transition, err := core.Next(ctx, reviewtransaction.NewLineageAuthority{}, reviewtransaction.CoreRequest{
		Kind: reviewtransaction.CoreRequestStart, CandidateIdentity: identity, Tier: assessment.Level,
		SelectedLenses: lenses, OriginalChangedLines: changedLines, ConsentGranted: true,
	})
	if err != nil {
		return fmt.Errorf("review core start: %w", err)
	}
	if transition.Kind != reviewtransaction.CoreTransitionContinue || transition.Authority == nil {
		return fmt.Errorf("review core start returned unexpected transition kind %q", transition.Kind) // refusal:by-design world-action: ReviewCore.Next's own start() always returns CoreTransitionContinue with a non-nil Authority on success; anything else reaching here is a bug in review_core.go itself, fixed by editing that code, not an operator command
	}
	store, err := reviewtransaction.NewLineageAuthorityStore(ctx, root, lineage)
	if err != nil {
		return err
	}
	if _, err := store.Mutate(ctx, "", func(next *reviewtransaction.NewLineageAuthority) error {
		*next = *transition.Authority
		next.LineageID = lineage
		return next.RecordTransition(snapshot.Identity, nil)
	}); err != nil {
		return fmt.Errorf("freeze new-lineage authority: %w", err)
	}
	record, err := store.Load()
	if err != nil {
		return err
	}
	result := ReviewFacadeStartNewLineageResult{
		Operation: "review/start", LineageID: lineage, State: record.Authority.State,
		RiskLevel: record.Authority.Tier, SelectedLenses: append([]string{}, record.Authority.SelectedLenses...),
		CorrectionBudget: record.Authority.CorrectionBudgetLines, ChangedLines: changedLines,
		TargetIdentity: snapshot.Identity, BaseTree: snapshot.BaseTree, CandidateTree: snapshot.CandidateTree,
	}
	return encodeReviewJSON(stdout, result)
}
