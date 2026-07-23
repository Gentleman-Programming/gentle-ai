package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"reflect"
	"strconv"
	"strings"

	"github.com/gentleman-programming/gentle-ai/internal/reviewtransaction"
	"github.com/gentleman-programming/gentle-ai/internal/sddstatus"
)

const ReviewIntegrationOperationSchema = "gentle-ai.review-integration.operation/v1"
const ReviewIntegrationOperationSchemaID = "https://gentle-ai.dev/contracts/review-integration/v1/schemas/operation.schema.json"
const ReviewIntegrationFailureSchema = "gentle-ai.review-integration.failure/v1"
const ReviewIntegrationFailureSchemaID = "https://gentle-ai.dev/contracts/review-integration/v1/schemas/failure.schema.json"
const ReviewIntegrationFailureSchemaV2 = "gentle-ai.review-integration.failure/v2"
const ReviewIntegrationFailureSchemaIDV2 = "https://gentle-ai.dev/contracts/review-integration/v1/schemas/failure-v2.schema.json"

const (
	ReviewIntegrationOperationFinalize               = "review.finalize"
	ReviewIntegrationOperationValidate               = "review.validate"
	ReviewIntegrationOperationBindSDD                = "review.bind_sdd"
	ReviewIntegrationOperationRetryFinalVerification = "review.retry_final_verification"
)

type reviewIntegrationOperationMetadata struct {
	Command          string
	Operation        string
	Label            string
	ValueFlags       []string
	BoolFlags        []string
	IntFlags         []string
	MutatesAuthority bool
	JoinOnTimeout    bool
	TimeoutRetryable bool
	ReadOnlyFlag     string
}

// reviewIntegrationOperationRegistry is the single policy source for
// negotiated routing, safe flag extraction, aggregate-timeout mutation truth,
// capability publication, and operation-specific diagnostics.
var reviewIntegrationOperationRegistry = []reviewIntegrationOperationMetadata{
	{Command: "bind-sdd", Operation: ReviewIntegrationOperationBindSDD, Label: "Review BIND-SDD", ValueFlags: []string{"cwd", "change", "lineage", "expected-binding-revision"}, MutatesAuthority: true, JoinOnTimeout: true, TimeoutRetryable: true},
	{Command: "capabilities", Operation: "review.capabilities", Label: "Review CAPABILITIES"},
	{Command: "finalize", Operation: ReviewIntegrationOperationFinalize, Label: "Review FINALIZE", ValueFlags: []string{"cwd", "lineage", "validation", "refuter", "evidence", "trace", "result"}, BoolFlags: []string{"failed"}, IntFlags: []string{"correction-lines"}, MutatesAuthority: true},
	{Command: "repair", Operation: "review.repair", Label: "Review REPAIR", ValueFlags: []string{"cwd", "class", "lineage", "expected-revision", "cause", "disposition", "repository-binding", "actor", "reason", "maintainer-authorization"}, BoolFlags: []string{"preflight"}, MutatesAuthority: true, JoinOnTimeout: true, ReadOnlyFlag: "preflight"},
	{Command: "retry-final-verification", Operation: ReviewIntegrationOperationRetryFinalVerification, Label: "Review RETRY-FINAL-VERIFICATION", ValueFlags: []string{"cwd", "predecessor-lineage", "expected-predecessor-revision", "successor-lineage", "incident", "actor", "reason", "maintainer-authorization"}, MutatesAuthority: true, JoinOnTimeout: true},
	{Command: "start", Operation: "review.start", Label: "Review START", ValueFlags: []string{"cwd", "target", "lineage", "policy", "focus", "base-ref", "projection", "trace"}, BoolFlags: []string{"committed-only", "workspace-overlay"}, MutatesAuthority: true},
	{Command: "status", Operation: "review.status", Label: "Review STATUS", ValueFlags: []string{"cwd", "lineage", "projection", "base-ref", "base-tree", "gate", "recovery-successor-lineage", "recovery-reason", "recovery-actor", "recovery-authorization", "repair-actor", "repair-reason", "repair-authorization"}, BoolFlags: []string{"workspace-overlay", "action-eligibility", "next-transition"}},
	{Command: "validate", Operation: ReviewIntegrationOperationValidate, Label: "Review VALIDATE", ValueFlags: []string{"cwd", "lineage", "gate", "base-ref", "pre-pr-ci-attestation", "policy", "release-configuration", "release-generated", "release-provenance", "release-publication-boundary", "release-evidence-freshness"}},
}

func reviewIntegrationOperationByCommand(command string) (reviewIntegrationOperationMetadata, bool) {
	for _, metadata := range reviewIntegrationOperationRegistry {
		if metadata.Command == command {
			return metadata, true
		}
	}
	return reviewIntegrationOperationMetadata{}, false
}

func reviewIntegrationOperationByName(operation string) (reviewIntegrationOperationMetadata, bool) {
	for _, metadata := range reviewIntegrationOperationRegistry {
		if metadata.Operation == operation {
			return metadata, true
		}
	}
	return reviewIntegrationOperationMetadata{}, false
}

func reviewIntegrationOperationNames() []string {
	operations := make([]string, 0, len(reviewIntegrationOperationRegistry))
	for _, metadata := range reviewIntegrationOperationRegistry {
		operations = append(operations, metadata.Operation)
	}
	return operations
}

type ReviewMutationOutcome string

const (
	ReviewMutationNotStarted ReviewMutationOutcome = "not_started"
	ReviewMutationUnknown    ReviewMutationOutcome = "unknown"
	ReviewMutationCommitted  ReviewMutationOutcome = "committed"
)

type ReviewIntegrationFailure struct {
	Schema                 string                           `json:"schema"`
	Contract               string                           `json:"contract"`
	Operation              string                           `json:"operation"`
	Phase                  string                           `json:"phase"`
	Code                   string                           `json:"code"`
	Message                string                           `json:"message"`
	MutationOutcome        ReviewMutationOutcome            `json:"mutation_outcome"`
	AuthorityApplicability string                           `json:"authority_applicability"`
	RetrySafe              bool                             `json:"retry_safe"`
	Replayability          reviewtransaction.Replayability  `json:"replayability"`
	LineageID              string                           `json:"lineage_id,omitempty"`
	RequestDigest          string                           `json:"request_digest,omitempty"`
	ProgressIdentity       string                           `json:"progress_identity,omitempty"`
	RequiredInputs         []string                         `json:"required_inputs"`
	Candidates             []string                         `json:"candidates,omitempty"`
	NextAction             string                           `json:"next_action"`
	CauseCategory          string                           `json:"cause_category,omitempty"`
	Context                *ReviewIntegrationFailureContext `json:"context,omitempty"`
}

