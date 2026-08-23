package cli

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/gentleman-programming/gentle-ai/v2/internal/model"
	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
)

const reviewLastEventClosureSchema = "gentle-ai.review-last-event-closure/v1"

const reviewApprovedLastEventBurnedAction = "the approved review completed on the last admitted event and burned; delivery follows ordinary repository policy"

type reviewLastEventClosureResult struct {
	Schema           string                                `json:"schema"`
	Operation        string                                `json:"operation"`
	LineageID        string                                `json:"lineage_id"`
	State            reviewtransaction.State               `json:"state"`
	Action           string                                `json:"action"`
	AdvisoryFindings *reviewtransaction.AdvisoryFindingSet `json:"advisory_findings,omitempty"`
	StoreRevision    string                                `json:"store_revision"`
}

func closeCorrectionOnCapturedValidator(
	ctx context.Context,
	repo string,
	store reviewtransaction.CompactStore,
	record reviewtransaction.CompactRecord,
	fix reviewtransaction.Snapshot,
	request reviewtransaction.TargetedValidationRequest,
	validation reviewtransaction.ScopedValidationResult,
) (*reviewLastEventClosureResult, error) {
	state := record.State
	if err := reviewtransaction.ValidateTargetedValidationRequest(request); err != nil ||
		validation.TargetedValidationRequestHash != request.RequestHash ||
		validation.CorrectionTargetIdentity != request.CorrectionTargetIdentity {
		return nil, fmt.Errorf("targeted validator result does not bind the correction request") // refusal:by-design operator-knowledge: only the exact provider-issued request can close its bound correction
	}
	actual, err := (reviewtransaction.SnapshotBuilder{Repo: repo}).ChangedLines(ctx, fix)
	if err != nil {
		return nil, err
	}
	complete, err := (reviewtransaction.SnapshotBuilder{Repo: repo}).BuildCorrectedCandidate(ctx, state.InitialSnapshot, fix)
	if err != nil {
		return nil, err
	}
	if err := state.CompleteCorrectionVerification(fix, actual, validation, complete); err != nil {
		return nil, err
	}
	revision, err := store.Replace(record.Revision, "review/complete-correction-verification", state)
	if err != nil {
		return nil, err
	}
	result := &reviewLastEventClosureResult{
		Schema: reviewLastEventClosureSchema, Operation: "review/capture-validation",
		LineageID: state.LineageID, State: state.State, StoreRevision: revision,
		AdvisoryFindings: reviewtransaction.AdvisoryFindingSetFor(state),
	}
	switch state.State {
	case reviewtransaction.StateApproved:
		result.Action = reviewApprovedLastEventBurnedAction
		if err := reviewtransaction.BurnApprovedCompactAuthority(ctx, repo, state.LineageID, revision); err != nil {
			return nil, fmt.Errorf("burn approved review after targeted validator capture: %w", err)
		}
	case reviewtransaction.StateEscalated:
		result.Action = "the targeted validator rejected the correction; maintainer action is informational"
	default:
		return nil, fmt.Errorf("targeted validator capture produced unsupported state %q", state.State) // refusal:by-design human-authority: an unmodeled terminal authority outcome requires maintainer inspection
	}
	return result, nil
}

// reviewLastCapturedLensClosureSuperseded recognizes a sibling capture that
// completed the one terminal transition after this call durably admitted its
// own lens result. That admitted call may still return its ordinary capture
// acknowledgement; it must not report a stale revision as a capture failure.
func reviewLastCapturedLensClosureSuperseded(store reviewtransaction.CompactStore, record reviewtransaction.CompactRecord) bool {
	current, err := store.Load()
	if errors.Is(err, os.ErrNotExist) {
		return true
	}
	return err == nil && current.Revision != record.Revision && current.State.State != reviewtransaction.StateReviewing
}

