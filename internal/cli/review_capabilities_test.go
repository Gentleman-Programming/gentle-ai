package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"runtime/debug"
	"slices"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
)

const capabilityFixtureExecutable = "gentle-ai capability fixture\n"

func TestReviewCapabilitiesMatchesConformanceFixtureOutsideRepository(t *testing.T) {
	fixturePath, err := filepath.Abs(filepath.Join("..", "..", "contracts", "review-integration", "v1", "fixtures", "capabilities-v1.5.fixture.json"))
	if err != nil {
		t.Fatal(err)
	}
	fixture, err := os.ReadFile(fixturePath)
	if err != nil {
		t.Fatal(err)
	}
	executable := filepath.Join(t.TempDir(), "gentle-ai-fixture")
	if err := os.WriteFile(executable, []byte(capabilityFixtureExecutable), 0o755); err != nil {
		t.Fatal(err)
	}
	restore := stubReviewCapabilityIdentity(t, executable)
	defer restore()
	outside := t.TempDir()
	if _, err := os.Stat(filepath.Join(outside, ".git")); !os.IsNotExist(err) {
		t.Fatalf("outside runtime directory unexpectedly contains Git metadata: %v", err)
	}
	previous, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(outside); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(previous) }()

	var first, second bytes.Buffer
	args := []string{"capabilities", "--contract", ReviewIntegrationContractV1}
	if err := RunReview(args, &first); err != nil {
		t.Fatal(err)
	}
	if err := RunReview(args, &second); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(first.Bytes(), second.Bytes()) {
		t.Fatalf("capabilities changed across identical calls:\nfirst=%s\nsecond=%s", first.String(), second.String())
	}
	if !bytes.Equal(first.Bytes(), fixture) {
		t.Fatalf("capabilities do not match conformance fixture:\ngot=%s\nwant=%s", first.String(), fixture)
	}
	var result ReviewCapabilitiesResult
	decoder := json.NewDecoder(bytes.NewReader(first.Bytes()))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&result); err != nil {
		t.Fatal(err)
	}
	if err := result.Validate(); err != nil {
		t.Fatalf("capabilities validation: %v", err)
	}
	if result.Protocol != (ReviewCapabilitiesProtocol{Major: 1, Minor: 5}) ||
		!slices.ContainsFunc(result.Features.Optional, func(feature ReviewCapabilityFeature) bool {
			return feature.Name == "native_frozen_candidate_context" && feature.Supported && slices.Equal(feature.Requires, []string{"immutable_snapshot"})
		}) || !slices.ContainsFunc(result.Features.Optional, func(feature ReviewCapabilityFeature) bool {
		return feature.Name == "classified_authority_repair" && feature.Supported && slices.Equal(feature.Requires, []string{"native_next_transition", "uniform_failure_envelope"})
	}) {
		t.Fatalf("capabilities do not advertise the current optional surface: %#v", result)
	}
	entries, err := os.ReadDir(outside)
	if err != nil || len(entries) != 0 {
		t.Fatalf("capabilities mutated outside-repository directory: entries=%v err=%v", entries, err)
	}
}

func TestReviewCapabilitiesV2MatchesConformanceFixture(t *testing.T) {
	fixture, err := os.ReadFile(filepath.Join("..", "..", "contracts", "review-integration", "v2", "fixtures", "capabilities.fixture.json"))
	if err != nil {
		t.Fatal(err)
	}
	executable := filepath.Join(t.TempDir(), "gentle-ai-fixture")
	if err := os.WriteFile(executable, []byte(capabilityFixtureExecutable), 0o755); err != nil {
		t.Fatal(err)
	}
	restore := stubReviewCapabilityIdentity(t, executable)
	defer restore()
	var output bytes.Buffer
	if err := RunReview([]string{"capabilities", "--contract", ReviewIntegrationContractV2}, &output); err != nil {
		t.Fatal(err)
	}
	var got, want ReviewCapabilitiesResult
	if err := json.Unmarshal(output.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(fixture, &want); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("v2 capabilities do not match conformance fixture:\ngot=%#v\nwant=%#v", got, want)
	}
}

