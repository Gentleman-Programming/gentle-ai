package cli

import (
	"errors"
	"fmt"
	"reflect"
	"strings"

	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
)

const ReviewIntegrationStartSchemaV1 = "gentle-ai.review-integration.start/v1"
const ReviewIntegrationStartSchemaIDV1 = "https://gentle-ai.dev/contracts/review-integration/v1/schemas/start.schema.json"
const ReviewIntegrationStartSchemaV2 = "gentle-ai.review-integration.start/v2"
const ReviewIntegrationStartSchemaIDV2 = "https://gentle-ai.dev/contracts/review-integration/v1/schemas/start-v2.schema.json"
const ReviewIntegrationStartSchema = "gentle-ai.review-integration.start/v3"
const ReviewIntegrationStartSchemaID = "https://gentle-ai.dev/contracts/review-integration/v2/schemas/start.schema.json"

// ReviewIntegrationStartResult is the explicitly negotiated START response.
// The legacy ReviewFacadeStartResult remains byte- and schema-compatible.
type ReviewIntegrationStartResult struct {
	Schema              string                                        `json:"schema"`
	Contract            string                                        `json:"contract"`
	Operation           string                                        `json:"operation"`
	Action              string                                        `json:"action"`
	LensesRequired      bool                                          `json:"lenses_required"`
	LineageID           string                                        `json:"lineage_id"`
	State               reviewtransaction.State                       `json:"state"`
	RiskLevel           reviewtransaction.RiskLevel                   `json:"risk_level"`
	SelectedLenses      []string                                      `json:"selected_lenses"`
	Projection          reviewtransaction.Projection                  `json:"projection"`
	TargetMode          reviewtransaction.TargetKind                  `json:"target_mode,omitempty"`
	TargetIdentity      string                                        `json:"target_identity,omitempty"`
	BaseTree            string                                        `json:"base_tree,omitempty"`
	CandidateTree       string                                        `json:"candidate_tree,omitempty"`
	ChangedFiles        int                                           `json:"changed_files"`
	ChangedLines        int                                           `json:"changed_lines"`
	CorrectionBudget    int                                           `json:"correction_budget"`
	RiskReasons         []reviewtransaction.RiskReason                `json:"risk_reasons"`
	ArtifactSubjects    []reviewtransaction.ArtifactSubject           `json:"artifact_subjects"`
	CandidateDiff       *reviewtransaction.FrozenCandidateDiff        `json:"candidate_diff,omitempty"`
	ChangedPathManifest *[]reviewtransaction.ChangedPathManifestEntry `json:"changed_path_manifest,omitempty"`
	RepositoryContext   *ReviewRepositoryContextReference             `json:"repository_context,omitempty"`
}

// ReviewRepositoryContextReference is the path-free provider context that a
// capture transition can carry across process cwd boundaries.
type ReviewRepositoryContextReference struct {
	Capability     string                                            `json:"capability"`
	Handle         string                                            `json:"handle"`
	Revision       string                                            `json:"revision"`
	TargetIdentity string                                            `json:"target_identity"`
	EventID        string                                            `json:"event_id,omitempty"`
	Outcome        reviewtransaction.CompactRepositoryContextOutcome `json:"outcome,omitempty"`
}

