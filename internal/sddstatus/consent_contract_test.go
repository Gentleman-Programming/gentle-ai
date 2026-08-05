package sddstatus

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"testing"
)

// sddConsentGrantInvocationShape pins the REAL grant invocation as shipped by
// S3 (#2558) plus S5's reconciliation (#2559), the conscious contract change
// S4a's provisional pin announced: #2563 updated this pin, the schema, and
// the fixture together, adding --request-id (found by S3) and
// --change-instance (added by S5) and admitting the optional
// --expected-revision a widening grant chains on:
//
//	gentle-ai sdd-attempt grant --cwd <repo> --change <name> [--expected-revision <rev>] --root <path>... --actor <actor> --reason <reason> --request-id <id> --change-instance <token>
var sddConsentGrantInvocationShape = regexp.MustCompile(
	`^gentle-ai sdd-attempt grant --cwd \S+ --change \S+( --expected-revision \S+)?( --root \S+)+ --actor \S+ --reason \S+ --request-id \S+ --change-instance \S+$`)

// sddConsentDeclineInvocationShape pins the decline re-entry: declining
// persists nothing, so the runnable follow-up is native SDD status for the
// same change.
var sddConsentDeclineInvocationShape = regexp.MustCompile(
	`^gentle-ai sdd-status \S+ --cwd \S+$`)

