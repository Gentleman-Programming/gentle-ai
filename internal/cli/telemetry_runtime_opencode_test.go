package cli

import (
	"bytes"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/telemetry"
)

// Published V1 AssistantMessage subset: no batch/session/message identities.
const completedOpenCodeEnvelope = `{"schema":"gentle-ai.telemetry-opencode/v1","info":{"role":"assistant","time":{"created":1,"completed":3},"providerID":"PRIVATE_PROVIDER","modelID":"PRIVATE_MODEL"}}`

type noOpenCodeRead struct{ t *testing.T }

type openCodeRoundTripFunc func(*http.Request) (*http.Response, error)

func (fn openCodeRoundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return fn(request)
}

func (r noOpenCodeRead) Read([]byte) (int, error) {
	r.t.Fatal("read event without permission")
	return 0, io.EOF
}

func TestTelemetryRuntimeOpenCodeDirectSend(t *testing.T) {
	home := runtimeCLIHome(t)
	before := runtimeCLIDisk(t, home)
	requests := 0
	wantProvider := "custom"
	runtimeCLIServer(t, func(w http.ResponseWriter, r *http.Request) {
		requests++
		body, _ := io.ReadAll(r.Body)
		event, err := telemetry.ParseRuntimeEvent(body)
		if err != nil {
			t.Error(err)
		} else if event.Host != "opencode" || event.Rows[0].Model.Provider != wantProvider || event.Rows[0].Model.ID != "custom" || event.Rows[0].Duration.Kind != "message" || string(event.Rows[0].Duration.SumMS) != "2" {
			t.Error("incorrect normalized event")
		}
		for _, private := range []string{"PRIVATE", "batch_id", "created", "completed", "session", "task_id"} {
			if bytes.Contains(body, []byte(private)) {
				t.Error("private wire field", private)
			}
		}
		_, _ = io.WriteString(w, `{"schema":"gentle-ai.telemetry-runtime-delivery/v1","decision":"stored"}`)
	})
	for i, provider := range []string{"PRIVATE_PROVIDER", "opencode", "OpenCode", "openai"} {
		wantProvider = "custom"
		if provider == "opencode" {
			wantProvider = "opencode"
		}
		input := strings.Replace(completedOpenCodeEnvelope, "PRIVATE_PROVIDER", provider, 1)
		var out bytes.Buffer
		if err := runTelemetryRuntimeInput([]string{"opencode", "--json"}, &out, strings.NewReader(input)); err != nil || !strings.Contains(out.String(), `"stored"`) {
			t.Fatal(out.String(), err)
		}
		if requests != i+1 || !reflect.DeepEqual(before, runtimeCLIDisk(t, home)) {
			t.Fatal("extra attempt or disk mutation")
		}
	}
}

