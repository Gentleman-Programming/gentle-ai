package telemetry

import (
	"bytes"
	"encoding/json"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/opencode"
	"github.com/santhosh-tekuri/jsonschema/v6"
)

const tokenAbsent = `{"reported":0,"unavailable":1,"unsupported":0,"sum":0}`
const tokenZero = `{"reported":1,"unavailable":0,"unsupported":0,"sum":0}`
const tokenNone = `{"reported":0,"unavailable":0,"unsupported":0,"sum":0}`
const runtimeFixture = `{"schema":"gentle-ai.telemetry-runtime-aggregate/v1","registry":1,"host":"claude-code","rows":[{"model":{"provider":"anthropic","id":"claude-opus-5"},"agent_kind":"orchestrator","agent_class":"unknown","model_evidence":"response","selected_effort":"unavailable","effective_effort":"high","launches":1,"responses":0,"input_tokens":` + tokenZero + `,"output_tokens":` + tokenAbsent + `,"cache_read_tokens":{"reported":0,"unavailable":0,"unsupported":1,"sum":0},"cache_creation_tokens":` + tokenAbsent + `,"reasoning_tokens":` + tokenAbsent + `,"total_tokens":` + tokenAbsent + `,"error_category":"unknown","duration":{"kind":"unavailable","measured_count":0,"sum_ms":null}}]}`

func tokenReported(sum string) string {
	return `{"reported":1,"unavailable":0,"unsupported":0,"sum":` + sum + `}`
}

func TestRuntimeAgentClassesMatchPackagedAgents(t *testing.T) {
	want := append([]string{"orchestrator", "worker", "explore", "verify", "unknown"}, opencode.ConfigurableAgentPhases()...)
	// Pi packages exactly these additional agents outside the configurable OpenCode phases.
	piOnlyPackagedAgents := []string{"sdd-status", "sdd-sync"}
	want = append(want, piOnlyPackagedAgents...)
	goClasses := strings.Split(runtimeAgentClasses, "|")

	data, err := os.ReadFile("../../contracts/telemetry/runtime/v1/schemas/aggregate.schema.json")
	if err != nil {
		t.Fatal(err)
	}
	var document struct {
		Definitions struct {
			Row struct {
				Properties struct {
					AgentClass struct {
						Enum []string `json:"enum"`
					} `json:"agent_class"`
				} `json:"properties"`
			} `json:"row"`
		} `json:"$defs"`
	}
	if err := json.Unmarshal(data, &document); err != nil {
		t.Fatal(err)
	}
	schemaClasses := document.Definitions.Row.Properties.AgentClass.Enum

	slices.Sort(want)
	for source, got := range map[string][]string{"Go": goClasses, "JSON Schema": schemaClasses} {
		slices.Sort(got)
		if !slices.Equal(got, want) {
			t.Errorf("%s runtime agent classes = %v, want fixed classes plus OpenCode and Pi packaged agents %v", source, got, want)
		}
	}
}

func TestDeprecatedRuntimeAgentClassAliasNormalized(t *testing.T) {
	body := strings.Replace(runtimeFixture, `"agent_class":"unknown"`, `"agent_class":"sdd-proposal"`, 1)
	batch, err := decodeRuntime([]byte(body))
	if err != nil {
		t.Fatal(err)
	}
	if got := batch.Rows[0].AgentClass; got != "sdd-propose" {
		t.Fatalf("deprecated agent class normalized to %q, want sdd-propose", got)
	}
}

// Retained means preserved by canonicalization in memory, never persisted.
// Pure source-normalizer tests share this assertion.
func assertRuntimeRetained(t *testing.T, row RuntimeRow, host string) {
	t.Helper()
	batch := RuntimeBatch{Schema: RuntimeSchema, Registry: json.RawMessage("1"), Host: host, Rows: []RuntimeRow{row}}
	body, err := json.Marshal(batch)
	if err != nil {
		t.Fatal(err)
	}
	got, err := decodeRuntime(body)
	if err != nil {
		t.Fatal(err)
	}
	want, _ := json.Marshal(row)
	actual, _ := json.Marshal(got.Rows[0])
	if !bytes.Equal(want, actual) {
		t.Fatal("normalization changed observed row")
	}
}

