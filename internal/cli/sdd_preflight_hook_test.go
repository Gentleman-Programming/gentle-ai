package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const testSDDPreflightQuestions = `{"questions":[{"header":"Pace","question":"Gentle AI SDD preflight 1/3: How should phases run?","options":[{"label":"Interactive","description":"Pause"},{"label":"Automatic","description":"Continue"}]},{"header":"Artifacts","question":"Gentle AI SDD preflight 2/3: Where should artifacts live?","options":[{"label":"OpenSpec","description":"Files"},{"label":"Engram","description":"Memory"},{"label":"Both","description":"Both"}]},{"header":"PR strategy","question":"Gentle AI SDD preflight 3/3: How should oversized delivery be handled?","options":[{"label":"Ask me","description":"Ask"},{"label":"Single PR","description":"One"},{"label":"Auto","description":"Split"}]}]}`

const testSDDPreflightAnswersAutomatic = `{"Gentle AI SDD preflight 1/3: How should phases run?":"Automatic","Gentle AI SDD preflight 2/3: Where should artifacts live?":"OpenSpec","Gentle AI SDD preflight 3/3: How should oversized delivery be handled?":"Ask me"}`

const testSDDPreflightAnswersInteractive = `{"Gentle AI SDD preflight 1/3: How should phases run?":"Interactive","Gentle AI SDD preflight 2/3: Where should artifacts live?":"Engram","Gentle AI SDD preflight 3/3: How should oversized delivery be handled?":"Auto"}`

func runSDDPreflightHookTest(t *testing.T, payload string) (string, error) {
	t.Helper()
	var out bytes.Buffer
	err := runSDDPreflightHook([]string{"--agent", "claude-code"}, strings.NewReader(payload), &out)
	return out.String(), err
}

// writeSDDPreflightTranscript writes one root-transcript JSONL file made of
// assistant tool_use / user tool_result record pairs, using the real Claude
// Code record shape (top-level type/isSidechain/sessionId/message keys).
func writeSDDPreflightTranscript(t *testing.T, dir, sessionID string, pairs []struct {
	toolUseID  string
	sidechain  bool
	sessionID  string
	answersRaw string // raw JSON object literal for toolUseResult.answers; empty means no tool_result at all
	isError    bool
}) string {
	t.Helper()
	path := filepath.Join(dir, sessionID+".jsonl")
	var sb strings.Builder
	for _, pair := range pairs {
		recordSession := pair.sessionID
		if recordSession == "" {
			recordSession = sessionID
		}
		assistant := map[string]any{
			"type":        "assistant",
			"isSidechain": pair.sidechain,
			"sessionId":   recordSession,
			"uuid":        "assistant-" + pair.toolUseID,
			"message": map[string]any{
				"role": "assistant",
				"content": []any{
					map[string]any{
						"type":  "tool_use",
						"id":    pair.toolUseID,
						"name":  "AskUserQuestion",
						"input": json.RawMessage(testSDDPreflightQuestions),
					},
				},
			},
		}
		assistantEncoded, err := json.Marshal(assistant)
		if err != nil {
			t.Fatal(err)
		}
		sb.Write(assistantEncoded)
		sb.WriteString("\n")

		if pair.answersRaw == "" {
			continue
		}
		toolResultContent := map[string]any{
			"tool_use_id": pair.toolUseID,
			"type":        "tool_result",
			"content":     "The user answered the questions.",
		}
		if pair.isError {
			toolResultContent["is_error"] = true
		}
		user := map[string]any{
			"type":        "user",
			"isSidechain": pair.sidechain,
			"sessionId":   recordSession,
			"uuid":        "user-" + pair.toolUseID,
			"message": map[string]any{
				"role":    "user",
				"content": []any{toolResultContent},
			},
			"toolUseResult": map[string]any{
				"questions":   json.RawMessage(testSDDPreflightQuestions[len(`{"questions":`) : len(testSDDPreflightQuestions)-1]),
				"answers":     json.RawMessage(pair.answersRaw),
				"annotations": map[string]any{},
			},
		}
		userEncoded, err := json.Marshal(user)
		if err != nil {
			t.Fatal(err)
		}
		sb.Write(userEncoded)
		sb.WriteString("\n")
	}
	if err := os.WriteFile(path, []byte(sb.String()), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func preToolUseAgentPayload(sessionID, transcriptPath, subagentType, prompt, agentID string) string {
	toolInput := `{"subagent_type":` + jsonStr(subagentType) + `,"prompt":` + jsonStr(prompt) + `,"description":"desc","model":"sonnet"}`
	extra := ""
	if agentID != "" {
		extra = `,"agent_id":` + jsonStr(agentID)
	}
	return `{"session_id":` + jsonStr(sessionID) + `,"transcript_path":` + jsonStr(transcriptPath) + `,"hook_event_name":"PreToolUse","tool_name":"Agent","tool_input":` + toolInput + extra + `}`
}

func jsonStr(s string) string {
	encoded, err := json.Marshal(s)
	if err != nil {
		panic(err)
	}
	return string(encoded)
}

func TestSDDPreflightHookAllowsCorroboratedCanonicalPreflight(t *testing.T) {
	dir := t.TempDir()
	sessionID := "sess-1"
	transcript := writeSDDPreflightTranscript(t, dir, sessionID, []struct {
		toolUseID  string
		sidechain  bool
		sessionID  string
		answersRaw string
		isError    bool
	}{
		{toolUseID: "toolu-1", answersRaw: testSDDPreflightAnswersAutomatic},
	})
	pre := preToolUseAgentPayload(sessionID, transcript, "sdd-explore", "Explore the request", "")
	out, err := runSDDPreflightHookTest(t, pre)
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("decode output %q: %v", out, err)
	}
	specific := got["hookSpecificOutput"].(map[string]any)
	if specific["permissionDecision"] != "allow" {
		t.Fatalf("decision = %#v", specific)
	}
	updated := specific["updatedInput"].(map[string]any)
	prompt := updated["prompt"].(string)
	for _, want := range []string{"## SDD Session Preflight", "- Pace: auto", "- Artifact store: openspec", "- Delivery strategy: ask-on-risk", "- Review policy: 400 changed lines", "Explore the request"} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("prompt missing %q: %s", want, prompt)
		}
	}
	if !strings.HasPrefix(prompt, "## SDD Session Preflight") {
		t.Fatalf("block must precede original prompt: %s", prompt)
	}
	if updated["description"] != "desc" || updated["model"] != "sonnet" {
		t.Fatalf("other input keys not preserved: %#v", updated)
	}
}

