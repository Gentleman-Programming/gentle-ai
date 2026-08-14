package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"path"
	"reflect"
	"strconv"
	"strings"

	"github.com/gentleman-programming/gentle-ai/v2/internal/model"
	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewerprovider"
	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
)

const ReviewIntegrationStatusSchemaV1 = "gentle-ai.review-integration.status/v1"
const ReviewIntegrationStatusSchemaIDV1 = "https://gentle-ai.dev/contracts/review-integration/v1/schemas/status.schema.json"
const ReviewIntegrationStatusSchemaV2 = "gentle-ai.review-integration.status/v2"
const ReviewIntegrationStatusSchemaIDV2 = "https://gentle-ai.dev/contracts/review-integration/v1/schemas/status-v2.schema.json"
const ReviewIntegrationStatusSchemaV3 = "gentle-ai.review-integration.status/v3"
const ReviewIntegrationStatusSchemaIDV3 = "https://gentle-ai.dev/contracts/review-integration/v2/schemas/status.schema.json"
const ReviewIntegrationStatusSchemaV4 = "gentle-ai.review-integration.status/v4"
const ReviewIntegrationStatusSchemaIDV4 = "https://gentle-ai.dev/contracts/review-integration/v2/schemas/status-v4.schema.json"
const ReviewIntegrationStatusSchemaV5 = "gentle-ai.review-integration.status/v5"
const ReviewIntegrationStatusSchemaIDV5 = "https://gentle-ai.dev/contracts/review-integration/v2/schemas/status-v5.schema.json"
const ReviewIntegrationStatusSchema = ReviewIntegrationStatusSchemaV5
const ReviewIntegrationStatusSchemaID = ReviewIntegrationStatusSchemaIDV5
const ReviewIntegrationProjectionSchema = "gentle-ai.review-integration.projection/v1"
const ReviewIntegrationProjectionSchemaID = "https://gentle-ai.dev/contracts/review-integration/v1/schemas/projection.schema.json"

type ReviewReceiptStatus string

const (
	ReviewReceiptExpectedMissing    ReviewReceiptStatus = "expected_missing"
	ReviewReceiptPresent            ReviewReceiptStatus = "present"
	ReviewReceiptPublicationPending ReviewReceiptStatus = "publication_pending"
	ReviewReceiptNotApplicable      ReviewReceiptStatus = "not_applicable"
)

type ReviewForecastHorizon string

const (
	ForecastHorizonPartial  ReviewForecastHorizon = "partial"
	ForecastHorizonTerminal ReviewForecastHorizon = "terminal"
)

type ReviewForecastItem struct {
	Step        int    `json:"step"`
	Kind        string `json:"kind"`
	ReasonCode  string `json:"reason_code"`
	Description string `json:"description"`
}

type ReviewForecast struct {
	Horizon ReviewForecastHorizon `json:"horizon"`
	Steps   []ReviewForecastItem  `json:"steps"`
}

type ReviewTargetStatusResult struct {
	Schema        string                                `json:"schema"`
	Contract      string                                `json:"contract"`
	Operation     string                                `json:"operation"`
	Applicability reviewtransaction.TargetApplicability `json:"applicability"`
	Authority     *ReviewTargetStatusAuthority          `json:"authority,omitempty"`
	Receipt       ReviewTargetStatusReceipt             `json:"receipt"`
	Action        reviewtransaction.TargetStatusAction  `json:"action"`
	// ActionDisposition names the provider recovery class accepted by the
	// selected action. Generic recover and final-verification retry remain
	// distinct operations.
	ActionDisposition       reviewtransaction.RecoveryDisposition       `json:"action_disposition,omitempty"`
	Replayability           reviewtransaction.Replayability             `json:"replayability"`
	Frozen                  *ReviewTargetStatusFrozen                   `json:"frozen,omitempty"`
	TargetIdentity          string                                      `json:"target_identity"`
	AuthorityTargetIdentity string                                      `json:"authority_target_identity,omitempty"`
	Projection              ReviewTargetStatusProjection                `json:"projection"`
	Repair                  reviewtransaction.AuthorityRepairAssessment `json:"repair"`
	// Disposition is Wave 6's negotiated-route provider preview (rdd-closure-
	// disposition-execution / "Reachable Through the Negotiated Transition
	// Route"): populated only when Repair is not eligible but a closed
	// closure disposition plan derives and admits. It carries the same two
	// fields `review repair --preflight` already publishes for this route
	// (ReviewRepairDispositionProviderInputs) — nothing a maintainer could
	// not already derive read-only, and nothing that changes without a real
	// authorization.
	Disposition            *ReviewRepairDispositionProviderInputs               `json:"disposition,omitempty"`
	Candidates             []string                                             `json:"candidates"`
	Reconciliation         *ReviewFinalizeReconciliation                        `json:"reconciliation,omitempty"`
	Eligibility            *ReviewActionEligibility                             `json:"eligibility,omitempty"`
	Forecast               *ReviewForecast                                      `json:"forecast,omitempty"`
	NextTransition         *ReviewNextTransition                                `json:"next_transition,omitempty"`
	RepositoryContext      *ReviewRepositoryContextReference                    `json:"repository_context,omitempty"`
	ValidationRequest      *reviewtransaction.TargetedValidationRequest         `json:"validation_request,omitempty"`
	FinalVerificationRetry *reviewtransaction.FinalVerificationRetryEligibility `json:"final_verification_retry,omitempty"`
	decision               reviewtransaction.TargetStatusDecision               `json:"-"`
	intendedUntracked      reviewIntendedUntrackedScope
	repositoryRoot         string
	rddMode                reviewtransaction.RDDModeStatus
	rddModeResolved        bool
}

// ReviewActionEligibility remains an additive compatibility detail for older
// consumers. A negotiated next_transition is the sole routing authority.
type ReviewActionEligibility struct {
	AllowedActions   []ReviewEligibleAction  `json:"allowed_actions"`
	ForbiddenActions []ReviewForbiddenAction `json:"forbidden_actions"`
}

type ReviewEligibleAction struct {
	Action         string                                `json:"action"`
	ReasonCode     string                                `json:"reason_code"`
	RequiredInputs []string                              `json:"required_inputs"`
	Disposition    reviewtransaction.RecoveryDisposition `json:"disposition,omitempty"`
	Binding        *ReviewActionBinding                  `json:"binding,omitempty"`
}

type ReviewForbiddenAction struct {
	Action     string `json:"action"`
	ReasonCode string `json:"reason_code"`
}

// ReviewActionBinding is a proof reference, not an authorization template.
// It is emitted only for a natively eligible maintainer-authorized recovery.
type ReviewActionBinding struct {
	LineageID      string `json:"lineage_id"`
	Revision       string `json:"revision"`
	TargetIdentity string `json:"target_identity"`
}

var reviewManagedActions = []string{
	"review.abandon",
	"review.finalize",
	"review.invalidate",
	"review.quarantine-legacy",
	"review.reclaim",
	"review.reconcile-authority",
	"review.reconcile-authority-batch",
	"review.recover",
	"review.repair",
	ReviewIntegrationOperationRetryFinalVerification,
	"review.start",
	"review.validate",
}

// reviewFinalizeManagedActions preserves the published operation/v1 action
// eligibility surface. Classified repair is advertised only by status/v2.
var reviewFinalizeManagedActions = []string{
	"review.abandon",
	"review.finalize",
	"review.invalidate",
	"review.quarantine-legacy",
	"review.reclaim",
	"review.reconcile-authority",
	"review.reconcile-authority-batch",
	"review.recover",
	"review.start",
	"review.validate",
}

const (
	reviewActionEligibleCurrent                = "eligible_current_target"
	reviewActionEligibleEscalatedRecovery      = "eligible_recovery_escalated"
	reviewActionEligibleRecovery               = "eligible_recovery"
	reviewActionEligibleClassifiedRepair       = "eligible_classified_authority_repair"
	reviewActionEligibleFinalVerificationRetry = "eligible_final_verification_retry"
	reviewActionForbiddenNotSelected           = "forbidden_not_selected_by_native_status"
	reviewActionForbiddenAmbiguous             = "forbidden_ambiguous_authority"
	reviewActionForbiddenCorrupted             = "forbidden_corrupted_authority"
	reviewActionForbiddenUnrelated             = "forbidden_unrelated_target"
	reviewActionForbiddenTerminalEscalated     = "forbidden_terminal_escalated_authority"
	reviewActionForbiddenUnchangedEscalated    = "forbidden_unchanged_escalated_candidate"
	reviewActionForbiddenManualIntervention    = "forbidden_manual_intervention_required"
	reviewActionForbiddenReconciliation        = "forbidden_reconciliation_requires_exact_request"
	reviewActionForbiddenInputsUnavailable     = "forbidden_required_inputs_unavailable"
	reviewActionForbiddenFinalizeStatus        = "forbidden_finalize_requires_target_status"
	reviewActionForbiddenRDDDisabled           = "forbidden_rdd_disabled"
)

type ReviewFinalizeReconciliation struct {
	Required bool `json:"required"`
}

type ReviewTargetStatusAuthority struct {
	Version    reviewtransaction.AuthorityVersion `json:"version"`
	LineageID  string                             `json:"lineage_id"`
	State      reviewtransaction.State            `json:"state"`
	Generation int                                `json:"generation"`
	Revision   string                             `json:"revision"`
}

type ReviewTargetStatusReceipt struct {
	Status   ReviewReceiptStatus `json:"status"`
	Identity string              `json:"identity,omitempty"`
}

type ReviewTargetStatusFrozen struct {
	Tier                 reviewtransaction.RiskLevel `json:"tier"`
	OriginalChangedLines int                         `json:"original_changed_lines"`
	CorrectionBudget     int                         `json:"correction_budget"`
}

type ReviewTargetStatusProjection struct {
	Schema                  string                       `json:"schema"`
	Kind                    reviewtransaction.TargetKind `json:"kind"`
	Projection              reviewtransaction.Projection `json:"projection"`
	BaseTree                string                       `json:"base_tree"`
	InitialReviewTree       string                       `json:"initial_review_tree"`
	CurrentCandidateTree    string                       `json:"current_candidate_tree"`
	PathsDigest             string                       `json:"paths_digest"`
	Paths                   []string                     `json:"paths"`
	IntendedUntracked       []string                     `json:"intended_untracked"`
	IntendedUntrackedProof  string                       `json:"intended_untracked_proof"`
	InitialSnapshotIdentity string                       `json:"initial_snapshot_identity"`
	CurrentSnapshotIdentity string                       `json:"current_snapshot_identity"`
}

