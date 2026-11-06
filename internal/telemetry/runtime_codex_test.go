package telemetry

import (
	"encoding/json"
	"strings"
	"testing"
)

const codexSubagentHookFixture = `{"session_id":"PRIVATE_SESSION","transcript_path":"PRIVATE_PARENT_PATH","cwd":"PRIVATE_CWD","hook_event_name":"SubagentStop","model":"gpt-5.6-sol","permission_mode":"default","turn_id":"PRIVATE_TURN","agent_id":"PRIVATE_AGENT","agent_type":"sdd-apply","agent_transcript_path":"PRIVATE_AGENT_PATH","stop_hook_active":false,"last_assistant_message":"PRIVATE_MESSAGE"}`

const codexTranscriptFixture = `{"timestamp":"PRIVATE_TIME","type":"turn_context","payload":{"turn_id":"PRIVATE_TURN","cwd":"PRIVATE_CWD","model":"gpt-5.6-sol","effort":"high","summary":"PRIVATE_SUMMARY"}}
{"timestamp":"PRIVATE_TIME","type":"event_msg","payload":{"type":"token_count","info":{"last_token_usage":{"input_tokens":120,"cached_input_tokens":20,"cache_write_input_tokens":4,"output_tokens":30,"reasoning_output_tokens":10,"total_tokens":164},"total_token_usage":{"input_tokens":999}},"rate_limits":{"plan_type":"PRIVATE_PLAN"}}}
`

const codexRealSubagentTranscriptFixture = `{"timestamp":"PRIVATE_TIME","type":"session_meta","payload":{"id":"PRIVATE_SESSION","source":{"subagent":{"thread_spawn":{"parent_thread_id":"PRIVATE_PARENT","depth":1,"agent_nickname":"PRIVATE_NICKNAME","agent_path":"/root/sdd_explore"}}}}}
{"timestamp":"PRIVATE_TIME","type":"turn_context","payload":{"turn_id":"PRIVATE_TURN","model":"gpt-5.6-sol","effort":"medium","collaboration_mode":{"settings":{"reasoning_effort":"medium"}}}}
{"timestamp":"PRIVATE_TIME","type":"event_msg","payload":{"type":"token_count","info":{"last_token_usage":{"input_tokens":120,"cached_input_tokens":20,"cache_write_input_tokens":4,"output_tokens":30,"reasoning_output_tokens":10,"total_tokens":164}}}}
`

func TestCodexRuntimeCompletedSubagent(t *testing.T) {
	source, err := DecodeCodexHook(strings.NewReader(codexSubagentHookFixture))
	if err != nil || source == nil {
		t.Fatalf("decode hook: source=%+v err=%v", source, err)
	}
	o, err := NormalizeCodex(*source, strings.NewReader(codexTranscriptFixture), CodexAssignment{Model: "gpt-5.6-terra", Effort: "medium"})
	if err != nil || o == nil {
		t.Fatalf("normalize: observation=%+v err=%v", o, err)
	}
	r := o.Row
	if r.AgentKind != "built_in" || r.AgentClass != "sdd-apply" || r.Model != (RuntimeModel{Provider: "openai-codex", ID: "gpt-5.6-sol"}) || r.ModelEvidence != "response" {
		t.Fatalf("identity: %+v", r)
	}
	if r.SelectedEffort != "medium" || r.EffectiveEffort != "high" || string(r.Launches) != "1" || string(r.Responses) != "1" {
		t.Fatalf("selection/counts: %+v", r)
	}
	for name, got := range map[string]string{
		"input": string(r.Input), "cache_read": string(r.CacheRead), "cache_creation": string(r.CacheCreation),
		"output": string(r.Output), "reasoning": string(r.ReasoningTokens), "total": string(r.TotalTokens),
	} {
		want := map[string]string{"input": "120", "cache_read": "20", "cache_creation": "4", "output": "30", "reasoning": "10", "total": "164"}[name]
		if got != tokenReported(want) {
			t.Errorf("%s = %s, want %s", name, got, tokenReported(want))
		}
	}
	if r.Duration.Kind != "unavailable" || string(r.Duration.MeasuredCount) != "0" || string(r.Duration.SumMS) != "null" || r.ErrorCategory != "none" {
		t.Fatalf("non-token metrics: %+v", r)
	}
	assertRuntimeRetained(t, r, "codex")
	out, _ := json.Marshal(o)
	for _, private := range []string{"PRIVATE", "session_id", "transcript_path", "agent_id", "last_assistant_message"} {
		if strings.Contains(string(out), private) {
			t.Fatalf("private source data leaked: %s", out)
		}
	}
}

