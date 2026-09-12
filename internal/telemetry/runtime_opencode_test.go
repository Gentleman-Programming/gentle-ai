package telemetry

import (
	"encoding/json"
	"strings"
	"testing"
)

// Source-shaped fixture, not a released SDK compatibility claim.
const openCodeFixture = `{"type":"message.updated","properties":{"info":{"id":"PRIVATE_ID","sessionID":"PRIVATE_SESSION","role":"assistant","parentID":"PRIVATE_PARENT","modelID":"gpt-5.4","providerID":"openai","agent":"build","path":{"cwd":"PRIVATE_PATH","root":"PRIVATE_ROOT"},"cost":0,"time":{"created":1000,"completed":1250},"tokens":{"input":12,"output":8,"reasoning":3,"cache":{"read":2,"write":0}},"finish":"stop"}}}`

func TestOpenCodeRuntimeSanitizedIdentity(t *testing.T) {
	for _, tc := range []struct {
		provider, model, wantProvider, wantModel, wantEvidence string
	}{
		{"opencode", "PRIVATE_MODEL", "opencode", "custom", "response"},
		{"opencode", "gpt-5.4", "opencode", "custom", "response"},
		{"opencode", "custom", "opencode", "custom", "response"},
		{"OpenCode", "PRIVATE_MODEL", "custom", "custom", "response"},
		{"PRIVATE_PROVIDER", "PRIVATE_MODEL", "custom", "custom", "response"},
		{"openai", "PRIVATE_MODEL", "custom", "custom", "response"},
		{"openai", "gpt-5.4", "openai", "gpt-5.4", "response"},
		{"opencode", "", "unknown", "unknown", "unknown"},
		{"", "PRIVATE_MODEL", "unknown", "unknown", "unknown"},
	} {
		t.Run(tc.provider+"/"+tc.model, func(t *testing.T) {
			input := strings.Replace(openCodeFixture, `"providerID":"openai"`, `"providerID":"`+tc.provider+`"`, 1)
			input = strings.Replace(input, `"modelID":"gpt-5.4"`, `"modelID":"`+tc.model+`"`, 1)
			o, err := NormalizeOpenCode(strings.NewReader(input))
			if err != nil || o == nil {
				t.Fatal(err)
			}
			if o.Row.Model != (RuntimeModel{Provider: tc.wantProvider, ID: tc.wantModel}) || o.Row.ModelEvidence != tc.wantEvidence {
				t.Fatalf("unexpected model attribution: %+v evidence %q", o.Row.Model, o.Row.ModelEvidence)
			}
			assertRuntimeRetained(t, o.Row, "opencode")
		})
	}
}

func TestOpenCodeRuntimeCompleted(t *testing.T) {
	o, err := NormalizeOpenCode(strings.NewReader(openCodeFixture))
	if err != nil || o == nil {
		t.Fatalf("normalize: %v, %v", o, err)
	}
	if o.Row.Model.ID != "gpt-5.4" || o.Row.ModelEvidence != "response" || o.Row.AgentKind != "orchestrator" || o.Row.AgentClass != "orchestrator" || o.Row.SelectedEffort != "unavailable" || o.Row.EffectiveEffort != "unavailable" {
		t.Fatalf("row: %+v", o.Row)
	}
	if string(o.Row.Input) != tokenReported("12") || string(o.Row.Output) != tokenReported("8") || string(o.Row.CacheRead) != tokenReported("2") || string(o.Row.CacheCreation) != tokenAbsent || string(o.ReasoningTokens) != tokenReported("3") || o.MessageDurationMS == nil || *o.MessageDurationMS != 250 {
		t.Fatalf("metrics: %+v", o)
	}
	if string(o.Row.ReasoningTokens) != tokenReported("3") || string(o.Row.TotalTokens) != tokenAbsent || o.Row.ErrorCategory != "none" || o.Row.Duration.Kind != "message" || string(o.Row.Duration.MeasuredCount) != "1" || string(o.Row.Duration.SumMS) != "25e1" {
		t.Fatalf("common metrics: %+v", o.Row)
	}
	assertRuntimeRetained(t, o.Row, "opencode")
	b, _ := json.Marshal(RuntimeBatch{Schema: RuntimeSchema, Registry: json.RawMessage("1"), BatchID: strings.Repeat("a", 32), Host: "opencode", Rows: []RuntimeRow{o.Row}})
	if _, err := decodeRuntime(b); err != nil {
		t.Fatal(err)
	}
	out, _ := json.Marshal(o)
	if strings.Contains(string(out), "PRIVATE") || strings.Contains(string(out), `"duration_ms"`) {
		t.Fatalf("leaked or mislabeled: %s", out)
	}
	again, err := NormalizeOpenCode(strings.NewReader(openCodeFixture))
	if err != nil || again == nil {
		t.Fatal("pure normalizer must not lifetime-dedupe")
	}
}

