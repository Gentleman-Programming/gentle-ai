package telemetryruntime

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/state"
	"github.com/gentleman-programming/gentle-ai/v2/internal/telemetry"
)

const codexRealBridgeTranscriptFixture = `{"timestamp":"PRIVATE_TIME","type":"session_meta","payload":{"id":"PRIVATE_SESSION","source":{"subagent":{"thread_spawn":{"parent_thread_id":"PRIVATE_PARENT","depth":1,"agent_nickname":"PRIVATE_NICKNAME","agent_path":"/root/sdd_explore"}}}}}
{"timestamp":"PRIVATE_TIME","type":"turn_context","payload":{"turn_id":"PRIVATE_TURN","model":"gpt-5.6-sol","effort":"medium","collaboration_mode":{"settings":{"reasoning_effort":"medium"}}}}
{"timestamp":"PRIVATE_TIME","type":"event_msg","payload":{"type":"token_count","info":{"last_token_usage":{"input_tokens":120,"cached_input_tokens":20,"cache_write_input_tokens":4,"output_tokens":30,"reasoning_output_tokens":10,"total_tokens":164}}}}
`

func codexBridgeHome(t *testing.T, enabled bool) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("DO_NOT_TRACK", "")
	t.Setenv("GENTLE_AI_TELEMETRY", "")
	t.Setenv("CI", "")
	t.Setenv("GITHUB_ACTIONS", "")
	if err := telemetry.Save(home, telemetry.State{InstallID: "PRIVATE_INSTALL", Enabled: enabled, NoticeShown: true}); err != nil {
		t.Fatal(err)
	}
	return home
}

func codexBridgeHook(path string) string {
	return codexBridgeHookForAgent(path, "sdd-apply")
}

func codexBridgeHookForAgent(path, agentType string) string {
	encoded, _ := json.Marshal(map[string]any{
		"session_id": "PRIVATE_SESSION", "transcript_path": "PRIVATE_PARENT", "cwd": "PRIVATE_CWD",
		"hook_event_name": "SubagentStop", "model": "gpt-5.6-sol", "permission_mode": "default", "turn_id": "PRIVATE_TURN",
		"agent_id": "PRIVATE_AGENT", "agent_type": agentType, "agent_transcript_path": path,
		"stop_hook_active": false, "last_assistant_message": "PRIVATE_MESSAGE",
	})
	return string(encoded)
}

func codexStopBridgeHook(path string) string {
	encoded, _ := json.Marshal(map[string]any{
		"session_id": "PRIVATE_SESSION", "transcript_path": path, "cwd": "PRIVATE_CWD",
		"hook_event_name": "Stop", "model": "gpt-5.6-sol", "permission_mode": "default", "turn_id": "PRIVATE_TURN",
		"stop_hook_active": false, "last_assistant_message": "PRIVATE_MESSAGE",
	})
	return string(encoded)
}

type codexRoundTrip func(*http.Request) (*http.Response, error)

func (f codexRoundTrip) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

