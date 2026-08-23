package cli

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
)

// These tests prove each surviving collect-satisfying capture operation
// (`capture-result`, `capture-refuter`, `capture-validation`) emits one
// `gentle-ai.review-integration.failure/v2` envelope on stdout when it
// refuses. Machine callers can therefore classify the refusal as not started
// instead of mistaking empty stdout for an unknown mutation outcome.

// decodeCaptureRefusalEnvelope decodes the single stdout document a refused
// capture operation must emit and asserts the caller-facing typed error
// carries the same envelope.
func decodeCaptureRefusalEnvelope(t *testing.T, err error, output []byte) ReviewIntegrationFailure {
	t.Helper()
	if err == nil {
		t.Fatal("capture refusal returned success")
	}
	var typed *ReviewIntegrationFailureError
	if !errors.As(err, &typed) {
		t.Fatalf("capture refusal error is not a negotiated failure: %v", err)
	}
	if len(bytes.TrimSpace(output)) == 0 {
		t.Fatalf("capture refusal printed empty stdout; machine callers classify that as mutation outcome unknown: %v", err)
	}
	var failure ReviewIntegrationFailure
	decodeStrictReviewJSON(t, output, &failure)
	if failure.Code != typed.Failure.Code || failure.Operation != typed.Failure.Operation {
		t.Fatalf("stdout envelope %#v does not match typed error envelope %#v", failure, typed.Failure)
	}
	if failure.Schema != ReviewIntegrationFailureSchemaV2 || failure.Contract != ReviewIntegrationContractV2 {
		t.Fatalf("capture refusal envelope identity = %q/%q, want failure/v2", failure.Schema, failure.Contract)
	}
	return failure
}

// TestCaptureResultRefusalsEmitTypedFailureEnvelopes covers the reviewer-lens
// collection: the request-shape refusal takes the shared preflight code, and
// the binding refusal earns the typed capture_binding_mismatch whose
// continuation is a fresh STATUS.
func TestCaptureResultRefusalsEmitTypedFailureEnvelopes(t *testing.T) {
	reviewEnabledHome(t)
	t.Run("missing inputs", func(t *testing.T) {
		repo := initReviewCLIRepo(t)
		var output bytes.Buffer
		err := RunReview([]string{"capture-result", "--cwd", repo}, &output)
		failure := decodeCaptureRefusalEnvelope(t, err, output.Bytes())
		if failure.Operation != "review.capture-result" || failure.Phase != "preflight" ||
			failure.Code != "invalid_request" || failure.NextAction != "correct_request" ||
			failure.MutationOutcome != ReviewMutationNotStarted {
			t.Fatalf("missing-input refusal envelope = %#v", failure)
		}
		schema := compileWholePublishedReviewSchema(t, "v2", "failure.schema.json")
		validatePublishedReviewSchema(t, schema, output.Bytes())
	})
	t.Run("binding mismatch", func(t *testing.T) {
		const staleTarget = "sha256:6718ffa9d77c4965113517101482479c71763d40de8c366d2ebac11a367e6e1d"
		repo, started, _, record := newArtifactReview(t, false)
		input := filepath.Join(t.TempDir(), "result.json")
		if err := os.WriteFile(input, admittedReviewerPayloadForTest(t, repo, record, record.State.SelectedLenses[0], 0), 0o600); err != nil {
			t.Fatal(err)
		}
		var output bytes.Buffer
		err := RunReview([]string{
			"capture-result", "--cwd", repo, "--lineage", started.LineageID,
			"--target", staleTarget, "--lens", record.State.SelectedLenses[0], "--order", "0", "--input", input,
		}, &output)
		failure := decodeCaptureRefusalEnvelope(t, err, output.Bytes())
		if failure.Operation != "review.capture-result" || failure.Phase != "preflight" ||
			failure.Code != "capture_binding_mismatch" || failure.NextAction != "review.status" ||
			failure.MutationOutcome != ReviewMutationNotStarted || !failure.RetrySafe ||
			failure.LineageID != started.LineageID {
			t.Fatalf("binding refusal envelope = %#v", failure)
		}
		schema := compileWholePublishedReviewSchema(t, "v2", "failure.schema.json")
		validatePublishedReviewSchema(t, schema, output.Bytes())
	})
	t.Run("occupied slot", func(t *testing.T) {
		repo, started, _, record := newArtifactReview(t, true)
		if len(record.State.SelectedLenses) < 2 {
			t.Fatalf("selected lenses = %v, want multiple lenses for a nonterminal occupied slot", record.State.SelectedLenses)
		}
		first := filepath.Join(t.TempDir(), "first.json")
		if err := os.WriteFile(first, admittedReviewerPayloadForTest(t, repo, record, record.State.SelectedLenses[0], 0), 0o600); err != nil {
			t.Fatal(err)
		}
		binding := []string{
			"capture-result", "--cwd", repo, "--lineage", started.LineageID,
			"--target", record.State.InitialSnapshot.Identity,
			"--lens", record.State.SelectedLenses[0], "--order", "0",
		}
		if err := RunReview(append(binding, "--input", first), &bytes.Buffer{}); err != nil {
			t.Fatalf("capture first selected lens: %v", err)
		}
		conflicting := filepath.Join(t.TempDir(), "conflicting.json")
		payload := admittedReviewerPayloadForTest(t, repo, record, record.State.SelectedLenses[0], 0, "a different inspection narrative")
		if err := os.WriteFile(conflicting, payload, 0o600); err != nil {
			t.Fatal(err)
		}
		var output bytes.Buffer
		err := RunReview(append(binding, "--input", conflicting), &output)
		failure := decodeCaptureRefusalEnvelope(t, err, output.Bytes())
		if failure.Operation != "review.capture-result" || failure.Code != reviewerResultSlotOccupiedCode ||
			failure.NextAction != "review.status" || failure.MutationOutcome != ReviewMutationNotStarted {
			t.Fatalf("occupied-slot refusal envelope = %#v", failure)
		}
	})
}