func closeReviewOnLastCapturedLens(
	ctx context.Context,
	repo string,
	store reviewtransaction.CompactStore,
	record reviewtransaction.CompactRecord,
	runtime ...model.AgentID,
) (*reviewLastEventClosureResult, error) {
	state := record.State
	artifacts, err := discoverCapturedReviewerArtifacts(ctx, repo, store.Dir, state, record.Revision)
	if errors.Is(err, reviewtransaction.ErrCompactRoleResultSlotPartiallyPublished) {
		// A sibling capture is still publishing its immutable sidecar pair. This
		// call already admitted its own lens result, so return its nonterminal
		// acknowledgement and let the later stable capture elect the closer.
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if len(artifacts) != len(state.SelectedLenses) {
		return nil, nil
	}

	results, err := readCapturedReviewerResults(ctx, repo, store.Dir, state, record.Revision)
	if err != nil {
		return nil, err
	}
	input, err := prepareCompactReviewerResults(state, results, facadeRefuterResult{}, facadeRepositoryEvidence{ctx: ctx, repo: repo})
	if err != nil {
		return nil, err
	}
	claims, err := reviewProviderRefuterClaims(state.InitialSnapshot.Identity, input)
	if errors.Is(err, errReviewProviderRefuterNotRequired) {
		claims = nil
	} else if err != nil {
		return nil, err
	}
	if len(claims) > 0 {
		slot, err := reviewtransaction.ReadCompactRefuterResultSlot(store.Dir)
		if err != nil {
			return nil, err
		}
		if !slot.Occupied {
			if len(runtime) == 1 && reviewProviderCaptureRuntime(runtime[0]) {
				if _, captured, err := reviewProviderCaptureRefuter(ctx, repo, store, state, record.Revision, runtime[0]); err != nil {
					return nil, err
				} else if !captured {
					return nil, errors.New("compiled provider refuter was required but no result was captured; rerun `gentle-ai review status --cwd <repo> --contract gentle-ai.review-integration/v2 --next-transition` and follow its capture route")
				}
			} else {
				return nil, nil
			}
		}
		refuter, err := readCapturedProviderRefuterResult(ctx, repo, store.Dir, state, record.Revision)
		if err != nil {
			return nil, err
		}
		input, err = prepareCompactReviewerResults(state, results, refuter, facadeRepositoryEvidence{ctx: ctx, repo: repo})
		if err != nil {
			return nil, err
		}
	}

	state.ReviewerContextLevel = discoverReviewerContextLevel(ctx, repo, store.Dir, state, record.Revision)
	if err := state.CompleteReview(input); err != nil {
		return nil, err
	}
	if state.State == reviewtransaction.StateValidating {
		if err := state.CloseCleanReviewOnLastEvent(); err != nil {
			return nil, err
		}
	}
	revision, err := store.Replace(record.Revision, "review/complete-review", state)
	if err != nil {
		return nil, err
	}

	result := &reviewLastEventClosureResult{
		Schema:        reviewLastEventClosureSchema,
		Operation:     "review/capture-result",
		LineageID:     state.LineageID,
		State:         state.State,
		StoreRevision: revision,
	}
	switch state.State {
	case reviewtransaction.StateApproved:
		result.Action = reviewApprovedLastEventBurnedAction
		result.AdvisoryFindings = reviewtransaction.AdvisoryFindingSetFor(state)
		if err := reviewtransaction.BurnApprovedCompactAuthority(ctx, repo, state.LineageID, revision); err != nil {
			return nil, fmt.Errorf("burn approved review after final lens capture: %w", err)
		}
	case reviewtransaction.StateCorrectionRequired:
		result.Action = "candidate-caused severe findings require one bounded correction"
	case reviewtransaction.StateEscalated:
		result.Action = "review completed with inconclusive severe findings; maintainer action is informational"
	default:
		return nil, fmt.Errorf("last reviewer capture produced unsupported state %q", state.State) // refusal:by-design human-authority: an unmodeled terminal authority outcome requires maintainer inspection
	}
	return result, nil
}
