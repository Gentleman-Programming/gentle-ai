package cli

import (
	"errors"
	"fmt"
	"reflect"
	"strings"

	"github.com/gentleman-programming/gentle-ai/internal/reviewtransaction"
)

const ReviewIntegrationStartSchema = "gentle-ai.review-integration.start/v1"
const ReviewIntegrationStartSchemaID = "https://gentle-ai.dev/contracts/review-integration/v1/schemas/start.schema.json"

// ReviewIntegrationStartResult is the explicitly negotiated START response.
// The legacy ReviewFacadeStartResult remains byte- and schema-compatible.
type ReviewIntegrationStartResult struct {
	Schema           string                         `json:"schema"`
	Contract         string                         `json:"contract"`
	Operation        string                         `json:"operation"`
	Action           string                         `json:"action"`
	LensesRequired   bool                           `json:"lenses_required"`
	LineageID        string                         `json:"lineage_id"`
	State            reviewtransaction.State        `json:"state"`
	RiskLevel        reviewtransaction.RiskLevel    `json:"risk_level"`
	SelectedLenses   []string                       `json:"selected_lenses"`
	Projection       reviewtransaction.Projection   `json:"projection"`
	TargetMode       reviewtransaction.TargetKind   `json:"target_mode,omitempty"`
	TargetIdentity   string                         `json:"target_identity,omitempty"`
	BaseTree         string                         `json:"base_tree,omitempty"`
	CandidateTree    string                         `json:"candidate_tree,omitempty"`
	ChangedFiles     int                            `json:"changed_files"`
	ChangedLines     int                            `json:"changed_lines"`
	CorrectionBudget int                            `json:"correction_budget"`
	RiskReasons      []reviewtransaction.RiskReason `json:"risk_reasons"`
}

func newReviewIntegrationStartResult(legacy ReviewFacadeStartResult, assessment reviewtransaction.RiskAssessment, targetMode reviewtransaction.TargetKind) (ReviewIntegrationStartResult, error) {
	assessment, err := reviewStartAssessmentForFrozenAuthority(legacy, assessment)
	if err != nil {
		return ReviewIntegrationStartResult{}, err
	}
	if assessment.Level != legacy.RiskLevel || assessment.ChangedLines != legacy.ChangedLines {
		return ReviewIntegrationStartResult{}, errors.New("negotiated START risk assessment does not match frozen authority")
	}
	result := ReviewIntegrationStartResult{
		Schema: ReviewIntegrationStartSchema, Contract: ReviewIntegrationContractV1, Operation: "review.start",
		Action: legacy.Action, LensesRequired: legacy.LensesRequired, LineageID: legacy.LineageID,
		State: legacy.State, RiskLevel: legacy.RiskLevel, SelectedLenses: append([]string{}, legacy.SelectedLenses...),
		Projection: legacy.Projection, ChangedFiles: legacy.ChangedFiles, ChangedLines: legacy.ChangedLines,
		CorrectionBudget: legacy.CorrectionBudget, RiskReasons: append([]reviewtransaction.RiskReason{}, assessment.Reasons...),
	}
	if targetMode == reviewtransaction.TargetBaseWorkspaceOverlay {
		result.TargetMode = targetMode
		result.TargetIdentity = legacy.TargetIdentity
		result.BaseTree = legacy.BaseTree
		result.CandidateTree = legacy.CandidateTree
	}
	if err := result.Validate(); err != nil {
		return ReviewIntegrationStartResult{}, fmt.Errorf("validate negotiated START response: %w", err)
	}
	return result, nil
}