type ReviewIntegrationFailureContext struct {
	ScopeChange     *ReviewIntegrationScopeChange             `json:"scope_change,omitempty"`
	BindingRevision *ReviewIntegrationBindingRevisionConflict `json:"binding_revision,omitempty"`
}

type ReviewIntegrationBindingRevisionConflict struct {
	Expected string `json:"expected"`
	Current  string `json:"current"`
}

type ReviewIntegrationScopeChange struct {
	Expected               ReviewIntegrationScopeTarget `json:"expected"`
	Actual                 ReviewIntegrationScopeTarget `json:"actual"`
	DifferingPathCount     int                          `json:"differing_path_count"`
	DifferingPathsDigest   string                       `json:"differing_paths_digest"`
	PredecessorLineageID   string                       `json:"predecessor_lineage_id"`
	PredecessorRevision    string                       `json:"predecessor_revision"`
	RecoveryOperation      string                       `json:"recovery_operation"`
	RecoveryRequiredInputs []string                     `json:"recovery_required_inputs"`
}

type ReviewIntegrationScopeTarget struct {
	CandidateTree string `json:"candidate_tree"`
	PathsDigest   string `json:"paths_digest"`
}

type ReviewIntegrationFailureError struct {
	Failure ReviewIntegrationFailure
	cause   error
}

func (err *ReviewIntegrationFailureError) Error() string {
	return fmt.Sprintf("%s [%s]", err.Failure.Message, err.Failure.Code)
}

func (err *ReviewIntegrationFailureError) Unwrap() error { return err.cause }

func newReviewIntegrationFailureError(failure ReviewIntegrationFailure, cause error) *ReviewIntegrationFailureError {
	return &ReviewIntegrationFailureError{Failure: failure, cause: cause}
}

type reviewIntegrationPreflightError struct{ cause error }

func (err *reviewIntegrationPreflightError) Error() string { return err.cause.Error() }
func (err *reviewIntegrationPreflightError) Unwrap() error { return err.cause }

func reviewPreflightError(err error) error {
	if err == nil {
		return nil
	}
	return &reviewIntegrationPreflightError{cause: err}
}

func reviewIntegrationFailureRoute(args []string) (string, bool, *ReviewIntegrationFailure) {
	if len(args) == 0 {
		return "", false, nil
	}
	metadata, known := reviewIntegrationOperationByCommand(args[0])
	if !known {
		return "", false, nil
	}
	operation := metadata.Operation
	provided, contract, missing := reviewIntegrationContractArgument(args[1:])
	if args[0] != "capabilities" && !provided {
		return operation, false, nil
	}
	if !provided {
		contract = ReviewIntegrationContractV1
	}
	if missing {
		failure := newReviewIntegrationPreflightFailure(operation, "invalid_request", "The negotiated review request is invalid.")
		failure.LineageID = safeReviewIntegrationLineage(operation, args[1:])
		return operation, true, &failure
	}
	if contract == "" {
		failure := newReviewIntegrationPreflightFailure(operation, "empty_contract", "The review integration contract cannot be empty.")
		failure.LineageID = safeReviewIntegrationLineage(operation, args[1:])
		return operation, true, &failure
	}
	if contract != ReviewIntegrationContractV1 {
		failure := newReviewIntegrationPreflightFailure(operation, "unsupported_contract", "The requested review integration contract is not supported.")
		failure.LineageID = safeReviewIntegrationLineage(operation, args[1:])
		return operation, true, &failure
	}
	return operation, true, nil
}

func reviewIntegrationContractArgument(args []string) (provided bool, value string, missing bool) {
	for index := 0; index < len(args); index++ {
		arg := args[index]
		if arg == "--" {
			break
		}
		if strings.HasPrefix(arg, "--contract=") {
			provided, value, missing = true, strings.TrimPrefix(arg, "--contract="), false
			continue
		}
		if arg != "--contract" {
			continue
		}
		provided = true
		if index+1 >= len(args) {
			return true, "", true
		}
		value, missing = args[index+1], false
		index++
	}
	return provided, value, missing
}

func reviewIntegrationProtocol(args []string) (int, []string, error) {
	minor := 4
	filtered := make([]string, 0, len(args))
	for index := 0; index < len(args); index++ {
		argument := args[index]
		if argument == "--" {
			filtered = append(filtered, args[index:]...)
			break
		}
		if argument != "--protocol-minor" && !strings.HasPrefix(argument, "--protocol-minor=") {
			filtered = append(filtered, argument)
			continue
		}
		value := strings.TrimPrefix(argument, "--protocol-minor=")
		if argument == "--protocol-minor" {
			index++
			if index == len(args) {
				return minor, filtered, errors.New("protocol minor is missing")
			}
			value = args[index]
		}
		parsed, err := strconv.Atoi(value)
		if err != nil || parsed < 4 || parsed > 5 {
			return minor, filtered, fmt.Errorf("unsupported protocol minor %q", value)
		}
		minor = parsed
	}
	return minor, filtered, nil
}

func reviewIntegrationFailureForProtocol(failure ReviewIntegrationFailure, minor int) ReviewIntegrationFailure {
	if minor >= 5 {
		failure.Schema = ReviewIntegrationFailureSchemaV2
		if failure.Code == "receipt_ambiguous" {
			failure.NextAction = ReviewIntegrationOperationValidate
		}
		return failure
	}
	failure.Candidates = nil
	if failure.Code == "receipt_ambiguous" {
		failure.RequiredInputs = []string{}
		failure.NextAction = "stop"
	}
	return failure
}

func newReviewIntegrationPreflightFailure(operation, code, message string) ReviewIntegrationFailure {
	return ReviewIntegrationFailure{
		Schema: ReviewIntegrationFailureSchema, Contract: ReviewIntegrationContractV1, Operation: operation,
		Phase: "preflight", Code: code, Message: message, MutationOutcome: ReviewMutationNotStarted,
		AuthorityApplicability: "not_evaluated", RetrySafe: true,
		Replayability: reviewtransaction.ReplayabilityNotReplayable, RequiredInputs: []string{}, NextAction: "correct_request",
	}
}

