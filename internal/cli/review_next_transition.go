package cli

import (
	"errors"
	"fmt"
	"strings"

	"github.com/gentleman-programming/gentle-ai/v2/internal/model"
	"github.com/gentleman-programming/gentle-ai/v2/internal/pathquote"
	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewerprovider"
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
	ProviderTask        *ReviewProviderTask                           `json:"provider_task,omitempty"`
	Submission          *ReviewTransitionSubmission                   `json:"submission,omitempty"`
	ArtifactSubject     *reviewtransaction.ArtifactSubject            `json:"artifact_subject,omitempty"`
	CandidateDiff       *reviewtransaction.FrozenCandidateDiff        `json:"candidate_diff,omitempty"`
	BaseTree            string                                        `json:"base_tree,omitempty"`
	CandidateTree       string                                        `json:"candidate_tree,omitempty"`
	ChangedPathManifest *[]reviewtransaction.ChangedPathManifestEntry `json:"changed_path_manifest,omitempty"`
	ValidationRequest   *reviewtransaction.TargetedValidationRequest  `json:"validation_request,omitempty"`
}

// ReviewProviderTask is a Go-issued host task. OpenCode relays its opaque
// prompt and final bytes through one live child process; Go owns admission.
type ReviewProviderTask struct {
	Agent  string `json:"agent"`
	Role   string `json:"role"`
	Prompt string `json:"prompt"`
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
	// really is argv: on ReviewTransitionExecution.Arguments, on the
	// Arguments of a ReviewTransitionInput whose CaptureOperation names an
	// operation this product performs (see reviewNativeCaptureVerb), and on
	// the SelectorArguments of a reviewing START status continuation, whose
	// rows are byte-identical copies of already-tokenized argument rows
	// (issue #3894). It stays empty on Preconditions, which are assertions
	// rather than argv, on the normalized selector echoes older transitions
	// carry, and on the Arguments of an "external.*" capture operation,
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
	// RepositoryRoot is rendered as the --cwd token beside the repository
	// context and never serialized into the binding object. The context handle
	// is a digest over this repository and the binding above, so the capture
	// command has to be told which repository to verify it against.
	RepositoryRoot string `json:"-"`
}

// reviewRepositoryContextArguments renders only the opaque digest. The
// repository it commits to is deliberately absent: a rendered transition
// carries no filesystem path, and the caller already holds the repository it
// asked STATUS about. A host runs these tokens in that repository, exactly as
// the submission descriptors -- which refuse a --cwd token outright -- require.
func reviewRepositoryContextArguments(binding ReviewTransitionBinding) []ReviewTransitionArgument {
	return []ReviewTransitionArgument{{Name: "repository-context", Value: binding.RepositoryContext}}
}

// ReviewTransitionArtifact deliberately excludes the provider-owned path. The
// native closure path discovers immutable captured bytes itself.
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

