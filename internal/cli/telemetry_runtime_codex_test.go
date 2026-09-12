package cli

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gentleman-programming/gentle-ai/v2/internal/state"
	"github.com/gentleman-programming/gentle-ai/v2/internal/telemetry"
)

func TestTelemetryRuntimeCodexDirectSend(t *testing.T) {
	home := runtimeCLIHome(t)
	if err := state.Write(home, state.InstallState{
		CodexModelAssignments:      map[string]string{"sdd-apply": "medium"},
		CodexPhaseModelAssignments: map[string]string{"sdd-apply": "gpt-5.6-terra"},
	}); err != nil {
		t.Fatal(err)
	}
	transcript := filepath.Join(t.TempDir(), "rollout.jsonl")
	if err := os.WriteFile(transcript, []byte(`{"type":"turn_context","payload":{"model":"gpt-5.6-sol","effort":"high"}}
{"type":"event_msg","payload":{"type":"token_count","info":{"last_token_usage":{"input_tokens":3,"output_tokens":2,"total_tokens":5}}}}
`), 0o600); err != nil {
		t.Fatal(err)
	}
	hook, _ := json.Marshal(map[string]any{
		"session_id": "PRIVATE_SESSION", "transcript_path": "PRIVATE_PARENT", "cwd": "PRIVATE_CWD",
		"hook_event_name": "SubagentStop", "model": "gpt-5.6-sol", "permission_mode": "default", "turn_id": "PRIVATE_TURN",
		"agent_id": "PRIVATE_AGENT", "agent_type": "sdd-apply", "agent_transcript_path": transcript,
		"stop_hook_active": false, "last_assistant_message": "PRIVATE_MESSAGE",
	})
	requests := 0
	oldClient := runtimeHTTPClient
	runtimeHTTPClient = func() *http.Client {
		return &http.Client{Transport: codexCLIRoundTrip(func(r *http.Request) (*http.Response, error) {
			requests++
			body, _ := io.ReadAll(r.Body)
			event, err := telemetry.ParseRuntimeEvent(body)
			if err != nil || event.Host != "codex" || event.Rows[0].AgentClass != "sdd-apply" || event.Rows[0].Model.ID != "gpt-5.6-sol" {
				t.Fatalf("event=%+v err=%v", event, err)
			}
			if bytes.Contains(body, []byte("PRIVATE")) || bytes.Contains(body, []byte(transcript)) {
				t.Fatalf("private data leaked: %s", body)
			}
			return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"schema":"gentle-ai.telemetry-runtime-delivery/v1","decision":"stored"}`))}, nil
		})}
	}
	t.Cleanup(func() { runtimeHTTPClient = oldClient })
	t.Setenv(telemetry.EndpointEnvVar, "https://telemetry.example.invalid")
	before := runtimeCLIDisk(t, home)
	var out bytes.Buffer
	if err := runTelemetryRuntimeInput([]string{"codex", "--json"}, &out, bytes.NewReader(hook)); err != nil || out.Len() != 0 || requests != 1 {
		t.Fatalf("out=%s requests=%d err=%v", out.String(), requests, err)
	}
	if after := runtimeCLIDisk(t, home); !equalCodexCLIDisk(before, after) {
		t.Fatal("Codex runtime command mutated disk")
	}
}

func TestTelemetryRuntimeCodexProcessesHeldOpenHookStdin(t *testing.T) {
	home := runtimeCLIHome(t)
	hook, _ := json.Marshal(map[string]any{
		"session_id": "PRIVATE_SESSION", "transcript_path": nil, "cwd": "PRIVATE_CWD",
		"hook_event_name": "Stop", "model": "gpt-5.6-sol", "permission_mode": "default", "turn_id": "PRIVATE_TURN",
		"stop_hook_active": false, "last_assistant_message": nil,
	})
	requests := 0
	oldClient := runtimeHTTPClient
	runtimeHTTPClient = func() *http.Client {
		return &http.Client{Transport: codexCLIRoundTrip(func(*http.Request) (*http.Response, error) {
			requests++
			return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"schema":"gentle-ai.telemetry-runtime-delivery/v1","decision":"stored"}`))}, nil
		})}
	}
	t.Cleanup(func() { runtimeHTTPClient = oldClient })
	t.Setenv(telemetry.EndpointEnvVar, "https://telemetry.example.invalid")
	old, oldHook := runtimeStdinTimeout, runtimeHookStdinTimeout
	runtimeStdinTimeout = 10 * time.Millisecond
	runtimeHookStdinTimeout = 100 * time.Millisecond
	t.Cleanup(func() {
		runtimeStdinTimeout = old
		runtimeHookStdinTimeout = oldHook
	})
	r, w := io.Pipe()
	defer r.Close()
	defer w.Close()
	done := make(chan error, 1)
	var out bytes.Buffer
	go func() { done <- runTelemetryRuntimeInput([]string{"codex", "--json"}, &out, r) }()
	if _, err := w.Write(hook); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if err != nil || requests != 1 || out.Len() != 0 {
			t.Fatalf("err=%v requests=%d out=%q", err, requests, out.String())
		}
	case <-time.After(time.Second):
		t.Fatal("Codex hook waited for stdin EOF")
	}
	if entries, err := os.ReadDir(home); err != nil || len(entries) == 0 {
		t.Fatalf("test home unavailable: entries=%d err=%v", len(entries), err)
	}
}

type codexCLIRoundTrip func(*http.Request) (*http.Response, error)

func (f codexCLIRoundTrip) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

func equalCodexCLIDisk(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for key, value := range a {
		if b[key] != value {
			return false
		}
	}
	return true
}
