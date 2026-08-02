package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"reflect"
	"strings"

	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
)

const ReviewIntegrationRepairSchema = "gentle-ai.review-integration.repair/v1"
const ReviewIntegrationRepairSchemaID = "https://gentle-ai.dev/contracts/review-integration/v1/schemas/repair.schema.json"
const ReviewIntegrationRepairSchemaV2 = "gentle-ai.review-integration.repair/v2"
const ReviewIntegrationRepairSchemaIDV2 = "https://gentle-ai.dev/contracts/review-integration/v2/schemas/repair.schema.json"

type ReviewRepairMode string

const (
	ReviewRepairModePreflight ReviewRepairMode = "preflight"
	ReviewRepairModeExecute   ReviewRepairMode = "execute"
)

type ReviewRepairProviderInputs struct {
	Class               reviewtransaction.AuthorityRepairClass       `json:"class"`
	LineageID           string                                       `json:"lineage_id"`
	ExpectedRevision    string                                       `json:"expected_revision"`
	Cause               reviewtransaction.AuthorityRepairCause       `json:"cause"`
	Disposition         reviewtransaction.AuthorityRepairDisposition `json:"disposition"`
	RepositoryBinding   string                                       `json:"repository_binding"`
	AuthorizationSchema string                                       `json:"authorization_schema"`
}

type ReviewRepairResult struct {
	Schema         string                                                `json:"schema"`
	Contract       string                                                `json:"contract"`
	Operation      string                                                `json:"operation"`
	Mode           ReviewRepairMode                                      `json:"mode"`
	Assessment     reviewtransaction.AuthorityRepairAssessment           `json:"assessment"`
	ProviderInputs *ReviewRepairProviderInputs                           `json:"provider_inputs,omitempty"`
	RequiredInputs []string                                              `json:"required_inputs"`
	Execution      *reviewtransaction.ClassifiedAuthorityRepairExecution `json:"execution,omitempty"`
}

func (result ReviewRepairResult) Validate() error {
	legacyContract := result.Schema == ReviewIntegrationRepairSchema && result.Contract == ReviewIntegrationContractV1
	nativeGitContract := result.Schema == ReviewIntegrationRepairSchemaV2 && result.Contract == ReviewIntegrationContractV2
	if (!legacyContract && !nativeGitContract) ||
		result.Operation != "review.repair" {
		return errors.New("review repair result identity is invalid")
	}
	if err := result.Assessment.Validate(); err != nil {
		return fmt.Errorf("review repair assessment: %w", err)
	}
	switch result.Mode {
	case ReviewRepairModePreflight:
		if result.Execution != nil {
			return errors.New("review repair preflight contains execution output")
		}
		if result.Assessment.Status != reviewtransaction.AuthorityRepairEligible {
			if result.ProviderInputs != nil || len(result.RequiredInputs) != 0 {
				return errors.New("stopped review repair preflight contains executable inputs")
			}
			return nil
		}
		candidate := result.Assessment.Candidate
		inputs := result.ProviderInputs
		if candidate == nil || inputs == nil ||
			inputs.Class != result.Assessment.Class || inputs.LineageID != candidate.LineageID ||
			inputs.ExpectedRevision != candidate.Revision || inputs.Cause != result.Assessment.Cause ||
			inputs.Disposition != result.Assessment.Disposition || inputs.RepositoryBinding != result.Assessment.RepositoryBinding ||
			inputs.AuthorizationSchema != reviewtransaction.AuthorityRepairAuthorizationSchema ||
			!reflect.DeepEqual(result.RequiredInputs, []string{"actor", "reason", "maintainer_authorization"}) {
			return errors.New("eligible review repair preflight inputs are incomplete")
		}
	case ReviewRepairModeExecute:
		if result.ProviderInputs != nil || len(result.RequiredInputs) != 0 || result.Execution == nil {
			return errors.New("review repair execution shape is invalid")
		}
		execution := result.Execution
		if execution.Status != reviewtransaction.CompactReclaimCommitted ||
			execution.Class != reviewtransaction.AuthorityRepairClassLegacyV1HistoricalAlias ||
			execution.Cause != reviewtransaction.AuthorityRepairCauseUnsupportedHistoricalV1OperationAlias ||
			execution.Disposition != reviewtransaction.AuthorityRepairDispositionQuarantineHistoricalAlias ||
			strings.TrimSpace(execution.LineageID) == "" || !validReviewCapabilitySHA256(execution.Revision) ||
			!validReviewCapabilitySHA256(execution.ChainIdentity) ||
			!validReviewCapabilitySHA256(execution.AssessmentDigest) ||
			!validReviewCapabilitySHA256(execution.RequestDigest) ||
			!validReviewCapabilitySHA256(execution.RecordIdentity) {
			return errors.New("review repair execution output is invalid")
		}
	default:
		return errors.New("review repair mode is invalid")
	}
	return nil
}

