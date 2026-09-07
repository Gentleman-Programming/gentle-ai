package sddtaskresult

import (
	"encoding/json"
	"strings"
	"testing"
)

// Without change identity, the handoff stays byte-compatible with OpenCode.
// Consumers preserve it unchanged and follow its continuation exactly once,
// executing only supplied commands.

const wantUnscopedContinuation = "Return to the active SDD coordinator and inspect only its retained structured status for the selected change and artifact store. If that status is unavailable, report this terminal failure and ask the user to select the change and artifact store. Do not infer either, run unscoped status discovery, retry, or launch another phase."

func TestHandoffWithoutChangeRetainsCoordinatorIdentity(t *testing.T) {
	for _, cwd := range []string{"", "/", `C:\`, "/re'po"} {
		t.Run(cwd, func(t *testing.T) {
			for _, class := range []Class{ClassEmpty, ClassMalformed} {
				for _, handoff := range []string{
					Handoff(class, "sdd-apply", cwd, "", ""),
					DispatchLatched("sdd-verify", "sdd-apply", class.FailureCode(), cwd, ""),
				} {
					var decoded map[string]any
					if err := json.Unmarshal([]byte(strings.TrimPrefix(handoff, HandoffPrefix)), &decoded); err != nil {
						t.Fatal(err)
					}
					if decoded["continuation"] != wantUnscopedContinuation {
						t.Errorf("continuation = %#v, want non-command coordinator guidance", decoded["continuation"])
					}
					if decoded["status"] != "blocked" || decoded["schemaName"] != handoffSchema {
						t.Errorf("lost terminal v1 contract: %s", handoff)
					}
				}
			}
		})
	}
}

func TestHandoffCarriesThePrefixAndSchema(t *testing.T) {
	handoff := Handoff(ClassEmpty, "sdd-apply", "/repo", "", "")
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
		"continuation": wantUnscopedContinuation,
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
	handoff := Handoff(ClassMalformed, "sdd-verify", "/re'po", "feat'x", "")
	if err := json.Unmarshal([]byte(strings.TrimPrefix(handoff, "GENTLE_AI_SDD_FAILURE ")), &decoded); err != nil {
		t.Fatalf("handoff payload is not JSON: %v", err)
	}
	const want = `gentle-ai sdd-status 'feat'\''x' --cwd '/re'\''po' --json`
	if decoded["continuation"] != want {
		t.Errorf("continuation = %#v, want %q", decoded["continuation"], want)
	}
}

func TestHandoffCarriesAValidatedTaskModel(t *testing.T) {
	if handoff := Handoff(ClassEmpty, "sdd-apply", "/repo", "", "openai/gpt-5.6"); !strings.Contains(handoff, `"taskModel":"openai/gpt-5.6"`) {
		t.Errorf("handoff dropped a valid task model: %q", handoff)
	}
	// A route token that cannot be trusted is omitted, never echoed.
	if handoff := Handoff(ClassEmpty, "sdd-apply", "/repo", "", "not a model/../etc"); strings.Contains(handoff, "taskModel") {
		t.Errorf("handoff echoed an untrusted task model: %q", handoff)
	}
}

func TestDispatchLatchedNamesBothPhases(t *testing.T) {
	handoff := DispatchLatched("sdd-verify", "sdd-apply", "sdd_task_result_empty", "/repo", "")
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
	if handoff := Handoff(ClassOK, "sdd-apply", "/repo", "", ""); handoff != "" {
		t.Errorf("an admitted result produced a failure handoff: %q", handoff)
	}
}

// #2790: with two active changes, a selector-less sdd-status answers
// select-change, so a handoff that knows the change must name it.
func TestHandoffContinuationNamesTheChangeWhenKnown(t *testing.T) {
	for _, tt := range []struct{ change, want string }{
		{change: "", want: wantUnscopedContinuation},
		{change: "feat-x", want: "gentle-ai sdd-status 'feat-x' --cwd '/repo' --json"},
	} {
		var decoded map[string]any
		handoff := Handoff(ClassEmpty, "sdd-apply", "/repo", tt.change, "")
		if err := json.Unmarshal([]byte(strings.TrimPrefix(handoff, HandoffPrefix)), &decoded); err != nil {
			t.Fatalf("handoff payload is not JSON: %v", err)
		}
		if decoded["continuation"] != tt.want {
			t.Errorf("change %q: continuation = %#v, want %q", tt.change, decoded["continuation"], tt.want)
		}
		if latched := DispatchLatched("sdd-verify", "sdd-apply", "sdd_task_result_empty", "/repo", tt.change); !strings.Contains(latched, tt.want) {
			t.Errorf("change %q: latched handoff does not carry %q: %s", tt.change, tt.want, latched)
		}
	}
}