func newReviewIntegrationFailure(operation string, args []string, runErr error) ReviewIntegrationFailure {
	failure := ReviewIntegrationFailure{
		Schema: ReviewIntegrationFailureSchema, Contract: ReviewIntegrationContractV1, Operation: operation,
		Phase: "native_running", Code: "operation_outcome_unknown",
		Message:         "The negotiated review operation failed without authoritative mutation evidence.",
		MutationOutcome: ReviewMutationUnknown, AuthorityApplicability: "not_evaluated", RetrySafe: false,
		Replayability: reviewtransaction.ReplayabilityStatusRequired, RequiredInputs: []string{}, NextAction: "review.status",
	}
	failure.LineageID = safeReviewIntegrationLineage(operation, args)
	var retryDenied *reviewtransaction.FinalVerificationRetryDeniedError
	if errors.As(runErr, &retryDenied) {
		failure.Phase = "pre_native"
		failure.Code = "final_verification_retry_denied"
		failure.Message = "The final-verification retry did not satisfy the exact provider-owned admission boundary."
		failure.MutationOutcome = ReviewMutationNotStarted
		failure.AuthorityApplicability = "current_target"
		failure.RetrySafe = false
		failure.Replayability = reviewtransaction.ReplayabilityManualActionRequired
		failure.RequiredInputs = []string{}
		failure.NextAction = "stop"
		return failure
	}
	var bindingConflict *sddstatus.BindingRevisionConflictError
	if errors.As(runErr, &bindingConflict) {
		failure.Phase = "pre_native"
		failure.Code = "binding_revision_conflict"
		failure.Message = "The expected SDD review binding revision does not match the current native binding."
		failure.MutationOutcome = ReviewMutationNotStarted
		failure.AuthorityApplicability = "current_target"
		failure.RetrySafe = true
		failure.Replayability = reviewtransaction.ReplayabilityNotReplayable
		failure.RequiredInputs = []string{"change", "lineage_id", "expected_binding_revision"}
		failure.NextAction = ReviewIntegrationOperationBindSDD
		failure.Context = &ReviewIntegrationFailureContext{BindingRevision: &ReviewIntegrationBindingRevisionConflict{
			Expected: bindingConflict.Expected, Current: bindingConflict.Current,
		}}
		return failure
	}
	var replayMismatch *reviewtransaction.FinalizeAttemptReplayMismatchError
	if errors.As(runErr, &replayMismatch) {
		failure.Phase = "reconciliation"
		failure.Code = "finalize_request_mismatch"
		failure.Message = "The FINALIZE request differs from the durable incomplete attempt."
		failure.MutationOutcome = ReviewMutationUnknown
		failure.AuthorityApplicability = "current_target"
		failure.RetrySafe = false
		failure.Replayability = reviewtransaction.ReplayabilityStatusRequired
		failure.LineageID = replayMismatch.LineageID
		failure.RequiredInputs = []string{"lineage_id"}
		failure.NextAction = "review.status"
		return failure
	}
	var publication *ReviewFacadeReceiptPublicationError
	if errors.As(runErr, &publication) {
		failure.Phase = "native_committed"
		if publication.Replayability == string(reviewtransaction.ReplayabilityManualActionRequired) {
			failure.Code = "receipt_publication_conflict"
			failure.Message = "Receipt publication conflicts with an existing immutable artifact."
			failure.MutationOutcome = ReviewMutationCommitted
			failure.AuthorityApplicability = "current_target"
			failure.Replayability = reviewtransaction.ReplayabilityManualActionRequired
			failure.LineageID = publication.LineageID
			failure.RequestDigest = publication.RequestDigest
			failure.RequiredInputs = []string{}
			failure.NextAction = "explicit-maintainer-action"
			return failure
		}
		failure.Code = "receipt_publication_pending"
		failure.Message = "Receipt publication did not complete after terminal authority was committed."
		failure.MutationOutcome = ReviewMutationCommitted
		failure.AuthorityApplicability = "current_target"
		failure.Replayability = reviewtransaction.ReplayabilityExactReplaySafe
		failure.LineageID = publication.LineageID
		failure.RequestDigest = publication.RequestDigest
		failure.RequiredInputs = []string{"lineage_id"}
		failure.NextAction = "review.finalize"
		return failure
	}
	var bindingPublication *sddstatus.ReviewBindingPublicationError
	if errors.As(runErr, &bindingPublication) {
		failure.Phase = "native_committed"
		failure.Code = "binding_publication_pending"
		failure.Message = "SDD review binding publication requires an exact replay to confirm directory durability."
		failure.MutationOutcome = ReviewMutationCommitted
		failure.AuthorityApplicability = "current_target"
		failure.RetrySafe = true
		failure.Replayability = reviewtransaction.ReplayabilityExactReplaySafe
		failure.RequestDigest = facadeValueHash("bind-sdd-request", args)
		failure.RequiredInputs = []string{"change", "lineage_id", "expected_binding_revision"}
		failure.NextAction = ReviewIntegrationOperationBindSDD
		return failure
	}
	var startContext *reviewStartContextError
	if errors.As(runErr, &startContext) {
		failure.Code = "candidate_context_unavailable"
		failure.LineageID = startContext.LineageID
		failure.RequiredInputs = []string{}
		if !startContext.AuthoritySelected {
			failure.Phase = "pre_native"
			failure.Message = "Frozen candidate context could not be rendered before START authority creation."
			failure.MutationOutcome = ReviewMutationNotStarted
			failure.AuthorityApplicability = "not_evaluated"
			failure.RetrySafe = false
			failure.Replayability = reviewtransaction.ReplayabilityManualActionRequired
			failure.NextAction = "stop"
			return failure
		}
		failure.Phase = "native_committed"
		failure.Message = "Frozen candidate context could not be rendered for the selected durable START authority."
		failure.MutationOutcome = ReviewMutationUnknown
		failure.AuthorityApplicability = "current_target"
		failure.RetrySafe = false
		failure.Replayability = reviewtransaction.ReplayabilityStatusRequired
		failure.RequiredInputs = []string{"lineage_id"}
		failure.NextAction = "review.status"
		return failure
	}
	var progress *reviewFacadeOperationProgressError
	if errors.As(runErr, &progress) {
		failure.Phase = "native_committed"
		failure.MutationOutcome = ReviewMutationUnknown
		failure.AuthorityApplicability = "current_target"
		failure.RetrySafe = false
		failure.Replayability = reviewtransaction.ReplayabilityStatusRequired
		failure.LineageID = progress.LineageID
		failure.RequiredInputs = []string{}
		failure.NextAction = "review.status"
		var progressedGitTimeout *reviewtransaction.GitCommandTimeoutError
		var progressedGitFailure *reviewtransaction.GitCommandError
		var progressedGitControl *reviewtransaction.GitProcessControlError
		switch {
		case errors.As(runErr, &progressedGitTimeout):
			failure.Code = "git_command_timeout"
			failure.Message = "A bounded Git subprocess timed out after review authority committed a native transition."
		case errors.As(runErr, &progressedGitFailure):
			failure.Code = "git_command_failed"
			failure.Message = "A Git subprocess failed after review authority committed a native transition."
		case errors.As(runErr, &progressedGitControl):
			failure.Code = "git_command_failed"
			failure.Message = "A Git subprocess could not be started or controlled after review authority committed a native transition: " + progressedGitControl.Error()
		case errors.Is(runErr, context.DeadlineExceeded):
			failure.Code = "operation_timeout"
			failure.Message = "The negotiated review operation timed out after review authority committed a native transition."
		}
		return failure
	}
	var repairProgress *reviewtransaction.ClassifiedAuthorityRepairProgressError
	if errors.As(runErr, &repairProgress) {
		progress := repairProgress.Progress
		failure.LineageID = progress.LineageID
		failure.RequestDigest = progress.RequestDigest
		failure.ProgressIdentity = progress.RecordIdentity
		failure.AuthorityApplicability = "not_evaluated"
		failure.RetrySafe = false
		failure.RequiredInputs = []string{"lineage_id"}
		if errors.Is(runErr, context.DeadlineExceeded) || errors.Is(runErr, context.Canceled) {
			failure.Code = "operation_timeout"
			failure.Message = "The negotiated review repair timed out after durable native repair progress."
		} else {
			failure.Code = "repair_progress_pending"
			failure.Message = "The negotiated review repair stopped after durable native repair progress."
		}
		if progress.Status == reviewtransaction.CompactReclaimCommitted {
			failure.Phase = "native_committed"
			failure.MutationOutcome = ReviewMutationCommitted
		} else {
			failure.Phase = "native_running"
			failure.MutationOutcome = ReviewMutationUnknown
		}
		if repairProgress.ExactReplaySafe {
			failure.RetrySafe = true
			failure.Replayability = reviewtransaction.ReplayabilityExactReplaySafe
			failure.NextAction = "review.repair"
		} else {
			failure.Replayability = reviewtransaction.ReplayabilityStatusRequired
			failure.NextAction = "review.status"
		}
		return failure
	}
	var gitTimeout *reviewtransaction.GitCommandTimeoutError
	if errors.As(runErr, &gitTimeout) {
		if gitTimeout.Aggregate {
			return reviewOperationTimeoutFailure(failure, operation, args)
		}
		failure.Phase = "pre_native"
		failure.Code = "git_command_timeout"
		failure.Message = "A bounded Git subprocess timed out before review authority mutation."
		failure.MutationOutcome = ReviewMutationNotStarted
		failure.AuthorityApplicability = "not_evaluated"
		failure.RetrySafe = false
		failure.Replayability = reviewtransaction.ReplayabilityManualActionRequired
		failure.NextAction = "stop"
		return failure
	}
	var gitFailure *reviewtransaction.GitCommandError
	if errors.As(runErr, &gitFailure) {
		failure.Phase = "pre_native"
		failure.Code = "git_command_failed"
		failure.Message = "A Git subprocess failed before review authority mutation."
		failure.MutationOutcome = ReviewMutationNotStarted
		failure.AuthorityApplicability = "not_evaluated"
		failure.RetrySafe = false
		failure.Replayability = reviewtransaction.ReplayabilityManualActionRequired
		failure.NextAction = "stop"
		return failure
	}
	var gitControl *reviewtransaction.GitProcessControlError
	if errors.As(runErr, &gitControl) {
		failure.Phase = "pre_native"
		failure.Code = "git_command_failed"
		failure.Message = "A Git subprocess could not be started or controlled before review authority mutation: " + gitControl.Error()
		failure.MutationOutcome = ReviewMutationNotStarted
		failure.AuthorityApplicability = "not_evaluated"
		failure.RetrySafe = false
		failure.Replayability = reviewtransaction.ReplayabilityManualActionRequired
		failure.NextAction = "stop"
		return failure
	}
	if errors.Is(runErr, context.DeadlineExceeded) {
		return reviewOperationTimeoutFailure(failure, operation, args)
	}
	var preflight *reviewIntegrationPreflightError
	if errors.As(runErr, &preflight) {
		preflightFailure := newReviewIntegrationPreflightFailure(operation, "invalid_request", "The negotiated review request is invalid.")
		preflightFailure.LineageID = failure.LineageID
		return preflightFailure
	}
	var legacy *reviewtransaction.LegacyReadOnlyError
	if errors.As(runErr, &legacy) {
		failure.Code = reviewtransaction.LegacyReadOnlyErrorCode
		failure.Message = "Legacy v1 review authority is read-only and cannot be mutated."
		failure.MutationOutcome = ReviewMutationNotStarted
		failure.AuthorityApplicability = "current_target"
		failure.Replayability = reviewtransaction.ReplayabilityNotReplayable
		failure.NextAction = "stop"
		return failure
	}
	var lockTimeout *reviewtransaction.AuthorityLockTimeoutError
	var lockCancelled *reviewtransaction.AuthorityLockCancelledError
	if errors.As(runErr, &lockTimeout) || errors.As(runErr, &lockCancelled) {
		label := reviewLockOperationLabel(operation)
		failure.Phase = "pre_native"
		failure.Code = "authority_lock_timeout"
		failure.Message = label + " could not acquire the authority lock within the bounded wait."
		if lockCancelled != nil {
			failure.Code = "authority_lock_cancelled"
			failure.Message = label + " authority lock acquisition was cancelled."
		}
		failure.MutationOutcome = ReviewMutationNotStarted
		failure.AuthorityApplicability = "not_evaluated"
		failure.RetrySafe = lockTimeout != nil
		failure.Replayability = reviewtransaction.ReplayabilityManualActionRequired
		failure.NextAction = "retry_with_bounded_backoff"
		if lockCancelled != nil {
			failure.NextAction = "stop"
		}
		return failure
	}
	var targetResolution *reviewtransaction.GateTargetResolutionError
	if errors.As(runErr, &targetResolution) {
		failure.Phase = "pre_native"
		failure.Code = "target_resolution_failed"
		failure.Message = "The pre-push target cannot be resolved; configure an upstream or pass --base-ref <remote>/<branch>."
		failure.MutationOutcome = ReviewMutationNotStarted
		failure.AuthorityApplicability = "not_evaluated"
		failure.RetrySafe = true
		failure.Replayability = reviewtransaction.ReplayabilityNotReplayable
		failure.RequiredInputs = []string{"base_ref"}
		failure.NextAction = "correct_request"
		return failure
	}
	var denied ReviewGateDeniedError
	if errors.As(runErr, &denied) {
		failure.Phase = "preflight"
		failure.Code = "gate_" + strings.ReplaceAll(string(denied.Result), "-", "_")
		failure.Message = "The review delivery gate denied the current target."
		failure.MutationOutcome = ReviewMutationNotStarted
		failure.AuthorityApplicability = "current_target"
		failure.RetrySafe = true
		failure.Replayability = reviewtransaction.ReplayabilityManualActionRequired
		failure.NextAction = reviewGateAction(denied.Result)
		if denied.Context.Gate != "" && denied.Context.ScopeChange != nil {
			failure.Context = publicReviewScopeChangeContext(denied.Context.ScopeChange)
		}
		if denied.Result == reviewtransaction.GateScopeChanged && denied.Context.ScopeChange != nil {
			failure.RetrySafe = false
			failure.RequiredInputs = append([]string{}, denied.Context.ScopeChange.RecoveryRequiredInputs...)
		}
		if lineage := safeReviewIntegrationLineage(operation, args); lineage != "" {
			failure.LineageID = lineage
		}
		return failure
	}
	var discovery *ReviewReceiptDiscoveryError
	if errors.As(runErr, &discovery) {
		failure.Phase = "pre_native"
		failure.Code = string(discovery.Kind)
		failure.Message = "No unique exact review receipt applies to the live gate target."
		failure.MutationOutcome = ReviewMutationNotStarted
		failure.AuthorityApplicability = "unrelated"
		failure.RetrySafe = false
		failure.Replayability = reviewtransaction.ReplayabilityManualActionRequired
		failure.NextAction = "stop"
		switch discovery.Kind {
		case ReviewReceiptMissing:
			failure.AuthorityApplicability = "not_evaluated"
		case ReviewReceiptScopeChanged:
			if discovery.Context != nil {
				failure.Context = publicReviewScopeChangeContext(discovery.Context.ScopeChange)
				failure.AuthorityApplicability = "current_target"
				if failure.Context != nil && failure.Context.ScopeChange != nil {
					failure.RequiredInputs = append([]string{}, failure.Context.ScopeChange.RecoveryRequiredInputs...)
				}
			}
			failure.NextAction = "explicit-maintainer-action"
		case ReviewReceiptAmbiguous:
			failure.AuthorityApplicability = "ambiguous"
			failure.RequiredInputs = []string{"lineage_id"}
			failure.Candidates = append([]string{}, discovery.Candidates...)
			failure.NextAction = "review.status"
		case ReviewAuthorityCorrupted:
			failure.AuthorityApplicability = "corrupted"
			failure.CauseCategory = discovery.Category
		}
		return failure
	}
	if operation == "review.capabilities" || operation == "review.status" || operation == "review.validate" {
		failure.Phase = "pre_native"
		failure.Code = "operation_failed"
		failure.Message = "The negotiated read-only review operation failed safely."
		failure.MutationOutcome = ReviewMutationNotStarted
		failure.RetrySafe = true
		failure.Replayability = reviewtransaction.ReplayabilityNotReplayable
		failure.NextAction = "retry"
	}
	return failure
}