type reviewRepairOperationError struct {
	message string
	cause   error
}

func (err *reviewRepairOperationError) Error() string { return err.message }
func (err *reviewRepairOperationError) Unwrap() error { return err.cause }

func RunReviewRepair(args []string, stdout io.Writer) error {
	return runReviewRepair(context.Background(), args, stdout)
}

func runReviewRepair(ctx context.Context, args []string, stdout io.Writer) error {
	flags := newReviewFlagSet("review repair", stdout, "Assess the complete review authority inventory and execute only one provider-owned classified repair. Run --preflight first. It emits bounded path-free provider inputs, never an authorization template. A maintainer supplies actor, reason, and an exact gentle-ai.review-repair-authorization/v1 binding. The compatibility-only repair-legacy-alias command remains available for established automation.")
	cwd := flags.String("cwd", ".", "repository path")
	contract := flags.String("contract", ReviewIntegrationContractV1, "review integration contract")
	preflight := flags.Bool("preflight", false, "perform deterministic read-only classification without authority mutation")
	class := flags.String("class", "", "provider-owned classified repair class")
	lineage := flags.String("lineage", "", "optional preflight selector or exact provider-owned lineage")
	expectedRevision := flags.String("expected-revision", "", "exact provider-owned authority revision")
	cause := flags.String("cause", "", "provider-owned classified repair cause")
	disposition := flags.String("disposition", "", "provider-owned classified repair disposition")
	repositoryBinding := flags.String("repository-binding", "", "opaque provider-owned repository binding")
	actor := flags.String("actor", "", "maintainer actor; never emitted in public output")
	reason := flags.String("reason", "", "maintainer reason; never emitted in public output")
	authorization := flags.String("maintainer-authorization", "", "exact nine-line LF-only maintainer authorization; never emitted in public output")
	if err := parseReviewFlags(flags, args); err != nil {
		return err
	}
	if reviewHelpRequested(args) {
		return nil
	}
	if flags.NArg() != 0 {
		return reviewPreflightError(errors.New("review repair received an unexpected positional argument"))
	}
	if err := validateReviewIntegrationContract(*contract); err != nil {
		return reviewPreflightError(err)
	}
	if *lineage != "" && !validReviewIntegrationLineage(*lineage) {
		return reviewPreflightError(errors.New("review repair lineage is invalid"))
	}
	root, err := (reviewtransaction.SnapshotBuilder{Repo: *cwd}).ResolveRepositoryRoot(ctx)
	if err != nil {
		return &reviewRepairOperationError{message: "review repair could not resolve repository authority", cause: err}
	}
	// --preflight only assesses and reports; execution is what repairs durable
	// authority, so only execution is refused while reviews are off.
	if !*preflight {
		if err := authorizeReviewAuthorityMutation(ctx, root); err != nil {
			return err
		}
	}
	assessment, err := reviewtransaction.AssessAuthorityRepairAtRepositoryRoot(ctx, root)
	if err != nil {
		return &reviewRepairOperationError{message: "review repair assessment failed safely", cause: err}
	}
	if *preflight {
		if repairExecutionInputPresent(*class, *expectedRevision, *cause, *disposition, *repositoryBinding, *actor, *reason, *authorization) {
			return reviewPreflightError(errors.New("review repair --preflight does not accept execution inputs"))
		}
		if *lineage != "" && assessment.Status == reviewtransaction.AuthorityRepairEligible &&
			(assessment.Candidate == nil || assessment.Candidate.LineageID != *lineage) {
			return reviewPreflightError(errors.New("review repair selector does not match the unique classified candidate"))
		}
		result := newReviewRepairPreflightResult(assessment, *contract)
		if err := result.Validate(); err != nil {
			return fmt.Errorf("validate review repair preflight: %w", err)
		}
		return encodeReviewJSON(stdout, result)
	}
	for _, required := range []string{*class, *lineage, *expectedRevision, *cause, *disposition, *repositoryBinding, *actor, *reason, *authorization} {
		if strings.TrimSpace(required) == "" {
			return reviewPreflightError(errors.New("review repair execution requires provider inputs, actor, reason, and maintainer authorization"))
		}
	}
	request := reviewtransaction.ClassifiedAuthorityRepairRequest{
		Class: reviewtransaction.AuthorityRepairClass(*class), LineageID: *lineage, ExpectedRevision: *expectedRevision,
		Cause: reviewtransaction.AuthorityRepairCause(*cause), Disposition: reviewtransaction.AuthorityRepairDisposition(*disposition),
		RepositoryBinding: *repositoryBinding, Actor: *actor, Reason: *reason, MaintainerAuthorization: *authorization,
	}
	if assessment.Status == reviewtransaction.AuthorityRepairEligible {
		if err := reviewtransaction.ValidateClassifiedAuthorityRepairRequest(request, assessment); err != nil {
			return reviewPreflightError(&reviewRepairOperationError{message: "review repair authorization does not match the provider assessment", cause: err})
		}
	}
	execution, err := reviewtransaction.RepairClassifiedAuthority(ctx, root, request)
	if err != nil {
		return &reviewRepairOperationError{message: "review repair did not complete", cause: err}
	}
	result := ReviewRepairResult{
		Schema: ReviewIntegrationRepairSchema, Contract: ReviewIntegrationContractV1, Operation: "review.repair",
		Mode: ReviewRepairModeExecute, Assessment: assessment, RequiredInputs: []string{}, Execution: &execution,
	}
	if *contract == ReviewIntegrationContractV2 {
		result.Schema, result.Contract = ReviewIntegrationRepairSchemaV2, ReviewIntegrationContractV2
	}
	if err := result.Validate(); err != nil {
		return fmt.Errorf("validate review repair execution: %w", err)
	}
	return encodeReviewJSON(stdout, result)
}

