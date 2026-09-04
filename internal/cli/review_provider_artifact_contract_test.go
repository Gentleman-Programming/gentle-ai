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
		"fixtures/start.fixture.json":             "3b963b221cd1560eb8872cbabbb5407096f593ced2f13eb9cb06eb61e4cca4d1",
		// issue #2659: start-v2/status-v2 embed a freshly minted target_identity,
		// and the purified identity domain legitimately changed that hash for
		// every new snapshot. Deliberate, not drift.
		"fixtures/start-v2.fixture.json":         "2699660832c0d944184d5d314f08774ab9a02f5b8a7a4c2a07983440e0e346ad",
		"fixtures/status.fixture.json":           "a1f28b7d5351e000aca5238ed6348a0838fe2c0e64ce894ebcd8b43851063ff6",
		"fixtures/status-v2.fixture.json":        "ff3690a9e716c9fa48e3c26a67047f9b4ce4c3cce8391a240dbe9834bd4e13ee",
		"fixtures/status-ambiguous.fixture.json": "ee695fd58ba72adfb3b51dfd16432a177498173a45bfcb594d6bdc53bfa32e6e",
		"fixtures/status-corrupted.fixture.json": "4cfc0048c28a39cec8a32fecfaad66e56e5c1248263ceb4ce66b6717981880b2",
		"fixtures/status-recover.fixture.json":   "714f762f72380ce93d567626cafbaa536ab3aae02af73d3d40ca123f1f30d8b0",
		"fixtures/status-unrelated.fixture.json": "deab36c877ced3c9b480ca33724c10d88f75c761d6426fa14be850345122891d",
		"schemas/admitted-result.schema.json":    "7796e8dbba331434594108c902dfab7ec46f691fa447a9259a78f2448111b0de",
		"schemas/artifact-subject.schema.json":   "f7dcd934e27e8f3735a37f3d0ec8048dd8ccc1811b9df61124a1dcbf8a03f40e",
		"schemas/capabilities-v1.4.schema.json":  "926b61c8ac0f870f09214f6bd8af1b035c5b72f14f0b83c0d4a7bdbb277f5447",
		"schemas/result-artifact.schema.json":    "91296bd2c261fd2fe03bffd63efe58badd4927e0d0d8480cd4213f651ecacdf6",
		"schemas/start.schema.json":              "4296aebbd4128ce51945a2f6d3228aa77ac7215c802978d559bff5279ec56229",
		// Frozen v1 START artifacts do not project the v3 replay or retired
		// stale-burn fields.
		"schemas/start-v2.schema.json":             "ec8550cd93bbe84af1ce87dfd7abfa9e24692f42b20f8f0bf9cac1d4b88ea46c",
		"schemas/status.schema.json":               "86d0a5ff09a833ff723804c3e31185a80826cbd81a73cf61026feea8c5df2314",
		"schemas/status-v2.schema.json":            "7c51627d133592839ba4afa860b358b68109afd5f70ee998cd421f563201b23e",
		"schemas/transition-execution.schema.json": "ddee03bd0c1b6e70f21c399bae7fe528aa4ad46cebb5a48ec72b6e6b3694aa2d",
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
		"fixtures/capabilities.fixture.json": "8d5e1a8491db1a5a2f6329e8c1d5cd210dd175e0525ff4d51fa914351d2fcf08",
		"fixtures/consent.fixture.json":      "203cc96d5c29ba0f27b5c4db04c2e88566e0a923d3a0cdb317f78d9065349075",
		"fixtures/status.fixture.json":       "846377e06df2cae3587c4258ea75fe1ec1b51f08d01f1d498378c3bf13e93921",
		"schemas/capabilities.schema.json":   "df1d1d36bfb8b7816d3eb1c44c1350b4a36e27ac321922963add9dd25ed5a1a2",
		"schemas/consent.schema.json":        "b2b4465338497f11927de91cb2e5da12b6cb4a1039afe05aebe1abbf53b21858",
		"schemas/status.schema.json":         "3b257b417270744061dc943a97537e253e36e34de4591b0400e3c38ea3efde80",
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
		"fixtures/capabilities-v2.1.fixture.json": "96d157898c2bed6d028203999c081fcbb7992fb91a61f27b7eaab80c95245bd6",
		// issue #2659: consent-v3 embeds a freshly minted target_identity;
		// the purified identity domain legitimately changed that hash.
		// Deliberate, not drift.
		"fixtures/consent-v3.fixture.json":      "feb1dc7705f7da6490698ef48021bb7730de154ae23ec73d033d8d96fa996a21",
		"schemas/capabilities-v2.1.schema.json": "95d2b8b46e9be6e6fbc874fc763029cb7994951336c8974dc1694834d64bf06e",
		// Cross-lane battery conformance fix: the schema pinned the choice
		// invocations to `--agent claude-code`, but the live emitter omits the
		// agent token when the caller declared no runtime (the pinned fixture
		// itself carries no --agent), and #2676 binds the declared runtime
		// (claude-code, opencode, codex) when there is one. The schema now
		// follows the emitter. The agent enum also admits "pi": the Pi host
		// relay drives consent with its own declared runtime identity, which
		// the emitter legitimately publishes once the relay handshake is
		// declared. Deliberate, not drift.
		"schemas/consent-v3.schema.json": "f56b1809c1bff21713795ef37a095c6ecfdbbb3cf928bcf604b8d5f33be3dea5",
		"schemas/status.schema.json":     "3b257b417270744061dc943a97537e253e36e34de4591b0400e3c38ea3efde80",
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