func newReviewTargetStatusResultForContract(native reviewtransaction.TargetStatusResult, contract string) ReviewTargetStatusResult {
	schema := ReviewIntegrationStatusSchema
	if contract == ReviewIntegrationContractV1 {
		schema = ReviewIntegrationStatusSchemaV2
	}
	result := ReviewTargetStatusResult{
		Schema: schema, Contract: contract, Operation: "review.status",
		Applicability: native.Applicability, Action: native.Action, ActionDisposition: native.ActionDisposition,
		Replayability:  native.Replayability,
		TargetIdentity: native.TargetIdentity,
		decision:       native.Decision,
		Candidates:     append([]string{}, native.CandidateLineageIDs...),
		Repair:         reviewtransaction.UnsupportedAuthorityRepairAssessment(),
		Projection: ReviewTargetStatusProjection{
			Schema: ReviewIntegrationProjectionSchema, Kind: native.Projection.Kind, Projection: facadeProjection(native.Projection.Projection),
			BaseTree: native.Projection.BaseTree, InitialReviewTree: native.Projection.InitialReviewTree,
			CurrentCandidateTree: native.Projection.CurrentCandidateTree, PathsDigest: native.Projection.PathsDigest,
			Paths: append([]string{}, native.Projection.Paths...), IntendedUntracked: append([]string{}, native.Projection.IntendedUntracked...),
			IntendedUntrackedProof:  native.Projection.IntendedUntrackedProof,
			InitialSnapshotIdentity: native.Projection.InitialSnapshotIdentity, CurrentSnapshotIdentity: native.Projection.CurrentSnapshotIdentity,
		},
		Receipt: ReviewTargetStatusReceipt{Status: ReviewReceiptNotApplicable},
	}
	if native.AuthorityVersion == reviewtransaction.AuthorityVersionCompact &&
		native.AuthorityTargetIdentity != "" && native.AuthorityTargetIdentity != native.TargetIdentity {
		result.AuthorityTargetIdentity = native.AuthorityTargetIdentity
	}
	if native.FinalVerificationRetry != nil {
		eligibility := *native.FinalVerificationRetry
		result.FinalVerificationRetry = &eligibility
	}
	if native.Applicability != reviewtransaction.TargetApplicabilityCurrent {
		return result
	}
	if native.Action == reviewtransaction.TargetStatusActionReconcileFinalize {
		result.Reconciliation = &ReviewFinalizeReconciliation{Required: true}
	}
	result.Authority = &ReviewTargetStatusAuthority{
		Version: native.AuthorityVersion, LineageID: native.LineageID, State: native.State,
		Generation: native.Generation, Revision: native.Revision,
	}
	if native.AuthorityVersion == reviewtransaction.AuthorityVersionCompact {
		correctionBudget, _ := reviewtransaction.CorrectionBudget(native.OriginalChangedLines)
		result.Frozen = &ReviewTargetStatusFrozen{
			Tier: native.Tier, OriginalChangedLines: native.OriginalChangedLines, CorrectionBudget: correctionBudget,
		}
	}
	if native.ReceiptIdentity != "" {
		result.Receipt = ReviewTargetStatusReceipt{Status: ReviewReceiptPresent, Identity: native.ReceiptIdentity}
	} else if native.AuthorityVersion == reviewtransaction.AuthorityVersionCompact &&
		(native.State == reviewtransaction.StateApproved || native.State == reviewtransaction.StateEscalated) {
		result.Receipt = ReviewTargetStatusReceipt{Status: ReviewReceiptPublicationPending}
	} else {
		result.Receipt = ReviewTargetStatusReceipt{Status: ReviewReceiptExpectedMissing}
	}
	return result
}

func newReviewActionEligibility(status ReviewTargetStatusResult) *ReviewActionEligibility {
	allowed := ReviewEligibleAction{RequiredInputs: []string{}}
	switch status.Action {
	case reviewtransaction.TargetStatusActionStart:
		if status.rddModeResolved && !status.rddMode.Enabled() {
			allowed.Action, allowed.ReasonCode = "stop", reviewActionForbiddenRDDDisabled
		} else {
			allowed.Action, allowed.ReasonCode = "review.start", reviewActionEligibleCurrent
		}
	case reviewtransaction.TargetStatusActionValidate:
		allowed.Action, allowed.ReasonCode = "stop", reviewActionForbiddenInputsUnavailable
	case reviewtransaction.TargetStatusActionFinalize:
		if status.Replayability == reviewtransaction.ReplayabilityExactReplaySafe {
			allowed.Action, allowed.ReasonCode, allowed.RequiredInputs = "review.finalize", reviewActionEligibleCurrent, []string{"lineage_id"}
		} else {
			allowed.Action, allowed.ReasonCode = "stop", reviewActionForbiddenInputsUnavailable
		}
	case reviewtransaction.TargetStatusActionReconcileFinalize:
		allowed.Action, allowed.ReasonCode = "stop", reviewActionForbiddenReconciliation
	case reviewtransaction.TargetStatusActionRecover:
		allowed.Action, allowed.Disposition = "review.recover", status.ActionDisposition
		allowed.RequiredInputs = []string{"predecessor_lineage", "expected_predecessor_revision", "successor_lineage", "disposition", "reason", "actor", "maintainer_authorization"}
		allowed.ReasonCode = reviewActionEligibleRecovery
		if status.ActionDisposition == reviewtransaction.RecoveryEscalated {
			allowed.ReasonCode = reviewActionEligibleEscalatedRecovery
		}
		if status.Authority != nil {
			allowed.Binding = &ReviewActionBinding{
				LineageID: status.Authority.LineageID,
				Revision:  status.Authority.Revision, TargetIdentity: status.TargetIdentity,
			}
		}
	case reviewtransaction.TargetStatusActionRetryFinalVerification:
		allowed.Action = ReviewIntegrationOperationRetryFinalVerification
		allowed.ReasonCode = reviewActionEligibleFinalVerificationRetry
		allowed.Disposition = reviewtransaction.RecoveryFinalVerificationRetry
		allowed.RequiredInputs = []string{"predecessor_lineage", "expected_predecessor_revision", "successor_lineage", "incident", "actor", "reason", "maintainer_authorization"}
		if status.Authority != nil {
			allowed.Binding = &ReviewActionBinding{
				LineageID: status.Authority.LineageID,
				Revision:  status.Authority.Revision, TargetIdentity: reviewAuthorityTargetIdentity(status),
			}
		}
	case reviewtransaction.TargetStatusActionRepairAuthority:
		if status.Repair.Status == reviewtransaction.AuthorityRepairEligible && status.Repair.Candidate != nil {
			allowed.Action, allowed.ReasonCode = "review.repair", reviewActionEligibleClassifiedRepair
			allowed.RequiredInputs = []string{"actor", "reason", "maintainer_authorization"}
		} else {
			allowed.Action, allowed.ReasonCode = "stop", reviewActionForbiddenManualIntervention
		}
	default:
		allowed.Action, allowed.ReasonCode = "stop", reviewActionForbiddenManualIntervention
	}
	forbiddenReason := reviewActionForbiddenNotSelected
	switch {
	case status.Applicability == reviewtransaction.TargetApplicabilityAmbiguous:
		forbiddenReason = reviewActionForbiddenAmbiguous
	case status.Applicability == reviewtransaction.TargetApplicabilityCorrupted:
		forbiddenReason = reviewActionForbiddenCorrupted
	case status.Applicability == reviewtransaction.TargetApplicabilityUnrelated:
		forbiddenReason = reviewActionForbiddenUnrelated
	case status.Action == reviewtransaction.TargetStatusActionStop && status.Authority != nil && status.Authority.State == reviewtransaction.StateEscalated:
		forbiddenReason = reviewActionForbiddenTerminalEscalated
	case status.Action == reviewtransaction.TargetStatusActionStop && status.Authority != nil && status.Authority.State == reviewtransaction.StateCorrectionRequired:
		forbiddenReason = reviewActionForbiddenUnchangedEscalated
	case status.Action == reviewtransaction.TargetStatusActionReconcileFinalize:
		forbiddenReason = reviewActionForbiddenReconciliation
	case status.Action == reviewtransaction.TargetStatusActionStart && status.rddModeResolved && !status.rddMode.Enabled():
		forbiddenReason = reviewActionForbiddenRDDDisabled
	case allowed.Action == "stop" && allowed.ReasonCode == reviewActionForbiddenInputsUnavailable:
		forbiddenReason = reviewActionForbiddenInputsUnavailable
	}
	forbidden := make([]ReviewForbiddenAction, 0, len(reviewManagedActions))
	for _, action := range reviewManagedActions {
		if action != allowed.Action {
			forbidden = append(forbidden, ReviewForbiddenAction{Action: action, ReasonCode: forbiddenReason})
		}
	}
	return &ReviewActionEligibility{AllowedActions: []ReviewEligibleAction{allowed}, ForbiddenActions: forbidden}
}

func reviewStopEligibility(reason string, requiredInputs []string) *ReviewActionEligibility {
	forbidden := make([]ReviewForbiddenAction, len(reviewFinalizeManagedActions))
	for index, action := range reviewFinalizeManagedActions {
		forbidden[index] = ReviewForbiddenAction{Action: action, ReasonCode: reason}
	}
	return &ReviewActionEligibility{
		AllowedActions:   []ReviewEligibleAction{{Action: "stop", ReasonCode: reason, RequiredInputs: requiredInputs}},
		ForbiddenActions: forbidden,
	}
}

type reviewStatusCompactAuthority struct {
	OriginalChangedLines   int
	CorrectionBudget       int
	CorrectionBudgetPolicy string
}

func (result ReviewTargetStatusResult) Validate() error {
	return result.validateWithCompactAuthority(nil)
}