func repairExecutionInputPresent(values ...string) bool {
	for _, value := range values {
		if value != "" {
			return true
		}
	}
	return false
}

func newReviewRepairPreflightResult(assessment reviewtransaction.AuthorityRepairAssessment, contracts ...string) ReviewRepairResult {
	result := ReviewRepairResult{
		Schema: ReviewIntegrationRepairSchema, Contract: ReviewIntegrationContractV1, Operation: "review.repair",
		Mode: ReviewRepairModePreflight, Assessment: assessment, RequiredInputs: []string{},
	}
	if len(contracts) > 0 && contracts[0] == ReviewIntegrationContractV2 {
		result.Schema, result.Contract = ReviewIntegrationRepairSchemaV2, ReviewIntegrationContractV2
	}
	if assessment.Status != reviewtransaction.AuthorityRepairEligible || assessment.Candidate == nil {
		return result
	}
	result.ProviderInputs = &ReviewRepairProviderInputs{
		Class: assessment.Class, LineageID: assessment.Candidate.LineageID, ExpectedRevision: assessment.Candidate.Revision,
		Cause: assessment.Cause, Disposition: assessment.Disposition, RepositoryBinding: assessment.RepositoryBinding,
		AuthorizationSchema: assessment.AuthorizationSchema,
	}
	result.RequiredInputs = []string{"actor", "reason", "maintainer_authorization"}
	return result
}
