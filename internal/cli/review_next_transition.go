package cli

import (
	"fmt"
	"strings"

	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
)

const (
	reviewNextTransitionExecute = "execute"
	reviewNextTransitionCollect = "collect"
	reviewNextTransitionStop    = "stop"
)

// ReviewNextTransition is the sole negotiated routing decision. Its execute
// form is complete, its collect form identifies one externally supplied input,
// and its stop form intentionally contains no command-shaped data.
type ReviewNextTransition struct {
	Kind       string                      `json:"kind"`
	ReasonCode string                      `json:"reason_code"`
	Execute    *ReviewTransitionExecution  `json:"execute,omitempty"`
	Collect    *ReviewTransitionCollection `json:"collect,omitempty"`
}

type ReviewTransitionExecution struct {
	Operation string `json:"operation"`
	// Command is the complete, literally runnable command line for this
	// transition, e.g. "gentle-ai review start --contract=... --target=...".
	// Operation alone is a dotted logical name, so a caller had to already know
	// that "review.start" means "gentle-ai review start" before it could run
	// anything. Operation, Arguments and their Tokens stay byte-identical, so
	// existing consumers never move.
	Command           string                      `json:"command,omitempty"`
	Arguments         []ReviewTransitionArgument  `json:"arguments"`
	SelectorArguments *[]ReviewTransitionArgument `json:"selector_arguments,omitempty"`
	Preconditions     []ReviewTransitionArgument  `json:"preconditions"`
	Binding           ReviewTransitionBinding     `json:"binding"`
	Artifacts         []ReviewTransitionArtifact  `json:"artifacts,omitempty"`
}

type ReviewTransitionCollection struct {
	Inputs []ReviewTransitionInput `json:"inputs"`
}

type ReviewTransitionInput struct {
	Name                string                                        `json:"name"`
	Schema              string                                        `json:"schema"`
	CaptureOperation    string                                        `json:"capture_operation"`
	Arguments           []ReviewTransitionArgument                    `json:"arguments"`
	ArtifactSubject     *reviewtransaction.ArtifactSubject            `json:"artifact_subject,omitempty"`
	CandidateDiff       *reviewtransaction.FrozenCandidateDiff        `json:"candidate_diff,omitempty"`
	ChangedPathManifest *[]reviewtransaction.ChangedPathManifestEntry `json:"changed_path_manifest,omitempty"`
	ValidationRequest   *reviewtransaction.TargetedValidationRequest  `json:"validation_request,omitempty"`
}

type reviewCaptureContext struct {
	FrozenContext    reviewtransaction.FrozenCandidateContext
	ArtifactSubjects []reviewtransaction.ArtifactSubject
}

type ReviewTransitionArgument struct {
	Name  string `json:"name"`
	Value string `json:"value"`
	// Token is the exact, literally executable argv token for this argument
	// (e.g. "--captured-results=true"). It is populated wherever the argument
	// really is argv: on ReviewTransitionExecution.Arguments, and on the
	// Arguments of a ReviewTransitionInput whose CaptureOperation names an
	// operation this product performs (see reviewNativeCaptureVerb). It stays
	// empty on Preconditions, which are assertions rather than argv, on
	// SelectorArguments, which are a normalized echo of arguments already
	// carried, and on the Arguments of an "external.*" capture operation,
	// which are values to hand to whoever performs it somewhere this product
	// does not run. Name/Value stay byte-identical so existing consumers of
	// those two fields never move.
	Token string `json:"token,omitempty"`
}

type ReviewTransitionBinding struct {
	LineageID         string `json:"lineage_id,omitempty"`
	Revision          string `json:"revision,omitempty"`
	TargetIdentity    string `json:"target_identity"`
	RepositoryContext string `json:"repository_context,omitempty"`
}

// ReviewTransitionArtifact deliberately excludes the provider-owned path. The
// native finalize command discovers the immutable captured bytes itself.
type ReviewTransitionArtifact struct {
	Schema            string                                      `json:"schema"`
	Capability        string                                      `json:"capability"`
	SHA256            string                                      `json:"sha256"`
	LineageID         string                                      `json:"lineage_id"`
	TargetIdentity    string                                      `json:"target_identity"`
	Lens              string                                      `json:"lens"`
	SelectedOrder     int                                         `json:"selected_order"`
	SubjectHash       string                                      `json:"subject_hash"`
	AdmissionDecision reviewtransaction.ArtifactAdmissionDecision `json:"admission_decision"`
}

