package cli

import (
	"errors"
	"fmt"
	"strings"

	"github.com/gentleman-programming/gentle-ai/v2/internal/model"
	"github.com/gentleman-programming/gentle-ai/v2/internal/pathquote"
	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
)

const (
	reviewNextTransitionExecute     = "execute"
	reviewNextTransitionCollect     = "collect"
	reviewNextTransitionStop        = "stop"
	reviewTargetedValidationPurpose = "targeted-validation"
)

// ReviewNextTransition is the sole negotiated routing decision. Its execute
// form is complete, its collect form identifies one externally supplied input,
// and its stop form intentionally contains no command-shaped data.
type ReviewNextTransition struct {
	Kind              string                                   `json:"kind"`
	ReasonCode        string                                   `json:"reason_code"`
	Execute           *ReviewTransitionExecution               `json:"execute,omitempty"`
	Collect           *ReviewTransitionCollection              `json:"collect,omitempty"`
	CorrectionRequest *reviewtransaction.CorrectionPlanRequest `json:"correction_request,omitempty"`
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
	Submission          *ReviewTransitionSubmission                   `json:"submission,omitempty"`
	ArtifactSubject     *reviewtransaction.ArtifactSubject            `json:"artifact_subject,omitempty"`
	CandidateDiff       *reviewtransaction.FrozenCandidateDiff        `json:"candidate_diff,omitempty"`
	BaseTree            string                                        `json:"base_tree,omitempty"`
	CandidateTree       string                                        `json:"candidate_tree,omitempty"`
	ChangedPathManifest *[]reviewtransaction.ChangedPathManifestEntry `json:"changed_path_manifest,omitempty"`
	ValidationRequest   *reviewtransaction.TargetedValidationRequest  `json:"validation_request,omitempty"`
}

// ReviewTransitionSubmission is the provider-owned argv template. Consumers
// substitute only the declared Value or Values slots.
type ReviewTransitionSubmission struct {
	OperationToken string                            `json:"operation_token"`
	ArgumentTokens []string                          `json:"argument_tokens"`
	Value          *ReviewTransitionSubmissionValue  `json:"value,omitempty"`
	Values         []ReviewTransitionSubmissionValue `json:"values,omitempty"`
}

