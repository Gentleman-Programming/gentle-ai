package cli

import (
	"errors"
	"fmt"
	"path"
	"reflect"
	"strconv"
	"strings"

	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
)

const ReviewIntegrationStatusSchemaV1 = "gentle-ai.review-integration.status/v1"
const ReviewIntegrationStatusSchemaIDV1 = "https://gentle-ai.dev/contracts/review-integration/v1/schemas/status.schema.json"
const ReviewIntegrationStatusSchemaV2 = "gentle-ai.review-integration.status/v2"
const ReviewIntegrationStatusSchemaIDV2 = "https://gentle-ai.dev/contracts/review-integration/v1/schemas/status-v2.schema.json"
const ReviewIntegrationStatusSchema = "gentle-ai.review-integration.status/v3"
const ReviewIntegrationStatusSchemaID = "https://gentle-ai.dev/contracts/review-integration/v2/schemas/status.schema.json"
const ReviewIntegrationProjectionSchema = "gentle-ai.review-integration.projection/v1"
const ReviewIntegrationProjectionSchemaID = "https://gentle-ai.dev/contracts/review-integration/v1/schemas/projection.schema.json"

type ReviewReceiptStatus string

const (
	ReviewReceiptExpectedMissing    ReviewReceiptStatus = "expected_missing"
	ReviewReceiptPresent            ReviewReceiptStatus = "present"
	ReviewReceiptPublicationPending ReviewReceiptStatus = "publication_pending"
	ReviewReceiptNotApplicable      ReviewReceiptStatus = "not_applicable"
)

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
	ActionDisposition       reviewtransaction.RecoveryDisposition                `json:"action_disposition,omitempty"`
	Replayability           reviewtransaction.Replayability                      `json:"replayability"`
	Frozen                  *ReviewTargetStatusFrozen                            `json:"frozen,omitempty"`
	TargetIdentity          string                                               `json:"target_identity"`
	AuthorityTargetIdentity string                                               `json:"authority_target_identity,omitempty"`
	Projection              ReviewTargetStatusProjection                         `json:"projection"`
	Repair                  reviewtransaction.AuthorityRepairAssessment          `json:"repair"`
	Candidates              []string                                             `json:"candidates"`
	Reconciliation          *ReviewFinalizeReconciliation                        `json:"reconciliation,omitempty"`
	Eligibility             *ReviewActionEligibility                             `json:"eligibility,omitempty"`
	NextTransition          *ReviewNextTransition                                `json:"next_transition,omitempty"`
	ValidationRequest       *reviewtransaction.TargetedValidationRequest         `json:"validation_request,omitempty"`
	FinalVerificationRetry  *reviewtransaction.FinalVerificationRetryEligibility `json:"final_verification_retry,omitempty"`
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
		result.Frozen = &ReviewTargetStatusFrozen{
			Tier: native.Tier, OriginalChangedLines: native.OriginalChangedLines, CorrectionBudget: native.CorrectionBudget,
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
		allowed.Action, allowed.ReasonCode = "review.start", reviewActionEligibleCurrent
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

func (result ReviewTargetStatusResult) Validate() error {
	legacyTransport := result.Schema == ReviewIntegrationStatusSchemaV2 && result.Contract == ReviewIntegrationContractV1
	nativeGitTransport := result.Schema == ReviewIntegrationStatusSchema && result.Contract == ReviewIntegrationContractV2
	if (!legacyTransport && !nativeGitTransport) || result.Operation != "review.status" {
		return errors.New("invalid negotiated review status identity")
	}
	if !validReviewCapabilitySHA256(result.TargetIdentity) || result.Candidates == nil {
		return errors.New("invalid negotiated review target identity")
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
		transitionRequest := reviewTransitionValidationRequest(result.NextTransition)
		correctionEvidenceFirst := transitionRequest == nil && result.ValidationRequest != nil &&
			(result.NextTransition.ReasonCode == "correction_repository_verification_required" ||
				result.NextTransition.ReasonCode == "correction_repository_tooling_failed")
		if !correctionEvidenceFirst && ((transitionRequest == nil) != (result.ValidationRequest == nil) ||
			transitionRequest != nil && !reflect.DeepEqual(*transitionRequest, *result.ValidationRequest)) {
			return errors.New("negotiated status validation request copies differ")
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
			budget, err := reviewtransaction.CorrectionBudget(result.Frozen.OriginalChangedLines)
			if err != nil || budget != result.Frozen.CorrectionBudget {
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
		if result.Authority != nil || result.Frozen != nil || result.AuthorityTargetIdentity != "" || result.Receipt.Status != ReviewReceiptNotApplicable || result.Action != reviewtransaction.TargetStatusActionStart && !(result.Action == reviewtransaction.TargetStatusActionStop && result.Projection.Kind == reviewtransaction.TargetBaseWorkspaceOverlay && result.Projection.Projection == reviewtransaction.ProjectionStaged && result.Replayability == reviewtransaction.ReplayabilityManualActionRequired) {
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

func (result ReviewTargetStatusResult) validateNextTransitionTargets() error {
	if result.NextTransition == nil {
		return nil
	}
	if result.Applicability == reviewtransaction.TargetApplicabilityUnrelated {
		if result.Action == reviewtransaction.TargetStatusActionStop {
			if result.NextTransition.Kind != reviewNextTransitionStop || result.NextTransition.ReasonCode != "staged_workspace_overlay_recovery_unavailable" {
				return errors.New("fresh staged workspace-overlay target lacks a STOP transition")
			}
			return nil
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
	arguments, err := reviewTransitionArgumentMap(transition.Execute.Arguments)
	if err != nil {
		return err
	}
	lineage := arguments["lineage"]
	if lineage != "" && !validReviewIntegrationLineage(lineage) {
		return errors.New("fresh target START lineage is not canonical")
	}
	wantArguments := reviewStartArguments(result, lineage)
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
		arguments, err := reviewTransitionArgumentMap(transition.Execute.Arguments)
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
			if input.CaptureOperation == "review.capture-result" {
				order, orderErr := strconv.Atoi(arguments["order"])
				legacyTransport := input.ArtifactSubject != nil && input.ArtifactSubject.Schema == reviewtransaction.ArtifactSubjectSchemaV1
				nativeGitTransport := input.ArtifactSubject != nil && input.ArtifactSubject.Schema == reviewtransaction.ArtifactSubjectSchema
				argumentCount := 6
				if nativeGitTransport {
					argumentCount = 7
				}
				if len(arguments) != argumentCount || !reviewStartSupportedLens(arguments["lens"]) || orderErr != nil || order < 0 ||
					!validReviewCapabilitySHA256(arguments["expected-revision"]) || !validReviewCapabilitySHA256(arguments["target"]) ||
					strings.TrimSpace(arguments["lineage"]) == "" || reviewtransaction.ValidateReviewRepositoryContextHandle(arguments["repository-context"]) != nil ||
					input.ArtifactSubject == nil || input.ChangedPathManifest == nil ||
					nativeGitTransport && arguments["subject-hash"] != input.ArtifactSubject.SubjectHash ||
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
				(input.Schema != reviewtransaction.VerificationEvidenceRecordSchema || len(arguments) != 3 ||
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
		arguments, err := reviewTransitionArgumentMap(transition.Execute.Arguments)
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

func reviewTransitionArgumentMap(arguments []ReviewTransitionArgument) (map[string]string, error) {
	values := make(map[string]string, len(arguments))
	for _, argument := range arguments {
		if _, duplicate := values[argument.Name]; duplicate {
			return nil, errors.New("review transition repeats an argument")
		}
		values[argument.Name] = argument.Value
	}
	return values, nil
}

func validateReviewTransitionExecution(execution ReviewTransitionExecution, arguments map[string]string) error {
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
			arguments["base-ref"] != "" && (gate != reviewtransaction.GatePrePR || !validReviewTransitionSelector(arguments["base-ref"])) {
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
	if status.Action == reviewtransaction.TargetStatusActionStart &&
		(allowed.Action != "review.start" || allowed.ReasonCode != reviewActionEligibleCurrent || len(allowed.RequiredInputs) != 0) {
		return errors.New("fresh target eligibility does not allow START")
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
