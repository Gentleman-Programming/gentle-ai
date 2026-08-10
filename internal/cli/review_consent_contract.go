package cli

import (
	"errors"
	"fmt"
	"strings"

	"github.com/gentleman-programming/gentle-ai/v2/internal/consentenvelope"
	"github.com/gentleman-programming/gentle-ai/v2/internal/model"
	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
)

const ReviewIntegrationConsentSchema = "gentle-ai.review-integration.consent/v1"
const ReviewIntegrationConsentSchemaID = "https://gentle-ai.dev/contracts/review-integration/v1/schemas/consent.schema.json"
const ReviewIntegrationConsentSchemaV2 = "gentle-ai.review-integration.consent/v2"
const ReviewIntegrationConsentSchemaIDV2 = "https://gentle-ai.dev/contracts/review-integration/v2/schemas/consent.schema.json"
const ReviewIntegrationConsentSchemaV3 = "gentle-ai.review-integration.consent/v3"
const ReviewIntegrationConsentSchemaIDV3 = "https://gentle-ai.dev/contracts/review-integration/v2/schemas/consent-v3.schema.json"

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
	Agent     string `json:"agent,omitempty"`
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
// runnable follow-up invocation scoped to this candidate. It is an alias of
// the shared consent-envelope core's Choice (#2554): the JSON field names are
// shipped contract bytes, pinned byte-for-byte by
// TestReviewConsentEnvelopeSerializedBytesUnchanged.
type ReviewIntegrationConsentChoice = consentenvelope.Choice

// ReviewIntegrationConsentOffPath names the documented permanent-disable
// command that is deliberately not part of the choice set. Alias of the
// shared core's OffPath under the same byte pin.
type ReviewIntegrationConsentOffPath = consentenvelope.OffPath

const reviewConsentActionRequired = "consent_required"

const (
	reviewConsentGrantedEffect = "Reviews this exact frozen candidate now; nothing is granted for later candidates, so each later medium- or high-risk candidate asks again."
	// reviewConsentDeclinedEffect is the published v1 wording. Keep it exact for
	// legacy consumers; v2 states the new candidate-decline delivery behavior.
	reviewConsentDeclinedEffect = "Skips the review for this candidate only; nothing is persisted and the next candidate is asked again. " +
		"This is not the kill switch."
	reviewConsentDeclinedEffectV2 = "Skips the review for this exact candidate only; no review lineage or receipt is created, and ordinary delivery is unmanaged by candidate choice. " +
		"The next candidate is asked again. This is not the kill switch."
)

type reviewConsentEnvelopeText struct {
	headline, reason, value                    string
	evidence                                   []string
	grantedLabel, grantedEffect                string
	declinedLabel, declinedEffect, offPathNote string
}

func reviewConsentEnvelopeTextFor(locale reviewConsentLocale, assessment reviewtransaction.RiskAssessment, contract string) reviewConsentEnvelopeText {
	if locale != reviewConsentLocaleSpanish {
		declinedEffect := reviewConsentDeclinedEffect
		if contract == ReviewIntegrationContractV2 {
			declinedEffect = reviewConsentDeclinedEffectV2
		}
		return reviewConsentEnvelopeText{
			headline: reviewConsentHeadline, reason: reviewConsentReason(assessment), value: reviewConsentValue,
			evidence: reviewConsentRiskEvidence(assessment), grantedLabel: reviewConsentAnswerRunLabel,
			grantedEffect: reviewConsentGrantedEffect, declinedLabel: reviewConsentAnswerNotNowLabel,
			declinedEffect: declinedEffect, offPathNote: reviewConsentOffPathNote,
		}
	}
	return reviewConsentEnvelopeText{
		headline:       "Gentle AI puede revisar este cambio antes de que lo des por terminado.",
		reason:         reviewConsentSpanishReason(assessment),
		value:          "La revisión lleva un poco más de tiempo y hace que el resultado sea considerablemente más seguro.",
		evidence:       reviewConsentSpanishRiskEvidence(assessment),
		grantedLabel:   "Ejecutar la revisión ahora",
		grantedEffect:  "Revisa ahora este candidato congelado exacto; no se otorga nada para candidatos posteriores, por lo que cada candidato posterior de riesgo medio o alto vuelve a pedir confirmación.",
		declinedLabel:  "Ahora no, solo esta vez",
		declinedEffect: "Omite la revisión solo para este candidato exacto; no se crea ninguna línea de revisión ni recibo, y la entrega ordinaria queda sin administrar por elección del candidato. El siguiente candidato vuelve a pedir confirmación. No es el interruptor de apagado.",
		offPathNote:    "Para desactivar las revisiones de forma permanente, ejecuta '" + reviewConsentOffPathCommand + "'.",
	}
}

