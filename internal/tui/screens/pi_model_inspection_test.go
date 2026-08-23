package screens

import (
	"encoding/json"
	"errors"
	"github.com/gentleman-programming/gentle-ai/v2/internal/agents/pi"
	"strings"
	"testing"
)

func TestPiModelInspectionEditorTransitionsAndOwnership(t *testing.T) {
	model, thinking := "z/model", pi.ModelRoutingThinkingMax
	assignment := pi.ModelRoutingAssignment{Model: &model, Thinking: &thinking}
	state := NewPiModelInspectionState()
	state.SetResult(pi.ModelRoutingInspection{
		Targets: map[pi.ModelRoutingTarget]pi.ModelRoutingTargetInspection{
			pi.ModelRoutingTargetProject: {Assignments: map[string]pi.ModelRoutingAssignment{"worker": assignment}},
			pi.ModelRoutingTargetGlobal:  {Assignments: map[string]pi.ModelRoutingAssignment{}},
		}, Agents: []pi.ModelRoutingAgent{{Name: "worker", Configurable: true}},
		Models: []pi.ModelRoutingModel{
			{CanonicalID: "z/model", Provider: "z", Configured: true, Available: true, SupportedThinkingLevels: []string{"off", "max"}},
			{CanonicalID: "a/model", Provider: "a", Configured: true, Available: true},
			{CanonicalID: "z/catalog", Provider: "z", Catalog: true, Configured: false, Available: true},
			{CanonicalID: "z/down", Provider: "z", Configured: true, Available: false},
		},
	}, nil)
	if got := state.ProviderOptions(); len(got) != 2 || got[0] != "a" || got[1] != "z" {
		t.Fatalf("providers = %v", got)
	}
	if !state.BeginEdit() || state.Mode != PiModelInspectionModeProviders || state.SelectedProvider != "z" {
		t.Fatalf("begin = %#v", state)
	}
	state.SelectEditor()
	if state.Mode != PiModelInspectionModeModels || len(state.ModelOptions()) != 1 {
		t.Fatalf("model phase = %#v", state)
	}
	state.SelectEditor()
	if got := state.ThinkingOptions(); len(got) != 3 || got[1] != "off" || got[2] != "max" {
		t.Fatalf("thinking = %v", got)
	}
	state.Cursor = 2
	state.SelectEditor()
	draft := state.Draft["worker"]
	if draft.Model == nil || *draft.Model != model || draft.Thinking == nil || *draft.Thinking != thinking || draft.Model == assignment.Model || state.Inspection.Targets[pi.ModelRoutingTargetProject].Assignments["worker"].Model != assignment.Model {
		t.Fatalf("draft/ownership = %#v", draft)
	}
	if !state.Rows()[0].Pending {
		t.Fatal("committed row is not pending")
	}
	state.SelectTarget(pi.ModelRoutingTargetGlobal)
	if !state.Rows()[0].Pending {
		t.Fatal("target switch erased pending marker")
	}
	state.BeginEdit()
	state.Cursor = 0
	state.SelectEditor()
	state.SelectEditor()
	if got := state.Draft["worker"]; got.Model != nil || got.Thinking != nil {
		t.Fatalf("inherit draft = %#v", got)
	}
	state.BeginEdit()
	state.BackEditor()
	if state.Mode != PiModelInspectionModeAgents || state.Draft["worker"].Model != nil {
		t.Fatal("esc did not back without mutation")
	}
}