func TestSDDPreflightHookLastAnswerWins(t *testing.T) {
	dir := t.TempDir()
	sessionID := "sess-last"
	transcript := writeSDDPreflightTranscript(t, dir, sessionID, []struct {
		toolUseID  string
		sidechain  bool
		sessionID  string
		answersRaw string
		isError    bool
	}{
		{toolUseID: "toolu-1", answersRaw: testSDDPreflightAnswersAutomatic},
		{toolUseID: "toolu-2", answersRaw: testSDDPreflightAnswersInteractive},
	})
	pre := preToolUseAgentPayload(sessionID, transcript, "sdd-apply", "Apply", "")
	out, err := runSDDPreflightHookTest(t, pre)
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatal(err)
	}
	specific := got["hookSpecificOutput"].(map[string]any)
	updated := specific["updatedInput"].(map[string]any)
	prompt := updated["prompt"].(string)
	if !strings.Contains(prompt, "- Pace: interactive") || !strings.Contains(prompt, "- Artifact store: engram") || !strings.Contains(prompt, "- Delivery strategy: auto-chain") {
		t.Fatalf("last answer did not win: %s", prompt)
	}
}

func denyCase(t *testing.T, payload string, wantContains ...string) {
	t.Helper()
	out, err := runSDDPreflightHookTest(t, payload)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, `"permissionDecision":"deny"`) || !strings.Contains(out, "SDD child dispatch refused:") {
		t.Fatalf("output = %s", out)
	}
	for _, want := range wantContains {
		if !strings.Contains(out, want) {
			t.Fatalf("output missing %q: %s", want, out)
		}
	}
}

func TestSDDPreflightHookDeniesMissingPreflight(t *testing.T) {
	dir := t.TempDir()
	sessionID := "sess-missing"
	transcript := writeSDDPreflightTranscript(t, dir, sessionID, nil)
	pre := preToolUseAgentPayload(sessionID, transcript, "sdd-apply", "Apply", "")
	denyCase(t, pre, "missing, invalid, or uncorroborated")
}

func TestSDDPreflightHookDeniesSidechainPreflight(t *testing.T) {
	dir := t.TempDir()
	sessionID := "sess-sidechain"
	transcript := writeSDDPreflightTranscript(t, dir, sessionID, []struct {
		toolUseID  string
		sidechain  bool
		sessionID  string
		answersRaw string
		isError    bool
	}{
		{toolUseID: "toolu-1", sidechain: true, answersRaw: testSDDPreflightAnswersAutomatic},
	})
	pre := preToolUseAgentPayload(sessionID, transcript, "sdd-apply", "Apply", "")
	denyCase(t, pre, "missing, invalid, or uncorroborated")
}