// TestProviderRoleCaptureRefusalsEmitTypedFailureEnvelopes covers the two
// non-lens provider role collections. A request-shape refusal is the shared
// preflight code; what matters is that stdout carries the typed envelope
// instead of nothing.
func TestProviderRoleCaptureRefusalsEmitTypedFailureEnvelopes(t *testing.T) {
	for _, tt := range []struct {
		verb      string
		operation string
	}{
		{verb: "capture-refuter", operation: "review.capture-refuter"},
		{verb: "capture-validation", operation: "review.capture-validation"},
	} {
		t.Run(tt.verb, func(t *testing.T) {
			repo := initReviewCLIRepo(t)
			var output bytes.Buffer
			err := RunReview([]string{tt.verb, "--cwd", repo}, &output)
			failure := decodeCaptureRefusalEnvelope(t, err, output.Bytes())
			if failure.Operation != tt.operation || failure.Phase != "preflight" ||
				failure.Code != "invalid_request" || failure.NextAction != "correct_request" ||
				failure.MutationOutcome != ReviewMutationNotStarted {
				t.Fatalf("%s refusal envelope = %#v", tt.verb, failure)
			}
			schema := compileWholePublishedReviewSchema(t, "v2", "failure.schema.json")
			validatePublishedReviewSchema(t, schema, output.Bytes())
		})
	}
}

// TestCaptureRefusalKeepsOperatorErrorAndKillSwitchIdentity pins two behaviors
// the envelope emission must not change: the operator-facing error text stays
// (stderr keeps its human-readable line) and a kill-switch refusal keeps its
// typed identity for errors.Is dispatch.
func TestCaptureRefusalKeepsOperatorErrorAndKillSwitchIdentity(t *testing.T) {
	repo, _ := disabledReviewRepo(t, "review-capture-envelope-disabled")
	input := filepath.Join(t.TempDir(), "input.txt")
	if err := os.WriteFile(input, []byte("evidence\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	const digest = "sha256:0000000000000000000000000000000000000000000000000000000000000000"
	var output bytes.Buffer
	err := RunReview([]string{
		"capture-result", "--cwd", repo, "--lineage", "review-capture-envelope-disabled",
		"--target", digest, "--lens", "review-risk", "--order", "0", "--input", input,
	}, &output)
	if !errors.Is(err, reviewtransaction.ErrRDDDisabled) {
		t.Fatalf("kill-switch identity lost through envelope emission: %v", err)
	}
	failure := decodeCaptureRefusalEnvelope(t, err, output.Bytes())
	if failure.Code != "rdd_disabled" || failure.MutationOutcome != ReviewMutationNotStarted {
		t.Fatalf("kill-switch refusal envelope = %#v", failure)
	}
}

// TestCollectCaptureOperationsStayOffTheNegotiatedSurface pins the boundary:
// the capture operations join the failure envelope vocabulary without joining
// the published capabilities operations array (its length is a pinned
// contract) and without gaining a negotiated --contract route.
func TestCollectCaptureOperationsStayOffTheNegotiatedSurface(t *testing.T) {
	for _, operation := range []string{
		"review.capture-result", "review.capture-refuter", "review.capture-validation",
	} {
		metadata, known := reviewIntegrationOperationByName(operation)
		if !known {
			t.Fatalf("collect capture operation %q is not in the operation registry", operation)
		}
		if metadata.Negotiated {
			t.Fatalf("collect capture operation %q must not join the negotiated surface", operation)
		}
		if _, negotiated := reviewIntegrationOperationByCommand(metadata.Command); negotiated {
			t.Fatalf("collect capture command %q is routed as a negotiated command", metadata.Command)
		}
		for _, published := range reviewIntegrationOperationNames() {
			if published == operation {
				t.Fatalf("collect capture operation %q leaked into the published capabilities operations array", operation)
			}
		}
		if !validReviewIntegrationFailureOperation(operation) {
			t.Fatalf("collect capture operation %q is not admitted to the failure envelope vocabulary", operation)
		}
		lineage := safeReviewIntegrationLineage(operation, []string{"--lineage", "review-capture-lineage"})
		if lineage != "review-capture-lineage" {
			t.Fatalf("collect capture operation %q drops lineage from safe argument extraction: %q", operation, lineage)
		}
	}
}

// reviewEmitFailureWriter simulates a dead stdout (closed pipe, halted host
// relay) so envelope emission fails underneath a real native refusal.
type reviewEmitFailureWriter struct{ err error }

func (w reviewEmitFailureWriter) Write([]byte) (int, error) { return 0, w.err }

// TestCaptureRefusalEmitFailurePreservesNativeRefusal pins that a failed
// envelope emission never discards the refusal: the returned chain carries
// both errors, refusal primary, so errors.Is/As dispatch keeps working.
func TestCaptureRefusalEmitFailurePreservesNativeRefusal(t *testing.T) {
	emitErr := errors.New("stdout gone: broken pipe")
	err := RunReview([]string{"capture-result", "--cwd", initReviewCLIRepo(t)}, reviewEmitFailureWriter{err: emitErr})
	var typed *ReviewIntegrationFailureError
	if !errors.As(err, &typed) || typed.Failure.Code != "invalid_request" {
		t.Fatalf("native refusal lost when envelope emission failed: %v", err)
	}
	if !errors.Is(err, emitErr) {
		t.Fatalf("emit failure missing from the error chain: %v", err)
	}
	if !strings.HasPrefix(err.Error(), typed.Error()) {
		t.Fatalf("refusal is not the primary error: %q", err.Error())
	}
}