func reviewStartAssessmentForFrozenAuthority(legacy ReviewFacadeStartResult, assessment reviewtransaction.RiskAssessment) (reviewtransaction.RiskAssessment, error) {
	if assessment.ChangedLines != legacy.ChangedLines {
		return reviewtransaction.RiskAssessment{}, errors.New("negotiated START changed lines do not match frozen authority")
	}
	if assessment.Level == legacy.RiskLevel {
		return assessment, nil
	}
	// Authorities created before the pure-documentation policy remain valid and
	// retain their frozen high/4R route when the same immutable snapshot replays.
	if legacy.RiskLevel == reviewtransaction.RiskHigh && assessment.Level == reviewtransaction.RiskMedium &&
		assessment.DominantLens == reviewtransaction.LensReadability && len(assessment.Reasons) == 2 &&
		assessment.Reasons[0].Code == reviewtransaction.RiskReasonLargeChange &&
		assessment.Reasons[1].Code == reviewtransaction.RiskReasonNonExecutableOnly &&
		validateReviewStartLenses(legacy.RiskLevel, legacy.SelectedLenses) == nil {
		assessment.Level = reviewtransaction.RiskHigh
		assessment.DominantLens = ""
		assessment.Reasons = []reviewtransaction.RiskReason{{Code: reviewtransaction.RiskReasonLargeChange}}
		return assessment, nil
	}
	return reviewtransaction.RiskAssessment{}, errors.New("negotiated START risk assessment does not match frozen authority")
}

func (result ReviewIntegrationStartResult) Validate() error {
	if result.Schema != ReviewIntegrationStartSchema || result.Contract != ReviewIntegrationContractV1 || result.Operation != "review.start" {
		return errors.New("invalid negotiated START identity")
	}
	if strings.TrimSpace(result.LineageID) == "" || result.SelectedLenses == nil || result.RiskReasons == nil {
		return errors.New("negotiated START response is incomplete")
	}
	switch result.Action {
	case string(reviewtransaction.CompactStartCreated), string(reviewtransaction.CompactStartResumed),
		string(reviewtransaction.CompactStartReuseReceipt), string(reviewtransaction.CompactStartBlocked):
	default:
		return fmt.Errorf("unsupported negotiated START action %q", result.Action)
	}
	if result.Projection != reviewtransaction.ProjectionWorkspace && result.Projection != reviewtransaction.ProjectionStaged {
		return fmt.Errorf("unsupported negotiated START projection %q", result.Projection)
	}
	if result.TargetMode != "" && result.TargetMode != reviewtransaction.TargetBaseWorkspaceOverlay {
		return fmt.Errorf("unsupported negotiated START target mode %q", result.TargetMode)
	}
	if result.TargetMode == reviewtransaction.TargetBaseWorkspaceOverlay {
		if !validReviewCapabilitySHA256(result.TargetIdentity) || !validReviewGitTree(result.BaseTree) || !validReviewGitTree(result.CandidateTree) {
			return errors.New("negotiated overlay START target identity is incomplete")
		}
	} else if result.TargetIdentity != "" || result.BaseTree != "" || result.CandidateTree != "" {
		return errors.New("negotiated non-overlay START cannot contain overlay identity")
	}
	if result.ChangedFiles < 0 || result.ChangedLines < 0 {
		return errors.New("negotiated START change counts cannot be negative")
	}
	budget, err := reviewtransaction.CorrectionBudget(result.ChangedLines)
	if err != nil || budget != result.CorrectionBudget {
		return errors.New("negotiated START correction budget is inconsistent")
	}
	if err := validateReviewStartRiskReasons(result.RiskReasons); err != nil {
		return err
	}
	if reviewStartRiskLevel(result.RiskReasons) != result.RiskLevel {
		return errors.New("negotiated START risk reasons do not match the frozen tier")
	}
	if err := validateReviewStartLenses(result.RiskLevel, result.SelectedLenses); err != nil {
		return err
	}
	return nil
}