func TestSDDPreflightHookDeniesForeignSessionPreflight(t *testing.T) {
	dir := t.TempDir()
	sessionID := "sess-foreign"
	transcript := writeSDDPreflightTranscript(t, dir, sessionID, []struct {
		toolUseID  string
		sidechain  bool
		sessionID  string
		answersRaw string
		isError    bool
	}{
		{toolUseID: "toolu-1", sessionID: "sess-other", answersRaw: testSDDPreflightAnswersAutomatic},
	})
	pre := preToolUseAgentPayload(sessionID, transcript, "sdd-apply", "Apply", "")
	denyCase(t, pre, "missing, invalid, or uncorroborated")
}

func TestSDDPreflightHookDeniesUnansweredPreflight(t *testing.T) {
	dir := t.TempDir()
	sessionID := "sess-unanswered"
	transcript := writeSDDPreflightTranscript(t, dir, sessionID, []struct {
		toolUseID  string
		sidechain  bool
		sessionID  string
		answersRaw string
		isError    bool
	}{
		{toolUseID: "toolu-1", answersRaw: ""},
	})
	pre := preToolUseAgentPayload(sessionID, transcript, "sdd-apply", "Apply", "")
	denyCase(t, pre, "missing, invalid, or uncorroborated")
}

func TestSDDPreflightHookDeniesErroredToolResult(t *testing.T) {
	dir := t.TempDir()
	sessionID := "sess-errored"
	transcript := writeSDDPreflightTranscript(t, dir, sessionID, []struct {
		toolUseID  string
		sidechain  bool
		sessionID  string
		answersRaw string
		isError    bool
	}{
		{toolUseID: "toolu-1", answersRaw: testSDDPreflightAnswersAutomatic, isError: true},
	})
	pre := preToolUseAgentPayload(sessionID, transcript, "sdd-apply", "Apply", "")
	denyCase(t, pre, "missing, invalid, or uncorroborated")
}

func TestSDDPreflightHookDeniesOutsideDomainAnswer(t *testing.T) {
	dir := t.TempDir()
	sessionID := "sess-baddomain"
	transcript := writeSDDPreflightTranscript(t, dir, sessionID, []struct {
		toolUseID  string
		sidechain  bool
		sessionID  string
		answersRaw string
		isError    bool
	}{
		{toolUseID: "toolu-1", answersRaw: `{"Gentle AI SDD preflight 1/3: How should phases run?":"Nonsense","Gentle AI SDD preflight 2/3: Where should artifacts live?":"OpenSpec","Gentle AI SDD preflight 3/3: How should oversized delivery be handled?":"Ask me"}`},
	})
	pre := preToolUseAgentPayload(sessionID, transcript, "sdd-apply", "Apply", "")
	denyCase(t, pre, "missing, invalid, or uncorroborated")
}

func TestSDDPreflightHookDeniesMultiSelectAnswer(t *testing.T) {
	dir := t.TempDir()
	sessionID := "sess-multiselect"
	transcript := writeSDDPreflightTranscript(t, dir, sessionID, []struct {
		toolUseID  string
		sidechain  bool
		sessionID  string
		answersRaw string
		isError    bool
	}{
		{toolUseID: "toolu-1", answersRaw: `{"Gentle AI SDD preflight 1/3: How should phases run?":["Automatic","Interactive"],"Gentle AI SDD preflight 2/3: Where should artifacts live?":"OpenSpec","Gentle AI SDD preflight 3/3: How should oversized delivery be handled?":"Ask me"}`},
	})
	pre := preToolUseAgentPayload(sessionID, transcript, "sdd-apply", "Apply", "")
	denyCase(t, pre, "missing, invalid, or uncorroborated")
}