func newReviewNextTransition(status ReviewTargetStatusResult, selectedLenses []string, artifacts []ReviewTransitionArtifact, evidenceAvailable bool, artifactErr error, input reviewNextTransitionInput) ReviewNextTransition {
	if status.Applicability != reviewtransaction.TargetApplicabilityCurrent {
		switch status.Applicability {
		case reviewtransaction.TargetApplicabilityUnrelated:
			if input.Selector != nil && input.Selector.Kind == reviewtransaction.TargetBaseWorkspaceOverlay &&
				input.Selector.Projection == reviewtransaction.ProjectionStaged {
				return reviewStopTransition("staged_workspace_overlay_recovery_unavailable")
			}
			return reviewExecuteTransition("fresh_target_ready", "review.start", reviewStartArguments(status, input.StartLineage), []ReviewTransitionArgument{{Name: "target_identity", Value: status.TargetIdentity}}, ReviewTransitionBinding{LineageID: input.StartLineage, TargetIdentity: status.TargetIdentity}, nil)
		case reviewtransaction.TargetApplicabilityAmbiguous:
			return reviewCollectTransition("lineage_selection_required", ReviewTransitionInput{
				Name: "lineage_selection", Schema: "gentle-ai.review-lineage-selection/v1", CaptureOperation: "external.select_lineage",
				Arguments: append(reviewTargetArguments(status), ReviewTransitionArgument{Name: "candidates", Value: strings.Join(status.Candidates, ",")}),
			})
		case reviewtransaction.TargetApplicabilityCorrupted:
			if status.Repair.Status == reviewtransaction.AuthorityRepairEligible && status.Repair.Candidate != nil {
				return reviewRepairTransition(status, input)
			}
			return reviewStopTransition("corrupted_or_unverifiable_authority")
		default:
			return reviewStopTransition("corrupted_or_unverifiable_authority")
		}
	}
	if status.Authority == nil {
		return reviewStopTransition("missing_authority_binding")
	}
	bindingTarget := status.TargetIdentity
	if status.Action == reviewtransaction.TargetStatusActionRetryFinalVerification || status.Authority.State == reviewtransaction.StateValidating {
		bindingTarget = reviewAuthorityTargetIdentity(status)
	}
	binding := reviewTransitionBinding(status.Authority, bindingTarget, input.RepositoryContext)
	if status.Action == reviewtransaction.TargetStatusActionReconcileFinalize {
		return reviewStopTransition("original_finalize_request_required")
	}
	if status.Action == reviewtransaction.TargetStatusActionRetryFinalVerification {
		return reviewFinalVerificationRetryCollection(status, binding)
	}
	if status.Action == reviewtransaction.TargetStatusActionStop {
		if status.Authority.State == reviewtransaction.StateCorrectionRequired {
			return reviewStopTransition("unchanged_or_unverified_authority")
		}
		return reviewStopTransition("native_stop_required")
	}
	switch status.Authority.State {
	case reviewtransaction.StateReviewing:
		if artifactErr != nil {
			return reviewStopTransition("captured_artifacts_unverifiable")
		}
		if len(artifacts) != len(selectedLenses) {
			return reviewMissingCaptureTransition(binding, selectedLenses, artifacts, input.CaptureContext)
		}
		return reviewExecuteTransition("captured_results_ready", "review.finalize", []ReviewTransitionArgument{
			{Name: "lineage", Value: binding.LineageID}, {Name: "captured_results", Value: "true"},
		}, []ReviewTransitionArgument{{Name: "state", Value: "reviewing"}, {Name: "captured_artifacts", Value: "complete"}}, binding, artifacts)
	case reviewtransaction.StateCorrectionRequired:
		if status.Action == reviewtransaction.TargetStatusActionRecover {
			return reviewRecoveryCollection(status, binding, input)
		}
		if input.ValidationRequest != nil {
			validationBinding := binding
			validationBinding.TargetIdentity = input.ValidationRequest.CorrectionTargetIdentity
			return reviewCollectTransition("targeted_validation_required", ReviewTransitionInput{
				Name: "targeted_validation", Schema: reviewtransaction.TargetedValidationRequestSchema,
				CaptureOperation: "external.run_targeted_validation", Arguments: reviewBindingArguments(validationBinding),
				ValidationRequest: input.ValidationRequest,
			})
		}
		if input.CorrectionForecasted {
			return reviewStopTransition("corrected_candidate_unavailable")
		}
		return reviewCollectTransition("correction_plan_required", ReviewTransitionInput{
			Name: "correction_lines", Schema: "gentle-ai.review-correction-plan/v1", CaptureOperation: "external.plan_correction",
			Arguments: reviewBindingArguments(binding),
		})
	case reviewtransaction.StateValidating:
		if evidenceAvailable {
			return reviewExecuteTransition("captured_verification_evidence_ready", "review.finalize", []ReviewTransitionArgument{{Name: "lineage", Value: binding.LineageID}, {Name: "captured_evidence", Value: "true"}}, []ReviewTransitionArgument{{Name: "state", Value: "validating"}, {Name: "verification_evidence", Value: "captured"}}, binding, nil)
		}
		if status.Frozen != nil && status.Frozen.Tier == reviewtransaction.RiskLow {
			return reviewExecuteTransition("native_low_risk_verification", "review.finalize", []ReviewTransitionArgument{{Name: "lineage", Value: binding.LineageID}}, []ReviewTransitionArgument{{Name: "state", Value: "validating"}, {Name: "risk_level", Value: "low"}}, binding, nil)
		}
		return reviewCollectTransition("verification_evidence_required", ReviewTransitionInput{
			Name: "evidence", Schema: "gentle-ai.review-verification-evidence/v1", CaptureOperation: "review.capture-evidence",
			Arguments: reviewBindingArguments(binding),
		})
	case reviewtransaction.StateInvalidated:
		return reviewRecoveryCollection(status, binding, input)
	case reviewtransaction.StateApproved:
		if status.Action == reviewtransaction.TargetStatusActionRecover {
			return reviewRecoveryCollection(status, binding, input)
		}
		if status.Receipt.Status == ReviewReceiptPresent {
			if input.Selector != nil && input.gate() == reviewtransaction.GatePrePR && !input.Selector.PrePRRepresentable {
				return reviewStopTransition("pre_pr_selector_unrepresentable")
			}
			arguments := []ReviewTransitionArgument{{Name: "lineage", Value: binding.LineageID}, {Name: "gate", Value: string(input.gate())}}
			selectors := []ReviewTransitionArgument{}
			if input.Selector != nil && input.gate() == reviewtransaction.GatePrePR && input.Selector.BaseRef != "" {
				selectors = append(selectors, ReviewTransitionArgument{Name: "base-ref", Value: input.Selector.BaseRef})
				arguments = append(arguments, selectors...)
			}
			transition := reviewExecuteTransition("approved_receipt_ready", "review.validate", arguments, []ReviewTransitionArgument{{Name: "state", Value: "approved"}, {Name: "receipt", Value: "present"}}, binding, nil)
			if input.Selector != nil {
				transition.Execute.SelectorArguments = reviewTransitionSelectorArguments(selectors)
			}
			return transition
		}
		if status.Replayability == reviewtransaction.ReplayabilityExactReplaySafe {
			return reviewExecuteTransition("exact_receipt_replay", "review.finalize", []ReviewTransitionArgument{{Name: "lineage", Value: binding.LineageID}}, []ReviewTransitionArgument{{Name: "state", Value: "approved"}, {Name: "receipt", Value: "publication_pending"}}, binding, nil)
		}
		return reviewCollectTransition("delivery_gate_required", ReviewTransitionInput{
			Name: "gate", Schema: "gentle-ai.review-gate-selection/v1", CaptureOperation: "external.select_gate",
			Arguments: reviewBindingArguments(binding),
		})
	case reviewtransaction.StateEscalated:
		// Escalated recovery is admissible either against a changed target
		// (validateCompactRecoveryEdge's non-evidence RecoveryEscalated
		// branch, requiring compactEscalatedRecoveryTargetChanged) or, for
		// an accounting-only escalation, against the unchanged
		// already-corrected candidate via the evidence-bound branch that
		// RecoverCompactAuthority derives automatically
		// (deriveCompactRecoveredEvidence). Native STATUS
		// (assessTargetStatusSnapshot, target_status.go:176-199) only ever
		// sets Action == TargetStatusActionRecover for StateEscalated when
		// one of those two edges already holds; TargetStatusActionStop is
		// the sole other outcome and is routed away to native_stop_required
		// before this switch is reached (see the TargetStatusActionStop
		// branch above). A target-identity re-check here therefore
		// duplicated a decision status already made, and did so incorrectly
		// for the accounting-only edge: it forced a false
		// "escalated_recovery_requires_changed_target" stop on an
		// already-eligible recovery. Trust status.Action like every other
		// case in this switch; when a Selector is present, the generic
		// recovery_scope_unchanged guard inside reviewRecoveryCollection
		// still applies unchanged.
		status.ActionDisposition = reviewtransaction.RecoveryEscalated
		return reviewRecoveryCollection(status, binding, input)
	default:
		if status.Action == reviewtransaction.TargetStatusActionReconcileFinalize {
			return reviewStopTransition("original_finalize_request_required")
		}
		return reviewStopTransition("manual_intervention_required")
	}
}

