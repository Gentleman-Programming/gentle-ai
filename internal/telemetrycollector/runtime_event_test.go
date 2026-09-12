package telemetrycollector

import (
	"bytes"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/telemetry"
	"github.com/santhosh-tekuri/jsonschema/v6"
)

func runtimeFixture() []byte {
	token := `{"reported":1,"unavailable":2,"unsupported":3,"sum":999999999999}`
	row := `{"model":{"provider":"openai","id":"gpt-5.4"},"model_evidence":"response","agent_kind":"built_in","agent_class":"sdd-apply","selected_effort":"minimal","effective_effort":"unavailable","launches":null,"responses":6,`
	for _, key := range []string{"input_tokens", "output_tokens", "cache_read_tokens", "cache_creation_tokens", "reasoning_tokens", "total_tokens"} {
		row += `"` + key + `":` + token + `,`
	}
	row += `"error_category":"none","duration":{"kind":"message","measured_count":2,"sum_ms":1.25}}`
	return []byte(`{"schema":"gentle-ai.telemetry-runtime-event/v1","registry":1,"delivery_id":"0123456789abcdef0123456789abcdef","host":"pi","rows":[` + row + `]}`)
}

func TestRuntimeEventContract(t *testing.T) {
	compiler := jsonschema.NewCompiler()
	const base = "https://runtime.test/"
	for _, file := range []string{"aggregate.schema.json", "event.schema.json"} {
		raw, err := os.ReadFile("../../contracts/telemetry/runtime/v1/schemas/" + file)
		if err != nil {
			t.Fatal(err)
		}
		doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(raw))
		if err != nil {
			t.Fatal(err)
		}
		if err := compiler.AddResource(base+file, doc); err != nil {
			t.Fatal(err)
		}
	}
	schema, err := compiler.Compile(base + "event.schema.json")
	if err != nil {
		t.Fatal(err)
	}
	ackSchema, err := compiler.Compile(base + "event.schema.json#/$defs/delivery")
	if err != nil {
		t.Fatal(err)
	}
	for _, decision := range []string{"stored", "duplicate", "disabled"} {
		doc := map[string]any{"schema": telemetry.RuntimeDeliverySchema, "decision": decision}
		if err := ackSchema.Validate(doc); (err == nil) != (decision != "disabled") {
			t.Fatalf("ack %s: %v", decision, err)
		}
		doc["delivery_id"] = "private-canary"
		if err := ackSchema.Validate(doc); err == nil {
			t.Fatal("ack accepts extra identity")
		}
	}
	valid := string(runtimeFixture())
	cases := []struct {
		name, raw string
		valid     bool
	}{
		{"valid", valid, true},
		{"opencode sanitized", strings.Replace(valid, `{"provider":"openai","id":"gpt-5.4"}`, `{"provider":"opencode","id":"custom"}`, 1), true},
		{"opencode raw model", strings.Replace(valid, `{"provider":"openai","id":"gpt-5.4"}`, `{"provider":"opencode","id":"PRIVATE_MODEL"}`, 1), false},
		{"opencode other registry model", strings.Replace(valid, `{"provider":"openai","id":"gpt-5.4"}`, `{"provider":"opencode","id":"gpt-5.4"}`, 1), false},
		{"private provider custom", strings.Replace(valid, `{"provider":"openai","id":"gpt-5.4"}`, `{"provider":"PRIVATE_PROVIDER","id":"custom"}`, 1), false},
		{"opencode case", strings.Replace(valid, `{"provider":"openai","id":"gpt-5.4"}`, `{"provider":"OpenCode","id":"custom"}`, 1), false},
		{"numeric equivalent", strings.ReplaceAll(valid, `"registry":1`, `"registry":1e0`), true},
		{"local identity", strings.Replace(valid, "delivery_id", "batch_id", 1), false},
		{"wrong registry", strings.Replace(valid, `"registry":1`, `"registry":2`, 1), false},
		{"uppercase identity", strings.Replace(valid, "0123456789abcdef0123456789abcdef", "0123456789ABCDEF0123456789ABCDEF", 1), false},
		{"unknown host", strings.Replace(valid, `"host":"pi"`, `"host":"private-host"`, 1), false},
		{"missing class", strings.Replace(valid, `"agent_class":"sdd-apply",`, "", 1), false},
		{"unknown effort", strings.Replace(valid, `"minimal"`, `"private-effort"`, 1), false},
		{"raw error", strings.Replace(valid, `"error_category":"none"`, `"error_category":"/private/canary"`, 1), false},
		{"unmeasured duration", strings.Replace(valid, `"measured_count":2`, `"measured_count":0`, 1), false},
		{"wrong casing", strings.Replace(valid, "agent_class", "Agent_Class", 1), false},
		{"private model", strings.Replace(valid, "gpt-5.4", "/private/canary", 1), false},
		{"joined class", strings.Replace(valid, "sdd-apply", "sdd-apply|worker", 1), false},
		{"scalar coverage", strings.Replace(valid, `"input_tokens":{"reported":1,"unavailable":2,"unsupported":3,"sum":999999999999}`, `"input_tokens":12`, 1), false},
		{"unobserved sum", strings.ReplaceAll(valid, `"reported":1`, `"reported":0`), false},
	}
	for _, key := range []string{"localbatch_id", "local_batch_id", "install_id", "session_id", "path", "raw_error", "otlp"} {
		cases = append(cases, struct {
			name, raw string
			valid     bool
		}{key, strings.Replace(valid, `"host":"pi"`, `"host":"pi","`+key+`":"/private/canary"`, 1), false})
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			event, err := telemetry.ParseRuntimeEvent([]byte(tc.raw))
			if (err == nil) != tc.valid {
				t.Fatalf("parser valid=%v err=%v", tc.valid, err)
			}
			if err != nil && strings.Contains(err.Error(), "canary") {
				t.Fatal("private error echoed")
			}
			doc, err := jsonschema.UnmarshalJSON(strings.NewReader(tc.raw))
			if err != nil {
				t.Fatal(err)
			}
			if err := schema.Validate(doc); (err == nil) != tc.valid {
				t.Fatalf("schema valid=%v err=%v", tc.valid, err)
			}
			if tc.valid && string(event.Registry) != "1" {
				t.Fatal("registry not canonical")
			}
		})
	}
	for _, raw := range []string{
		strings.Replace(valid, `"host":"pi"`, `"host":"pi","host":"pi"`, 1),
		strings.Replace(valid, `"reported":1`, `"reported":1,"reported":1`, 1),
		valid + ` {}`, strings.Repeat(" ", telemetry.RuntimeMaxBytes) + valid,
	} {
		if _, err := telemetry.ParseRuntimeEvent([]byte(raw)); err == nil {
			t.Fatal("accepted duplicate, trailing or oversize JSON")
		}
	}
	event, err := telemetry.ParseRuntimeEvent(runtimeFixture())
	if err != nil {
		t.Fatal(err)
	}
	for _, count := range []int{0, 32, 33} {
		expanded := event
		expanded.Rows = make([]telemetry.RuntimeRow, count)
		for i := range expanded.Rows {
			expanded.Rows[i] = event.Rows[0]
		}
		raw, _ := json.Marshal(expanded)
		// The byte limit is independent of the schema's row count limit.
		_, parseErr := telemetry.ParseRuntimeEvent(raw)
		doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(raw))
		if err != nil {
			t.Fatal(err)
		}
		schemaErr := schema.Validate(doc)
		if (schemaErr == nil) != (count == 32) {
			t.Fatalf("schema row bound %d: %v", count, schemaErr)
		}
		if parseErr == nil {
			t.Fatalf("accepted empty or oversized %d-row fixture", count)
		}
	}
	canonical, _ := json.Marshal(event)
	if bytes.Contains(canonical, []byte("batch_id")) {
		t.Fatal("local identity in transport")
	}

}

func TestRuntimeEventDeprecatedAgentClassAliasNormalized(t *testing.T) {
	legacy := strings.Replace(string(runtimeFixture()), `"agent_class":"sdd-apply"`, `"agent_class":"sdd-proposal"`, 1)
	legacyRow := strings.TrimSuffix(strings.SplitN(legacy, `"rows":[`, 2)[1], `]}`)
	legacy = strings.TrimSuffix(legacy, `]}`) + `,` + legacyRow + `]}`
	legacyEvent, err := telemetry.ParseRuntimeEvent([]byte(legacy))
	if err != nil {
		t.Fatal(err)
	}
	if got := len(legacyEvent.Rows); got != 2 {
		t.Fatalf("parsed %d rows, want 2", got)
	}
	for i, row := range legacyEvent.Rows {
		if got := row.AgentClass; got != "sdd-propose" {
			t.Fatalf("row %d deprecated agent class normalized to %q, want sdd-propose", i, got)
		}
	}
}
