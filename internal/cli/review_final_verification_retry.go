package cli

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
)

// ReviewFinalVerificationRetryResult is intentionally path-free. The failed
// evidence remains a provider-owned predecessor artifact and the incident is
// represented only by its content digest in public output.
type ReviewFinalVerificationRetryResult struct {
	Operation            string                                `json:"operation"`
	PredecessorLineageID string                                `json:"predecessor_lineage_id"`
	PredecessorRevision  string                                `json:"predecessor_revision"`
	LineageID            string                                `json:"lineage_id"`
	State                reviewtransaction.State               `json:"state"`
	StoreRevision        string                                `json:"store_revision"`
	TargetIdentity       string                                `json:"target_identity"`
	IncidentDigest       string                                `json:"incident_digest"`
	RecoveryDisposition  reviewtransaction.RecoveryDisposition `json:"recovery_disposition"`
}

func (result ReviewFinalVerificationRetryResult) Validate() error {
	if result.Operation != ReviewIntegrationOperationRetryFinalVerification ||
		!validReviewIntegrationLineage(result.PredecessorLineageID) ||
		!validReviewIntegrationLineage(result.LineageID) ||
		result.PredecessorLineageID == result.LineageID ||
		!validReviewCapabilitySHA256(result.PredecessorRevision) ||
		!validReviewCapabilitySHA256(result.StoreRevision) ||
		!validReviewCapabilitySHA256(result.TargetIdentity) ||
		!validReviewCapabilitySHA256(result.IncidentDigest) ||
		result.State != reviewtransaction.StateValidating ||
		result.RecoveryDisposition != reviewtransaction.RecoveryFinalVerificationRetry {
		return errors.New("final-verification retry result is incomplete")
	}
	return nil
}

