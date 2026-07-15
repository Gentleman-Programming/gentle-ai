package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"runtime/debug"
	"slices"
	"strings"
	"testing"
)

const capabilityFixtureExecutable = "gentle-ai capability fixture\n"

func TestReviewCapabilitiesMatchesConformanceFixtureOutsideRepository(t *testing.T) {
	fixturePath, err := filepath.Abs(filepath.Join("..", "..", "contracts", "review-integration", "v1", "fixtures", "capabilities.fixture.json"))
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
	entries, err := os.ReadDir(outside)
	if err != nil || len(entries) != 0 {
		t.Fatalf("capabilities mutated outside-repository directory: entries=%v err=%v", entries, err)
	}
}

func TestReviewCapabilitiesContractValidationIsExactAndReadOnly(t *testing.T) {
	tests := []struct {
		name     string
		contract string
		wantErr  bool
	}{
		{name: "supported", contract: ReviewIntegrationContractV1},
		{name: "empty", contract: "", wantErr: true},
		{name: "future major", contract: "gentle-ai.review-integration/v2", wantErr: true},
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
	err := RunReview([]string{"capabilities", "--contract", "gentle-ai.review-integration/v2"}, &output)
	if err == nil {
		t.Fatalf("unsupported contract result = %q, %v", output.String(), err)
	}
	if failure := decodeReviewIntegrationFailure(t, output.Bytes()); failure.Code != "unsupported_contract" {
		t.Fatalf("unsupported contract failure = %#v", failure)
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
		"review.bind_sdd", "review.capabilities", "review.finalize", "review.start", "review.status", "review.validate",
	}
	wantGates := []string{"post-apply", "pre-commit", "pre-push", "pre-pr", "release"}
	wantProjections := []string{"staged", "workspace"}
	if !slices.Equal(result.Operations, wantOperations) || !slices.Equal(result.Gates, wantGates) || !slices.Equal(result.Projections, wantProjections) {
		t.Fatalf("capability surface = operations %v gates %v projections %v", result.Operations, result.Gates, result.Projections)
	}
	if !slices.Contains(result.Schemas, ReviewIntegrationOperationSchema) || !slices.Contains(result.Schemas, ReviewIntegrationStartSchema) || !slices.Contains(result.Schemas, ReviewIntegrationStatusSchema) || !slices.Contains(result.Schemas, ReviewIntegrationProjectionSchema) {
		t.Fatalf("capability schemas do not advertise negotiated operations/START/status/projection: %v", result.Schemas)
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
	schemaPayload, err := os.ReadFile(filepath.Join(root, "schemas", "capabilities.schema.json"))
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
	fixture, err := os.ReadFile(filepath.Join(root, "fixtures", "capabilities.fixture.json"))
	if err != nil {
		t.Fatal(err)
	}
	decoder := json.NewDecoder(bytes.NewReader(fixture))
	decoder.DisallowUnknownFields()
	var result ReviewCapabilitiesResult
	if err := decoder.Decode(&result); err != nil {
		t.Fatal(err)
	}
	if err := result.Validate(); err != nil {
		t.Fatal(err)
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

func stubReviewCapabilityIdentity(t *testing.T, executable string) func() {
	t.Helper()
	previousVersion := AppVersion
	previousExecutable := reviewCapabilitiesExecutablePath
	previousBuildInfo := reviewCapabilitiesBuildInfoReader
	AppVersion = "2.1.6-test"
	reviewCapabilitiesExecutablePath = func() (string, error) { return executable, nil }
	reviewCapabilitiesBuildInfoReader = func() (*debug.BuildInfo, bool) {
		return &debug.BuildInfo{
			GoVersion: "go1.25.10",
			Main:      debug.Module{Path: "github.com/gentleman-programming/gentle-ai", Version: "v2.1.6"},
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
		{Name: "risk_reasons", Supported: true, Requires: []string{"repository_independent_capabilities"}},
	}
	if !reflect.DeepEqual(result.Features.Mandatory, wantMandatory) || !reflect.DeepEqual(result.Features.Optional, wantOptional) {
		t.Fatalf("feature requirements = %#v", result.Features)
	}
	if result.Compatibility.LegacyWindow.State != "active" || !result.Compatibility.LegacyWindow.ReadOnly || !result.Compatibility.LegacyWindow.DeprecationStarted || result.Compatibility.LegacyWindow.Removal != "not-scheduled" || result.Compatibility.LegacyWindow.MinimumCompatibilityReleases != 1 {
		t.Fatalf("legacy compatibility window = %#v", result.Compatibility.LegacyWindow)
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
		"six strict JSON Schemas", "eight deterministic conformance fixtures",
		"Legacy-v1 never reports `publication_pending`", "retry and replay disabled",
		"Historical `ordinary_4r` legacy status omits `frozen`", "START, finalize, BIND-SDD, invalidation, and direct append",
	} {
		if !strings.Contains(document, required) {
			t.Fatalf("review integration documentation is missing %q", required)
		}
	}
	for _, stale := range []string{"five strict JSON Schemas", "four deterministic conformance fixtures"} {
		if strings.Contains(document, stale) {
			t.Fatalf("review integration documentation retains stale claim %q", stale)
		}
	}
}