func (result ReviewTargetStatusResult) validateWithCompactAuthority(authority *reviewStatusCompactAuthority) error {
	legacyTransport := result.Schema == ReviewIntegrationStatusSchemaV2 && result.Contract == ReviewIntegrationContractV1
	nativeGitTransport := (result.Schema == ReviewIntegrationStatusSchemaV3 || result.Schema == ReviewIntegrationStatusSchemaV4 || result.Schema == ReviewIntegrationStatusSchemaV5) && result.Contract == ReviewIntegrationContractV2
	if (!legacyTransport && !nativeGitTransport) || result.Operation != "review.status" {
		return errors.New("invalid negotiated review status identity")
	}
	if !validReviewCapabilitySHA256(result.TargetIdentity) || result.Candidates == nil {
		return errors.New("invalid negotiated review target identity")
	}
	if result.RepositoryContext != nil {
		if result.RepositoryContext.Capability != reviewtransaction.ReviewRepositoryContextCapability ||
			reviewtransaction.ValidateReviewRepositoryContextHandle(result.RepositoryContext.Handle) != nil ||
			!validReviewCapabilitySHA256(result.RepositoryContext.Revision) ||
			!validReviewCapabilitySHA256(result.RepositoryContext.TargetIdentity) ||
			validateReviewRepositoryContextReference(*result.RepositoryContext) != nil ||
			result.Authority == nil || result.RepositoryContext.Revision != result.Authority.Revision ||
			result.RepositoryContext.TargetIdentity != reviewAuthorityTargetIdentity(result) {
			return errors.New("negotiated STATUS repository context is invalid") // refusal:by-design world-action: the provider-built envelope is internally inconsistent and requires a code fix
		}
	}
	if err := result.Repair.Validate(); err != nil {
		return err
	}
	if result.Repair.Status == reviewtransaction.AuthorityRepairEligible &&
		(result.Applicability != reviewtransaction.TargetApplicabilityCorrupted || result.Action != reviewtransaction.TargetStatusActionRepairAuthority) {
		return errors.New("eligible authority repair is not bound to corrupted status")
	}
	if err := result.Projection.Validate(); err != nil {
		return err
	}
	if result.TargetIdentity != result.Projection.CurrentSnapshotIdentity {
		return errors.New("negotiated review target identity differs from its current projection")
	}
	if retry := result.FinalVerificationRetry; retry != nil {
		if result.Applicability != reviewtransaction.TargetApplicabilityCurrent || result.Authority == nil ||
			result.Authority.Version != reviewtransaction.AuthorityVersionCompact || result.Authority.State != reviewtransaction.StateEscalated ||
			result.Action != reviewtransaction.TargetStatusActionRetryFinalVerification || result.ActionDisposition != reviewtransaction.RecoveryFinalVerificationRetry ||
			retry.IncidentSchema != reviewtransaction.FinalVerificationIncidentSchema ||
			retry.IncidentClass != reviewtransaction.FinalVerificationIncidentProceduralToolingFailure ||
			retry.TargetIdentity != reviewAuthorityTargetIdentity(result) || !validReviewCapabilitySHA256(retry.ValidatingRevision) ||
			!validReviewCapabilitySHA256(retry.FailedEvidenceHash) ||
			retry.FailedEvidenceRecordDigest != "" && !validReviewCapabilitySHA256(retry.FailedEvidenceRecordDigest) ||
			!validReviewCapabilitySHA256(retry.FinalizeRequestDigest) {
			return errors.New("final-verification retry status metadata is invalid")
		}
	} else if result.Action == reviewtransaction.TargetStatusActionRetryFinalVerification {
		return errors.New("final-verification retry action lacks provider eligibility")
	}
	if result.Eligibility != nil {
		if err := result.Eligibility.Validate(result); err != nil {
			return err
		}
	}
	if result.NextTransition != nil {
		if err := result.NextTransition.Validate(); err != nil {
			return err
		}
		if err := result.validateNextTransitionTargets(); err != nil {
			return err
		}
		if err := result.validateSubmissionDescriptors(); err != nil {
			return err
		}
		transitionRequest := reviewTransitionValidationRequest(result.NextTransition)
		correctionEvidenceFirst := transitionRequest == nil && result.ValidationRequest != nil &&
			(result.NextTransition.ReasonCode == "correction_repository_verification_required" ||
				result.NextTransition.ReasonCode == "correction_repository_tooling_failed")
		if !correctionEvidenceFirst && ((transitionRequest == nil) != (result.ValidationRequest == nil) ||
			transitionRequest != nil && !reflect.DeepEqual(*transitionRequest, *result.ValidationRequest)) {
			return errors.New("negotiated status validation request copies differ")
		}
		if request := result.NextTransition.CorrectionRequest; request != nil {
			expectedBudget := 0
			if result.Frozen != nil {
				expectedBudget = result.Frozen.CorrectionBudget
			}
			if authority != nil {
				budget, budgetErr := reviewtransaction.CompactExpectedBudget(authority.OriginalChangedLines, authority.CorrectionBudgetPolicy)
				if budgetErr != nil || budget != authority.CorrectionBudget {
					return errors.New("native compact status budget is invalid") // refusal:by-design world-action: provider-generated status and persisted compact authority budget require a code fix when they disagree
				}
				expectedBudget = authority.CorrectionBudget
			}
			if result.Authority == nil || result.Frozen == nil || result.Authority.Version != reviewtransaction.AuthorityVersionCompact ||
				result.Authority.State != reviewtransaction.StateCorrectionRequired || request.LineageID != result.Authority.LineageID ||
				request.ExpectedRevision != result.Authority.Revision || request.TargetIdentity != reviewAuthorityTargetIdentity(result) ||
				request.CorrectionBudget != expectedBudget {
				return errors.New("negotiated status correction request binding is invalid") // refusal:by-design world-action: provider-generated status and request bindings require a code fix when they disagree
			}
		}
	}
	if result.Forecast != nil {
		if result.Contract != ReviewIntegrationContractV2 {
			return errors.New("forecast requires the v2 review integration contract") // refusal:by-design world-action: frozen v1 status cannot accept additive routing data
		}
		if result.NextTransition == nil {
			return errors.New("forecast without next_transition is invalid") // refusal:by-design world-action: status forecast requires next_transition
		}
		switch result.Forecast.Horizon {
		case ForecastHorizonPartial, ForecastHorizonTerminal:
		default:
			return fmt.Errorf("invalid forecast horizon %q", result.Forecast.Horizon) // refusal:by-design world-action: status forecast requires valid horizon
		}
		if len(result.Forecast.Steps) != 1 {
			return errors.New("forecast must contain exactly one step") // refusal:by-design world-action: status forecast is descriptive only
		}
		step := result.Forecast.Steps[0]
		if step.Step != 1 {
			return fmt.Errorf("forecast step must be 1, got %d", step.Step) // refusal:by-design world-action: forecast head must remain singular
		}
		switch step.Kind {
		case reviewNextTransitionExecute, reviewNextTransitionCollect, reviewNextTransitionStop:
		default:
			return fmt.Errorf("forecast step 1 has invalid kind %q", step.Kind) // refusal:by-design world-action: status forecast step kind must be valid
		}
		if strings.TrimSpace(step.ReasonCode) == "" {
			return errors.New("forecast step 1 has empty reason_code") // refusal:by-design world-action: status forecast step reason_code must not be empty
		}
		if strings.TrimSpace(step.Description) == "" {
			return errors.New("forecast step 1 has empty description") // refusal:by-design world-action: status forecast step description must not be empty
		}
		if step.Kind != result.NextTransition.Kind || step.ReasonCode != result.NextTransition.ReasonCode {
			return fmt.Errorf("forecast head (%s/%s) diverges from next_transition (%s/%s)", // refusal:by-design world-action: status forecast head must match next_transition
				step.Kind, step.ReasonCode, result.NextTransition.Kind, result.NextTransition.ReasonCode)
		}
		if result.NextTransition.Kind == reviewNextTransitionStop && result.Forecast.Horizon != ForecastHorizonTerminal {
			return errors.New("stop transition requires a terminal forecast") // refusal:by-design world-action: stop transition forecast must be terminal
		}
		if result.NextTransition.Kind != reviewNextTransitionStop && result.Forecast.Horizon != ForecastHorizonPartial {
			return errors.New("non-stop transition requires a partial forecast") // refusal:by-design world-action: callers must refresh status after action
		}
	}
	switch result.Applicability {
	case reviewtransaction.TargetApplicabilityCurrent:
		if result.Authority == nil || result.Authority.Generation < 1 ||
			!validReviewCapabilitySHA256(result.Authority.Revision) || strings.TrimSpace(result.Authority.LineageID) == "" || len(result.Candidates) != 0 {
			return errors.New("current-target status authority is incomplete")
		}
		switch result.Authority.Version {
		case reviewtransaction.AuthorityVersionCompact:
			if result.Frozen == nil || result.AuthorityTargetIdentity != "" && !validReviewCapabilitySHA256(result.AuthorityTargetIdentity) {
				return errors.New("compact current-target status requires frozen inputs")
			}
			if result.Frozen.Tier != reviewtransaction.RiskLow && result.Frozen.Tier != reviewtransaction.RiskMedium && result.Frozen.Tier != reviewtransaction.RiskHigh {
				return errors.New("current-target frozen tier is invalid")
			}
			if !reviewContractCorrectionBudgetValid(result.Frozen.OriginalChangedLines, result.Frozen.CorrectionBudget) {
				return errors.New("current-target frozen budget is invalid")
			}
		case reviewtransaction.AuthorityVersionLegacy:
			if result.Frozen != nil || result.AuthorityTargetIdentity != "" {
				return errors.New("legacy current-target status cannot contain compact frozen inputs")
			}
			if result.Receipt.Status == ReviewReceiptPublicationPending {
				return errors.New("legacy current-target status cannot use compact publication semantics")
			}
			if result.Authority.State == reviewtransaction.StateApproved && result.Receipt.Status != ReviewReceiptPresent {
				return errors.New("approved legacy current-target status requires a present receipt")
			}
		default:
			return errors.New("current-target authority version is unsupported")
		}
		if result.Receipt.Status == ReviewReceiptPresent && !validReviewCapabilitySHA256(result.Receipt.Identity) ||
			result.Receipt.Status != ReviewReceiptPresent && result.Receipt.Identity != "" {
			return errors.New("current-target receipt identity is invalid")
		}
		if result.Receipt.Status != ReviewReceiptPresent && result.Receipt.Status != ReviewReceiptExpectedMissing && result.Receipt.Status != ReviewReceiptPublicationPending {
			return errors.New("current-target receipt status is invalid")
		}
	case reviewtransaction.TargetApplicabilityUnrelated:
		// Candidates is intentionally NOT constrained to empty here: plural
		// stale (scope-changed) lineages report this exact
		// applicability/action/replayability shape (organic-dx Phase 3e —
		// nothing governs, so nothing decides) while still listing those
		// stale lineages as optional, discoverable recovery candidates. An
		// unrelated target with zero candidates remains equally valid.
		if result.Authority != nil || result.Frozen != nil || result.AuthorityTargetIdentity != "" || result.Receipt.Status != ReviewReceiptNotApplicable || result.Action != reviewtransaction.TargetStatusActionStart && !(result.Action == reviewtransaction.TargetStatusActionStop && result.Replayability == reviewtransaction.ReplayabilityManualActionRequired && ((result.Projection.Kind == reviewtransaction.TargetBaseWorkspaceOverlay && result.Projection.Projection == reviewtransaction.ProjectionStaged) || (result.Projection.Kind == reviewtransaction.TargetBaseDiff && len(result.Projection.Paths) == 0))) {
			return errors.New("unrelated target status is inconsistent")
		}
	case reviewtransaction.TargetApplicabilityAmbiguous:
		if result.Authority != nil || result.Frozen != nil || result.AuthorityTargetIdentity != "" || result.Receipt.Status != ReviewReceiptNotApplicable || result.Action != reviewtransaction.TargetStatusActionSelectLineage || len(result.Candidates) < 2 {
			return errors.New("ambiguous target status is inconsistent")
		}
	case reviewtransaction.TargetApplicabilityCorrupted:
		if result.Authority != nil || result.Frozen != nil || result.AuthorityTargetIdentity != "" || result.Receipt.Status != ReviewReceiptNotApplicable || result.Action != reviewtransaction.TargetStatusActionRepairAuthority {
			return errors.New("corrupted target status is inconsistent")
		}
	default:
		return errors.New("unsupported target applicability")
	}
	if strings.TrimSpace(string(result.Action)) == "" {
		return errors.New("negotiated review status requires exactly one action")
	}
	if result.Action == reviewtransaction.TargetStatusActionReconcileFinalize {
		if result.Applicability != reviewtransaction.TargetApplicabilityCurrent || result.Reconciliation == nil || !result.Reconciliation.Required || result.Replayability != reviewtransaction.ReplayabilityStatusRequired {
			return errors.New("pending finalize status requires current-target reconciliation")
		}
	} else if result.Reconciliation != nil {
		return errors.New("only pending finalize status may contain reconciliation")
	}
	switch result.Replayability {
	case reviewtransaction.ReplayabilityNotReplayable, reviewtransaction.ReplayabilityExactReplaySafe,
		reviewtransaction.ReplayabilityStatusRequired, reviewtransaction.ReplayabilityManualActionRequired:
	default:
		return errors.New("unsupported review status replayability")
	}
	if result.ValidationRequest != nil {
		if result.Authority == nil || result.Authority.State != reviewtransaction.StateCorrectionRequired ||
			result.ValidationRequest.LineageID != result.Authority.LineageID ||
			result.ValidationRequest.ExpectedRevision != result.Authority.Revision ||
			result.ValidationRequest.TargetIdentity != result.Projection.InitialSnapshotIdentity ||
			result.ValidationRequest.Projection != result.Projection.Projection ||
			result.ValidationRequest.CorrectionCandidateTree != result.Projection.CurrentCandidateTree ||
			!reviewStatusPathsContain(result.Projection.Paths, result.ValidationRequest.CorrectionPaths) ||
			reviewtransaction.ValidateTargetedValidationRequest(*result.ValidationRequest) != nil {
			return errors.New("negotiated status validation request is invalid")
		}
	}
	switch result.ActionDisposition {
	case "":
		if result.Action == reviewtransaction.TargetStatusActionRecover || result.Action == reviewtransaction.TargetStatusActionRetryFinalVerification {
			return errors.New("recover status requires the recovery disposition recovery accepts")
		}
	case reviewtransaction.RecoveryScopeChanged, reviewtransaction.RecoveryInvalidated, reviewtransaction.RecoveryEscalated:
		if result.Action != reviewtransaction.TargetStatusActionRecover {
			return errors.New("only recover status may carry a recovery disposition")
		}
	case reviewtransaction.RecoveryFinalVerificationRetry:
		if result.Action != reviewtransaction.TargetStatusActionRetryFinalVerification {
			return errors.New("only final-verification retry status may carry its dedicated disposition")
		}
	default:
		return errors.New("unsupported review status recovery disposition")
	}
	if result.Applicability == reviewtransaction.TargetApplicabilityCurrent && result.Authority != nil &&
		result.Authority.State == reviewtransaction.StateApproved && result.Action == reviewtransaction.TargetStatusActionRecover {
		if result.Receipt.Status != ReviewReceiptPresent || result.ActionDisposition != reviewtransaction.RecoveryScopeChanged {
			return errors.New("approved recovery status requires a published scope-changed target") // refusal:by-design world-action: this envelope is built and validated by the same product; the exit is a code fix, not a command
		}
		stagedScopeExpansion := result.Projection.Kind == reviewtransaction.TargetBaseWorkspaceOverlay &&
			result.Projection.Projection == reviewtransaction.ProjectionStaged
		// The second approved recovery shape (issue #1826): the live target
		// keeps the approved delivery scope kind while its candidate tree
		// changed, so START refuses a fresh lineage and only recovery of this
		// exact predecessor continues.
		sameScopeChangedCandidate := result.Projection.Kind == reviewtransaction.TargetCurrentChanges ||
			result.Projection.Kind == reviewtransaction.TargetBaseDiff
		if !stagedScopeExpansion && !sameScopeChangedCandidate {
			return errors.New("approved recovery status requires a published staged scope-expansion or scope-changed target") // refusal:by-design world-action: this envelope is built and validated by the same product; the exit is a code fix, not a command
		}
	}
	return nil
}

