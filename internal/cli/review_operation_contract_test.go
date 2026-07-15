package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/internal/reviewtransaction"
	"github.com/gentleman-programming/gentle-ai/internal/sddstatus"
)

func TestNegotiatedReviewFinalizePreservesLegacyResultAndCanonicalIdentities(t *testing.T) {
	legacyRepo := initReviewCLIRepo(t)
	negotiatedRepo := initReviewCLIRepo(t)
	for _, repo := range []string{legacyRepo, negotiatedRepo} {
		writeNegotiatedOperationChange(t, repo, "thin")
	}
	lineage := "review-operation-finalize"
	legacyOutput, legacyStore := finalizeNegotiatedOperationFixture(t, legacyRepo, lineage, false)
	negotiatedOutput, negotiatedStore := finalizeNegotiatedOperationFixture(t, negotiatedRepo, lineage, true)

	legacyFields := strictReviewJSONFields(t, legacyOutput)
	wantLegacyFields := []string{"action", "lineage_id", "operation", "receipt_path", "state", "store_revision"}
	if !reflect.DeepEqual(legacyFields, wantLegacyFields) {
		t.Fatalf("legacy finalize fields = %v, want %v\n%s", legacyFields, wantLegacyFields, legacyOutput)
	}
	var legacy ReviewFacadeFinalizeResult
	decodeStrictReviewJSON(t, legacyOutput, &legacy)
	envelope := decodeReviewOperationEnvelope(t, negotiatedOutput)
	if envelope.Operation != ReviewIntegrationOperationFinalize {
		t.Fatalf("negotiated finalize operation = %q", envelope.Operation)
	}
	var negotiated ReviewIntegrationFinalizeResult
	decodeStrictReviewJSON(t, envelope.Result, &negotiated)
	if negotiated.Operation != legacy.Operation || negotiated.LineageID != legacy.LineageID || negotiated.State != legacy.State ||
		negotiated.Action != legacy.Action || negotiated.StoreRevision != legacy.StoreRevision {
		t.Fatalf("negotiated finalize result = %#v, legacy %#v", negotiated, legacy)
	}
	assertNoPrivateReviewOperationFields(t, negotiatedOutput)

	legacyAuthority := readReviewOperationFile(t, legacyStore.StatePath())
	negotiatedAuthority := readReviewOperationFile(t, negotiatedStore.StatePath())
	legacyReceipt := readReviewOperationFile(t, legacyStore.ReceiptPath())
	negotiatedReceipt := readReviewOperationFile(t, negotiatedStore.ReceiptPath())
	if !bytes.Equal(legacyAuthority, negotiatedAuthority) || !bytes.Equal(legacyReceipt, negotiatedReceipt) {
		t.Fatalf("negotiation changed canonical authority or receipt bytes")
	}
}