func TestCodexRuntimeUsesExistingAgentClassAllowlist(t *testing.T) {
	known := ""
	for _, candidate := range strings.Split(runtimeAgentClasses, "|") {
		if candidate != "" && candidate != "unknown" && candidate != "orchestrator" {
			known = candidate
			break
		}
	}
	if known == "" {
		t.Fatal("runtime agent-class allowlist has no subagent class")
	}
	unknown := "not-a-runtime-agent-class"
	if runtimeMember(unknown, runtimeAgentClasses) {
		t.Fatal("test unknown unexpectedly belongs to runtime agent-class allowlist")
	}
	for _, tt := range []struct {
		agentType, wantKind, wantClass string
	}{
		{known, "built_in", known},
		{unknown, "custom", "unknown"},
		{"", "custom", "unknown"},
	} {
		t.Run(tt.agentType, func(t *testing.T) {
			hook := strings.Replace(codexSubagentHookFixture, `"agent_type":"sdd-apply"`, `"agent_type":"`+tt.agentType+`"`, 1)
			source, err := DecodeCodexHook(strings.NewReader(hook))
			if err != nil || source == nil {
				t.Fatal(err)
			}
			o, err := NormalizeCodex(*source, strings.NewReader(""), CodexAssignment{})
			if err != nil || o == nil || o.Row.AgentKind != tt.wantKind || o.Row.AgentClass != tt.wantClass {
				t.Fatalf("observation=%+v err=%v", o, err)
			}
		})
	}
}

func TestCodexRuntimeDerivesAllowlistedAgentFromFirstSessionMeta(t *testing.T) {
	for _, tt := range []struct {
		name, agentType, transcriptHead, wantAgentType string
	}{
		{"unknown hook uses underscored task path", "default", codexRealSubagentTranscriptFixture, "sdd-explore"},
		{"direct allowlisted hook wins", "sdd-apply", codexRealSubagentTranscriptFixture, "sdd-apply"},
		{"unknown derived name stays unknown", "default", strings.Replace(codexRealSubagentTranscriptFixture, "/root/sdd_explore", "/root/private_custom", 1), "default"},
		{"only first record is eligible", "default", `{"type":"event_msg","payload":{}}
` + codexRealSubagentTranscriptFixture, "default"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			source := CodexHook{Event: "SubagentStop", AgentType: tt.agentType}
			resolved, err := ResolveCodexAgentType(source, strings.NewReader(tt.transcriptHead))
			if err != nil || resolved.AgentType != tt.wantAgentType {
				t.Fatalf("resolved=%+v err=%v", resolved, err)
			}
		})
	}
}

func TestCodexRuntimeEffectiveEffortFallsBackToCollaborationSettings(t *testing.T) {
	transcript := `{"type":"turn_context","payload":{"model":"gpt-5.6-sol","effort":null,"collaboration_mode":{"settings":{"reasoning_effort":"medium"}}}}
`
	o, err := NormalizeCodex(CodexHook{Event: "SubagentStop", AgentType: "sdd-explore"}, strings.NewReader(transcript), CodexAssignment{Effort: "xhigh"})
	if err != nil || o == nil {
		t.Fatal(err)
	}
	if o.Row.SelectedEffort != "xhigh" || o.Row.EffectiveEffort != "medium" {
		t.Fatalf("row=%+v", o.Row)
	}
}