func (result ReviewTargetStatusResult) validateSubmissionDescriptors() error {
	transition := result.NextTransition
	if transition == nil || transition.Collect == nil {
		return nil
	}
	for _, input := range transition.Collect.Inputs {
		if input.Submission != nil && result.Contract != ReviewIntegrationContractV2 {
			return errors.New("legacy negotiated status contains a submission descriptor") // refusal:by-design world-action: only a provider code fix can remove a descriptor from a legacy response
		}
	}
	if result.Contract != ReviewIntegrationContractV2 {
		return nil
	}
	if result.Schema == ReviewIntegrationStatusSchemaV3 {
		for _, input := range transition.Collect.Inputs {
			if input.Submission != nil {
				return errors.New("v3 negotiated status contains a v4 submission descriptor") // refusal:by-design world-action: only a provider code fix can preserve the v3 wire contract
			}
			if input.ProviderTask != nil {
				return errors.New("v3 negotiated status contains a provider role task") // refusal:by-design world-action: only the v5 provider can emit a Go-issued provider task
			} else if input.CaptureOperation == "external.run_provider_role" {
				return errors.New("provider role collection lacks its Go-issued task") // refusal:by-design world-action: provider role collection must be materialized by Go
			}
		}
		return nil
	}
	if result.Schema != ReviewIntegrationStatusSchemaV4 && result.Schema != ReviewIntegrationStatusSchemaV5 {
		return errors.New("submission descriptor status schema is unsupported") // refusal:by-design world-action: only a provider code fix can select a supported descriptor schema
	}
	for _, input := range transition.Collect.Inputs {
		if input.ProviderTask == nil {
			if input.CaptureOperation == "external.run_provider_role" {
				return errors.New("provider role collection lacks its Go-issued task") // refusal:by-design world-action: provider role collection must be materialized by Go
			}
			continue
		}
		if result.Schema != ReviewIntegrationStatusSchemaV5 {
			return errors.New("v4 negotiated status contains a provider role task") // refusal:by-design world-action: only the v5 provider can emit a Go-issued provider task
		}
		arguments, err := reviewTransitionArgumentMap(input.Arguments)
		if err != nil {
			return err
		}
		if err := validateReviewProviderTaskInput(input, arguments); err != nil {
			return err
		}
	}
	switch transition.ReasonCode {
	case "correction_plan_required":
		if len(transition.Collect.Inputs) != 1 {
			return errors.New("submission descriptor transition must contain exactly one input") // refusal:by-design world-action: only a provider code fix can produce the required single input
		}
		input := transition.Collect.Inputs[0]
		if input.CaptureOperation != "external.plan_correction" || result.Authority == nil || transition.CorrectionRequest == nil || input.Submission == nil {
			return errors.New("correction submission descriptor has no provider request") // refusal:by-design world-action: only a provider code fix can bind the correction request
		}
		context, err := input.submissionRepositoryContext()
		if err != nil {
			return err
		}
		want := reviewCorrectionPlanSubmission(result.Contract, ReviewTransitionBinding{
			LineageID: result.Authority.LineageID, Revision: result.Authority.Revision,
			TargetIdentity: result.TargetIdentity, RepositoryContext: context,
		}, *transition.CorrectionRequest)
		if want == nil || !reflect.DeepEqual(*input.Submission, *want) {
			return errors.New("correction submission descriptor is not provider-bound") // refusal:by-design world-action: only a provider code fix can bind descriptor tokens to its request
		}
	case "targeted_validation_required":
		if len(transition.Collect.Inputs) != 1 {
			return errors.New("submission descriptor transition must contain exactly one input") // refusal:by-design world-action: only a provider code fix can produce the required single input
		}
		input := transition.Collect.Inputs[0]
		if input.CaptureOperation != "external.run_targeted_validation" || result.Authority == nil || result.ValidationRequest == nil || input.Submission == nil {
			return errors.New("targeted validation submission descriptor has no provider request") // refusal:by-design world-action: only a provider code fix can bind the validation request
		}
		context, err := input.submissionRepositoryContext()
		if err != nil {
			return err
		}
		if arguments, err := reviewTransitionArgumentMap(input.Arguments); err != nil || len(arguments) != 6 ||
			arguments["lineage"] != result.Authority.LineageID || arguments["expected-revision"] != result.Authority.Revision ||
			arguments["target"] != result.ValidationRequest.CorrectionTargetIdentity || arguments["repository-context"] != context ||
			reviewtransaction.ValidateReviewRepositoryContextHandle(arguments["repository-context"]) != nil ||
			arguments["purpose"] != reviewTargetedValidationPurpose || arguments["request-hash"] != result.ValidationRequest.RequestHash {
			return errors.New("targeted validation transition lacks the corrected inspection binding") // refusal:by-design world-action: only STATUS can issue a complete corrected-candidate binding
		}
		want := reviewTargetedValidationSubmission(result.Contract, ReviewTransitionBinding{
			LineageID: result.Authority.LineageID, Revision: result.Authority.Revision,
			TargetIdentity: result.ValidationRequest.CorrectionTargetIdentity, RepositoryContext: context,
		}, *result.ValidationRequest)
		if want == nil || !reflect.DeepEqual(*input.Submission, *want) {
			return errors.New("targeted validation submission descriptor is not provider-bound") // refusal:by-design world-action: only a provider code fix can bind descriptor tokens to its request
		}
	case "verification_evidence_required", "correction_repository_verification_required":
		if result.Schema == ReviewIntegrationStatusSchemaV4 {
			for _, input := range transition.Collect.Inputs {
				if input.Submission != nil {
					return errors.New("v4 verification evidence transition contains a submission descriptor") // refusal:by-design world-action: only a provider code fix can preserve the v4 wire contract
				}
			}
			return nil
		}
		if len(transition.Collect.Inputs) != 1 || result.Authority == nil {
			return errors.New("verification evidence submission transition is incomplete") // refusal:by-design world-action: only a provider code fix can produce the required single input
		}
		input := transition.Collect.Inputs[0]
		arguments, err := reviewTransitionArgumentMap(input.Arguments)
		if err != nil || input.CaptureOperation != "review.capture-evidence" || input.Schema != reviewVerificationEvidenceSchemaID || input.Submission == nil {
			return errors.New("verification evidence submission descriptor has no provider binding") // refusal:by-design world-action: only a provider code fix can bind the capture operation
		}
		target := reviewAuthorityTargetIdentity(result)
		if result.Authority.State == reviewtransaction.StateCorrectionRequired && result.ValidationRequest != nil {
			target = result.ValidationRequest.CorrectionTargetIdentity
		}
		binding := ReviewTransitionBinding{
			LineageID: result.Authority.LineageID, Revision: result.Authority.Revision,
			TargetIdentity: target, RepositoryContext: arguments["repository-context"],
		}
		want := reviewCaptureEvidenceSubmission(result.Contract, binding)
		if want == nil || !reflect.DeepEqual(*input.Submission, *want) {
			return errors.New("verification evidence submission descriptor is not provider-bound") // refusal:by-design world-action: only a provider code fix can bind descriptor tokens to its authority
		}
	default:
		for _, input := range transition.Collect.Inputs {
			if input.Submission != nil {
				return errors.New("submission descriptor is attached to an unrelated collection input") // refusal:by-design world-action: only a provider code fix can remove the unrelated descriptor
			}
		}
	}
	return nil
}

func validateReviewProviderTaskInput(input ReviewTransitionInput, arguments map[string]string) error {
	task := input.ProviderTask
	role := reviewerprovider.Role(task.Role)
	contract, err := reviewerprovider.ContractFor(role)
	if err != nil || input.CaptureOperation != "external.run_provider_role" || input.Schema != string(contract.ResultSchema) ||
		task.Agent != reviewProviderRoleOpenCodeAgent(role) || len(arguments) != 6 || arguments["agent"] != string(model.AgentOpenCode) ||
		arguments["role"] != task.Role || arguments["repository-context"] == "" {
		return errors.New("provider role collection is not an exact Go-owned binding") // refusal:by-design world-action: provider role collection must remain an exact Go-issued binding
	}
	encoded, found := strings.CutPrefix(task.Prompt, reviewProviderTaskBindingHeader+" ")
	if !found {
		return errors.New("provider role task prompt has no Go-issued binding") // refusal:by-design world-action: provider role task prompts must carry their Go-issued binding
	}
	decoder := json.NewDecoder(bytes.NewBufferString(encoded))
	decoder.DisallowUnknownFields()
	var binding reviewProviderTaskBinding
	if err := decoder.Decode(&binding); err != nil {
		return errors.New("provider role task binding is malformed") // refusal:by-design world-action: provider role task bindings are Go-issued strict JSON
	}
	var extra any
	if err := decoder.Decode(&extra); err == nil || binding.LineageID != arguments["lineage"] || binding.Revision != arguments["expected-revision"] ||
		binding.TargetIdentity != arguments["target"] || binding.RepositoryContext != arguments["repository-context"] || binding.Role != task.Role ||
		reviewtransaction.ValidateReviewRepositoryContextHandle(binding.RepositoryContext) != nil || !validReviewCapabilitySHA256(binding.Revision) || !validReviewCapabilitySHA256(binding.TargetIdentity) {
		return errors.New("provider role task binding is incomplete") // refusal:by-design world-action: provider role task bindings must match the current authority exactly
	}
	return nil
}