func newReviewNextTransition(status ReviewTargetStatusResult, selectedLenses []string, artifacts []ReviewTransitionArtifact, artifactErr error, input reviewNextTransitionInput) ReviewNextTransition {
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
			// #3102: an explicitly selected base-diff with no paths reaches the
			// same empty_candidate_scope preflight refusal as a clean workspace.
			// The base is already provider-bound, so collecting it again cannot
			// change the scope. Follow #1641's authorized empty-root bootstrap
			// policy instead; STATUS must not advertise a START it knows fails.
			if status.Projection.Kind == reviewtransaction.TargetBaseDiff && len(status.Projection.Paths) == 0 {
				return reviewStopTransition("empty_base_diff_bootstrap_required")
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
	if status.Authority.State == reviewtransaction.StateValidating || status.Authority.State == reviewtransaction.StateCorrectionRequired ||
		status.Authority.State == reviewtransaction.StateApproved && input.Acknowledgement != nil {
		// Correction-plan capture is bound to the severe reviewer event's frozen
		// candidate, never to a live correction candidate STATUS may be
		// projecting. Targeted validation replaces this value with its own
		// correction target below.
		bindingTarget = reviewAuthorityTargetIdentity(status)
	}
	binding := reviewTransitionBinding(status.Authority, bindingTarget, status.repositoryRoot, input.RepositoryContext)
	captureBinding := binding
	if status.Authority.CapturePhaseRevision != "" {
		captureBinding.Revision = status.Authority.CapturePhaseRevision
	}
	// The pending acknowledgement is the lineage's own next step, not a v2
	// feature (issue #3940): gating it on the contract sent every v1 caller to
	// native_stop_required one step before the burn it was asked to perform.
	if status.Authority.State == reviewtransaction.StateApproved && input.Acknowledgement != nil {
		acknowledgement := *input.Acknowledgement
		if acknowledgement.LineageID != binding.LineageID || acknowledgement.TargetIdentity != binding.TargetIdentity || acknowledgement.ExpectedRevision != binding.Revision {
			return reviewStopTransition("corrupted_or_unverifiable_authority")
		}
		return ReviewNextTransition{Kind: reviewNextTransitionExecute, ReasonCode: "approved_acknowledgement_required", Execute: reviewApprovedAcknowledgementTransition(status.repositoryRoot, acknowledgement)}
	}
	if artifactErr != nil && (status.Authority.State == reviewtransaction.StateReviewing || input.ValidationRequest != nil) {
		return reviewStopTransition("captured_artifacts_unverifiable")
	}
	if status.Authority.State == reviewtransaction.StateReviewing && input.LensContextBudgetExceeded {
		return reviewStopTransition("lens_context_budget_exceeded")
	}
	// The core action remains stop while a compact review is in progress: only
	// this adapter can project its next missing capture. Reviewing and
	// correction-required authority therefore fall through to their bound
	// capture routes below. Every other stopped state has no public event left
	// to admit and remains a terminal native stop.
	if status.Action == reviewtransaction.TargetStatusActionStop &&
		status.Authority.State != reviewtransaction.StateReviewing &&
		status.Authority.State != reviewtransaction.StateCorrectionRequired {
		return reviewStopTransition("native_stop_required")
	}
	switch status.Authority.State {
	case reviewtransaction.StateReviewing:
		if artifactErr != nil {
			return reviewStopTransition("captured_artifacts_unverifiable")
		}
		if len(artifacts) != len(selectedLenses) {
			return reviewMissingCaptureTransition(captureBinding, selectedLenses, artifacts, input.CaptureContext, input.RuntimeAgent)
		}
		if input.ProviderRole == reviewerprovider.RoleRefuter {
			return reviewProviderRoleTransition("provider_refuter_required", captureBinding, input.ProviderRole, input.RuntimeAgent, nil)
		}
		return reviewStopTransition("manual_intervention_required")
	case reviewtransaction.StateCorrectionRequired:
		if status.Action == reviewtransaction.TargetStatusActionRecover {
			// Recovery binds the successor target STATUS selected. The frozen
			// correction target above is only for correction-plan capture.
			recoveryBinding := binding
			recoveryBinding.TargetIdentity = status.TargetIdentity
			return reviewRecoveryCollection(status, recoveryBinding, input)
		}
		if input.ValidationRequest != nil {
			validationBinding := captureBinding
			validationBinding.TargetIdentity = input.ValidationRequest.CorrectionTargetIdentity
			if input.ProviderRole == reviewerprovider.RoleTargetedValidator {
				// Same Go-issued role task either way; only the reason differs,
				// so a consumer can tell a first validation apart from one being
				// run again because the captured attempt produced no verdict.
				reason := "targeted_validation_required"
				if input.CapturedProviderTargetedValidatorInconclusive {
					reason = reviewInconclusiveTargetedValidationReason
				}
				return reviewProviderRoleTransition(reason, validationBinding, input.ProviderRole, input.RuntimeAgent, input.ValidationRequest)
			}
			if input.CapturedProviderTargetedValidatorInconclusive || input.CapturedProviderTargetedValidator {
				return reviewStopTransition("manual_intervention_required")
			}
			return reviewStopTransition("manual_intervention_required")
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
			Name: "correction_lines", Schema: "gentle-ai.review-correction-plan/v1", CaptureOperation: reviewCaptureCorrectionPlanOperation,
			Arguments:  append(append(reviewBindingArguments(captureBinding), reviewRepositoryContextArguments(captureBinding)...), ReviewTransitionArgument{Name: "request-hash", Value: input.CorrectionRequest.RequestHash}),
			Submission: reviewCorrectionPlanSubmission(input.Contract, captureBinding, *input.CorrectionRequest),
		})
		transition.CorrectionRequest = input.CorrectionRequest
		return transition
	case reviewtransaction.StateValidating:
		return reviewStopTransition("manual_intervention_required")
	case reviewtransaction.StateInvalidated:
		return reviewRecoveryCollection(status, binding, input)
	case reviewtransaction.StateApproved:
		if status.Action == reviewtransaction.TargetStatusActionRecover {
			return reviewRecoveryCollection(status, binding, input)
		}
		return reviewStopTransition("manual_intervention_required")
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
		return reviewStopTransition("manual_intervention_required")
	}
}