type reviewFinalizeTransitionContext struct {
	RepositoryContext string
	ValidationRequest *reviewtransaction.TargetedValidationRequest
	CaptureContext    *reviewCaptureContext
}

func reviewFinalizeNextTransition(state reviewtransaction.CompactState, revision string, artifacts []ReviewTransitionArtifact, artifactErr error, contexts ...reviewFinalizeTransitionContext) ReviewNextTransition {
	status := ReviewTargetStatusResult{
		Applicability:           reviewtransaction.TargetApplicabilityCurrent,
		Authority:               &ReviewTargetStatusAuthority{LineageID: state.LineageID, Revision: revision, State: state.State},
		TargetIdentity:          state.CurrentSnapshot.Identity,
		AuthorityTargetIdentity: state.CurrentSnapshot.Identity,
		Frozen:                  &ReviewTargetStatusFrozen{Tier: state.RiskLevel},
	}
	if state.State == reviewtransaction.StateCorrectionRequired && state.CorrectionAttemptConsumed() {
		status.Action = reviewtransaction.TargetStatusActionStop
		status.Replayability = reviewtransaction.ReplayabilityManualActionRequired
	}
	transitionContext := reviewFinalizeTransitionContext{}
	if len(contexts) > 0 {
		transitionContext = contexts[0]
	}
	if state.State == reviewtransaction.StateReviewing && artifactErr == nil && len(artifacts) != len(state.SelectedLenses) {
		return reviewMissingCaptureTransition(reviewTransitionBinding(status.Authority, status.TargetIdentity, transitionContext.RepositoryContext), state.SelectedLenses, artifacts, transitionContext.CaptureContext)
	}
	if state.State == reviewtransaction.StateReviewing && artifactErr == nil {
		return reviewExecuteTransition("captured_results_ready", "review.finalize", []ReviewTransitionArgument{{Name: "lineage", Value: state.LineageID}, {Name: "captured_results", Value: "true"}}, []ReviewTransitionArgument{{Name: "state", Value: "reviewing"}, {Name: "captured_artifacts", Value: "complete"}}, reviewTransitionBinding(status.Authority, status.TargetIdentity), artifacts)
	}
	return newReviewNextTransition(status, state.SelectedLenses, artifacts, false, artifactErr, reviewNextTransitionInput{
		RepositoryContext: transitionContext.RepositoryContext, ValidationRequest: transitionContext.ValidationRequest,
		CorrectionForecasted: state.ProposedCorrectionLines != nil, CaptureContext: transitionContext.CaptureContext,
	})
}

