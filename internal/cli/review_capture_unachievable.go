package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
)

// reviewCaptureUnachievableCaptureOperation names the native capture verb a
// host uses to report that a bound selected lens slot could not be completed
// under current conditions (issue #3442) -- for example a reviewer prompt
// exceeding a host relay's transport bound. It is the single wording source
// the runnable CLI verb derives from, exactly like the other capture
// operations declared beside it.
const reviewCaptureUnachievableCaptureOperation = "review.capture-unachievable"

const reviewUnachievableLensCaptureSchema = "gentle-ai.review-capture-unachievable/v1"

// reviewUnachievableLensDetailLimit bounds the free-text evidence a host may
// attach. It is diagnostic context, not a place to smuggle arbitrary payload
// through the compact authority store.
const reviewUnachievableLensDetailLimit = 512

type reviewUnachievableLensCaptureArtifact struct {
	Schema         string `json:"schema"`
	LineageID      string `json:"lineage_id"`
	TargetIdentity string `json:"target_identity"`
	Lens           string `json:"lens"`
	SelectedOrder  int    `json:"selected_order"`
	Reason         string `json:"reason"`
	Recorded       bool   `json:"recorded"`
}

const reviewUnachievableLensWithdrawalSchema = "gentle-ai.review-capture-unachievable-withdrawal/v1"

type reviewUnachievableLensWithdrawalArtifact struct {
	Schema         string `json:"schema"`
	LineageID      string `json:"lineage_id"`
	TargetIdentity string `json:"target_identity"`
	Withdrawn      bool   `json:"withdrawn"`
}