func TestReviewProviderArtifactV25StatusContractsArePinned(t *testing.T) {
	root := filepath.Join("..", "..", "contracts", "review-integration", "v2")
	want := map[string]string{
		"fixtures/status-v5.fixture.json": "da401836833192a400493787b256b5f19b3a5ec5fd325ad45d8dcaeadfeea81e",
		// Cross-lane battery conformance fix: live negotiated STATUS publishes
		// the top-level repository_context reference (review_status_contract.go's
		// ReviewTargetStatusResult, populated since the recovered-units merges),
		// which the schema did not admit; and targeted_validation_required also
		// arrives as the provider-task (external.run_provider_role) and pi
		// host-relay (review.capture-validation) shapes, which the schema
		// rejected as missing the generic submission; and the negotiated-route
		// disposition preview (ReviewRepairDispositionProviderInputs) is real
		// optional emitter output the strict schema must admit; and the pi
		// host-relay materialize path renders the reviewer_result collect
		// input with a capture-result submission descriptor, which the
		// submission oneOf and the no-submission allOf rule both rejected.
		// Deliberate, not drift.
		"schemas/start.schema.json":     "27954ad34319719a68f90768c90f39254d94c62cf7f8ea90525ec4e2dbafd182",
		"schemas/status-v5.schema.json": "997bd9628ea59871640e4a17b46d61f8590c93f64e9344f24d809eb6b7cbcf6c",
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

// TestReviewProviderArtifactV23StartContractsArePinned pins the artifacts the
// start/v4 continuation work first published (issue #3894): the start-v4
// envelope with its provider-issued reviewing STATUS re-entry, and the v2.3
// capabilities advertisement that names it.
func TestReviewProviderArtifactV23StartContractsArePinned(t *testing.T) {
	root := filepath.Join("..", "..", "contracts", "review-integration", "v2")
	want := map[string]string{
		"fixtures/capabilities-v2.3.fixture.json": "ed5fb324791eec28287c621f19dffd69323120f61ce537e7b329fc018a29fe42",
		"fixtures/start-v4.fixture.json":          "639a6e78b40cb5e000ec15265fd444c243e28594035c7d376c378142162bfb02",
		"schemas/capabilities-v2.3.schema.json":   "606efa4b691605b0e7b668c616d48712a2a925c819244ebe2bc63d9885658bb3",
		"schemas/start-v4.schema.json":            "770c6a7e40a62a945d1134cba933cfd811f4c5e6ab407a36a26ba56508bc00e4",
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

func TestReviewProviderArtifactV24IntendedUntrackedContractsArePinned(t *testing.T) {
	root := filepath.Join("..", "..", "contracts", "review-integration", "v2")
	want := map[string]string{
		"schemas/capabilities-v2.4.schema.json":            "fc4d55dbad6b19cc4c289e8ed94bd1839800ca2892e449640459b668e0c7b0b5",
		"schemas/intended-untracked-selection.schema.json": "6f300c4cc10ab669fa3ef8cc608829df623a453cd5e6629958786e0724430259",
		"schemas/status-v6.schema.json":                    "0aa731e4d3961d678b4e51a6be0af93f2de82a4a326c3366e2fbe6a3e687236c",
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

// TestReviewProviderArtifactConformanceSchemasArePinned pins the schemas the
// cross-lane battery conformance work first published: the delivery gate
// result (gentle-ai.review-gate-result/v1) and the OpenCode provider-role
// capture acknowledgement (gentle-ai.opencode-review-provider-role/v1). Both
// envelopes already shipped on the wire; only their published schemas are new.
func TestReviewProviderArtifactConformanceSchemasArePinned(t *testing.T) {
	root := filepath.Join("..", "..", "contracts", "review-integration", "v2")
	want := map[string]string{
		"schemas/gate-result.schema.json":        "afe5e2a030fae9949305811bcac0a6dbc8b4f28802fa61d1e31e58e895f9fcae",
		"schemas/last-event-closure.schema.json": "9059651e39278f6932929392f4dacc3911d65fe3769171e2401b87df55da9030",
		// issue #3894: start/v4 publishes the reviewing status continuation, so
		// transition-execution gains the start_status_execution definition it
		// references. Deliberate, not drift.
		// issue #3932: start_status_execution carries the opaque
		// repository-context row, so a foreign process cwd fails closed.
		"schemas/transition-execution.schema.json":   "3743a16d915f5d95be047af1f0454f342aa4c3eb7bcb0d8991f81ae3b89873c1",
		"schemas/opencode-provider-role.schema.json": "c6b9f216f89c044f8e844b55e7200114850cfbc16642bca0677f30a399d8aa9b",
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

// TestReviewProviderArtifactStatusV7ContractsArePinned pins the schemas issue
// #4040's fix first published: status/v7, which publishes
// eligible_untracked_inventory unconditionally at the top level, and
// capabilities-v2.5, which advertises status/v7 (design decision 5).
func TestReviewProviderArtifactStatusV7ContractsArePinned(t *testing.T) {
	root := filepath.Join("..", "..", "contracts", "review-integration", "v2")
	want := map[string]string{
		"schemas/status-v7.schema.json":         "5a5e4a7052689f97767d02625ad7572628cc4d5a62677497f569fb2d78608aa2",
		"schemas/capabilities-v2.5.schema.json": "9fcdb1717a54bcd4f73d4dee1283d9ec2f27cccbb5d54804ee8b40a6ed2db553",
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
		{name: "transition-execution.schema.json", id: "https://gentle-ai.dev/contracts/review-integration/v1/schemas/transition-execution.schema.json"},
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
	transitionExecution := documents["transition-execution.schema.json"]
	transitionArtifact := transitionExecution["$defs"].(map[string]any)["transition_artifact"].(map[string]any)
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
		{name: "start.schema.json", id: ReviewIntegrationStartSchemaIDV3},
		{name: "start-v4.schema.json", id: ReviewIntegrationStartSchemaIDV4},
		{name: "status.schema.json", id: ReviewIntegrationStatusSchemaIDV3},
		{name: "status-v4.schema.json", id: ReviewIntegrationStatusSchemaIDV4},
		{name: "status-v5.schema.json", id: ReviewIntegrationStatusSchemaIDV5},
		{name: "status-v6.schema.json", id: ReviewIntegrationStatusSchemaIDV6},
		{name: "status-v7.schema.json", id: ReviewIntegrationStatusSchemaIDV7},
		{name: "capabilities.schema.json", id: ReviewIntegrationCapabilitiesSchemaIDV2},
		{name: "capabilities-v2.1.schema.json", id: ReviewIntegrationCapabilitiesSchemaIDV21},
		{name: "capabilities-v2.2.schema.json", id: ReviewIntegrationCapabilitiesSchemaIDV22},
		{name: "capabilities-v2.3.schema.json", id: ReviewIntegrationCapabilitiesSchemaIDV23},
		{name: "capabilities-v2.4.schema.json", id: ReviewIntegrationCapabilitiesSchemaIDV24},
		{name: "capabilities-v2.5.schema.json", id: ReviewIntegrationCapabilitiesSchemaIDV25},
		{name: "intended-untracked-selection.schema.json", id: reviewIntendedUntrackedSelectionSchema},
		{name: "consent.schema.json", id: ReviewIntegrationConsentSchemaIDV2},
		{name: "consent-v3.schema.json", id: ReviewIntegrationConsentSchemaIDV3},
		{name: "failure.schema.json", id: ReviewIntegrationFailureSchemaIDV2},
		{name: "operation.schema.json", id: ReviewIntegrationOperationSchemaIDV2},
		{name: "repair.schema.json", id: ReviewIntegrationRepairSchemaIDV2},
		{name: "gate-result.schema.json", id: "https://gentle-ai.dev/contracts/review-integration/v2/schemas/gate-result.schema.json"},
		{name: "last-event-closure.schema.json", id: "https://gentle-ai.dev/contracts/review-integration/v2/schemas/last-event-closure.schema.json"},
		{name: "opencode-provider-role.schema.json", id: "https://gentle-ai.dev/contracts/review-integration/v2/schemas/opencode-provider-role.schema.json"},
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
	startV4Payload, err := os.ReadFile(filepath.Join(root, "start-v4.fixture.json"))
	if err != nil {
		t.Fatal(err)
	}
	var startV4 ReviewIntegrationStartResult
	if err := json.Unmarshal(startV4Payload, &startV4); err != nil {
		t.Fatal(err)
	}
	if err := startV4.Validate(); err != nil {
		t.Fatalf("v4 START fixture: %v", err)
	}
	if startV4.NextTransition == nil || startV4.NextTransition.Execute == nil ||
		startV4.NextTransition.Execute.Operation != "review.status" {
		t.Fatalf("v4 START fixture continuation = %#v", startV4.NextTransition)
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
	v5StatusPayload, err := os.ReadFile(filepath.Join(root, "status-v5.fixture.json"))
	if err != nil {
		t.Fatal(err)
	}
	var v5Status ReviewTargetStatusResult
	if err := json.Unmarshal(v5StatusPayload, &v5Status); err != nil {
		t.Fatal(err)
	}
	if err := v5Status.Validate(); err != nil {
		t.Fatalf("v5 STATUS fixture: %v", err)
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