func reviewMissingCaptureTransition(binding ReviewTransitionBinding, selectedLenses []string, artifacts []ReviewTransitionArtifact, context *reviewCaptureContext) ReviewNextTransition {
	captured := make(map[int]bool, len(artifacts))
	for _, artifact := range artifacts {
		captured[artifact.SelectedOrder] = true
	}
	inputs := make([]ReviewTransitionInput, 0)
	for order, lens := range selectedLenses {
		if !captured[order] {
			inputs = append(inputs, reviewCaptureInput(binding, lens, order, context))
		}
	}
	if len(inputs) == 0 {
		return reviewStopTransition("captured_result_selection_unavailable")
	}
	return reviewCollectTransition("reviewer_results_required", inputs...)
}

// reviewCaptureResultCaptureOperation is the single wording source for the
// reviewer-result capture operation named by the collect transition
// (reviewCaptureInput below). reviewCaptureResultCommandName derives the
// human-runnable command name from this same constant so a finalize-time
// refusal naming the continuation can never drift from what the collect form
// itself already emits.
const reviewCaptureResultCaptureOperation = "review.capture-result"

// reviewNativeCaptureOperationPrefix marks a capture_operation this product
// performs itself. Everything after it is the runnable `gentle-ai review`
// verb, which is exactly why such an input's arguments are argv.
const reviewNativeCaptureOperationPrefix = "review."

// reviewNativeCaptureVerb resolves the runnable CLI verb of a capture
// operation this product performs: "review.capture-result" yields
// "capture-result". It is the single classifier separating the two kinds of
// collect input. A native one names a real command whose flags are the input's
// own arguments, so those arguments are tokenized. Anything else -- today the
// "external.*" family, which is performed where this product does not run --
// resolves to no verb and is never tokenized, because rendering those values
// as flags would invent a command line nothing here can execute. The check
// fails closed: an unrecognised prefix is treated as non-native rather than
// guessed at.
func reviewNativeCaptureVerb(captureOperation string) (string, bool) {
	verb, found := strings.CutPrefix(captureOperation, reviewNativeCaptureOperationPrefix)
	if !found || verb == "" {
		return "", false
	}
	return verb, true
}

// reviewCaptureResultCommandName renders the exact runnable command name for
// reviewCaptureResultCaptureOperation, e.g. "gentle-ai review capture-result".
func reviewCaptureResultCommandName() string {
	verb, _ := reviewNativeCaptureVerb(reviewCaptureResultCaptureOperation)
	return reviewTransitionCommandTool + " review " + verb
}

func reviewCaptureInput(binding ReviewTransitionBinding, lens string, order int, context *reviewCaptureContext) ReviewTransitionInput {
	arguments := reviewBindingArguments(binding)
	if binding.RepositoryContext != "" {
		arguments = append(arguments, ReviewTransitionArgument{Name: "repository-context", Value: binding.RepositoryContext})
	}
	input := ReviewTransitionInput{
		Name: "reviewer_result", Schema: reviewReviewerSchemaID, CaptureOperation: reviewCaptureResultCaptureOperation,
		Arguments: append(arguments, ReviewTransitionArgument{Name: "lens", Value: lens}, ReviewTransitionArgument{Name: "order", Value: fmt.Sprint(order)}),
	}
	if context != nil && order >= 0 && order < len(context.ArtifactSubjects) {
		subject := context.ArtifactSubjects[order]
		diff := context.FrozenContext.CandidateDiff
		manifest := append([]reviewtransaction.ChangedPathManifestEntry(nil), context.FrozenContext.ChangedPathManifest...)
		if manifest == nil {
			manifest = []reviewtransaction.ChangedPathManifestEntry{}
		}
		input.ArtifactSubject, input.CandidateDiff, input.ChangedPathManifest = &subject, &diff, &manifest
	}
	return input
}

