package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/internal/reviewtransaction"
)

func TestNegotiatedReviewFailuresUseOneEnvelopeAcrossRoutes(t *testing.T) {
	tests := []struct {
		name      string
		args      []string
		operation string
	}{
		{name: "capabilities", args: []string{"capabilities", "unexpected"}, operation: "review.capabilities"},
		{name: "start", args: []string{"start", "--contract", ReviewIntegrationContractV1, "unexpected"}, operation: "review.start"},
		{name: "status", args: []string{"status", "--contract", ReviewIntegrationContractV1, "unexpected"}, operation: "review.status"},
		{name: "finalize", args: []string{"finalize", "--contract", ReviewIntegrationContractV1, "unexpected"}, operation: "review.finalize"},
		{name: "validate", args: []string{"validate", "--contract", ReviewIntegrationContractV1, "unexpected"}, operation: "review.validate"},
		{name: "bind sdd", args: []string{"bind-sdd", "--contract", ReviewIntegrationContractV1, "unexpected"}, operation: "review.bind_sdd"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var output bytes.Buffer
			err := RunReview(tt.args, &output)
			if err == nil {
				t.Fatal("negotiated invalid request succeeded")
			}
			failure := decodeReviewIntegrationFailure(t, output.Bytes())
			if failure.Operation != tt.operation || failure.Code != "invalid_request" ||
				failure.MutationOutcome != ReviewMutationNotStarted || failure.RetrySafe ||
				failure.Replayability != reviewtransaction.ReplayabilityNotReplayable {
				t.Fatalf("failure = %#v", failure)
			}
			var publicErr *ReviewIntegrationFailureError
			if !errors.As(err, &publicErr) {
				t.Fatalf("error = %T, want *ReviewIntegrationFailureError", err)
			}
			assertNoPrivateReviewOperationFields(t, output.Bytes())
		})
	}
}

func TestNegotiatedReviewContractFailuresArePreMutationAndLegacyErrorsStayCompatible(t *testing.T) {
	tests := []struct {
		name string
		args []string
		code string
	}{
		{name: "capabilities unsupported", args: []string{"capabilities", "--contract", "gentle-ai.review-integration/v2"}, code: "unsupported_contract"},
		{name: "start empty", args: []string{"start", "--contract="}, code: "empty_contract"},
		{name: "finalize malformed", args: []string{"finalize", "--contract"}, code: "invalid_request"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var output bytes.Buffer
			if err := RunReview(tt.args, &output); err == nil {
				t.Fatal("invalid contract request succeeded")
			}
			failure := decodeReviewIntegrationFailure(t, output.Bytes())
			if failure.Code != tt.code || failure.MutationOutcome != ReviewMutationNotStarted {
				t.Fatalf("failure = %#v", failure)
			}
		})
	}

	var legacy bytes.Buffer
	err := RunReview([]string{"start", "unexpected"}, &legacy)
	if err == nil || legacy.Len() != 0 {
		t.Fatalf("legacy invalid request = output %q, error %v", legacy.String(), err)
	}
	var publicErr *ReviewIntegrationFailureError
	if errors.As(err, &publicErr) {
		t.Fatalf("unnegotiated error became negotiated: %v", err)
	}
}