func reviewLockOperationLabel(operation string) string {
	if metadata, ok := reviewIntegrationOperationByName(operation); ok {
		return metadata.Label
	}
	return "Review operation"
}

func publicReviewScopeChangeContext(scope *reviewtransaction.GateScopeChangeDiagnostics) *ReviewIntegrationFailureContext {
	if scope == nil {
		return nil
	}
	return &ReviewIntegrationFailureContext{ScopeChange: &ReviewIntegrationScopeChange{
		Expected:           ReviewIntegrationScopeTarget{CandidateTree: scope.Expected.CandidateTree, PathsDigest: scope.Expected.PathsDigest},
		Actual:             ReviewIntegrationScopeTarget{CandidateTree: scope.Actual.CandidateTree, PathsDigest: scope.Actual.PathsDigest},
		DifferingPathCount: scope.DifferingPathCount, DifferingPathsDigest: scope.DifferingPathsDigest,
		PredecessorLineageID: scope.PredecessorLineageID, PredecessorRevision: scope.PredecessorRevision,
		RecoveryOperation: scope.RecoveryOperation, RecoveryRequiredInputs: append([]string{}, scope.RecoveryRequiredInputs...),
	}}
}

func reviewOperationTimeoutFailure(failure ReviewIntegrationFailure, operation string, args []string) ReviewIntegrationFailure {
	failure.Code = "operation_timeout"
	failure.Message = "The negotiated review operation exceeded its aggregate time budget."
	metadata, known := reviewIntegrationOperationByName(operation)
	if known && metadata.TimeoutRetryable {
		failure.Phase = "pre_native"
		failure.MutationOutcome = ReviewMutationNotStarted
		failure.AuthorityApplicability = "not_evaluated"
		failure.RetrySafe = true
		failure.Replayability = reviewtransaction.ReplayabilityNotReplayable
		failure.NextAction = "retry"
		return failure
	}
	failure.RetrySafe = false
	if known && reviewIntegrationOperationMutates(metadata, args) {
		failure.Phase = "native_running"
		failure.MutationOutcome = ReviewMutationUnknown
		failure.AuthorityApplicability = "not_evaluated"
		failure.Replayability = reviewtransaction.ReplayabilityStatusRequired
		failure.NextAction = "review.status"
		return failure
	}
	failure.Phase = "pre_native"
	failure.MutationOutcome = ReviewMutationNotStarted
	failure.AuthorityApplicability = "not_evaluated"
	failure.Replayability = reviewtransaction.ReplayabilityManualActionRequired
	failure.NextAction = "stop"
	return failure
}

