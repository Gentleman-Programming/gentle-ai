package cli

import (
	"fmt"
	"strings"

	"github.com/gentleman-programming/gentle-ai/internal/reviewtransaction"
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
	Operation         string                      `json:"operation"`
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
		if status.Action == reviewtransaction.TargetStatusActionRecover {
			return reviewRecoveryCollection(status, binding, input)
		}
		return reviewStopTransition("escalated_authority")
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

func reviewCaptureInput(binding ReviewTransitionBinding, lens string, order int, context *reviewCaptureContext) ReviewTransitionInput {
	arguments := reviewBindingArguments(binding)
	if binding.RepositoryContext != "" {
		arguments = append(arguments, ReviewTransitionArgument{Name: "repository-context", Value: binding.RepositoryContext})
	}
	input := ReviewTransitionInput{
		Name: "reviewer_result", Schema: reviewReviewerSchemaID, CaptureOperation: "review.capture-result",
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

func reviewExecuteTransition(reason, operation string, arguments, preconditions []ReviewTransitionArgument, binding ReviewTransitionBinding, artifacts []ReviewTransitionArtifact) ReviewNextTransition {
	return ReviewNextTransition{Kind: reviewNextTransitionExecute, ReasonCode: reason, Execute: &ReviewTransitionExecution{Operation: operation, Arguments: arguments, Preconditions: preconditions, Binding: binding, Artifacts: artifacts}}
}

func reviewCollectTransition(reason string, inputs ...ReviewTransitionInput) ReviewNextTransition {
	return ReviewNextTransition{Kind: reviewNextTransitionCollect, ReasonCode: reason, Collect: &ReviewTransitionCollection{Inputs: inputs}}
}

func reviewStopTransition(reason string) ReviewNextTransition {
	return ReviewNextTransition{Kind: reviewNextTransitionStop, ReasonCode: reason}
}
