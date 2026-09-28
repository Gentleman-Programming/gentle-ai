package telemetryruntime

import (
	"context"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v3/internal/model"
	"github.com/gentleman-programming/gentle-ai/v3/internal/telemetry"
)

type openCodeTestRoundTrip func(*http.Request) (*http.Response, error)

func (fn openCodeTestRoundTrip) RoundTrip(request *http.Request) (*http.Response, error) {
	return fn(request)
}

func TestSendOpenCodeReadsAssignmentFromRealisticLargeConfig(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("DO_NOT_TRACK", "")
	t.Setenv("GENTLE_AI_TELEMETRY", "")
	t.Setenv("CI", "")
	t.Setenv("GITHUB_ACTIONS", "")
	if err := telemetry.Save(home, telemetry.State{InstallID: "local", Enabled: true, NoticeShown: true}); err != nil {
		t.Fatal(err)
	}
	config := `{"padding":"` + strings.Repeat("x", 70*1024) + `","agent":{"gentle-orchestrator":{"model":"openai/gpt-5.6-sol","variant":"medium"}}}`
	path := filepath.Join(home, ".config", "opencode", "opencode.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(config), 0o600); err != nil {
		t.Fatal(err)
	}
	var got telemetry.RuntimeEvent
	client := &http.Client{Transport: openCodeTestRoundTrip(func(request *http.Request) (*http.Response, error) {
		body, err := io.ReadAll(request.Body)
		if err != nil {
			t.Fatal(err)
		}
		got, err = telemetry.ParseRuntimeEvent(body)
		if err != nil {
			t.Fatal(err)
		}
		return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"schema":"gentle-ai.telemetry-runtime-delivery/v1","decision":"stored"}`))}, nil
	})}
	envelope := `{"schema":"gentle-ai.telemetry-opencode/v1","info":{"role":"assistant","time":{"created":1,"completed":3},"providerID":"openai","modelID":"gpt-5.6-sol","agent":"gentle-orchestrator"}}`
	if decision := SendOpenCode(context.Background(), home, os.Getenv, strings.NewReader(envelope), client); decision != "stored" {
		t.Fatalf("SendOpenCode() decision = %q, want stored", decision)
	}
	if len(got.Rows) != 1 {
		t.Fatalf("runtime rows = %d, want 1", len(got.Rows))
	}
	row := got.Rows[0]
	if row.AgentKind != "orchestrator" || row.AgentClass != "orchestrator" || row.SelectedEffort != "medium" || row.Model != (telemetry.RuntimeModel{Provider: "openai", ID: "gpt-5.6-sol"}) || row.ModelEvidence != "response" {
		t.Fatalf("runtime row = %+v", row)
	}
}

func TestReadOpenCodeAssignment(t *testing.T) {
	for _, tt := range []struct {
		name   string
		config string
		agent  string
		want   model.ModelAssignment
	}{
		{
			name:   "reads model and effort for the observed agent",
			config: `{"agent":{"sdd-apply":{"model":"openai/gpt-5.6","variant":"high"}}}`,
			agent:  "sdd-apply",
			want:   model.ModelAssignment{ProviderID: "openai", ModelID: "gpt-5.6", Effort: "high"},
		},
		{name: "reads orchestrator assignment", config: `{"agent":{"gentle-orchestrator":{"model":"openai/gpt-5.6-sol","variant":"medium"}}}`, agent: "gentle-orchestrator", want: model.ModelAssignment{ProviderID: "openai", ModelID: "gpt-5.6-sol", Effort: "medium"}},
		{name: "reads native build assignment", config: `{"agent":{"build":{"model":"openai/gpt-5.6","variant":"low"}}}`, agent: "build", want: model.ModelAssignment{ProviderID: "openai", ModelID: "gpt-5.6", Effort: "low"}},
		{name: "reads native plan assignment", config: `{"agent":{"plan":{"model":"openai/gpt-5.6","variant":"max"}}}`, agent: "plan", want: model.ModelAssignment{ProviderID: "openai", ModelID: "gpt-5.6", Effort: "max"}},
		{name: "reads custom assignment without filtering", config: `{"agent":{"private-agent":{"model":"anthropic/claude-opus-5","variant":"xhigh"}}}`, agent: "private-agent", want: model.ModelAssignment{ProviderID: "anthropic", ModelID: "claude-opus-5", Effort: "xhigh"}},
		{name: "missing agent", config: `{"agent":{}}`, agent: "sdd-apply"},
		{name: "invalid config", config: `{`, agent: "sdd-apply"},
		{
			name:   "oversized config",
			config: `{"agent":{"sdd-apply":{"model":"openai/gpt-5.6","variant":"high"}},"padding":"` + strings.Repeat("x", openCodeConfigMaxBytes) + `"}`,
			agent:  "sdd-apply",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			t.Setenv("XDG_CONFIG_HOME", "")
			path := filepath.Join(home, ".config", "opencode", "opencode.json")
			if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, []byte(tt.config), 0o600); err != nil {
				t.Fatal(err)
			}
			if got := openCodeAssignment(readOpenCodeConfig(home), tt.agent); got != tt.want {
				t.Fatalf("openCodeAssignment() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestApplyOpenCodeAssignment(t *testing.T) {
	for _, tt := range []struct {
		name       string
		row        telemetry.RuntimeRow
		assignment model.ModelAssignment
		wantModel  telemetry.RuntimeModel
		wantProof  string
		wantEffort string
	}{
		{
			name:       "response model wins and selected effort is retained",
			row:        telemetry.RuntimeRow{Model: telemetry.RuntimeModel{Provider: "openai", ID: "gpt-5.4"}, ModelEvidence: "response", SelectedEffort: "unavailable"},
			assignment: model.ModelAssignment{ProviderID: "openai", ModelID: "gpt-5.6", Effort: "high"},
			wantModel:  telemetry.RuntimeModel{Provider: "openai", ID: "gpt-5.4"}, wantProof: "response", wantEffort: "high",
		},
		{
			name:       "selected model fills absent response model",
			row:        telemetry.RuntimeRow{Model: telemetry.RuntimeModel{Provider: "unknown", ID: "unknown"}, ModelEvidence: "unknown", SelectedEffort: "unavailable"},
			assignment: model.ModelAssignment{ProviderID: "anthropic", ModelID: "claude-opus-5", Effort: "xhigh"},
			wantModel:  telemetry.RuntimeModel{Provider: "anthropic", ID: "claude-opus-5"}, wantProof: "selected", wantEffort: "xhigh",
		},
		{
			name:       "invalid effort is unavailable",
			row:        telemetry.RuntimeRow{Model: telemetry.RuntimeModel{Provider: "unknown", ID: "unknown"}, ModelEvidence: "unknown", SelectedEffort: "unavailable"},
			assignment: model.ModelAssignment{ProviderID: "private", ModelID: "private", Effort: "turbo"},
			wantModel:  telemetry.RuntimeModel{Provider: "custom", ID: "custom"}, wantProof: "selected", wantEffort: "unavailable",
		},
		{name: "low effort", row: telemetry.RuntimeRow{SelectedEffort: "unavailable"}, assignment: model.ModelAssignment{Effort: "low"}, wantEffort: "low"},
		{name: "medium effort", row: telemetry.RuntimeRow{SelectedEffort: "unavailable"}, assignment: model.ModelAssignment{Effort: "medium"}, wantEffort: "medium"},
		{name: "max effort", row: telemetry.RuntimeRow{SelectedEffort: "unavailable"}, assignment: model.ModelAssignment{Effort: "max"}, wantEffort: "max"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			applyOpenCodeAssignment(&tt.row, tt.assignment)
			if tt.row.Model != tt.wantModel || tt.row.ModelEvidence != tt.wantProof || tt.row.SelectedEffort != tt.wantEffort {
				t.Fatalf("row = %+v", tt.row)
			}
		})
	}
}

// V1 falls back to OpenCode's documented reasoningEffort options when the agent
// has no configured variant: agent options (https://opencode.ai/docs/agents/,
// "Additional options") override the model's global options
// (https://opencode.ai/docs/models/, "Configure models").
func TestSendOpenCodeResolvesConfiguredReasoningEffort(t *testing.T) {
	modelOption := func(effort string) string {
		return `"provider":{"openai":{"models":{"gpt-5.6":{"options":{"reasoningEffort":` + effort + `}}}}}`
	}
	for _, tt := range []struct {
		name   string
		schema string
		agent  string
		config string
		want   string
	}{
		{name: "agent reasoningEffort option", schema: "v1", agent: "build", config: `{"agent":{"build":{"reasoningEffort":"high"}}}`, want: "high"},
		{name: "model reasoningEffort option", schema: "v1", agent: "build", config: `{` + modelOption(`"low"`) + `}`, want: "low"},
		{name: "model option without an agent name", schema: "v1", agent: "", config: `{` + modelOption(`"minimal"`) + `}`, want: "minimal"},
		{name: "agent option overrides model option", schema: "v1", agent: "build", config: `{"agent":{"build":{"reasoningEffort":"xhigh"}},` + modelOption(`"low"`) + `}`, want: "xhigh"},
		{name: "agent variant wins over options", schema: "v1", agent: "build", config: `{"agent":{"build":{"model":"openai/gpt-5.6","variant":"medium","reasoningEffort":"high"}},` + modelOption(`"low"`) + `}`, want: "medium"},
		{name: "custom variant blocks option fallback", schema: "v1", agent: "build", config: `{"agent":{"build":{"model":"openai/gpt-5.6","variant":"thinking"}},` + modelOption(`"high"`) + `}`, want: "unavailable"},
		{name: "variant without a model blocks option fallback", schema: "v1", agent: "build", config: `{"agent":{"build":{"variant":"thinking","reasoningEffort":"high"}},` + modelOption(`"high"`) + `}`, want: "unavailable"},
		{name: "option for another model is ignored", schema: "v1", agent: "build", config: `{"provider":{"openai":{"models":{"gpt-5.4":{"options":{"reasoningEffort":"high"}}}}}}`, want: "unavailable"},
		{name: "unenumerated provider value", schema: "v1", agent: "build", config: `{` + modelOption(`"none"`) + `}`, want: "unavailable"},
		{name: "contract meta value is not an effort", schema: "v1", agent: "build", config: `{"agent":{"build":{"reasoningEffort":"unknown"}}}`, want: "unavailable"},
		{name: "free text is dropped", schema: "v1", agent: "build", config: `{"agent":{"build":{"reasoningEffort":"PRIVATE_TEXT"}}}`, want: "unavailable"},
		{name: "non-string option is dropped", schema: "v1", agent: "build", config: `{` + modelOption(`3`) + `}`, want: "unavailable"},
		{name: "malformed provider shape is dropped", schema: "v1", agent: "build", config: `{"provider":{"openai":"PRIVATE_TEXT"}}`, want: "unavailable"},
		{name: "no configuration evidence", schema: "v1", agent: "build", config: `{}`, want: "unavailable"},
		{name: "v2 never consults configuration", schema: "v2", agent: "build", config: `{"agent":{"build":{"reasoningEffort":"high"}},` + modelOption(`"low"`) + `}`, want: "unavailable"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			t.Setenv("XDG_CONFIG_HOME", "")
			for _, key := range []string{"DO_NOT_TRACK", "GENTLE_AI_TELEMETRY", "CI", "GITHUB_ACTIONS"} {
				t.Setenv(key, "")
			}
			if err := telemetry.Save(home, telemetry.State{InstallID: "local", Enabled: true, NoticeShown: true}); err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(home, ".config", "opencode", "opencode.json")
			if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, []byte(tt.config), 0o600); err != nil {
				t.Fatal(err)
			}
			var got telemetry.RuntimeEvent
			var sent []byte
			client := &http.Client{Transport: openCodeTestRoundTrip(func(request *http.Request) (*http.Response, error) {
				body, err := io.ReadAll(request.Body)
				if err != nil {
					t.Fatal(err)
				}
				sent = body
				got, err = telemetry.ParseRuntimeEvent(body)
				if err != nil {
					t.Fatal(err)
				}
				return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"schema":"gentle-ai.telemetry-runtime-delivery/v1","decision":"stored"}`))}, nil
			})}
			agent := ""
			if tt.agent != "" {
				agent = `,"agent":"` + tt.agent + `"`
			}
			envelope := `{"schema":"gentle-ai.telemetry-opencode/` + tt.schema + `","info":{"role":"assistant","time":{"created":1,"completed":3},"providerID":"openai","modelID":"gpt-5.6"` + agent + `}}`
			if decision := SendOpenCode(context.Background(), home, os.Getenv, strings.NewReader(envelope), client); decision != "stored" {
				t.Fatalf("SendOpenCode() decision = %q, want stored", decision)
			}
			if len(got.Rows) != 1 {
				t.Fatalf("runtime rows = %d, want 1", len(got.Rows))
			}
			if got.Rows[0].SelectedEffort != tt.want || got.Rows[0].EffectiveEffort != "unavailable" {
				t.Fatalf("selected effort = %q, effective = %q, want selected %q", got.Rows[0].SelectedEffort, got.Rows[0].EffectiveEffort, tt.want)
			}
			if strings.Contains(string(sent), "PRIVATE") {
				t.Fatalf("private configuration leaked: %s", sent)
			}
		})
	}
}