type reviewIntegrationFlagKind uint8

const (
	reviewIntegrationValueFlag reviewIntegrationFlagKind = iota
	reviewIntegrationBoolFlag
	reviewIntegrationIntFlag
)

func safeReviewIntegrationLineage(operation string, args []string) string {
	values, valid := safeReviewIntegrationArguments(operation, args)
	if !valid {
		return ""
	}
	value := values["lineage"]
	if operation == ReviewIntegrationOperationRetryFinalVerification {
		value = values["predecessor-lineage"]
	}
	if !validReviewIntegrationLineage(value) {
		return ""
	}
	return value
}

func safeReviewIntegrationArguments(operation string, args []string) (map[string]string, bool) {
	shape := reviewIntegrationOperationFlagShape(operation)
	values := map[string]string{}
	valid := true
	for index := 0; index < len(args); index++ {
		arg := args[index]
		if arg == "--" {
			break
		}
		if arg == "" || arg == "-" || arg[0] != '-' {
			break
		}
		nameValue := strings.TrimPrefix(arg, "-")
		nameValue = strings.TrimPrefix(nameValue, "-")
		name, flagValue, hasValue := nameValue, "", false
		if separator := strings.IndexByte(nameValue, '='); separator >= 0 {
			name, flagValue, hasValue = nameValue[:separator], nameValue[separator+1:], true
		}
		kind, known := shape[name]
		if !known {
			valid = false
			break
		}
		switch kind {
		case reviewIntegrationBoolFlag:
			if hasValue {
				if _, err := strconv.ParseBool(flagValue); err != nil {
					valid = false
					index = len(args)
				}
			} else {
				flagValue = "true"
			}
			values[name] = flagValue
			continue
		case reviewIntegrationValueFlag, reviewIntegrationIntFlag:
			if !hasValue {
				if index+1 >= len(args) {
					valid = false
					index = len(args)
					continue
				}
				index++
				flagValue = args[index]
			}
			if kind == reviewIntegrationIntFlag {
				if _, err := strconv.Atoi(flagValue); err != nil {
					valid = false
					index = len(args)
				}
			}
		}
		values[name] = flagValue
	}
	return values, valid
}

