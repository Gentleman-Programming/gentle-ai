package cli

import (
	"errors"
	"reflect"

	"github.com/gentleman-programming/gentle-ai/v2/internal/consentenvelope"
	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
)

const ReviewFinalVerificationRetryConsentSchema = "gentle-ai.review-final-verification-retry-consent/v1"

const (
	reviewFinalVerificationRetryConsentGranted  = "granted"
	reviewFinalVerificationRetryConsentDeclined = "declined"
)

type ReviewFinalVerificationRetryConsent struct {
	Schema    string                   `json:"schema"`
	Operation string                   `json:"operation"`
	Action    string                   `json:"action"`
	Agent     string                   `json:"agent,omitempty"`
	Blocking  bool                     `json:"blocking"`
	Headline  string                   `json:"headline"`
	Reason    string                   `json:"reason"`
	Value     string                   `json:"value"`
	Evidence  []string                 `json:"evidence"`
	Choices   []consentenvelope.Choice `json:"choices"`
	OffPath   consentenvelope.OffPath  `json:"off_path"`
}

func newFinalVerificationRetryConsent(binding ReviewTransitionBinding, retry reviewtransaction.FinalVerificationRetryEligibility, agent string) ReviewFinalVerificationRetryConsent {
	base := "gentle-ai review retry-final-verification --repository-context " + binding.RepositoryContext
	offPath := "gentle-ai review status --contract gentle-ai.review-integration/v2"
	if agent != "" {
		offPath += " --agent " + agent
	}
	offPath += " --next-transition --repository-context " + binding.RepositoryContext
	return ReviewFinalVerificationRetryConsent{
		Schema: ReviewFinalVerificationRetryConsentSchema, Operation: ReviewIntegrationOperationRetryFinalVerification,
		Action: "consent_required", Agent: agent, Blocking: true,
		Headline: "Final verification needs a one-time retry decision.",
		Reason:   "Provider-owned evidence proves a procedural tooling failure after this exact candidate reached terminal escalation.",
		Value:    "Granting creates one validating successor for the exact frozen candidate; declining leaves the terminal authority unchanged.",
		Evidence: []string{"target=" + retry.TargetIdentity, "failed_evidence_hash=" + retry.FailedEvidenceHash, "finalize_request_digest=" + retry.FinalizeRequestDigest},
		Choices: []consentenvelope.Choice{
			{Answer: reviewFinalVerificationRetryConsentGranted, Label: "Retry final verification", Effect: "Creates the one provider-derived validating successor for this exact procedural failure.", Invocation: base + " --consent " + reviewFinalVerificationRetryConsentGranted},
			{Answer: reviewFinalVerificationRetryConsentDeclined, Label: "Keep this escalation", Effect: "Leaves lifecycle authority unchanged; a later STATUS query may ask again while this exact retry remains eligible.", Invocation: base + " --consent " + reviewFinalVerificationRetryConsentDeclined},
		},
		OffPath: consentenvelope.OffPath{Note: "Inspect the current provider-owned retry eligibility before choosing.", Command: offPath},
	}
}

func (consent ReviewFinalVerificationRetryConsent) Validate(binding ReviewTransitionBinding, retry reviewtransaction.FinalVerificationRetryEligibility) error {
	if consent.Schema != ReviewFinalVerificationRetryConsentSchema || consent.Operation != ReviewIntegrationOperationRetryFinalVerification || consent.Action != "consent_required" || !consent.Blocking {
		return errors.New("final-verification retry consent identity is invalid")
	}
	if err := (consentenvelope.Core{Headline: consent.Headline, Reason: consent.Reason, Value: consent.Value, Evidence: consent.Evidence, Choices: consent.Choices, OffPath: consent.OffPath}).ValidateCompleteness(reviewFinalVerificationRetryConsentGranted, reviewFinalVerificationRetryConsentDeclined); err != nil {
		return err
	}
	want := newFinalVerificationRetryConsent(binding, retry, consent.Agent)
	if !reflect.DeepEqual(consent, want) {
		return errors.New("final-verification retry consent is not provider-bound")
	}
	return nil
}

type ReviewFinalVerificationRetryConsentResult struct {
	Operation            string `json:"operation"`
	Disposition          string `json:"disposition"`
	PredecessorLineageID string `json:"predecessor_lineage_id"`
	PredecessorRevision  string `json:"predecessor_revision"`
	TargetIdentity       string `json:"target_identity"`
}

func (result ReviewFinalVerificationRetryConsentResult) Validate() error {
	if result.Operation != ReviewIntegrationOperationRetryFinalVerification || result.Disposition != reviewFinalVerificationRetryConsentDeclined ||
		!validReviewIntegrationLineage(result.PredecessorLineageID) || !validReviewCapabilitySHA256(result.PredecessorRevision) ||
		!validReviewCapabilitySHA256(result.TargetIdentity) {
		return errors.New("final-verification retry decline result is incomplete")
	}
	return nil
}
