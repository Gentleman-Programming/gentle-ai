package cli

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
)

func TestReviewProviderArtifactV1ContractsArePinned(t *testing.T) {
	root := filepath.Join("..", "..", "contracts", "review-integration", "v1")
	want := map[string]string{
		"fixtures/capabilities-v1.4.fixture.json": "84e0db457b76b97b35c2be772dfc647f9eab66810ea98f64fed85645c3c266ba",
		"fixtures/start.fixture.json":             "f369160ac26eb3427b57de2dd01c9d8c81e51c8a2bd546446780129d31b1945b",
		"fixtures/start-v2.fixture.json":          "388c7c21374b89afe2d42d64bd1987d17ec0e2c7151cab1c56a08969ffb2ea0e",
		"fixtures/status.fixture.json":            "555054d8046a896162995dcb117752f9cd1ef903fb9ebaad29af1b7e7f319bb3",
		"fixtures/status-v2.fixture.json":         "5410d8bbae1b7152a43b3a5c4c880e9e98a5e47b76d910456f5ef13f19836f3a",
		"fixtures/status-ambiguous.fixture.json":  "ee695fd58ba72adfb3b51dfd16432a177498173a45bfcb594d6bdc53bfa32e6e",
		"fixtures/status-corrupted.fixture.json":  "4cfc0048c28a39cec8a32fecfaad66e56e5c1248263ceb4ce66b6717981880b2",
		"fixtures/status-recover.fixture.json":    "714f762f72380ce93d567626cafbaa536ab3aae02af73d3d40ca123f1f30d8b0",
		"fixtures/status-unrelated.fixture.json":  "deab36c877ced3c9b480ca33724c10d88f75c761d6426fa14be850345122891d",
		"schemas/admitted-result.schema.json":     "7796e8dbba331434594108c902dfab7ec46f691fa447a9259a78f2448111b0de",
		"schemas/artifact-subject.schema.json":    "f7dcd934e27e8f3735a37f3d0ec8048dd8ccc1811b9df61124a1dcbf8a03f40e",
		"schemas/capabilities-v1.4.schema.json":   "926b61c8ac0f870f09214f6bd8af1b035c5b72f14f0b83c0d4a7bdbb277f5447",
		"schemas/result-artifact.schema.json":     "91296bd2c261fd2fe03bffd63efe58badd4927e0d0d8480cd4213f651ecacdf6",
		"schemas/start.schema.json":               "4296aebbd4128ce51945a2f6d3228aa77ac7215c802978d559bff5279ec56229",
		"schemas/start-v2.schema.json":            "ec8550cd93bbe84af1ce87dfd7abfa9e24692f42b20f8f0bf9cac1d4b88ea46c",
		"schemas/status.schema.json":              "250d2c646b8822b38eaefafd2bfdefa1134cc23a00e553a7201f33257573149a",
		"schemas/status-v2.schema.json":           "dd9914b647a1d9edc4ecdcbed4f0c800b39ec290912d5c2a4cc6ba3098d5f21e",
	}
	for name, expected := range want {
		payload, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(name)))
		if err != nil {
			t.Fatal(err)
		}
		digest := sha256.Sum256(payload)
		if actual := hex.EncodeToString(digest[:]); actual != expected {
			t.Fatalf("%s digest = %s, want %s", name, actual, expected)
		}
	}
}

func TestReviewProviderArtifactV20ContractsArePinned(t *testing.T) {
	root := filepath.Join("..", "..", "contracts", "review-integration", "v2")
	want := map[string]string{
		"fixtures/capabilities.fixture.json": "17c150d851c15b3f0c20d18c2e2741eb2232ffa24f35aa71d6d30e90a85e42b7",
		"fixtures/consent.fixture.json":      "203cc96d5c29ba0f27b5c4db04c2e88566e0a923d3a0cdb317f78d9065349075",
		"fixtures/status.fixture.json":       "d5438578f2969f17635fecea94c7ef46d14c78fa668e50df48c4254254d5e935",
		"schemas/capabilities.schema.json":   "7ab061ed27bd3b929d6033cc20f56097e851f4454ca14a815255748b50191248",
		"schemas/consent.schema.json":        "b2b4465338497f11927de91cb2e5da12b6cb4a1039afe05aebe1abbf53b21858",
		"schemas/status.schema.json":         "c4dcc736cfc6300560a3c4262d2d982368529d5c49d58d499552a3b0beef9212",
	}
	for name, expected := range want {
		payload, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(name)))
		if err != nil {
			t.Fatal(err)
		}
		digest := sha256.Sum256(payload)
		if actual := hex.EncodeToString(digest[:]); actual != expected {
			t.Fatalf("%s digest = %s, want %s", name, actual, expected)
		}
	}
}

