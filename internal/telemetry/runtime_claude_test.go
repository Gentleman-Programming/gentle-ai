package telemetry

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
	"testing"
)

// expectedClaudeStopDeliveryID mirrors the documented Stop delivery-id
// derivation as an independent test oracle: SHA-256 of a fixed
// domain-separation prefix concatenated with the message id, truncated to
// the first 16 bytes and hex-encoded.
func expectedClaudeStopDeliveryID(messageID string) string {
	sum := sha256.Sum256([]byte("gentle-ai.telemetry-runtime-claude-stop/v1\x00" + messageID))
	return hex.EncodeToString(sum[:16])
}

const claudeSubagentHook = `{"session_id":"PRIVATE_SESSION","transcript_path":"PRIVATE_MAIN_PATH","cwd":"PRIVATE_CWD","permission_mode":"default","hook_event_name":"SubagentStop","stop_hook_active":false,"agent_id":"PRIVATE_AGENT_ID","agent_type":"sdd-apply","agent_transcript_path":"PRIVATE_AGENT_PATH","last_assistant_message":"FINAL_MESSAGE"}`

func TestClaudeRuntimeSubagentSelectsLastAssistantUsageBeforeTrailingRecords(t *testing.T) {
	hook, err := ParseClaudeHook(strings.NewReader(claudeSubagentHook))
	if err != nil {
		t.Fatal(err)
	}
	transcript := []byte("not-json\n" +
		`{"type":"assistant","message":{"model":"claude-haiku-4-5","usage":{"input_tokens":1,"output_tokens":2,"cache_read_input_tokens":3,"cache_creation_input_tokens":4}}}` + "\n" +
		`{"type":"user","message":{"content":"PRIVATE_PROMPT"}}` + "\n" +
		`{"type":"assistant","message":{"model":"claude-opus-5","content":[{"type":"text","text":"FINAL_MESSAGE"}],"usage":{"input_tokens":11,"output_tokens":12,"cache_read_input_tokens":13,"cache_creation_input_tokens":14}}}` + "\n")
	usage, ok := ParseClaudeTranscriptTail(transcript, false, hook.LastAssistantDigest)
	if !ok {
		t.Fatal("usage not found")
	}
	if !usage.Correlated {
		t.Fatal("matching final message was not retained as stronger evidence")
	}
	o := NormalizeClaude(hook, usage, []byte("---\nname: sdd-apply\nmodel: claude-haiku-4-5\neffort: high\n---\nPRIVATE_PROMPT"))
	if o == nil {
		t.Fatal("observation missing")
	}
	if o.Row.AgentKind != "built_in" || o.Row.AgentClass != "sdd-apply" || o.Row.Model != (RuntimeModel{Provider: "anthropic", ID: "claude-opus-5"}) || o.Row.ModelEvidence != "response" || o.Row.SelectedEffort != "high" || o.Row.EffectiveEffort != "unavailable" {
		t.Fatalf("row: %+v", o.Row)
	}
	for got, want := range map[string]string{string(o.Row.Input): tokenReported("11"), string(o.Row.Output): tokenReported("12"), string(o.Row.CacheRead): tokenReported("13"), string(o.Row.CacheCreation): tokenReported("14"), string(o.Row.ReasoningTokens): `{"reported":0,"unavailable":0,"unsupported":1,"sum":0}`, string(o.Row.TotalTokens): tokenAbsent} {
		if got != want {
			t.Fatalf("token %s, want %s", got, want)
		}
	}
	if string(o.Row.Responses) != "1" || string(o.Row.Launches) != "null" || o.Row.Duration.Kind != "unavailable" || o.Row.ErrorCategory != "none" {
		t.Fatalf("coverage: %+v", o.Row)
	}
	if o.DeliveryID != "" {
		t.Fatalf("SubagentStop must never derive a hashed delivery id: %q", o.DeliveryID)
	}
	out, _ := json.Marshal(o)
	if strings.Contains(string(out), "PRIVATE") {
		t.Fatalf("private source leaked: %s", out)
	}
}