func (result ReviewTargetStatusResult) validateNextTransitionTargets() error {
	if result.NextTransition == nil {
		return nil
	}
	if result.Applicability == reviewtransaction.TargetApplicabilityUnrelated {
		if result.Action == reviewtransaction.TargetStatusActionStop {
			if result.NextTransition.Kind != reviewNextTransitionStop {
				// refusal:by-design world-action: a provider-built status envelope paired STOP with a non-STOP transition; only a producer code fix can make that invariant true
				return errors.New("fresh target STOP action lacks a STOP transition")
			}
			switch result.Projection.Kind {
			case reviewtransaction.TargetBaseWorkspaceOverlay:
				if result.Projection.Projection != reviewtransaction.ProjectionStaged || result.NextTransition.ReasonCode != "staged_workspace_overlay_recovery_unavailable" {
					return errors.New("fresh staged workspace-overlay target lacks a STOP transition")
				}
			case reviewtransaction.TargetBaseDiff:
				if len(result.Projection.Paths) != 0 || result.NextTransition.ReasonCode != "empty_base_diff_bootstrap_required" {
					// refusal:by-design world-action: a provider-built zero-path base-diff omitted its one admissible STOP classification; only a producer code fix can make the envelope executable
					return errors.New("fresh zero-path base-diff target lacks an empty-root bootstrap STOP transition")
				}
			default:
				// refusal:by-design world-action: this negotiated status invariant supports only the explicitly classified fresh STOP projections; a producer must choose one of those projections
				return errors.New("fresh target STOP action has an unsupported projection")
			}
			return nil
		}
		if result.Action == reviewtransaction.TargetStatusActionStart && result.rddModeResolved && !result.rddMode.Enabled() {
			if result.NextTransition.Kind != reviewNextTransitionStop || result.NextTransition.ReasonCode != "rdd_disabled" {
				// refusal:by-design world-action: only a producer defect can pair a disabled effective mode with a fresh transition other than rdd_disabled
				return errors.New("disabled fresh target lacks an RDD STOP transition")
			}
			return nil
		}
		// The one fresh target that has no representable START: a workspace
		// candidate with zero paths (issue #2584). It collects the base the
		// caller must choose instead, so requiring an executable START here
		// would only relocate the contradiction.
		if result.Projection.Kind == reviewtransaction.TargetCurrentChanges && len(result.Projection.Paths) == 0 {
			if result.NextTransition.Kind == reviewNextTransitionCollect && result.NextTransition.ReasonCode == "intended_untracked_selection_required" {
				return result.validateIntendedUntrackedSelectionTransition()
			}
			if result.NextTransition.Kind != reviewNextTransitionCollect || result.NextTransition.ReasonCode != "empty_candidate_base_ref_required" ||
				result.NextTransition.Collect == nil || len(result.NextTransition.Collect.Inputs) != 1 {
				return errors.New("fresh empty workspace target lacks a base-ref collection transition") // refusal:by-design world-action: only a provider code fix can emit the base-ref collection this classification requires
			}
			input := result.NextTransition.Collect.Inputs[0]
			if input.Name != "base_ref" || input.Schema != "gentle-ai.review-base-ref-selection/v1" ||
				input.CaptureOperation != "external.select_base_ref" || input.Submission != nil ||
				!reflect.DeepEqual(input.Arguments, reviewTargetArguments(result)) {
				return errors.New("fresh empty workspace target lacks a base-ref collection transition") // refusal:by-design world-action: only a provider code fix can emit the base-ref collection this classification requires
			}
			return nil
		}
		if result.NextTransition.Kind == reviewNextTransitionCollect && result.NextTransition.ReasonCode == "intended_untracked_selection_required" {
			return result.validateIntendedUntrackedSelectionTransition()
		}
		return result.validateStartNextTransition()
	}
	expectedExecutionTarget := result.TargetIdentity
	if result.Authority != nil && result.Authority.State == reviewtransaction.StateValidating {
		expectedExecutionTarget = reviewAuthorityTargetIdentity(result)
	} else if result.Authority != nil && result.Authority.State == reviewtransaction.StateCorrectionRequired &&
		result.ValidationRequest != nil && result.NextTransition.ReasonCode == "correction_repository_tooling_failed" {
		expectedExecutionTarget = result.ValidationRequest.CorrectionTargetIdentity
	}
	if result.NextTransition.Execute != nil && result.NextTransition.Execute.Binding.TargetIdentity != expectedExecutionTarget {
		return errors.New("negotiated status execution target differs from the current target identity")
	}
	if err := result.validateSelectorNextTransition(); err != nil {
		return err
	}
	if result.Repair.Status == reviewtransaction.AuthorityRepairEligible {
		if err := result.validateRepairNextTransition(); err != nil {
			return err
		}
	}
	if result.FinalVerificationRetry != nil {
		if err := result.validateFinalVerificationRetryNextTransition(); err != nil {
			return err
		}
	}
	if result.NextTransition.Collect == nil {
		return nil
	}
	for _, input := range result.NextTransition.Collect.Inputs {
		if input.CaptureOperation == "review.capture-evidence" {
			expectedTarget := reviewAuthorityTargetIdentity(result)
			if result.Authority != nil && result.Authority.State == reviewtransaction.StateCorrectionRequired && result.ValidationRequest != nil {
				expectedTarget = result.ValidationRequest.CorrectionTargetIdentity
			}
			arguments, err := reviewTransitionArgumentMap(input.Arguments)
			if err != nil || arguments["target"] != expectedTarget {
				return errors.New("negotiated status evidence target differs from the frozen authority target identity")
			}
			continue
		}
		if input.CaptureOperation != "review.capture-result" {
			continue
		}
		arguments, err := reviewTransitionArgumentMap(input.Arguments)
		if err != nil || arguments["target"] != result.Projection.InitialSnapshotIdentity || input.ArtifactSubject == nil ||
			input.ArtifactSubject.TargetIdentity != result.Projection.InitialSnapshotIdentity || input.ChangedPathManifest == nil ||
			!reflect.DeepEqual(manifestPathsForStatus(*input.ChangedPathManifest), result.Projection.Paths) {
			return errors.New("negotiated status capture target differs from the frozen target identity")
		}
		if result.Contract == ReviewIntegrationContractV1 && (input.CandidateDiff == nil || input.BaseTree != "" || input.CandidateTree != "") ||
			result.Contract == ReviewIntegrationContractV2 && (input.CandidateDiff != nil || input.BaseTree != result.Projection.BaseTree || input.CandidateTree != result.Projection.InitialReviewTree) {
			return errors.New("negotiated status capture transport differs from its contract") // refusal:by-design world-action: provider-built STATUS mixed negotiated transports and requires a code fix
		}
	}
	return nil
}

func (result ReviewTargetStatusResult) validateIntendedUntrackedSelectionTransition() error {
	if result.NextTransition.Collect == nil || len(result.NextTransition.Collect.Inputs) != 1 {
		return errors.New("fresh target lacks an intended-untracked selection transition; rerun `gentle-ai review status --next-transition`")
	}
	input := result.NextTransition.Collect.Inputs[0]
	if input.Name != "intended_untracked_selection" || input.Schema != reviewIntendedUntrackedSelectionSchema ||
		input.CaptureOperation != "external.select_intended_untracked" || input.Submission != nil || len(input.Arguments) != 6 {
		return errors.New("fresh target lacks an intended-untracked selection transition; rerun `gentle-ai review status --next-transition`")
	}
	if !reflect.DeepEqual(input.Arguments[:4], reviewTargetArguments(result)) || input.Arguments[4].Name != "eligible_paths_json" ||
		input.Arguments[5].Name != "expected_untracked_inventory" || input.Arguments[5].Value == "" {
		return errors.New("fresh target lacks an intended-untracked selection transition; rerun `gentle-ai review status --next-transition`")
	}
	return nil
}

func (result ReviewTargetStatusResult) validateSelectorNextTransition() error {
	execution := result.NextTransition.Execute
	if execution == nil || (execution.Operation != "review.validate" && execution.Operation != "review.recover") {
		return nil
	}
	arguments, err := reviewTransitionArgumentMap(execution.Arguments)
	if err != nil {
		return err
	}
	selectorsPresent := execution.SelectorArguments != nil
	selectors := []ReviewTransitionArgument{}
	if selectorsPresent {
		selectors = *execution.SelectorArguments
	}
	for _, selector := range selectors {
		if arguments[selector.Name] != selector.Value {
			return errors.New("negotiated transition changed its normalized selector")
		}
	}
	base, hasBase := arguments["base-ref"]
	_, hasCommitted := arguments["committed-only"]
	projection, hasProjection := arguments["projection"]
	workspaceOverlay, hasWorkspaceOverlay := arguments["workspace-overlay"]
	if !selectorsPresent && (hasBase || hasCommitted || hasProjection || hasWorkspaceOverlay) {
		return errors.New("negotiated transition omitted its normalized selector")
	}
	if hasWorkspaceOverlay && workspaceOverlay != "true" {
		return errors.New("RECOVER transition workspace-overlay selector is invalid")
	}
	if execution.Operation == "review.validate" {
		if result.Projection.Kind == reviewtransaction.TargetCurrentChanges && hasBase ||
			reviewtransaction.GateKind(arguments["gate"]) == reviewtransaction.GatePrePR && result.Projection.Kind == reviewtransaction.TargetBaseDiff && !hasBase {
			return errors.New("negotiated VALIDATE transition does not reproduce the selected target")
		}
		return nil
	}
	switch result.Projection.Kind {
	case reviewtransaction.TargetCurrentChanges:
		if hasBase || hasCommitted || hasWorkspaceOverlay {
			return errors.New("current-changes RECOVER transition invented target selectors")
		}
	case reviewtransaction.TargetBaseDiff:
		if !hasBase || !hasCommitted || hasWorkspaceOverlay {
			return errors.New("base-diff RECOVER transition lacks exact target selectors")
		}
	case reviewtransaction.TargetBaseWorkspaceOverlay:
		if !hasBase || hasCommitted {
			return errors.New("workspace-overlay RECOVER transition has incompatible target selectors")
		}
		if result.Projection.Projection == reviewtransaction.ProjectionStaged &&
			(!hasProjection || projection != string(reviewtransaction.ProjectionStaged) || !hasWorkspaceOverlay) {
			return errors.New("staged workspace-overlay RECOVER transition lacks exact target selectors")
		}
		if result.Projection.Projection != reviewtransaction.ProjectionStaged && hasWorkspaceOverlay {
			return errors.New("workspace-overlay RECOVER transition invented a staged selector")
		}
	default:
		return errors.New("RECOVER transition target kind is unsupported")
	}
	if result.Projection.Kind != reviewtransaction.TargetCurrentChanges && base == "" ||
		hasProjection && projection != string(result.Projection.Projection) {
		return errors.New("RECOVER transition selectors do not match the selected target")
	}
	return nil
}