func TestSendCodexPolicyNormalizeAndSendOnce(t *testing.T) {
	home := codexBridgeHome(t, true)
	if err := state.Write(home, state.InstallState{
		CodexModelAssignments:      map[string]string{"sdd-apply": "medium"},
		CodexPhaseModelAssignments: map[string]string{"sdd-apply": "gpt-5.6-terra"},
	}); err != nil {
		t.Fatal(err)
	}
	transcript := filepath.Join(t.TempDir(), "rollout.jsonl")
	content := `{"type":"turn_context","payload":{"model":"gpt-5.6-sol","effort":"high","summary":"PRIVATE_SUMMARY"}}
{"type":"event_msg","payload":{"type":"token_count","info":{"last_token_usage":{"input_tokens":7,"output_tokens":2,"total_tokens":9}}}}
`
	if err := os.WriteFile(transcript, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	requests := 0
	client := &http.Client{Transport: codexRoundTrip(func(r *http.Request) (*http.Response, error) {
		requests++
		body, _ := io.ReadAll(r.Body)
		event, err := telemetry.ParseRuntimeEvent(body)
		if err != nil || event.Host != "codex" || len(event.Rows) != 1 {
			t.Fatalf("event=%+v err=%v", event, err)
		}
		row := event.Rows[0]
		if row.AgentClass != "sdd-apply" || row.SelectedEffort != "medium" || row.EffectiveEffort != "high" || row.Model.ID != "gpt-5.6-sol" || row.ModelEvidence != "response" {
			t.Fatalf("row=%+v", row)
		}
		for _, private := range []string{"PRIVATE", transcript, "transcript_path", "agent_id", "last_assistant_message"} {
			if bytes.Contains(body, []byte(private)) {
				t.Fatalf("private data leaked: %s", body)
			}
		}
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"schema":"gentle-ai.telemetry-runtime-delivery/v1","decision":"stored"}`))}, nil
	})}
	t.Setenv(telemetry.EndpointEnvVar, "https://telemetry.example.invalid")
	before := codexBridgeDisk(t, home)
	if got := SendCodex(context.Background(), home, os.Getenv, strings.NewReader(codexBridgeHook(transcript)), client); got != "stored" || requests != 1 {
		t.Fatalf("decision=%q requests=%d", got, requests)
	}
	if !reflect.DeepEqual(before, codexBridgeDisk(t, home)) {
		t.Fatal("Codex bridge mutated disk")
	}
}

func TestSendCodexDerivesTaskNameAndAssignmentFromTranscriptHead(t *testing.T) {
	home := codexBridgeHome(t, true)
	if err := state.Write(home, state.InstallState{
		CodexModelAssignments:      map[string]string{"sdd-explore": "xhigh"},
		CodexPhaseModelAssignments: map[string]string{"sdd-explore": "gpt-5.4"},
	}); err != nil {
		t.Fatal(err)
	}
	transcript := filepath.Join(t.TempDir(), "rollout.jsonl")
	if err := os.WriteFile(transcript, []byte(codexRealBridgeTranscriptFixture), 0o600); err != nil {
		t.Fatal(err)
	}
	client := &http.Client{Transport: codexRoundTrip(func(r *http.Request) (*http.Response, error) {
		body, _ := io.ReadAll(r.Body)
		event, err := telemetry.ParseRuntimeEvent(body)
		if err != nil {
			t.Fatal(err)
		}
		row := event.Rows[0]
		if row.AgentKind != "built_in" || row.AgentClass != "sdd-explore" || row.Model != (telemetry.RuntimeModel{Provider: "openai-codex", ID: "gpt-5.6-sol"}) || row.ModelEvidence != "response" || row.SelectedEffort != "xhigh" || row.EffectiveEffort != "medium" {
			t.Fatalf("row=%+v", row)
		}
		for name, got := range map[string]string{
			"input": string(row.Input), "cache_read": string(row.CacheRead), "cache_creation": string(row.CacheCreation),
			"output": string(row.Output), "reasoning": string(row.ReasoningTokens), "total": string(row.TotalTokens),
		} {
			want := map[string]string{"input": "120", "cache_read": "20", "cache_creation": "4", "output": "30", "reasoning": "10", "total": "164"}[name]
			if got != tokenReportedBridge(want) {
				t.Errorf("%s=%s want=%s", name, got, tokenReportedBridge(want))
			}
		}
		for _, private := range []string{"PRIVATE", "/root/sdd_explore", "agent_path", "agent_nickname"} {
			if bytes.Contains(body, []byte(private)) {
				t.Fatalf("private transcript metadata leaked: %s", body)
			}
		}
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"schema":"gentle-ai.telemetry-runtime-delivery/v1","decision":"stored"}`))}, nil
	})}
	t.Setenv(telemetry.EndpointEnvVar, "https://telemetry.example.invalid")
	if got := SendCodex(context.Background(), home, os.Getenv, strings.NewReader(codexBridgeHookForAgent(transcript, "default")), client); got != "stored" {
		t.Fatal(got)
	}
}

type codexNoRead struct{ t *testing.T }

func (r codexNoRead) Read([]byte) (int, error) {
	r.t.Fatal("read before telemetry policy")
	return 0, io.EOF
}

func TestSendCodexPolicyBeforeRead(t *testing.T) {
	home := codexBridgeHome(t, false)
	if got := SendCodex(context.Background(), home, os.Getenv, codexNoRead{t}, nil); got != "disabled" {
		t.Fatalf("decision=%q", got)
	}
}