type reviewNextTransitionInput struct {
	Gate                                           reviewtransaction.GateKind
	Successor, Reason, Actor, Authorization        string
	RepairActor, RepairReason, RepairAuthorization string
	StartLineage                                   string
	RepositoryContext                              string
	ValidationRequest                              *reviewtransaction.TargetedValidationRequest
	CorrectionForecasted                           bool
	CaptureContext                                 *reviewCaptureContext
	Selector                                       *reviewTransitionSelector
}

type reviewTransitionSelector struct {
	Kind                  reviewtransaction.TargetKind
	Projection            reviewtransaction.Projection
	BaseRef, BaseTree     string
	WorkspaceOverlay      bool
	RecoveryRepresentable bool
	RecoveryProjection    reviewtransaction.Projection
	PrePRRepresentable    bool
}

func reviewStartArguments(status ReviewTargetStatusResult, lineage string) []ReviewTransitionArgument {
	arguments := []ReviewTransitionArgument{
		{Name: "contract", Value: ReviewIntegrationContractV1},
		{Name: "target", Value: status.TargetIdentity},
		{Name: "projection", Value: string(status.Projection.Projection)},
	}
	switch status.Projection.Kind {
	case reviewtransaction.TargetBaseDiff:
		arguments = append(arguments, ReviewTransitionArgument{Name: "base-ref", Value: status.Projection.BaseTree}, ReviewTransitionArgument{Name: "committed-only", Value: "true"})
	case reviewtransaction.TargetBaseWorkspaceOverlay:
		arguments = append(arguments, ReviewTransitionArgument{Name: "base-ref", Value: status.Projection.BaseTree}, ReviewTransitionArgument{Name: "workspace-overlay", Value: "true"})
	}
	if strings.TrimSpace(lineage) != "" {
		arguments = append(arguments, ReviewTransitionArgument{Name: "lineage", Value: lineage})
	}
	return arguments
}

func reviewRepairTransition(status ReviewTargetStatusResult, input reviewNextTransitionInput) ReviewNextTransition {
	assessment := status.Repair
	candidate := assessment.Candidate
	binding := ReviewTransitionBinding{LineageID: candidate.LineageID, Revision: candidate.Revision, TargetIdentity: status.TargetIdentity}
	providerArguments := []ReviewTransitionArgument{
		{Name: "class", Value: string(assessment.Class)}, {Name: "lineage", Value: candidate.LineageID},
		{Name: "expected-revision", Value: candidate.Revision}, {Name: "cause", Value: string(assessment.Cause)},
		{Name: "disposition", Value: string(assessment.Disposition)}, {Name: "repository-binding", Value: assessment.RepositoryBinding},
	}
	request := reviewtransaction.ClassifiedAuthorityRepairRequest{
		Class: assessment.Class, LineageID: candidate.LineageID, ExpectedRevision: candidate.Revision,
		Cause: assessment.Cause, Disposition: assessment.Disposition, RepositoryBinding: assessment.RepositoryBinding,
		Actor: input.RepairActor, Reason: input.RepairReason, MaintainerAuthorization: input.RepairAuthorization,
	}
	if reviewtransaction.ValidateClassifiedAuthorityRepairRequest(request, assessment) == nil {
		arguments := append([]ReviewTransitionArgument{}, providerArguments...)
		arguments = append(arguments,
			ReviewTransitionArgument{Name: "actor", Value: input.RepairActor},
			ReviewTransitionArgument{Name: "reason", Value: input.RepairReason},
			ReviewTransitionArgument{Name: "maintainer-authorization", Value: "provided"},
		)
		return reviewExecuteTransition("repair_authorized", "review.repair", arguments, []ReviewTransitionArgument{
			{Name: "repair_status", Value: string(reviewtransaction.AuthorityRepairEligible)},
			{Name: "unique_candidate", Value: "true"}, {Name: "current_head", Value: candidate.Revision},
			{Name: "repair_authorization", Value: "provided"},
		}, binding, nil)
	}
	return reviewCollectTransition("repair_authorization_required", ReviewTransitionInput{
		Name: "repair_authorization", Schema: assessment.AuthorizationSchema, CaptureOperation: "external.authorize_repair",
		Arguments: providerArguments,
	})
}

