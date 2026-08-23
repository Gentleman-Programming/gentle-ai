package cli

import (
	"errors"
	"testing"
)

func TestCaptureResultPreflightRefusalUsesCurrentOperation(t *testing.T) {
	failure := newReviewIntegrationFailure(reviewCaptureResultCaptureOperation, nil,
		reviewPreflightRefusal(reviewPreflightCaptureBindingMismatchReason, errors.New("stale capture binding")))
	if failure.Operation != reviewCaptureResultCaptureOperation || failure.Code != reviewPreflightCaptureBindingMismatchReason.Code ||
		failure.MutationOutcome != ReviewMutationNotStarted || failure.NextAction != "review.status" {
		t.Fatalf("capture preflight failure = %#v", failure)
	}
}

func TestStartPreflightRefusalUsesGenericInvalidRequest(t *testing.T) {
	failure := newReviewIntegrationFailure("review.start", nil, reviewPreflightError(errors.New("missing target")))
	if failure.Code != reviewIntegrationInvalidRequestCode || failure.MutationOutcome != ReviewMutationNotStarted ||
		failure.NextAction != "correct_request" {
		t.Fatalf("start preflight failure = %#v", failure)
	}
}
