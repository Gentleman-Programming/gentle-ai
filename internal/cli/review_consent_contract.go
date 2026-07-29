package cli

import (
	"errors"
	"fmt"
	"strings"

	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
)

const ReviewIntegrationConsentSchema = "gentle-ai.review-integration.consent/v1"
const ReviewIntegrationConsentSchemaID = "https://gentle-ai.dev/contracts/review-integration/v1/schemas/consent.schema.json"
const ReviewIntegrationConsentSchemaV2 = "gentle-ai.review-integration.consent/v2"
const ReviewIntegrationConsentSchemaIDV2 = "https://gentle-ai.dev/contracts/review-integration/v2/schemas/consent.schema.json"

// ReviewIntegrationConsentResult is the typed per-candidate consent question a
// relay-declared negotiated START answers with instead of proceeding. It is a
// blocking decision envelope in the sense of the orchestrator contract's
// Lossless Blocking Prompts rule: it carries WHY input is required (the same
// human phrases the interactive question speaks), the COMPLETE choice set, and
// the EXACT runnable way to answer, scoped to the one frozen candidate.
//
// It exists only behind the --consent relay declaration. A caller that
// declares nothing never sees this shape, which is what keeps every already
// shipped negotiated flow byte-for-byte unchanged.
type ReviewIntegrationConsentResult struct {
	Schema    string `json:"schema"`
	Contract  string `json:"contract"`
	Operation string `json:"operation"`
	Action    string `json:"action"`
	// Blocking marks this envelope as a decision the caller must relay before
	// any review work starts; nothing has been persisted.
	Blocking       bool                             `json:"blocking"`
	TargetIdentity string                           `json:"target_identity"`
	Projection     reviewtransaction.Projection     `json:"projection"`
	RiskLevel      reviewtransaction.RiskLevel      `json:"risk_level"`
	ChangedFiles   int                              `json:"changed_files"`
	ChangedLines   int                              `json:"changed_lines"`
	Headline       string                           `json:"headline"`
	Reason         string                           `json:"reason"`
	Value          string                           `json:"value"`
	RiskEvidence   []string                         `json:"risk_evidence"`
	Choices        []ReviewIntegrationConsentChoice `json:"choices"`
	// OffPath documents the permanent disable outside the choice set, exactly
	// as the interactive prompt does: a decline is deliberately not the kill
	// switch, and turning reviews off for good must cost more than answering
	// a relayed question in a hurry.
	OffPath ReviewIntegrationConsentOffPath `json:"off_path"`
}

// ReviewIntegrationConsentChoice is one allowed answer: its token, the human
// label the interactive prompt uses, what choosing it does, and the exact
// runnable follow-up invocation scoped to this candidate.
type ReviewIntegrationConsentChoice struct {
	Answer     string `json:"answer"`
	Label      string `json:"label"`
	Effect     string `json:"effect"`
	Invocation string `json:"invocation"`
}

// ReviewIntegrationConsentOffPath names the documented permanent-disable
// command that is deliberately not part of the choice set.
type ReviewIntegrationConsentOffPath struct {
	Note    string `json:"note"`
	Command string `json:"command"`
}

const reviewConsentActionRequired = "consent_required"

const (
	reviewConsentGrantedEffect  = "Reviews this candidate now and records the one-time question as answered, so future candidates are reviewed without asking again."
	reviewConsentDeclinedEffect = "Skips the review for this candidate only; nothing is persisted and the next candidate is asked again. " +
		"This is not the kill switch."
)

