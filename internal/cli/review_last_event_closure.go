package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/gentleman-programming/gentle-ai/v2/internal/model"
	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
)

const reviewLastEventClosureSchema = "gentle-ai.review-last-event-closure/v1"

const reviewApprovedLastEventAcknowledgementAction = "the approved review completed on the last admitted event and awaits its exact acknowledgement"

type reviewLastEventClosureResult struct {
	Schema                    string                                              `json:"schema"`
	Operation                 string                                              `json:"operation"`
	LineageID                 string                                              `json:"lineage_id"`
	State                     reviewtransaction.State                             `json:"state"`
	Action                    string                                              `json:"action"`
	TargetedValidatorEvidence *reviewtransaction.CompactTargetedValidatorEvidence `json:"targeted_validator_evidence,omitempty"`
	AdvisoryFindings          *reviewtransaction.AdvisoryFindingSet               `json:"advisory_findings,omitempty"`
	StatusContinuation        *ReviewTransitionExecution                          `json:"status_continuation,omitempty"`
	Acknowledgement           *ReviewTransitionExecution                          `json:"acknowledgement,omitempty"`
	StoreRevision             string                                              `json:"store_revision"`
}

func reviewApprovedAcknowledgementTransition(repo string, acknowledgement reviewtransaction.ApprovedCompactAcknowledgement) *ReviewTransitionExecution {
	return reviewExecuteTransition("approved_acknowledgement_required", "review.acknowledge-approved", []ReviewTransitionArgument{
		{Name: "cwd", Value: repo},
		{Name: "lineage", Value: acknowledgement.LineageID},
		{Name: "target", Value: acknowledgement.TargetIdentity},
		{Name: "expected-revision", Value: acknowledgement.ExpectedRevision},
		{Name: "token", Value: acknowledgement.Token},
	}, []ReviewTransitionArgument{{Name: "state", Value: string(reviewtransaction.StateApproved)}}, ReviewTransitionBinding{
		LineageID: acknowledgement.LineageID, TargetIdentity: acknowledgement.TargetIdentity, Revision: acknowledgement.ExpectedRevision,
	}, nil).Execute
}

// RunReviewAcknowledgeApproved executes the one v2-local acknowledgement
// continuation. It intentionally returns no independent result: ambiguous
// delivery is resolved by rerunning the STATUS transition against authority.
func RunReviewAcknowledgeApproved(args []string, _ io.Writer) error {
	flags := newReviewFlagSet("review acknowledge-approved", io.Discard, "Acknowledge one approved review authority using its exact v2 continuation.")
	cwd := flags.String("cwd", ".", "repository path")
	lineage := flags.String("lineage", "", "exact approved review lineage")
	target := flags.String("target", "", "exact approved target identity")
	expectedRevision := flags.String("expected-revision", "", "exact approved authority revision")
	token := flags.String("token", "", "opaque approved acknowledgement token")
	if err := parseReviewFlags(flags, args); err != nil {
		return err
	}
	if reviewHelpRequested(args) {
		return nil
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("unexpected review acknowledge-approved argument %q", flags.Arg(0)) // refusal:by-design operator-knowledge: run the exact acknowledgement continuation without positional arguments
	}
	if *lineage == "" || *target == "" || *expectedRevision == "" || *token == "" {
		return errors.New("review acknowledge-approved requires --lineage, --target, --expected-revision, and --token") // refusal:by-design operator-knowledge: run the exact v2 acknowledgement continuation emitted by STATUS or terminal closure
	}
	return reviewtransaction.AcknowledgeApprovedCompactAuthority(context.Background(), *cwd, *lineage, *target, *expectedRevision, *token)
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
	var revision string
	var acknowledgement reviewtransaction.ApprovedCompactAcknowledgement
	if state.State == reviewtransaction.StateApproved {
		acknowledgement, err = reviewtransaction.CommitApprovedCompactAcknowledgement(ctx, store, record.Revision, "review/complete-correction-verification", state)
		if err != nil {
			return nil, fmt.Errorf("commit approved review acknowledgement with targeted validator capture: %w", err)
		}
		revision = acknowledgement.ExpectedRevision
	} else {
		revision, err = store.Replace(record.Revision, "review/complete-correction-verification", state)
		if err != nil {
			return nil, err
		}
	}
	result := &reviewLastEventClosureResult{
		Schema: reviewLastEventClosureSchema, Operation: "review/capture-validation",
		LineageID: state.LineageID, State: state.State, StoreRevision: revision,
		AdvisoryFindings: reviewtransaction.AdvisoryFindingSetFor(state),
	}
	switch state.State {
	case reviewtransaction.StateApproved:
		result.Action = reviewApprovedLastEventAcknowledgementAction
		result.Acknowledgement = reviewApprovedAcknowledgementTransition(repo, acknowledgement)
	case reviewtransaction.StateEscalated:
		result.Action = "the targeted validator rejected the correction; maintainer action is informational"
	default:
		return nil, fmt.Errorf("targeted validator capture produced unsupported state %q", state.State) // refusal:by-design human-authority: an unmodeled terminal authority outcome requires maintainer inspection
	}
	return result, nil
}