func newReviewCaptureContext(state reviewtransaction.CompactState, revision string, frozen reviewtransaction.FrozenCandidateContext) (*reviewCaptureContext, error) {
	subjects := make([]reviewtransaction.ArtifactSubject, len(state.SelectedLenses))
	for order, lens := range state.SelectedLenses {
		subject, err := reviewtransaction.NewArtifactSubject(state, revision, frozen, lens, order, "")
		if err != nil {
			return nil, fmt.Errorf("derive restart artifact subject %d: %w", order, err)
		}
		subjects[order] = subject
	}
	return &reviewCaptureContext{FrozenContext: frozen, ArtifactSubjects: subjects}, nil
}

func (input reviewNextTransitionInput) gate() reviewtransaction.GateKind {
	if validReviewIntegrationGate(input.Gate) {
		return input.Gate
	}
	return reviewtransaction.GatePreCommit
}

func reviewRecoveryCollection(status ReviewTargetStatusResult, binding ReviewTransitionBinding, input reviewNextTransitionInput) ReviewNextTransition {
	disposition := status.ActionDisposition
	if disposition == "" {
		disposition = reviewtransaction.RecoveryInvalidated
	}
	var selectorArguments []ReviewTransitionArgument
	if input.Selector != nil {
		if status.TargetIdentity == reviewAuthorityTargetIdentity(status) {
			return reviewStopTransition("recovery_scope_unchanged")
		}
		var representable bool
		selectorArguments, representable = input.Selector.recoveryArguments()
		if !representable {
			return reviewStopTransition("recovery_target_unrepresentable")
		}
	}
	if input.recoveryAuthorized(binding) {
		arguments := []ReviewTransitionArgument{{Name: "predecessor-lineage", Value: binding.LineageID}, {Name: "expected-predecessor-revision", Value: binding.Revision}, {Name: "successor-lineage", Value: input.Successor}, {Name: "disposition", Value: string(disposition)}, {Name: "reason", Value: input.Reason}, {Name: "actor", Value: input.Actor}, {Name: "maintainer-authorization", Value: input.Authorization}}
		transition := reviewExecuteTransition("recovery_authorized", "review.recover", append(arguments, selectorArguments...), []ReviewTransitionArgument{{Name: "state", Value: string(status.Authority.State)}, {Name: "recovery_authorization", Value: "provided"}}, binding, nil)
		if input.Selector != nil {
			transition.Execute.SelectorArguments = reviewTransitionSelectorArguments(selectorArguments)
		}
		return transition
	}
	return reviewCollectTransition("recovery_authorization_required", ReviewTransitionInput{
		Name: "recovery_authorization", Schema: "gentle-ai.review-recovery-authorization/v1", CaptureOperation: "external.authorize_recovery",
		Arguments: append(reviewBindingArguments(binding), ReviewTransitionArgument{Name: "disposition", Value: string(disposition)}),
	})
}

func (selector reviewTransitionSelector) recoveryArguments() ([]ReviewTransitionArgument, bool) {
	if !selector.RecoveryRepresentable {
		return nil, false
	}
	arguments := []ReviewTransitionArgument{}
	switch selector.Kind {
	case reviewtransaction.TargetCurrentChanges:
		if selector.BaseRef != "" || selector.BaseTree != "" || selector.WorkspaceOverlay {
			return nil, false
		}
	case reviewtransaction.TargetBaseDiff:
		if selector.BaseRef == "" || selector.BaseTree != "" || selector.WorkspaceOverlay {
			return nil, false
		}
		arguments = append(arguments, ReviewTransitionArgument{Name: "base-ref", Value: selector.BaseRef}, ReviewTransitionArgument{Name: "committed-only", Value: "true"})
	case reviewtransaction.TargetBaseWorkspaceOverlay:
		if !selector.WorkspaceOverlay || (selector.BaseRef == "") == (selector.BaseTree == "") {
			return nil, false
		}
		base := selector.BaseRef
		if base == "" {
			base = selector.BaseTree
		}
		arguments = append(arguments, ReviewTransitionArgument{Name: "base-ref", Value: base})
		if selector.RecoveryProjection == reviewtransaction.ProjectionStaged {
			arguments = append(arguments,
				ReviewTransitionArgument{Name: "projection", Value: string(reviewtransaction.ProjectionStaged)},
				ReviewTransitionArgument{Name: "workspace-overlay", Value: "true"},
			)
			return arguments, true
		}
	default:
		return nil, false
	}
	if selector.RecoveryProjection != "" {
		arguments = append(arguments, ReviewTransitionArgument{Name: "projection", Value: string(selector.RecoveryProjection)})
	}
	return arguments, true
}

