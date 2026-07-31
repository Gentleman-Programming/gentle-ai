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

	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
	"github.com/gentleman-programming/gentle-ai/v2/internal/sddstatus"
)

const ReviewIntegrationOperationSchema = "gentle-ai.review-integration.operation/v1"
const ReviewIntegrationOperationSchemaID = "https://gentle-ai.dev/contracts/review-integration/v1/schemas/operation.schema.json"
const ReviewIntegrationOperationSchemaV2 = "gentle-ai.review-integration.operation/v2"
const ReviewIntegrationOperationSchemaIDV2 = "https://gentle-ai.dev/contracts/review-integration/v2/schemas/operation.schema.json"
const ReviewIntegrationFailureSchema = "gentle-ai.review-integration.failure/v1"
const ReviewIntegrationFailureSchemaID = "https://gentle-ai.dev/contracts/review-integration/v1/schemas/failure.schema.json"
const ReviewIntegrationFailureSchemaV2 = "gentle-ai.review-integration.failure/v2"
const ReviewIntegrationFailureSchemaIDV2 = "https://gentle-ai.dev/contracts/review-integration/v2/schemas/failure.schema.json"

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
	// The reviewer-result and evidence routes are all declared here, including
	// the retired "result": an undeclared flag makes safeReviewIntegrationArguments
	// treat the whole invocation as unparseable, which silently drops lineage_id
	// from the failure envelope. Only "result" was ever listed, so every
	// negotiated finalize failure on the admitted routes reported less than the
	// unsafe one did.
	{Command: "finalize", Operation: ReviewIntegrationOperationFinalize, Label: "Review FINALIZE", ValueFlags: []string{"cwd", "lineage", "validation", "refuter", "evidence", "trace", "result", "result-artifact", "result-artifact-file"}, BoolFlags: []string{"failed", "captured-results", "captured-evidence"}, IntFlags: []string{"correction-lines"}, MutatesAuthority: true},
	{Command: "repair", Operation: "review.repair", Label: "Review REPAIR", ValueFlags: []string{"cwd", "class", "lineage", "expected-revision", "cause", "disposition", "repository-binding", "actor", "reason", "maintainer-authorization"}, BoolFlags: []string{"preflight"}, MutatesAuthority: true, JoinOnTimeout: true, ReadOnlyFlag: "preflight"},
	{Command: "retry-final-verification", Operation: ReviewIntegrationOperationRetryFinalVerification, Label: "Review RETRY-FINAL-VERIFICATION", ValueFlags: []string{"cwd", "predecessor-lineage", "expected-predecessor-revision", "successor-lineage", "incident", "actor", "reason", "maintainer-authorization"}, MutatesAuthority: true, JoinOnTimeout: true},
	{Command: "start", Operation: "review.start", Label: "Review START", ValueFlags: []string{"cwd", "target", "lineage", "policy", "focus", "base-ref", "projection", "trace", "consent", "locale"}, BoolFlags: []string{"committed-only", "workspace-overlay"}, MutatesAuthority: true},
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
	Schema                 string                          `json:"schema"`
	Contract               string                          `json:"contract"`
	Operation              string                          `json:"operation"`
	Phase                  string                          `json:"phase"`
	Code                   string                          `json:"code"`
	Message                string                          `json:"message"`
	MutationOutcome        ReviewMutationOutcome           `json:"mutation_outcome"`
	AuthorityApplicability string                          `json:"authority_applicability"`
	RetrySafe              bool                            `json:"retry_safe"`
	Replayability          reviewtransaction.Replayability `json:"replayability"`
	LineageID              string                          `json:"lineage_id,omitempty"`
	RequestDigest          string                          `json:"request_digest,omitempty"`
	ProgressIdentity       string                          `json:"progress_identity,omitempty"`
	RequiredInputs         []string                        `json:"required_inputs"`
	NextAction             string                          `json:"next_action"`
	CauseCategory          string                          `json:"cause_category,omitempty"`
	// Cause is additive: the wrapped real native cause for the
	// operation_outcome_unknown default envelope, so a caller is never left
	// with only a fixed placeholder message. It is never populated on the
	// read-only catch-all, which stays deliberately content-free.
	Cause   string                           `json:"cause,omitempty"`
	Context *ReviewIntegrationFailureContext `json:"context,omitempty"`
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
	// defectReportClause is the optional " A defect report was saved at ..."
	// tail for the unanticipated-residue class. It decorates only the
	// operator-facing error line; the schema-bounded envelope never carries it.
	defectReportClause string
}

