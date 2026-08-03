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

// ReviewRepairDispositionProviderInputs is the plan-bound preflight output
// for Slice S3's leaf authority disposition wiring (tasks.md 3.1): only the
// plan digest and the authority inventory revision it is bound to — nothing
// a maintainer could not already derive read-only through
// reviewtransaction.DeriveAuthorityDispositionPlanAtRepo, and nothing that
// changes if the maintainer supplies no authorization.
type ReviewRepairDispositionProviderInputs struct {
	PlanDigest                 string `json:"plan_digest"`
	AuthorityInventoryRevision string `json:"authority_inventory_revision"`
}

// ReviewRepairDispositionExecution is the safe, path-free projection of a
// committed leaf authority disposition quarantine (mirrors
// reviewtransaction.AuthorityDispositionProof; never carries SourcePath or
// QuarantinePath).
type ReviewRepairDispositionExecution struct {
	Schema                     string `json:"schema"`
	Status                     string `json:"status"`
	LineageID                  string `json:"lineage_id"`
	PlanDigest                 string `json:"plan_digest"`
	AuthorityInventoryRevision string `json:"authority_inventory_revision"`
	AnomalyClass               string `json:"anomaly_class"`
	AuthorizationSHA256        string `json:"authorization_sha256"`
}

type ReviewRepairResult struct {
	Schema                    string                                                `json:"schema"`
	Contract                  string                                                `json:"contract"`
	Operation                 string                                                `json:"operation"`
	Mode                      ReviewRepairMode                                      `json:"mode"`
	Assessment                reviewtransaction.AuthorityRepairAssessment           `json:"assessment"`
	ProviderInputs            *ReviewRepairProviderInputs                           `json:"provider_inputs,omitempty"`
	RequiredInputs            []string                                              `json:"required_inputs"`
	Execution                 *reviewtransaction.ClassifiedAuthorityRepairExecution `json:"execution,omitempty"`
	DispositionProviderInputs *ReviewRepairDispositionProviderInputs                `json:"disposition_provider_inputs,omitempty"`
	DispositionExecution      *ReviewRepairDispositionExecution                     `json:"disposition_execution,omitempty"`
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
		if result.Execution != nil || result.DispositionExecution != nil {
			return errors.New("review repair preflight contains execution output")
		}
		if result.DispositionProviderInputs != nil {
			inputs := result.DispositionProviderInputs
			if !validReviewCapabilitySHA256(inputs.PlanDigest) || !validReviewCapabilitySHA256(inputs.AuthorityInventoryRevision) {
				// refusal:by-design human-authority: this result is built by runReviewRepair itself from a freshly-derived plan digest and inventory revision; reaching this means a product defect a maintainer must fix, not a value any operator command supplies
				return errors.New("review repair preflight disposition provider inputs are incomplete")
			}
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
		if result.ProviderInputs != nil || len(result.RequiredInputs) != 0 || result.DispositionProviderInputs != nil {
			return errors.New("review repair execution shape is invalid")
		}
		switch {
		case result.DispositionExecution != nil:
			if result.Execution != nil {
				return errors.New("review repair execution shape is invalid")
			}
			exec := result.DispositionExecution
			if exec.Schema != reviewtransaction.AuthorityDispositionProofSchema || exec.Status != string(reviewtransaction.CompactReclaimCommitted) ||
				strings.TrimSpace(exec.LineageID) == "" || !validReviewCapabilitySHA256(exec.PlanDigest) ||
				!validReviewCapabilitySHA256(exec.AuthorityInventoryRevision) || strings.TrimSpace(exec.AnomalyClass) == "" ||
				!validReviewCapabilitySHA256(exec.AuthorizationSHA256) {
				// refusal:by-design human-authority: this result is built by newReviewRepairDispositionExecutionResult from a committed CompactReclaimRecord RepairAuthorityDisposition itself returned; reaching this means a product defect a maintainer must fix
				return errors.New("review repair disposition execution output is invalid")
			}
		case result.Execution != nil:
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
			return errors.New("review repair execution shape is invalid")
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
	flags := newReviewFlagSet("review repair", stdout, "Assess the complete review authority inventory and execute only one provider-owned classified repair. Run --preflight first. It emits bounded path-free provider inputs, never an authorization template. A maintainer supplies actor, reason, and an exact gentle-ai.review-repair-authorization/v1 binding. The compatibility-only repair-legacy-alias command remains available for established automation. --preflight also surfaces a plan-bound leaf authority disposition digest and inventory revision (Wave 2) for an eligible content-mismatched leaf; execute it with --plan-digest --inventory-revision --actor --reason --authorization.")
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
	planDigest := flags.String("plan-digest", "", "exact provider-owned leaf authority disposition plan digest")
	inventoryRevision := flags.String("inventory-revision", "", "exact provider-owned authority inventory revision the plan is bound to")
	dispositionAuthorization := flags.String("authorization", "", "exact maintainer authorization binding for an eligible leaf authority disposition plan; never emitted in public output")
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
		if repairExecutionInputPresent(*class, *expectedRevision, *cause, *disposition, *repositoryBinding, *actor, *reason, *authorization) ||
			repairExecutionInputPresent(*planDigest, *inventoryRevision, *dispositionAuthorization) {
			return reviewPreflightError(errors.New("review repair --preflight does not accept execution inputs"))
		}
		if *lineage != "" && assessment.Status == reviewtransaction.AuthorityRepairEligible &&
			(assessment.Candidate == nil || assessment.Candidate.LineageID != *lineage) {
			return reviewPreflightError(errors.New("review repair selector does not match the unique classified candidate"))
		}
		result := newReviewRepairPreflightResult(assessment, *contract)
		// A read-only preview: actor/reason stay empty, so nothing maintainer-
		// specific is ever derived or published here. Only an eligible leaf
		// (derivation succeeds AND admits as cardinality-one) surfaces a plan —
		// never a partial or generic fallback (rdd-authority-disposition-plan /
		// "Closed Anomaly Classification Required for Derivation").
		if plan, planErr := reviewtransaction.DeriveAuthorityDispositionPlanAtRepo(ctx, root, "", ""); planErr == nil {
			if reviewtransaction.AdmitAuthorityDispositionLeaf(plan) == nil {
				result.DispositionProviderInputs = &ReviewRepairDispositionProviderInputs{
					PlanDigest: plan.PlanDigest, AuthorityInventoryRevision: plan.AuthorityInventoryRevision,
				}
			}
		}
		if err := result.Validate(); err != nil {
			return fmt.Errorf("validate review repair preflight: %w", err)
		}
		return encodeReviewJSON(stdout, result)
	}
	if repairExecutionInputPresent(*planDigest, *inventoryRevision, *dispositionAuthorization) {
		if repairExecutionInputPresent(*class, *lineage, *expectedRevision, *cause, *disposition, *repositoryBinding, *authorization) {
			return reviewPreflightError(errors.New("review repair execution accepts either classified repair inputs or leaf authority disposition inputs, not both; run `gentle-ai review repair` again with only one input set"))
		}
		for _, required := range []string{*planDigest, *inventoryRevision, *actor, *reason, *dispositionAuthorization} {
			if strings.TrimSpace(required) == "" {
				return reviewPreflightError(errors.New("review repair leaf authority disposition execution requires --plan-digest --inventory-revision --actor --reason --authorization; run `gentle-ai review repair --preflight` first to obtain --plan-digest and --inventory-revision"))
			}
		}
		plan, err := reviewtransaction.DeriveAuthorityDispositionPlanAtRepo(ctx, root, *actor, *reason)
		if err != nil {
			return &reviewRepairOperationError{message: "review repair leaf authority disposition derivation failed safely", cause: err}
		}
		if err := reviewtransaction.AdmitAuthorityDispositionLeaf(plan); err != nil {
			return &reviewRepairOperationError{message: "review repair leaf authority disposition execution refused", cause: err}
		}
		if plan.PlanDigest != *planDigest || plan.AuthorityInventoryRevision != *inventoryRevision {
			return reviewPreflightError(errors.New("review repair leaf authority disposition inputs do not match the current provider-derived plan; run `gentle-ai review repair --preflight` again for the current values"))
		}
		record, err := reviewtransaction.RepairAuthorityDisposition(ctx, root, *actor, *reason, *dispositionAuthorization)
		if err != nil {
			return &reviewRepairOperationError{message: "review repair leaf authority disposition execution did not complete", cause: err}
		}
		result, err := newReviewRepairDispositionExecutionResult(assessment, record, *contract)
		if err != nil {
			return &reviewRepairOperationError{message: "review repair leaf authority disposition execution produced an invalid audit record", cause: err}
		}
		if err := result.Validate(); err != nil {
			return fmt.Errorf("validate review repair leaf authority disposition execution: %w", err)
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

// newReviewRepairDispositionExecutionResult projects a committed leaf
// authority disposition CompactReclaimRecord into the safe, path-free public
// result shape. assessment is the already-computed legacy classified-repair
// assessment (unrelated to this execution but still a required result field).
func newReviewRepairDispositionExecutionResult(assessment reviewtransaction.AuthorityRepairAssessment, record reviewtransaction.CompactReclaimRecord, contracts ...string) (ReviewRepairResult, error) {
	proof := record.AuthorityDisposition
	if record.Status != reviewtransaction.CompactReclaimCommitted || proof == nil || strings.TrimSpace(record.LineageID) == "" {
		// refusal:by-design human-authority: record is what reviewtransaction.RepairAuthorityDisposition itself just returned from a committed quarantine; reaching this means a product defect a maintainer must fix, not a value any operator command supplies
		return ReviewRepairResult{}, errors.New("review repair leaf authority disposition audit record is invalid")
	}
	result := ReviewRepairResult{
		Schema: ReviewIntegrationRepairSchema, Contract: ReviewIntegrationContractV1, Operation: "review.repair",
		Mode: ReviewRepairModeExecute, Assessment: assessment, RequiredInputs: []string{},
		DispositionExecution: &ReviewRepairDispositionExecution{
			Schema: proof.Schema, Status: record.Status, LineageID: record.LineageID,
			PlanDigest: proof.PlanDigest, AuthorityInventoryRevision: proof.AuthorityInventoryRevision,
			AnomalyClass: proof.AnomalyClass, AuthorizationSHA256: proof.AuthorizationSHA256,
		},
	}
	if len(contracts) > 0 && contracts[0] == ReviewIntegrationContractV2 {
		result.Schema, result.Contract = ReviewIntegrationRepairSchemaV2, ReviewIntegrationContractV2
	}
	return result, nil
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