func reviewFinalVerificationRetryCollection(status ReviewTargetStatusResult, binding ReviewTransitionBinding) ReviewNextTransition {
	retry := status.FinalVerificationRetry
	if retry == nil || status.ActionDisposition != reviewtransaction.RecoveryFinalVerificationRetry {
		return reviewStopTransition("final_verification_retry_unavailable")
	}
	return reviewCollectTransition("final_verification_retry_authorization_required", ReviewTransitionInput{
		Name: "final_verification_retry_authorization", Schema: reviewtransaction.FinalVerificationRetryAuthorizationSchema,
		CaptureOperation: "external.authorize_final_verification_retry",
		Arguments: []ReviewTransitionArgument{
			{Name: "predecessor-lineage", Value: binding.LineageID},
			{Name: "expected-predecessor-revision", Value: binding.Revision},
			{Name: "validating-revision", Value: retry.ValidatingRevision},
			{Name: "target", Value: retry.TargetIdentity},
			{Name: "failed-evidence-hash", Value: retry.FailedEvidenceHash},
			{Name: "finalize-request-digest", Value: retry.FinalizeRequestDigest},
			{Name: "incident-schema", Value: retry.IncidentSchema},
			{Name: "incident-class", Value: retry.IncidentClass},
		},
	})
}

func (input reviewNextTransitionInput) recoveryAuthorized(binding ReviewTransitionBinding) bool {
	successor := ""
	if input.Selector != nil {
		successor = input.Successor
	}
	return input.Successor != "" && input.Reason != "" && input.Actor != "" && input.Authorization == reviewTransitionRecoveryAuthorization(binding, successor, input.Actor, input.Reason)
}

func reviewTransitionRecoveryAuthorization(binding ReviewTransitionBinding, successor, actor, reason string) string {
	value := "gentle-ai.review-recovery-authorization/v1\npredecessor_lineage=" + binding.LineageID + "\npredecessor_revision=" + binding.Revision + "\ntarget_identity=" + binding.TargetIdentity
	if successor != "" {
		value += "\nsuccessor_lineage=" + successor
	}
	return value + "\nactor=" + actor + "\nreason=" + reason
}

func reviewTransitionSelectorArguments(arguments []ReviewTransitionArgument) *[]ReviewTransitionArgument {
	selectors := append([]ReviewTransitionArgument{}, arguments...)
	return &selectors
}

func reviewTargetArguments(status ReviewTargetStatusResult) []ReviewTransitionArgument {
	return []ReviewTransitionArgument{
		{Name: "target_identity", Value: status.TargetIdentity},
		{Name: "projection", Value: string(status.Projection.Projection)},
		{Name: "base_tree", Value: status.Projection.BaseTree},
		{Name: "candidate_tree", Value: status.Projection.CurrentCandidateTree},
	}
}

func reviewBindingArguments(binding ReviewTransitionBinding) []ReviewTransitionArgument {
	return []ReviewTransitionArgument{{Name: "lineage", Value: binding.LineageID}, {Name: "expected-revision", Value: binding.Revision}, {Name: "target", Value: binding.TargetIdentity}}
}

func reviewTransitionBinding(authority *ReviewTargetStatusAuthority, target string, repositoryContext ...string) ReviewTransitionBinding {
	contextHandle := ""
	if len(repositoryContext) > 0 {
		contextHandle = repositoryContext[0]
	}
	return ReviewTransitionBinding{LineageID: authority.LineageID, Revision: authority.Revision, TargetIdentity: target, RepositoryContext: contextHandle}
}

// reviewTokenizedTransitionArguments renders the literal argv token for every
// argument of one transition. It is the single tokenizer: the execute form and
// the native-capture collect form both call it, so a caller can never be handed
// two different renderings of the same flag.
func reviewTokenizedTransitionArguments(arguments []ReviewTransitionArgument) []ReviewTransitionArgument {
	tokenized := make([]ReviewTransitionArgument, len(arguments))
	for index, argument := range arguments {
		argument.Token = reviewTransitionArgumentToken(argument)
		tokenized[index] = argument
	}
	return tokenized
}

func reviewExecuteTransition(reason, operation string, arguments, preconditions []ReviewTransitionArgument, binding ReviewTransitionBinding, artifacts []ReviewTransitionArtifact) ReviewNextTransition {
	tokenized := reviewTokenizedTransitionArguments(arguments)
	return ReviewNextTransition{Kind: reviewNextTransitionExecute, ReasonCode: reason, Execute: &ReviewTransitionExecution{
		Operation: operation, Command: reviewTransitionCommandLine(operation, tokenized),
		Arguments: tokenized, Preconditions: preconditions, Binding: binding, Artifacts: artifacts,
	}}
}

// reviewTransitionCommandTool is the canonical published tool name used to
// assemble every emitted command line. It is deliberately NOT os.Args[0]: the
// binary is routinely invoked through a shim, a wrapper, or an absolute path
// (benchmarking runs do exactly that), and echoing that path back would emit a
// command that only runs on the machine that generated the payload. The
// canonical name is the one every caller already has on PATH.
const reviewTransitionCommandTool = "gentle-ai"

