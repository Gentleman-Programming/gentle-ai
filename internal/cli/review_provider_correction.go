package cli

import (
	"bytes"
	"context"
	"fmt"

	"github.com/gentleman-programming/gentle-ai/v2/internal/model"
	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewerprovider"
	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
)

// Every in-process reviewer runtime returns free text, and one out-of-schema
// nested field or one truncated array used to end the capture with the only
// exit being a relaunch of the same slot that failed the same way (issues
// #3942, #2791, #4061). The runtime never learned what was wrong. The capture
// now grants exactly one corrective re-invocation whose prompt is the original
// materialized prompt plus the exact admission error, so the model can see the
// census of what it left open. The bound is a named constant, the retry lives
// only here, and it never applies to --input submissions or the OpenCode host
// relay: those hosts own their reviewer and receive the same preserved
// payload instead.
const (
	maxReviewerResultAdmissionAttempts     = 2
	reviewProviderCorrectiveFeedbackHeader = "GENTLE_AI_REVIEW_ADMISSION_FEEDBACK"
)

// reviewProviderCapture is everything one in-process lens capture needs to
// invoke, admit, preserve, and name its continuation.
type reviewProviderCapture struct {
	root    string
	runtime model.AgentID
	adapter reviewerprovider.Adapter
	state   reviewtransaction.CompactState
	frozen  reviewtransaction.FrozenCandidateContext
	subject reviewtransaction.ArtifactSubject
}

func (capture reviewProviderCapture) admit(ctx context.Context, raw []byte) (reviewProviderAdmittedResult, error) {
	return reviewProviderAdmitRaw(ctx, capture.root, capture.state, capture.state.CapturePhaseRevision, capture.frozen, capture.subject, raw)
}

func (capture reviewProviderCapture) preserve(ctx context.Context, attempt int, admission error, raw []byte) string {
	return reviewRejectedResultClause(ctx, capture.root, reviewRejectedResultMeta{
		LineageID: capture.state.LineageID, Lens: capture.subject.Lens, Attempt: attempt, Reason: admission.Error(),
	}, raw)
}

func (capture reviewProviderCapture) continuation() string {
	return reviewProviderCaptureContinuation(capture.runtime, capture.state.LineageID)
}

// reviewProviderCaptureContinuation names the exact STATUS re-query every
// capture role points a caller at once its bound slot is reoffered.
func reviewProviderCaptureContinuation(runtime model.AgentID, lineageID string) string {
	return fmt.Sprintf("gentle-ai review status --cwd <repo> --contract %s --agent %s --lineage %s --next-transition", ReviewIntegrationContractV2, runtime, lineageID)
}

// reviewProviderCaptureRefusedError wraps the final refusal after both
// admission attempts failed, so a caller can classify it as a provider-side
// defect -- the provider returned malformed output twice -- distinctly from a
// transport failure or a role-specific sentinel such as an inconclusive
// targeted validation.
type reviewProviderCaptureRefusedError struct{ cause error }

func (err *reviewProviderCaptureRefusedError) Error() string { return err.cause.Error() }
func (err *reviewProviderCaptureRefusedError) Unwrap() error { return err.cause }

// reviewProviderCaptureWithOneCorrection invokes the reviewer, admits its raw
// bytes, and on an admission failure invokes it once more with feedback. A
// transport failure is not an admission failure and is returned as is.
func reviewProviderCaptureWithOneCorrection(ctx context.Context, capture reviewProviderCapture, invocation reviewerprovider.Invocation) (reviewProviderAdmittedResult, []byte, error) {
	return reviewProviderCaptureRetry(ctx, capture.adapter, invocation, capture.admit, capture.preserve, capture.continuation, nil)
}

// reviewProviderCaptureRetryable reports whether an admission failure should
// consume the single corrective re-invocation. A nil predicate always
// retries, matching the lens role. The targeted validator role opts a
// sentinel out of it: an inconclusive verdict already owns its own retry
// ladder (relaunch after the validator regains tree access, not an in-process
// correction), so routing it through this mechanism would consume the wrong
// budget for the wrong reason.
type reviewProviderCaptureRetryable func(error) bool

