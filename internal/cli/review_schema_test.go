package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
)

func TestFinalVerificationIncidentSchemaIsClosedToProceduralToolingFailure(t *testing.T) {
	var output bytes.Buffer
	if err := RunReviewSchema([]string{"final-verification-incident"}, &output); err != nil {
		t.Fatal(err)
	}
	var schema map[string]any
	if err := json.Unmarshal(output.Bytes(), &schema); err != nil {
		t.Fatal(err)
	}
	if schema["additionalProperties"] != false || schema["$id"] != "https://gentle-ai.dev/contracts/review-integration/v1/schemas/final-verification-incident.schema.json" {
		t.Fatalf("incident schema header = %#v", schema)
	}
	properties := schema["properties"].(map[string]any)
	if properties["schema"].(map[string]any)["const"] != "gentle-ai.review-final-verification-incident/v1" ||
		properties["class"].(map[string]any)["const"] != "procedural_tooling_failure" {
		t.Fatalf("incident identity = %#v", properties)
	}
	if len(schemaStringArray(t, schema["required"])) != 8 {
		t.Fatalf("incident required fields = %#v", schema["required"])
	}
	contractPayload, err := os.ReadFile(filepath.Join("..", "..", "contracts", "review-integration", "v1", "schemas", "final-verification-incident.schema.json"))
	if err != nil {
		t.Fatal(err)
	}
	var contract map[string]any
	if err := json.Unmarshal(contractPayload, &contract); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(schema, contract) {
		t.Fatal("runtime and packaged final-verification incident schemas differ")
	}
}

func TestReviewerSchemaMatchesProviderAdmissionEnvelope(t *testing.T) {
	var output bytes.Buffer
	if err := RunReviewSchema([]string{"reviewer"}, &output); err != nil {
		t.Fatal(err)
	}
	var schema map[string]any
	if err := json.Unmarshal(output.Bytes(), &schema); err != nil {
		t.Fatal(err)
	}
	required := schemaStringArray(t, schema["required"])
	for _, field := range []string{"subject_hash", "inspection", "findings", "evidence"} {
		if !containsString(required, field) {
			t.Fatalf("reviewer schema required fields = %v, missing %q", required, field)
		}
	}
	properties := schema["properties"].(map[string]any)
	if properties["subject_hash"].(map[string]any)["pattern"] != "^sha256:[0-9a-f]{64}$" {
		t.Fatalf("reviewer subject_hash schema = %#v", properties["subject_hash"])
	}
	inspection := properties["inspection"].(map[string]any)
	if inspection["additionalProperties"] != false {
		t.Fatalf("reviewer inspection schema is not strict: %#v", inspection)
	}
	inspectionRequired := schemaStringArray(t, inspection["required"])
	if !containsString(inspectionRequired, "status") || !containsString(inspectionRequired, "paths") {
		t.Fatalf("reviewer inspection required fields = %v", inspectionRequired)
	}
	finding := properties["findings"].(map[string]any)["items"].(map[string]any)
	id := finding["properties"].(map[string]any)["id"].(map[string]any)
	if id["pattern"] != "^R[1-4]-[A-Za-z0-9][A-Za-z0-9._-]*$" {
		t.Fatalf("reviewer finding id pattern = %#v", id)
	}
}

// TestReviewSchemaVerificationEvidenceEntry is the RED-first proof for 1775:
// review schema must publish the input contract that review capture-evidence
// actually accepts and readCapturedFinalEvidence actually enforces — raw,
// non-empty evidence content up to the native artifact bound — not an
// invented structured shape.
func TestReviewSchemaVerificationEvidenceEntry(t *testing.T) {
	var output bytes.Buffer
	if err := RunReviewSchema([]string{"verification-evidence"}, &output); err != nil {
		t.Fatal(err)
	}
	var schema map[string]any
	if err := json.Unmarshal(output.Bytes(), &schema); err != nil {
		t.Fatal(err)
	}
	if schema["$id"] != "https://gentle-ai.dev/schema/review/verification-evidence/v1" || schema["type"] != "string" {
		t.Fatalf("verification-evidence schema header = %#v", schema)
	}
	minLength, ok := schema["minLength"].(float64)
	if !ok || minLength != 1 {
		t.Fatalf("verification-evidence schema minLength = %#v, want 1 (matches readCapturedFinalEvidence's non-empty requirement)", schema["minLength"])
	}
	maxLength, ok := schema["maxLength"].(float64)
	if !ok || maxLength != float64(reviewResultArtifactLimit) {
		t.Fatalf("verification-evidence schema maxLength = %#v, want %d (matches the native artifact bound)", schema["maxLength"], reviewResultArtifactLimit)
	}

	var usageOutput bytes.Buffer
	err := RunReviewSchema(nil, &usageOutput)
	if err == nil || !containsAll(err.Error(), []string{"verification-evidence"}) {
		t.Fatalf("review schema usage error = %v, want it to name verification-evidence", err)
	}
}

func TestReviewSchemaVerificationEvidenceRecordMatchesContractFixture(t *testing.T) {
	var output bytes.Buffer
	if err := RunReviewSchema([]string{"verification-evidence-record"}, &output); err != nil {
		t.Fatal(err)
	}
	contractPath := filepath.Join("..", "..", "contracts", "review-integration", "v1", "schemas", "verification-evidence.schema.json")
	contract, err := os.ReadFile(contractPath)
	if err != nil {
		t.Fatal(err)
	}
	var emitted, published any
	if err := json.Unmarshal(output.Bytes(), &emitted); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(contract, &published); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(emitted, published) {
		t.Fatalf("verification evidence record schema differs from published contract\nemitted=%#v\npublished=%#v", emitted, published)
	}
	fixturePath := filepath.Join("..", "..", "contracts", "review-integration", "v1", "fixtures", "verification-evidence.fixture.json")
	fixture, err := os.ReadFile(fixturePath)
	if err != nil {
		t.Fatal(err)
	}
	record, err := reviewtransaction.ParseVerificationEvidenceRecord(fixture)
	if err != nil || record.Outcome != reviewtransaction.VerificationOutcomePassed {
		t.Fatalf("verification evidence fixture = %#v, %v", record, err)
	}
}

func containsAll(value string, substrings []string) bool {
	for _, substring := range substrings {
		if !bytes.Contains([]byte(value), []byte(substring)) {
			return false
		}
	}
	return true
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