// newReviewIntegrationConsentResult projects the frozen candidate, its risk
// assessment, and the caller's own invocation into the typed consent question.
// Every phrase comes from the same wording sources the interactive prompt
// uses, so the relayed question and the terminal question cannot drift.
func newReviewIntegrationConsentResult(
	snapshot reviewtransaction.Snapshot,
	assessment reviewtransaction.RiskAssessment,
	followUpBase string,
	contract string,
) (ReviewIntegrationConsentResult, error) {
	// The evidence phrases may legitimately be empty (a large change with no
	// sensitive path still escalates); the reason sentence always explains the
	// tier, and an empty list must encode as [] rather than null.
	evidence := reviewConsentRiskEvidence(assessment)
	if evidence == nil {
		evidence = []string{}
	}
	result := ReviewIntegrationConsentResult{
		Schema:         ReviewIntegrationConsentSchema,
		Contract:       ReviewIntegrationContractV1,
		Operation:      "review.start",
		Action:         reviewConsentActionRequired,
		Blocking:       true,
		TargetIdentity: snapshot.Identity,
		Projection:     facadeProjection(snapshot.Projection),
		RiskLevel:      assessment.Level,
		ChangedFiles:   len(snapshot.Paths),
		ChangedLines:   assessment.ChangedLines,
		Headline:       reviewConsentHeadline,
		Reason:         reviewConsentReason(assessment),
		Value:          reviewConsentValue,
		RiskEvidence:   evidence,
		Choices: []ReviewIntegrationConsentChoice{
			{
				Answer:     string(reviewConsentModeGranted),
				Label:      reviewConsentAnswerRunLabel,
				Effect:     reviewConsentGrantedEffect,
				Invocation: followUpBase + " --consent " + string(reviewConsentModeGranted),
			},
			{
				Answer:     string(reviewConsentModeDeclined),
				Label:      reviewConsentAnswerNotNowLabel,
				Effect:     reviewConsentDeclinedEffect,
				Invocation: followUpBase + " --consent " + string(reviewConsentModeDeclined),
			},
		},
		OffPath: ReviewIntegrationConsentOffPath{
			Note:    reviewConsentOffPathNote,
			Command: reviewConsentOffPathCommand,
		},
	}
	if contract == ReviewIntegrationContractV2 {
		result.Schema, result.Contract = ReviewIntegrationConsentSchemaV2, ReviewIntegrationContractV2
	}
	if err := result.Validate(); err != nil {
		return ReviewIntegrationConsentResult{}, fmt.Errorf("validate consent question: %w", err)
	}
	return result, nil
}

func (result ReviewIntegrationConsentResult) Validate() error {
	legacyContract := result.Schema == ReviewIntegrationConsentSchema && result.Contract == ReviewIntegrationContractV1
	nativeGitContract := result.Schema == ReviewIntegrationConsentSchemaV2 && result.Contract == ReviewIntegrationContractV2
	if (!legacyContract && !nativeGitContract) ||
		result.Operation != "review.start" || result.Action != reviewConsentActionRequired || !result.Blocking {
		return errors.New("invalid consent question identity") // refusal:by-design world-action: this envelope is built and validated by the same file; the exit is a code fix, not a command
	}
	if !validReviewCapabilitySHA256(result.TargetIdentity) {
		return errors.New("consent question requires the exact frozen target identity") // refusal:by-design world-action: this envelope is built and validated by the same file; the exit is a code fix, not a command
	}
	if result.Projection != reviewtransaction.ProjectionWorkspace && result.Projection != reviewtransaction.ProjectionStaged {
		return fmt.Errorf("unsupported consent question projection %q", result.Projection) // refusal:by-design world-action: this envelope is built and validated by the same file; the exit is a code fix, not a command
	}
	if result.RiskLevel != reviewtransaction.RiskMedium && result.RiskLevel != reviewtransaction.RiskHigh {
		return fmt.Errorf("consent question tier %q asks no question", result.RiskLevel) // refusal:by-design world-action: this envelope is built and validated by the same file; the exit is a code fix, not a command
	}
	if result.ChangedFiles < 0 || result.ChangedLines < 0 {
		return errors.New("consent question change counts cannot be negative") // refusal:by-design world-action: this envelope is built and validated by the same file; the exit is a code fix, not a command
	}
	if result.Headline == "" || result.Reason == "" || result.Value == "" || result.RiskEvidence == nil {
		return errors.New("consent question must state why input is required") // refusal:by-design world-action: this envelope is built and validated by the same file; the exit is a code fix, not a command
	}
	if len(result.Choices) != 2 ||
		result.Choices[0].Answer != string(reviewConsentModeGranted) ||
		result.Choices[1].Answer != string(reviewConsentModeDeclined) {
		return errors.New("consent question requires exactly the granted and declined choices") // refusal:by-design world-action: this envelope is built and validated by the same file; the exit is a code fix, not a command
	}
	for _, choice := range result.Choices {
		if choice.Label == "" || choice.Effect == "" {
			return fmt.Errorf("consent choice %q is incomplete", choice.Answer) // refusal:by-design world-action: this envelope is built and validated by the same file; the exit is a code fix, not a command
		}
		if !strings.HasPrefix(choice.Invocation, "gentle-ai review start ") ||
			!strings.Contains(choice.Invocation, " --target "+result.TargetIdentity) ||
			!strings.Contains(choice.Invocation, " --consent "+choice.Answer) {
			return fmt.Errorf("consent choice %q does not name a runnable candidate-scoped invocation", choice.Answer) // refusal:by-design world-action: this envelope is built and validated by the same file; the exit is a code fix, not a command
		}
	}
	if result.OffPath.Note == "" || result.OffPath.Command != reviewConsentOffPathCommand {
		return errors.New("consent question must document the deliberate off path") // refusal:by-design world-action: this envelope is built and validated by the same file; the exit is a code fix, not a command
	}
	return nil
}