func (result ReviewTargetStatusResult) validateStartNextTransition() error {
	transition := result.NextTransition
	if result.Projection.Kind != reviewtransaction.TargetCurrentChanges && result.Projection.Kind != reviewtransaction.TargetBaseDiff &&
		result.Projection.Kind != reviewtransaction.TargetBaseWorkspaceOverlay {
		return errors.New("fresh target START projection kind is unsupported")
	}
	if transition.Kind != reviewNextTransitionExecute || transition.ReasonCode != "fresh_target_ready" || transition.Execute == nil ||
		transition.Execute.Operation != "review.start" || len(transition.Execute.Artifacts) != 0 {
		return errors.New("fresh target lacks an executable START transition")
	}
	arguments, err := reviewTransitionArgumentMap(transition.Execute.Arguments, transition.Execute.Operation)
	if err != nil {
		return err
	}
	if result.repositoryRoot == "" {
		result.repositoryRoot = arguments["cwd"]
	}
	lineage := arguments["lineage"]
	if lineage != "" && !validReviewIntegrationLineage(lineage) {
		return errors.New("fresh target START lineage is not canonical")
	}
	runtime := model.AgentID(arguments["agent"])
	if runtime != "" {
		if _, err := reviewRuntimeWithImmutableTransport(string(runtime)); err != nil {
			// refusal:by-design world-action: a START transition with an unproven runtime transport cannot be safely executed
			return errors.New("fresh target START runtime lacks immutable review transport")
		}
	}
	wantArguments := reviewStartArguments(result, lineage, runtime, result.intendedUntracked)
	for index, argument := range wantArguments {
		argument.Token = reviewTransitionArgumentToken(argument)
		wantArguments[index] = argument
	}
	wantPreconditions := []ReviewTransitionArgument{{Name: "target_identity", Value: result.TargetIdentity}}
	wantBinding := ReviewTransitionBinding{LineageID: lineage, TargetIdentity: result.TargetIdentity}
	if !reflect.DeepEqual(transition.Execute.Arguments, wantArguments) ||
		!reflect.DeepEqual(transition.Execute.Preconditions, wantPreconditions) || transition.Execute.Binding != wantBinding {
		return errors.New("fresh target START transition is not exactly bound")
	}
	return nil
}

func reviewAuthorityTargetIdentity(status ReviewTargetStatusResult) string {
	if status.AuthorityTargetIdentity != "" {
		return status.AuthorityTargetIdentity
	}
	return status.TargetIdentity
}

func (result ReviewTargetStatusResult) validateFinalVerificationRetryNextTransition() error {
	transition, retry := result.NextTransition, result.FinalVerificationRetry
	if transition == nil || retry == nil || result.Authority == nil ||
		transition.Kind != reviewNextTransitionCollect || transition.ReasonCode != "final_verification_retry_authorization_required" ||
		transition.Collect == nil || len(transition.Collect.Inputs) != 1 {
		return errors.New("final-verification retry authorization transition is incomplete")
	}
	input := transition.Collect.Inputs[0]
	arguments, err := reviewTransitionArgumentMap(input.Arguments)
	want := map[string]string{
		"predecessor-lineage":           result.Authority.LineageID,
		"expected-predecessor-revision": result.Authority.Revision,
		"validating-revision":           retry.ValidatingRevision,
		"target":                        retry.TargetIdentity,
		"failed-evidence-hash":          retry.FailedEvidenceHash,
		"finalize-request-digest":       retry.FinalizeRequestDigest,
		"incident-schema":               retry.IncidentSchema,
		"incident-class":                retry.IncidentClass,
	}
	if retry.FailedEvidenceRecordDigest != "" {
		want["failed-evidence-record-digest"] = retry.FailedEvidenceRecordDigest
	}
	if err != nil || input.Name != "final_verification_retry_authorization" ||
		input.Schema != reviewtransaction.FinalVerificationRetryAuthorizationSchema ||
		input.CaptureOperation != "external.authorize_final_verification_retry" || !reflect.DeepEqual(arguments, want) {
		return errors.New("final-verification retry authorization transition is not provider-bound")
	}
	return nil
}

func (result ReviewTargetStatusResult) validateRepairNextTransition() error {
	transition := result.NextTransition
	assessment, candidate := result.Repair, result.Repair.Candidate
	if transition == nil || candidate == nil {
		return errors.New("eligible authority repair lacks a classified transition")
	}
	provider := map[string]string{
		"class": string(assessment.Class), "lineage": candidate.LineageID,
		"expected-revision": candidate.Revision, "cause": string(assessment.Cause),
		"disposition": string(assessment.Disposition), "repository-binding": assessment.RepositoryBinding,
	}
	switch transition.Kind {
	case reviewNextTransitionCollect:
		if transition.ReasonCode != "repair_authorization_required" || transition.Collect == nil || len(transition.Collect.Inputs) != 1 {
			return errors.New("classified repair authorization transition is incomplete")
		}
		input := transition.Collect.Inputs[0]
		arguments, err := reviewTransitionArgumentMap(input.Arguments)
		if err != nil || input.Name != "repair_authorization" || input.Schema != assessment.AuthorizationSchema ||
			input.CaptureOperation != "external.authorize_repair" || !reflect.DeepEqual(arguments, provider) {
			return errors.New("classified repair authorization transition is not provider-bound")
		}
	case reviewNextTransitionExecute:
		if transition.ReasonCode != "repair_authorized" || transition.Execute == nil || transition.Execute.Operation != "review.repair" ||
			transition.Execute.Binding.LineageID != candidate.LineageID || transition.Execute.Binding.Revision != candidate.Revision {
			return errors.New("classified repair execution transition is incomplete")
		}
		arguments, err := reviewTransitionArgumentMap(transition.Execute.Arguments, transition.Execute.Operation)
		if err != nil || len(arguments) != len(provider)+3 {
			return errors.New("classified repair execution arguments are incomplete")
		}
		for name, value := range provider {
			if arguments[name] != value {
				return errors.New("classified repair execution arguments differ from provider assessment")
			}
		}
		if strings.TrimSpace(arguments["actor"]) == "" || strings.TrimSpace(arguments["reason"]) == "" || arguments["maintainer-authorization"] != "provided" {
			return errors.New("classified repair execution exposes or omits authorization state")
		}
		preconditions, err := reviewTransitionArgumentMap(transition.Execute.Preconditions)
		wantPreconditions := map[string]string{
			"repair_status": string(reviewtransaction.AuthorityRepairEligible), "unique_candidate": "true",
			"current_head": candidate.Revision, "repair_authorization": "provided",
		}
		if err != nil || !reflect.DeepEqual(preconditions, wantPreconditions) {
			return errors.New("classified repair execution preconditions are incomplete")
		}
	default:
		return errors.New("eligible authority repair may only collect authorization or execute repair")
	}
	return nil
}

func manifestPathsForStatus(entries []reviewtransaction.ChangedPathManifestEntry) []string {
	paths := make([]string, len(entries))
	for index, entry := range entries {
		paths[index] = entry.Path
	}
	return paths
}

func reviewStatusPathsContain(candidate, correction []string) bool {
	available := make(map[string]struct{}, len(candidate))
	for _, value := range candidate {
		available[value] = struct{}{}
	}
	for _, value := range correction {
		if _, exists := available[value]; !exists {
			return false
		}
	}
	return len(correction) > 0
}

func reviewTransitionValidationRequest(transition *ReviewNextTransition) *reviewtransaction.TargetedValidationRequest {
	if transition == nil || transition.Collect == nil || len(transition.Collect.Inputs) != 1 {
		return nil
	}
	return transition.Collect.Inputs[0].ValidationRequest
}

func (transition ReviewNextTransition) Validate() error {
	if strings.TrimSpace(transition.ReasonCode) == "" {
		return errors.New("review next transition requires a reason code")
	}
	correctionRequestRequired := transition.ReasonCode == "correction_plan_required" || transition.ReasonCode == "corrected_candidate_unavailable"
	if correctionRequestRequired != (transition.CorrectionRequest != nil) {
		return errors.New("correction transition must carry exactly one provider-owned request") // refusal:by-design world-action: provider-generated routing requires a code fix when its request is missing or misplaced
	}
	if transition.CorrectionRequest != nil && reviewtransaction.ValidateCorrectionPlanRequest(*transition.CorrectionRequest) != nil {
		return errors.New("correction transition request is invalid") // refusal:by-design world-action: malformed provider-owned findings cannot safely authorize planning
	}
	switch transition.Kind {
	case reviewNextTransitionStop:
		if transition.Execute != nil || transition.Collect != nil {
			return errors.New("stop transition contains routing data")
		}
	case reviewNextTransitionCollect:
		if transition.Execute != nil || transition.Collect == nil || len(transition.Collect.Inputs) == 0 {
			return errors.New("collection transition is incomplete")
		}
		for _, input := range transition.Collect.Inputs {
			if strings.TrimSpace(input.Name) == "" || strings.TrimSpace(input.Schema) == "" || strings.TrimSpace(input.CaptureOperation) == "" || len(input.Arguments) == 0 {
				return errors.New("collection transition has an incomplete input")
			}
			for _, argument := range input.Arguments {
				if strings.TrimSpace(argument.Name) == "" || strings.TrimSpace(argument.Value) == "" {
					return errors.New("collection transition has an incomplete argument")
				}
			}
			arguments, err := reviewTransitionArgumentMap(input.Arguments)
			if err != nil {
				return err
			}
			submissionAllowed := input.CaptureOperation == "external.plan_correction" || input.CaptureOperation == "external.run_targeted_validation" || input.CaptureOperation == "review.capture-evidence"
			if input.Submission != nil && !submissionAllowed {
				return errors.New("collection transition submission placement is invalid") // refusal:by-design world-action: only a provider code fix can place a descriptor on a supported input
			}
			if input.Submission != nil {
				if err := input.Submission.Validate(); err != nil {
					return err
				}
			}
			if input.CaptureOperation == "review.capture-result" {
				order, orderErr := strconv.Atoi(arguments["order"])
				legacyTransport := input.ArtifactSubject != nil && input.ArtifactSubject.Schema == reviewtransaction.ArtifactSubjectSchemaV1
				nativeGitTransport := input.ArtifactSubject != nil && input.ArtifactSubject.Schema == reviewtransaction.ArtifactSubjectSchema
				argumentCount := 6
				if nativeGitTransport {
					argumentCount = 7
				}
				providerRuntime := model.AgentID(arguments["agent"])
				if providerRuntime != "" && reviewProviderCaptureRuntime(providerRuntime) {
					argumentCount++
				}
				if len(arguments) != argumentCount || !reviewStartSupportedLens(arguments["lens"]) || orderErr != nil || order < 0 ||
					!validReviewCapabilitySHA256(arguments["expected-revision"]) || !validReviewCapabilitySHA256(arguments["target"]) ||
					strings.TrimSpace(arguments["lineage"]) == "" || reviewtransaction.ValidateReviewRepositoryContextHandle(arguments["repository-context"]) != nil ||
					input.ArtifactSubject == nil || input.ChangedPathManifest == nil ||
					nativeGitTransport && arguments["subject-hash"] != input.ArtifactSubject.SubjectHash || providerRuntime != "" && !reviewProviderCaptureRuntime(providerRuntime) ||
					legacyTransport && input.CandidateDiff == nil || nativeGitTransport && (!validReviewGitTree(input.BaseTree) || !validReviewGitTree(input.CandidateTree)) ||
					(!legacyTransport && !nativeGitTransport) {
					return errors.New("review capture transition lacks an exact repository and authority binding")
				}
				subject := input.ArtifactSubject
				manifestDigest, manifestErr := reviewtransaction.ChangedPathManifestDigest(*input.ChangedPathManifest)
				if reviewtransaction.ValidateArtifactSubject(*subject) != nil || manifestErr != nil ||
					subject.LineageID != arguments["lineage"] || subject.AuthorityRevision != arguments["expected-revision"] ||
					subject.TargetIdentity != arguments["target"] || subject.Lens != arguments["lens"] || subject.SelectedOrder != order ||
					subject.ChangedPathManifestSHA256 != manifestDigest ||
					legacyTransport && subject.CandidateDiffSHA256 != input.CandidateDiff.SHA256 ||
					nativeGitTransport && (subject.BaseTree != input.BaseTree || subject.CandidateTree != input.CandidateTree) {
					return errors.New("review capture transition frozen subject or candidate context is invalid")
				}
				if legacyTransport {
					if _, diffErr := input.CandidateDiff.Bytes(); diffErr != nil || input.BaseTree != "" || input.CandidateTree != "" {
						return errors.New("review capture transition legacy candidate diff is invalid") // refusal:by-design world-action: provider-built v1 transition contains invalid immutable transport and requires a code fix
					}
				} else if input.CandidateDiff != nil {
					return errors.New("review capture transition native Git context contains a legacy candidate diff") // refusal:by-design world-action: provider-built v2 transition leaked legacy transport and requires a code fix
				}
			} else if input.ArtifactSubject != nil || input.CandidateDiff != nil || input.BaseTree != "" || input.CandidateTree != "" || input.ChangedPathManifest != nil {
				return errors.New("non-reviewer collection transition contains frozen reviewer context")
			}
			if input.CaptureOperation == "review.capture-evidence" &&
				((input.Schema != reviewtransaction.VerificationEvidenceRecordSchema || len(arguments) != 3) &&
					(input.Schema != reviewVerificationEvidenceSchemaID || len(arguments) != 4 ||
						reviewtransaction.ValidateReviewRepositoryContextHandle(arguments["repository-context"]) != nil) ||
					strings.TrimSpace(arguments["lineage"]) == "" || !validReviewCapabilitySHA256(arguments["expected-revision"]) ||
					!validReviewCapabilitySHA256(arguments["target"])) {
				return errors.New("verification evidence capture transition lacks an exact authority and candidate binding") // refusal:by-design world-action: only a producer-code repair can make a malformed machine transition trustworthy
			}
			if input.CaptureOperation == "external.run_targeted_validation" && input.ValidationRequest == nil {
				return errors.New("targeted validation transition lacks its provider-owned request")
			}
			if input.ValidationRequest != nil {
				request := input.ValidationRequest
				if input.Schema != reviewtransaction.TargetedValidationRequestSchema || input.CaptureOperation != "external.run_targeted_validation" ||
					arguments["lineage"] != request.LineageID || arguments["expected-revision"] != request.ExpectedRevision ||
					arguments["target"] != request.CorrectionTargetIdentity || reviewtransaction.ValidateTargetedValidationRequest(*request) != nil {
					return errors.New("targeted validation transition request is invalid")
				}
			}
		}
	case reviewNextTransitionExecute:
		if transition.Collect != nil || transition.Execute == nil || transition.Execute.Arguments == nil || len(transition.Execute.Preconditions) == 0 || !validReviewCapabilitySHA256(transition.Execute.Binding.TargetIdentity) {
			return errors.New("execution transition is incomplete")
		}
		if transition.Execute.Operation != "review.start" && transition.Execute.Operation != "review.finalize" && transition.Execute.Operation != "review.recover" && transition.Execute.Operation != "review.repair" && transition.Execute.Operation != "review.validate" || transition.Execute.Operation != "review.start" && (strings.TrimSpace(transition.Execute.Binding.LineageID) == "" || !validReviewCapabilitySHA256(transition.Execute.Binding.Revision)) {
			return errors.New("execution transition operation or binding is invalid")
		}
		if transition.Execute.Binding.RepositoryContext != "" && reviewtransaction.ValidateReviewRepositoryContextHandle(transition.Execute.Binding.RepositoryContext) != nil {
			return errors.New("execution transition repository context is invalid")
		}
		for _, argument := range transition.Execute.Arguments {
			if strings.TrimSpace(argument.Name) == "" || strings.TrimSpace(argument.Value) == "" {
				return errors.New("execution transition has an incomplete argument")
			}
		}
		for _, precondition := range transition.Execute.Preconditions {
			if strings.TrimSpace(precondition.Name) == "" || strings.TrimSpace(precondition.Value) == "" {
				return errors.New("execution transition has an incomplete precondition")
			}
		}
		arguments, err := reviewTransitionArgumentMap(transition.Execute.Arguments, transition.Execute.Operation)
		if err != nil {
			return err
		}
		if err := validateReviewTransitionExecution(*transition.Execute, arguments); err != nil {
			return err
		}
	default:
		return errors.New("unsupported review next transition kind")
	}
	return nil
}