// reviewProviderCaptureRetry is the shared corrective re-invocation core every
// in-process provider role capture uses: invoke, admit, and on one admission
// failure that the role considers retryable, invoke once more with the exact
// admission error appended to the original prompt. Both rejected payloads are
// preserved outside the authority store. It is parameterized by the role's
// admitted result type so the lens, refuter, and targeted validator roles can
// each keep their own native admission and durable-capture logic while
// sharing exactly this retry shape.
func reviewProviderCaptureRetry[T any](
	ctx context.Context,
	adapter reviewerprovider.Adapter,
	invocation reviewerprovider.Invocation,
	admit func(ctx context.Context, raw []byte) (T, error),
	preserve func(ctx context.Context, attempt int, admission error, raw []byte) string,
	continuation func() string,
	retryable reviewProviderCaptureRetryable,
) (T, []byte, error) {
	var zero T
	raw, err := adapter.Review(ctx, invocation)
	if err != nil {
		return zero, nil, fmt.Errorf("invoke provider reviewer: %w", err)
	}
	admitted, firstErr := admit(ctx, raw)
	if firstErr == nil {
		return admitted, raw, nil
	}
	if retryable != nil && !retryable(firstErr) {
		return zero, raw, firstErr
	}
	firstClause := preserve(ctx, 1, firstErr, raw)
	corrective := reviewProviderCorrectivePrompt(invocation.Prompt(), firstErr)
	if len(corrective) > reviewLensContextByteBudget {
		return zero, raw, &reviewProviderCaptureRefusedError{cause: fmt.Errorf("%w%s; the corrective re-invocation was skipped because its prompt exceeds the native reviewer context budget; re-query %s and run the reoffered capture", firstErr, firstClause, continuation())}
	}
	correctiveRaw, err := adapter.Review(ctx, reviewerprovider.NewInvocation(corrective))
	if err != nil {
		return zero, nil, fmt.Errorf("invoke provider reviewer on corrective attempt %d of %d: %w (attempt 1 was refused: %v%s)", maxReviewerResultAdmissionAttempts, maxReviewerResultAdmissionAttempts, err, firstErr, firstClause)
	}
	admitted, secondErr := admit(ctx, correctiveRaw)
	if secondErr == nil {
		return admitted, correctiveRaw, nil
	}
	if retryable != nil && !retryable(secondErr) {
		return zero, correctiveRaw, secondErr
	}
	secondClause := preserve(ctx, maxReviewerResultAdmissionAttempts, secondErr, correctiveRaw)
	return zero, correctiveRaw, &reviewProviderCaptureRefusedError{cause: fmt.Errorf("provider reviewer result was refused on both admission attempts: attempt 1: %v%s; corrective attempt %d: %w%s; re-query %s and run the reoffered capture", firstErr, firstClause, maxReviewerResultAdmissionAttempts, secondErr, secondClause, continuation())}
}

// reviewProviderCorrectivePrompt appends the Go-owned feedback section to the
// original materialized prompt. The section quotes the exact admission error
// -- for a malformed payload this is the structural census naming what was
// left open -- and restates the two hard framing rules every provider role
// shares plus a schema-field rule that stays true for every role because it
// names the schema already declared earlier in the same prompt rather than
// restating one role's fields.
func reviewProviderCorrectivePrompt(original []byte, admission error) []byte {
	var prompt bytes.Buffer
	prompt.Write(original)
	prompt.WriteString("\n\n" + reviewProviderCorrectiveFeedbackHeader + "\n")
	fmt.Fprintf(&prompt, "Your previous result for this exact binding was rejected and nothing was admitted. Admission error: %s\n", admission)
	prompt.WriteString("This is the single corrective attempt. Three hard rules:\n")
	prompt.WriteString("1. Return exactly one JSON object and nothing else: no prose, no code fence, no second object.\n")
	prompt.WriteString("2. Close every object and array; a truncated payload is rejected.\n")
	prompt.WriteString("3. Use only the fields declared in this prompt's output schema; any other field is rejected.\n")
	prompt.WriteString(reviewProviderCorrectiveFeedbackHeader + "_END\n")
	return prompt.Bytes()
}