func TestNegotiatedReviewValidateCoversAllGatesAndPreservesLegacyResult(t *testing.T) {
	repo := initReviewCLIRepo(t)
	writeNegotiatedOperationChange(t, repo, "thin")
	lineage := "review-operation-validate"
	_, _ = finalizeNegotiatedOperationFixture(t, repo, lineage, true)
	branch := strings.TrimSpace(runReviewCLIGit(t, repo, "symbolic-ref", "--short", "HEAD"))
	configureCLIReviewPublicationRemote(t, repo, branch)
	runReviewCLIGit(t, repo, "add", "-A")
	runReviewCLIGit(t, repo, "commit", "-qm", "reviewed candidate")

	var legacyOutput bytes.Buffer
	if err := RunReviewFacadeValidate([]string{"--cwd", repo, "--lineage", lineage, "--gate", string(reviewtransaction.GatePostApply)}, &legacyOutput); err != nil {
		t.Fatal(err)
	}
	wantLegacyFields := []string{"action", "allowed", "context", "reason", "result", "schema"}
	if fields := strictReviewJSONFields(t, legacyOutput.Bytes()); !reflect.DeepEqual(fields, wantLegacyFields) {
		t.Fatalf("legacy validate fields = %v, want %v", fields, wantLegacyFields)
	}
	var legacy ReviewValidateResult
	decodeStrictReviewJSON(t, legacyOutput.Bytes(), &legacy)
	var negotiatedOutput bytes.Buffer
	if err := RunReviewFacadeValidate([]string{
		"--contract", ReviewIntegrationContractV1, "--cwd", repo, "--lineage", lineage, "--gate", string(reviewtransaction.GatePostApply),
	}, &negotiatedOutput); err != nil {
		t.Fatal(err)
	}
	var negotiatedLegacyEquivalent ReviewValidateResult
	decodeStrictReviewJSON(t, decodeReviewOperationEnvelope(t, negotiatedOutput.Bytes()).Result, &negotiatedLegacyEquivalent)
	if !reflect.DeepEqual(legacy, negotiatedLegacyEquivalent) {
		t.Fatalf("negotiated validate result changed legacy semantics:\nlegacy=%#v\nnegotiated=%#v", legacy, negotiatedLegacyEquivalent)
	}

	allowed, denied := 0, 0
	for _, gate := range []reviewtransaction.GateKind{
		reviewtransaction.GatePostApply, reviewtransaction.GatePreCommit, reviewtransaction.GatePrePush,
		reviewtransaction.GatePrePR, reviewtransaction.GateRelease,
	} {
		negotiatedOutput.Reset()
		err := RunReviewFacadeValidate([]string{
			"--contract", ReviewIntegrationContractV1, "--cwd", repo, "--lineage", lineage, "--gate", string(gate),
		}, &negotiatedOutput)
		if negotiatedOutput.Len() == 0 {
			t.Fatalf("%s negotiated validation emitted no result: %v", gate, err)
		}
		envelope := decodeReviewOperationEnvelope(t, negotiatedOutput.Bytes())
		if envelope.Operation != ReviewIntegrationOperationValidate {
			t.Fatalf("%s negotiated operation = %q", gate, envelope.Operation)
		}
		var result ReviewValidateResult
		decodeStrictReviewJSON(t, envelope.Result, &result)
		if result.Context.Gate != "" && result.Context.Gate != gate || result.Allowed != (err == nil) {
			t.Fatalf("%s negotiated validation = %#v, err %v", gate, result, err)
		}
		if result.Allowed {
			allowed++
		} else {
			var deniedError ReviewGateDeniedError
			if !errors.As(err, &deniedError) {
				t.Fatalf("%s denial error = %T %v", gate, err, err)
			}
			denied++
		}
		assertNoPrivateReviewOperationFields(t, negotiatedOutput.Bytes())
	}
	if allowed == 0 || denied == 0 {
		t.Fatalf("all-gate negotiated matrix requires success and denial, got allow=%d deny=%d", allowed, denied)
	}
}

func TestNegotiatedReviewBindSDDPreservesLegacyResultAndBindingHashes(t *testing.T) {
	legacyRepo := initReviewCLIRepo(t)
	negotiatedRepo := initReviewCLIRepo(t)
	for _, repo := range []string{legacyRepo, negotiatedRepo} {
		writeNegotiatedOperationChange(t, repo, "thin")
	}
	lineage := "review-operation-binding"
	_, legacyStore := finalizeNegotiatedOperationFixture(t, legacyRepo, lineage, false)
	_, negotiatedStore := finalizeNegotiatedOperationFixture(t, negotiatedRepo, lineage, true)

	var legacyOutput bytes.Buffer
	if err := RunReview([]string{
		"bind-sdd", "--cwd", legacyRepo, "--change", "thin", "--lineage", lineage, "--expected-binding-revision=",
	}, &legacyOutput); err != nil {
		t.Fatal(err)
	}
	wantLegacyFields := []string{"authority_revision", "change", "gate_context", "lineage", "receipt_hash", "revision", "schema"}
	if fields := strictReviewJSONFields(t, legacyOutput.Bytes()); !reflect.DeepEqual(fields, wantLegacyFields) {
		t.Fatalf("legacy bind-sdd fields = %v, want %v", fields, wantLegacyFields)
	}
	var legacy sddstatus.ReviewBinding
	decodeStrictReviewJSON(t, legacyOutput.Bytes(), &legacy)
	var negotiatedOutput bytes.Buffer
	if err := RunReview([]string{
		"bind-sdd", "--contract", ReviewIntegrationContractV1, "--cwd", negotiatedRepo, "--change", "thin",
		"--lineage", lineage, "--expected-binding-revision=",
	}, &negotiatedOutput); err != nil {
		t.Fatal(err)
	}
	envelope := decodeReviewOperationEnvelope(t, negotiatedOutput.Bytes())
	if envelope.Operation != ReviewIntegrationOperationBindSDD {
		t.Fatalf("negotiated bind-sdd operation = %q", envelope.Operation)
	}
	var negotiated sddstatus.ReviewBinding
	decodeStrictReviewJSON(t, envelope.Result, &negotiated)
	if !reflect.DeepEqual(legacy, negotiated) {
		t.Fatalf("negotiated binding changed identities:\nlegacy=%#v\nnegotiated=%#v", legacy, negotiated)
	}
	legacyBinding := readReviewOperationFile(t, reviewOperationBindingPath(legacyStore, "thin"))
	negotiatedBinding := readReviewOperationFile(t, reviewOperationBindingPath(negotiatedStore, "thin"))
	if !bytes.Equal(legacyBinding, negotiatedBinding) {
		t.Fatal("contract negotiation changed canonical SDD binding bytes")
	}
	assertNoPrivateReviewOperationFields(t, negotiatedOutput.Bytes())
}