type ReviewTransitionSubmissionValue struct {
	Slot                 string   `json:"slot"`
	Domain               string   `json:"domain"`
	Schema               string   `json:"schema,omitempty"`
	Minimum              int      `json:"minimum,omitempty"`
	Maximum              int      `json:"maximum,omitempty"`
	AllowedValues        []string `json:"allowed_values,omitempty"`
	SubstitutionLocation int      `json:"substitution_location"`
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

func newReviewNextTransition(status ReviewTargetStatusResult, selectedLenses []string, artifacts []ReviewTransitionArtifact, capturedEvidence *reviewtransaction.VerificationEvidenceRecord, artifactErr error, input reviewNextTransitionInput) ReviewNextTransition {
	if status.Applicability != reviewtransaction.TargetApplicabilityCurrent {
		switch status.Applicability {
		case reviewtransaction.TargetApplicabilityUnrelated:
			if input.RDDModeResolved && !input.RDDMode.Enabled() {
				return reviewStopTransition("rdd_disabled")
			}
			if input.Selector != nil && input.Selector.Kind == reviewtransaction.TargetBaseWorkspaceOverlay &&
				input.Selector.Projection == reviewtransaction.ProjectionStaged {
				return reviewStopTransition("staged_workspace_overlay_recovery_unavailable")
			}
			// A workspace candidate that froze zero paths is the one fresh
			// target whose START cannot succeed: the facade refuses it in
			// preflight with empty_candidate_scope and names base_ref as the
			// input it needs. Returning that START anyway made status and
			// preflight disagree forever, since the refusal has no way back
			// into this classification (issue #2584). Collect the base
			// instead, and — exactly like the refusal it replaces — name it
			// without deriving it, so the caller keeps choosing the scope.
			if status.Projection.Kind == reviewtransaction.TargetCurrentChanges && len(status.Projection.Paths) == 0 {
				return reviewCollectTransition("empty_candidate_base_ref_required", ReviewTransitionInput{
					Name: "base_ref", Schema: "gentle-ai.review-base-ref-selection/v1", CaptureOperation: "external.select_base_ref",
					Arguments: reviewTargetArguments(status),
				})
			}
			return reviewExecuteTransition("fresh_target_ready", "review.start", reviewStartArguments(status, input.StartLineage, input.RuntimeAgent, input.IntendedUntracked), []ReviewTransitionArgument{{Name: "target_identity", Value: status.TargetIdentity}}, ReviewTransitionBinding{LineageID: input.StartLineage, TargetIdentity: status.TargetIdentity}, nil)
		case reviewtransaction.TargetApplicabilityAmbiguous:
			return reviewCollectTransition("lineage_selection_required", ReviewTransitionInput{
				Name: "lineage_selection", Schema: "gentle-ai.review-lineage-selection/v1", CaptureOperation: "external.select_lineage",
				Arguments: append(reviewTargetArguments(status), ReviewTransitionArgument{Name: "candidates", Value: strings.Join(status.Candidates, ",")}),
			})
		case reviewtransaction.TargetApplicabilityCorrupted:
			if status.Repair.Status == reviewtransaction.AuthorityRepairEligible && status.Repair.Candidate != nil {
				return reviewRepairTransition(status, input)
			}
			// Wave 6 (rdd-closure-disposition-execution / "Reachable Through
			// the Negotiated Transition Route"): the classified vocabulary
			// above has nothing to say about a closed content-mismatched-
			// recovery-authorization closure — status.Disposition is
			// populated only for exactly that gap, so it is checked only
			// once the classified route above has already refused.
			if status.Disposition != nil {
				return reviewDispositionTransition(status, input)
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
			if input.EvidenceErr != nil {
				if !errors.Is(input.EvidenceErr, reviewtransaction.ErrCapturedVerificationEvidenceMissing) &&
					!errors.Is(input.EvidenceErr, reviewtransaction.ErrCapturedVerificationEvidenceMetadataMissing) {
					return reviewStopTransition("captured_verification_evidence_invalid")
				}
				return reviewCollectTransition("correction_repository_verification_required", reviewCaptureEvidenceInput(input.Contract, validationBinding))
			}
			if capturedEvidence == nil {
				return reviewCollectTransition("correction_repository_verification_required", reviewCaptureEvidenceInput(input.Contract, validationBinding))
			}
			switch capturedEvidence.Outcome {
			case reviewtransaction.VerificationOutcomeFailed:
				return reviewStopTransition("correction_repository_verification_failed")
			case reviewtransaction.VerificationOutcomeProceduralFailure:
				return reviewExecuteTransition("correction_repository_tooling_failed", "review.finalize",
					[]ReviewTransitionArgument{{Name: "lineage", Value: binding.LineageID}, {Name: "captured_evidence", Value: "true"}},
					[]ReviewTransitionArgument{{Name: "state", Value: "correction_required"}, {Name: "verification_outcome", Value: string(capturedEvidence.Outcome)}}, validationBinding, nil)
			case reviewtransaction.VerificationOutcomePassed:
			default:
				return reviewStopTransition("captured_verification_evidence_invalid")
			}
			return reviewCollectTransition("targeted_validation_required", ReviewTransitionInput{
				Name: "targeted_validation", Schema: reviewtransaction.TargetedValidationRequestSchema,
				CaptureOperation: "external.run_targeted_validation", Arguments: reviewTargetedValidationArguments(input.Contract, validationBinding, *input.ValidationRequest),
				ValidationRequest: input.ValidationRequest,
				Submission:        reviewTargetedValidationSubmission(input.Contract, validationBinding, *input.ValidationRequest),
			})
		}
		if input.CorrectionForecasted {
			if input.CorrectionRequest == nil {
				return reviewStopTransition("corrupted_or_unverifiable_authority")
			}
			transition := reviewStopTransition("corrected_candidate_unavailable")
			transition.CorrectionRequest = input.CorrectionRequest
			return transition
		}
		if input.CorrectionRequest == nil {
			return reviewStopTransition("corrupted_or_unverifiable_authority")
		}
		transition := reviewCollectTransition("correction_plan_required", ReviewTransitionInput{
			Name: "correction_lines", Schema: "gentle-ai.review-correction-plan/v1", CaptureOperation: "external.plan_correction",
			Arguments: reviewBindingArguments(binding), Submission: reviewCorrectionPlanSubmission(input.Contract, binding, *input.CorrectionRequest),
		})
		transition.CorrectionRequest = input.CorrectionRequest
		return transition
	case reviewtransaction.StateValidating:
		if input.EvidenceErr != nil && !errors.Is(input.EvidenceErr, reviewtransaction.ErrCapturedVerificationEvidenceMissing) &&
			!errors.Is(input.EvidenceErr, reviewtransaction.ErrCapturedVerificationEvidenceMetadataMissing) {
			return reviewStopTransition("captured_verification_evidence_invalid")
		}
		if capturedEvidence != nil {
			reason := "captured_verification_evidence_passed"
			switch capturedEvidence.Outcome {
			case reviewtransaction.VerificationOutcomeFailed:
				reason = "captured_verification_failed"
			case reviewtransaction.VerificationOutcomeProceduralFailure:
				reason = "captured_verification_tooling_failed"
			case reviewtransaction.VerificationOutcomePassed:
			default:
				return reviewStopTransition("captured_verification_evidence_invalid")
			}
			return reviewExecuteTransition(reason, "review.finalize", []ReviewTransitionArgument{{Name: "lineage", Value: binding.LineageID}, {Name: "captured_evidence", Value: "true"}}, []ReviewTransitionArgument{{Name: "state", Value: "validating"}, {Name: "verification_outcome", Value: string(capturedEvidence.Outcome)}}, binding, nil)
		}
		if status.Frozen != nil && status.Frozen.Tier == reviewtransaction.RiskLow {
			return reviewExecuteTransition("native_low_risk_verification", "review.finalize", []ReviewTransitionArgument{{Name: "lineage", Value: binding.LineageID}}, []ReviewTransitionArgument{{Name: "state", Value: "validating"}, {Name: "risk_level", Value: "low"}}, binding, nil)
		}
		return reviewCollectTransition("verification_evidence_required", reviewCaptureEvidenceInput(input.Contract, binding))
	case reviewtransaction.StateInvalidated:
		return reviewRecoveryCollection(status, binding, input)
	case reviewtransaction.StateApproved:
		if status.Action == reviewtransaction.TargetStatusActionRecover {
			return reviewRecoveryCollection(status, binding, input)
		}
		if status.Receipt.Status == ReviewReceiptPresent {
			if input.gate() == reviewtransaction.GatePreCommit && input.PreCommitDeliveryAssessment != nil &&
				*input.PreCommitDeliveryAssessment != reviewtransaction.CompactGateTargetExact {
				return reviewStopTransition("staged_delivery_candidate_required")
			}
			if input.Selector != nil && input.gate() == reviewtransaction.GatePrePR && !input.Selector.PrePRRepresentable {
				// Root 7 (#2471): the caller supplied a raw commit SHA where
				// pre-PR needs a symbolic ref. That is a missing input, not a
				// terminal state, and the reason code stays byte-identical so
				// consumers routing on it keep working while the kind stops
				// lying about there being nothing to do. Same shape as
				// empty_candidate_base_ref_required above: name the input
				// without deriving it, because only the caller knows which
				// ref is the intended base.
				return reviewCollectTransition("pre_pr_selector_unrepresentable", ReviewTransitionInput{
					Name: "base_ref", Schema: "gentle-ai.review-base-ref-selection/v1", CaptureOperation: "external.select_base_ref",
					Arguments: reviewTargetArguments(status),
				})
			}
			arguments := []ReviewTransitionArgument{{Name: "lineage", Value: binding.LineageID}, {Name: "gate", Value: string(input.gate())}}
			selectors := []ReviewTransitionArgument{}
			if input.Selector != nil &&
				(input.gate() == reviewtransaction.GatePrePush || input.gate() == reviewtransaction.GatePrePR) &&
				input.Selector.BaseRef != "" {
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
	Contract          string
	RepositoryContext string
	ValidationRequest *reviewtransaction.TargetedValidationRequest
	CorrectionRequest *reviewtransaction.CorrectionPlanRequest
	CaptureContext    *reviewCaptureContext
	CapturedEvidence  *reviewtransaction.VerificationEvidenceRecord
	EvidenceErr       error
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
	if state.State == reviewtransaction.StateCorrectionRequired && !state.CorrectionAttemptConsumed() && transitionContext.CorrectionRequest == nil {
		request, err := reviewtransaction.BuildCorrectionPlanRequest(state, revision)
		if err != nil {
			return reviewStopTransition("corrupted_or_unverifiable_authority")
		}
		transitionContext.CorrectionRequest = &request
	}
	if state.State == reviewtransaction.StateReviewing && artifactErr == nil && len(artifacts) != len(state.SelectedLenses) {
		return reviewMissingCaptureTransition(reviewTransitionBinding(status.Authority, status.TargetIdentity, transitionContext.RepositoryContext), state.SelectedLenses, artifacts, transitionContext.CaptureContext)
	}
	if state.State == reviewtransaction.StateReviewing && artifactErr == nil {
		return reviewExecuteTransition("captured_results_ready", "review.finalize", []ReviewTransitionArgument{{Name: "lineage", Value: state.LineageID}, {Name: "captured_results", Value: "true"}}, []ReviewTransitionArgument{{Name: "state", Value: "reviewing"}, {Name: "captured_artifacts", Value: "complete"}}, reviewTransitionBinding(status.Authority, status.TargetIdentity), artifacts)
	}
	return newReviewNextTransition(status, state.SelectedLenses, artifacts, transitionContext.CapturedEvidence, artifactErr, reviewNextTransitionInput{
		Contract: transitionContext.Contract, RepositoryContext: transitionContext.RepositoryContext, ValidationRequest: transitionContext.ValidationRequest,
		CorrectionRequest: transitionContext.CorrectionRequest, EvidenceErr: transitionContext.EvidenceErr, CorrectionForecasted: state.ProposedCorrectionLines != nil,
		CaptureContext: transitionContext.CaptureContext,
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
		manifest := append([]reviewtransaction.ChangedPathManifestEntry(nil), context.FrozenContext.ChangedPathManifest...)
		if manifest == nil {
			manifest = []reviewtransaction.ChangedPathManifestEntry{}
		}
		input.ArtifactSubject = &subject
		input.ChangedPathManifest = &manifest
		if context.FrozenContext.LegacyCandidateDiff != nil {
			diff := *context.FrozenContext.LegacyCandidateDiff
			input.CandidateDiff = &diff
		} else {
			input.Arguments = append(input.Arguments, ReviewTransitionArgument{Name: "subject-hash", Value: subject.SubjectHash})
			input.BaseTree, input.CandidateTree = context.FrozenContext.BaseTree, context.FrozenContext.CandidateTree
		}
	}
	return input
}

type reviewNextTransitionInput struct {
	Gate                                           reviewtransaction.GateKind
	Successor, Reason, Actor, Authorization        string
	RepairActor, RepairReason, RepairAuthorization string
	StartLineage                                   string
	RuntimeAgent                                   model.AgentID
	Contract                                       string
	RepositoryContext                              string
	ValidationRequest                              *reviewtransaction.TargetedValidationRequest
	CorrectionRequest                              *reviewtransaction.CorrectionPlanRequest
	EvidenceErr                                    error
	CorrectionForecasted                           bool
	CaptureContext                                 *reviewCaptureContext
	Selector                                       *reviewTransitionSelector
	IntendedUntracked                              reviewIntendedUntrackedScope
	RDDMode                                        reviewtransaction.RDDModeStatus
	RDDModeResolved                                bool
	PreCommitDeliveryAssessment                    *reviewtransaction.CompactGateTargetApplicability
}

const reviewSubmissionValuePlaceholder = "{{value}}"

func reviewCorrectionPlanSubmission(contract string, binding ReviewTransitionBinding, request reviewtransaction.CorrectionPlanRequest) *ReviewTransitionSubmission {
	if contract != ReviewIntegrationContractV2 || binding.RepositoryContext == "" {
		return nil
	}
	return reviewFinalizeSubmission([]string{
		reviewTransitionArgumentToken(ReviewTransitionArgument{Name: "contract", Value: contract}),
		reviewTransitionArgumentToken(ReviewTransitionArgument{Name: "lineage", Value: binding.LineageID}),
		reviewTransitionArgumentToken(ReviewTransitionArgument{Name: "expected-revision", Value: binding.Revision}),
		reviewTransitionArgumentToken(ReviewTransitionArgument{Name: "target", Value: binding.TargetIdentity}),
		reviewTransitionArgumentToken(ReviewTransitionArgument{Name: "request-hash", Value: request.RequestHash}),
		reviewTransitionArgumentToken(ReviewTransitionArgument{Name: "repository-context", Value: binding.RepositoryContext}),
		"--correction-lines=" + reviewSubmissionValuePlaceholder,
	}, ReviewTransitionSubmissionValue{
		Slot: "correction_lines", Domain: "positive_correction_lines", Minimum: 1,
		Maximum: request.CorrectionBudget, SubstitutionLocation: 6,
	})
}

func reviewTargetedValidationSubmission(contract string, binding ReviewTransitionBinding, request reviewtransaction.TargetedValidationRequest) *ReviewTransitionSubmission {
	if contract != ReviewIntegrationContractV2 || binding.RepositoryContext == "" {
		return nil
	}
	return reviewFinalizeSubmission([]string{
		reviewTransitionArgumentToken(ReviewTransitionArgument{Name: "contract", Value: contract}),
		reviewTransitionArgumentToken(ReviewTransitionArgument{Name: "lineage", Value: binding.LineageID}),
		reviewTransitionArgumentToken(ReviewTransitionArgument{Name: "expected-revision", Value: binding.Revision}),
		reviewTransitionArgumentToken(ReviewTransitionArgument{Name: "target", Value: binding.TargetIdentity}),
		reviewTransitionArgumentToken(ReviewTransitionArgument{Name: "request-hash", Value: request.RequestHash}),
		reviewTransitionArgumentToken(ReviewTransitionArgument{Name: "repository-context", Value: binding.RepositoryContext}),
		"--validation=" + reviewSubmissionValuePlaceholder,
		"--captured-evidence=true",
	}, ReviewTransitionSubmissionValue{
		Slot: "validation", Domain: "artifact_path_or_stdin", Schema: reviewValidatorSchemaID,
		SubstitutionLocation: 6,
	})
}

func reviewFinalizeSubmission(argumentTokens []string, value ReviewTransitionSubmissionValue) *ReviewTransitionSubmission {
	return &ReviewTransitionSubmission{OperationToken: "finalize", ArgumentTokens: argumentTokens, Value: &value}
}

func reviewCaptureEvidenceInput(contract string, binding ReviewTransitionBinding) ReviewTransitionInput {
	arguments := reviewBindingArguments(binding)
	schema := reviewtransaction.VerificationEvidenceRecordSchema
	if contract == ReviewIntegrationContractV2 {
		schema = reviewVerificationEvidenceSchemaID
		arguments = append(arguments, ReviewTransitionArgument{Name: "repository-context", Value: binding.RepositoryContext})
	}
	return ReviewTransitionInput{
		Name: "evidence", Schema: schema, CaptureOperation: "review.capture-evidence", Arguments: arguments,
		Submission: reviewCaptureEvidenceSubmission(contract, binding),
	}
}

func reviewCaptureEvidenceSubmission(contract string, binding ReviewTransitionBinding) *ReviewTransitionSubmission {
	if contract != ReviewIntegrationContractV2 || binding.RepositoryContext == "" {
		return nil
	}
	return &ReviewTransitionSubmission{
		OperationToken: "capture-evidence",
		ArgumentTokens: []string{
			reviewTransitionArgumentToken(ReviewTransitionArgument{Name: "lineage", Value: binding.LineageID}),
			reviewTransitionArgumentToken(ReviewTransitionArgument{Name: "expected-revision", Value: binding.Revision}),
			reviewTransitionArgumentToken(ReviewTransitionArgument{Name: "target", Value: binding.TargetIdentity}),
			reviewTransitionArgumentToken(ReviewTransitionArgument{Name: "repository-context", Value: binding.RepositoryContext}),
			"--outcome={{outcome}}",
			"--input={{input}}",
		},
		Values: []ReviewTransitionSubmissionValue{
			{
				Slot: "outcome", Domain: "verification_outcome",
				AllowedValues: []string{
					string(reviewtransaction.VerificationOutcomePassed),
					string(reviewtransaction.VerificationOutcomeFailed),
					string(reviewtransaction.VerificationOutcomeProceduralFailure),
				},
				SubstitutionLocation: 4,
			},
			{
				Slot: "input", Domain: "artifact_path_or_stdin", Schema: reviewVerificationEvidenceSchemaID,
				SubstitutionLocation: 5,
			},
		},
	}
}

type reviewTransitionSelector struct {
	Kind                               reviewtransaction.TargetKind
	Projection                         reviewtransaction.Projection
	BaseRef, BaseTree                  string
	WorkspaceOverlay                   bool
	Recovery                           *reviewtransaction.Target
	SelectorFreeAccountingOnlyRecovery bool
	PrePRRepresentable                 bool
}

func reviewStartArguments(status ReviewTargetStatusResult, lineage string, runtime model.AgentID, intended reviewIntendedUntrackedScope) []ReviewTransitionArgument {
	contract := status.Contract
	if contract == "" {
		contract = ReviewIntegrationContractV1
	}
	arguments := []ReviewTransitionArgument{
		{Name: "cwd", Value: status.repositoryRoot}, {Name: "contract", Value: contract},
		{Name: "target", Value: status.TargetIdentity},
		{Name: "projection", Value: string(status.Projection.Projection)},
	}
	if status.repositoryRoot == "" {
		arguments = arguments[1:]
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
	if runtime != "" {
		arguments = append(arguments, ReviewTransitionArgument{Name: "agent", Value: string(runtime)})
	}
	if contract == ReviewIntegrationContractV2 {
		arguments = append(arguments, ReviewTransitionArgument{Name: "consent", Value: string(reviewConsentModeRelay)})
	}
	arguments = append(arguments, reviewStartIntendedUntrackedArguments(intended)...)
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

// reviewDispositionTransition is Wave 6's negotiated route for a closure
// disposition plan (rdd-closure-disposition-execution / "Reachable Through
// the Negotiated Transition Route", design decision D7): it serves the exact
// same `review repair` verb reviewRepairTransition already routes to — no
// new operation, no new CLI command — through collect{disposition_
// authorization} then execute{review.repair, --plan-digest
// --inventory-revision --actor --reason --authorization}, mirroring
// reviewRepairTransition's own collect-then-execute shape byte-for-byte so a
// caller who already understands one understands both. It is only ever
// reached once the classified route above it has already refused (its
// caller checks status.Disposition only after status.Repair), so it never
// competes with or shadows Wave 2's classified vocabulary.
func reviewDispositionTransition(status ReviewTargetStatusResult, input reviewNextTransitionInput) ReviewNextTransition {
	disposition := status.Disposition
	providerArguments := []ReviewTransitionArgument{
		{Name: "plan-digest", Value: disposition.PlanDigest},
		{Name: "inventory-revision", Value: disposition.AuthorityInventoryRevision},
	}
	if strings.TrimSpace(input.RepairActor) != "" && strings.TrimSpace(input.RepairReason) != "" && strings.TrimSpace(input.RepairAuthorization) != "" {
		arguments := append([]ReviewTransitionArgument{}, providerArguments...)
		arguments = append(arguments,
			ReviewTransitionArgument{Name: "actor", Value: input.RepairActor},
			ReviewTransitionArgument{Name: "reason", Value: input.RepairReason},
			// The authorization VALUE is deliberately the "provided" sentinel,
			// never the real bytes — mirroring reviewRepairTransition's own
			// "maintainer-authorization" argument (tasks.md 4.5's threat
			// matrix: emitted tokens carry no authorization bytes).
			ReviewTransitionArgument{Name: "authorization", Value: "provided"},
		)
		// Every other "review.repair" execute transition carries a concrete
		// lineage_id/revision binding (reviewRepairTransition's own
		// candidate.LineageID/candidate.Revision) — the disposition plan's
		// seed is the matching identity here (Wave 6 D7's status.Disposition
		// carries it for exactly this reason).
		return reviewExecuteTransition("disposition_authorized", "review.repair", arguments, []ReviewTransitionArgument{
			{Name: "plan_digest", Value: disposition.PlanDigest},
			{Name: "authority_inventory_revision", Value: disposition.AuthorityInventoryRevision},
			{Name: "disposition_authorization", Value: "provided"},
		}, ReviewTransitionBinding{LineageID: disposition.SeedLineageID, Revision: disposition.SeedExpectedRevision, TargetIdentity: status.TargetIdentity}, nil)
	}
	return reviewCollectTransition("disposition_authorization_required", ReviewTransitionInput{
		Name: "disposition_authorization", Schema: reviewtransaction.AuthorityDispositionAuthorizationSchema, CaptureOperation: "external.authorize_repair",
		Arguments: providerArguments,
	})
}

func newReviewCaptureContext(state reviewtransaction.CompactState, revision string, frozen reviewtransaction.FrozenCandidateContext) (*reviewCaptureContext, error) {
	subjects := make([]reviewtransaction.ArtifactSubject, len(state.SelectedLenses))
	for order, lens := range state.SelectedLenses {
		var subject reviewtransaction.ArtifactSubject
		var err error
		if frozen.LegacyCandidateDiff != nil {
			subject, err = reviewtransaction.NewLegacyArtifactSubject(state, revision, frozen, lens, order, "")
		} else {
			subject, err = reviewtransaction.NewArtifactSubject(state, revision, frozen, lens, order, "")
		}
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
	// The core has already narrowed this to the evidence-bound,
	// accounting-only edge. It is the only selector-free recovery.
	if input.Selector != nil && !input.Selector.SelectorFreeAccountingOnlyRecovery {
		if input.Selector.Recovery == nil {
			return reviewCollectTransition("recovery_target_unrepresentable", ReviewTransitionInput{
				Name: "recovery_target_selector", Schema: "gentle-ai.review-recovery-target-selection/v1",
				CaptureOperation: "external.select_recovery_target", Arguments: reviewTargetArguments(status),
			})
		}
		if status.TargetIdentity == reviewAuthorityTargetIdentity(status) {
			return reviewStopTransition("recovery_scope_unchanged")
		}
		var representable bool
		selectorArguments, representable = input.Selector.recoveryArguments()
		if !representable {
			// Root 7 (#2471): the selector the caller supplied cannot be
			// represented as a recovery target, so the missing thing is a
			// different selector they choose, not a dead end.
			return reviewCollectTransition("recovery_target_unrepresentable", ReviewTransitionInput{
				Name: "recovery_target_selector", Schema: "gentle-ai.review-recovery-target-selection/v1",
				CaptureOperation: "external.select_recovery_target", Arguments: reviewTargetArguments(status),
			})
		}
	}
	if input.recoveryAuthorized(binding) {
		arguments := []ReviewTransitionArgument{{Name: "predecessor-lineage", Value: binding.LineageID}, {Name: "expected-predecessor-revision", Value: binding.Revision}, {Name: "successor-lineage", Value: input.Successor}, {Name: "disposition", Value: string(disposition)}, {Name: "reason", Value: input.Reason}, {Name: "actor", Value: input.Actor}, {Name: "maintainer-authorization", Value: input.Authorization}}
		transition := reviewExecuteTransition("recovery_authorized", "review.recover", append(arguments, selectorArguments...), []ReviewTransitionArgument{{Name: "state", Value: string(status.Authority.State)}, {Name: "recovery_authorization", Value: "provided"}}, binding, nil)
		if input.Selector != nil && !input.Selector.SelectorFreeAccountingOnlyRecovery {
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
	if selector.Recovery == nil {
		return nil, false
	}
	target := *selector.Recovery
	if target.BaseRef == "" {
		target.BaseRef = selector.BaseRef
	}
	arguments := []ReviewTransitionArgument{}
	switch target.Kind {
	case reviewtransaction.TargetCurrentChanges:
		if target.Projection == reviewtransaction.ProjectionStaged {
			arguments = append(arguments, ReviewTransitionArgument{Name: "projection", Value: string(target.Projection)})
		}
	case reviewtransaction.TargetBaseDiff:
		if target.BaseRef == "" || target.Projection != reviewtransaction.ProjectionWorkspace {
			return nil, false
		}
		arguments = append(arguments, ReviewTransitionArgument{Name: "base-ref", Value: target.BaseRef}, ReviewTransitionArgument{Name: "committed-only", Value: "true"})
	case reviewtransaction.TargetBaseWorkspaceOverlay:
		if target.BaseRef == "" {
			return nil, false
		}
		arguments = append(arguments, ReviewTransitionArgument{Name: "base-ref", Value: target.BaseRef})
		if target.Projection == reviewtransaction.ProjectionStaged {
			arguments = append(arguments,
				ReviewTransitionArgument{Name: "projection", Value: string(reviewtransaction.ProjectionStaged)},
				ReviewTransitionArgument{Name: "workspace-overlay", Value: "true"},
			)
			return arguments, true
		}
		arguments = append(arguments, ReviewTransitionArgument{Name: "workspace-overlay", Value: "true"})
	default:
		return nil, false
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
			{Name: "failed-evidence-record-digest", Value: retry.FailedEvidenceRecordDigest},
			{Name: "finalize-request-digest", Value: retry.FinalizeRequestDigest},
			{Name: "incident-schema", Value: retry.IncidentSchema},
			{Name: "incident-class", Value: retry.IncidentClass},
		},
	})
}

func (input reviewNextTransitionInput) recoveryAuthorized(binding ReviewTransitionBinding) bool {
	successor := ""
	if input.Selector != nil && !input.Selector.SelectorFreeAccountingOnlyRecovery {
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

func reviewTargetedValidationArguments(contract string, binding ReviewTransitionBinding, request reviewtransaction.TargetedValidationRequest) []ReviewTransitionArgument {
	arguments := reviewBindingArguments(binding)
	if contract == ReviewIntegrationContractV2 {
		arguments = append(arguments, ReviewTransitionArgument{Name: "repository-context", Value: binding.RepositoryContext},
			ReviewTransitionArgument{Name: "purpose", Value: reviewTargetedValidationPurpose}, ReviewTransitionArgument{Name: "request-hash", Value: request.RequestHash})
	}
	return arguments
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

// reviewTransitionShellWord retains the review command assembly seam while the
// shared renderer owns POSIX shell-word policy for every printed continuation.
func reviewTransitionShellWord(token string) string {
	return pathquote.ShellWord(token)
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

func reviewReasonDescription(reason string) string {
	switch reason {
	case "fresh_target_ready":
		return "Target is unreviewed and ready for initial review start"
	case "captured_results_ready":
		return "Captured reviewer results are complete and ready for finalization"
	case "native_low_risk_verification":
		return "Low risk candidate eligible for native verification"
	case "approved_receipt_ready":
		return "Review is approved and receipt is ready for gate validation"
	case "exact_receipt_replay":
		return "Exact receipt replay safe for finalization"
	case "lineage_selection_required":
		return "Multiple lineages match target; select an explicit lineage"
	case "reviewer_results_required":
		return "Reviewer lens artifacts required for current revision"
	case "targeted_validation_required":
		return "Targeted validation run required for correction plan"
	case "correction_plan_required":
		return "Correction plan required to resolve review findings"
	case "verification_evidence_required":
		return "Verification evidence required prior to finalization"
	case "delivery_gate_required":
		return "Delivery gate selection required before validation"
	case "staged_delivery_candidate_required":
		return "The staged delivery candidate must exactly match the approved review"
	case "staged_workspace_overlay_recovery_unavailable":
		return "Staged workspace overlay recovery is unavailable"
	case "corrupted_or_unverifiable_authority":
		return "Review authority is corrupted or unverifiable"
	case "missing_authority_binding":
		return "Target authority binding is missing"
	case "original_finalize_request_required":
		return "Original finalize request is required to reconcile"
	case "unchanged_or_unverified_authority":
		return "Authority requires a changed or verified candidate"
	case "native_stop_required":
		return "Native stop transition required by authority"
	case "captured_artifacts_unverifiable":
		return "Captured artifacts failed verification or are missing"
	case "corrected_candidate_unavailable":
		return "Corrected candidate is unavailable for forecasted correction"
	case "pre_pr_selector_unrepresentable":
		return "Selected base-ref cannot be represented for pre-PR gate"
	case "manual_intervention_required":
		return "Manual intervention is required to proceed"
	default:
		return strings.ReplaceAll(reason, "_", " ")
	}
}

func newReviewForecast(head ReviewNextTransition) ReviewForecast {
	horizon := ForecastHorizonPartial
	if head.Kind == reviewNextTransitionStop {
		horizon = ForecastHorizonTerminal
	}
	return ReviewForecast{
		Horizon: horizon,
		Steps: []ReviewForecastItem{{
			Step:        1,
			Kind:        head.Kind,
			ReasonCode:  head.ReasonCode,
			Description: reviewReasonDescription(head.ReasonCode),
		}},
	}
}