func TestCodexRuntimeStopIsOrchestratorAndSelectedFallback(t *testing.T) {
	hook := `{"session_id":"PRIVATE_SESSION","transcript_path":null,"cwd":"PRIVATE_CWD","hook_event_name":"Stop","model":"gpt-5.6-sol","permission_mode":"default","turn_id":"PRIVATE_TURN","stop_hook_active":false,"last_assistant_message":null}`
	source, err := DecodeCodexHook(strings.NewReader(hook))
	if err != nil || source == nil {
		t.Fatal(err)
	}
	o, err := NormalizeCodex(*source, strings.NewReader(""), CodexAssignment{Model: "gpt-5.6-terra", Effort: "medium"})
	if err != nil || o == nil {
		t.Fatal(err)
	}
	if o.Row.AgentKind != "orchestrator" || o.Row.AgentClass != "orchestrator" || o.Row.Model.ID != "gpt-5.6-terra" || o.Row.ModelEvidence != "selected" || o.Row.SelectedEffort != "medium" || o.Row.EffectiveEffort != "unavailable" || string(o.Row.Launches) != "null" || string(o.Row.Responses) != "1" {
		t.Fatalf("row: %+v", o.Row)
	}
}

func TestCodexRuntimeFinalContextSegmentOwnsEvidence(t *testing.T) {
	for _, tt := range []struct {
		name                  string
		transcript            string
		selected              CodexAssignment
		wantModel             RuntimeModel
		wantModelEvidence     string
		wantEffort, wantInput string
	}{
		{
			name: "latest context and latest token count win together",
			transcript: `{"type":"turn_context","payload":{"model":"gpt-5.4","effort":"low"}}
{"type":"event_msg","payload":{"type":"token_count","info":{"last_token_usage":{"input_tokens":1}}}}
{"type":"turn_context","payload":{"model":"gpt-5.6-sol","effort":"high"}}
{"type":"event_msg","payload":{"type":"token_count","info":{"last_token_usage":{"input_tokens":2}}}}
{"type":"event_msg","payload":{"type":"token_count","info":{"last_token_usage":{"input_tokens":3}}}}
`,
			wantModel:         RuntimeModel{Provider: "openai-codex", ID: "gpt-5.6-sol"},
			wantModelEvidence: "response",
			wantEffort:        "high",
			wantInput:         tokenReported("3"),
		},
		{
			name: "invalid final context clears prior turn evidence",
			transcript: `{"type":"turn_context","payload":{"model":"gpt-5.4","effort":"low"}}
{"type":"event_msg","payload":{"type":"token_count","info":{"last_token_usage":{"input_tokens":1}}}}
{"type":"turn_context","payload":{"model":123,"effort":"PRIVATE_EFFORT"}}
{"type":"event_msg","payload":{"type":"token_count","info":{"last_token_usage":{"input_tokens":2}}}}
`,
			selected:          CodexAssignment{Model: "gpt-5.6-terra"},
			wantModel:         RuntimeModel{Provider: "openai-codex", ID: "gpt-5.6-terra"},
			wantModelEvidence: "selected",
			wantEffort:        "unavailable",
			wantInput:         tokenReported("2"),
		},
		{
			name: "final context without token clears prior usage",
			transcript: `{"type":"turn_context","payload":{"model":"gpt-5.4","effort":"low"}}
{"type":"event_msg","payload":{"type":"token_count","info":{"last_token_usage":{"input_tokens":1}}}}
{"type":"turn_context","payload":{"model":"gpt-5.6-sol","effort":"medium"}}
`,
			wantModel:         RuntimeModel{Provider: "openai-codex", ID: "gpt-5.6-sol"},
			wantModelEvidence: "response",
			wantEffort:        "medium",
			wantInput:         tokenAbsent,
		},
		{
			name: "token before any context is not attributed",
			transcript: `{"type":"event_msg","payload":{"type":"token_count","info":{"last_token_usage":{"input_tokens":7}}}}
`,
			wantModel:         RuntimeModel{Provider: "unknown", ID: "unknown"},
			wantModelEvidence: "unknown",
			wantEffort:        "unavailable",
			wantInput:         tokenAbsent,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			source, err := DecodeCodexHook(strings.NewReader(codexSubagentHookFixture))
			if err != nil || source == nil {
				t.Fatal(err)
			}
			o, err := NormalizeCodex(*source, strings.NewReader(tt.transcript), tt.selected)
			if err != nil || o == nil {
				t.Fatal(err)
			}
			if o.Row.Model != tt.wantModel || o.Row.ModelEvidence != tt.wantModelEvidence || o.Row.EffectiveEffort != tt.wantEffort || string(o.Row.Input) != tt.wantInput {
				t.Fatalf("row: %+v", o.Row)
			}
		})
	}
}