func TestNegotiatedReviewBindSDDRejectsHistoricalLegacyThroughTypedFailureEnvelope(t *testing.T) {
	fixture := newLegacyCLIFixture(t, "historical-bind-sdd")
	writeNegotiatedOperationChange(t, fixture.repo, "thin")
	commonDir := strings.TrimSpace(runReviewCLIGit(t, fixture.repo, "rev-parse", "--path-format=absolute", "--git-common-dir"))
	authorityRoot := filepath.Join(commonDir, "gentle-ai")
	before := readLegacyAuthorityTree(t, authorityRoot)

	var output bytes.Buffer
	err := RunReview([]string{
		"bind-sdd", "--contract", ReviewIntegrationContractV1, "--cwd", fixture.repo, "--change", "thin",
		"--lineage", fixture.lineage, "--expected-binding-revision=",
	}, &output)
	if err == nil {
		t.Fatalf("legacy bind-sdd succeeded: %s", output.String())
	}
	failure := decodeReviewIntegrationFailure(t, output.Bytes())
	if failure.Operation != ReviewIntegrationOperationBindSDD || failure.Code != reviewtransaction.LegacyReadOnlyErrorCode ||
		failure.MutationOutcome != ReviewMutationNotStarted || failure.RetrySafe ||
		failure.Replayability != reviewtransaction.ReplayabilityNotReplayable || failure.NextAction != "stop" ||
		strings.Contains(output.String(), fixture.repo) || strings.Contains(output.String(), fixture.store.Dir) {
		t.Fatalf("legacy bind-sdd failure = %#v\n%s", failure, output.String())
	}
	var typed *reviewtransaction.LegacyReadOnlyError
	if !errors.Is(err, reviewtransaction.ErrLegacyReadOnly) || !errors.As(err, &typed) ||
		typed.Operation != "review/bind-sdd" || typed.LineageID != fixture.lineage {
		t.Fatalf("legacy bind-sdd lost typed cause: %#v", err)
	}
	if after := readLegacyAuthorityTree(t, authorityRoot); !reflect.DeepEqual(before, after) {
		t.Fatal("legacy bind-sdd changed authority bytes")
	}
}