func (err *ReviewIntegrationFailureError) Error() string {
	return fmt.Sprintf("%s [%s]%s", err.Failure.Message, err.Failure.Code, err.defectReportClause)
}

func (err *ReviewIntegrationFailureError) Unwrap() error { return err.cause }

func newReviewIntegrationFailureError(failure ReviewIntegrationFailure, cause error) *ReviewIntegrationFailureError {
	return &ReviewIntegrationFailureError{Failure: failure, cause: cause}
}

const (
	// reviewIntegrationInvalidRequestCode is the machine-branchable code for a
	// preflight refusal the caller genuinely can fix by correcting the request
	// it sent.
	reviewIntegrationInvalidRequestCode = "invalid_request"
	// reviewIntegrationGenericPreflightMessage is the stable contract sentence
	// that accompanies reviewIntegrationInvalidRequestCode. It is deliberately
	// content-free: the specific reason belongs in the additive `cause` field,
	// never in this bounded 240-character contract string.
	reviewIntegrationGenericPreflightMessage = "The negotiated review request is invalid."
	// reviewPreflightStaleTargetCode names the one preflight class where
	// "correct_request" is an actively wrong instruction. The target identity
	// is derived from the live workspace snapshot, so anything that writes a
	// file between the proposed transition and its execution invalidates it.
	// No edit to the request text can make a stale snapshot fresh; the caller
	// must re-derive the exact next transition instead.
	reviewPreflightStaleTargetCode = "stale_target_identity"
	// reviewPreflightStaleTargetMessage stays inside the published 240-char
	// bound and names the runnable continuation, per the standing rule that
	// every block names one.
	reviewPreflightStaleTargetMessage = "The negotiated review request no longer matches the live workspace snapshot; re-derive the exact next transition before retrying."
)

// reviewPreflightReason is the machine-branchable half of a preflight
// refusal. It carries only fields the published v1 failure schema already
// defines, so classifying a refusal never changes the wire contract. The
// human-readable half travels separately in the additive `cause` field.
type reviewPreflightReason struct {
	Code           string
	Message        string
	RequiredInputs []string
	NextAction     string
}

// reviewPreflightStaleTargetReason classifies every refusal whose precondition
// is "the frozen identity no longer describes the live workspace".
var reviewPreflightStaleTargetReason = reviewPreflightReason{
	Code:       reviewPreflightStaleTargetCode,
	Message:    reviewPreflightStaleTargetMessage,
	NextAction: "review.status",
}

// reviewPreflightUntrackedScopeReason classifies a START whose untracked scope
// discovery refused the repository's working-tree shape (issue #1881: an
// embedded foreign repository, or a hostile untracked path Git reported).
// Discovery runs strictly before any snapshot is built or any authority is
// created, so the honest classification is a not_started preflight refusal —
// the operation_outcome_unknown default it previously fell into claimed an
// unknown mutation and sent the caller to STATUS, both false for a failure
// that provably wrote nothing. `next_action` is "stop" because the exit is a
// repository-layout change, not a request edit; the specific blocking path and
// the way out travel in the additive `cause` field.
var reviewPreflightUntrackedScopeReason = reviewPreflightReason{
	Code:       "untracked_scope_undiscoverable",
	Message:    "The untracked review scope could not be discovered from the repository working tree; the cause names the blocking path and the way out.",
	NextAction: "stop",
}

