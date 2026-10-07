package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"testing"

	jsonschema "github.com/santhosh-tekuri/jsonschema/v6"
)

// rdd-risk-gated S14 (A3): callers such as gentle-shell detect the optional
// `review start --request-context` and `--escalate-item/--escalate-reason`
// inputs from the negotiated capabilities instead of probing START.
var reviewStartInputCapabilityFeatures = []ReviewCapabilityFeature{
	{Name: "start_agent_escalation", Supported: true, Requires: []string{"risk_reasons"}},
	{Name: "start_request_context", Supported: true, Requires: []string{"compact_v2_authority"}},
}

// rdd-risk-gated S17: the STATUS preflight of those START inputs is its own
// optional feature. The two START features above promise direct START only,
// so a caller forwards the inputs through STATUS only when this one is listed.
var reviewStartOptionsPreflightCapabilityFeature = ReviewCapabilityFeature{
	Name: "start_options_preflight", Supported: true,
	Requires: []string{"native_next_transition", "start_agent_escalation", "start_request_context"},
}

// verify-always-rdd-high S8 phase A: callers detect that START and the STATUS
// preflight accept --lenses/--lenses-reason before sending them, because an
// older binary refuses unknown flags.
var reviewStartLensSelectionCapabilityFeature = ReviewCapabilityFeature{
	Name: "start_lens_selection", Supported: true, Requires: []string{"start_options_preflight"},
}

func TestReviewCapabilitiesAdvertiseStartRequestContextAndEscalation(t *testing.T) {
	tests := []struct {
		name     string
		contract string
		validate func(t *testing.T, payload []byte) *jsonschema.Schema
	}{
		{
			name: "v1.5", contract: ReviewIntegrationContractV1,
			validate: func(t *testing.T, payload []byte) *jsonschema.Schema {
				schema := compileWholePublishedReviewSchema(t, "v1", "capabilities-v1.5.schema.json")
				validatePublishedReviewSchema(t, schema, payload)
				return schema
			},
		},
		{
			name: "v2.6", contract: ReviewIntegrationContractV2,
			validate: func(t *testing.T, payload []byte) *jsonschema.Schema {
				return validateReviewCapabilitiesSchema(t, "capabilities-v2.6.schema.json", ReviewIntegrationCapabilitiesSchemaIDV26, payload)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var output bytes.Buffer
			if err := RunReview([]string{"capabilities", "--contract", tt.contract}, &output); err != nil {
				t.Fatal(err)
			}
			var got ReviewCapabilitiesResult
			decodeStrictReviewJSON(t, output.Bytes(), &got)
			for _, want := range append(slices.Clone(reviewStartInputCapabilityFeatures), reviewStartOptionsPreflightCapabilityFeature, reviewStartLensSelectionCapabilityFeature) {
				if !slices.ContainsFunc(got.Features.Optional, func(feature ReviewCapabilityFeature) bool {
					return feature.Name == want.Name && feature.Supported == want.Supported && slices.Equal(feature.Requires, want.Requires)
				}) {
					t.Fatalf("%s capabilities do not advertise %#v: %#v", tt.name, want, got.Features.Optional)
				}
			}
			schema := tt.validate(t, output.Bytes())
			var document map[string]any
			if err := json.Unmarshal(output.Bytes(), &document); err != nil {
				t.Fatal(err)
			}
			features := document["features"].(map[string]any)
			current := features["optional"].([]any)
			// Released binaries advertise the same contract version without
			// the STATUS preflight (A3, with the two START inputs) or without
			// any of the three. Both historical advertisements remain valid.
			wantHistoricalCount := 13
			if tt.contract == ReviewIntegrationContractV2 {
				wantHistoricalCount = 15
			}
			for _, historical := range []struct {
				removed []string
				count   int
			}{
				{removed: []string{"start_lens_selection"}, count: wantHistoricalCount + 3},
				{removed: []string{"start_lens_selection", "start_options_preflight"}, count: wantHistoricalCount + 2},
				{removed: []string{"start_lens_selection", "start_options_preflight", "start_request_context", "start_agent_escalation"}, count: wantHistoricalCount},
			} {
				features["optional"] = slices.DeleteFunc(slices.Clone(current), func(feature any) bool {
					return slices.Contains(historical.removed, feature.(map[string]any)["name"].(string))
				})
				if got := len(features["optional"].([]any)); got != historical.count {
					t.Fatalf("%s historical optional count without %v = %d, want %d", tt.name, historical.removed, got, historical.count)
				}
				if err := schema.Validate(document); err != nil {
					t.Fatalf("%s schema rejected a historical advertisement without %v: %v", tt.name, historical.removed, err)
				}
			}
			features["optional"] = append(slices.Clone(current), map[string]any{"name": "start_options_preflight_v2", "supported": true, "requires": []any{}})
			if err := schema.Validate(document); err == nil {
				t.Fatalf("%s schema accepted an unknown optional feature name", tt.name)
			}
		})
	}
}

// TestReviewCapabilitiesOlderV2AdvertisementsKeepTheirExactFeatureCount
// proves the in-place schema edit stays additive: capabilities v2.3 through
// v2.5 still pin exactly the 15 optional features their binaries emitted, and
// v2.6 admits the historical advertisements, the two START input features,
// and their STATUS preflight.
func TestReviewCapabilitiesOlderV2AdvertisementsKeepTheirExactFeatureCount(t *testing.T) {
	fixture, err := os.ReadFile(filepath.Join("..", "..", "contracts", "review-integration", "v2", "fixtures", "capabilities-v2.3.fixture.json"))
	if err != nil {
		t.Fatal(err)
	}
	schema := validateReviewCapabilitiesSchema(t, "capabilities-v2.3.schema.json", ReviewIntegrationCapabilitiesSchemaIDV23, fixture)
	var document map[string]any
	if err := json.Unmarshal(fixture, &document); err != nil {
		t.Fatal(err)
	}
	features := document["features"].(map[string]any)
	optional := features["optional"].([]any)
	if len(optional) != 15 {
		t.Fatalf("v2.3 fixture optional features = %d, want 15", len(optional))
	}
	for _, feature := range append(slices.Clone(reviewStartInputCapabilityFeatures), reviewStartOptionsPreflightCapabilityFeature) {
		features["optional"] = append(slices.Clone(optional), map[string]any{"name": feature.Name, "supported": true, "requires": []any{feature.Requires[0]}})
		if err := schema.Validate(document); err == nil {
			t.Fatalf("v2.3 schema accepted a 16th optional feature %q", feature.Name)
		}
	}
	for _, name := range []string{"capabilities-v2.4.schema.json", "capabilities-v2.5.schema.json"} {
		payload, err := os.ReadFile(filepath.Join("..", "..", "contracts", "review-integration", "v2", "schemas", name))
		if err != nil {
			t.Fatal(err)
		}
		var older map[string]any
		if err := json.Unmarshal(payload, &older); err != nil {
			t.Fatal(err)
		}
		if ref := older["properties"].(map[string]any)["features"].(map[string]any)["$ref"]; ref != "capabilities-v2.3.schema.json#/properties/features" {
			t.Fatalf("%s features = %#v, want the v2.3 15-feature block", name, ref)
		}
	}
}