// RunReviewCaptureUnachievable persists one bound declaration that a
// selected lens slot cannot be completed under current conditions, or --
// with --withdraw=true -- retracts that exact declaration (issue #3442's
// resilience gap: a host that mistook a transient failure, such as a host
// relay transport bound, for a deterministic one had no way back into the
// same lineage). Both forms bind to the exact lineage, revision, target,
// and SubjectHash the negotiated collect transition already offered for
// that slot (rendered as its `subject-hash` argument), exactly like every
// other capture verb -- so a host cannot skip a lens or fabricate a
// result, only report that the one slot it was actually asked to fill
// could not be filled, or take that report back. Declaring stops a
// restarted STATUS negotiation from re-offering the slot (a typed, named
// stop instead); withdrawing makes STATUS re-offer it again.
func RunReviewCaptureUnachievable(args []string, stdout io.Writer) error {
	flags := newReviewFlagSet("review capture-unachievable", stdout, "Declare one selected lens slot unachievable under current conditions, or retract that declaration with --withdraw=true, bound to the exact frozen request the collect transition offered for it.")
	cwd := flags.String("cwd", ".", "repository path")
	repositoryContext := flags.String("repository-context", "", "opaque provider-issued repository context; supplied by the collect transition and verified against --cwd")
	lineage := flags.String("lineage", "", "exact review lineage identifier")
	target := flags.String("target", "", "exact frozen target identity")
	revision := flags.String("expected-revision", "", "exact reviewing authority revision")
	requestHash := flags.String("request-hash", "", "exact subject-hash the collect transition offered for the declared or withdrawn slot")
	reason := flags.String("reason", "", "short machine-readable cause, e.g. relay_transport_bound_exceeded; required unless --withdraw is set")
	detail := flags.String("detail", "", "optional bounded evidence, e.g. elapsed and limit; refused together with --withdraw")
	withdraw := flags.Bool("withdraw", false, "retract a previously recorded unachievable declaration for the exact --request-hash instead of recording a new one")
	if err := parseReviewFlags(flags, args); err != nil {
		return err
	}
	if reviewHelpRequested(args) {
		return nil
	}
	if flags.NArg() != 0 || strings.TrimSpace(*lineage) == "" || strings.TrimSpace(*target) == "" ||
		strings.TrimSpace(*revision) == "" || strings.TrimSpace(*requestHash) == "" {
		return reviewPreflightError(fmt.Errorf("review capture-unachievable requires --lineage, --target, --expected-revision, and --request-hash; `gentle-ai review status --contract %s --next-transition` prints the exact bindings", ReviewIntegrationContractV2))
	}
	if *withdraw {
		if strings.TrimSpace(*reason) != "" || strings.TrimSpace(*detail) != "" {
			return reviewPreflightError(errors.New("review capture-unachievable --withdraw=true cannot be combined with --reason or --detail: a withdrawal retracts the exact recorded declaration and carries no new evidence")) // refusal:by-design world-action: a withdrawal names no new fact about the candidate, only that the earlier one no longer applies
		}
	} else if strings.TrimSpace(*reason) == "" {
		return reviewPreflightError(fmt.Errorf("review capture-unachievable requires --reason unless --withdraw=true is set; `gentle-ai review status --contract %s --next-transition` prints the exact bindings", ReviewIntegrationContractV2))
	}
	if len(strings.TrimSpace(*detail)) > reviewUnachievableLensDetailLimit {
		return reviewPreflightError(fmt.Errorf("review capture-unachievable --detail exceeds %d bytes", reviewUnachievableLensDetailLimit)) // refusal:by-design operator-knowledge: only the caller knows what evidence it intended to attach; the product only enforces the bound
	}
	contextHandle := strings.TrimSpace(*repositoryContext)
	ctx := context.Background()
	var root string
	var err error
	if contextHandle != "" {
		root, err = resolveOpaqueReviewRepositoryRoot(ctx, *cwd, contextHandle, reviewtransaction.ReviewRepositoryContextBinding{
			LineageID: *lineage, TargetIdentity: *target, Revision: *revision,
		})
		if err != nil {
			return err
		}
	} else {
		root, err = (reviewtransaction.SnapshotBuilder{Repo: *cwd}).ResolveRepositoryRoot(ctx)
		if err != nil {
			return fmt.Errorf("resolve review repository root: %w", err)
		}
	}
	if err := authorizeReviewAuthorityMutation(ctx, root); err != nil {
		return err
	}
	store, record, err := discoverCompactFacadeReview(ctx, root, *lineage, false)
	if err != nil {
		if contextHandle != "" {
			return reviewOpaqueContextCause("repository_context_authority_unavailable", "refresh the exact native next_transition before retrying", err)
		}
		return reviewPreflightError(fmt.Errorf("resolve reviewing authority for lineage %q under repository %q: %w", *lineage, root, err))
	}
	state := record.State
	if state.State != reviewtransaction.StateReviewing || state.LineageID != *lineage || state.InitialSnapshot.Identity != *target ||
		state.CapturePhaseRevision != *revision {
		if contextHandle != "" {
			return reviewPreflightRefusal(reviewPreflightCaptureBindingMismatchReason, fmt.Errorf("capture-unachievable binding does not match the current reviewing authority under the provider-issued repository context; ask the parent orchestrator to refresh the exact native next transition by running %s", reviewNextTransitionRefreshCommandV21))
		}
		return reviewPreflightRefusal(reviewPreflightCaptureBindingMismatchReason, fmt.Errorf("capture-unachievable binding does not match the current reviewing authority under repository %q; verify the frozen lineage, target, and revision by running gentle-ai review status --cwd %s --contract %s --next-transition, or re-run with --cwd set to the repository where the review was started", root, root, ReviewIntegrationContractV2))
	}
	if *withdraw {
		removed, err := store.WithdrawUnachievableLensAttempt(ctx, reviewtransaction.CompactUnachievableLensWithdrawalRequest{
			ExpectedRevision: state.CapturePhaseRevision, TargetIdentity: state.InitialSnapshot.Identity, SubjectHash: *requestHash,
		})
		if err != nil {
			return reviewPreflightError(err)
		}
		// Both outcomes are success: removed=true mutated the authority,
		// removed=false found nothing to retract (already withdrawn, or
		// never declared). Either way the guarantee this call promises
		// already holds -- no declaration with this SubjectHash blocks the
		// negotiation -- so this is not an error, only an honest report of
		// whether a mutation happened.
		return encodeReviewJSON(stdout, reviewUnachievableLensWithdrawalArtifact{
			Schema: reviewUnachievableLensWithdrawalSchema, LineageID: state.LineageID,
			TargetIdentity: state.InitialSnapshot.Identity, Withdrawn: removed,
		})
	}
	frozen, err := (reviewtransaction.SnapshotBuilder{Repo: root}).FrozenCandidateContext(ctx, state.InitialSnapshot)
	if err != nil {
		return reviewPreflightError(fmt.Errorf("derive reviewer artifact subject: %w", err))
	}
	lens, order, found := "", -1, false
	for candidateOrder, candidateLens := range state.SelectedLenses {
		if _, alreadyCaptured, lookupErr := state.ActiveAdmittedLensResult(candidateOrder); lookupErr != nil {
			return reviewPreflightError(lookupErr)
		} else if alreadyCaptured {
			continue
		}
		subject, subjectErr := reviewtransaction.NewArtifactSubject(state, state.CapturePhaseRevision, frozen, candidateLens, candidateOrder, "")
		if subjectErr != nil {
			return reviewPreflightError(fmt.Errorf("derive reviewer artifact subject: %w", subjectErr))
		}
		if subject.SubjectHash == *requestHash {
			lens, order, found = candidateLens, candidateOrder, true
			break
		}
	}
	if !found {
		return reviewPreflightRefusal(reviewPreflightCaptureBindingMismatchReason, errors.New("review capture-unachievable request hash does not match any outstanding selected lens slot; refresh the binding with gentle-ai review status --cwd <repo> --contract <same-contract> --next-transition"))
	}
	if _, err := store.RecordUnachievableLensAttempt(ctx, reviewtransaction.CompactUnachievableLensAttemptRequest{
		ExpectedRevision: state.CapturePhaseRevision, TargetIdentity: state.InitialSnapshot.Identity,
		Lens: lens, SelectedOrder: order, SubjectHash: *requestHash,
		Reason: strings.TrimSpace(*reason), Detail: strings.TrimSpace(*detail),
	}); err != nil {
		return reviewPreflightError(err)
	}
	return encodeReviewJSON(stdout, reviewUnachievableLensCaptureArtifact{
		Schema: reviewUnachievableLensCaptureSchema, LineageID: state.LineageID, TargetIdentity: state.InitialSnapshot.Identity,
		Lens: lens, SelectedOrder: order, Reason: strings.TrimSpace(*reason), Recorded: true,
	})
}