// reviewPreflightMissingInputsReason names the contract-level inputs a caller
// must supply. Callers pass only inputs the published required_inputs enum
// actually defines; a refusal whose missing inputs are not all expressible
// there keeps an empty list and names them in prose instead, because a partial
// list would read as a complete one.
func reviewPreflightMissingInputsReason(inputs ...string) reviewPreflightReason {
	return reviewPreflightReason{
		Code:           reviewIntegrationInvalidRequestCode,
		Message:        reviewIntegrationGenericPreflightMessage,
		RequiredInputs: append([]string{}, inputs...),
		NextAction:     "correct_request",
	}
}

type reviewIntegrationPreflightError struct {
	cause  error
	reason *reviewPreflightReason
}

func (err *reviewIntegrationPreflightError) Error() string { return err.cause.Error() }
func (err *reviewIntegrationPreflightError) Unwrap() error { return err.cause }

// classification returns the machine-branchable reason for this refusal,
// defaulting to the honest "correct the request you sent" shape.
func (err *reviewIntegrationPreflightError) classification() reviewPreflightReason {
	if err.reason != nil {
		return *err.reason
	}
	return reviewPreflightReason{
		Code: reviewIntegrationInvalidRequestCode, Message: reviewIntegrationGenericPreflightMessage,
		NextAction: "correct_request",
	}
}

func reviewPreflightError(err error) error {
	if err == nil {
		return nil
	}
	return &reviewIntegrationPreflightError{cause: err}
}

// reviewPreflightRefusal is reviewPreflightError with an explicit
// classification, for refusals whose precondition the caller must branch on.
func reviewPreflightRefusal(reason reviewPreflightReason, err error) error {
	if err == nil {
		return nil
	}
	return &reviewIntegrationPreflightError{cause: err, reason: &reason}
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
	if contract != ReviewIntegrationContractV1 && contract != ReviewIntegrationContractV2 {
		failure := newReviewIntegrationPreflightFailure(operation, "unsupported_contract", "The requested review integration contract is not supported.")
		failure.LineageID = safeReviewIntegrationLineage(operation, args[1:])
		return operation, true, &failure
	}
	return operation, true, nil
}

