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
	"time"

	"github.com/gentleman-programming/gentle-ai/v2/internal/telemetry"
)

type claudeRoundTripFunc func(*http.Request) (*http.Response, error)

func (f claudeRoundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestTelemetryRuntimeClaudeDirectSend(t *testing.T) {
	home := runtimeCLIHome(t)
	transcript := filepath.Join(home, ".claude", "projects", "agent.jsonl")
	agent := filepath.Join(home, ".claude", "agents", "sdd-apply.md")
	if err := os.MkdirAll(filepath.Dir(transcript), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(agent), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(transcript, []byte(`{"type":"assistant","message":{"model":"claude-opus-5-1","content":"PRIVATE_MESSAGE","usage":{"input_tokens":1,"output_tokens":2,"cache_read_input_tokens":3,"cache_creation_input_tokens":4}}}`+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(agent, []byte("---\nname: sdd-apply\nmodel: sonnet\neffort: high\n---\nPRIVATE_PROMPT"), 0o600); err != nil {
		t.Fatal(err)
	}
	before := runtimeCLIDisk(t, home)
	requests := 0
	oldClient := runtimeHTTPClient
	runtimeHTTPClient = func() *http.Client {
		return &http.Client{Transport: claudeRoundTripFunc(func(r *http.Request) (*http.Response, error) {
			requests++
			body, _ := io.ReadAll(r.Body)
			event, err := telemetry.ParseRuntimeEvent(body)
			if err != nil || event.Host != "claude-code" || event.Rows[0].AgentClass != "sdd-apply" || event.Rows[0].Model.ID != "claude-opus-5" || event.Rows[0].ModelEvidence != "response" || event.Rows[0].SelectedEffort != "high" || string(event.Rows[0].Responses) != "1" || bytes.Contains(body, []byte("PRIVATE")) {
				t.Error("incorrect or unsafe event", err, string(body))
			}
			return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"schema":"gentle-ai.telemetry-runtime-delivery/v1","decision":"stored"}`))}, nil
		})}
	}
	t.Cleanup(func() { runtimeHTTPClient = oldClient })
	t.Setenv(telemetry.EndpointEnvVar, "https://telemetry.invalid")
	input := `{"session_id":"PRIVATE_SESSION","transcript_path":"PRIVATE_MAIN","cwd":"PRIVATE_CWD","permission_mode":"default","hook_event_name":"SubagentStop","stop_hook_active":false,"agent_id":"PRIVATE_ID","agent_type":"sdd-apply","agent_transcript_path":` + strconvQuote(transcript) + `,"last_assistant_message":"DIFFERENT_PRIVATE_MESSAGE"}`
	var out bytes.Buffer
	if err := runTelemetryRuntimeInput([]string{"claude", "--json"}, &out, strings.NewReader(input)); err != nil || !strings.Contains(out.String(), `"stored"`) || requests != 1 {
		t.Fatal(out.String(), err, requests)
	}
	if !reflect.DeepEqual(before, runtimeCLIDisk(t, home)) {
		t.Fatal("Claude adapter mutated the filesystem")
	}
}

type blockingClaudeReader struct{ release <-chan struct{} }

func (r blockingClaudeReader) Read([]byte) (int, error) { <-r.release; return 0, io.EOF }

func TestTelemetryRuntimeClaudeStdinBounds(t *testing.T) {
	runtimeCLIHome(t)
	for _, input := range []io.Reader{strings.NewReader(strings.Repeat("x", telemetry.RuntimeMaxBytes+1))} {
		var out bytes.Buffer
		if err := runTelemetryRuntimeInput([]string{"claude", "--json"}, &out, input); err != nil || !strings.Contains(out.String(), `"discarded"`) {
			t.Fatal(out.String(), err)
		}
	}
	release := make(chan struct{})
	old := runtimeStdinTimeout
	runtimeStdinTimeout = time.Millisecond
	t.Cleanup(func() { runtimeStdinTimeout = old; close(release) })
	var out bytes.Buffer
	if err := runTelemetryRuntimeInput([]string{"claude", "--json"}, &out, blockingClaudeReader{release}); err != nil || !strings.Contains(out.String(), `"discarded"`) {
		t.Fatal(out.String(), err)
	}
}

func TestTelemetryRuntimeClaudeIgnoredAndPolicyBeforeRead(t *testing.T) {
	home := runtimeCLIHome(t)
	var out bytes.Buffer
	if err := runTelemetryRuntimeInput([]string{"claude", "--json"}, &out, strings.NewReader(`{"hook_event_name":"PreToolUse"}`)); err != nil || !strings.Contains(out.String(), `"ignored"`) {
		t.Fatal(out.String(), err)
	}
	if err := telemetry.Save(home, telemetry.State{InstallID: "local", Enabled: false, NoticeShown: true}); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	if err := runTelemetryRuntimeInput([]string{"claude", "--json"}, &out, noOpenCodeRead{t}); err != nil || !strings.Contains(out.String(), `"disabled"`) {
		t.Fatal(out.String(), err)
	}
}

func strconvQuote(s string) string { return `"` + strings.ReplaceAll(s, `\`, `\\`) + `"` }