func TestClaudeRuntimeSubagentUsesUsageWithoutLastMessageCorrelation(t *testing.T) {
	mismatching := []byte(`{"type":"assistant","message":{"model":"claude-opus-5","content":[{"type":"text","text":"STALE_MESSAGE"}],"usage":{"input_tokens":99,"output_tokens":99}}}` + "\n")
	for name, input := range map[string]string{
		"mismatching message": claudeSubagentHook,
		"missing message":     strings.Replace(claudeSubagentHook, `,"last_assistant_message":"FINAL_MESSAGE"`, "", 1),
	} {
		t.Run(name, func(t *testing.T) {
			hook, err := ParseClaudeHook(strings.NewReader(input))
			if err != nil {
				t.Fatal(err)
			}
			usage, ok := ParseClaudeTranscriptTail(mismatching, false, hook.LastAssistantDigest)
			if !ok || !usage.Evidence || usage.Correlated || string(usage.Input) != "99" {
				t.Fatalf("optional final-message correlation discarded subagent usage: %+v %v", usage, ok)
			}
			o := NormalizeClaude(hook, usage, nil)
			if string(o.Row.Launches) != "null" || string(o.Row.Responses) != "1" || string(o.Row.Input) != tokenReported("99") {
				t.Fatalf("optional final-message correlation did not report response: %+v", o.Row)
			}
		})
	}
}