func reviewIntegrationContractArgument(args []string) (provided bool, value string, missing bool) {
	for index := 0; index < len(args); index++ {
		arg := args[index]
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
	if provided, contract, _ := reviewIntegrationContractArgument(args); provided && contract == ReviewIntegrationContractV2 {
		failure.Schema, failure.Contract = ReviewIntegrationFailureSchemaV2, ReviewIntegrationContractV2
	}
	failure.LineageID = safeReviewIntegrationLineage(operation, args)
	// Both branches below refuse inside authorizeReviewStart, strictly before
	// any review authority is created or mutated (organic-dx Phase 3b task
	// 3b.6). Falling through to the generic operation_outcome_unknown default
	// below would report MutationOutcome: unknown — a false safety claim for
	// an operation that provably never started. Typed here, they get the
	// stronger, correct not_started classification instead.
	if errors.Is(runErr, errReviewDeclinedForCandidate) {
		failure.Phase = "pre_native"
		failure.Code = "review_declined"
		failure.Message = "The operator declined the one-time review consent for this candidate; nothing was persisted."
		failure.MutationOutcome = ReviewMutationNotStarted
		failure.AuthorityApplicability = "not_evaluated"
		failure.RetrySafe = true
		failure.Replayability = reviewtransaction.ReplayabilityNotReplayable
		failure.RequiredInputs = []string{}
		// The decline is never latched, so re-running review.start for the
		// same candidate simply asks again.
		failure.NextAction = "review.start"
		return failure
	}
	var rddDisabled *reviewtransaction.RDDDisabledError
	if errors.As(runErr, &rddDisabled) {
		failure.Phase = "pre_native"
		failure.Code = "rdd_disabled"
		failure.Message = "Receipt-driven development is disabled; this operation never started."
		failure.MutationOutcome = ReviewMutationNotStarted
		failure.AuthorityApplicability = "not_evaluated"
		failure.RetrySafe = false
		failure.Replayability = reviewtransaction.ReplayabilityNotReplayable
		failure.RequiredInputs = []string{}
		// `next_action` is a closed vocabulary and none of its members is
		// "re-enable the kill switch", so it stays "stop": the operation really
		// must not be retried as issued. The runnable way out is not thrown away
		// for it -- the typed error already names the exact scoped
		// `review mode enable` command, and the additive `cause` field is where
		// that prose belongs, so a machine caller reading only this envelope is
		// not left with a terminal stop and nothing to run.
		failure.NextAction = "stop"
		failure.Cause = reviewIntegrationFailureCause(rddDisabled)
		return failure
	}
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
		// STATUS already re-derives retry eligibility for this lineage; the
		// denial itself named nothing to look at next.
		failure.NextAction = "review.status"
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
		remedy := reviewStartContextBoundRemedy(startContext.Cause)
		if !startContext.AuthoritySelected {
			failure.Phase = "pre_native"
			// The machine envelope carries the same numbers and the same way
			// out as the human error line. A consumer that only reads this
			// field would otherwise receive a bare code with nothing to act on.
			failure.Message = reviewStartContextFailureMessage("Frozen candidate context could not be rendered before START authority creation.", remedy)
			failure.MutationOutcome = ReviewMutationNotStarted
			failure.AuthorityApplicability = "not_evaluated"
			failure.RetrySafe = false
			failure.Replayability = reviewtransaction.ReplayabilityManualActionRequired
			failure.NextAction = "stop"
			return failure
		}
		failure.Phase = "native_committed"
		failure.Message = reviewStartContextFailureMessage("Frozen candidate context could not be rendered for the selected durable START authority.", remedy)
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
	// Transient advisory contention, proven non-mutating by where it is
	// produced rather than by how it reads (1861). ErrStoreLockContended is
	// only ever returned by the non-blocking acquisition syscall refusing an
	// already-held lock, so the guarded body never ran and this operation
	// wrote nothing under it. Every branch above already claimed any failure
	// that followed a committed native transition -- reviewFacadeOperationProgressError
	// and ClassifiedAuthorityRepairProgressError wrap those -- so a contention
	// error that reaches here has no durable native transition behind it.
	//
	// This is deliberately narrow. `operation_outcome_unknown` exists for
	// mutations that may or may not have landed, and a caller that retries one
	// of those can double-apply it; nothing here may widen to cover them.
	if errors.Is(runErr, reviewtransaction.ErrStoreLockContended) {
		failure.Phase = "pre_native"
		failure.Code = "authority_lock_contention"
		failure.Message = reviewLockOperationLabel(operation) +
			" lost a transient race for the authority lock; nothing was evaluated or written, so retry."
		failure.MutationOutcome = ReviewMutationNotStarted
		failure.AuthorityApplicability = "not_evaluated"
		failure.RetrySafe = true
		// Nothing durable exists to replay: `exact_replay_safe` in this
		// envelope means resuming a committed mutation, and this operation
		// committed none. The caller simply issues the same request again,
		// which is what `not_replayable` plus a retry route already means for
		// the read-only catch-all. Backoff rather than bare retry, because an
		// immediate re-drive from every loser is what amplifies contention.
		failure.Replayability = reviewtransaction.ReplayabilityNotReplayable
		failure.NextAction = "retry_with_bounded_backoff"
		return failure
	}
	// The other half of the same race, and the reason a concurrent FINALIZE can
	// lose without ever touching the lock contention above. The expected compact
	// revision is read before the authority lock is taken, so a writer that wins
	// the lock can still find that another writer advanced the authority in the
	// meantime. That window is intended -- the lock serializes the commit, the
	// compare-and-set detects the lost update -- but its refusal is not an
	// unknown outcome. CompactRevisionConflictError is produced only by the
	// compare-and-set standing in front of the guarded write, so this operation
	// provably published no successor, and the caller is owed that fact instead
	// of a trip to STATUS to rediscover it.
	//
	// Narrow for the same reason as the branch above: every failure that
	// followed a committed native transition was already claimed by the progress
	// branches, and the untyped ErrConcurrentUpdate sentinel -- which non-write
	// preconditions across the transaction package also report -- deliberately
	// still falls through to `operation_outcome_unknown`.
	var revisionConflict *reviewtransaction.CompactRevisionConflictError
	if errors.As(runErr, &revisionConflict) {
		failure.Phase = "pre_native"
		failure.Code = "authority_revision_conflict"
		failure.Message = reviewLockOperationLabel(operation) +
			" lost the authority revision to a concurrent writer; this operation published nothing, so retry."
		failure.MutationOutcome = ReviewMutationNotStarted
		// Unlike advisory contention, this operation did load and evaluate the
		// authority governing its target; what moved is that target's revision.
		failure.AuthorityApplicability = "current_target"
		failure.RetrySafe = true
		// Nothing durable exists to replay, exactly as for advisory contention:
		// the caller re-issues the same request, which re-reads the authority
		// the winner advanced. Backoff, because an immediate re-drive from every
		// loser is what amplifies the race.
		failure.Replayability = reviewtransaction.ReplayabilityNotReplayable
		failure.NextAction = "retry_with_bounded_backoff"
		if failure.LineageID == "" && validReviewIntegrationLineage(revisionConflict.LineageID) {
			failure.LineageID = revisionConflict.LineageID
		}
		// Both revisions are information the refusal holds; the unknown default
		// used to be the only branch that preserved them.
		failure.Cause = reviewIntegrationFailureCause(revisionConflict)
		return failure
	}
	var preLock *reviewtransaction.StoreLockPreAcquisitionError
	if errors.As(runErr, &preLock) {
		failure.Phase = "pre_native"
		failure.Code = "store_lock_unavailable"
		failure.Message = "The authoritative review store lock could not be acquired before review authority mutation: " + preLock.Error()
		failure.MutationOutcome = ReviewMutationNotStarted
		failure.AuthorityApplicability = "not_evaluated"
		// The lock is advisory contention, not damage — the same wait-and-
		// retry route authority_lock_timeout already names below, and the
		// LOCK diagnostics a caller needs live in STATUS.
		failure.RetrySafe = true
		failure.Replayability = reviewtransaction.ReplayabilityManualActionRequired
		failure.NextAction = "retry_with_bounded_backoff"
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
		// The refusal splits in two. `code`, `required_inputs`, and
		// `next_action` are the stable machine-branchable dimension and come
		// only from the closed vocabulary the published schema already
		// defines. The specific native reason is prose, so it travels in the
		// additive `cause` field -- privacy-gated and bounded, never
		// concatenated into the contract `message`. Cause is set here rather
		// than at the 79 refusal sites precisely so a refusal added later
		// cannot forget to name itself.
		reason := preflight.classification()
		preflightFailure := newReviewIntegrationPreflightFailure(operation, reason.Code, reason.Message)
		preflightFailure.Schema, preflightFailure.Contract = failure.Schema, failure.Contract
		preflightFailure.LineageID = failure.LineageID
		preflightFailure.RequiredInputs = append([]string{}, reason.RequiredInputs...)
		preflightFailure.NextAction = reason.NextAction
		preflightFailure.Cause = reviewIntegrationFailureCause(preflight)
		return preflightFailure
	}
	var legacy *reviewtransaction.LegacyReadOnlyError
	if errors.As(runErr, &legacy) {
		failure.Code = reviewtransaction.LegacyReadOnlyErrorCode
		// The non-negotiated START collision (review_facade.go) already names
		// this exact route; the negotiated envelope now carries the same one
		// instead of a bare stop.
		failure.Message = "Legacy v1 review authority is read-only and cannot be mutated: choose a new lineage for compact authority."
		failure.MutationOutcome = ReviewMutationNotStarted
		failure.AuthorityApplicability = "current_target"
		failure.Replayability = reviewtransaction.ReplayabilityNotReplayable
		failure.NextAction = "review.start"
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
		// The same typed failure now reaches this branch from the pre-PR
		// publication boundary as well as the pre-push upstream, so the
		// sentence names the input both gates need instead of one gate's name.
		failure.Message = "The publication target cannot be resolved; configure an upstream or pass --base-ref <remote>/<branch>."
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
			// No governing review exists yet for this target; review.start is
			// the exact way in, and an agent that only sees this failure
			// should not have to consult documentation to find it.
			failure.NextAction = "review.start"
		case ReviewReceiptUnrelated:
			// Every terminal receipt on file targets something else. STATUS
			// re-derives the same discovery and can name the candidate
			// lineages (or confirm none apply) without guessing.
			failure.NextAction = "review.status"
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
			if discovery.DeterministicallyStaleOnly {
				// organic-dx Phase 3c (community blocker + maintainer scope
				// extension): every contributing lineage is a
				// deterministically-typed stale receipt -- none of them
				// govern this candidate. The gate asks exactly one question:
				// does an approved receipt cover exactly these bytes? Old
				// lineages exist in that search only to NOT match. Blocking
				// stays correct (the candidate genuinely was never reviewed),
				// but the caller is told the truth instead of being sent to
				// disambiguate history: review the candidate, with a prior
				// lineage offered as optional recovery, never required.
				failure.AuthorityApplicability = "not_evaluated"
				failure.Message = "No terminal review receipt governs this candidate."
				if len(discovery.Candidates) > 0 {
					failure.Message += " A prior lineage is available for optional recovery instead of a fresh review: " + strings.Join(discovery.Candidates, ", ") + "."
				}
				failure.NextAction = "review.start"
				failure.RequiredInputs = []string{}
			} else {
				// len(exact) > 1, or an undecidable mixture
				// (assessmentUnknown/scopeWithoutContext): the gate genuinely
				// cannot pick, or could not even finish assessing every
				// candidate. Byte-identical to before Phase 3c.
				failure.AuthorityApplicability = "ambiguous"
				failure.RequiredInputs = []string{"lineage_id"}
				failure.NextAction = "review.status"
			}
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
		return failure
	}
	// The true operation_outcome_unknown default: no typed branch above
	// matched, and the operation mutates authority, so the caller cannot be
	// told a safe canned story. Code, Message, and MutationOutcome stay
	// byte-identical to the struct literal above; only the additive Cause
	// field carries the real native reason instead of discarding it.
	failure.Cause = reviewIntegrationFailureCause(runErr)
	return failure
}

// reviewIntegrationFailureCause renders a native error as the caller-visible
// `cause` prose. It reuses the same field-level privacy gate every other
// caller-visible narrative string passes through, so a refusal that wrapped an
// *os.PathError cannot turn the negotiated envelope into a path leak, and then
// bounds the result to the published single-line, 4000-character shape.
func reviewIntegrationFailureCause(err error) string {
	if err == nil {
		return ""
	}
	text := reviewScrubDefectReportField(strings.ReplaceAll(err.Error(), "\r", "\n"))
	if runes := []rune(text); len(runes) > reviewIntegrationFailureCauseLimit {
		text = strings.TrimSpace(string(runes[:reviewIntegrationFailureCauseLimit]))
	}
	return text
}

// reviewIntegrationFailureCauseLimit mirrors the published maxLength for
// failure.schema.json's `cause`.
const reviewIntegrationFailureCauseLimit = 4000

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
	legacyContract := failure.Schema == ReviewIntegrationFailureSchema && failure.Contract == ReviewIntegrationContractV1
	nativeGitContract := failure.Schema == ReviewIntegrationFailureSchemaV2 && failure.Contract == ReviewIntegrationContractV2
	if (!legacyContract && !nativeGitContract) ||
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
	if failure.Cause != "" && (strings.ContainsAny(failure.Cause, "\r\n") ||
		len([]rune(failure.Cause)) > reviewIntegrationFailureCauseLimit) {
		return errors.New("invalid negotiated review failure cause")
	}
	for _, input := range failure.RequiredInputs {
		if !supportedReviewIntegrationFailureInput(input) {
			return errors.New("unsupported negotiated review failure input")
		}
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
	Operation string                  `json:"operation"`
	LineageID string                  `json:"lineage_id"`
	State     reviewtransaction.State `json:"state"`
	Action    string                  `json:"action"`
	// Escalation carries the same correction-budget accounting sentence as
	// ReviewFacadeFinalizeResult.Escalation, so the negotiated and legacy
	// finalize surfaces explain a terminal escalation identically.
	Escalation        string                                       `json:"escalation,omitempty"`
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

func encodeReviewIntegrationOperation(stdout io.Writer, negotiated bool, operation string, legacyResult, publicResult any, contracts ...string) error {
	if !negotiated {
		return encodeReviewJSON(stdout, legacyResult)
	}
	payload, err := json.Marshal(publicResult)
	if err != nil {
		return fmt.Errorf("encode negotiated %s result: %w", operation, err)
	}
	schema, contract := ReviewIntegrationOperationSchema, ReviewIntegrationContractV1
	if len(contracts) > 0 && contracts[0] == ReviewIntegrationContractV2 {
		schema, contract = ReviewIntegrationOperationSchemaV2, ReviewIntegrationContractV2
	}
	envelope := ReviewIntegrationOperationResult{
		Schema: schema, Contract: contract,
		Operation: operation, Result: payload,
	}
	if err := envelope.Validate(); err != nil {
		return fmt.Errorf("validate negotiated %s result: %w", operation, err)
	}
	return encodeReviewJSON(stdout, envelope)
}

func (result ReviewIntegrationOperationResult) Validate() error {
	legacyContract := result.Schema == ReviewIntegrationOperationSchema && result.Contract == ReviewIntegrationContractV1
	nativeGitContract := result.Schema == ReviewIntegrationOperationSchemaV2 && result.Contract == ReviewIntegrationContractV2
	if (!legacyContract && !nativeGitContract) || len(result.Result) == 0 {
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
			correctionEvidenceFirst := transitionRequest == nil && finalized.ValidationRequest != nil &&
				(finalized.NextTransition.ReasonCode == "correction_repository_verification_required" ||
					finalized.NextTransition.ReasonCode == "correction_repository_tooling_failed")
			if !correctionEvidenceFirst && ((transitionRequest == nil) != (finalized.ValidationRequest == nil) ||
				transitionRequest != nil && !reflect.DeepEqual(*transitionRequest, *finalized.ValidationRequest)) {
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

// reviewIntegrationGatesInOrder is the single ordered source of truth for
// every valid --gate value. validReviewIntegrationGate and any refusal that
// must enumerate the valid set both derive from it, so the accepted values
// and the values a message names can never drift apart.
var reviewIntegrationGatesInOrder = []reviewtransaction.GateKind{
	reviewtransaction.GatePostApply, reviewtransaction.GatePreCommit, reviewtransaction.GatePrePush,
	reviewtransaction.GatePrePR, reviewtransaction.GateRelease,
}

func validReviewIntegrationGate(gate reviewtransaction.GateKind) bool {
	for _, valid := range reviewIntegrationGatesInOrder {
		if gate == valid {
			return true
		}
	}
	return false
}

// reviewIntegrationGateNames renders reviewIntegrationGatesInOrder as plain
// strings for refusal messages that must name the valid gate values.
func reviewIntegrationGateNames() []string {
	names := make([]string, len(reviewIntegrationGatesInOrder))
	for index, gate := range reviewIntegrationGatesInOrder {
		names[index] = string(gate)
	}
	return names
}