func TestReviewProviderArtifactV21ContractsArePinned(t *testing.T) {
	root := filepath.Join("..", "..", "contracts", "review-integration", "v2")
	want := map[string]string{
		"fixtures/capabilities-v2.1.fixture.json": "4bbcbaed1b20e6ea8f9c615f35ff17b13ee69b4648784a4906191880751c668d",
		"fixtures/consent-v3.fixture.json":        "ead7d16aa1dabe9db4b7675d7e3b3de5594d7856f98960d8ec9bba99f91c0e31",
		"schemas/capabilities-v2.1.schema.json":   "9ede8ebbe3e169cf6ca4f4a6882c9c4e588a6d1073d8e22a155649cd41d38cd0",
		"schemas/consent-v3.schema.json":          "80915f5f4f43a494826253d1e7251fc463989f41d2cf163a6a52a8b4328c023c",
		"schemas/status.schema.json":              "c4dcc736cfc6300560a3c4262d2d982368529d5c49d58d499552a3b0beef9212",
	}
	for name, expected := range want {
		payload, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(name)))
		if err != nil {
			t.Fatal(err)
		}
		digest := sha256.Sum256(payload)
		if actual := hex.EncodeToString(digest[:]); actual != expected {
			t.Fatalf("%s digest = %s, want %s", name, actual, expected)
		}
	}
}