func TestCodexRuntimeStopRetainsOrchestratorTranscriptUsage(t *testing.T) {
	source := CodexHook{Event: "Stop"}
	o, err := NormalizeCodex(source, strings.NewReader(codexTranscriptFixture), CodexAssignment{})
	if err != nil || o == nil {
		t.Fatal(err)
	}
	if o.Row.AgentKind != "orchestrator" || o.Row.AgentClass != "orchestrator" || o.Row.Model.ID != "gpt-5.6-sol" || string(o.Row.Input) != tokenReported("120") || string(o.Row.TotalTokens) != tokenReported("164") {
		t.Fatalf("row: %+v", o.Row)
	}
}

func TestCodexRuntimeHookCasesAndBounds(t *testing.T) {
	for _, tt := range []struct {
		name, input string
		ignored     bool
		invalid     bool
	}{
		{"other event", strings.Replace(codexSubagentHookFixture, "SubagentStop", "SubagentStart", 1), true, false},
		{"duplicate", strings.Replace(codexSubagentHookFixture, `"session_id":`, `"session_id":"x","session_id":`, 1), false, true},
		{"case alias", strings.Replace(codexSubagentHookFixture, `"hook_event_name"`, `"Hook_Event_Name"`, 1), false, true},
		{"nullable parent path", strings.Replace(codexSubagentHookFixture, `"transcript_path":"PRIVATE_PARENT_PATH"`, `"transcript_path":null`, 1), false, false},
		{"nullable subagent path", strings.Replace(codexSubagentHookFixture, `"agent_transcript_path":"PRIVATE_AGENT_PATH"`, `"agent_transcript_path":null`, 1), false, false},
		{"missing subagent path", strings.Replace(codexSubagentHookFixture, `,"agent_transcript_path":"PRIVATE_AGENT_PATH"`, "", 1), false, true},
		{"missing model", strings.Replace(codexSubagentHookFixture, `,"model":"gpt-5.6-sol"`, "", 1), false, true},
		{"wrong model type", strings.Replace(codexSubagentHookFixture, `"model":"gpt-5.6-sol"`, `"model":153`, 1), false, true},
		{"wrong active type", strings.Replace(codexSubagentHookFixture, `"stop_hook_active":false`, `"stop_hook_active":"false"`, 1), false, true},
		{"unknown field", strings.Replace(codexSubagentHookFixture, `"agent_id":`, `"prompt":"PRIVATE_PROMPT","agent_id":`, 1), false, true},
		{"oversized", strings.Repeat(" ", CodexMaxBytes+1), false, true},
		{"trailing", codexSubagentHookFixture + `{}`, false, true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			source, err := DecodeCodexHook(strings.NewReader(tt.input))
			if (err != nil) != tt.invalid || (err == nil && (source == nil) != tt.ignored) {
				t.Fatalf("source=%+v err=%v", source, err)
			}
		})
	}
	if source, _ := DecodeCodexHook(strings.NewReader(codexSubagentHookFixture)); source != nil {
		if _, err := NormalizeCodex(*source, strings.NewReader(strings.Repeat("x", CodexTranscriptMaxBytes+1)), CodexAssignment{}); err == nil {
			t.Fatal("oversized transcript accepted")
		}
	}
}