func (input ReviewTransitionInput) submissionRepositoryContext() (string, error) {
	if input.Submission == nil {
		return "", errors.New("submission descriptor is missing") // refusal:by-design world-action: only a provider code fix can emit the required descriptor
	}
	for _, token := range input.Submission.ArgumentTokens {
		value, found := strings.CutPrefix(token, "--repository-context=")
		if found {
			return value, nil
		}
	}
	return "", errors.New("submission descriptor has no repository context") // refusal:by-design world-action: only a provider code fix can bind repository context
}

func (submission ReviewTransitionSubmission) Validate() error {
	if submission.Value != nil {
		return submission.validateFinalize()
	}
	if submission.OperationToken != "capture-evidence" || len(submission.Values) != 2 {
		return errors.New("submission descriptor identity is incomplete") // refusal:by-design world-action: only a provider code fix can restore descriptor identity
	}
	for _, token := range submission.ArgumentTokens {
		if strings.TrimSpace(token) == "" || !strings.HasPrefix(token, "--") || strings.ContainsAny(token, " \t\r\n") || strings.HasPrefix(token, "--cwd=") {
			return errors.New("submission descriptor contains an unsafe argument token") // refusal:by-design world-action: only a provider code fix can emit safe argv tokens
		}
	}
	if len(submission.ArgumentTokens) != 6 || len(submission.Values) != 2 {
		return errors.New("submission descriptor value substitution is malformed") // refusal:by-design world-action: only a provider code fix can restore the single value slot
	}
	if !strings.HasPrefix(submission.ArgumentTokens[0], "--lineage=") || !validReviewIntegrationLineage(strings.TrimPrefix(submission.ArgumentTokens[0], "--lineage=")) ||
		!strings.HasPrefix(submission.ArgumentTokens[1], "--expected-revision=") || !validReviewCapabilitySHA256(strings.TrimPrefix(submission.ArgumentTokens[1], "--expected-revision=")) ||
		!strings.HasPrefix(submission.ArgumentTokens[2], "--target=") || !validReviewCapabilitySHA256(strings.TrimPrefix(submission.ArgumentTokens[2], "--target=")) ||
		!strings.HasPrefix(submission.ArgumentTokens[3], "--repository-context=") ||
		reviewtransaction.ValidateReviewRepositoryContextHandle(strings.TrimPrefix(submission.ArgumentTokens[3], "--repository-context=")) != nil ||
		submission.ArgumentTokens[4] != "--outcome={{outcome}}" || submission.ArgumentTokens[5] != "--input={{input}}" {
		return errors.New("submission descriptor bindings are invalid") // refusal:by-design world-action: only a provider code fix can restore authority bindings
	}
	outcome, input := submission.Values[0], submission.Values[1]
	if outcome.Slot != "outcome" || outcome.Domain != "verification_outcome" || outcome.Schema != "" || outcome.Minimum != 0 || outcome.Maximum != 0 ||
		outcome.SubstitutionLocation != 4 || !reflect.DeepEqual(outcome.AllowedValues, []string{
		string(reviewtransaction.VerificationOutcomePassed),
		string(reviewtransaction.VerificationOutcomeFailed),
		string(reviewtransaction.VerificationOutcomeProceduralFailure),
	}) || input.Slot != "input" || input.Domain != "artifact_path_or_stdin" || input.Schema != reviewVerificationEvidenceSchemaID ||
		input.Minimum != 0 || input.Maximum != 0 || len(input.AllowedValues) != 0 || input.SubstitutionLocation != 5 {
		return errors.New("verification evidence submission descriptor values are invalid") // refusal:by-design world-action: only a provider code fix can restore the capture value domains
	}
	return nil
}

func (submission ReviewTransitionSubmission) validateFinalize() error {
	if submission.OperationToken != "finalize" || len(submission.Values) != 0 || submission.Value.SubstitutionLocation != 6 {
		return errors.New("submission descriptor identity is incomplete") // refusal:by-design world-action: only a provider code fix can restore descriptor identity
	}
	for _, token := range submission.ArgumentTokens {
		if strings.TrimSpace(token) == "" || !strings.HasPrefix(token, "--") || strings.ContainsAny(token, " \t\r\n") || strings.HasPrefix(token, "--cwd=") {
			return errors.New("submission descriptor contains an unsafe argument token") // refusal:by-design world-action: only a provider code fix can emit safe argv tokens
		}
	}
	if len(submission.ArgumentTokens) < 7 || submission.Value.SubstitutionLocation >= len(submission.ArgumentTokens) {
		return errors.New("submission descriptor value substitution is malformed") // refusal:by-design world-action: only a provider code fix can restore the single value slot
	}
	common := submission.ArgumentTokens[:6]
	if common[0] != "--contract="+ReviewIntegrationContractV2 ||
		!strings.HasPrefix(common[1], "--lineage=") || !validReviewIntegrationLineage(strings.TrimPrefix(common[1], "--lineage=")) ||
		!validReviewCapabilitySHA256(strings.TrimPrefix(common[2], "--expected-revision=")) ||
		!validReviewCapabilitySHA256(strings.TrimPrefix(common[3], "--target=")) ||
		!validReviewCapabilitySHA256(strings.TrimPrefix(common[4], "--request-hash=")) ||
		reviewtransaction.ValidateReviewRepositoryContextHandle(strings.TrimPrefix(common[5], "--repository-context=")) != nil ||
		!strings.HasPrefix(common[2], "--expected-revision=") || !strings.HasPrefix(common[3], "--target=") ||
		!strings.HasPrefix(common[4], "--request-hash=") || !strings.HasPrefix(common[5], "--repository-context=") {
		return errors.New("submission descriptor bindings are invalid") // refusal:by-design world-action: only a provider code fix can restore authority bindings
	}
	switch submission.Value.Slot {
	case "correction_lines":
		if len(submission.ArgumentTokens) != 7 || submission.ArgumentTokens[6] != "--correction-lines="+reviewSubmissionValuePlaceholder ||
			submission.Value.Domain != "positive_correction_lines" || submission.Value.Schema != "" ||
			submission.Value.Minimum != 1 || submission.Value.Maximum < submission.Value.Minimum ||
			submission.Value.Maximum > reviewtransaction.MaxCorrectionChangedLines || len(submission.Value.AllowedValues) != 0 {
			return errors.New("correction submission descriptor value domain is invalid") // refusal:by-design world-action: only a provider code fix can restore the correction domain
		}
	case "validation":
		if len(submission.ArgumentTokens) != 8 || submission.ArgumentTokens[6] != "--validation="+reviewSubmissionValuePlaceholder ||
			submission.ArgumentTokens[7] != "--captured-evidence=true" || submission.Value.Domain != "artifact_path_or_stdin" ||
			submission.Value.Schema != reviewValidatorSchemaID || submission.Value.Minimum != 0 || submission.Value.Maximum != 0 || len(submission.Value.AllowedValues) != 0 {
			return errors.New("validation submission descriptor value domain is invalid") // refusal:by-design world-action: only a provider code fix can restore the validation domain
		}
	default:
		return errors.New("submission descriptor value slot is unsupported") // refusal:by-design world-action: only a provider code fix can restore a supported value slot
	}
	return nil
}