func TestRuntimeMixedCoverageRetained(t *testing.T) {
	var batch map[string]any
	if err := json.Unmarshal([]byte(runtimeFixture), &batch); err != nil {
		t.Fatal(err)
	}
	row := batch["rows"].([]any)[0].(map[string]any)
	row["agent_class"] = "sdd-apply"
	row["selected_effort"], row["effective_effort"] = "off", "not_selected"
	for _, coverage := range []string{`{"reported":2,"unavailable":3,"unsupported":4,"sum":0}`, tokenNone, tokenZero, tokenAbsent} {
		for _, field := range []string{"input_tokens", "output_tokens", "cache_read_tokens", "cache_creation_tokens", "reasoning_tokens", "total_tokens"} {
			row[field] = json.RawMessage(coverage)
		}
		body, _ := json.Marshal(batch)
		got, err := decodeRuntime(body)
		if err != nil {
			t.Fatal(err)
		}
		for _, metric := range []json.RawMessage{got.Rows[0].Input, got.Rows[0].Output, got.Rows[0].CacheRead, got.Rows[0].CacheCreation, got.Rows[0].ReasoningTokens, got.Rows[0].TotalTokens} {
			if string(metric) != coverage {
				t.Fatal("independent observation coverage changed")
			}
		}
		assertRuntimeRetained(t, got.Rows[0], "pi")
	}
}

func TestRuntimeMetricCanonicalization(t *testing.T) {
	body := strings.Replace(runtimeFixture, `"reasoning_tokens":`+tokenAbsent, `"reasoning_tokens":{"reported":1.0e0,"unavailable":2.0,"unsupported":0,"sum":3.0e0}`, 1)
	body = strings.Replace(body, `{"kind":"unavailable","measured_count":0,"sum_ms":null}`, `{"kind":"request","measured_count":2.0,"sum_ms":12.500}`, 1)
	batch, err := decodeRuntime([]byte(body))
	if err != nil {
		t.Fatal(err)
	}
	row := batch.Rows[0]
	if string(row.ReasoningTokens) != `{"reported":1,"unavailable":2,"unsupported":0,"sum":3}` || string(row.Duration.MeasuredCount) != "2" || string(row.Duration.SumMS) != "125e-1" {
		t.Fatal("not canonical")
	}
	assertRuntimeRetained(t, row, "pi")
}