func TestTelemetryRuntimeOpenCodeUsesAgentAssignment(t *testing.T) {
	for _, tt := range []struct {
		name         string
		agent        string
		provider     string
		model        string
		config       string
		wantModel    telemetry.RuntimeModel
		wantEvidence string
		wantKind     string
		wantClass    string
		wantEffort   string
	}{
		{
			name: "response model wins while selected effort is retained", agent: "sdd-apply", provider: "openai", model: "gpt-5.4",
			config:    `{"agent":{"sdd-apply":{"model":"openai/gpt-5.6","variant":"high"}}}`,
			wantModel: telemetry.RuntimeModel{Provider: "openai", ID: "gpt-5.4"}, wantEvidence: "response", wantKind: "built_in", wantClass: "sdd-apply", wantEffort: "high",
		},
		{
			name: "selected model fills absent response model", agent: "gentle-orchestrator",
			config:    `{"agent":{"gentle-orchestrator":{"model":"anthropic/claude-opus-5","variant":"xhigh"}}}`,
			wantModel: telemetry.RuntimeModel{Provider: "anthropic", ID: "claude-opus-5"}, wantEvidence: "selected", wantKind: "orchestrator", wantClass: "orchestrator", wantEffort: "xhigh",
		},
		{
			name: "custom agent name stays private and invalid effort falls back", agent: "private-team-agent",
			config:    `{"agent":{"private-team-agent":{"model":"openai/gpt-5.4","variant":"turbo"}}}`,
			wantModel: telemetry.RuntimeModel{Provider: "openai", ID: "gpt-5.4"}, wantEvidence: "selected", wantKind: "custom", wantClass: "unknown", wantEffort: "unavailable",
		},
		{
			name: "oversized config fails open to unavailable attribution", agent: "sdd-apply",
			config:    strings.Repeat(" ", 65537),
			wantModel: telemetry.RuntimeModel{Provider: "unknown", ID: "unknown"}, wantEvidence: "unknown", wantKind: "built_in", wantClass: "sdd-apply", wantEffort: "unavailable",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			runtimeCLIHome(t)
			configPath := filepath.Join(os.Getenv("XDG_CONFIG_HOME"), "opencode", "opencode.json")
			if err := os.MkdirAll(filepath.Dir(configPath), 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(configPath, []byte(tt.config), 0o600); err != nil {
				t.Fatal(err)
			}
			previousClient := runtimeHTTPClient
			t.Cleanup(func() { runtimeHTTPClient = previousClient })
			runtimeHTTPClient = func() *http.Client {
				return &http.Client{Transport: openCodeRoundTripFunc(func(r *http.Request) (*http.Response, error) {
					body, _ := io.ReadAll(r.Body)
					event, err := telemetry.ParseRuntimeEvent(body)
					if err != nil {
						t.Fatal(err)
					}
					row := event.Rows[0]
					if row.Model != tt.wantModel || row.ModelEvidence != tt.wantEvidence || row.AgentKind != tt.wantKind || row.AgentClass != tt.wantClass || row.SelectedEffort != tt.wantEffort || row.EffectiveEffort != "unavailable" {
						t.Fatalf("row = %+v", row)
					}
					if bytes.Contains(body, []byte(tt.agent)) && tt.wantClass == "unknown" {
						t.Fatal("custom agent name leaked")
					}
					return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"schema":"gentle-ai.telemetry-runtime-delivery/v1","decision":"stored"}`)), Header: make(http.Header)}, nil
				})}
			}
			input := `{"schema":"gentle-ai.telemetry-opencode/v1","info":{"role":"assistant","time":{"created":1,"completed":3},"providerID":"` + tt.provider + `","modelID":"` + tt.model + `","agent":"` + tt.agent + `"}}`
			var out bytes.Buffer
			if err := runTelemetryRuntimeInput([]string{"opencode", "--json"}, &out, strings.NewReader(input)); err != nil || !strings.Contains(out.String(), `"stored"`) {
				t.Fatalf("send = %q, %v", out.String(), err)
			}
		})
	}
}

func TestTelemetryRuntimeOpenCodePolicyBeforeRead(t *testing.T) {
	for _, scenario := range []string{"missing", "disabled", "unenrolled", "env"} {
		t.Run(scenario, func(t *testing.T) {
			home := telemetryTestHome(t)
			enableTelemetryForTest(t)
			if scenario != "missing" {
				if err := telemetry.Save(home, telemetry.State{InstallID: "local", Enabled: scenario != "disabled", NoticeShown: scenario != "unenrolled"}); err != nil {
					t.Fatal(err)
				}
			}
			if scenario == "env" {
				t.Setenv("DO_NOT_TRACK", "1")
			}
			before := runtimeCLIDisk(t, home)
			var out bytes.Buffer
			if err := runTelemetryRuntimeInput([]string{"opencode", "--json"}, &out, noOpenCodeRead{t}); err != nil || !strings.Contains(out.String(), `"disabled"`) {
				t.Fatal(out.String(), err)
			}
			if !reflect.DeepEqual(before, runtimeCLIDisk(t, home)) {
				t.Fatal("disabled adapter wrote state")
			}
		})
	}
}

func TestTelemetryRuntimeOpenCodeIgnoresPartial(t *testing.T) {
	home := runtimeCLIHome(t)
	runtimeCLIServer(t, func(w http.ResponseWriter, r *http.Request) { t.Error("ignored event made HTTP request") })
	before := runtimeCLIDisk(t, home)
	for _, input := range []string{
		strings.Replace(completedOpenCodeEnvelope, `,"completed":3`, "", 1),
		strings.Replace(completedOpenCodeEnvelope, `"role":"assistant"`, `"role":"user"`, 1),
		strings.Replace(completedOpenCodeEnvelope, `"role":"assistant"`, `"role":"assistant","summary":true`, 1),
	} {
		var out bytes.Buffer
		if err := runTelemetryRuntimeInput([]string{"opencode", "--json"}, &out, strings.NewReader(input)); err != nil || !strings.Contains(out.String(), `"ignored"`) {
			t.Fatal(out.String(), err)
		}
	}
	if !reflect.DeepEqual(before, runtimeCLIDisk(t, home)) {
		t.Fatal("ignored adapter wrote state")
	}
}

func TestTelemetryRuntimeOpenCodeRejectsUnsafeEnvelope(t *testing.T) {
	home := runtimeCLIHome(t)
	before := runtimeCLIDisk(t, home)
	// Any accidental send is confined to a local server, never an external endpoint.
	runtimeCLIServer(t, func(w http.ResponseWriter, r *http.Request) { t.Error("invalid envelope made HTTP request") })
	for _, input := range []string{
		strings.Replace(completedOpenCodeEnvelope, `"role":"assistant"`, `"role":"assistant","parts":["PRIVATE_PROMPT"]`, 1),
		strings.Replace(completedOpenCodeEnvelope, `"role":"assistant"`, `"role":"assistant","role":"user"`, 1),
		strings.Replace(completedOpenCodeEnvelope, `"created":1,`, "", 1),
		strings.Replace(completedOpenCodeEnvelope, `"role":"assistant"`, `"role":"assistant","tokens":{"input":"2"}`, 1),
		strings.Replace(completedOpenCodeEnvelope, `"role":"assistant"`, `"role":"assistant","id":"PRIVATE_SOURCE_ID"`, 1),
		strings.Replace(completedOpenCodeEnvelope, `"role":"assistant"`, `"role":"assistant","agent":"`+strings.Repeat("x", 65)+`"`, 1),
		strings.Replace(completedOpenCodeEnvelope, `"role":"assistant"`, `"role":"assistant","agent":"bad\nagent"`, 1),
		strings.Replace(completedOpenCodeEnvelope, `"info":`, `"batch_id":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","info":`, 1),
		completedOpenCodeEnvelope + `{}`, strings.Repeat("x", 16385),
	} {
		var out bytes.Buffer
		if err := runTelemetryRuntimeInput([]string{"opencode", "--json"}, &out, strings.NewReader(input)); err != nil || !strings.Contains(out.String(), `"discarded"`) || strings.Contains(out.String(), "PRIVATE") {
			t.Fatal("unsafe input accepted or exposed", err)
		}
	}
	if !reflect.DeepEqual(before, runtimeCLIDisk(t, home)) {
		t.Fatal("invalid adapter wrote state")
	}
	// Existing policy remains byte-for-byte intact, including private install ID.
	if body, err := os.ReadFile(telemetry.Path(home)); err != nil || !bytes.Contains(body, []byte("PRIVATE_INSTALL")) {
		t.Fatal("policy replaced")
	}
}