func runReviewRetryFinalVerification(ctx context.Context, args []string, stdout io.Writer) error {
	flags := newReviewFlagSet("review retry-final-verification", stdout, "Create the one provider-derived validating successor admitted only for an exact completed failed final-verification tooling incident.")
	cwd := flags.String("cwd", ".", "repository path")
	contract := flags.String("contract", ReviewIntegrationContractV1, "optional negotiated review integration contract")
	predecessor := flags.String("predecessor-lineage", "", "exact escalated predecessor lineage")
	expected := flags.String("expected-predecessor-revision", "", "exact escalated predecessor revision")
	successor := flags.String("successor-lineage", "", "distinct validating successor lineage")
	incidentPath := flags.String("incident", "", "canonical final-verification incident record")
	actor := flags.String("actor", "", "maintainer actor")
	reason := flags.String("reason", "", "maintainer reason")
	authorization := flags.String("maintainer-authorization", "", "exact LF-only final-verification retry authorization")
	repositoryContext := flags.String("repository-context", "", "provider-issued opaque repository context")
	consent := flags.String("consent", "", "provider-issued final-verification retry consent: granted or declined")
	if err := parseReviewFlags(flags, args); err != nil {
		return err
	}
	if reviewHelpRequested(args) {
		return nil
	}
	if flags.NArg() != 0 {
		return reviewPreflightError(fmt.Errorf("unexpected review retry-final-verification argument %q", flags.Arg(0)))
	}
	negotiated, err := reviewIntegrationNegotiation(flags, *contract)
	if err != nil {
		return err
	}
	if strings.TrimSpace(*repositoryContext) != "" {
		if reviewFlagWasProvided(flags, "cwd") {
			return reviewPreflightError(errors.New("opaque final-verification retry cannot combine --repository-context with --cwd"))
		}
		for _, name := range []string{"predecessor-lineage", "expected-predecessor-revision", "successor-lineage", "incident", "actor", "reason", "maintainer-authorization"} {
			if reviewFlagWasProvided(flags, name) {
				return reviewPreflightError(errors.New("opaque final-verification retry derives predecessor, incident, authorization, successor, actor, and reason provider-side"))
			}
		}
		if reviewFlagWasProvided(flags, "contract") && *contract != ReviewIntegrationContractV2 {
			return reviewPreflightError(errors.New("opaque final-verification retry requires the v2 contract when --contract is supplied"))
		}
		return runReviewRetryFinalVerificationConsent(ctx, *repositoryContext, *consent, stdout)
	}
	if strings.TrimSpace(*predecessor) == "" || strings.TrimSpace(*expected) == "" ||
		strings.TrimSpace(*successor) == "" || strings.TrimSpace(*incidentPath) == "" ||
		strings.TrimSpace(*actor) == "" || strings.TrimSpace(*reason) == "" || *authorization == "" {
		// Every input this refusal names is expressible in the published
		// required_inputs enum, so the negotiated caller gets the complete
		// list rather than a misleading subset.
		return reviewPreflightRefusal(
			reviewPreflightMissingInputsReason(
				"predecessor_lineage_id", "expected_predecessor_revision", "successor_lineage_id",
				"incident", "actor", "reason", "maintainer_authorization",
			),
			errors.New("review retry-final-verification requires --predecessor-lineage, --expected-predecessor-revision, --successor-lineage, --incident, --actor, --reason, and --maintainer-authorization"),
		)
	}
	payload, err := readFinalVerificationIncidentInput(ctx, *incidentPath)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return err
		}
		if errors.Is(err, errFinalVerificationIncidentInputTooLarge) {
			return reviewPreflightError(err)
		}
		return reviewPreflightError(errors.New("final-verification incident is unavailable"))
	}
	incident, err := reviewtransaction.ParseFinalVerificationIncident(payload)
	if err != nil {
		return reviewPreflightError(fmt.Errorf("parse final-verification incident: %w", err))
	}
	root, err := resolveReviewMutationRoot(ctx, *cwd)
	if err != nil {
		return fmt.Errorf("resolve review repository root: %w", err)
	}
	request := reviewtransaction.FinalVerificationRetryRequest{
		PredecessorLineageID:        *predecessor,
		ExpectedPredecessorRevision: *expected,
		SuccessorLineageID:          *successor,
		Incident:                    incident,
		Actor:                       *actor,
		Reason:                      *reason,
		MaintainerAuthorization:     *authorization,
	}
	record, err := reviewtransaction.RetryCompactFinalVerification(ctx, root, request)
	if err != nil {
		return err
	}
	result := ReviewFinalVerificationRetryResult{
		Operation:            ReviewIntegrationOperationRetryFinalVerification,
		PredecessorLineageID: request.PredecessorLineageID,
		PredecessorRevision:  request.ExpectedPredecessorRevision,
		LineageID:            record.State.LineageID,
		State:                record.State.State,
		StoreRevision:        record.Revision,
		TargetIdentity:       record.State.CurrentSnapshot.Identity,
		IncidentDigest:       reviewtransaction.FinalVerificationIncidentDigest(incident),
		RecoveryDisposition:  reviewtransaction.RecoveryFinalVerificationRetry,
	}
	if err := result.Validate(); err != nil {
		return err
	}
	return encodeReviewIntegrationOperation(stdout, negotiated, ReviewIntegrationOperationRetryFinalVerification, result, result, *contract)
}