func reviewIntegrationOperationFlagShape(operation string) map[string]reviewIntegrationFlagKind {
	metadata, _ := reviewIntegrationOperationByName(operation)
	valueFlags := append([]string{"contract"}, metadata.ValueFlags...)
	boolFlags := append([]string{}, metadata.BoolFlags...)
	intFlags := append([]string{}, metadata.IntFlags...)
	shape := make(map[string]reviewIntegrationFlagKind, len(valueFlags)+len(boolFlags)+len(intFlags)+2)
	for _, name := range valueFlags {
		shape[name] = reviewIntegrationValueFlag
	}
	for _, name := range boolFlags {
		shape[name] = reviewIntegrationBoolFlag
	}
	for _, name := range intFlags {
		shape[name] = reviewIntegrationIntFlag
	}
	shape["h"] = reviewIntegrationBoolFlag
	shape["help"] = reviewIntegrationBoolFlag
	return shape
}

func reviewIntegrationOperationMutates(metadata reviewIntegrationOperationMetadata, args []string) bool {
	if !metadata.MutatesAuthority {
		return false
	}
	if metadata.ReadOnlyFlag == "" {
		return true
	}
	values, valid := safeReviewIntegrationArguments(metadata.Operation, args)
	if !valid {
		return true
	}
	readOnly, err := strconv.ParseBool(values[metadata.ReadOnlyFlag])
	return err != nil || !readOnly
}

func reviewNamedArgument(args []string, name string) (provided bool, value string, missing bool) {
	prefix := "--" + name + "="
	flagName := "--" + name
	for index := 0; index < len(args); index++ {
		if strings.HasPrefix(args[index], prefix) {
			provided, value, missing = true, strings.TrimPrefix(args[index], prefix), false
			continue
		}
		if args[index] != flagName {
			continue
		}
		provided = true
		if index+1 >= len(args) {
			return true, "", true
		}
		value, missing = args[index+1], false
		index++
	}
	return provided, value, missing
}

func validReviewIntegrationLineage(value string) bool {
	if value == "" || len(value) > 128 || value[0] == '-' || value[len(value)-1] == '-' {
		return false
	}
	previousHyphen := false
	for _, char := range value {
		if char != '-' && (char < 'a' || char > 'z') && (char < '0' || char > '9') {
			return false
		}
		if char == '-' && previousHyphen {
			return false
		}
		previousHyphen = char == '-'
	}
	return true
}