func TestNegotiatedLegacyReadOnlyFailurePreservesTypedCauseAcrossMutationRoutes(t *testing.T) {
	tests := []struct {
		name              string
		contractOperation string
		legacyOperation   string
	}{
		{name: "start collision", contractOperation: "review.start", legacyOperation: "review/start"},
		{name: "finalize", contractOperation: "review.finalize", legacyOperation: "review/finalize"},
		{name: "review step", contractOperation: "review.finalize", legacyOperation: "review/freeze-findings"},
		{name: "invalidate", contractOperation: "review.finalize", legacyOperation: "review/invalidate"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lineage := "legacy-negotiated-" + strings.ReplaceAll(tt.name, " ", "-")
			typed := reviewtransaction.NewLegacyReadOnlyError(tt.legacyOperation, lineage)
			secret := "/tmp/private-authority token=secret"
			runErr := fmt.Errorf("%s: %w", secret, typed)
			failure := newReviewIntegrationFailure(tt.contractOperation, []string{"--lineage", lineage}, runErr)
			if err := failure.Validate(); err != nil {
				t.Fatal(err)
			}
			if failure.Code != reviewtransaction.LegacyReadOnlyErrorCode || failure.MutationOutcome != ReviewMutationNotStarted ||
				failure.RetrySafe || failure.Replayability != reviewtransaction.ReplayabilityNotReplayable ||
				failure.NextAction != "stop" || strings.Contains(failure.Message, secret) || strings.Contains(failure.Message, "/tmp/") {
				t.Fatalf("legacy negotiated failure = %#v", failure)
			}
			publicErr := newReviewIntegrationFailureError(failure, runErr)
			var preserved *reviewtransaction.LegacyReadOnlyError
			if !errors.Is(publicErr, reviewtransaction.ErrLegacyReadOnly) || !errors.As(publicErr, &preserved) || preserved != typed {
				t.Fatalf("negotiated wrapper lost typed legacy cause: %#v", publicErr)
			}
		})
	}
}

