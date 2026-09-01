package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
)

func TestRetiredBindSDDRejectsBothPublishedContractsWithoutAnEnvelope(t *testing.T) {
	for _, contract := range []string{ReviewIntegrationContractV1, ReviewIntegrationContractV2} {
		t.Run(contract, func(t *testing.T) {
			var output bytes.Buffer
			err := RunReview([]string{"bind-sdd", "--contract", contract, "--cwd", t.TempDir()}, &output)
			if err == nil || !strings.Contains(err.Error(), "unknown review command") {
				t.Fatalf("retired bind-sdd error = %v", err)
			}
			if output.Len() != 0 {
				t.Fatalf("retired bind-sdd emitted an operation envelope: %q", output.String())
			}
		})
	}
}

func TestNegotiatedReviewOperationsRejectInvalidContractsBeforeMutation(t *testing.T) {
	reviewEnabledHome(t)
	for _, contract := range []string{"", "gentle-ai.review-integration/v3"} {
		t.Run("validate_"+contract, func(t *testing.T) {
			repo := initReviewCLIRepo(t)
			writeNegotiatedOperationChange(t, repo, "thin")
			lineage := "review-invalid-read-boundary"
			fixture := seedHistoricalCompatibilityApprovedCompactReceipt(t, repo, lineage, reviewtransaction.Target{
				Kind: reviewtransaction.TargetCurrentChanges, Projection: reviewtransaction.ProjectionWorkspace, IntendedUntracked: []string{},
			})
			store := fixture.Store
			for _, call := range []struct {
				name string
				args []string
			}{
				{name: "validate", args: []string{"validate", "--contract=" + contract, "--cwd", repo, "--lineage", lineage, "--gate", string(reviewtransaction.GatePostApply)}},
			} {
				var output bytes.Buffer
				err := RunReview(call.args, &output)
				if err == nil {
					t.Fatalf("invalid %s contract %q = %q, %v", call.name, contract, output.String(), err)
				}
				failure := decodeReviewIntegrationFailure(t, output.Bytes())
				wantCode := "unsupported_contract"
				if contract == "" {
					wantCode = "empty_contract"
				}
				if failure.Code != wantCode || failure.MutationOutcome != ReviewMutationNotStarted {
					t.Fatalf("invalid %s contract %q failure = %#v", call.name, contract, failure)
				}
			}
			assertArtifactRevision(t, store, fixture.Record.Revision)
			if _, err := os.Stat(reviewOperationBindingPath(store, "thin")); !os.IsNotExist(err) {
				t.Fatalf("invalid contract published runtime state: %v", err)
			}
		})
	}
}

func TestNegotiatedReviewOperationSchemaAndFixturesAreStrict(t *testing.T) {
	root := filepath.Join("..", "..", "contracts", "review-integration", "v1")
	schemaPayload, err := os.ReadFile(filepath.Join(root, "schemas", "operation.schema.json"))
	if err != nil {
		t.Fatal(err)
	}
	var schema map[string]any
	if err := json.Unmarshal(schemaPayload, &schema); err != nil {
		t.Fatal(err)
	}
	if schema["$schema"] != "https://json-schema.org/draft/2020-12/schema" ||
		schema["$id"] != ReviewIntegrationOperationSchemaID || schema["additionalProperties"] != false {
		t.Fatalf("operation schema header = %#v", schema)
	}
	payload, err := os.ReadFile(filepath.Join(root, "fixtures", "operation.fixture.json"))
	if err != nil {
		t.Fatal(err)
	}
	envelope := decodeReviewOperationEnvelope(t, payload)
	if err := envelope.Validate(); err != nil {
		t.Fatal(err)
	}
	assertNoPrivateReviewOperationFields(t, payload)
}

func startReviewOperationFixture(t *testing.T, repo, lineage string) ReviewFacadeStartResult {
	t.Helper()
	startedBytes, err := runLegacyFacadeStartForTestBytes(t, []string{"--cwd", repo, "--lineage", lineage})
	if err != nil {
		t.Fatal(err)
	}
	var started ReviewFacadeStartResult
	decodeStrictReviewJSON(t, startedBytes, &started)
	return started
}

func writeNegotiatedOperationChange(t *testing.T, repo, change string) {
	t.Helper()
	for path, content := range map[string]string{
		"tasks.md": "- [x] 1.1 Done\n", "proposal.md": "# Proposal\n", "design.md": "# Design\n", "specs/binding/spec.md": "# Spec\n",
	} {
		writeReviewStartCandidate(t, repo, "openspec/changes/"+change+"/"+path, content, 0o644)
	}
}

func decodeReviewOperationEnvelope(t *testing.T, payload []byte) ReviewIntegrationOperationResult {
	t.Helper()
	var envelope ReviewIntegrationOperationResult
	decodeStrictReviewJSON(t, payload, &envelope)
	if err := envelope.Validate(); err != nil {
		t.Fatal(err)
	}
	return envelope
}

func decodeStrictReviewJSON(t *testing.T, payload []byte, target any) {
	t.Helper()
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		t.Fatal(err)
	}
}

func strictReviewJSONFields(t *testing.T, payload []byte) []string {
	t.Helper()
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(payload, &fields); err != nil {
		t.Fatal(err)
	}
	names := make([]string, 0, len(fields))
	for name := range fields {
		names = append(names, name)
	}
	sortStrings(names)
	return names
}

func assertNoPrivateReviewOperationFields(t *testing.T, payload []byte) {
	t.Helper()
	var document any
	if err := json.Unmarshal(payload, &document); err != nil {
		t.Fatal(err)
	}
	forbidden := map[string]struct{}{
		"model": {}, "provider": {}, "profile": {}, "cwd": {}, "repository": {}, "store_path": {}, "receipt_path": {}, "binding_path": {},
	}
	if field := findCapabilityForbiddenField(document, forbidden); field != "" {
		t.Fatalf("negotiated operation exposed private field %q: %s", field, payload)
	}
}

func readReviewOperationFile(t *testing.T, path string) []byte {
	t.Helper()
	payload, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return payload
}

func reviewOperationBindingPath(store reviewtransaction.CompactStore, change string) string {
	common := filepath.Dir(filepath.Dir(filepath.Dir(filepath.Dir(store.Dir))))
	return filepath.Join(common, "gentle-ai", "sdd-runtime", "v1", change, "HEAD")
}