// reviewTransitionCommandVerb resolves the runnable CLI verb for one
// transition operation. reviewIntegrationOperationRegistry -- the single
// policy source for negotiated routing, safe flag extraction, capability
// publication, and operation-specific diagnostics -- owns the answer through
// its Command field, so the verb a caller is told to run can never diverge
// from the verb negotiated routing recognises.
func reviewTransitionCommandVerb(operation string) (string, bool) {
	if metadata, known := reviewIntegrationOperationByName(operation); known {
		return metadata.Command, true
	}
	return "", false
}

// reviewTransitionCommandLine assembles the complete, literally runnable
// command line for one execute transition. The argument portion is exactly the
// tokens already present on the payload, in the order they already appear:
// nothing is invented, added, or reordered, and every argument therefore
// carries the --flag=value form parseReviewFlags requires (its recorded
// DECISION keeps detached boolean values refused across the review command
// family). An operation with no resolvable verb yields no command at all
// rather than a half-assembled one.
func reviewTransitionCommandLine(operation string, arguments []ReviewTransitionArgument) string {
	verb, resolved := reviewTransitionCommandVerb(operation)
	if !resolved {
		return ""
	}
	parts := make([]string, 0, len(arguments)+3)
	parts = append(parts, reviewTransitionCommandTool, "review", verb)
	for _, argument := range arguments {
		if argument.Token == "" {
			return ""
		}
		parts = append(parts, reviewTransitionShellWord(argument.Token))
	}
	return strings.Join(parts, " ")
}

// reviewTransitionShellWord renders one already-assembled --flag=value token
// as a single POSIX shell word. Most tokens (hashes, lineages, enum values,
// booleans) are already shell-safe and are emitted verbatim, so the common
// command reads exactly like the token list. Operator-supplied free text --
// review.repair carries --reason and --actor straight from `review status
// --repair-reason` / `--repair-actor` -- can contain spaces and quotes; joined
// raw, the shell would split those into stray positional arguments that every
// review verb refuses with "unexpected review <verb> argument". Quoting keeps
// the promise that the printed command runs exactly as printed, and the quotes
// are shell syntax only: the argv entry the shell delivers stays byte-
// identical to the payload's own Token.
func reviewTransitionShellWord(token string) string {
	if token != "" && !strings.ContainsFunc(token, reviewTransitionShellUnsafe) {
		return token
	}
	return "'" + strings.ReplaceAll(token, "'", `'\''`) + "'"
}

// reviewTransitionShellUnsafe reports whether a rune needs quoting. It is an
// allowlist on purpose: anything not proven inert to every POSIX shell (word
// splitting, globbing, expansion, tilde, history) is quoted rather than
// guessed at.
func reviewTransitionShellUnsafe(char rune) bool {
	switch {
	case char >= 'a' && char <= 'z', char >= 'A' && char <= 'Z', char >= '0' && char <= '9':
		return false
	}
	return !strings.ContainsRune("-_=./:,+@", char)
}

// reviewTransitionArgumentToken renders the literal, executable argv token
// for one Execute.Arguments entry. The real CLI flag name is the argument's
// Name with underscores hyphenated (Name itself stays byte-identical for
// existing consumers, e.g. "captured_results" -> "--captured-results=true").
func reviewTransitionArgumentToken(argument ReviewTransitionArgument) string {
	return "--" + strings.ReplaceAll(argument.Name, "_", "-") + "=" + argument.Value
}

// reviewCollectTransition assembles one collect transition.
//
// The arguments of an input whose capture_operation names an operation this
// product performs are tokenized through the same single tokenizer the execute
// form uses, because they are the same thing: the flags of a real
// `gentle-ai review <verb>` command. A caller no longer re-derives
// "--lineage=" + value by hand, which is where a hand-assembled invocation
// twice dropped or mispaired --repository-context. An "external.*" input is
// left untokenized on purpose; see reviewNativeCaptureVerb.
//
// No `command` is emitted, deliberately, and tokenizing the arguments does not
// change that. `review capture-result` also requires --input pointing at a
// reviewer-result artifact that does not exist yet, because no model has run
// the lens; a printed command carrying a placeholder for it would break the
// rule that a named command runs exactly as printed. The tokens are each
// individually runnable; the command line as a whole is not yet complete.
func reviewCollectTransition(reason string, inputs ...ReviewTransitionInput) ReviewNextTransition {
	collected := make([]ReviewTransitionInput, len(inputs))
	for index, input := range inputs {
		if _, native := reviewNativeCaptureVerb(input.CaptureOperation); native {
			input.Arguments = reviewTokenizedTransitionArguments(input.Arguments)
		}
		collected[index] = input
	}
	return ReviewNextTransition{Kind: reviewNextTransitionCollect, ReasonCode: reason, Collect: &ReviewTransitionCollection{Inputs: collected}}
}

func reviewStopTransition(reason string) ReviewNextTransition {
	return ReviewNextTransition{Kind: reviewNextTransitionStop, ReasonCode: reason}
}
