package sddtaskresult

import (
	"encoding/json"
	"strings"
	"testing"
)

// The handoff must stay byte-compatible with what the OpenCode transport
// already emits, because consumers are told to preserve it unchanged and run
// its continuation exactly once.

func TestHandoffCarriesThePrefixAndSchema(t *testing.T) {
	handoff := Handoff(ClassEmpty, "sdd-apply", "/repo", "")
	if !strings.HasPrefix(handoff, "GENTLE_AI_SDD_FAILURE ") {
		t.Fatalf("handoff lost its literal prefix: %q", handoff)
	}
	var decoded map[string]any
	if err := json.Unmarshal([]byte(strings.TrimPrefix(handoff, "GENTLE_AI_SDD_FAILURE ")), &decoded); err != nil {
		t.Fatalf("handoff payload is not JSON: %v", err)
	}
	for field, want := range map[string]any{
		"schemaName":   "gentle-ai.sdd-task-result-failure/v1",
		"status":       "blocked",
		"code":         "sdd_task_result_empty",
		"phase":        "sdd-apply",
		"continuation": "gentle-ai sdd-status --cwd '/repo' --json",
	} {
		if decoded[field] != want {
			t.Errorf("handoff %s = %#v, want %#v", field, decoded[field], want)
		}
	}
	if _, present := decoded["taskModel"]; present {
		t.Error("handoff carries taskModel when none was supplied")
	}
}

func TestHandoffQuotesACwdContainingASingleQuote(t *testing.T) {
	// Assert on the DECODED continuation: comparing against the handoff string
	// would compare against JSON escaping rather than the command a consumer
	// actually runs.
	var decoded map[string]any
	handoff := Handoff(ClassMalformed, "sdd-verify", "/re'po", "")
	if err := json.Unmarshal([]byte(strings.TrimPrefix(handoff, "GENTLE_AI_SDD_FAILURE ")), &decoded); err != nil {
		t.Fatalf("handoff payload is not JSON: %v", err)
	}
	const want = `gentle-ai sdd-status --cwd '/re'\''po' --json`
	if decoded["continuation"] != want {
		t.Errorf("continuation = %#v, want %q", decoded["continuation"], want)
	}
}

func TestHandoffCarriesAValidatedTaskModel(t *testing.T) {
	if handoff := Handoff(ClassEmpty, "sdd-apply", "/repo", "openai/gpt-5.6"); !strings.Contains(handoff, `"taskModel":"openai/gpt-5.6"`) {
		t.Errorf("handoff dropped a valid task model: %q", handoff)
	}
	// A route token that cannot be trusted is omitted, never echoed.
	if handoff := Handoff(ClassEmpty, "sdd-apply", "/repo", "not a model/../etc"); strings.Contains(handoff, "taskModel") {
		t.Errorf("handoff echoed an untrusted task model: %q", handoff)
	}
}

func TestDispatchLatchedNamesBothPhases(t *testing.T) {
	handoff := DispatchLatched("sdd-verify", "sdd-apply", "sdd_task_result_empty", "/repo")
	var decoded map[string]any
	if err := json.Unmarshal([]byte(strings.TrimPrefix(handoff, "GENTLE_AI_SDD_FAILURE ")), &decoded); err != nil {
		t.Fatalf("latched payload is not JSON: %v", err)
	}
	for field, want := range map[string]any{
		"code":         "sdd_task_dispatch_latched",
		"phase":        "sdd-verify",
		"latchedPhase": "sdd-apply",
		"latchedCode":  "sdd_task_result_empty",
	} {
		if decoded[field] != want {
			t.Errorf("latched %s = %#v, want %#v", field, decoded[field], want)
		}
	}
	if decoded["exit"] == nil || decoded["exit"] == "" {
		t.Error("latched handoff carries no exit")
	}
}

func TestHandoffIsEmptyForAnAdmittedResult(t *testing.T) {
	if handoff := Handoff(ClassOK, "sdd-apply", "/repo", ""); handoff != "" {
		t.Errorf("an admitted result produced a failure handoff: %q", handoff)
	}
}