func TestPiModelInspectionStateRowsScrollAndSafeError(t *testing.T) {
	state := NewPiModelInspectionState()
	if state.Status != PiModelInspectionLoading || state.Target != pi.ModelRoutingTargetProject {
		t.Fatal("state must start loading at project")
	}
	var inspection pi.ModelRoutingInspection
	payload := `{"targets":{"project":{"provenance":{"source":"project","status":"valid"},"assignments":{"a":{"model":"m","thinking":"high"},"b":{"inheritModel":true,"thinking":"low"},"c":{"model":"p","inheritThinking":true},"d":{}}},"global":{"provenance":{"source":"global","status":"valid"},"assignments":{"a":{"model":"g","thinking":"low"}}}},"agents":[{"name":"a","configurable":true},{"name":"b","configurable":true},{"name":"c","configurable":true},{"name":"d","configurable":true},{"name":"hidden","configurable":false}]}`
	if err := json.Unmarshal([]byte(payload), &inspection); err != nil {
		t.Fatal(err)
	}
	state.SetResult(inspection, nil)
	rows := state.Rows()
	if len(rows) != 4 || rows[0].Model != "m" || rows[1].Model != "inherited" || rows[1].Thinking != "low" || rows[2].Thinking != "inherited" || rows[3].Model != "unset" {
		t.Fatalf("rows = %#v", rows)
	}
	state.Cursor = 3
	state.Move(0, 15)
	if state.Scroll != 1 {
		t.Fatalf("bottom scroll = %d, want 1", state.Scroll)
	}
	if out := RenderPiModelInspection(state, 24); !strings.Contains(out, "a  model: m") || !strings.Contains(out, "b  model: inherited") || !strings.Contains(out, "c  model: p") || !strings.Contains(out, "d  model: unset") {
		t.Fatalf("resized render = %q", out)
	}
	state.SelectTarget(pi.ModelRoutingTargetGlobal)
	if state.Rows()[0].Model != "g" || state.Cursor != 0 || state.Scroll != 0 {
		t.Fatalf("global target = %#v", state)
	}
	if out := RenderPiModelInspection(state, 24); !strings.Contains(out, "source: global") || strings.Contains(out, "hidden") {
		t.Fatal("render lost provenance or filtered agent")
	}
	state.SetResult(pi.ModelRoutingInspection{}, errors.New("bad\x1b\nerror\t"))
	out := RenderPiModelInspection(state, 24)
	if strings.Contains(out, "\x1b") || strings.Contains(out, "\t") || !strings.Contains(out, "Details:") {
		t.Fatalf("unsafe error render: %q", out)
	}
}
func TestPiModelInspectionValidationStatesRenderSafely(t *testing.T) {
	state := NewPiModelInspectionState()
	state.SetResult(pi.ModelRoutingInspection{}, nil)
	model := "provider/model"
	state.Draft = pi.ModelRoutingDraft{"worker": {Model: &model}}
	if !state.BeginValidation() || state.Mode != PiModelInspectionModeValidating || state.ValidationErr != nil {
		t.Fatalf("begin validation = %#v", state)
	}
	if out := RenderPiModelInspection(state, 10); !strings.Contains(out, "Validating") || strings.Contains(out, "No changes were applied") {
		t.Fatalf("validating render = %q", out)
	}
	warning := "warning\x1b\nmessage"
	state.SetValidationResult(pi.ModelRoutingValidationResult{OK: true, Diagnostics: []pi.ModelRoutingDiagnostic{{Code: "W", Message: warning, Severity: pi.ModelRoutingDiagnosticSeverityWarning}}}, nil)
	out := RenderPiModelInspection(state, 10)
	if state.Mode != PiModelInspectionModeReviewReady || !strings.Contains(out, "No changes were applied") || !strings.Contains(out, "warningmessage") || strings.ContainsAny(out, "\x1b\t") {
		t.Fatalf("review-ready = %#v/%q", state, out)
	}
	state.ClearValidation()
	state.BeginValidation()
	err := &pi.ModelRoutingClientError{Kind: pi.ModelRoutingClientErrorSemantic, ExitClass: "invalid-input", Cause: errors.New("provider-secret")}
	state.SetValidationResult(pi.ModelRoutingValidationResult{Diagnostics: []pi.ModelRoutingDiagnostic{{Code: "E", Message: "bad\x1b\ninput", Severity: pi.ModelRoutingDiagnosticSeverityError}}}, err)
	out = RenderPiModelInspection(state, 10)
	if state.Mode != PiModelInspectionModeValidationResult || state.ValidationErr != err || !strings.Contains(out, "invalid") || !strings.Contains(out, "badinput") || strings.Contains(out, "provider-secret") || strings.ContainsAny(out, "\x1b\t") {
		t.Fatalf("invalid result = %#v/%q", state, out)
	}
	for _, tt := range []struct {
		name, want string
		err        error
	}{
		{name: "unsupported", want: "does not support", err: &pi.ModelRoutingClientError{Kind: pi.ModelRoutingClientErrorSemantic, ExitClass: "unsupported-contract"}},
		{name: "unavailable", want: "unavailable", err: &pi.ModelRoutingClientError{Kind: pi.ModelRoutingClientErrorSemantic, ExitClass: "unavailable-runtime"}},
		{name: "timeout", want: "timed out", err: &pi.TransportError{Kind: pi.TransportErrorTimeout, Cause: errors.New("private timeout")}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			state.ClearValidation()
			state.BeginValidation()
			state.SetValidationResult(pi.ModelRoutingValidationResult{}, tt.err)
			if out := RenderPiModelInspection(state, 10); !strings.Contains(out, tt.want) || strings.Contains(out, "private timeout") {
				t.Fatalf("error render = %q", out)
			}
		})
	}
}

func TestPiModelApplyResultRendersTypedOutcomesAndSafeMaterialization(t *testing.T) {
	target, path := pi.ModelRoutingTargetProject, "/cfg\x1b[31m"
	cases := []struct {
		name   string
		result pi.ModelRoutingApplyResult
		want   []string
	}{
		{"success", pi.ModelRoutingApplyResult{Outcome: pi.ModelRoutingApplyOutcomeSuccess, Saved: true, Target: &target, ConfigPath: &path, Materialization: &pi.ModelRoutingApplyMaterialization{Affected: []string{"global", "project"}, Succeeded: []string{"global", "project"}}}, []string{"saved", "/cfg[31m", "affected=2"}},
		{"persistence", pi.ModelRoutingApplyResult{Outcome: pi.ModelRoutingApplyOutcomePersistenceFailure}, []string{"not saved", "persistence-failure"}},
		{"partial", pi.ModelRoutingApplyResult{Outcome: pi.ModelRoutingApplyOutcomePartial, Saved: true, Materialization: &pi.ModelRoutingApplyMaterialization{Succeeded: []string{"global\t"}, Failed: []pi.ModelRoutingApplyFailure{{Target: "project\n", Message: "write\x1bfailed"}}}}, []string{"partially materialized", "Saved: true", "Succeeded targets", "Failed targets", "writefailed"}},
		{"unknown", pi.ModelRoutingApplyResult{}, []string{"Outcome unknown", "Inspect Pi configuration before retrying"}},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			state := NewPiModelInspectionState()
			state.SetApplyResult(tt.result, errors.New("provider\x1bdetail"))
			out := RenderPiModelInspection(state, 24)
			for _, want := range tt.want {
				if !strings.Contains(out, want) {
					t.Fatalf("render = %q, missing %q", out, want)
				}
			}
			if strings.ContainsAny(out, "\x00\x1b\t\r") {
				t.Fatalf("unsafe render = %q", out)
			}
		})
	}
}