func TestSDDIntegrationConsentContractsArePinned(t *testing.T) {
	root := filepath.Join("..", "..", "contracts", "sdd-integration", "v1")
	want := map[string]string{
		"fixtures/consent.fixture.json": "aecd0f4d3ae85527a33ebc708a7a49d7f9fda99b13ccabd1fbe9b89bbf2c41e9",
		"schemas/consent.schema.json":   "249e415c09223e75983abf6c55caa0eea57bd3389c23115a123e09da2ce07efb",
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

func TestSDDIntegrationConsentSchemaIsStrictAndBound(t *testing.T) {
	payload, err := os.ReadFile(filepath.Join("..", "..", "contracts", "sdd-integration", "v1", "schemas", "consent.schema.json"))
	if err != nil {
		t.Fatal(err)
	}
	var schema map[string]any
	if err := json.Unmarshal(payload, &schema); err != nil {
		t.Fatal(err)
	}
	if schema["$schema"] != "https://json-schema.org/draft/2020-12/schema" ||
		schema["$id"] != SDDIntegrationConsentSchemaID ||
		schema["additionalProperties"] != false {
		t.Fatalf("consent schema header = %#v", schema)
	}
	properties := schema["properties"].(map[string]any)
	if properties["schema"].(map[string]any)["const"] != SDDIntegrationConsentSchema ||
		properties["contract"].(map[string]any)["const"] != SDDIntegrationContractV1 ||
		properties["operation"].(map[string]any)["const"] != "sdd-attempt.grant" ||
		properties["action"].(map[string]any)["const"] != "consent_required" ||
		properties["blocking"].(map[string]any)["const"] != true {
		t.Fatalf("consent schema identity = %#v", properties)
	}
	choices := properties["choices"].(map[string]any)
	if choices["minItems"] != float64(2) || choices["maxItems"] != float64(2) || choices["items"] != false {
		t.Fatalf("consent schema does not require exactly two choices: %#v", choices)
	}
	defs := schema["$defs"].(map[string]any)
	grantedPattern := defs["choice_granted"].(map[string]any)["properties"].(map[string]any)["invocation"].(map[string]any)["pattern"].(string)
	if _, err := regexp.Compile(grantedPattern); err != nil {
		t.Fatalf("granted invocation pattern does not compile: %v", err)
	}
	declinedPattern := defs["choice_declined"].(map[string]any)["properties"].(map[string]any)["invocation"].(map[string]any)["pattern"].(string)
	if _, err := regexp.Compile(declinedPattern); err != nil {
		t.Fatalf("declined invocation pattern does not compile: %v", err)
	}
}

func TestSDDIntegrationConsentFixtureValidates(t *testing.T) {
	payload, err := os.ReadFile(filepath.Join("..", "..", "contracts", "sdd-integration", "v1", "fixtures", "consent.fixture.json"))
	if err != nil {
		t.Fatal(err)
	}
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	var consent SDDIntegrationConsentResult
	if err := decoder.Decode(&consent); err != nil {
		t.Fatalf("consent fixture no longer decodes into the live type: %v", err)
	}
	if err := consent.Validate(); err != nil {
		t.Fatalf("consent fixture no longer validates: %v", err)
	}

	// The same byte discipline the review consent envelope carries: the live
	// type re-encodes to the exact fixture bytes, so a JSON tag or field-order
	// change cannot ship unnoticed.
	remarshaled, err := json.MarshalIndent(consent, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	remarshaled = append(remarshaled, '\n')
	if !bytes.Equal(remarshaled, payload) {
		t.Fatalf("consent envelope no longer serializes to the shipped fixture bytes\n--- fixture ---\n%s\n--- remarshaled ---\n%s", payload, remarshaled)
	}

	// Provisional S3 pin: the fixture's invocations carry the exact flag sets
	// the future verbs must honor.
	if !sddConsentGrantInvocationShape.MatchString(consent.Choices[0].Invocation) {
		t.Fatalf("granted invocation does not match the provisional S3 grant shape: %q", consent.Choices[0].Invocation)
	}
	if !sddConsentDeclineInvocationShape.MatchString(consent.Choices[1].Invocation) {
		t.Fatalf("declined invocation does not match the status re-entry shape: %q", consent.Choices[1].Invocation)
	}
}

func TestSDDIntegrationConsentValidateRejectsIncompleteEnvelopes(t *testing.T) {
	valid := validSDDConsentResult()
	if err := valid.Validate(); err != nil {
		t.Fatalf("valid envelope: %v", err)
	}
	tests := []struct {
		name   string
		mutate func(result *SDDIntegrationConsentResult)
	}{
		{name: "wrong schema", mutate: func(result *SDDIntegrationConsentResult) { result.Schema = "gentle-ai.review-integration.consent/v1" }},
		{name: "not blocking", mutate: func(result *SDDIntegrationConsentResult) { result.Blocking = false }},
		{name: "missing change name", mutate: func(result *SDDIntegrationConsentResult) { result.Change = "" }},
		{name: "no missing roots", mutate: func(result *SDDIntegrationConsentResult) { result.MissingRoots = nil }},
		{name: "blank missing root", mutate: func(result *SDDIntegrationConsentResult) { result.MissingRoots = []string{" "} }},
		{name: "missing headline", mutate: func(result *SDDIntegrationConsentResult) { result.Headline = "" }},
		{name: "nil evidence", mutate: func(result *SDDIntegrationConsentResult) { result.Evidence = nil }},
		{name: "root absent from evidence", mutate: func(result *SDDIntegrationConsentResult) { result.Evidence = []string{"unrelated"} }},
		{name: "one choice", mutate: func(result *SDDIntegrationConsentResult) { result.Choices = result.Choices[:1] }},
		{name: "swapped answers", mutate: func(result *SDDIntegrationConsentResult) {
			result.Choices[0].Answer, result.Choices[1].Answer = result.Choices[1].Answer, result.Choices[0].Answer
		}},
		{name: "grant invocation missing a root", mutate: func(result *SDDIntegrationConsentResult) {
			result.Choices[0].Invocation = "gentle-ai sdd-attempt grant --cwd /workspace/planning --change multi-repo-rollout --root /workspace/service-a --actor maintainer --reason rollout --request-id grant-1 --change-instance sdd-1"
		}},
		{name: "grant invocation missing actor", mutate: func(result *SDDIntegrationConsentResult) {
			result.Choices[0].Invocation = "gentle-ai sdd-attempt grant --cwd /workspace/planning --change multi-repo-rollout --root /workspace/service-a --root /workspace/service-b --reason rollout --request-id grant-1 --change-instance sdd-1"
		}},
		{name: "grant invocation missing request-id", mutate: func(result *SDDIntegrationConsentResult) {
			result.Choices[0].Invocation = "gentle-ai sdd-attempt grant --cwd /workspace/planning --change multi-repo-rollout --root /workspace/service-a --root /workspace/service-b --actor maintainer --reason rollout --change-instance sdd-1"
		}},
		{name: "grant invocation missing change-instance", mutate: func(result *SDDIntegrationConsentResult) {
			result.Choices[0].Invocation = "gentle-ai sdd-attempt grant --cwd /workspace/planning --change multi-repo-rollout --root /workspace/service-a --root /workspace/service-b --actor maintainer --reason rollout --request-id grant-1"
		}},
		{name: "decline invocation is not status re-entry", mutate: func(result *SDDIntegrationConsentResult) {
			result.Choices[1].Invocation = "gentle-ai review status"
		}},
		{name: "empty choice effect", mutate: func(result *SDDIntegrationConsentResult) { result.Choices[1].Effect = "" }},
		{name: "off path outside status", mutate: func(result *SDDIntegrationConsentResult) { result.OffPath.Command = "rm -rf tasks.md" }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := validSDDConsentResult()
			tt.mutate(&result)
			if err := result.Validate(); err == nil {
				t.Fatal("Validate() accepted an incomplete envelope")
			}
		})
	}
}

func validSDDConsentResult() SDDIntegrationConsentResult {
	payload, err := os.ReadFile(filepath.Join("..", "..", "contracts", "sdd-integration", "v1", "fixtures", "consent.fixture.json"))
	if err != nil {
		panic(err)
	}
	var consent SDDIntegrationConsentResult
	if err := json.Unmarshal(payload, &consent); err != nil {
		panic(err)
	}
	return consent
}

// TestEditAuthorityConsentBuilderMatchesFixture pins the shipped fixture to
// the EXACT envelope the status layer builds (#2563): the fixture is not a
// hand-maintained example that merely validates, it is the builder's own
// output for the canonical two-sibling scenario, so contract bytes and
// production bytes can never drift apart silently. The empty expected
// revision is the fresh-ledger case the consent flow grants against; a
// widening grant embeds the committed revision, covered by the shape pin.
func TestEditAuthorityConsentBuilderMatchesFixture(t *testing.T) {
	built := newEditAuthorityConsent(
		"multi-repo-rollout", "/workspace/planning",
		[]string{"/workspace/service-a", "/workspace/service-b"},
		"sdd-b2f4c0a1d8e64953a7c15b9d3e0f2a68", "")
	payload, err := json.MarshalIndent(built, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	payload = append(payload, '\n')
	fixture, err := os.ReadFile(filepath.Join("..", "..", "contracts", "sdd-integration", "v1", "fixtures", "consent.fixture.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(payload, fixture) {
		t.Fatalf("builder output no longer matches the shipped fixture\n--- built ---\n%s\n--- fixture ---\n%s", payload, fixture)
	}
}