func reviewConsentSpanishReason(assessment reviewtransaction.RiskAssessment) string {
	evidence := reviewConsentSpanishEvidence(assessment.Reasons)
	if assessment.Level != reviewtransaction.RiskHigh {
		if evidence == "" {
			return "este cambio no es documentación puramente pasiva, por lo que recibe una revisión consolidada."
		}
		return "este cambio no es documentación puramente pasiva, por lo que recibe una revisión consolidada. La revisión parte de " + evidence + "."
	}
	if evidence == "" {
		return "este cambio toca algo sensible, por lo que recibe una revisión más profunda."
	}
	return "este cambio recibe una revisión más profunda porque afecta a " + evidence + "."
}

func reviewConsentSpanishRiskEvidence(assessment reviewtransaction.RiskAssessment) []string {
	switch assessment.Level {
	case reviewtransaction.RiskHigh:
		return reviewConsentSpanishEvidencePhrases(assessment.Reasons)
	case reviewtransaction.RiskMedium:
		return append([]string{"este cambio no es documentación puramente pasiva, por lo que recibe una revisión consolidada."}, reviewConsentSpanishEvidencePhrases(assessment.Reasons)...)
	default:
		return nil
	}
}

func reviewConsentSpanishEvidence(reasons []reviewtransaction.RiskReason) string {
	phrases := reviewConsentSpanishEvidencePhrases(reasons)
	switch len(phrases) {
	case 0:
		return ""
	case 1:
		return phrases[0]
	case 2:
		return phrases[0] + " y " + phrases[1]
	default:
		return fmt.Sprintf("%s, %s y %d más", phrases[0], phrases[1], len(phrases)-2)
	}
}

func reviewConsentSpanishEvidencePhrases(reasons []reviewtransaction.RiskReason) []string {
	var phrases []string
	for _, reason := range reasons {
		if phrase := reviewConsentSpanishEvidencePhrase(reason); phrase != "" {
			phrases = append(phrases, phrase)
		}
	}
	return phrases
}

func reviewConsentSpanishEvidencePhrase(reason reviewtransaction.RiskReason) string {
	if reason.Code == reviewtransaction.RiskReasonEmptyContent {
		if strings.TrimSpace(reason.Path) == "" {
			return ""
		}
		return fmt.Sprintf("%s, un archivo vacío cuyo tipo no puede determinarse por su contenido", reason.Path)
	}
	subject := reviewConsentSpanishEvidenceSubject(reason)
	if subject == "" || strings.TrimSpace(reason.Path) == "" {
		return subject
	}
	return fmt.Sprintf("%s en %s", subject, reason.Path)
}

func reviewConsentSpanishEvidenceSubject(reason reviewtransaction.RiskReason) string {
	switch reason.Code {
	case reviewtransaction.RiskReasonServiceToken:
		return "credenciales de servicio"
	case reviewtransaction.RiskReasonShellSource:
		return "scripts de shell"
	case reviewtransaction.RiskReasonProcessBoundary, reviewtransaction.RiskReasonProcessScanLimit:
		return "código que inicia otros procesos"
	case reviewtransaction.RiskReasonExecutableMode:
		return "un cambio de permiso ejecutable"
	case reviewtransaction.RiskReasonExecutableChange:
		return "un cambio ejecutable"
	case reviewtransaction.RiskReasonConfigurationChange:
		return "un cambio de configuración"
	case reviewtransaction.RiskReasonHotPath:
		return reviewConsentSpanishSignalSubject(reason.Signal)
	default:
		return ""
	}
}

func reviewConsentSpanishSignalSubject(signal reviewtransaction.RiskSignal) string {
	switch signal {
	case reviewtransaction.SignalAuth:
		return "autenticación"
	case reviewtransaction.SignalSecurity:
		return "seguridad"
	case reviewtransaction.SignalPayments:
		return "pagos"
	case reviewtransaction.SignalDataExposure:
		return "exposición de datos"
	case reviewtransaction.SignalDataLoss:
		return "pérdida de datos"
	case reviewtransaction.SignalPermissions:
		return "permisos"
	case reviewtransaction.SignalUpdate:
		return "la ruta de actualización"
	case reviewtransaction.SignalShellProcess:
		return "ejecución de shell o procesos"
	default:
		return "un área sensible"
	}
}