// TestClaudeRuntimeStopAttributesTranscriptIdempotently replaces the former
// "Stop never attributes transcript evidence" behavior: Stop now reads its
// own transcript tail the same way SubagentStop reads a subagent transcript,
// and delivery is made idempotent through a deterministic hash of the
// transcript's API message id rather than by discarding the evidence.
func TestClaudeRuntimeStopAttributesTranscriptIdempotently(t *testing.T) {
	input := strings.Replace(claudeSubagentHook, `"hook_event_name":"SubagentStop","stop_hook_active":false,"agent_id":"PRIVATE_AGENT_ID","agent_type":"sdd-apply","agent_transcript_path":"PRIVATE_AGENT_PATH"`, `"hook_event_name":"Stop","stop_hook_active":false`, 1)
	hook, err := ParseClaudeHook(strings.NewReader(input))
	if err != nil {
		t.Fatal(err)
	}
	record := func(messageID string) []byte {
		idField := ""
		if messageID != "" {
			idField = `"id":"` + messageID + `",`
		}
		return []byte(`{"type":"assistant","uuid":"record-uuid-1","message":{` + idField + `"model":"claude-opus-5","content":"FINAL_MESSAGE","usage":{"input_tokens":77,"output_tokens":88}}}` + "\n")
	}

	transcript := record("msg_abc123")
	usage, ok := ParseClaudeTranscriptTail(transcript, false, hook.LastAssistantDigest)
	if !ok || usage.MessageID != "msg_abc123" {
		t.Fatalf("message id not captured: %+v %v", usage, ok)
	}
	o := NormalizeClaude(hook, usage, nil)
	if o == nil {
		t.Fatal("observation missing")
	}
	if string(o.Row.Launches) != "null" || string(o.Row.Responses) != "1" || o.Row.ModelEvidence != "response" || o.Row.Model.ID != "claude-opus-5" || o.Row.AgentKind != "orchestrator" || o.Row.AgentClass != "orchestrator" || string(o.Row.Input) != tokenReported("77") || string(o.Row.Output) != tokenReported("88") {
		t.Fatalf("Stop did not attribute transcript evidence: %+v", o.Row)
	}
	want := expectedClaudeStopDeliveryID("msg_abc123")
	if o.DeliveryID != want {
		t.Fatalf("delivery id = %q, want %q", o.DeliveryID, want)
	}

	// The same transcript parsed twice yields the same delivery id.
	usageAgain, ok := ParseClaudeTranscriptTail(transcript, false, hook.LastAssistantDigest)
	if !ok {
		t.Fatal("re-parse failed")
	}
	if oAgain := NormalizeClaude(hook, usageAgain, nil); oAgain.DeliveryID != want {
		t.Fatalf("delivery id not stable across re-parse: got %q want %q", oAgain.DeliveryID, want)
	}

	// A different message id yields a different delivery id.
	otherUsage, ok := ParseClaudeTranscriptTail(record("msg_other456"), false, hook.LastAssistantDigest)
	if !ok {
		t.Fatal("other transcript parse failed")
	}
	otherObservation := NormalizeClaude(hook, otherUsage, nil)
	if otherObservation.DeliveryID == want || otherObservation.DeliveryID != expectedClaudeStopDeliveryID("msg_other456") {
		t.Fatalf("delivery id did not vary with message id: %q", otherObservation.DeliveryID)
	}

	// A record without message.id falls back to the record-level uuid.
	noMessageID := []byte(`{"type":"assistant","uuid":"record-uuid-2","message":{"model":"claude-opus-5","content":"FINAL_MESSAGE","usage":{"input_tokens":1}}}` + "\n")
	fallbackUsage, ok := ParseClaudeTranscriptTail(noMessageID, false, hook.LastAssistantDigest)
	if !ok || fallbackUsage.MessageID != "record-uuid-2" {
		t.Fatalf("uuid fallback not applied: %+v %v", fallbackUsage, ok)
	}
	fallbackObservation := NormalizeClaude(hook, fallbackUsage, nil)
	if fallbackObservation.DeliveryID != expectedClaudeStopDeliveryID("record-uuid-2") {
		t.Fatalf("delivery id not derived from uuid fallback: %q", fallbackObservation.DeliveryID)
	}

	// A record with neither id yields an empty delivery id but still carries usage.
	neither := []byte(`{"type":"assistant","message":{"model":"claude-opus-5","content":"FINAL_MESSAGE","usage":{"input_tokens":5}}}` + "\n")
	neitherUsage, ok := ParseClaudeTranscriptTail(neither, false, hook.LastAssistantDigest)
	if !ok || neitherUsage.MessageID != "" || string(neitherUsage.Input) != "5" {
		t.Fatalf("neither-id usage mishandled: %+v %v", neitherUsage, ok)
	}
	neitherObservation := NormalizeClaude(hook, neitherUsage, nil)
	if neitherObservation.DeliveryID != "" || string(neitherObservation.Row.Input) != tokenReported("5") {
		t.Fatalf("neither-id observation mishandled: %+v %q", neitherObservation.Row, neitherObservation.DeliveryID)
	}

	// A Stop hook with no readable transcript stays activity-only with an empty delivery id.
	noEvidence := NormalizeClaude(hook, ClaudeUsage{}, nil)
	if string(noEvidence.Row.Launches) != "1" || string(noEvidence.Row.Responses) != "null" || noEvidence.Row.ModelEvidence != "unknown" || noEvidence.Row.Model.ID != "unknown" || noEvidence.DeliveryID != "" {
		t.Fatalf("no-evidence Stop unexpectedly attributed: %+v", noEvidence.Row)
	}
}

func TestClaudeRuntimeSubagentCorrelatesMultipleTextBlocks(t *testing.T) {
	hook, err := ParseClaudeHook(strings.NewReader(claudeSubagentHook))
	if err != nil {
		t.Fatal(err)
	}
	transcript := []byte(`{"type":"assistant","message":{"model":"claude-opus-5","content":[{"type":"text","text":"FINAL_"},{"type":"text","text":"MESSAGE"}],"usage":{"input_tokens":55}}}` + "\n")
	usage, ok := ParseClaudeTranscriptTail(transcript, false, hook.LastAssistantDigest)
	if !ok || string(usage.Input) != "55" {
		t.Fatalf("multi-block correlation failed: %+v %v", usage, ok)
	}
}