func (failure ReviewIntegrationFailure) Validate() error {
	if failure.Schema != ReviewIntegrationFailureSchema && failure.Schema != ReviewIntegrationFailureSchemaV2 || failure.Contract != ReviewIntegrationContractV1 ||
		!validReviewIntegrationFailureOperation(failure.Operation) {
		return errors.New("invalid negotiated review failure identity")
	}
	if !validReviewIntegrationFailureCode(failure.Code) || strings.TrimSpace(failure.Message) != failure.Message ||
		failure.Message == "" || len(failure.Message) > 240 || strings.ContainsAny(failure.Message, "\r\n") {
		return errors.New("invalid negotiated review failure message")
	}
	switch failure.Phase {
	case "preflight", "pre_native", "native_running", "native_committed", "reconciliation":
	default:
		return errors.New("invalid negotiated review failure phase")
	}
	switch failure.MutationOutcome {
	case ReviewMutationNotStarted, ReviewMutationUnknown, ReviewMutationCommitted:
	default:
		return errors.New("invalid negotiated review mutation outcome")
	}
	switch failure.AuthorityApplicability {
	case "current_target", "unrelated", "ambiguous", "corrupted", "not_evaluated":
	default:
		return errors.New("invalid negotiated review authority applicability")
	}
	switch failure.Replayability {
	case reviewtransaction.ReplayabilityNotReplayable, reviewtransaction.ReplayabilityExactReplaySafe,
		reviewtransaction.ReplayabilityStatusRequired, reviewtransaction.ReplayabilityManualActionRequired:
	default:
		return errors.New("invalid negotiated review failure replayability")
	}
	if failure.RequiredInputs == nil || strings.TrimSpace(failure.NextAction) == "" {
		return errors.New("negotiated review failure action is incomplete")
	}
	for _, input := range failure.RequiredInputs {
		if !supportedReviewIntegrationFailureInput(input) {
			return errors.New("unsupported negotiated review failure input")
		}
	}
	if failure.Schema == ReviewIntegrationFailureSchemaV2 && failure.Code == "receipt_ambiguous" {
		if len(failure.Candidates) < 2 {
			return errors.New("negotiated review ambiguity candidates are incomplete")
		}
		for _, lineage := range failure.Candidates {
			if !validReviewIntegrationLineage(lineage) {
				return errors.New("invalid negotiated review ambiguity candidate")
			}
		}
	} else if len(failure.Candidates) != 0 {
		return errors.New("negotiated review candidates require receipt ambiguity")
	}
	if failure.Code == "receipt_ambiguous" && (failure.Schema == ReviewIntegrationFailureSchemaV2 && (!reflect.DeepEqual(failure.RequiredInputs, []string{"lineage_id"}) || failure.NextAction != ReviewIntegrationOperationValidate) ||
		failure.Schema == ReviewIntegrationFailureSchema && (len(failure.RequiredInputs) != 0 || failure.NextAction != "stop")) {
		return errors.New("negotiated review ambiguity route is inconsistent")
	}
	if failure.Context != nil {
		if (failure.Context.ScopeChange == nil) == (failure.Context.BindingRevision == nil) {
			return errors.New("negotiated review failure context must select exactly one diagnostic")
		}
		if scope := failure.Context.ScopeChange; scope != nil {
			if failure.Operation != ReviewIntegrationOperationValidate {
				return errors.New("negotiated review scope context is not a gate denial")
			}
			if failure.Code != "gate_scope_changed" && failure.Code != "receipt_scope_changed" || scope.DifferingPathCount < 0 || scope.DifferingPathCount > 1000000 ||
				!validReviewGitTree(scope.Expected.CandidateTree) || !validReviewCapabilitySHA256(scope.Expected.PathsDigest) ||
				!validReviewGitTree(scope.Actual.CandidateTree) || !validReviewCapabilitySHA256(scope.Actual.PathsDigest) || !validReviewCapabilitySHA256(scope.DifferingPathsDigest) ||
				!validReviewIntegrationLineage(scope.PredecessorLineageID) || !validReviewCapabilitySHA256(scope.PredecessorRevision) ||
				scope.RecoveryOperation != "review.recover" || !reflect.DeepEqual(failure.RequiredInputs, scope.RecoveryRequiredInputs) ||
				!reflect.DeepEqual(scope.RecoveryRequiredInputs, []string{"predecessor_lineage_id", "expected_predecessor_revision", "successor_lineage_id", "disposition", "reason", "actor"}) {
				return errors.New("negotiated review scope-change diagnostics are incomplete")
			}
		}
		if conflict := failure.Context.BindingRevision; conflict != nil {
			if failure.Operation != ReviewIntegrationOperationBindSDD || failure.Code != "binding_revision_conflict" ||
				!validOptionalReviewSHA256(conflict.Expected) || !validOptionalReviewSHA256(conflict.Current) ||
				!reflect.DeepEqual(failure.RequiredInputs, []string{"change", "lineage_id", "expected_binding_revision"}) ||
				failure.NextAction != ReviewIntegrationOperationBindSDD {
				return errors.New("negotiated review binding-revision diagnostics are incomplete")
			}
		}
	}
	if failure.LineageID != "" && !validReviewIntegrationLineage(failure.LineageID) ||
		failure.RequestDigest != "" && !validReviewCapabilitySHA256(failure.RequestDigest) ||
		failure.RequestDigest != "" && failure.LineageID == "" ||
		failure.ProgressIdentity != "" && (!validReviewCapabilitySHA256(failure.ProgressIdentity) || failure.RequestDigest == "" || failure.Operation != "review.repair") ||
		failure.Operation == "review.repair" && failure.RequestDigest != "" && failure.ProgressIdentity == "" {
		return errors.New("invalid negotiated review failure replay identity")
	}
	if failure.MutationOutcome == ReviewMutationUnknown {
		exactRepairReplay := failure.Operation == "review.repair" && failure.RetrySafe &&
			failure.Replayability == reviewtransaction.ReplayabilityExactReplaySafe && failure.NextAction == "review.repair" &&
			failure.RequestDigest != "" && failure.ProgressIdentity != ""
		if !exactRepairReplay && (failure.RetrySafe || failure.Replayability != reviewtransaction.ReplayabilityStatusRequired || failure.NextAction != "review.status") {
			return errors.New("unknown negotiated review mutation must require status or one bound repair replay")
		}
	}
	if failure.Replayability == reviewtransaction.ReplayabilityExactReplaySafe {
		mutationAllowsExactReplay := failure.MutationOutcome == ReviewMutationCommitted ||
			failure.Operation == "review.repair" && failure.MutationOutcome == ReviewMutationUnknown
		if !mutationAllowsExactReplay || failure.LineageID == "" || failure.RequestDigest == "" {
			return errors.New("exact negotiated review replay is incomplete")
		}
		switch failure.Operation {
		case ReviewIntegrationOperationFinalize:
			if !reflect.DeepEqual(failure.RequiredInputs, []string{"lineage_id"}) || failure.NextAction != ReviewIntegrationOperationFinalize {
				return errors.New("exact negotiated review replay is incomplete")
			}
		case ReviewIntegrationOperationBindSDD:
			if !reflect.DeepEqual(failure.RequiredInputs, []string{"change", "lineage_id", "expected_binding_revision"}) || failure.NextAction != ReviewIntegrationOperationBindSDD {
				return errors.New("exact negotiated review replay is incomplete")
			}
		case "review.repair":
			if !reflect.DeepEqual(failure.RequiredInputs, []string{"lineage_id"}) || failure.NextAction != "review.repair" ||
				failure.ProgressIdentity == "" {
				return errors.New("exact negotiated review repair replay is incomplete")
			}
		default:
			return errors.New("exact negotiated review replay operation is unsupported")
		}
	}
	return nil
}

func validOptionalReviewSHA256(value string) bool {
	return value == "" || validReviewCapabilitySHA256(value)
}

func supportedReviewIntegrationFailureInput(input string) bool {
	switch input {
	case "lineage_id", "change", "expected_binding_revision", "predecessor_lineage_id", "expected_predecessor_revision", "successor_lineage_id", "disposition", "reason", "actor", "incident", "maintainer_authorization", "base_ref":
		return true
	default:
		return false
	}
}

func validReviewIntegrationFailureOperation(operation string) bool {
	_, valid := reviewIntegrationOperationByName(operation)
	return valid
}

func validReviewIntegrationFailureCode(code string) bool {
	if code == "" {
		return false
	}
	for _, char := range code {
		if char != '_' && (char < 'a' || char > 'z') && (char < '0' || char > '9') {
			return false
		}
	}
	return true
}

func emitReviewIntegrationFailure(stdout io.Writer, failure ReviewIntegrationFailure) error {
	if err := failure.Validate(); err != nil {
		return fmt.Errorf("validate negotiated review failure: %w", err)
	}
	return encodeReviewJSON(stdout, failure)
}

type ReviewIntegrationOperationResult struct {
	Schema    string          `json:"schema"`
	Contract  string          `json:"contract"`
	Operation string          `json:"operation"`
	Result    json.RawMessage `json:"result"`
}

// ReviewIntegrationFinalizeResult preserves the existing finalize semantics
// while excluding the provider-private receipt path from negotiated output.
type ReviewIntegrationFinalizeResult struct {
	Operation         string                                       `json:"operation"`
	LineageID         string                                       `json:"lineage_id"`
	State             reviewtransaction.State                      `json:"state"`
	Action            string                                       `json:"action"`
	StoreRevision     string                                       `json:"store_revision"`
	Eligibility       *ReviewActionEligibility                     `json:"eligibility,omitempty"`
	NextTransition    *ReviewNextTransition                        `json:"next_transition,omitempty"`
	ValidationRequest *reviewtransaction.TargetedValidationRequest `json:"validation_request,omitempty"`
}