func reviewTransitionArgumentMap(arguments []ReviewTransitionArgument, operation ...string) (map[string]string, error) {
	allowIntendedUntracked := len(operation) == 1 && operation[0] == "review.start"
	values := make(map[string]string, len(arguments))
	for _, argument := range arguments {
		if previous, duplicate := values[argument.Name]; duplicate && (!allowIntendedUntracked || argument.Name != "intended-untracked" || previous == argument.Value) {
			return nil, errors.New("review transition repeats an argument")
		}
		values[argument.Name] = argument.Value
	}
	return values, nil
}

func validateReviewTransitionExecution(execution ReviewTransitionExecution, arguments map[string]string) error {
	if execution.Command != reviewTransitionCommandLine(execution.Operation, execution.Arguments) {
		return errors.New("execution transition command does not match its arguments") // refusal:by-design world-action: a producer must publish the exact command its executable arguments define
	}
	exact := func(required []string, selectors []ReviewTransitionArgument) bool {
		if len(arguments) != len(required)+len(selectors) {
			return false
		}
		for _, name := range required {
			if _, present := arguments[name]; !present {
				return false
			}
		}
		for _, selector := range selectors {
			if selector.Name != "base-ref" && selector.Name != "committed-only" && selector.Name != "projection" && selector.Name != "workspace-overlay" ||
				arguments[selector.Name] != selector.Value {
				return false
			}
		}
		return true
	}
	switch execution.Operation {
	case "review.validate":
		gate := reviewtransaction.GateKind(arguments["gate"])
		wantSelectors := []ReviewTransitionArgument{}
		if base, present := arguments["base-ref"]; present {
			wantSelectors = append(wantSelectors, ReviewTransitionArgument{Name: "base-ref", Value: base})
		}
		if execution.SelectorArguments != nil && !reflect.DeepEqual(*execution.SelectorArguments, wantSelectors) {
			return errors.New("review validate transition selectors are invalid")
		}
		if !exact([]string{"lineage", "gate"}, wantSelectors) ||
			arguments["lineage"] != execution.Binding.LineageID || !validReviewIntegrationGate(gate) ||
			arguments["base-ref"] != "" &&
				((gate != reviewtransaction.GatePrePush && gate != reviewtransaction.GatePrePR) || !validReviewTransitionSelector(arguments["base-ref"])) {
			return errors.New("review validate transition selectors are invalid")
		}
	case "review.recover":
		wantSelectors := []ReviewTransitionArgument{}
		for _, name := range []string{"base-ref", "committed-only", "projection", "workspace-overlay"} {
			if value, present := arguments[name]; present {
				wantSelectors = append(wantSelectors, ReviewTransitionArgument{Name: name, Value: value})
			}
		}
		if execution.SelectorArguments != nil && !reflect.DeepEqual(*execution.SelectorArguments, wantSelectors) {
			return errors.New("review recover transition selectors are invalid")
		}
		if !exact([]string{"predecessor-lineage", "expected-predecessor-revision", "successor-lineage", "disposition", "reason", "actor", "maintainer-authorization"}, wantSelectors) ||
			arguments["predecessor-lineage"] != execution.Binding.LineageID ||
			arguments["expected-predecessor-revision"] != execution.Binding.Revision ||
			!validReviewIntegrationLineage(arguments["successor-lineage"]) ||
			arguments["successor-lineage"] == execution.Binding.LineageID {
			return errors.New("review recover transition binding is invalid")
		}
		disposition := reviewtransaction.RecoveryDisposition(arguments["disposition"])
		if disposition != reviewtransaction.RecoveryScopeChanged &&
			disposition != reviewtransaction.RecoveryInvalidated &&
			disposition != reviewtransaction.RecoveryEscalated {
			return errors.New("review recover transition disposition is invalid")
		}
		authorizationSuccessor := ""
		if execution.SelectorArguments != nil {
			authorizationSuccessor = arguments["successor-lineage"]
		}
		wantAuthorization := reviewTransitionRecoveryAuthorization(execution.Binding, authorizationSuccessor, arguments["actor"], arguments["reason"])
		base, hasBase := arguments["base-ref"]
		committed, hasCommitted := arguments["committed-only"]
		projection, hasProjection := arguments["projection"]
		workspaceOverlay, hasWorkspaceOverlay := arguments["workspace-overlay"]
		if arguments["maintainer-authorization"] != wantAuthorization ||
			hasBase && !validReviewTransitionSelector(base) ||
			hasCommitted && (!hasBase || committed != "true") ||
			hasProjection && projection != string(reviewtransaction.ProjectionWorkspace) &&
				projection != string(reviewtransaction.ProjectionStaged) ||
			hasWorkspaceOverlay && (!hasBase || hasCommitted || !hasProjection ||
				projection != string(reviewtransaction.ProjectionStaged) || workspaceOverlay != "true") {
			return errors.New("review recover transition selectors are invalid")
		}
	}
	return nil
}

func validReviewTransitionSelector(value string) bool {
	fields := strings.Fields(value)
	return len(fields) == 1 && fields[0] == value && !path.IsAbs(value) &&
		!strings.HasPrefix(value, "-") && !strings.ContainsRune(value, 0)
}

func (eligibility ReviewActionEligibility) Validate(status ReviewTargetStatusResult) error {
	if len(eligibility.AllowedActions) != 1 || eligibility.ForbiddenActions == nil {
		return errors.New("review action eligibility is incomplete")
	}
	allowed := eligibility.AllowedActions[0]
	if strings.TrimSpace(allowed.Action) == "" || strings.TrimSpace(allowed.ReasonCode) == "" || allowed.RequiredInputs == nil {
		return errors.New("review action eligibility has an invalid allowed action")
	}
	if status.Action == reviewtransaction.TargetStatusActionStart && (!status.rddModeResolved || status.rddMode.Enabled()) &&
		(allowed.Action != "review.start" || allowed.ReasonCode != reviewActionEligibleCurrent || len(allowed.RequiredInputs) != 0) {
		return errors.New("fresh target eligibility does not allow START")
	}
	if status.Action == reviewtransaction.TargetStatusActionStart && status.rddModeResolved && !status.rddMode.Enabled() &&
		(allowed.Action != "stop" || allowed.ReasonCode != reviewActionForbiddenRDDDisabled || len(allowed.RequiredInputs) != 0) {
		// refusal:by-design world-action: only a producer defect can advertise START after the resolved mode disabled it
		return errors.New("disabled fresh target eligibility does not stop START")
	}
	seen := map[string]bool{allowed.Action: true}
	if allowed.Action == "review.recover" || allowed.Action == ReviewIntegrationOperationRetryFinalVerification {
		expectedTarget := status.TargetIdentity
		if allowed.Action == ReviewIntegrationOperationRetryFinalVerification {
			expectedTarget = reviewAuthorityTargetIdentity(status)
		}
		if allowed.Disposition != status.ActionDisposition || allowed.Binding == nil ||
			allowed.Binding.TargetIdentity != expectedTarget || status.Authority == nil ||
			allowed.Binding.LineageID != status.Authority.LineageID || allowed.Binding.Revision != status.Authority.Revision {
			return errors.New("recovery eligibility lacks a current native binding")
		}
	} else if allowed.Disposition != "" || allowed.Binding != nil {
		return errors.New("only provider recovery eligibility may contain a binding or disposition")
	}
	for _, forbidden := range eligibility.ForbiddenActions {
		if strings.TrimSpace(forbidden.Action) == "" || strings.TrimSpace(forbidden.ReasonCode) == "" || seen[forbidden.Action] {
			return errors.New("review action eligibility has overlapping or invalid actions")
		}
		seen[forbidden.Action] = true
	}
	for _, action := range reviewManagedActions {
		if !seen[action] {
			return errors.New("review action eligibility does not classify every managed action")
		}
	}
	return nil
}

// ValidateFinalize rejects authorization-bearing guidance from FINALIZE. A
// recovery must be re-derived by target-scoped STATUS before it can carry a
// binding, so FINALIZE can only publish a non-authorizing next action.
func (eligibility ReviewActionEligibility) ValidateFinalize() error {
	if len(eligibility.AllowedActions) != 1 || eligibility.ForbiddenActions == nil {
		return errors.New("finalize action eligibility is incomplete")
	}
	allowed := eligibility.AllowedActions[0]
	if allowed.Action != "stop" || allowed.ReasonCode != reviewActionForbiddenFinalizeStatus ||
		!reflect.DeepEqual(allowed.RequiredInputs, []string{"target_scoped_status"}) || allowed.Disposition != "" || allowed.Binding != nil {
		return errors.New("finalize action eligibility contains authorization guidance")
	}
	seen := map[string]bool{allowed.Action: true}
	for _, forbidden := range eligibility.ForbiddenActions {
		if strings.TrimSpace(forbidden.Action) == "" || strings.TrimSpace(forbidden.ReasonCode) == "" || seen[forbidden.Action] {
			return errors.New("finalize action eligibility has overlapping or invalid actions")
		}
		seen[forbidden.Action] = true
	}
	for _, action := range reviewFinalizeManagedActions {
		if !seen[action] {
			return errors.New("finalize action eligibility does not classify every managed action")
		}
	}
	return nil
}

func (projection ReviewTargetStatusProjection) Validate() error {
	if projection.Schema != ReviewIntegrationProjectionSchema || projection.Paths == nil || projection.IntendedUntracked == nil {
		return errors.New("restart projection is incomplete")
	}
	for _, identity := range []string{projection.PathsDigest, projection.IntendedUntrackedProof, projection.InitialSnapshotIdentity, projection.CurrentSnapshotIdentity} {
		if !validReviewCapabilitySHA256(identity) {
			return errors.New("restart projection contains an invalid content identity")
		}
	}
	for _, tree := range []string{projection.BaseTree, projection.InitialReviewTree, projection.CurrentCandidateTree} {
		if !validReviewGitTree(tree) {
			return errors.New("restart projection contains an invalid Git tree")
		}
	}
	for _, paths := range [][]string{projection.Paths, projection.IntendedUntracked} {
		for _, value := range paths {
			if value == "" || strings.Contains(value, `\`) || strings.HasPrefix(value, "/") || len(value) >= 2 && value[1] == ':' || path.IsAbs(value) || path.Clean(value) != value || value == "." || value == ".." || strings.HasPrefix(value, "../") {
				return fmt.Errorf("restart projection path %q is not repository-relative", value)
			}
		}
	}
	if projection.Projection != reviewtransaction.ProjectionWorkspace && projection.Projection != reviewtransaction.ProjectionStaged {
		return errors.New("restart projection kind is invalid")
	}
	if !reflect.DeepEqual(sortedReviewStatusStrings(projection.Paths), projection.Paths) || !reflect.DeepEqual(sortedReviewStatusStrings(projection.IntendedUntracked), projection.IntendedUntracked) {
		return errors.New("restart projection paths are not canonical")
	}
	return nil
}

func sortedReviewStatusStrings(values []string) []string {
	copy := append([]string{}, values...)
	for index := 1; index < len(copy); index++ {
		for cursor := index; cursor > 0 && copy[cursor] < copy[cursor-1]; cursor-- {
			copy[cursor], copy[cursor-1] = copy[cursor-1], copy[cursor]
		}
	}
	return copy
}

func validReviewGitTree(value string) bool {
	if len(value) != 40 && len(value) != 64 {
		return false
	}
	for _, char := range value {
		if char < '0' || char > '9' && char < 'a' || char > 'f' {
			return false
		}
	}
	return true
}