func runReviewRetryFinalVerificationConsent(ctx context.Context, handle, consent string, stdout io.Writer) error {
	if consent != reviewFinalVerificationRetryConsentGranted && consent != reviewFinalVerificationRetryConsentDeclined {
		return reviewPreflightError(errors.New("opaque final-verification retry consent must be granted or declined"))
	}
	root, binding, err := reviewtransaction.ResolveReviewRepositoryContextBinding(ctx, handle)
	if err != nil {
		return reviewRepositoryContextResolutionFailure(err)
	}
	successor := finalVerificationRetryConsentSuccessor(binding)
	if store, storeErr := reviewtransaction.CompactAuthoritativeStore(ctx, root, successor); storeErr == nil {
		if existing, loadErr := store.Load(); loadErr == nil && existing.State.Recovery != nil &&
			existing.State.Recovery.Disposition == reviewtransaction.RecoveryFinalVerificationRetry &&
			existing.State.Recovery.PredecessorLineageID == binding.LineageID && existing.State.Recovery.PredecessorRevision == binding.Revision {
			if consent == reviewFinalVerificationRetryConsentDeclined {
				return reviewPreflightError(errors.New("final-verification retry consent slot was already consumed by grant"))
			}
			incident := existing.State.Recovery.FinalVerificationRetry.Incident
			result := finalVerificationRetryConsentResult(existing, binding, incident)
			return encodeReviewIntegrationOperation(stdout, true, ReviewIntegrationOperationRetryFinalVerification, result, result, ReviewIntegrationContractV2)
		}
	}
	eligibility, eligible, err := reviewtransaction.InspectCompactFinalVerificationRetrySource(ctx, root, binding.LineageID, binding.Revision)
	if err != nil {
		return err
	}
	if !eligible || eligibility.TargetIdentity != binding.TargetIdentity {
		return reviewPreflightError(errors.New("opaque final-verification retry context no longer binds an eligible procedural failure"))
	}
	if consent == reviewFinalVerificationRetryConsentDeclined {
		result := ReviewFinalVerificationRetryConsentResult{Operation: ReviewIntegrationOperationRetryFinalVerification,
			Disposition: reviewFinalVerificationRetryConsentDeclined, PredecessorLineageID: binding.LineageID,
			PredecessorRevision: binding.Revision, TargetIdentity: binding.TargetIdentity}
		if err := result.Validate(); err != nil {
			return err
		}
		return encodeReviewIntegrationOperation(stdout, true, ReviewIntegrationOperationRetryFinalVerification, result, result, ReviewIntegrationContractV2)
	}
	incident := reviewtransaction.FinalVerificationIncident{Schema: eligibility.IncidentSchema, Class: eligibility.IncidentClass,
		LineageID: binding.LineageID, TerminalRevision: binding.Revision, ValidatingRevision: eligibility.ValidatingRevision,
		TargetIdentity: eligibility.TargetIdentity, FailedEvidenceHash: eligibility.FailedEvidenceHash, FinalizeRequestDigest: eligibility.FinalizeRequestDigest}
	request := reviewtransaction.FinalVerificationRetryRequest{PredecessorLineageID: binding.LineageID, ExpectedPredecessorRevision: binding.Revision,
		SuccessorLineageID: successor, Incident: incident, Actor: "gentle-ai", Reason: "retry final verification after provider tooling failure"}
	request.MaintainerAuthorization, err = reviewtransaction.FinalVerificationRetryAuthorization(request)
	if err != nil {
		return err
	}
	record, err := reviewtransaction.RetryCompactFinalVerification(ctx, root, request)
	if err != nil {
		return err
	}
	result := finalVerificationRetryConsentResult(record, binding, incident)
	if err := result.Validate(); err != nil {
		return err
	}
	return encodeReviewIntegrationOperation(stdout, true, ReviewIntegrationOperationRetryFinalVerification, result, result, ReviewIntegrationContractV2)
}

func finalVerificationRetryConsentResult(record reviewtransaction.CompactRecord, binding reviewtransaction.ReviewRepositoryContextBinding, incident reviewtransaction.FinalVerificationIncident) ReviewFinalVerificationRetryResult {
	return ReviewFinalVerificationRetryResult{Operation: ReviewIntegrationOperationRetryFinalVerification,
		PredecessorLineageID: binding.LineageID, PredecessorRevision: binding.Revision,
		LineageID: record.State.LineageID, State: record.State.State, StoreRevision: record.Revision,
		TargetIdentity: record.State.CurrentSnapshot.Identity, IncidentDigest: reviewtransaction.FinalVerificationIncidentDigest(incident),
		RecoveryDisposition: reviewtransaction.RecoveryFinalVerificationRetry}
}

func finalVerificationRetryConsentSuccessor(binding reviewtransaction.ReviewRepositoryContextBinding) string {
	sum := sha256.Sum256([]byte(binding.LineageID + "\n" + binding.Revision + "\n" + binding.TargetIdentity))
	return fmt.Sprintf("retry-%x", sum[:8])
}