func TestSDDPreflightHookDeniesReorderedCanonicalLabels(t *testing.T) {
	dir := t.TempDir()
	sessionID := "sess-reordered"
	path := filepath.Join(dir, sessionID+".jsonl")
	reordered := strings.Replace(testSDDPreflightQuestions, `"label":"Interactive"`, `"label":"TEMP"`, 1)
	reordered = strings.Replace(reordered, `"label":"Automatic"`, `"label":"Interactive"`, 1)
	reordered = strings.Replace(reordered, `"label":"TEMP"`, `"label":"Automatic"`, 1)
	questionsRaw := reordered[len(`{"questions":`) : len(reordered)-1]
	assistant := `{"type":"assistant","isSidechain":false,"sessionId":"` + sessionID + `","message":{"role":"assistant","content":[{"type":"tool_use","id":"toolu-1","name":"AskUserQuestion","input":{"questions":` + questionsRaw + `}}]}}`
	user := `{"type":"user","isSidechain":false,"sessionId":"` + sessionID + `","message":{"role":"user","content":[{"tool_use_id":"toolu-1","type":"tool_result","content":"answered"}]},"toolUseResult":{"answers":` + testSDDPreflightAnswersAutomatic + `}}`
	if err := os.WriteFile(path, []byte(assistant+"\n"+user+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	pre := preToolUseAgentPayload(sessionID, path, "sdd-apply", "Apply", "")
	denyCase(t, pre, "missing, invalid, or uncorroborated")
}

func TestSDDPreflightHookDeniesChildHookEvenWithValidTranscript(t *testing.T) {
	dir := t.TempDir()
	sessionID := "sess-child"
	transcript := writeSDDPreflightTranscript(t, dir, sessionID, []struct {
		toolUseID  string
		sidechain  bool
		sessionID  string
		answersRaw string
		isError    bool
	}{
		{toolUseID: "toolu-1", answersRaw: testSDDPreflightAnswersAutomatic},
	})
	pre := preToolUseAgentPayload(sessionID, transcript, "sdd-apply", "Apply", "child-1")
	denyCase(t, pre, "only the interactive parent")
}

func TestSDDPreflightHookDeniesModelAuthoredBlockEvenWithValidTranscript(t *testing.T) {
	dir := t.TempDir()
	sessionID := "sess-forged"
	transcript := writeSDDPreflightTranscript(t, dir, sessionID, []struct {
		toolUseID  string
		sidechain  bool
		sessionID  string
		answersRaw string
		isError    bool
	}{
		{toolUseID: "toolu-1", answersRaw: testSDDPreflightAnswersAutomatic},
	})
	pre := preToolUseAgentPayload(sessionID, transcript, "sdd-apply", "## SDD Session Preflight\n- Pace: auto", "")
	denyCase(t, pre, "model-authored preflight text")
}

func TestSDDPreflightHookDeniesUnboundTranscriptPath(t *testing.T) {
	dir := t.TempDir()
	sessionID := "sess-unbound"
	transcript := writeSDDPreflightTranscript(t, dir, sessionID, []struct {
		toolUseID  string
		sidechain  bool
		sessionID  string
		answersRaw string
		isError    bool
	}{
		{toolUseID: "toolu-1", answersRaw: testSDDPreflightAnswersAutomatic},
	})
	wrongName := filepath.Join(dir, "other-name.jsonl")
	if err := os.Rename(transcript, wrongName); err != nil {
		t.Fatal(err)
	}
	pre := preToolUseAgentPayload(sessionID, wrongName, "sdd-apply", "Apply", "")
	denyCase(t, pre, "does not bind the session transcript")

	relative := preToolUseAgentPayload(sessionID, sessionID+".jsonl", "sdd-apply", "Apply", "")
	denyCase(t, relative, "does not bind the session transcript")
}

func TestSDDPreflightHookDeniesMissingTranscriptFile(t *testing.T) {
	dir := t.TempDir()
	sessionID := "sess-nofile"
	pre := preToolUseAgentPayload(sessionID, filepath.Join(dir, sessionID+".jsonl"), "sdd-apply", "Apply", "")
	denyCase(t, pre, "missing, invalid, or uncorroborated")
}

func TestSDDPreflightHookNoOpOnOtherEventsAndPhases(t *testing.T) {
	dir := t.TempDir()
	sessionID := "sess-noop"
	transcript := writeSDDPreflightTranscript(t, dir, sessionID, []struct {
		toolUseID  string
		sidechain  bool
		sessionID  string
		answersRaw string
		isError    bool
	}{
		{toolUseID: "toolu-1", answersRaw: testSDDPreflightAnswersAutomatic},
	})

	post := `{"session_id":` + jsonStr(sessionID) + `,"transcript_path":` + jsonStr(transcript) + `,"hook_event_name":"PostToolUse","tool_name":"Agent","tool_input":{"subagent_type":"sdd-apply","prompt":"Apply"}}`
	if out, err := runSDDPreflightHookTest(t, post); err != nil || out != "" {
		t.Fatalf("PostToolUse should be a no-op: out=%q err=%v", out, err)
	}

	nonPhase := preToolUseAgentPayload(sessionID, transcript, "general-purpose", "Explore", "")
	if out, err := runSDDPreflightHookTest(t, nonPhase); err != nil || out != "" {
		t.Fatalf("non-sdd-* subagent should be a no-op: out=%q err=%v", out, err)
	}
}

func TestSDDPreflightHookSkipsMalformedTranscriptLines(t *testing.T) {
	dir := t.TempDir()
	sessionID := "sess-malformed-lines"
	path := filepath.Join(dir, sessionID+".jsonl")
	assistant := `{"type":"assistant","isSidechain":false,"sessionId":"` + sessionID + `","message":{"role":"assistant","content":[{"type":"tool_use","id":"toolu-1","name":"AskUserQuestion","input":{"questions":` + testSDDPreflightQuestions[len(`{"questions":`):len(testSDDPreflightQuestions)-1] + `}}]}}`
	user := `{"type":"user","isSidechain":false,"sessionId":"` + sessionID + `","message":{"role":"user","content":[{"tool_use_id":"toolu-1","type":"tool_result","content":"answered"}]},"toolUseResult":{"answers":` + testSDDPreflightAnswersAutomatic + `}}`
	content := "not-json-at-all\n" + assistant + "\n{broken json\n" + user + "\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	pre := preToolUseAgentPayload(sessionID, path, "sdd-apply", "Apply", "")
	out, err := runSDDPreflightHookTest(t, pre)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, `"permissionDecision":"allow"`) {
		t.Fatalf("malformed lines should be skipped, not fatal: %s", out)
	}
}

// TestSDDPreflightHookSkipsOverlongTranscriptLineWithoutLosingLaterPreflight
// guards against a transcript whose first line is a single JSON record
// larger than maxSDDPreflightTranscriptLine (for example a huge tool result
// embedded in one line): that line must be discarded, not fatal, and must
// not prevent scanning from reaching a later, valid canonical preflight pair.
func TestSDDPreflightHookSkipsOverlongTranscriptLineWithoutLosingLaterPreflight(t *testing.T) {
	dir := t.TempDir()
	sessionID := "sess-overlong-line"
	path := filepath.Join(dir, sessionID+".jsonl")

	huge := `{"type":"user","isSidechain":false,"sessionId":"` + sessionID + `","huge":"` + strings.Repeat("A", 9*1024*1024) + `"}`
	assistant := `{"type":"assistant","isSidechain":false,"sessionId":"` + sessionID + `","message":{"role":"assistant","content":[{"type":"tool_use","id":"toolu-1","name":"AskUserQuestion","input":{"questions":` + testSDDPreflightQuestions[len(`{"questions":`):len(testSDDPreflightQuestions)-1] + `}}]}}`
	user := `{"type":"user","isSidechain":false,"sessionId":"` + sessionID + `","message":{"role":"user","content":[{"tool_use_id":"toolu-1","type":"tool_result","content":"answered"}]},"toolUseResult":{"answers":` + testSDDPreflightAnswersAutomatic + `}}`
	content := huge + "\n" + assistant + "\n" + user + "\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	pre := preToolUseAgentPayload(sessionID, path, "sdd-apply", "Apply", "")
	out, err := runSDDPreflightHookTest(t, pre)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, `"permissionDecision":"allow"`) {
		t.Fatalf("overlong line should be discarded, not fatal, and the later preflight must still corroborate: %s", out)
	}
}

