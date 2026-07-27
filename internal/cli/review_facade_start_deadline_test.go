package cli

import (
	"bytes"
	"context"
	"io"
	"testing"
	"time"
)

// TestNegotiatedStartSurvivesSharedDeadline is the deterministic proof for
// issue-1778's headroom invariant: a negotiated review.start whose native work
// outlives the shared facade deadline still completes, because RunReview
// selects the start-scoped deadline for it, while every other negotiated
// operation subjected to the exact same delay is cut off by the shared
// deadline. The e2e large-workspace journey
// (TestOrganicReviewStartDeadlineHeadroom) no longer needs to burn real wall
// clock past 25s to prove this.
func TestNegotiatedStartSurvivesSharedDeadline(t *testing.T) {
	originalRunner := reviewFacadeCommandRunner
	originalTimeout := reviewFacadeOperationTimeout
	originalStartTimeout := reviewFacadeStartOperationTimeout
	t.Cleanup(func() {
		reviewFacadeCommandRunner = originalRunner
		reviewFacadeOperationTimeout = originalTimeout
		reviewFacadeStartOperationTimeout = originalStartTimeout
	})
	reviewFacadeOperationTimeout = 100 * time.Millisecond
	reviewFacadeStartOperationTimeout = 2 * time.Second
	reviewFacadeCommandRunner = func(_ context.Context, _ []string, stdout io.Writer) error {
		time.Sleep(500 * time.Millisecond)
		_, err := io.WriteString(stdout, "native start completed\n")
		return err
	}

	var startOutput bytes.Buffer
	err := RunReview([]string{"start", "--contract", ReviewIntegrationContractV1, "--lineage", "start-deadline-headroom"}, &startOutput)
	if err != nil {
		t.Fatalf("negotiated start outliving the shared deadline failed under the start-scoped deadline: %v\n%s", err, startOutput.String())
	}
	if startOutput.String() != "native start completed\n" {
		t.Fatalf("negotiated start output = %q, want the native worker's complete output", startOutput.String())
	}

	// Sanity: a non-start negotiated operation under the exact same native
	// delay is killed by the shared deadline, so the start success above can
	// only come from the start-scoped selection.
	var statusOutput bytes.Buffer
	err = RunReview([]string{"status", "--contract", ReviewIntegrationContractV1, "--lineage", "start-deadline-headroom"}, &statusOutput)
	if err == nil {
		t.Fatalf("negotiated status outliving the shared deadline succeeded: %s", statusOutput.String())
	}
	failure := decodeReviewIntegrationFailure(t, statusOutput.Bytes())
	if failure.Code != "operation_timeout" {
		t.Fatalf("negotiated status failure code = %q, want operation_timeout from the shared deadline", failure.Code)
	}
}