func TestReviewProviderArtifactSchemasAreStrictAndBound(t *testing.T) {
	root := filepath.Join("..", "..", "contracts", "review-integration", "v1", "schemas")
	tests := []struct {
		name string
		id   string
	}{
		{name: "artifact-subject.schema.json", id: "https://gentle-ai.dev/contracts/review-integration/v1/schemas/artifact-subject.schema.json"},
		{name: "admitted-result.schema.json", id: "https://gentle-ai.dev/contracts/review-integration/v1/schemas/admitted-result.schema.json"},
		{name: "correction-plan-request.schema.json", id: reviewtransaction.CorrectionPlanRequestSchemaID},
		{name: "result-artifact-v2.schema.json", id: "https://gentle-ai.dev/contracts/review-integration/v1/schemas/result-artifact-v2.schema.json"},
		{name: "start-v2.schema.json", id: ReviewIntegrationStartSchemaIDV2},
		{name: "status-v2.schema.json", id: ReviewIntegrationStatusSchemaIDV2},
		{name: "authority-repair-assessment.schema.json", id: reviewtransaction.AuthorityRepairAssessmentSchemaID},
		{name: "repair.schema.json", id: ReviewIntegrationRepairSchemaID},
	}
	documents := make(map[string]map[string]any, len(tests))
	for _, tt := range tests {
		payload, err := os.ReadFile(filepath.Join(root, tt.name))
		if err != nil {
			t.Fatal(err)
		}
		var schema map[string]any
		if err := json.Unmarshal(payload, &schema); err != nil {
			t.Fatal(err)
		}
		if schema["$schema"] != "https://json-schema.org/draft/2020-12/schema" || schema["$id"] != tt.id || schema["additionalProperties"] != false {
			t.Fatalf("%s header = %#v", tt.name, schema)
		}
		documents[tt.name] = schema
	}

	artifact := documents["result-artifact-v2.schema.json"]
	artifactRequired := schemaStringArray(t, artifact["required"])
	for _, field := range []string{"subject_hash", "admission_decision"} {
		if !slices.Contains(artifactRequired, field) {
			t.Fatalf("result artifact v2 omits required %q: %v", field, artifactRequired)
		}
	}
	if artifact["oneOf"] == nil {
		t.Fatal("result artifact v2 does not require exactly one provider-owned locator")
	}

	start := documents["start-v2.schema.json"]
	if !slices.Contains(schemaStringArray(t, start["required"]), "artifact_subjects") {
		t.Fatal("START v2 does not require provider-owned artifact subjects")
	}
	riskCodes := start["$defs"].(map[string]any)["risk_reason"].(map[string]any)["properties"].(map[string]any)["code"].(map[string]any)["enum"]
	codes := schemaStringArray(t, riskCodes)
	for _, code := range []string{string(reviewtransaction.RiskReasonProcessBoundary), string(reviewtransaction.RiskReasonProcessScanLimit)} {
		if !slices.Contains(codes, code) {
			t.Fatalf("START v2 rejects runtime risk reason %q: %v", code, codes)
		}
	}
	startStates := schemaStringArray(t, start["properties"].(map[string]any)["state"].(map[string]any)["enum"])
	for _, state := range []string{string(reviewtransaction.StateCorrectionRequired), string(reviewtransaction.StateValidating)} {
		if !slices.Contains(startStates, state) {
			t.Fatalf("START v2 rejects valid compact state %q: %v", state, startStates)
		}
	}

	status := documents["status-v2.schema.json"]
	transitionArtifact := status["$defs"].(map[string]any)["transition_artifact"].(map[string]any)
	transitionRequired := schemaStringArray(t, transitionArtifact["required"])
	for _, field := range []string{"subject_hash", "admission_decision"} {
		if !slices.Contains(transitionRequired, field) {
			t.Fatalf("status v2 transition artifact omits %q: %v", field, transitionRequired)
		}
	}
	properties := transitionArtifact["properties"].(map[string]any)
	if properties["schema"].(map[string]any)["const"] != reviewResultArtifactSchema ||
		properties["admission_decision"].(map[string]any)["const"] != string(reviewtransaction.ArtifactAdmissionCompleted) {
		t.Fatalf("status v2 artifact identity = %#v", properties)
	}
	transitionInput := status["$defs"].(map[string]any)["transition_input"].(map[string]any)
	inputRules := transitionInput["allOf"].([]any)
	captureRule := inputRules[1].(map[string]any)
	captureThen := captureRule["then"].(map[string]any)
	for _, field := range []string{"artifact_subject", "candidate_diff", "changed_path_manifest"} {
		if !slices.Contains(schemaStringArray(t, captureThen["required"]), field) {
			t.Fatalf("legacy status v2 capture input omits required frozen context %q: %#v", field, captureThen)
		}
	}
	inputProperties := transitionInput["properties"].(map[string]any)
	if inputProperties["artifact_subject"].(map[string]any)["$ref"] != "artifact-subject.schema.json" ||
		inputProperties["candidate_diff"] == nil || inputProperties["base_tree"] != nil || inputProperties["candidate_tree"] != nil ||
		inputProperties["changed_path_manifest"].(map[string]any)["type"] != "array" {
		t.Fatalf("legacy status v2 capture input frozen context = %#v", inputProperties)
	}

	v2Root := filepath.Join("..", "..", "contracts", "review-integration", "v2", "schemas")
	v2Schemas := []struct {
		name string
		id   string
	}{
		{name: "artifact-subject.schema.json", id: "https://gentle-ai.dev/contracts/review-integration/v2/schemas/artifact-subject.schema.json"},
		{name: "admitted-result.schema.json", id: "https://gentle-ai.dev/contracts/review-integration/v2/schemas/admitted-result.schema.json"},
		{name: "start.schema.json", id: ReviewIntegrationStartSchemaID},
		{name: "status.schema.json", id: ReviewIntegrationStatusSchemaIDV3},
		{name: "status-v4.schema.json", id: ReviewIntegrationStatusSchemaID},
		{name: "capabilities.schema.json", id: ReviewIntegrationCapabilitiesSchemaIDV2},
		{name: "capabilities-v2.1.schema.json", id: ReviewIntegrationCapabilitiesSchemaIDV21},
		{name: "capabilities-v2.2.schema.json", id: ReviewIntegrationCapabilitiesSchemaIDV22},
		{name: "consent.schema.json", id: ReviewIntegrationConsentSchemaIDV2},
		{name: "consent-v3.schema.json", id: ReviewIntegrationConsentSchemaIDV3},
		{name: "failure.schema.json", id: ReviewIntegrationFailureSchemaIDV2},
		{name: "operation.schema.json", id: ReviewIntegrationOperationSchemaIDV2},
		{name: "repair.schema.json", id: ReviewIntegrationRepairSchemaIDV2},
	}
	v2Documents := make(map[string]map[string]any, len(v2Schemas))
	for _, tt := range v2Schemas {
		payload, err := os.ReadFile(filepath.Join(v2Root, tt.name))
		if err != nil {
			t.Fatal(err)
		}
		var schema map[string]any
		if err := json.Unmarshal(payload, &schema); err != nil {
			t.Fatal(err)
		}
		if schema["$schema"] != "https://json-schema.org/draft/2020-12/schema" || schema["$id"] != tt.id || schema["additionalProperties"] != false {
			t.Fatalf("v2 %s header = %#v", tt.name, schema)
		}
		v2Documents[tt.name] = schema
	}
	v2Input := v2Documents["status.schema.json"]["$defs"].(map[string]any)["transition_input"].(map[string]any)
	v2CaptureThen := v2Input["allOf"].([]any)[1].(map[string]any)["then"].(map[string]any)
	for _, field := range []string{"artifact_subject", "base_tree", "candidate_tree", "changed_path_manifest"} {
		if !slices.Contains(schemaStringArray(t, v2CaptureThen["required"]), field) {
			t.Fatalf("native Git status capture input omits %q: %#v", field, v2CaptureThen)
		}
	}
	v2Properties := v2Input["properties"].(map[string]any)
	if v2Properties["candidate_diff"] != nil || v2Properties["base_tree"] == nil || v2Properties["candidate_tree"] == nil {
		t.Fatalf("native Git status capture input = %#v", v2Properties)
	}
}