// reviewProviderRoleInputName renders the published collection-input name for
// one provider role. Input names are contract identifiers, not role tokens:
// the published transition_input schema (status-v5.schema.json) and gentle-pi's
// runtime decoder both pin them to ^[a-z0-9_]+$, so the hyphen the
// targeted-validator role token carries must project to an underscore here
// (cross-lane battery finding; the completed-input echo already published
// "provider_targeted_validator").
func reviewProviderRoleInputName(role reviewProviderRole) string {
	return "provider_" + strings.ReplaceAll(string(role), "-", "_")
}

func reviewProviderRoleTransition(reason string, binding ReviewTransitionBinding, role reviewProviderRole, runtime model.AgentID, validation *reviewtransaction.TargetedValidationRequest) ReviewNextTransition {
	if reviewProviderHostRelayMaterializeRuntime(runtime) || reviewProviderCaptureRuntime(runtime) {
		input, err := reviewProviderHostRelayRoleInput(binding, role, runtime, validation)
		if err != nil {
			return reviewStopTransition("captured_artifacts_unverifiable")
		}
		return reviewCollectTransition(reason, input)
	}
	task, err := newReviewProviderTask(role, binding)
	if err != nil {
		return reviewStopTransition("captured_artifacts_unverifiable")
	}
	return reviewCollectTransition(reason, ReviewTransitionInput{
		Name: reviewProviderRoleInputName(role), Schema: reviewProviderRoleTaskSchema(role), CaptureOperation: "external.run_provider_role",
		Arguments: append(reviewBindingArguments(binding),
			reviewRepositoryContextArguments(binding)[0],
			ReviewTransitionArgument{Name: "agent", Value: string(model.AgentOpenCode)},
			ReviewTransitionArgument{Name: "role", Value: string(role)}),
		ProviderTask: &task,
	})
}

// reviewCaptureRefuterCaptureOperation and its validation twin name the two
// native non-lens role capture operations every Go-owned provider runtime collects through.
// Like reviewCaptureResultCaptureOperation they are the single wording source:
// the collect transition, its submission operation token, and the runnable
// CLI verb all derive from the same constants.
const (
	reviewCaptureRefuterCaptureOperation    = "review.capture-refuter"
	reviewCaptureValidationCaptureOperation = "review.capture-validation"
)

// reviewProviderHostRelayRoleInput renders the one pi host-relay collection
// input for a Go-issued non-lens provider role. The vector is self-contained:
// its --execute form materializes the role request in Go, runs the Go-owned
// locked-down pi process on it, and admits the raw bytes -- so the rendered
// arguments themselves advance authority and no submission descriptor exists
// for a caller to author a verdict through.
func reviewProviderHostRelayRoleInput(binding ReviewTransitionBinding, role reviewProviderRole, runtime model.AgentID, validation *reviewtransaction.TargetedValidationRequest) (ReviewTransitionInput, error) {
	if binding.LineageID == "" || !providerSHA256(binding.Revision) || !providerSHA256(binding.TargetIdentity) ||
		reviewtransaction.ValidateReviewRepositoryContextHandle(binding.RepositoryContext) != nil {
		return ReviewTransitionInput{}, errors.New("provider role host-relay binding is incomplete") // refusal:by-design world-action: only a Go-issued STATUS transition may bind a host-relay provider role input
	}
	arguments := append(reviewBindingArguments(binding),
		reviewRepositoryContextArguments(binding)...)
	input := ReviewTransitionInput{Name: reviewProviderRoleInputName(role)}
	switch role {
	case reviewerprovider.RoleRefuter:
		input.Schema = reviewRefuterSchemaID
		input.CaptureOperation = reviewCaptureRefuterCaptureOperation
	case reviewerprovider.RoleTargetedValidator:
		if validation == nil {
			return ReviewTransitionInput{}, errors.New("provider targeted validator host-relay input requires the frozen validation request") // refusal:by-design world-action: only STATUS can bind the frozen correction request
		}
		arguments = append(arguments, ReviewTransitionArgument{Name: "request-hash", Value: validation.RequestHash})
		input.Schema = reviewValidatorSchemaID
		input.CaptureOperation = reviewCaptureValidationCaptureOperation
		input.ValidationRequest = validation
	default:
		return ReviewTransitionInput{}, fmt.Errorf("unsupported provider role %q", role) // refusal:by-design world-action: the pi host relay may collect only compiled provider roles
	}
	input.Arguments = append(arguments,
		ReviewTransitionArgument{Name: "agent", Value: string(runtime)},
		ReviewTransitionArgument{Name: "execute", Value: "true"})
	return input, nil
}