func TestSendCodexSelectedModelFallbackAndUnknownAgent(t *testing.T) {
	home := codexBridgeHome(t, true)
	if err := state.Write(home, state.InstallState{
		CodexModelAssignments:      map[string]string{"sdd-apply": "xhigh"},
		CodexPhaseModelAssignments: map[string]string{"sdd-apply": "gpt-5.4"},
	}); err != nil {
		t.Fatal(err)
	}
	client := &http.Client{Transport: codexRoundTrip(func(r *http.Request) (*http.Response, error) {
		body, _ := io.ReadAll(r.Body)
		event, err := telemetry.ParseRuntimeEvent(body)
		if err != nil {
			t.Fatal(err)
		}
		row := event.Rows[0]
		if row.Model.ID != "gpt-5.4" || row.ModelEvidence != "selected" || row.SelectedEffort != "xhigh" {
			t.Fatalf("row=%+v", row)
		}
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"schema":"gentle-ai.telemetry-runtime-delivery/v1","decision":"stored"}`))}, nil
	})}
	t.Setenv(telemetry.EndpointEnvVar, "https://telemetry.example.invalid")
	if got := SendCodex(context.Background(), home, os.Getenv, strings.NewReader(codexBridgeHook(filepath.Join(t.TempDir(), "missing.jsonl"))), client); got != "stored" {
		t.Fatal(got)
	}
}

func TestSendCodexStopReadsTranscriptAsOrchestratorUsage(t *testing.T) {
	home := codexBridgeHome(t, true)
	transcript := filepath.Join(t.TempDir(), "rollout.jsonl")
	content := `{"type":"turn_context","payload":{"model":"gpt-5.6-sol","effort":"high"}}
{"type":"event_msg","payload":{"type":"token_count","info":{"last_token_usage":{"input_tokens":7,"total_tokens":9}}}}
`
	if err := os.WriteFile(transcript, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	client := &http.Client{Transport: codexRoundTrip(func(r *http.Request) (*http.Response, error) {
		body, _ := io.ReadAll(r.Body)
		event, err := telemetry.ParseRuntimeEvent(body)
		if err != nil {
			t.Fatal(err)
		}
		row := event.Rows[0]
		if row.AgentKind != "orchestrator" || row.AgentClass != "orchestrator" || row.Model.ID != "gpt-5.6-sol" || string(row.Input) != tokenReportedBridge("7") || string(row.TotalTokens) != tokenReportedBridge("9") {
			t.Fatalf("row=%+v", row)
		}
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"schema":"gentle-ai.telemetry-runtime-delivery/v1","decision":"stored"}`))}, nil
	})}
	t.Setenv(telemetry.EndpointEnvVar, "https://telemetry.example.invalid")
	if got := SendCodex(context.Background(), home, os.Getenv, strings.NewReader(codexStopBridgeHook(transcript)), client); got != "stored" {
		t.Fatal(got)
	}
}

func TestCodexTranscriptTailIsBoundedAndDropsPartialLine(t *testing.T) {
	path := filepath.Join(t.TempDir(), "rollout.jsonl")
	prefix := bytes.Repeat([]byte("x"), telemetry.CodexTranscriptMaxBytes)
	want := []byte("{\"type\":\"turn_context\",\"payload\":{\"model\":\"gpt-5.4\",\"effort\":\"low\"}}\n")
	if err := os.WriteFile(path, append(append(prefix, '\n'), want...), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := readCodexTranscriptTail(path)
	if err != nil || !bytes.Equal(got, want) || len(got) > telemetry.CodexTranscriptMaxBytes {
		t.Fatalf("tail len=%d err=%v got-prefix=%q", len(got), err, got[:min(len(got), 32)])
	}
}

func TestCodexTranscriptTailKeepsRecordAtExactBoundary(t *testing.T) {
	path := filepath.Join(t.TempDir(), "rollout.jsonl")
	record := []byte("{\"type\":\"turn_context\",\"payload\":{\"model\":\"gpt-5.6-sol\",\"effort\":\"high\"}}\n")
	want := append([]byte(nil), record...)
	want = append(want, bytes.Repeat([]byte("\n"), telemetry.CodexTranscriptMaxBytes-len(want))...)
	if err := os.WriteFile(path, append([]byte("discarded\n"), want...), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := readCodexTranscriptTail(path)
	if err != nil || !bytes.Equal(got, want) || len(got) != telemetry.CodexTranscriptMaxBytes {
		t.Fatalf("tail len=%d err=%v starts-with-record=%t", len(got), err, bytes.HasPrefix(got, record))
	}
}

func TestCodexTranscriptHeadIsBounded(t *testing.T) {
	path := filepath.Join(t.TempDir(), "rollout.jsonl")
	content := bytes.Repeat([]byte("x"), codexTranscriptHeadMaxBytes+1024)
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := readCodexTranscriptHead(path)
	if err != nil || len(got) != codexTranscriptHeadMaxBytes || !bytes.Equal(got, content[:codexTranscriptHeadMaxBytes]) {
		t.Fatalf("head len=%d err=%v", len(got), err)
	}
}

func tokenReportedBridge(value string) string {
	return `{"reported":1,"unavailable":0,"unsupported":0,"sum":` + value + `}`
}

func codexBridgeDisk(t *testing.T, root string) map[string]string {
	t.Helper()
	result := map[string]string{}
	if err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, path)
		info, err := entry.Info()
		if err != nil {
			return err
		}
		result[rel] = info.Mode().String()
		if !entry.IsDir() {
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			result[rel] += string(data)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	return result
}