func TestClaudeRuntimeSubagentUsesLastAssistantUsage(t *testing.T) {
	hook, err := ParseClaudeHook(strings.NewReader(claudeSubagentHook))
	if err != nil {
		t.Fatal(err)
	}
	matching := `{"type":"assistant","message":{"model":"claude-opus-5","content":"FINAL_MESSAGE","usage":{"input_tokens":66}}}`
	for name, transcript := range map[string]string{
		"later non-usage record":    matching + "\n" + `{"type":"user","message":{"content":"next"}}` + "\n",
		"partial trailing record":   matching + "\n" + `{"type":"assistant","message":{"content":"FINAL_MESSAGE"`,
		"later malformed JSON line": matching + "\n" + `{bad` + "\n",
	} {
		t.Run(name, func(t *testing.T) {
			usage, ok := ParseClaudeTranscriptTail([]byte(transcript), false, hook.LastAssistantDigest)
			if !ok || !usage.Evidence || string(usage.Input) != "66" {
				t.Fatalf("last assistant usage not selected: %+v %v", usage, ok)
			}
		})
	}
}

func TestClaudeModelAliasesAndUnknownSelectors(t *testing.T) {
	hook, _ := ParseClaudeHook(strings.NewReader(claudeSubagentHook))
	for _, tt := range []struct {
		selector string
		want     RuntimeModel
	}{
		{selector: "sonnet", want: RuntimeModel{Provider: "anthropic", ID: "claude-sonnet-5"}},
		{selector: "opus", want: RuntimeModel{Provider: "anthropic", ID: "claude-opus-5"}},
		{selector: "haiku", want: RuntimeModel{Provider: "anthropic", ID: "claude-haiku-4-5"}},
		{selector: "inherit", want: RuntimeModel{Provider: "unknown", ID: "unknown"}},
		{selector: "default", want: RuntimeModel{Provider: "unknown", ID: "unknown"}},
		{selector: "", want: RuntimeModel{Provider: "unknown", ID: "unknown"}},
	} {
		t.Run(tt.selector, func(t *testing.T) {
			definition := []byte("---\nmodel: " + tt.selector + "\n---\n")
			o := NormalizeClaude(hook, ClaudeUsage{}, definition)
			if o.Row.Model != tt.want {
				t.Fatalf("model = %+v, want %+v", o.Row.Model, tt.want)
			}
			wantEvidence := "selected"
			if tt.want.Provider == "unknown" {
				wantEvidence = "unknown"
			}
			if o.Row.ModelEvidence != wantEvidence {
				t.Fatalf("model evidence = %q, want %q", o.Row.ModelEvidence, wantEvidence)
			}
		})
	}
}

// TestClaudeDatedTranscriptModelPassesThroughGenericPattern replaces the former
// closed-registry "longest prefix" folding: a dated or revisioned Claude model
// id now survives unfolded as long as it still matches the generic claude
// family pattern, instead of being collapsed onto a registered base id.
func TestClaudeDatedTranscriptModelPassesThroughGenericPattern(t *testing.T) {
	hook, _ := ParseClaudeHook(strings.NewReader(claudeSubagentHook))
	for _, tt := range []struct {
		model string
		want  string
	}{
		{model: "claude-sonnet-5-20260501", want: "claude-sonnet-5-20260501"},
		{model: "claude-opus-5-1", want: "claude-opus-5-1"},
		{model: "claude-haiku-4-5-20251001", want: "claude-haiku-4-5-20251001"},
	} {
		t.Run(tt.model, func(t *testing.T) {
			o := NormalizeClaude(hook, ClaudeUsage{Evidence: true, Model: tt.model, Input: json.RawMessage("1")}, nil)
			if o.Row.Model != (RuntimeModel{Provider: "anthropic", ID: tt.want}) || o.Row.ModelEvidence != "response" {
				t.Fatalf("model = %+v evidence = %q", o.Row.Model, o.Row.ModelEvidence)
			}
		})
	}
}