// newCorrectionCapturedValidatorClosure returns the terminal evidence already
// admitted with a rejected validator capture. It never performs another store
// mutation, so an exact replay returns the same canonical closure.
func newCorrectionCapturedValidatorClosure(repo string, state reviewtransaction.CompactState, revision string, request reviewtransaction.TargetedValidationRequest) (*reviewLastEventClosureResult, error) {
	if err := reviewtransaction.ValidateTargetedValidationRequest(request); err != nil ||
		state.State != reviewtransaction.StateEscalated || len(state.CorrectionAttempts) == 0 {
		return nil, errors.New("targeted validator closure does not bind an escalated correction") // refusal:by-design human-authority: a terminal validator closure without its bound correction requires authority inspection
	}
	attempt := state.CorrectionAttempts[len(state.CorrectionAttempts)-1]
	if request.RequestHash != attempt.TargetedValidationRequestHash || request.CorrectionTargetIdentity != attempt.CorrectionTargetIdentity {
		return nil, errors.New("targeted validator closure request does not bind the terminal correction") // refusal:by-design human-authority: an unbound terminal validator closure requires authority inspection
	}
	evidence, found, err := state.AdmittedTargetedValidatorEvidence(request.ExpectedRevision, request.CorrectionTargetIdentity, request.RequestHash)
	if err != nil || !found {
		return nil, errors.New("targeted validator closure has no canonical rejection evidence") // refusal:by-design human-authority: missing terminal evidence requires authority inspection
	}
	validation := reviewtransaction.ScopedValidationResult{
		LedgerIDs: append([]string(nil), state.FixFindingIDs...), FollowUps: evidence.FollowUps,
		OriginalCriteria: attempt.OriginalCriteria, CorrectionRegression: attempt.CorrectionRegression,
		TargetedValidationRequestHash: attempt.TargetedValidationRequestHash, CorrectionTargetIdentity: attempt.CorrectionTargetIdentity,
	}
	if err := evidence.Validate(request, validation); err != nil || validation.OriginalCriteria.Passed && validation.CorrectionRegression.Passed {
		return nil, errors.New("targeted validator closure evidence does not match its rejected verdict") // refusal:by-design human-authority: inconsistent rejection evidence requires authority inspection
	}
	return &reviewLastEventClosureResult{
		Schema: reviewLastEventClosureSchema, Operation: "review/capture-validation", LineageID: state.LineageID,
		State: state.State, Action: "the targeted validator rejected the correction; maintainer action is informational",
		TargetedValidatorEvidence: &evidence, AdvisoryFindings: reviewtransaction.AdvisoryFindingSetFor(state), StoreRevision: revision,
	}, nil
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
	runtime model.AgentID,
) (*reviewLastEventClosureResult, error) {
	state := record.State
	artifacts, err := discoverCapturedReviewerArtifacts(ctx, repo, store.Dir, state, state.CapturePhaseRevision)
	if err != nil {
		return nil, err
	}
	if len(artifacts) != len(state.SelectedLenses) {
		return nil, nil
	}

	view, _, err := capturedCompactReviewView(ctx, repo, store.Dir, state, state.CapturePhaseRevision)
	if err != nil {
		return nil, err
	}
	input := compactReviewInputFromView(view)
	claims, err := reviewProviderRefuterClaims(state.InitialSnapshot.Identity, input)
	if errors.Is(err, errReviewProviderRefuterNotRequired) {
		claims = nil
	} else if err != nil {
		return nil, err
	}
	if len(claims) > 0 {
		_, readErr := readCapturedProviderRefuterResult(ctx, repo, store.Dir, state, state.CapturePhaseRevision)
		if errors.Is(readErr, errReviewProviderRefuterResultNotCaptured) {
			if !reviewProviderCaptureRuntime(runtime) {
				return nil, nil
			}
			if _, captured, err := reviewProviderCaptureRefuter(ctx, repo, store, state, state.CapturePhaseRevision, runtime); err != nil {
				return nil, err
			} else if !captured {
				return nil, errors.New("compiled provider refuter was required but no result was captured; rerun `gentle-ai review status --cwd <repo> --contract gentle-ai.review-integration/v2 --next-transition` and follow its capture route")
			}
			current, err := store.LoadContext(ctx)
			if err != nil {
				return nil, err
			}
			return closeReviewOnLastCapturedLens(ctx, repo, store, current, runtime)
		}
		if readErr != nil {
			return nil, readErr
		}
		view, _, err = capturedCompactReviewView(ctx, repo, store.Dir, state, state.CapturePhaseRevision)
		if err != nil {
			return nil, err
		}
		input = compactReviewInputFromView(view)
	}

	// Current lifecycle context is ephemeral; historical diagnostics remain read-only.
	state.ReviewerContextLevel = ""
	if err := state.CompleteReview(input); err != nil {
		return nil, err
	}
	if state.State == reviewtransaction.StateValidating {
		if err := state.CloseCleanReviewOnLastEvent(); err != nil {
			return nil, err
		}
	}
	var revision string
	var acknowledgement reviewtransaction.ApprovedCompactAcknowledgement
	if state.State == reviewtransaction.StateApproved {
		acknowledgement, err = reviewtransaction.CommitApprovedCompactAcknowledgement(ctx, store, record.Revision, "review/complete-review", state)
		if err != nil {
			return nil, fmt.Errorf("commit approved review acknowledgement with final lens capture: %w", err)
		}
		revision = acknowledgement.ExpectedRevision
	} else {
		revision, err = store.Replace(record.Revision, "review/complete-review", state)
		if err != nil {
			return nil, err
		}
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
		result.Action = reviewApprovedLastEventAcknowledgementAction
		result.AdvisoryFindings = reviewtransaction.AdvisoryFindingSetFor(state)
		result.Acknowledgement = reviewApprovedAcknowledgementTransition(repo, acknowledgement)
	case reviewtransaction.StateCorrectionRequired:
		result.Action = "candidate-caused severe findings require one bounded correction"
		result.StatusContinuation = reviewCorrectionStatusContinuation(repo, state, revision, runtime)
		if result.StatusContinuation == nil {
			return nil, fmt.Errorf("correction-required review has unsupported initial target kind %q", state.InitialSnapshot.Kind) // refusal:by-design human-authority: only a recognized frozen selector may reopen correction planning
		}
	case reviewtransaction.StateEscalated:
		result.Action = "review completed with inconclusive severe findings; maintainer action is informational"
	default:
		return nil, fmt.Errorf("last reviewer capture produced unsupported state %q", state.State) // refusal:by-design human-authority: an unmodeled terminal authority outcome requires maintainer inspection
	}
	return result, nil
}