// newReviewIntegrationConsentResult projects the frozen candidate, its risk
// assessment, and the caller's own invocation into the typed consent question.
// Every phrase comes from the same wording sources the interactive prompt
// uses, so the relayed question and the terminal question cannot drift.
//
// runtimeAgent is the exact generated runtime identity the caller's own
// negotiated START already validated (review_facade.go's
// reviewRuntimeWithImmutableTransport gate runs before this constructor is
// ever reached whenever a runtime is declared at all), never a re-parse of
// followUpBase: parsing a rendered command back into structured data is
// exactly the class of bug this repo refuses. An undeclared runtime (the
// manual/non-agent compatibility path -- see review_facade.go's own comment
// on that path being "not gated") keeps today's compatibility default.
func newReviewIntegrationConsentResult(
	snapshot reviewtransaction.Snapshot,
	assessment reviewtransaction.RiskAssessment,
	followUpBase string,
	contract string,
	runtimeAgent string,
	locale reviewConsentLocale,
) (ReviewIntegrationConsentResult, error) {
	// The evidence phrases may legitimately be empty (a large change with no
	// sensitive path still escalates); the reason sentence always explains the
	// tier, and an empty list must encode as [] rather than null.
	copy := reviewConsentEnvelopeTextFor(locale, assessment, contract)
	if copy.evidence == nil {
		copy.evidence = []string{}
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
		Headline:       copy.headline,
		Reason:         copy.reason,
		Value:          copy.value,
		RiskEvidence:   copy.evidence,
		Choices: []ReviewIntegrationConsentChoice{
			{
				Answer:     string(reviewConsentModeGranted),
				Label:      copy.grantedLabel,
				Effect:     copy.grantedEffect,
				Invocation: followUpBase + " --consent " + string(reviewConsentModeGranted),
			},
			{
				Answer:     string(reviewConsentModeDeclined),
				Label:      copy.declinedLabel,
				Effect:     copy.declinedEffect,
				Invocation: followUpBase + " --consent " + string(reviewConsentModeDeclined),
			},
		},
		OffPath: ReviewIntegrationConsentOffPath{
			Note:    copy.offPathNote,
			Command: reviewConsentOffPathCommand,
		},
	}
	if contract == ReviewIntegrationContractV2 {
		// Issue #2676: this literal used to be unconditional, so a negotiated
		// START explicitly bound to another runtime (OpenCode, Codex) still
		// reported "claude-code" here while its own follow-up invocations
		// below were already rendered from the real binding. Bind the same
		// declared identity the caller proved eligible; only the undeclared
		// compatibility path (no runtime named at all) keeps the historical
		// default, matching every existing manual-caller test and fixture.
		agent := strings.TrimSpace(runtimeAgent)
		if agent == "" {
			agent = "claude-code"
		}
		result.Schema, result.Contract, result.Agent = ReviewIntegrationConsentSchemaV3, ReviewIntegrationContractV2, agent
	}
	if err := result.Validate(); err != nil {
		return ReviewIntegrationConsentResult{}, fmt.Errorf("validate consent question: %w", err)
	}
	if err := validateReviewConsentInvocations(result, followUpBase); err != nil {
		return ReviewIntegrationConsentResult{}, fmt.Errorf("validate consent invocations: %w", err)
	}
	return result, nil
}

// validateReviewConsentInvocations compares provider-owned command bytes with
// the same renderer that created the consent request. It never parses a command
// supplied by a caller, so duplicate or substituted runtime flags cannot pass.
func validateReviewConsentInvocations(result ReviewIntegrationConsentResult, followUpBase string) error {
	for _, choice := range result.Choices {
		expected := followUpBase + " --consent " + choice.Answer
		if choice.Invocation != expected {
			return fmt.Errorf("consent choice %q invocation does not match the provider-owned request", choice.Answer) // refusal:by-design world-action: provider-owned bytes are an internal invariant; the exit is a code fix, not a command
		}
	}
	return nil
}

func (result ReviewIntegrationConsentResult) Validate() error {
	legacyContract := result.Schema == ReviewIntegrationConsentSchema && result.Contract == ReviewIntegrationContractV1
	historicalNativeGitContract := result.Schema == ReviewIntegrationConsentSchemaV2 && result.Contract == ReviewIntegrationContractV2 && result.Agent == ""
	// The v3 shape must name a runtime that can actually carry immutable
	// receipt-review transport -- the exact same authority
	// reviewRuntimeWithImmutableTransport gates negotiated START on (Wave 4
	// S4's fixed RDD policy) -- rather than a fresh allowlist that could drift
	// from it. This accepts every declared runtime proven eligible at START
	// (claude-code, opencode, codex today), and fail-closed rejects an empty
	// identity, an unknown string, and a runtime that is eligible under the
	// RDD policy but still dormant for this contract (e.g. Kilocode has no
	// proven fresh-reviewer boundary yet), because none of those can ever
	// legitimately reach this envelope.
	currentNativeGitContract := result.Schema == ReviewIntegrationConsentSchemaV3 && result.Contract == ReviewIntegrationContractV2 &&
		reviewImmutableRuntimeCapability(model.AgentID(result.Agent)).supportsImmutableReceiptReview()
	if (!legacyContract && !historicalNativeGitContract && !currentNativeGitContract) ||
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
	// The completeness half (non-empty triple, evidence non-nil, exactly the
	// two token choices with label, effect, and a runnable invocation, off
	// path documented) is the shared core's contract (#2554); everything
	// around it in this method is review identity and stays here.
	core := consentenvelope.Core{
		Headline: result.Headline, Reason: result.Reason, Value: result.Value,
		Evidence: result.RiskEvidence, Choices: result.Choices, OffPath: result.OffPath,
	}
	if err := core.ValidateCompleteness(string(reviewConsentModeGranted), string(reviewConsentModeDeclined)); err != nil {
		return err
	}
	for _, choice := range result.Choices {
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