func TestOpenCodeRuntimeAgentAttribution(t *testing.T) {
	for _, tt := range []struct {
		name       string
		agent      string
		wantKind   string
		wantClass  string
		wantStored string
	}{
		{"native build agent", "build", "orchestrator", "orchestrator", "build"},
		{"native plan agent", "plan", "orchestrator", "orchestrator", "plan"},
		{"gentle ai orchestrator", "gentle-orchestrator", "orchestrator", "orchestrator", "gentle-orchestrator"},
		{"fallback explore agent", "explore", "built_in", "explore", "explore"},
		{"fallback general agent", "general", "built_in", "worker", "general"},
		{"named gentle ai agent", "sdd-apply", "built_in", "sdd-apply", "sdd-apply"},
		{"custom agent", "team-private-agent", "custom", "unknown", "team-private-agent"},
		{"missing agent", "", "unknown", "unknown", ""},
	} {
		t.Run(tt.name, func(t *testing.T) {
			input := openCodeFixture
			if tt.agent == "" {
				input = strings.Replace(input, `,"agent":"build"`, "", 1)
			} else {
				input = strings.Replace(input, `"agent":"build"`, `"agent":"`+tt.agent+`"`, 1)
			}
			o, err := NormalizeOpenCode(strings.NewReader(input))
			if err != nil || o == nil {
				t.Fatalf("NormalizeOpenCode() = %+v, %v", o, err)
			}
			if o.Row.AgentKind != tt.wantKind || o.Row.AgentClass != tt.wantClass || o.Agent != tt.wantStored {
				t.Fatalf("agent attribution = %q/%q stored %q, want %q/%q stored %q", o.Row.AgentKind, o.Row.AgentClass, o.Agent, tt.wantKind, tt.wantClass, tt.wantStored)
			}
		})
	}
}

func TestOpenCodeRuntimeCases(t *testing.T) {
	for _, tt := range []struct {
		name, old, replacement string
		skip, invalid          bool
	}{
		{"update", `,"completed":1250`, ``, true, false},
		{"user", `"assistant"`, `"user"`, true, false},
		{"summary", `"cost":0`, `"summary":true,"cost":0`, true, false},
		{"other", `message.updated`, `message.part.updated`, true, false},
		{"negative", `"input":12`, `"input":-1`, false, true},
		{"fraction", `"input":12`, `"input":1.5`, false, true},
		{"oversized", `"input":12`, `"input":1000000000000`, false, true},
		{"string", `"input":12`, `"input":"12"`, false, true},
		{"null", `"input":12`, `"input":null`, false, true},
		{"duplicate", `"input":12`, `"input":12,"input":13`, false, true},
		{"alias", `"completed":1250`, `"Completed":1250`, true, false},
		{"duration bound", `"completed":1250`, `"completed":1000000001000`, false, true},
		{"backwards", `"completed":1250`, `"completed":999`, false, true},
		{"time type", `"completed":1250`, `"completed":"1250"`, false, true},
		{"cache type", `"cache":{"read":2,"write":0}`, `"cache":null`, false, true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			o, err := NormalizeOpenCode(strings.NewReader(strings.Replace(openCodeFixture, tt.old, tt.replacement, 1)))
			if (err != nil) != tt.invalid || (err == nil && (o == nil) != tt.skip) {
				t.Fatalf("observation=%+v err=%v", o, err)
			}
		})
	}
}

