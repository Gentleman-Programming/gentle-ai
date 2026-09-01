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
// #3942, #2791). The runtime never learned what was wrong. The capture now
// grants exactly one corrective re-invocation whose prompt is the original
// materialized prompt plus the exact admission error and the three rules the
// result broke. The bound is a named constant, the retry lives only here, and
// it never applies to --input submissions or the OpenCode host relay: those
// hosts own their reviewer and receive the same preserved payload instead.
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
	return fmt.Sprintf("gentle-ai review status --cwd <repo> --contract %s --agent %s --lineage %s --next-transition", ReviewIntegrationContractV2, capture.runtime, capture.state.LineageID)
}

// reviewProviderCaptureWithOneCorrection invokes the reviewer, admits its raw
// bytes, and on an admission failure invokes it once more with feedback. A
// transport failure is not an admission failure and is returned as is.
func reviewProviderCaptureWithOneCorrection(ctx context.Context, capture reviewProviderCapture, invocation reviewerprovider.Invocation) (reviewProviderAdmittedResult, []byte, error) {
	raw, err := capture.adapter.Review(ctx, invocation)
	if err != nil {
		return reviewProviderAdmittedResult{}, nil, fmt.Errorf("invoke provider reviewer: %w", err)
	}
	admitted, firstErr := capture.admit(ctx, raw)
	if firstErr == nil {
		return admitted, raw, nil
	}
	firstClause := capture.preserve(ctx, 1, firstErr, raw)
	corrective := reviewProviderCorrectivePrompt(invocation.Prompt(), firstErr)
	if len(corrective) > reviewLensContextByteBudget {
		return reviewProviderAdmittedResult{}, nil, fmt.Errorf("%w%s; the corrective re-invocation was skipped because its prompt exceeds the native reviewer context budget; re-query %s and run the reoffered capture", firstErr, firstClause, capture.continuation())
	}
	raw, err = capture.adapter.Review(ctx, reviewerprovider.NewInvocation(corrective))
	if err != nil {
		return reviewProviderAdmittedResult{}, nil, fmt.Errorf("invoke provider reviewer on corrective attempt %d of %d: %w (attempt 1 was refused: %v%s)", maxReviewerResultAdmissionAttempts, maxReviewerResultAdmissionAttempts, err, firstErr, firstClause)
	}
	admitted, secondErr := capture.admit(ctx, raw)
	if secondErr == nil {
		return admitted, raw, nil
	}
	secondClause := capture.preserve(ctx, maxReviewerResultAdmissionAttempts, secondErr, raw)
	return reviewProviderAdmittedResult{}, nil, fmt.Errorf("provider reviewer result was refused on both admission attempts for lens %q: attempt 1: %v%s; corrective attempt %d: %w%s; re-query %s and run the reoffered capture", capture.subject.Lens, firstErr, firstClause, maxReviewerResultAdmissionAttempts, secondErr, secondClause, capture.continuation())
}

// reviewProviderCorrectivePrompt appends the Go-owned feedback section to the
// original materialized prompt. The section quotes the exact admission error
// and restates the three rules every rejected result so far has broken.
func reviewProviderCorrectivePrompt(original []byte, admission error) []byte {
	var prompt bytes.Buffer
	prompt.Write(original)
	prompt.WriteString("\n\n" + reviewProviderCorrectiveFeedbackHeader + "\n")
	fmt.Fprintf(&prompt, "Your previous result for this exact binding was rejected and nothing was admitted. Admission error: %s\n", admission)
	prompt.WriteString("This is the single corrective attempt. Three hard rules:\n")
	prompt.WriteString("1. Return exactly one JSON object and nothing else: no prose, no code fence, no second object.\n")
	prompt.WriteString("2. Close every object and array; a truncated payload is rejected.\n")
	prompt.WriteString("3. Use only the fields declared in the " + reviewLensContextResultSchema + " block: \"inspection\" accepts only \"status\" and \"paths\"; \"lens\" is allowed only at the top level and inside findings; any other field is rejected.\n")
	prompt.WriteString(reviewProviderCorrectiveFeedbackHeader + "_END\n")
	return prompt.Bytes()
}
