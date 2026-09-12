package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	jsonschema "github.com/santhosh-tekuri/jsonschema/v6"

	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
)

func compileWholePublishedReviewSchema(t *testing.T, version, name string) *jsonschema.Schema {
	t.Helper()
	compiler := jsonschema.NewCompiler()
	versions := []string{"v1"}
	if version != "v1" {
		versions = append(versions, version)
	}
	for _, resourceVersion := range versions {
		root := filepath.Join("..", "..", "contracts", "review-integration", resourceVersion, "schemas")
		entries, err := os.ReadDir(root)
		if err != nil {
			t.Fatal(err)
		}
		for _, entry := range entries {
			if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
				continue
			}
			payload, err := os.ReadFile(filepath.Join(root, entry.Name()))
			if err != nil {
				t.Fatal(err)
			}
			var document any
			if err := json.Unmarshal(payload, &document); err != nil {
				t.Fatal(err)
			}
			location := "https://gentle-ai.dev/contracts/review-integration/" + resourceVersion + "/schemas/" + entry.Name()
			if err := compiler.AddResource(location, document); err != nil {
				t.Fatal(err)
			}
		}
	}
	schema, err := compiler.Compile("https://gentle-ai.dev/contracts/review-integration/" + version + "/schemas/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return schema
}

func validatePublishedReviewSchema(t *testing.T, schema *jsonschema.Schema, payload []byte) {
	t.Helper()
	var document any
	if err := json.Unmarshal(payload, &document); err != nil {
		t.Fatal(err)
	}
	if err := schema.Validate(document); err != nil {
		t.Fatal(err)
	}
}

func TestLowRiskStartBurnsUnderPublishedStatusContract(t *testing.T) {
	reviewEnabledHome(t)
	repo := initReviewCLIRepo(t)
	lineage := startLowRiskFacadeReview(t, repo)
	store, err := reviewtransaction.CompactAuthoritativeStore(t.Context(), repo, lineage)
	if err != nil {
		t.Fatal(err)
	}
	assertApprovedCompactAuthorityBurned(t, store, lineage)

	var output bytes.Buffer
	if err := RunReview([]string{
		"validate", "--cwd", repo, "--contract", ReviewIntegrationContractV2,
		"--gate", string(reviewtransaction.GatePreCommit),
	}, &output); err != nil {
		t.Fatal(err)
	}
	var envelope ReviewIntegrationOperationResult
	decodeStrictReviewJSON(t, output.Bytes(), &envelope)
	if err := envelope.Validate(); err != nil {
		t.Fatalf("gate operation envelope = %v", err)
	}
}

func TestPublishedLastEventClosureSchemaAcceptsApprovedTerminalCapture(t *testing.T) {
	reviewEnabledHome(t)
	repo := initReviewCLIRepo(t)
	writeReviewStartCandidate(t, repo, "candidate.go", "package candidate\n\nfunc value() int { return 1 }\n", 0o644)
	started := runNegotiatedReviewStart(t, repo, "published-approved-terminal-closure")
	var output bytes.Buffer
	for order := range started.SelectedLenses {
		destination := &bytes.Buffer{}
		if order == len(started.SelectedLenses)-1 {
			destination = &output
		}
		captureCleanCLIReviewerResult(t, repo, ReviewFacadeStartResult{
			LineageID: started.LineageID, TargetIdentity: started.RepositoryContext.TargetIdentity, SelectedLenses: started.SelectedLenses,
		}, order, destination)
	}

	schema := compileWholePublishedReviewSchema(t, "v2", "last-event-closure.schema.json")
	validatePublishedReviewSchema(t, schema, output.Bytes())
	closure := decodeJSONObjectCopy(t, output.Bytes())
	if results, found := closure["reviewer_results"].([]any); !found || len(results) != len(started.SelectedLenses) {
		t.Fatalf("approved last-event closure reviewer_results = %#v, want every selected lens", closure["reviewer_results"])
	}
	closure["reviewer_results"] = []any{}
	if err := schema.Validate(closure); err != nil {
		t.Fatalf("published last-event closure schema rejected an approved zero-lens readback: %v", err)
	}
	closure = decodeJSONObjectCopy(t, output.Bytes())
	delete(closure, "reviewer_results")
	if err := schema.Validate(closure); err == nil {
		t.Fatal("published last-event closure schema accepted approved output without reviewer_results")
	}
	closure = decodeJSONObjectCopy(t, output.Bytes())
	closure["escalation"] = map[string]any{"cause": "unresolved_severe_findings", "finding_ids": []any{}}
	if err := schema.Validate(closure); err == nil {
		t.Fatal("published last-event closure schema accepted escalation on approved state")
	}
}

func TestPublishedLastEventClosureSchemaAcceptsTerminalRefuterCapture(t *testing.T) {
	reviewEnabledHome(t)
	t.Setenv(reviewPiHostRelayContractEnvironment, reviewPiHostRelayContract)
	repo, store, record, handle := piRefuterReview(t)
	overrideProviderRoleHostAdapter(t, providerTestAdapter{raw: piRefuterRawResult(t, repo, store, record)})

	var output bytes.Buffer
	if err := RunReview(append(append([]string{"capture-refuter"}, piRefuterBinding(repo, record, handle)...), "--agent", "pi", "--execute=true"), &output); err != nil {
		t.Fatal(err)
	}

	schema := compileWholePublishedReviewSchema(t, "v2", "last-event-closure.schema.json")
	validatePublishedReviewSchema(t, schema, output.Bytes())
}

func TestPublishedLastEventClosureSchemaRejectsNonStatusCorrectionContinuation(t *testing.T) {
	reviewEnabledHome(t)
	t.Setenv(reviewPiHostRelayContractEnvironment, reviewPiHostRelayContract)
	repo, store, record, handle := piRefuterReview(t)
	overrideProviderRoleHostAdapter(t, providerTestAdapter{raw: piRefuterRawResult(t, repo, store, record)})

	var output bytes.Buffer
	if err := RunReview(append(append([]string{"capture-refuter"}, piRefuterBinding(repo, record, handle)...), "--agent", "pi", "--execute=true"), &output); err != nil {
		t.Fatal(err)
	}
	var closure map[string]any
	if err := json.Unmarshal(output.Bytes(), &closure); err != nil {
		t.Fatal(err)
	}
	continuation, ok := closure["status_continuation"].(map[string]any)
	if !ok || closure["state"] != string(reviewtransaction.StateCorrectionRequired) {
		t.Fatalf("real correction closure = %#v", closure)
	}
	closure["reviewer_results"] = []any{}
	schema := compileWholePublishedReviewSchema(t, "v2", "last-event-closure.schema.json")
	if err := schema.Validate(closure); err == nil {
		t.Fatal("published last-event closure schema accepted reviewer_results on correction-required state")
	}
	delete(closure, "reviewer_results")
	continuation["operation"] = "review.start"

	if err := schema.Validate(closure); err == nil {
		t.Fatalf("published last-event closure schema accepted non-STATUS correction continuation: %#v", closure)
	}
}