func reviewMissingCaptureTransition(binding ReviewTransitionBinding, selectedLenses []string, artifacts []ReviewTransitionArtifact, context *reviewCaptureContext, runtime ...model.AgentID) ReviewNextTransition {
	providerRuntime := model.AgentID("")
	if len(runtime) > 0 && (reviewProviderCaptureRuntime(runtime[0]) || reviewProviderHostRelayMaterializeRuntime(runtime[0])) {
		providerRuntime = runtime[0]
	}
	captured := make(map[int]bool, len(artifacts))
	for _, artifact := range artifacts {
		captured[artifact.SelectedOrder] = true
	}
	inputs := make([]ReviewTransitionInput, 0)
	for order, lens := range selectedLenses {
		if !captured[order] {
			inputs = append(inputs, reviewCaptureInput(binding, lens, order, context, providerRuntime))
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
// human-runnable command name from this same constant so a closure refusal
// naming the continuation can never drift from what the collect form itself
// already emits.
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

func reviewCaptureInput(binding ReviewTransitionBinding, lens string, order int, context *reviewCaptureContext, runtime ...model.AgentID) ReviewTransitionInput {
	arguments := reviewBindingArguments(binding)
	if binding.RepositoryContext != "" {
		arguments = append(arguments, reviewRepositoryContextArguments(binding)...)
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
	if len(runtime) > 0 {
		switch {
		case reviewProviderCaptureRuntime(runtime[0]):
			input.Arguments = append(input.Arguments, ReviewTransitionArgument{Name: "agent", Value: string(runtime[0])})
		case reviewProviderHostRelayMaterializeRuntime(runtime[0]):
			// The Pi host relay learns the whole flow from this one input: the
			// materialize arguments are only the prelude that prints the
			// Go-issued opaque prompt bytes for its fresh locked-down reviewer
			// subprocess, and the submission descriptor -- the same binding and
			// runtime tokens with the raw result substituted into --input -- is
			// what actually advances reviewing authority. Keeping the runtime in
			// the provider-owned submission lets a terminal closure issue its exact
			// runtime-bound STATUS continuation without host reconstruction.
			// Snapshot only the binding arguments; runtime and materialize are appended after the submission is complete.
			bindingArguments := input.Arguments
			tokens := make([]string, 0, len(bindingArguments)+2)
			for _, argument := range bindingArguments {
				tokens = append(tokens, reviewTransitionArgumentToken(argument))
			}
			tokens = append(tokens,
				reviewTransitionArgumentToken(ReviewTransitionArgument{Name: "agent", Value: string(runtime[0])}),
				"--input="+reviewSubmissionValuePlaceholder,
			)
			input.Submission = &ReviewTransitionSubmission{
				OperationToken: "capture-result", ArgumentTokens: tokens,
				Value: &ReviewTransitionSubmissionValue{
					Slot: "reviewer_result", Domain: "artifact_path_or_stdin", Schema: reviewReviewerSchemaID,
					SubstitutionLocation: len(tokens) - 1,
				},
			}
			input.Arguments = append(input.Arguments,
				ReviewTransitionArgument{Name: "agent", Value: string(runtime[0])},
				ReviewTransitionArgument{Name: "materialize", Value: "true"})
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
	ProviderRole                                   reviewProviderRole
	CapturedProviderTargetedValidator              bool
	CapturedProviderTargetedValidatorInconclusive  bool
	Contract                                       string
	RepositoryContext                              string
	Acknowledgement                                *reviewtransaction.ApprovedCompactAcknowledgement
	ValidationRequest                              *reviewtransaction.TargetedValidationRequest
	CorrectionRequest                              *reviewtransaction.CorrectionPlanRequest
	CorrectionForecasted                           bool
	CaptureContext                                 *reviewCaptureContext
	Selector                                       *reviewTransitionSelector
	IntendedUntracked                              reviewIntendedUntrackedScope
	RDDMode                                        reviewtransaction.RDDModeStatus
	RDDModeResolved                                bool
	LensContextBudgetExceeded                      bool
}

const reviewSubmissionValuePlaceholder = "{{value}}"

func reviewCorrectionPlanSubmission(contract string, binding ReviewTransitionBinding, request reviewtransaction.CorrectionPlanRequest) *ReviewTransitionSubmission {
	if contract != ReviewIntegrationContractV2 || binding.RepositoryContext == "" {
		return nil
	}
	return &ReviewTransitionSubmission{
		OperationToken: "capture-correction-plan",
		ArgumentTokens: []string{
			reviewTransitionArgumentToken(ReviewTransitionArgument{Name: "lineage", Value: binding.LineageID}),
			reviewTransitionArgumentToken(ReviewTransitionArgument{Name: "expected-revision", Value: binding.Revision}),
			reviewTransitionArgumentToken(ReviewTransitionArgument{Name: "target", Value: binding.TargetIdentity}),
			reviewTransitionArgumentToken(ReviewTransitionArgument{Name: "request-hash", Value: request.RequestHash}),
			reviewTransitionArgumentToken(ReviewTransitionArgument{Name: "repository-context", Value: binding.RepositoryContext}),
			"--correction-lines=" + reviewSubmissionValuePlaceholder,
		},
		Value: &ReviewTransitionSubmissionValue{
			Slot: "correction_lines", Domain: "positive_correction_lines", Minimum: 1,
			Maximum: request.CorrectionBudget, SubstitutionLocation: 5,
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

// reviewStartStatusContinuation is the provider-issued re-entry a reviewing
// negotiated START carries (issue #3894): the exact follow-up STATUS
// invocation for the frozen scope, rendered from frozen authority facts
// rather than a caller's remembered selector spelling. Its scope selectors
// are echoed as byte-identical tokenized rows in selector_arguments so a
// consumer replays them without re-deriving any spelling. It deliberately
// carries no --cwd token: a negotiated START payload publishes no filesystem
// path, and the caller runs the command in the repository it already holds.
// It does carry the opaque repository context START published (issue #3932),
// so a process cwd that does not hold this lineage fails closed instead of
// silently preflighting a fresh target in whatever repository it found.
func reviewStartStatusContinuation(state reviewtransaction.CompactState, revision string, runtime model.AgentID, repositoryContext string) *ReviewNextTransition {
	arguments := []ReviewTransitionArgument{
		{Name: "contract", Value: ReviewIntegrationContractV2},
		{Name: "next-transition", Value: "true"},
		{Name: "lineage", Value: state.LineageID},
		{Name: "repository-context", Value: repositoryContext},
	}
	if runtime != "" {
		arguments = append(arguments, ReviewTransitionArgument{Name: "agent", Value: string(runtime)})
	}
	var selectors []ReviewTransitionArgument
	switch state.InitialSnapshot.Kind {
	case reviewtransaction.TargetBaseDiff:
		selectors = []ReviewTransitionArgument{
			{Name: "base-ref", Value: state.InitialSnapshot.BaseTree},
			{Name: "committed-only", Value: "true"},
		}
	case reviewtransaction.TargetCurrentChanges:
		// A frozen workspace snapshot stores no explicit projection; the
		// re-entry must still name one so the consumer replays the exact scope.
		projection := state.InitialSnapshot.Projection
		if projection == "" {
			projection = reviewtransaction.ProjectionWorkspace
		}
		selectors = []ReviewTransitionArgument{{Name: "projection", Value: string(projection)}}
	case reviewtransaction.TargetBaseWorkspaceOverlay:
		selectors = []ReviewTransitionArgument{
			{Name: "base-ref", Value: state.InitialSnapshot.BaseTree},
			{Name: "workspace-overlay", Value: "true"},
		}
		if state.InitialSnapshot.Projection == reviewtransaction.ProjectionStaged {
			selectors = append(selectors, ReviewTransitionArgument{Name: "projection", Value: string(reviewtransaction.ProjectionStaged)})
		}
	default:
		return nil
	}
	arguments = append(arguments, selectors...)
	transition := reviewExecuteTransition("review_status_required", "review.status", arguments,
		[]ReviewTransitionArgument{{Name: "state", Value: string(reviewtransaction.StateReviewing)}},
		ReviewTransitionBinding{LineageID: state.LineageID, Revision: revision, TargetIdentity: state.InitialSnapshot.Identity}, nil,
	)
	tokenized := transition.Execute.Arguments
	transition.Execute.SelectorArguments = reviewTransitionSelectorArguments(tokenized[len(tokenized)-len(selectors):])
	return &transition
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
		selectorArguments, representable = input.Selector.recoveryArguments(input.IntendedUntracked)
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

func (selector reviewTransitionSelector) recoveryArguments(scope reviewIntendedUntrackedScope) ([]ReviewTransitionArgument, bool) {
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
			break
		}
		// Issue #1972: STATUS authorized a target derived from the caller's
		// declared untracked selection, so the rendered RECOVER must replay
		// that exact selection. Without it, `review recover` re-derives the
		// successor target from predecessor inheritance alone and refuses
		// its own authorization whenever the recovery-time selection
		// diverges from the predecessor's frozen declaration. A declared
		// selection without its validated inventory digest cannot be
		// replayed and fails closed instead of rendering a partial selector.
		if scope.Declared {
			if scope.Digest == "" {
				return nil, false
			}
			arguments = append(arguments,
				ReviewTransitionArgument{Name: "untracked-scope", Value: map[bool]string{true: "select", false: "exclude"}[len(target.IntendedUntracked) != 0]},
				ReviewTransitionArgument{Name: "expected-untracked-inventory", Value: scope.Digest})
			for _, path := range target.IntendedUntracked {
				arguments = append(arguments, ReviewTransitionArgument{Name: "intended-untracked", Value: path})
			}
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
		if target.Projection != reviewtransaction.ProjectionStaged {
			return nil, false
		}
		arguments = append(arguments,
			ReviewTransitionArgument{Name: "base-ref", Value: target.BaseRef},
			ReviewTransitionArgument{Name: "projection", Value: string(reviewtransaction.ProjectionStaged)},
			ReviewTransitionArgument{Name: "workspace-overlay", Value: "true"},
		)
		return arguments, true
	default:
		return nil, false
	}
	return arguments, true
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

// reviewInconclusiveTargetedValidationReason names the one retryable
// captured-validation outcome (issue #3378). It is deliberately distinct from
// targeted_validation_required so a consumer can tell "no validation exists
// yet" apart from "the captured one produced no verdict, nothing was
// consumed, and the validator's access to the frozen trees must be restored
// before running it again". Both collect the same input through the same
// capture operation and submission descriptor.
const reviewInconclusiveTargetedValidationReason = "targeted_validation_inconclusive_recapture_required"

func reviewTransitionBinding(authority *ReviewTargetStatusAuthority, target, repositoryRoot string, repositoryContext ...string) ReviewTransitionBinding {
	contextHandle := ""
	if len(repositoryContext) > 0 {
		contextHandle = repositoryContext[0]
	}
	return ReviewTransitionBinding{
		LineageID: authority.LineageID, Revision: authority.Revision, TargetIdentity: target,
		RepositoryContext: contextHandle, RepositoryRoot: repositoryRoot,
	}
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
	case "native_low_risk_verification":
		return "Low risk candidate eligible for native verification"
	case "lineage_selection_required":
		return "Multiple lineages match target; select an explicit lineage"
	case "reviewer_results_required":
		return "Reviewer lens artifacts required for current revision"
	case "targeted_validation_required":
		return "Targeted validation run required for correction plan"
	case reviewInconclusiveTargetedValidationReason:
		return "Captured targeted validation produced no verdict and consumed no correction attempt; restore validator access to the frozen candidate and run it again"
	case "correction_plan_required":
		return "Correction plan required to resolve review findings"
	case "delivery_gate_required":
		return "Delivery gate selection required before validation"
	case "staged_workspace_overlay_recovery_unavailable":
		return "Staged workspace overlay recovery is unavailable"
	case "empty_base_diff_bootstrap_required":
		return "Committed base-diff has no paths; empty-root bootstrap is required"
	case "lens_context_budget_exceeded":
		return "Frozen reviewer context exceeds the native evidence budget"
	case "corrupted_or_unverifiable_authority":
		return "Review authority is corrupted or unverifiable"
	case "missing_authority_binding":
		return "Target authority binding is missing"
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