func newReviewIntegrationStartResult(legacy ReviewFacadeStartResult, assessment reviewtransaction.RiskAssessment, targetMode reviewtransaction.TargetKind, frozenContext *reviewtransaction.FrozenCandidateContext, repositoryContext *ReviewRepositoryContextReference, contracts ...string) (ReviewIntegrationStartResult, error) {
	assessment, err := reviewStartAssessmentForFrozenAuthority(legacy, assessment)
	if err != nil {
		return ReviewIntegrationStartResult{}, err
	}
	if assessment.Level != legacy.RiskLevel || assessment.ChangedLines != legacy.ChangedLines {
		return ReviewIntegrationStartResult{}, errors.New("negotiated START risk assessment does not match frozen authority")
	}
	legacyTransport := len(contracts) > 0 && contracts[0] == ReviewIntegrationContractV1
	if legacyTransport && frozenContext != nil && frozenContext.LegacyCandidateDiff == nil {
		return ReviewIntegrationStartResult{}, errors.New("legacy negotiated START requires its published candidate transport") // refusal:by-design world-action: provider-built v1 START omitted required immutable transport and requires a code fix
	}
	schema, contract := ReviewIntegrationStartSchema, ReviewIntegrationContractV2
	if legacyTransport {
		schema, contract = ReviewIntegrationStartSchemaV2, ReviewIntegrationContractV1
	}
	result := ReviewIntegrationStartResult{
		Schema: schema, Contract: contract, Operation: "review.start",
		Action: legacy.Action, LensesRequired: legacy.LensesRequired, LineageID: legacy.LineageID,
		State: legacy.State, RiskLevel: legacy.RiskLevel, SelectedLenses: append([]string{}, legacy.SelectedLenses...),
		Projection: legacy.Projection, ChangedFiles: legacy.ChangedFiles, ChangedLines: legacy.ChangedLines,
		CorrectionBudget: legacy.CorrectionBudget, RiskReasons: append([]reviewtransaction.RiskReason{}, assessment.Reasons...),
		ArtifactSubjects: []reviewtransaction.ArtifactSubject{}, RepositoryContext: repositoryContext,
	}
	if targetMode == reviewtransaction.TargetBaseWorkspaceOverlay {
		result.TargetMode = targetMode
		result.TargetIdentity = legacy.TargetIdentity
		result.BaseTree = legacy.BaseTree
		result.CandidateTree = legacy.CandidateTree
	}
	if frozenContext != nil {
		if result.BaseTree != "" && (result.BaseTree != frozenContext.BaseTree || result.CandidateTree != frozenContext.CandidateTree) {
			return ReviewIntegrationStartResult{}, errors.New("negotiated START trees do not match frozen reviewer context") // refusal:by-design world-action: this provider-built envelope is inconsistent; the exit is a code fix, not a command
		}
		if legacyTransport {
			diff := *frozenContext.LegacyCandidateDiff
			result.CandidateDiff = &diff
		} else {
			result.BaseTree, result.CandidateTree = frozenContext.BaseTree, frozenContext.CandidateTree
		}
		manifest := append([]reviewtransaction.ChangedPathManifestEntry(nil), frozenContext.ChangedPathManifest...)
		if manifest == nil {
			manifest = []reviewtransaction.ChangedPathManifestEntry{}
		}
		result.ChangedPathManifest = &manifest
		if repositoryContext != nil {
			paths := make([]string, len(manifest))
			for index, entry := range manifest {
				paths[index] = entry.Path
			}
			subjectState := reviewtransaction.CompactState{
				LineageID: legacy.LineageID,
				InitialSnapshot: reviewtransaction.Snapshot{
					Identity: repositoryContext.TargetIdentity, BaseTree: frozenContext.BaseTree,
					CandidateTree: frozenContext.CandidateTree, Paths: paths,
				},
				SelectedLenses: append([]string{}, legacy.SelectedLenses...),
			}
			result.ArtifactSubjects = make([]reviewtransaction.ArtifactSubject, len(legacy.SelectedLenses))
			for order, lens := range legacy.SelectedLenses {
				if legacyTransport {
					result.ArtifactSubjects[order], err = reviewtransaction.NewLegacyArtifactSubject(subjectState, repositoryContext.Revision, *frozenContext, lens, order, "")
				} else {
					result.ArtifactSubjects[order], err = reviewtransaction.NewArtifactSubject(subjectState, repositoryContext.Revision, *frozenContext, lens, order, "")
				}
				if err != nil {
					return ReviewIntegrationStartResult{}, fmt.Errorf("derive artifact subject %d: %w", order, err)
				}
			}
		}
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
	if legacy.Action == string(reviewtransaction.CompactStartResumed) && legacy.RiskLevel == reviewtransaction.RiskMedium &&
		assessment.Level == reviewtransaction.RiskHigh && len(assessment.Reasons) > 0 && assessment.Reasons[0].Path != "" &&
		validateReviewStartLenses(legacy.RiskLevel, legacy.SelectedLenses) == nil {
		assessment.Level, assessment.DominantLens = legacy.RiskLevel, ""
		assessment.Reasons = []reviewtransaction.RiskReason{{Code: reviewtransaction.RiskReasonExecutableChange, Path: assessment.Reasons[0].Path}}
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
	legacyTransport := result.Schema == ReviewIntegrationStartSchemaV2 && result.Contract == ReviewIntegrationContractV1
	nativeGitTransport := result.Schema == ReviewIntegrationStartSchema && result.Contract == ReviewIntegrationContractV2
	if (!legacyTransport && !nativeGitTransport) || result.Operation != "review.start" {
		return errors.New("invalid negotiated START identity")
	}
	if strings.TrimSpace(result.LineageID) == "" || result.SelectedLenses == nil || result.RiskReasons == nil || result.ArtifactSubjects == nil {
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
	} else if result.TargetIdentity != "" {
		return errors.New("negotiated non-overlay START cannot contain overlay target identity") // refusal:by-design world-action: this provider-built envelope is inconsistent; the exit is a code fix, not a command
	}
	if result.ChangedFiles < 0 || result.ChangedLines < 0 {
		return errors.New("negotiated START change counts cannot be negative")
	}
	if !reviewContractCorrectionBudgetValid(result.ChangedLines, result.CorrectionBudget) {
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
	hasTrees := result.BaseTree != "" || result.CandidateTree != ""
	hasDiff := result.CandidateDiff != nil
	hasManifest := result.ChangedPathManifest != nil
	if hasTrees && (!validReviewGitTree(result.BaseTree) || !validReviewGitTree(result.CandidateTree)) {
		return errors.New("negotiated START candidate tree context is incomplete") // refusal:by-design world-action: this provider-built envelope is inconsistent; the exit is a code fix, not a command
	}
	if legacyTransport && hasDiff != hasManifest || nativeGitTransport && result.TargetMode == "" && hasTrees != hasManifest ||
		legacyTransport && result.TargetMode == "" && hasTrees || nativeGitTransport && hasDiff {
		return errors.New("negotiated START candidate context is incomplete")
	}
	if len(result.SelectedLenses) > 0 && ((!legacyTransport || !hasDiff) && (!nativeGitTransport || !hasTrees) || !hasManifest) {
		return errors.New("negotiated START selected lenses require frozen candidate context")
	}
	needsRepositoryContext := result.State == reviewtransaction.StateReviewing &&
		(result.Action == string(reviewtransaction.CompactStartCreated) || result.Action == string(reviewtransaction.CompactStartResumed))
	if needsRepositoryContext != (result.RepositoryContext != nil) {
		return errors.New("negotiated START repository context does not match the active reviewing authority")
	}
	if needsRepositoryContext {
		if len(result.ArtifactSubjects) != len(result.SelectedLenses) {
			return errors.New("negotiated START requires one provider artifact subject per selected lens")
		}
	} else if len(result.ArtifactSubjects) != 0 {
		return errors.New("negotiated START cannot expose artifact subjects outside an active reviewing authority")
	}
	if result.RepositoryContext != nil {
		if result.RepositoryContext.Capability != reviewtransaction.ReviewRepositoryContextCapability ||
			reviewtransaction.ValidateReviewRepositoryContextHandle(result.RepositoryContext.Handle) != nil ||
			!validReviewCapabilitySHA256(result.RepositoryContext.Revision) ||
			!validReviewCapabilitySHA256(result.RepositoryContext.TargetIdentity) ||
			result.TargetIdentity != "" && result.RepositoryContext.TargetIdentity != result.TargetIdentity {
			return errors.New("negotiated START repository context is invalid")
		}
		if err := validateReviewRepositoryContextReference(*result.RepositoryContext); err != nil {
			return fmt.Errorf("negotiated START repository context is invalid: %w", err)
		}
	}
	if hasManifest {
		if hasDiff {
			diffBytes, diffErr := result.CandidateDiff.Bytes()
			if diffErr != nil || (len(*result.ChangedPathManifest) == 0) != (len(diffBytes) == 0) {
				return errors.New("negotiated START candidate diff does not match changed-path manifest")
			}
		}
		manifest := *result.ChangedPathManifest
		if err := reviewtransaction.ValidateChangedPathManifest(manifest); err != nil {
			return err
		}
		if len(manifest) != result.ChangedFiles {
			return errors.New("negotiated START changed-path manifest does not match changed_files")
		}
		for order, subject := range result.ArtifactSubjects {
			if err := reviewtransaction.ValidateArtifactSubject(subject); err != nil ||
				subject.LineageID != result.LineageID || subject.AuthorityRevision != result.RepositoryContext.Revision ||
				subject.TargetIdentity != result.RepositoryContext.TargetIdentity ||
				subject.Lens != result.SelectedLenses[order] || subject.SelectedOrder != order || subject.CorrectionTargetIdentity != "" {
				return fmt.Errorf("negotiated START artifact subject %d does not match frozen authority", order)
			}
			if legacyTransport && (subject.Schema != reviewtransaction.ArtifactSubjectSchemaV1 || subject.CandidateDiffSHA256 != result.CandidateDiff.SHA256) ||
				nativeGitTransport && (subject.Schema != reviewtransaction.ArtifactSubjectSchema || subject.BaseTree != result.BaseTree || subject.CandidateTree != result.CandidateTree) {
				return fmt.Errorf("negotiated START artifact subject %d uses the wrong context contract", order) // refusal:by-design world-action: provider-built START mixed contract identities and requires a code fix
			}
			manifestDigest, digestErr := reviewtransaction.ChangedPathManifestDigest(manifest)
			if digestErr != nil || subject.ChangedPathManifestSHA256 != manifestDigest {
				return fmt.Errorf("negotiated START artifact subject %d does not match changed-path manifest", order)
			}
		}
	}
	return nil
}

func validateReviewRepositoryContextReference(reference ReviewRepositoryContextReference) error {
	if (reference.EventID == "") != (reference.Outcome == "") {
		return errors.New("event identity and outcome must be present together") // refusal:by-design world-action: the provider-built envelope is internally inconsistent and requires a code fix
	}
	if reference.EventID == "" {
		return nil
	}
	if !validReviewCapabilitySHA256(reference.EventID) {
		return errors.New("event identity is invalid") // refusal:by-design world-action: the provider-built envelope is internally inconsistent and requires a code fix
	}
	switch reference.Outcome {
	case reviewtransaction.CompactRepositoryContextApplied, reviewtransaction.CompactRepositoryContextPending,
		reviewtransaction.CompactRepositoryContextBlocked, reviewtransaction.CompactRepositoryContextDurabilityLimited:
		return nil
	default:
		return errors.New("event outcome is invalid") // refusal:by-design world-action: the provider-built envelope is internally inconsistent and requires a code fix
	}
}

func reviewContractCorrectionBudgetValid(originalChangedLines, correctionBudget int) bool {
	legacyBudget, err := reviewtransaction.CorrectionBudget(originalChangedLines)
	return err == nil && correctionBudget == legacyBudget
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
		case reviewtransaction.RiskReasonShellSource, reviewtransaction.RiskReasonProcessBoundary, reviewtransaction.RiskReasonProcessScanLimit:
			if reason.Signal != reviewtransaction.SignalShellProcess || reason.Path == "" || reason.OldMode != "" || reason.NewMode != "" {
				return fmt.Errorf("invalid shell/process risk reason %#v", reason)
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
		case reviewtransaction.RiskReasonConfigurationChange, reviewtransaction.RiskReasonExecutableChange,
			reviewtransaction.RiskReasonEmptyContent:
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