func TestNegotiatedGateDenialUsesFailureEnvelopeWithoutAuthorityDrift(t *testing.T) {
	repo := initReviewCLIRepo(t)
	writeNegotiatedOperationChange(t, repo, "thin")
	lineage := "review-failure-gate"
	_, store := finalizeNegotiatedOperationFixture(t, repo, lineage, true)
	beforeAuthority := readReviewOperationFile(t, store.StatePath())
	beforeReceipt := readReviewOperationFile(t, store.ReceiptPath())
	if err := os.WriteFile(filepath.Join(repo, "openspec", "changes", "thin", "proposal.md"), []byte("# Drifted proposal\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	var output bytes.Buffer
	err := RunReview([]string{
		"validate", "--contract", ReviewIntegrationContractV1, "--cwd", repo, "--lineage", lineage,
		"--gate", string(reviewtransaction.GatePostApply),
	}, &output)
	if err == nil {
		t.Fatal("drifted target passed negotiated validation")
	}
	failure := decodeReviewIntegrationFailure(t, output.Bytes())
	if failure.Code != "gate_scope_changed" || failure.MutationOutcome != ReviewMutationNotStarted ||
		!failure.RetrySafe || failure.Replayability != reviewtransaction.ReplayabilityManualActionRequired ||
		failure.NextAction != "explicit-maintainer-action" {
		t.Fatalf("gate failure = %#v", failure)
	}
	if !bytes.Equal(beforeAuthority, readReviewOperationFile(t, store.StatePath())) ||
		!bytes.Equal(beforeReceipt, readReviewOperationFile(t, store.ReceiptPath())) {
		t.Fatal("negotiated gate denial changed authority or receipt bytes")
	}
}

func TestNegotiatedReceiptPublicationFailureIsSanitizedAndExactlyReplayable(t *testing.T) {
	repo := initReviewCLIRepo(t)
	writeNegotiatedOperationChange(t, repo, "thin")
	started := startFacadeReview(t, repo)
	store, err := reviewtransaction.CompactAuthoritativeStore(context.Background(), repo, started.LineageID)
	if err != nil {
		t.Fatal(err)
	}
	if err := RunReviewFacadeFinalize([]string{"--cwd", repo, "--lineage", started.LineageID}, io.Discard); err != nil {
		t.Fatal(err)
	}
	evidence := filepath.Join(t.TempDir(), "evidence.txt")
	if err := os.WriteFile(evidence, []byte("focused tests pass\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	original := writeCompactFacadeReceipt
	secret := "raw provider stderr token=secret /tmp/authority.lock"
	writeCompactFacadeReceipt = func(string, reviewtransaction.CompactReceipt) error { return errors.New(secret) }
	var output bytes.Buffer
	err = RunReview([]string{
		"finalize", "--contract", ReviewIntegrationContractV1, "--cwd", repo,
		"--lineage", started.LineageID, "--evidence", evidence,
	}, &output)
	writeCompactFacadeReceipt = original
	if err == nil {
		t.Fatal("receipt publication interruption succeeded")
	}
	failure := decodeReviewIntegrationFailure(t, output.Bytes())
	if failure.Code != "receipt_publication_pending" || failure.MutationOutcome != ReviewMutationCommitted ||
		failure.Replayability != reviewtransaction.ReplayabilityExactReplaySafe || failure.RetrySafe ||
		failure.LineageID != started.LineageID || !strings.HasPrefix(failure.RequestDigest, "sha256:") ||
		failure.NextAction != "review.finalize" {
		t.Fatalf("receipt failure = %#v", failure)
	}
	if strings.Contains(output.String(), secret) || strings.Contains(err.Error(), secret) ||
		strings.Contains(output.String(), "/tmp/") || strings.Contains(output.String(), "token=secret") {
		t.Fatalf("negotiated failure leaked private diagnostics: output=%s error=%v", output.String(), err)
	}
	pending, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	pendingAuthority := readReviewOperationFile(t, store.StatePath())
	if _, err := os.Stat(store.ReceiptPath()); !os.IsNotExist(err) {
		t.Fatalf("failed publication materialized receipt: %v", err)
	}

	output.Reset()
	if err := RunReview([]string{
		"finalize", "--contract", ReviewIntegrationContractV1, "--cwd", repo, "--lineage", started.LineageID,
	}, &output); err != nil {
		t.Fatalf("exact negotiated receipt replay: %v\n%s", err, output.String())
	}
	after, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if after.Revision != pending.Revision || !bytes.Equal(pendingAuthority, readReviewOperationFile(t, store.StatePath())) {
		t.Fatal("exact receipt replay changed authority identity or bytes")
	}
	if _, err := os.Stat(store.ReceiptPath()); err != nil {
		t.Fatalf("exact receipt replay did not publish receipt: %v", err)
	}
}

func TestReviewIntegrationFailureSchemaAndFixtureAreStrict(t *testing.T) {
	root := filepath.Join("..", "..", "contracts", "review-integration", "v1")
	schemaPayload, err := os.ReadFile(filepath.Join(root, "schemas", "failure.schema.json"))
	if err != nil {
		t.Fatal(err)
	}
	var schema map[string]any
	if err := json.Unmarshal(schemaPayload, &schema); err != nil {
		t.Fatal(err)
	}
	if schema["$schema"] != "https://json-schema.org/draft/2020-12/schema" ||
		schema["$id"] != ReviewIntegrationFailureSchemaID || schema["additionalProperties"] != false {
		t.Fatalf("failure schema header = %#v", schema)
	}
	fixture, err := os.ReadFile(filepath.Join(root, "fixtures", "failure.fixture.json"))
	if err != nil {
		t.Fatal(err)
	}
	failure := decodeReviewIntegrationFailure(t, fixture)
	if failure.Code != "receipt_publication_pending" {
		t.Fatalf("failure fixture = %#v", failure)
	}
	var raw map[string]any
	if err := json.Unmarshal(fixture, &raw); err != nil {
		t.Fatal(err)
	}
	raw["unknown"] = true
	malformed, err := json.Marshal(raw)
	if err != nil {
		t.Fatal(err)
	}
	decoder := json.NewDecoder(bytes.NewReader(malformed))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&ReviewIntegrationFailure{}); err == nil {
		t.Fatal("strict failure decoder accepted an unknown field")
	}
}

func decodeReviewIntegrationFailure(t *testing.T, payload []byte) ReviewIntegrationFailure {
	t.Helper()
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	var failure ReviewIntegrationFailure
	if err := decoder.Decode(&failure); err != nil {
		t.Fatalf("decode failure envelope %q: %v", payload, err)
	}
	if err := failure.Validate(); err != nil {
		t.Fatalf("validate failure envelope: %v\n%s", err, payload)
	}
	return failure
}