func TestSDDPreflightHookCoversEveryPackagedPhaseAndProfile(t *testing.T) {
	for _, phase := range []string{"sdd-init", "sdd-explore", "sdd-research", "sdd-propose", "sdd-spec", "sdd-design", "sdd-tasks", "sdd-apply", "sdd-verify", "sdd-archive", "sdd-onboard"} {
		if !isSDDPreflightHookPhase(phase) || !isSDDPreflightHookPhase(phase+"-backend") {
			t.Fatalf("phase %q is not guarded", phase)
		}
	}
	for _, notPhase := range []string{"general", "explore", "sdd", "sdd-unknown"} {
		if isSDDPreflightHookPhase(notPhase) {
			t.Fatalf("non-phase %q was guarded as an SDD phase", notPhase)
		}
	}
}

func TestSDDPreflightHookRequiresSupportedAgent(t *testing.T) {
	var out bytes.Buffer
	err := runSDDPreflightHook([]string{"--agent", "opencode"}, strings.NewReader(`{}`), &out)
	if err == nil {
		t.Fatal("unsupported agent should error")
	}
}

func TestSDDPreflightHookRejectsPositionalArguments(t *testing.T) {
	var out bytes.Buffer
	err := runSDDPreflightHook([]string{"--agent", "claude-code", "extra"}, strings.NewReader(`{}`), &out)
	if err == nil {
		t.Fatal("positional argument should error")
	}
}