// reviewCorrectionStatusContinuation is the one provider-owned re-entry after a
// final reviewer event opened the bounded correction. It uses frozen authority
// facts rather than a caller's remembered selector spelling.
func reviewCorrectionStatusContinuation(repo string, state reviewtransaction.CompactState, revision string, runtime model.AgentID) *ReviewTransitionExecution {
	arguments := []ReviewTransitionArgument{
		{Name: "cwd", Value: repo},
		{Name: "contract", Value: ReviewIntegrationContractV2},
		{Name: "next-transition", Value: "true"},
		{Name: "lineage", Value: state.LineageID},
	}
	if runtime != "" {
		arguments = append(arguments, ReviewTransitionArgument{Name: "agent", Value: string(runtime)})
	}
	switch state.InitialSnapshot.Kind {
	case reviewtransaction.TargetBaseDiff:
		arguments = append(arguments,
			ReviewTransitionArgument{Name: "base-ref", Value: state.InitialSnapshot.BaseTree},
			ReviewTransitionArgument{Name: "committed-only", Value: "true"},
		)
	case reviewtransaction.TargetCurrentChanges:
		if state.InitialSnapshot.Projection == reviewtransaction.ProjectionStaged {
			arguments = append(arguments, ReviewTransitionArgument{Name: "projection", Value: string(reviewtransaction.ProjectionStaged)})
		}
	case reviewtransaction.TargetBaseWorkspaceOverlay:
		arguments = append(arguments,
			ReviewTransitionArgument{Name: "base-ref", Value: state.InitialSnapshot.BaseTree},
			ReviewTransitionArgument{Name: "workspace-overlay", Value: "true"},
		)
		if state.InitialSnapshot.Projection == reviewtransaction.ProjectionStaged {
			arguments = append(arguments, ReviewTransitionArgument{Name: "projection", Value: string(reviewtransaction.ProjectionStaged)})
		}
	default:
		return nil
	}
	return reviewExecuteTransition("correction_status_required", "review.status", arguments,
		[]ReviewTransitionArgument{{Name: "state", Value: string(reviewtransaction.StateCorrectionRequired)}},
		ReviewTransitionBinding{LineageID: state.LineageID, Revision: revision, TargetIdentity: state.InitialSnapshot.Identity}, nil,
	).Execute
}