func TestReviewProviderArtifactV2FixturesValidate(t *testing.T) {
	root := filepath.Join("..", "..", "contracts", "review-integration", "v2", "fixtures")
	startPayload, err := os.ReadFile(filepath.Join(root, "start.fixture.json"))
	if err != nil {
		t.Fatal(err)
	}
	var start ReviewIntegrationStartResult
	if err := json.Unmarshal(startPayload, &start); err != nil {
		t.Fatal(err)
	}
	if err := start.Validate(); err != nil {
		t.Fatalf("v2 START fixture: %v", err)
	}
	statusPayload, err := os.ReadFile(filepath.Join(root, "status.fixture.json"))
	if err != nil {
		t.Fatal(err)
	}
	var status ReviewTargetStatusResult
	if err := json.Unmarshal(statusPayload, &status); err != nil {
		t.Fatal(err)
	}
	if err := status.Validate(); err != nil {
		t.Fatalf("v2 STATUS fixture: %v", err)
	}
	consentPayload, err := os.ReadFile(filepath.Join(root, "consent.fixture.json"))
	if err != nil {
		t.Fatal(err)
	}
	var consent ReviewIntegrationConsentResult
	if err := json.Unmarshal(consentPayload, &consent); err != nil {
		t.Fatal(err)
	}
	if err := consent.Validate(); err != nil {
		t.Fatalf("v2 consent fixture: %v", err)
	}
	consentV3Payload, err := os.ReadFile(filepath.Join(root, "consent-v3.fixture.json"))
	if err != nil {
		t.Fatal(err)
	}
	var consentV3 ReviewIntegrationConsentResult
	if err := json.Unmarshal(consentV3Payload, &consentV3); err != nil {
		t.Fatal(err)
	}
	if err := consentV3.Validate(); err != nil || consentV3.Agent != "claude-code" {
		t.Fatalf("v2.1 consent fixture: %#v, %v", consentV3, err)
	}
}

func schemaStringArray(t *testing.T, value any) []string {
	t.Helper()
	values, ok := value.([]any)
	if !ok {
		t.Fatalf("schema value is not an array: %#v", value)
	}
	result := make([]string, len(values))
	for index, value := range values {
		text, ok := value.(string)
		if !ok {
			t.Fatalf("schema array value is not a string: %#v", value)
		}
		result[index] = text
	}
	return result
}