func TestRuntimeSchemaParity(t *testing.T) {
	data, err := os.ReadFile("../../contracts/telemetry/runtime/v1/schemas/aggregate.schema.json")
	if err != nil {
		t.Fatal(err)
	}
	var document any
	if err = json.Unmarshal(data, &document); err != nil {
		t.Fatal(err)
	}
	compiler := jsonschema.NewCompiler()
	if err = compiler.AddResource("urn:runtime", document); err != nil {
		t.Fatal(err)
	}
	schema, err := compiler.Compile("urn:runtime")
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name, body string
		valid      bool
	}{
		{"valid", runtimeFixture, true},
		{"opencode sanitized", strings.Replace(runtimeFixture, `{"provider":"anthropic","id":"claude-opus-5"}`, `{"provider":"opencode","id":"custom"}`, 1), true},
		{"opencode raw model", strings.Replace(runtimeFixture, `{"provider":"anthropic","id":"claude-opus-5"}`, `{"provider":"opencode","id":"PRIVATE_MODEL"}`, 1), false},
		{"opencode other registry model", strings.Replace(runtimeFixture, `{"provider":"anthropic","id":"claude-opus-5"}`, `{"provider":"opencode","id":"gpt-5.4"}`, 1), false},
		{"private provider custom", strings.Replace(runtimeFixture, `{"provider":"anthropic","id":"claude-opus-5"}`, `{"provider":"PRIVATE_PROVIDER","id":"custom"}`, 1), false},
		{"opencode case", strings.Replace(runtimeFixture, `{"provider":"anthropic","id":"claude-opus-5"}`, `{"provider":"OpenCode","id":"custom"}`, 1), false},
		{"registry canonical", strings.Replace(runtimeFixture, `"registry":1`, `"registry":1.0e0`, 1), true},
		{"batch identity", strings.Replace(runtimeFixture, `"registry":1`, `"registry":1,"batch_id":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"`, 1), false},
		{"private model", strings.Replace(runtimeFixture, "claude-opus-5", "PRIVATE_MODEL", 1), false},
		{"trailing", runtimeFixture + `{}`, false},
	}
	// Reject each missing/aliased/private field at every public object boundary.
	var root map[string]any
	_ = json.Unmarshal([]byte(runtimeFixture), &root)
	row := root["rows"].([]any)[0].(map[string]any)
	objects := []map[string]any{root, row, row["model"].(map[string]any), row["duration"].(map[string]any)}
	for _, key := range []string{"input_tokens", "output_tokens", "cache_read_tokens", "cache_creation_tokens", "reasoning_tokens", "total_tokens"} {
		objects = append(objects, row[key].(map[string]any))
	}
	for _, object := range objects {
		keys := make([]string, 0, len(object))
		for key := range object {
			keys = append(keys, key)
		}
		for _, key := range keys {
			value := object[key]
			delete(object, key)
			raw, _ := json.Marshal(root)
			cases = append(cases, struct {
				name, body string
				valid      bool
			}{"missing " + key, string(raw), false})
			object[strings.ToUpper(key)] = value
			raw, _ = json.Marshal(root)
			cases = append(cases, struct {
				name, body string
				valid      bool
			}{"alias " + key, string(raw), false})
			delete(object, strings.ToUpper(key))
			object[key] = value
		}
		object["private_path"] = "PRIVATE"
		raw, _ := json.Marshal(root)
		delete(object, "private_path")
		cases = append(cases, struct {
			name, body string
			valid      bool
		}{"extra field", string(raw), false})
	}
	for _, field := range []string{"input_tokens", "output_tokens", "cache_read_tokens", "cache_creation_tokens", "reasoning_tokens", "total_tokens"} {
		original := row[field]
		for _, tc := range []struct {
			raw   string
			valid bool
		}{
			{tokenNone, true}, {tokenAbsent, true}, {tokenZero, true},
			{`{"reported":2,"unavailable":3,"unsupported":4,"sum":7}`, true},
			{`{"reported":0,"unavailable":1,"unsupported":0,"sum":1}`, false},
			{`{"reported":1,"unavailable":0,"unsupported":0,"sum":1000000000000}`, false},
			{`{"reported":1,"unavailable":-1,"unsupported":0,"sum":0}`, false},
			{`{"reported":1,"unavailable":0,"unsupported":0.5,"sum":0}`, false},
			{`null`, false}, {`1`, false}, {`"unsupported"`, false},
		} {
			row[field] = json.RawMessage(tc.raw)
			raw, _ := json.Marshal(root)
			cases = append(cases, struct {
				name, body string
				valid      bool
			}{field + tc.raw, string(raw), tc.valid})
		}
		row[field] = original
	}
	for _, tc := range []struct {
		old, replacement string
		valid            bool
	}{
		{`"launches":1`, `"launches":-1`, false}, {`"launches":1`, `"launches":1.5`, false},
		{`"launches":1`, `"launches":1e999`, false}, {`"launches":1`, `"launches":1e0`, true},
		{`"launches":1`, `"launches":null`, true}, {`"launches":1`, `"launches":"unsupported"`, true},
		{`"agent_class":"unknown"`, `"agent_class":"sdd-apply"`, true},
		{`"agent_class":"unknown"`, `"agent_class":"PRIVATE"`, false},
		{`"effective_effort":"high"`, `"effective_effort":"high|xhigh"`, false},
		{`"error_category":"unknown"`, `"error_category":"PRIVATE_ERROR"`, false},
		{`"host":"claude-code"`, `"host":"private"`, false},
		{`{"kind":"unavailable","measured_count":0,"sum_ms":null}`, `{"kind":"message","measured_count":1,"sum_ms":12.5}`, true},
		{`{"kind":"unavailable","measured_count":0,"sum_ms":null}`, `{"kind":"request","measured_count":0,"sum_ms":0}`, false},
		{`{"kind":"unavailable","measured_count":0,"sum_ms":null}`, `{"kind":"request","measured_count":1,"sum_ms":999999999999.0001}`, false},
	} {
		cases = append(cases, struct {
			name, body string
			valid      bool
		}{tc.replacement, strings.Replace(runtimeFixture, tc.old, tc.replacement, 1), tc.valid})
	}
	// Exercise every closed public enum and registry pair, not just examples.
	for field, values := range map[string]string{
		"agent_class":      runtimeAgentClasses,
		"selected_effort":  runtimeEfforts,
		"effective_effort": runtimeEfforts,
		"model_evidence":   "selected|response|unknown",
		"agent_kind":       "orchestrator|built_in|custom|unknown",
		"error_category":   "none|unknown|auth|output_length|aborted|api|rate_limit|server",
	} {
		original := row[field]
		for _, value := range append(strings.Split(values, "|"), "PRIVATE", values, "") {
			row[field] = value
			raw, _ := json.Marshal(root)
			cases = append(cases, struct {
				name, body string
				valid      bool
			}{field + value, string(raw), value != "PRIVATE" && value != values && value != ""})
		}
		row[field] = original
	}
	for provider, names := range map[string]string{
		"anthropic":    "claude-opus-5 claude-haiku-4-5 claude-haiku-4-5-20251001 claude-sonnet-5",
		"openai":       "gpt-6-astra gpt-5.6-sol gpt-5.6-terra gpt-5.6-luna gpt-5.3-codex-spark gpt-5.5 gpt-5.4 gpt-5.4-mini gpt-5.2 gpt-5.3-codex gpt-5.6",
		"openai-codex": "gpt-6-astra gpt-5.6-sol gpt-5.6-terra gpt-5.6-luna gpt-5.3-codex-spark gpt-5.5 gpt-5.4 gpt-5.4-mini gpt-5.2 gpt-5.3-codex gpt-5.6",
		"unknown":      "unknown", "custom": "custom",
	} {
		for _, id := range strings.Fields(names) {
			cases = append(cases, struct {
				name, body string
				valid      bool
			}{provider + id, strings.ReplaceAll(strings.ReplaceAll(runtimeFixture, "anthropic", provider), "claude-opus-5", id), true})
		}
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, goErr := decodeRuntime([]byte(tc.body))
			var value any
			d := json.NewDecoder(strings.NewReader(tc.body))
			d.UseNumber()
			err := d.Decode(&value)
			// JSON Schema validates one JSON value; trailing values are the reader's job.
			schemaValid := err == nil && schema.Validate(value) == nil && tc.name != "trailing"
			if (goErr == nil) != tc.valid || schemaValid != tc.valid {
				t.Fatalf("valid=%v Go=%v schemaValid=%v", tc.valid, goErr, schemaValid)
			}
		})
	}
	for _, raw := range []string{
		strings.Replace(runtimeFixture, `"registry":1`, `"registry":1,"registry":1`, 1),
		strings.Replace(runtimeFixture, `"reported":1`, `"reported":1,"reported":1`, 1),
	} {
		if _, err := decodeRuntime([]byte(raw)); err == nil {
			t.Fatal("duplicate JSON key accepted")
		}
	}
}