func validateReviewStartRiskReasons(reasons []reviewtransaction.RiskReason) error {
	if len(reasons) == 0 {
		return errors.New("negotiated START requires at least one risk reason")
	}
	for index, reason := range reasons {
		if index > 0 && !reviewStartRiskReasonLess(reasons[index-1], reason) {
			return errors.New("negotiated START risk reasons are not canonical")
		}
		switch reason.Code {
		case reviewtransaction.RiskReasonHotPath:
			if reason.Path == "" || reason.OldMode != "" || reason.NewMode != "" ||
				(reason.Signal != reviewtransaction.SignalAuth && reason.Signal != reviewtransaction.SignalUpdate &&
					reason.Signal != reviewtransaction.SignalSecurity && reason.Signal != reviewtransaction.SignalPayments) {
				return fmt.Errorf("invalid hot-path risk reason %#v", reason)
			}
		case reviewtransaction.RiskReasonServiceToken:
			if reason.Signal != reviewtransaction.SignalAuth || reason.Path == "" || reason.OldMode != "" || reason.NewMode != "" {
				return fmt.Errorf("invalid service-token risk reason %#v", reason)
			}
		case reviewtransaction.RiskReasonShellSource:
			if reason.Signal != reviewtransaction.SignalShellProcess || reason.Path == "" || reason.OldMode != "" || reason.NewMode != "" {
				return fmt.Errorf("invalid shell-source risk reason %#v", reason)
			}
		case reviewtransaction.RiskReasonExecutableMode:
			if reason.Signal != reviewtransaction.SignalPermissions || reason.Path == "" || reason.OldMode == "" || reason.NewMode == "" || reason.OldMode == reason.NewMode {
				return fmt.Errorf("invalid executable-mode risk reason %#v", reason)
			}
		case reviewtransaction.RiskReasonLargeChange:
			if reason.Signal != "" || reason.Path != "" || reason.OldMode != "" || reason.NewMode != "" {
				return fmt.Errorf("invalid large-change risk reason %#v", reason)
			}
		case reviewtransaction.RiskReasonNonExecutableOnly:
			if reason.Signal != "" || reason.Path != "" || reason.OldMode != "" || reason.NewMode != "" {
				return fmt.Errorf("invalid non-executable-only risk reason %#v", reason)
			}
		case reviewtransaction.RiskReasonConfigurationChange, reviewtransaction.RiskReasonExecutableChange:
			if reason.Signal != "" || reason.Path == "" || reason.OldMode != "" || reason.NewMode != "" {
				return fmt.Errorf("invalid fallback risk reason %#v", reason)
			}
		default:
			return fmt.Errorf("unsupported negotiated START risk reason %q", reason.Code)
		}
	}
	return nil
}

func reviewStartRiskReasonLess(left, right reviewtransaction.RiskReason) bool {
	if left.Code != right.Code {
		return left.Code < right.Code
	}
	if left.Signal != right.Signal {
		return left.Signal < right.Signal
	}
	if left.Path != right.Path {
		return left.Path < right.Path
	}
	if left.OldMode != right.OldMode {
		return left.OldMode < right.OldMode
	}
	return left.NewMode < right.NewMode
}

func reviewStartRiskLevel(reasons []reviewtransaction.RiskReason) reviewtransaction.RiskLevel {
	largeChange := false
	nonExecutableOnly := false
	for _, reason := range reasons {
		if reason.Signal != "" {
			return reviewtransaction.RiskHigh
		}
		largeChange = largeChange || reason.Code == reviewtransaction.RiskReasonLargeChange
		nonExecutableOnly = nonExecutableOnly || reason.Code == reviewtransaction.RiskReasonNonExecutableOnly
	}
	if largeChange {
		if nonExecutableOnly && len(reasons) == 2 {
			return reviewtransaction.RiskMedium
		}
		return reviewtransaction.RiskHigh
	}
	if len(reasons) == 1 && reasons[0].Code == reviewtransaction.RiskReasonNonExecutableOnly {
		return reviewtransaction.RiskLow
	}
	return reviewtransaction.RiskMedium
}

func validateReviewStartLenses(risk reviewtransaction.RiskLevel, lenses []string) error {
	switch risk {
	case reviewtransaction.RiskLow:
		if len(lenses) != 0 {
			return errors.New("low-risk negotiated START cannot select lenses")
		}
	case reviewtransaction.RiskMedium:
		if len(lenses) != 1 || !reviewStartSupportedLens(lenses[0]) {
			return errors.New("medium-risk negotiated START requires one supported lens")
		}
	case reviewtransaction.RiskHigh:
		want := []string{
			reviewtransaction.LensRisk, reviewtransaction.LensResilience,
			reviewtransaction.LensReadability, reviewtransaction.LensReliability,
		}
		if !reflect.DeepEqual(lenses, want) {
			return errors.New("high-risk negotiated START requires canonical 4R lenses")
		}
	default:
		return fmt.Errorf("unsupported negotiated START risk %q", risk)
	}
	return nil
}

func reviewStartSupportedLens(lens string) bool {
	switch lens {
	case reviewtransaction.LensRisk, reviewtransaction.LensResilience,
		reviewtransaction.LensReadability, reviewtransaction.LensReliability:
		return true
	default:
		return false
	}
}
