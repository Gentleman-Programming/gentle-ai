package cli

import (
	"context"
	"errors"
	"io"
	"strings"

	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
)

const reviewCaptureCorrectionPlanOperation = "review.capture-correction-plan"

type reviewCorrectionPlanCaptureResult struct {
	Schema          string                  `json:"schema"`
	Operation       string                  `json:"operation"`
	LineageID       string                  `json:"lineage_id"`
	TargetIdentity  string                  `json:"target_identity"`
	RequestHash     string                  `json:"request_hash"`
	CorrectionLines int                     `json:"correction_lines"`
	State           reviewtransaction.State `json:"state"`
	StoreRevision   string                  `json:"store_revision"`
}

// RunReviewCaptureCorrectionPlan records the one immutable, positive correction
// forecast before the user edits. It is the only public event that advances a
// correction_required authority into an open bounded correction.
func RunReviewCaptureCorrectionPlan(args []string, stdout io.Writer) error {
	flags := newReviewFlagSet("review capture-correction-plan", stdout, "Capture the exact bounded correction forecast for one correction-required review.")
	cwd := flags.String("cwd", ".", "repository path")
	repositoryContext := flags.String("repository-context", "", "opaque provider-issued repository context; supplied by the collect transition and verified against --cwd")
	lineage := flags.String("lineage", "", "exact review lineage identifier")
	target := flags.String("target", "", "exact provider-issued review target identity")
	revision := flags.String("expected-revision", "", "exact compact authority revision")
	requestHash := flags.String("request-hash", "", "provider-issued correction plan request hash")
	correctionLines := flags.Int("correction-lines", 0, "positive pre-edit correction line forecast")
	if err := parseReviewFlags(flags, args); err != nil {
		return err
	}
	if reviewHelpRequested(args) {
		return nil
	}
	if flags.NArg() != 0 || strings.TrimSpace(*lineage) == "" || strings.TrimSpace(*target) == "" ||
		strings.TrimSpace(*revision) == "" || strings.TrimSpace(*requestHash) == "" || *correctionLines <= 0 {
		return reviewPreflightError(errors.New("review capture-correction-plan requires --lineage, --target, --expected-revision, --request-hash, and positive --correction-lines")) // refusal:by-design operator-knowledge: STATUS provides the exact correction-plan binding and the user supplies a positive forecast
	}
	contextHandle := strings.TrimSpace(*repositoryContext)
	ctx := context.Background()
	var root string
	var err error
	if contextHandle != "" {
		root, err = resolveOpaqueReviewRepositoryRoot(ctx, *cwd, contextHandle, reviewtransaction.ReviewRepositoryContextBinding{
			LineageID: strings.TrimSpace(*lineage), TargetIdentity: strings.TrimSpace(*target), Revision: strings.TrimSpace(*revision),
		})
	} else {
		root, err = resolveReviewMutationRoot(ctx, *cwd)
	}
	if err != nil {
		return err
	}
	if contextHandle != "" {
		if err := authorizeReviewAuthorityMutation(ctx, root); err != nil {
			return err
		}
	}
	store, record, err := discoverCompactFacadeReview(ctx, root, strings.TrimSpace(*lineage), false)
	if err != nil {
		if contextHandle != "" {
			return reviewOpaqueContextCause("repository_context_authority_unavailable", "refresh the exact native next_transition before retrying", err)
		}
		return reviewPreflightError(err)
	}
	if record.State.CapturePhaseRevision != strings.TrimSpace(*revision) || record.State.State != reviewtransaction.StateCorrectionRequired ||
		record.State.CurrentSnapshot.Identity != strings.TrimSpace(*target) {
		return reviewPreflightRefusal(reviewPreflightCaptureBindingMismatchReason, errors.New("correction-plan capture binding does not match the current correction authority; rerun `gentle-ai review status --cwd <repo> --contract gentle-ai.review-integration/v2 --next-transition` before retrying"))
	}
	request, err := reviewtransaction.BuildCorrectionPlanRequest(record.State, record.State.CapturePhaseRevision)
	if err != nil {
		return reviewPreflightRefusal(reviewPreflightCaptureBindingMismatchReason, err)
	}
	if request.RequestHash != strings.TrimSpace(*requestHash) || request.TargetIdentity != strings.TrimSpace(*target) {
		return reviewPreflightRefusal(reviewPreflightCaptureBindingMismatchReason, errors.New("correction-plan capture request does not match current authority; rerun `gentle-ai review status --cwd <repo> --contract gentle-ai.review-integration/v2 --next-transition` before retrying"))
	}
	state := record.State
	if err := state.BeginCorrection(*correctionLines); err != nil {
		// The forecast is pre-edit caller input, and BeginCorrection refuses it
		// before recording a proposal or advancing the capture phase, so the
		// store is provably untouched here. Falling through untyped would earn
		// the generic operation_outcome_unknown envelope: a false claim that the
		// outcome is unknown, which sends the operator to recovery instead of to
		// a smaller forecast.
		//
		// The budget overrun is the only refusal that reaches this line. Every
		// other BeginCorrection precondition is already excluded above: a
		// non-positive forecast by the flag check, a wrong state, revision or
		// target by the binding check, and a proposal already recorded by the
		// revision it advanced. The typing is still written for the branch
		// rather than for the one error, because all of them describe the same
		// fact -- the command returns before store.Replace, so none of them
		// started.
		return reviewPreflightError(err)
	}
	nextRevision, err := store.Replace(record.Revision, "review/begin-fix", state)
	if err != nil {
		return err
	}
	return encodeReviewJSON(stdout, reviewCorrectionPlanCaptureResult{
		Schema: reviewLastEventClosureSchema, Operation: reviewCaptureCorrectionPlanOperation,
		LineageID: state.LineageID, TargetIdentity: request.TargetIdentity, RequestHash: request.RequestHash,
		CorrectionLines: *correctionLines, State: state.State, StoreRevision: nextRevision,
	})
}