func TestNegotiatedReviewOperationsRejectInvalidContractsBeforeMutation(t *testing.T) {
	for _, contract := range []string{"", "gentle-ai.review-integration/v2"} {
		t.Run("finalize_"+contract, func(t *testing.T) {
			repo := initReviewCLIRepo(t)
			writeNegotiatedOperationChange(t, repo, "thin")
			lineage := "review-invalid-finalize"
			started := startReviewOperationFixture(t, repo, lineage)
			store, err := reviewtransaction.CompactAuthoritativeStore(context.Background(), repo, started.LineageID)
			if err != nil {
				t.Fatal(err)
			}
			before := readReviewOperationFile(t, store.StatePath())
			evidence := filepath.Join(t.TempDir(), "evidence.txt")
			if err := os.WriteFile(evidence, []byte("tests pass\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			var output bytes.Buffer
			err = RunReviewFacadeFinalize([]string{
				"--contract=" + contract, "--cwd", repo, "--lineage", lineage, "--evidence", evidence,
			}, &output)
			if err == nil || output.Len() != 0 || !bytes.Equal(before, readReviewOperationFile(t, store.StatePath())) {
				t.Fatalf("invalid finalize contract %q mutated/output: %q, %v", contract, output.String(), err)
			}
			if _, err := os.Stat(store.ReceiptPath()); !os.IsNotExist(err) {
				t.Fatalf("invalid finalize contract published receipt: %v", err)
			}
		})

		t.Run("validate_and_bind_"+contract, func(t *testing.T) {
			repo := initReviewCLIRepo(t)
			writeNegotiatedOperationChange(t, repo, "thin")
			lineage := "review-invalid-read-boundary"
			_, store := finalizeNegotiatedOperationFixture(t, repo, lineage, false)
			beforeAuthority := readReviewOperationFile(t, store.StatePath())
			beforeReceipt := readReviewOperationFile(t, store.ReceiptPath())
			for _, call := range []struct {
				name string
				args []string
			}{
				{name: "validate", args: []string{"validate", "--contract=" + contract, "--cwd", repo, "--lineage", lineage, "--gate", string(reviewtransaction.GatePostApply)}},
				{name: "bind-sdd", args: []string{"bind-sdd", "--contract=" + contract, "--cwd", repo, "--change", "thin", "--lineage", lineage, "--expected-binding-revision="}},
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
			if !bytes.Equal(beforeAuthority, readReviewOperationFile(t, store.StatePath())) ||
				!bytes.Equal(beforeReceipt, readReviewOperationFile(t, store.ReceiptPath())) {
				t.Fatal("invalid contract changed authority or receipt")
			}
			if _, err := os.Stat(reviewOperationBindingPath(store, "thin")); !os.IsNotExist(err) {
				t.Fatalf("invalid bind contract published binding: %v", err)
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

func finalizeNegotiatedOperationFixture(t *testing.T, repo, lineage string, negotiated bool) ([]byte, reviewtransaction.CompactStore) {
	t.Helper()
	started := startReviewOperationFixture(t, repo, lineage)
	evidence := filepath.Join(t.TempDir(), "evidence.txt")
	if err := os.WriteFile(evidence, []byte("focused tests pass\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	args := []string{"--cwd", repo, "--lineage", started.LineageID, "--evidence", evidence}
	if negotiated {
		args = append([]string{"--contract", ReviewIntegrationContractV1}, args...)
	}
	var output bytes.Buffer
	if err := RunReviewFacadeFinalize(args, &output); err != nil {
		t.Fatal(err)
	}
	store, err := reviewtransaction.CompactAuthoritativeStore(context.Background(), repo, started.LineageID)
	if err != nil {
		t.Fatal(err)
	}
	return output.Bytes(), store
}

func startReviewOperationFixture(t *testing.T, repo, lineage string) ReviewFacadeStartResult {
	t.Helper()
	var output bytes.Buffer
	if err := RunReviewFacadeStart([]string{"--cwd", repo, "--lineage", lineage}, &output); err != nil {
		t.Fatal(err)
	}
	var started ReviewFacadeStartResult
	decodeStrictReviewJSON(t, output.Bytes(), &started)
	return started
}

func writeNegotiatedOperationChange(t *testing.T, repo, change string) {
	t.Helper()
	root := filepath.Join(repo, "openspec", "changes", change)
	for path, content := range map[string]string{
		"tasks.md": "- [x] 1.1 Done\n", "proposal.md": "# Proposal\n", "design.md": "# Design\n", "specs/binding/spec.md": "# Spec\n",
	} {
		fullPath := filepath.Join(root, filepath.FromSlash(path))
		if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(fullPath, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
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
	return filepath.Join(common, "gentle-ai", "sdd-review-bindings", "v1", change, "binding.json")
}