func TestReviewCapabilitiesContractValidationIsExactAndReadOnly(t *testing.T) {
	tests := []struct {
		name     string
		contract string
		wantErr  bool
	}{
		{name: "supported", contract: ReviewIntegrationContractV1},
		{name: "native Git", contract: ReviewIntegrationContractV2},
		{name: "empty", contract: "", wantErr: true},
		{name: "future major", contract: "gentle-ai.review-integration/v3", wantErr: true},
		{name: "surrounding whitespace", contract: " " + ReviewIntegrationContractV1, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := validateReviewIntegrationContract(tt.contract); (err != nil) != tt.wantErr {
				t.Fatalf("validateReviewIntegrationContract(%q) error = %v, wantErr %t", tt.contract, err, tt.wantErr)
			}
		})
	}
	outside := t.TempDir()
	var output bytes.Buffer
	err := RunReview([]string{"capabilities", "--contract", ReviewIntegrationContractV2}, &output)
	if err != nil {
		t.Fatal(err)
	}
	var nativeGit ReviewCapabilitiesResult
	if err := json.Unmarshal(output.Bytes(), &nativeGit); err != nil || nativeGit.Contract != ReviewIntegrationContractV2 || nativeGit.Schema != ReviewIntegrationCapabilitiesSchemaV2 {
		t.Fatalf("native Git capabilities = %#v, %v", nativeGit, err)
	}
	entries, readErr := os.ReadDir(outside)
	if readErr != nil || len(entries) != 0 {
		t.Fatalf("unsupported capability request mutated directory: %v, %v", entries, readErr)
	}
}