func TestClaudeRuntimeNoUsageBecomesLaunchStyle(t *testing.T) {
	for _, input := range []string{
		claudeSubagentHook,
		strings.Replace(claudeSubagentHook, `"hook_event_name":"SubagentStop","stop_hook_active":false,"agent_id":"PRIVATE_AGENT_ID","agent_type":"sdd-apply","agent_transcript_path":"PRIVATE_AGENT_PATH"`, `"hook_event_name":"Stop","stop_hook_active":false`, 1),
	} {
		hook, err := ParseClaudeHook(strings.NewReader(input))
		if err != nil {
			t.Fatal(err)
		}
		o := NormalizeClaude(hook, ClaudeUsage{}, nil)
		if o == nil || string(o.Row.Launches) != "1" || string(o.Row.Responses) != "null" {
			t.Fatalf("no-usage coverage: %+v", o)
		}
		if hook.HookEventName == "Stop" && (o.Row.AgentKind != "orchestrator" || o.Row.AgentClass != "orchestrator") {
			t.Fatalf("orchestrator mapping: %+v", o.Row)
		}
	}
}

func TestClaudeRuntimeAgentClassUsesCurrentAllowlist(t *testing.T) {
	for _, agentType := range strings.Split(runtimeAgentClasses, "|") {
		if agentType == "orchestrator" || agentType == "worker" || agentType == "explore" || agentType == "verify" || agentType == "unknown" {
			continue
		}
		input := strings.Replace(claudeSubagentHook, `"agent_type":"sdd-apply"`, `"agent_type":"`+agentType+`"`, 1)
		hook, err := ParseClaudeHook(strings.NewReader(input))
		if err != nil {
			t.Fatal(err)
		}
		o := NormalizeClaude(hook, ClaudeUsage{}, nil)
		if o.Row.AgentKind != "built_in" || o.Row.AgentClass != agentType {
			t.Fatalf("%s: %+v", agentType, o.Row)
		}
	}
	unknown, err := ParseClaudeHook(strings.NewReader(strings.Replace(claudeSubagentHook, `"agent_type":"sdd-apply"`, `"agent_type":"PRIVATE_CUSTOM"`, 1)))
	if err != nil {
		t.Fatal(err)
	}
	o := NormalizeClaude(unknown, ClaudeUsage{}, nil)
	if o.Row.AgentKind != "custom" || o.Row.AgentClass != "unknown" {
		t.Fatalf("unknown: %+v", o.Row)
	}
}

func TestClaudeRuntimeIgnoredAndBounds(t *testing.T) {
	hook, err := ParseClaudeHook(strings.NewReader(strings.Replace(claudeSubagentHook, "SubagentStop", "SubagentStart", 1)))
	if err != nil || NormalizeClaude(hook, ClaudeUsage{}, nil) != nil {
		t.Fatalf("ignored: %v", err)
	}
	for _, input := range []string{strings.Repeat("x", ClaudeMaxBytes+1), claudeSubagentHook + `{}`, strings.Replace(claudeSubagentHook, `"agent_id":"PRIVATE_AGENT_ID"`, `"agent_id":"x","agent_id":"y"`, 1)} {
		if _, err := ParseClaudeHook(strings.NewReader(input)); err == nil {
			t.Fatal("accepted invalid hook")
		}
	}
}

func TestClaudeTranscriptTailBoundsAndMalformedLines(t *testing.T) {
	if _, ok := ParseClaudeTranscriptTail(make([]byte, ClaudeTranscriptMaxBytes+1), false, sha256.Sum256([]byte("MATCH"))); ok {
		t.Fatal("accepted oversized tail")
	}
	line := `{"type":"assistant","message":{"model":"claude-opus-5","usage":{"input_tokens":7,"output_tokens":8,"cache_read_input_tokens":9,"cache_creation_input_tokens":10}}}`
	usage, ok := ParseClaudeTranscriptTail([]byte("partial\n{bad\n"+strings.Replace(line, `"usage"`, `"content":"MATCH","usage"`, 1)+"\n"), true, sha256.Sum256([]byte("MATCH")))
	if !ok || string(usage.Input) != "7" {
		t.Fatalf("usage: %+v %v", usage, ok)
	}
}