func TestOpenCodeRuntimePrivacyAndAvailability(t *testing.T) {
	s := strings.Replace(openCodeFixture, `"gpt-5.4"`, `"PRIVATE_MODEL"`, 1)
	s = strings.Replace(s, `"input":12`, `"input":0`, 1)
	s = strings.Replace(s, `"output":8,`, ``, 1)
	s = strings.Replace(s, `"reasoning":3`, `"reasoning":0`, 1)
	o, err := NormalizeOpenCode(strings.NewReader(s))
	if err != nil || o == nil {
		t.Fatal(err)
	}
	if o.Row.Model.Provider != "custom" || string(o.Row.Input) != tokenAbsent || string(o.Row.Output) != tokenAbsent || string(o.ReasoningTokens) != tokenAbsent {
		t.Fatalf("%+v", o)
	}
	if string(o.Row.ReasoningTokens) != tokenAbsent {
		t.Fatal("default reasoning zero became measured")
	}
	zero, err := NormalizeOpenCode(strings.NewReader(strings.Replace(openCodeFixture, `"completed":1250`, `"completed":1000`, 1)))
	if err != nil || zero == nil || zero.Row.Duration.Kind != "message" || string(zero.Row.Duration.MeasuredCount) != "1" || string(zero.Row.Duration.SumMS) != "0" {
		t.Fatal("measured message zero lost", err)
	}
	for _, name := range []string{"ProviderAuthError", "UnknownError", "MessageOutputLengthError", "MessageAbortedError", "APIError", "PRIVATE_ERROR"} {
		s := strings.Replace(openCodeFixture, `"cost":0`, `"cost":0,"error":{"name":"`+name+`","data":{"message":"PRIVATE_MESSAGE","responseBody":"PRIVATE_BODY","statusCode":429}}`, 1)
		o, err := NormalizeOpenCode(strings.NewReader(s))
		if err != nil || o == nil || o.ErrorCategory == "" {
			t.Fatalf("%s: %v", name, err)
		}
		if name == "APIError" && o.ErrorCategory != "rate_limit" {
			t.Fatal(o.ErrorCategory)
		}
		if o.Row.ErrorCategory != o.ErrorCategory || string(o.Row.ReasoningTokens) != string(o.ReasoningTokens) {
			t.Fatal("common fields diverged")
		}
		assertRuntimeRetained(t, o.Row, "opencode")
		out, _ := json.Marshal(o)
		if strings.Contains(string(out), "PRIVATE") {
			t.Fatal(string(out))
		}
	}
}

func TestOpenCodeRuntimeBounds(t *testing.T) {
	for _, s := range []string{
		strings.Repeat(" ", OpenCodeMaxBytes+1),
		openCodeFixture + ` {}`,
		`{"x":` + strings.Repeat(`[`, 34) + `0` + strings.Repeat(`]`, 34) + `}`,
		strings.Replace(openCodeFixture, `"agent":"build"`, `"agent":"`+strings.Repeat("x", 4097)+`"`, 1),
		strings.Replace(openCodeFixture, `"agent":"build"`, `"agent":"`+strings.Repeat("x", 65)+`"`, 1),
		strings.Replace(openCodeFixture, `"agent":"build"`, `"agent":"bad\nagent"`, 1),
		strings.Replace(openCodeFixture, `"cost":0`, `"error":{"name":"APIError","data":{"statusCode":999}}`, 1),
	} {
		if _, err := NormalizeOpenCode(strings.NewReader(s)); err == nil {
			t.Fatal("accepted invalid bounds")
		}
	}
}