func TestReviewCapabilitiesAdvertisesOnlyNativeSurface(t *testing.T) {
	executable := filepath.Join(t.TempDir(), "gentle-ai-fixture")
	if err := os.WriteFile(executable, []byte(capabilityFixtureExecutable), 0o755); err != nil {
		t.Fatal(err)
	}
	restore := stubReviewCapabilityIdentity(t, executable)
	defer restore()
	var output bytes.Buffer
	if err := RunReviewCapabilities([]string{"--contract", ReviewIntegrationContractV1}, &output); err != nil {
		t.Fatal(err)
	}
	var result ReviewCapabilitiesResult
	if err := json.Unmarshal(output.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	wantOperations := []string{
		"review.bind_sdd", "review.capabilities", "review.finalize", "review.repair", "review.retry_final_verification", "review.start", "review.status", "review.validate",
	}
	wantGates := []string{"post-apply", "pre-commit", "pre-push", "pre-pr", "release"}
	wantProjections := []string{"staged", "workspace"}
	if !slices.Equal(result.Operations, wantOperations) || !slices.Equal(result.Gates, wantGates) || !slices.Equal(result.Projections, wantProjections) {
		t.Fatalf("capability surface = operations %v gates %v projections %v", result.Operations, result.Gates, result.Projections)
	}
	if !slices.Contains(result.Schemas, reviewResultArtifactSchema) || !slices.Contains(result.Schemas, ReviewIntegrationOperationSchema) || !slices.Contains(result.Schemas, ReviewIntegrationStartSchemaV2) || !slices.Contains(result.Schemas, ReviewIntegrationStatusSchemaV2) || !slices.Contains(result.Schemas, ReviewIntegrationProjectionSchema) || !slices.Contains(result.Schemas, ReviewIntegrationRepairSchema) || !slices.Contains(result.Schemas, reviewtransaction.AuthorityRepairAssessmentSchema) || !slices.Contains(result.Schemas, reviewtransaction.FinalVerificationIncidentSchema) {
		t.Fatalf("capability schemas do not advertise the negotiated provider surface: %v", result.Schemas)
	}
	if result.Bootstrap == nil || result.Bootstrap.Command != "gentle-ai review status --cwd <repo> --contract gentle-ai.review-integration/v1 --next-transition" ||
		result.Bootstrap.RequiredFeature != "native_next_transition" || result.Bootstrap.UnsupportedOutcome != "unsupported-capability" || !result.Bootstrap.ParentOnly ||
		len(result.Bootstrap.TargetSelectorVariants) != 4 {
		t.Fatalf("capability bootstrap = %#v", result.Bootstrap)
	}
	if slices.Contains(result.Operations, "review.capture_result") {
		t.Fatal("headless capture-result was advertised as a negotiated repository operation")
	}
	if result.Executable.Evidence != "self-reported" || result.Executable.Verification != "compare-with-published-manifest" || result.Executable.SHA256 != "sha256:dcc846103b16d365eaeeb9d7f289c23fc4f2897f23def1cb3fe7f05557b64705" {
		t.Fatalf("executable identity = %#v", result.Executable)
	}
	var document any
	if err := json.Unmarshal(output.Bytes(), &document); err != nil {
		t.Fatal(err)
	}
	forbidden := map[string]struct{}{"model": {}, "provider": {}, "profile": {}, "cwd": {}, "repository": {}, "store_path": {}, "executable_path": {}}
	if field := findCapabilityForbiddenField(document, forbidden); field != "" {
		t.Fatalf("capabilities exposed forbidden field %q", field)
	}
}

func TestReviewCapabilitiesSchemaAndFixtureAreStrict(t *testing.T) {
	root := filepath.Join("..", "..", "contracts", "review-integration", "v1")
	schemaPayload, err := os.ReadFile(filepath.Join(root, "schemas", "capabilities-v1.5.schema.json"))
	if err != nil {
		t.Fatal(err)
	}
	var schema map[string]any
	if err := json.Unmarshal(schemaPayload, &schema); err != nil {
		t.Fatal(err)
	}
	if schema["$schema"] != "https://json-schema.org/draft/2020-12/schema" || schema["$id"] != ReviewIntegrationCapabilitiesSchemaID || schema["additionalProperties"] != false {
		t.Fatalf("capabilities schema header = %#v", schema)
	}
	fixture, err := os.ReadFile(filepath.Join(root, "fixtures", "capabilities-v1.5.fixture.json"))
	if err != nil {
		t.Fatal(err)
	}
	validateFinalVerificationContractSchema(t, "capabilities-v1.5.schema.json", fixture)
	decoder := json.NewDecoder(bytes.NewReader(fixture))
	decoder.DisallowUnknownFields()
	var result ReviewCapabilitiesResult
	if err := decoder.Decode(&result); err != nil {
		t.Fatal(err)
	}
	if err := result.Validate(); err != nil {
		t.Fatal(err)
	}
	featureSchema := schema["properties"].(map[string]any)["features"].(map[string]any)["properties"].(map[string]any)
	optionalSchema := featureSchema["optional"].(map[string]any)
	featureNames := schema["$defs"].(map[string]any)["feature"].(map[string]any)["properties"].(map[string]any)["name"].(map[string]any)["enum"].([]any)
	accepted := make(map[string]bool, len(featureNames))
	for _, name := range featureNames {
		accepted[name.(string)] = true
	}
	for _, surface := range [][]ReviewCapabilityFeature{result.Features.Optional, reviewCapabilitiesStaticSurface().Features.Optional} {
		if optionalSchema["minItems"] != float64(len(surface)) || optionalSchema["maxItems"] != float64(len(surface)) {
			t.Fatalf("optional feature bounds do not accept provider surface: %#v", optionalSchema)
		}
		for _, feature := range surface {
			if !accepted[feature.Name] {
				t.Fatalf("feature schema rejects %q", feature.Name)
			}
		}
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
	decoder = json.NewDecoder(bytes.NewReader(malformed))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&ReviewCapabilitiesResult{}); err == nil {
		t.Fatal("strict capabilities decoder accepted unknown top-level field")
	}
}

func TestReviewCapabilitiesVersionsKeepV1ReadableAndFailClosedAcrossSchemas(t *testing.T) {
	root := filepath.Join("..", "..", "contracts", "review-integration", "v1")
	legacySchemaPayload, err := os.ReadFile(filepath.Join(root, "schemas", "capabilities.schema.json"))
	if err != nil {
		t.Fatal(err)
	}
	var legacySchema map[string]any
	if err := json.Unmarshal(legacySchemaPayload, &legacySchema); err != nil {
		t.Fatal(err)
	}
	if legacySchema["$id"] != ReviewIntegrationCapabilitiesSchemaIDV1 {
		t.Fatalf("legacy capabilities schema identity = %#v", legacySchema["$id"])
	}
	v11SchemaPayload, err := os.ReadFile(filepath.Join(root, "schemas", "capabilities-v1.1.schema.json"))
	if err != nil {
		t.Fatal(err)
	}
	var v11Schema map[string]any
	if err := json.Unmarshal(v11SchemaPayload, &v11Schema); err != nil {
		t.Fatal(err)
	}
	if v11Schema["$id"] != ReviewIntegrationCapabilitiesSchemaIDV11 {
		t.Fatalf("v1.1 capabilities schema identity = %#v", v11Schema["$id"])
	}
	v12SchemaPayload, err := os.ReadFile(filepath.Join(root, "schemas", "capabilities-v1.2.schema.json"))
	if err != nil {
		t.Fatal(err)
	}
	var v12Schema map[string]any
	if err := json.Unmarshal(v12SchemaPayload, &v12Schema); err != nil {
		t.Fatal(err)
	}
	if v12Schema["$id"] != ReviewIntegrationCapabilitiesSchemaIDV12 {
		t.Fatalf("v1.2 capabilities schema identity = %#v", v12Schema["$id"])
	}
	v13SchemaPayload, err := os.ReadFile(filepath.Join(root, "schemas", "capabilities-v1.3.schema.json"))
	if err != nil {
		t.Fatal(err)
	}
	var v13Schema map[string]any
	if err := json.Unmarshal(v13SchemaPayload, &v13Schema); err != nil {
		t.Fatal(err)
	}
	if v13Schema["$id"] != ReviewIntegrationCapabilitiesSchemaIDV13 {
		t.Fatalf("v1.3 capabilities schema identity = %#v", v13Schema["$id"])
	}
	v14SchemaPayload, err := os.ReadFile(filepath.Join(root, "schemas", "capabilities-v1.4.schema.json"))
	if err != nil {
		t.Fatal(err)
	}
	var v14Schema map[string]any
	if err := json.Unmarshal(v14SchemaPayload, &v14Schema); err != nil {
		t.Fatal(err)
	}
	if v14Schema["$id"] != ReviewIntegrationCapabilitiesSchemaIDV14 {
		t.Fatalf("v1.4 capabilities schema identity = %#v", v14Schema["$id"])
	}
	legacyFixture, err := os.ReadFile(filepath.Join(root, "fixtures", "capabilities.fixture.json"))
	if err != nil {
		t.Fatal(err)
	}
	v11Fixture, err := os.ReadFile(filepath.Join(root, "fixtures", "capabilities-v1.1.fixture.json"))
	if err != nil {
		t.Fatal(err)
	}
	v12Fixture, err := os.ReadFile(filepath.Join(root, "fixtures", "capabilities-v1.2.fixture.json"))
	if err != nil {
		t.Fatal(err)
	}
	v13Fixture, err := os.ReadFile(filepath.Join(root, "fixtures", "capabilities-v1.3.fixture.json"))
	if err != nil {
		t.Fatal(err)
	}
	v14Fixture, err := os.ReadFile(filepath.Join(root, "fixtures", "capabilities-v1.4.fixture.json"))
	if err != nil {
		t.Fatal(err)
	}
	currentFixture, err := os.ReadFile(filepath.Join(root, "fixtures", "capabilities-v1.5.fixture.json"))
	if err != nil {
		t.Fatal(err)
	}
	type identity struct {
		Schema   string                     `json:"schema"`
		Protocol ReviewCapabilitiesProtocol `json:"protocol"`
	}
	validateLegacy := func(payload []byte) error {
		var value identity
		if err := json.Unmarshal(payload, &value); err != nil {
			return err
		}
		if value.Schema != ReviewIntegrationCapabilitiesSchemaV1 || value.Protocol != (ReviewCapabilitiesProtocol{Major: 1, Minor: 0}) {
			return errors.New("unknown legacy capabilities schema")
		}
		return nil
	}
	if err := validateLegacy(legacyFixture); err != nil {
		t.Fatalf("legacy consumer rejected v1.0 artifact: %v", err)
	}
	if err := validateLegacy(v11Fixture); err == nil {
		t.Fatal("legacy consumer accepted unknown v1.1 capabilities schema")
	}
	if err := validateLegacy(v12Fixture); err == nil {
		t.Fatal("legacy consumer accepted unknown v1.2 capabilities schema")
	}
	if err := validateLegacy(v13Fixture); err == nil {
		t.Fatal("legacy consumer accepted unknown v1.3 capabilities schema")
	}
	if err := validateLegacy(v14Fixture); err == nil {
		t.Fatal("legacy consumer accepted unknown v1.4 capabilities schema")
	}
	if err := validateLegacy(currentFixture); err == nil {
		t.Fatal("legacy consumer accepted unknown v1.5 capabilities schema")
	}
	validateV11 := func(payload []byte) error {
		var value identity
		if err := json.Unmarshal(payload, &value); err != nil {
			return err
		}
		if value.Schema != ReviewIntegrationCapabilitiesSchemaV11 || value.Protocol != (ReviewCapabilitiesProtocol{Major: 1, Minor: 1}) {
			return errors.New("unknown v1.1 capabilities schema")
		}
		return nil
	}
	if err := validateV11(v11Fixture); err != nil {
		t.Fatalf("v1.1 consumer rejected v1.1 artifact: %v", err)
	}
	if err := validateV11(v12Fixture); err == nil {
		t.Fatal("v1.1 consumer accepted unknown v1.2 capabilities schema")
	}
	if err := validateV11(v13Fixture); err == nil {
		t.Fatal("v1.1 consumer accepted unknown v1.3 capabilities schema")
	}
	if err := validateV11(v14Fixture); err == nil {
		t.Fatal("v1.1 consumer accepted unknown v1.4 capabilities schema")
	}
	if err := validateV11(currentFixture); err == nil {
		t.Fatal("v1.1 consumer accepted unknown v1.5 capabilities schema")
	}
	validateV12 := func(payload []byte) error {
		var value identity
		if err := json.Unmarshal(payload, &value); err != nil {
			return err
		}
		if value.Schema != ReviewIntegrationCapabilitiesSchemaV12 || value.Protocol != (ReviewCapabilitiesProtocol{Major: 1, Minor: 2}) {
			return errors.New("unknown v1.2 capabilities schema")
		}
		return nil
	}
	if err := validateV12(v12Fixture); err != nil {
		t.Fatalf("v1.2 consumer rejected v1.2 artifact: %v", err)
	}
	if err := validateV12(v13Fixture); err == nil {
		t.Fatal("v1.2 consumer accepted unknown v1.3 capabilities schema")
	}
	if err := validateV12(v14Fixture); err == nil {
		t.Fatal("v1.2 consumer accepted unknown v1.4 capabilities schema")
	}
	if err := validateV12(currentFixture); err == nil {
		t.Fatal("v1.2 consumer accepted unknown v1.5 capabilities schema")
	}
	validateV13 := func(payload []byte) error {
		var value identity
		if err := json.Unmarshal(payload, &value); err != nil {
			return err
		}
		if value.Schema != ReviewIntegrationCapabilitiesSchemaV13 || value.Protocol != (ReviewCapabilitiesProtocol{Major: 1, Minor: 3}) {
			return errors.New("unknown v1.3 capabilities schema")
		}
		return nil
	}
	if err := validateV13(v13Fixture); err != nil {
		t.Fatalf("v1.3 consumer rejected v1.3 artifact: %v", err)
	}
	if err := validateV13(v14Fixture); err == nil {
		t.Fatal("v1.3 consumer accepted unknown v1.4 capabilities schema")
	}
	if err := validateV13(currentFixture); err == nil {
		t.Fatal("v1.3 consumer accepted unknown v1.5 capabilities schema")
	}
	validateV14 := func(payload []byte) error {
		var value identity
		if err := json.Unmarshal(payload, &value); err != nil {
			return err
		}
		if value.Schema != ReviewIntegrationCapabilitiesSchemaV14 || value.Protocol != (ReviewCapabilitiesProtocol{Major: 1, Minor: 4}) {
			return errors.New("unknown v1.4 capabilities schema")
		}
		return nil
	}
	if err := validateV14(v14Fixture); err != nil {
		t.Fatalf("v1.4 consumer rejected v1.4 artifact: %v", err)
	}
	if err := validateV14(currentFixture); err == nil {
		t.Fatal("v1.4 consumer accepted unknown v1.5 capabilities schema")
	}
	var current ReviewCapabilitiesResult
	if err := json.Unmarshal(currentFixture, &current); err != nil {
		t.Fatal(err)
	}
	if err := current.Validate(); err != nil {
		t.Fatalf("v1.5 consumer rejected negotiated artifact: %v", err)
	}
	for name, payload := range map[string][]byte{"v1.0": legacyFixture, "v1.1": v11Fixture, "v1.2": v12Fixture, "v1.3": v13Fixture, "v1.4": v14Fixture} {
		var previous ReviewCapabilitiesResult
		if err := json.Unmarshal(payload, &previous); err != nil {
			t.Fatal(err)
		}
		if err := previous.Validate(); err == nil {
			t.Fatalf("v1.5 consumer accepted %s artifact without explicit negotiation", name)
		}
	}
}

func stubReviewCapabilityIdentity(t *testing.T, executable string) func() {
	t.Helper()
	previousVersion := AppVersion
	previousExecutable := reviewCapabilitiesExecutablePath
	previousBuildInfo := reviewCapabilitiesBuildInfoReader
	AppVersion = "2.1.7-test"
	reviewCapabilitiesExecutablePath = func() (string, error) { return executable, nil }
	reviewCapabilitiesBuildInfoReader = func() (*debug.BuildInfo, bool) {
		return &debug.BuildInfo{
			GoVersion: "go1.25.10",
			Main:      debug.Module{Path: "github.com/gentleman-programming/gentle-ai/v2", Version: "v2.1.7"},
			Settings: []debug.BuildSetting{
				{Key: "vcs", Value: "git"},
				{Key: "vcs.revision", Value: "0123456789abcdef0123456789abcdef01234567"},
				{Key: "vcs.time", Value: "2026-07-15T18:00:00Z"},
				{Key: "vcs.modified", Value: "false"},
			},
		}, true
	}
	return func() {
		AppVersion = previousVersion
		reviewCapabilitiesExecutablePath = previousExecutable
		reviewCapabilitiesBuildInfoReader = previousBuildInfo
	}
}

func findCapabilityForbiddenField(value any, forbidden map[string]struct{}) string {
	switch typed := value.(type) {
	case map[string]any:
		for key, child := range typed {
			if _, found := forbidden[strings.ToLower(key)]; found {
				return key
			}
			if found := findCapabilityForbiddenField(child, forbidden); found != "" {
				return found
			}
		}
	case []any:
		for _, child := range typed {
			if found := findCapabilityForbiddenField(child, forbidden); found != "" {
				return found
			}
		}
	}
	return ""
}

func TestReviewCapabilitiesFeatureRequirementsAreExplicit(t *testing.T) {
	executable := filepath.Join(t.TempDir(), "gentle-ai-fixture")
	if err := os.WriteFile(executable, []byte(capabilityFixtureExecutable), 0o755); err != nil {
		t.Fatal(err)
	}
	restore := stubReviewCapabilityIdentity(t, executable)
	defer restore()
	result, err := buildReviewCapabilities()
	if err != nil {
		t.Fatal(err)
	}
	wantMandatory := []ReviewCapabilityFeature{
		{Name: "compact_v2_authority", Supported: true, Requires: []string{}},
		{Name: "exact_receipt_replay", Supported: true, Requires: []string{"compact_v2_authority"}},
		{Name: "five_delivery_gates", Supported: true, Requires: []string{"compact_v2_authority"}},
		{Name: "immutable_snapshot", Supported: true, Requires: []string{}},
		{Name: "legacy_v1_target_scoped_read_only", Supported: true, Requires: []string{"target_scoped_status"}},
		{Name: "repository_independent_capabilities", Supported: true, Requires: []string{}},
		{Name: "restart_safe_projection", Supported: true, Requires: []string{"target_scoped_status"}},
		{Name: "sdd_receipt_binding", Supported: true, Requires: []string{"compact_v2_authority"}},
		{Name: "target_scoped_status", Supported: true, Requires: []string{"repository_independent_capabilities"}},
		{Name: "uniform_failure_envelope", Supported: true, Requires: []string{"repository_independent_capabilities"}},
	}
	wantOptional := []ReviewCapabilityFeature{
		{Name: "base_ref_workspace_overlay", Supported: true, Requires: []string{"immutable_snapshot", "restart_safe_projection"}},
		{Name: "bounded_process_waits", Supported: true, Requires: []string{"uniform_failure_envelope"}},
		{Name: "classified_authority_repair", Supported: true, Requires: []string{"native_next_transition", "uniform_failure_envelope"}},
		{Name: "exact_gate_receipt_discovery", Supported: true, Requires: []string{"five_delivery_gates"}},
		{Name: "native_frozen_candidate_context", Supported: true, Requires: []string{"immutable_snapshot"}},
		{Name: "native_low_risk_verification", Supported: true, Requires: []string{"compact_v2_authority"}},
		{Name: "native_next_transition", Supported: true, Requires: []string{"target_scoped_status"}},
		{Name: "one_shot_final_verification_retry", Supported: true, Requires: []string{"compact_v2_authority", "exact_receipt_replay", "native_next_transition"}},
		{Name: "opaque_repository_context", Supported: true, Requires: []string{"compact_v2_authority", "native_next_transition"}},
		{Name: "outcome_bound_verification_evidence", Supported: true, Requires: []string{"compact_v2_authority", "native_next_transition"}},
		{Name: "provider_artifact_admission", Supported: true, Requires: []string{"compact_v2_authority", "native_frozen_candidate_context", "opaque_repository_context"}},
		{Name: "provider_targeted_validation_request", Supported: true, Requires: []string{"compact_v2_authority", "native_next_transition"}},
		{Name: "recovered_correction_evidence", Supported: true, Requires: []string{"compact_v2_authority", "provider_targeted_validation_request"}},
		{Name: "risk_reasons", Supported: true, Requires: []string{"repository_independent_capabilities"}},
		{Name: "scope_change_diagnostics", Supported: true, Requires: []string{"uniform_failure_envelope"}},
		{Name: "validating_result_reopen", Supported: true, Requires: []string{"compact_v2_authority", "provider_artifact_admission"}},
	}
	if !reflect.DeepEqual(result.Features.Mandatory, wantMandatory) || !reflect.DeepEqual(result.Features.Optional, wantOptional) {
		t.Fatalf("feature requirements = %#v", result.Features)
	}
	if result.Compatibility.LegacyWindow.State != "active" || !result.Compatibility.LegacyWindow.ReadOnly || !result.Compatibility.LegacyWindow.DeprecationStarted || result.Compatibility.LegacyWindow.Removal != "not-scheduled" || result.Compatibility.LegacyWindow.MinimumCompatibilityReleases != 1 {
		t.Fatalf("legacy compatibility window = %#v", result.Compatibility.LegacyWindow)
	}
	result.Features.Optional = result.Features.Optional[3:4]
	if err := result.Validate(); err == nil {
		t.Fatal("capabilities accepted an incomplete optional feature surface")
	}
}

func TestReviewCapabilitiesBootstrapIsOptionalForExistingV1Consumers(t *testing.T) {
	executable := filepath.Join(t.TempDir(), "gentle-ai-fixture")
	if err := os.WriteFile(executable, []byte(capabilityFixtureExecutable), 0o755); err != nil {
		t.Fatal(err)
	}
	restore := stubReviewCapabilityIdentity(t, executable)
	defer restore()
	legacy, err := buildReviewCapabilities()
	if err != nil {
		t.Fatal(err)
	}
	legacy.Bootstrap = nil
	if err := legacy.Validate(); err != nil {
		t.Fatalf("existing v1 capability surface rejected: %v", err)
	}
}

func TestReviewIntegrationDocumentationMatchesRuntimeContract(t *testing.T) {
	payload, err := os.ReadFile(filepath.Join("..", "..", "docs", "review-integration.md"))
	if err != nil {
		t.Fatal(err)
	}
	document := string(payload)
	for _, required := range []string{
		"`stop`", "`legacy_v1_read_only`", "`mutation_outcome`", "`not_started`", "`unknown`", "`committed`",
		"Legacy-v1 never reports `publication_pending`", "retry and replay disabled",
		"Historical `ordinary_4r` legacy status omits `frozen`", "START, finalize, BIND-SDD, invalidation, and direct append",
		"`native_frozen_candidate_context`", "`base_tree`", "`candidate_tree`", "`changed_path_manifest`",
		"`opaque_repository_context`", "`provider_targeted_validation_request`",
		"`provider_artifact_admission`", "`validating_result_reopen`", "`recovered_correction_evidence`",
		"`one_shot_final_verification_retry`", "`outcome_bound_verification_evidence`", "`review.retry_final_verification`", "`procedural_tooling_failure`",
		"`artifact_subjects`", "`subject_hash`", "`admission_decision: completed`",
		"`native_low_risk_verification`", "`selected_lenses: []`", "`receipt_scope_changed`",
		"25-second aggregate budget", "15-second budget", "20-second budget", "one-second wait delay",
		"Persistent compact `LOCK` JSON is advisory diagnostics", "`context.scope_change`", "`review.recover`",
	} {
		if !strings.Contains(document, required) {
			t.Fatalf("review integration documentation is missing %q", required)
		}
	}
	for _, stale := range []string{"five strict JSON Schemas", "nine strict JSON Schemas", "ten strict JSON Schemas", "sixteen strict JSON Schemas", "eighteen strict JSON Schemas", "twenty strict JSON Schemas", "twenty-two strict JSON Schemas", "eleven deterministic conformance fixtures", "eighteen deterministic conformance fixtures", "nineteen deterministic conformance fixtures", "twenty-one deterministic conformance fixtures", "twenty-four deterministic conformance fixtures", "twenty-six deterministic conformance fixtures"} {
		if strings.Contains(document, stale) {
			t.Fatalf("review integration documentation retains stale claim %q", stale)
		}
	}
}

func TestReviewIntegrationDocumentationMatchesPublishedArtifactInventory(t *testing.T) {
	document := readReviewIntegrationDoc(t)
	for _, inventory := range []struct {
		directory string
		claim     string
	}{
		{directory: "schemas", claim: "strict JSON Schemas"},
		{directory: "fixtures", claim: "deterministic conformance fixtures"},
	} {
		path := filepath.Join("..", "..", "contracts", "review-integration", "v1", inventory.directory)
		entries, err := os.ReadDir(path)
		if err != nil {
			t.Fatal(err)
		}
		count := 0
		for _, entry := range entries {
			if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
				t.Fatalf("published %s inventory contains unexpected entry %q", inventory.directory, entry.Name())
			}
			count++
		}
		want := fmt.Sprintf("%d %s", count, inventory.claim)
		if !strings.Contains(document, want) {
			t.Fatalf("review integration documentation is missing published inventory claim %q", want)
		}
	}
}