func reviewIntegrationNegotiation(flags *flag.FlagSet, contract string) (bool, error) {
	if !reviewFlagWasProvided(flags, "contract") {
		return false, nil
	}
	if err := validateReviewIntegrationContract(contract); err != nil {
		return false, err
	}
	return true, nil
}

func reviewFlagWasProvided(flags *flag.FlagSet, name string) bool {
	provided := false
	flags.Visit(func(value *flag.Flag) {
		provided = provided || value.Name == name
	})
	return provided
}

func encodeReviewIntegrationOperation(stdout io.Writer, negotiated bool, operation string, legacyResult, publicResult any) error {
	if !negotiated {
		return encodeReviewJSON(stdout, legacyResult)
	}
	payload, err := json.Marshal(publicResult)
	if err != nil {
		return fmt.Errorf("encode negotiated %s result: %w", operation, err)
	}
	envelope := ReviewIntegrationOperationResult{
		Schema: ReviewIntegrationOperationSchema, Contract: ReviewIntegrationContractV1,
		Operation: operation, Result: payload,
	}
	if err := envelope.Validate(); err != nil {
		return fmt.Errorf("validate negotiated %s result: %w", operation, err)
	}
	return encodeReviewJSON(stdout, envelope)
}

func (result ReviewIntegrationOperationResult) Validate() error {
	if result.Schema != ReviewIntegrationOperationSchema || result.Contract != ReviewIntegrationContractV1 || len(result.Result) == 0 {
		return errors.New("invalid negotiated review operation identity")
	}
	var document any
	if err := json.Unmarshal(result.Result, &document); err != nil {
		return fmt.Errorf("parse negotiated review operation result: %w", err)
	}
	if _, object := document.(map[string]any); !object {
		return errors.New("negotiated review operation result must be an object")
	}
	if field := forbiddenReviewIntegrationResultField(document); field != "" {
		return fmt.Errorf("negotiated review operation result contains private field %q", field)
	}
	switch result.Operation {
	case ReviewIntegrationOperationFinalize:
		var finalized ReviewIntegrationFinalizeResult
		if err := decodeStrictReviewIntegrationResult(result.Result, &finalized); err != nil {
			return err
		}
		if finalized.Operation != "review/finalize" || strings.TrimSpace(finalized.LineageID) == "" ||
			strings.TrimSpace(finalized.Action) == "" || !validReviewCapabilitySHA256(finalized.StoreRevision) || strings.TrimSpace(string(finalized.State)) == "" {
			return errors.New("negotiated finalize result is incomplete")
		}
		if finalized.Eligibility != nil {
			if err := finalized.Eligibility.ValidateFinalize(); err != nil {
				return fmt.Errorf("negotiated finalize result action eligibility: %w", err)
			}
		}
		if finalized.NextTransition != nil {
			if err := finalized.NextTransition.Validate(); err != nil {
				return fmt.Errorf("negotiated finalize result next transition: %w", err)
			}
			transitionRequest := reviewTransitionValidationRequest(finalized.NextTransition)
			if (transitionRequest == nil) != (finalized.ValidationRequest == nil) ||
				transitionRequest != nil && !reflect.DeepEqual(*transitionRequest, *finalized.ValidationRequest) {
				return errors.New("negotiated finalize validation request copies differ")
			}
		}
		if finalized.ValidationRequest != nil {
			if finalized.State != reviewtransaction.StateCorrectionRequired ||
				finalized.ValidationRequest.LineageID != finalized.LineageID ||
				finalized.ValidationRequest.ExpectedRevision != finalized.StoreRevision ||
				reviewtransaction.ValidateTargetedValidationRequest(*finalized.ValidationRequest) != nil {
				return errors.New("negotiated finalize result validation request is invalid")
			}
		}
	case ReviewIntegrationOperationValidate:
		var validated ReviewValidateResult
		if err := decodeStrictReviewIntegrationResult(result.Result, &validated); err != nil {
			return err
		}
		if validated.Schema != ReviewValidateSchema || validated.Allowed != (validated.Result == reviewtransaction.GateAllow) ||
			strings.TrimSpace(validated.Action) == "" || strings.TrimSpace(validated.Reason) == "" ||
			(validated.Context.Gate != "" && !validReviewIntegrationGate(validated.Context.Gate)) ||
			(validated.Allowed && !validReviewIntegrationGate(validated.Context.Gate)) {
			return errors.New("negotiated validate result is inconsistent")
		}
	case ReviewIntegrationOperationBindSDD:
		var binding sddstatus.ReviewBinding
		if err := decodeStrictReviewIntegrationResult(result.Result, &binding); err != nil {
			return err
		}
		if binding.Schema != "gentle-ai.sdd-review-binding/v1" || strings.TrimSpace(binding.Change) == "" || strings.TrimSpace(binding.Lineage) == "" ||
			!validReviewCapabilitySHA256(binding.Revision) || !validReviewCapabilitySHA256(binding.AuthorityRevision) ||
			!validReviewCapabilitySHA256(binding.ReceiptHash) || binding.GateContext.Gate != reviewtransaction.GatePostApply {
			return errors.New("negotiated bind-sdd result is incomplete")
		}
	case ReviewIntegrationOperationRetryFinalVerification:
		var retried ReviewFinalVerificationRetryResult
		if err := decodeStrictReviewIntegrationResult(result.Result, &retried); err != nil {
			return err
		}
		if err := retried.Validate(); err != nil {
			return err
		}
	default:
		return fmt.Errorf("unsupported negotiated review operation %q", result.Operation)
	}
	return nil
}

func decodeStrictReviewIntegrationResult(payload []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return fmt.Errorf("decode negotiated review operation result: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return errors.New("negotiated review operation result contains multiple JSON values")
	}
	return nil
}

func forbiddenReviewIntegrationResultField(value any) string {
	switch typed := value.(type) {
	case map[string]any:
		for key, child := range typed {
			lower := strings.ToLower(key)
			if lower == "model" || lower == "provider" || lower == "profile" || lower == "cwd" || lower == "repository" ||
				lower == "path" || strings.HasSuffix(lower, "_path") {
				return key
			}
			if found := forbiddenReviewIntegrationResultField(child); found != "" {
				return found
			}
		}
	case []any:
		for _, child := range typed {
			if found := forbiddenReviewIntegrationResultField(child); found != "" {
				return found
			}
		}
	}
	return ""
}

func validReviewIntegrationGate(gate reviewtransaction.GateKind) bool {
	switch gate {
	case reviewtransaction.GatePostApply, reviewtransaction.GatePreCommit, reviewtransaction.GatePrePush,
		reviewtransaction.GatePrePR, reviewtransaction.GateRelease:
		return true
	default:
		return false
	}
}