func TestClaudeTranscriptTailMessageIDBoundsAndFallback(t *testing.T) {
	oversized := strings.Repeat("a", 129)
	transcript := []byte(`{"type":"assistant","uuid":"u1","message":{"id":"` + oversized + `","model":"claude-opus-5","usage":{"input_tokens":1}}}` + "\n")
	usage, ok := ParseClaudeTranscriptTail(transcript, false, [32]byte{})
	if !ok || usage.MessageID != "" {
		t.Fatalf("oversized message id not dropped: %+v %v", usage, ok)
	}

	// "café" contains a byte outside the printable-ASCII range and must be
	// dropped without falling back to the record uuid, because message.id is
	// present (not absent).
	nonPrintable := []byte(`{"type":"assistant","uuid":"u2","message":{"id":"msg_café","model":"claude-opus-5","usage":{"input_tokens":1}}}` + "\n")
	usage, ok = ParseClaudeTranscriptTail(nonPrintable, false, [32]byte{})
	if !ok || usage.MessageID != "" {
		t.Fatalf("non-printable message id not dropped: %+v %v", usage, ok)
	}

	multiline := []byte(
		`{"type":"assistant","uuid":"older","message":{"id":"msg_old","model":"claude-opus-5","usage":{"input_tokens":1}}}` + "\n" +
			`{"type":"assistant","uuid":"newer","message":{"id":"msg_new","model":"claude-opus-5","usage":{"input_tokens":2}}}` + "\n")
	usage, ok = ParseClaudeTranscriptTail(multiline, false, [32]byte{})
	if !ok || usage.MessageID != "msg_new" {
		t.Fatalf("newest record message id not selected: %+v %v", usage, ok)
	}
}

func TestClaudeTranscriptExactBoundaryKeepsFirstRecord(t *testing.T) {
	line := []byte(`{"type":"assistant","message":{"model":"claude-opus-5","content":"MATCH","usage":{"input_tokens":31}}}` + "\n")
	usage, ok := ParseClaudeTranscriptTail(line, false, sha256.Sum256([]byte("MATCH")))
	if !ok || string(usage.Input) != "31" {
		t.Fatalf("boundary record lost: %+v %v", usage, ok)
	}
}

func TestClaudeFrontmatterFallbackAndInvalidValues(t *testing.T) {
	hook, _ := ParseClaudeHook(strings.NewReader(claudeSubagentHook))
	o := NormalizeClaude(hook, ClaudeUsage{}, []byte("---\nname: sdd-apply\nmodel: claude-haiku-4-5\neffort: xhigh\n---\nignored"))
	if o.Row.Model.ID != "claude-haiku-4-5" || o.Row.ModelEvidence != "selected" || o.Row.SelectedEffort != "xhigh" {
		t.Fatalf("fallback: %+v", o.Row)
	}
	o = NormalizeClaude(hook, ClaudeUsage{}, []byte("---\nmodel: PRIVATE_MODEL\neffort: PRIVATE_EFFORT\n---\n"))
	if o.Row.Model != (RuntimeModel{Provider: "custom", ID: "custom"}) || o.Row.SelectedEffort != "unavailable" {
		t.Fatalf("custom: %+v", o.Row)
	}
	o = NormalizeClaude(hook, ClaudeUsage{Evidence: true, Input: json.RawMessage("1")}, []byte("---\nmodel: claude-haiku-4-5\neffort: low\n---\n"))
	if o.Row.Model.ID != "claude-haiku-4-5" || o.Row.ModelEvidence != "selected" || string(o.Row.Responses) != "1" {
		t.Fatalf("model fallback with usage: %+v", o.Row)
	}
}
