package screens_test

import (
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/model"
	"github.com/gentleman-programming/gentle-ai/v2/internal/opencode"
	"github.com/gentleman-programming/gentle-ai/v2/internal/tui/screens"
)

// ─── RenderProfileCreate step 0 (name input) ─────────────────────────────────

func TestRenderProfileCreate_Step0_ShowsNameInput(t *testing.T) {
	draft := model.Profile{}
	picker := screens.ModelPickerState{}
	output := screens.RenderProfileCreate(0, draft, "myprofile", 9, "", false, nil, picker, 0)

	if !strings.Contains(output, "myprofile") {
		t.Errorf("expected name input value 'myprofile' in output, got:\n%s", output)
	}
}

func TestRenderProfileCreate_Step0_ShowsValidationRules(t *testing.T) {
	draft := model.Profile{}
	picker := screens.ModelPickerState{}
	output := screens.RenderProfileCreate(0, draft, "", 0, "", false, nil, picker, 0)

	// Must mention lowercase or naming rules
	if !strings.Contains(output, "lowercase") && !strings.Contains(output, "slug") && !strings.Contains(output, "alphanumeric") {
		t.Errorf("expected validation rules in output (lowercase/slug/alphanumeric), got:\n%s", output)
	}
}

func TestRenderProfileCreate_Step0_ShowsValidationError(t *testing.T) {
	draft := model.Profile{}
	picker := screens.ModelPickerState{}
	output := screens.RenderProfileCreate(0, draft, "INVALID NAME", 12, "profile name must match", false, nil, picker, 0)

	if !strings.Contains(output, "profile name must match") {
		t.Errorf("expected validation error in output, got:\n%s", output)
	}
}

func TestRenderProfileCreate_Step0_Header(t *testing.T) {
	draft := model.Profile{}
	picker := screens.ModelPickerState{}
	output := screens.RenderProfileCreate(0, draft, "", 0, "", false, nil, picker, 0)

	if !strings.Contains(output, "Create SDD Profile") {
		t.Errorf("expected 'Create SDD Profile' header in output, got:\n%s", output)
	}
}

// ─── RenderProfileCreate step 2 (confirm) ────────────────────────────────────

func TestRenderProfileCreate_Step2_ShowsOrchestratorModel(t *testing.T) {
	draft := model.Profile{
		Name: "cheap",
		OrchestratorModel: model.ModelAssignment{
			ProviderID: "anthropic",
			ModelID:    "claude-haiku-4",
		},
		PhaseAssignments: map[string]model.ModelAssignment{
			"jd-judge-a": {ProviderID: "openai", ModelID: "gpt-5"},
		},
	}
	picker := screens.ModelPickerState{}
	output := screens.RenderProfileCreate(2, draft, "", 0, "", false, nil, picker, 0)

	for _, want := range []string{"anthropic", "claude-haiku-4", "Model assignments"} {
		if !strings.Contains(output, want) {
			t.Errorf("expected %q in confirm screen, got:\n%s", want, output)
		}
	}
	if strings.Contains(output, "Phase assignments") {
		t.Errorf("confirm step should not use SDD-only 'Phase assignments' copy; got:\n%s", output)
	}
}

func TestRenderProfileCreate_Step2_ShowsCreateAndSync(t *testing.T) {
	draft := model.Profile{Name: "cheap"}
	picker := screens.ModelPickerState{}
	output := screens.RenderProfileCreate(2, draft, "", 0, "", false, nil, picker, 0)

	if !strings.Contains(output, "Create & Sync") {
		t.Errorf("expected 'Create & Sync' button in confirm screen, got:\n%s", output)
	}
}

func TestRenderProfileCreate_Step1_ShowsJDRowsAssignmentAndClearHelp(t *testing.T) {
	draft := model.Profile{Name: "cheap"}
	picker := screens.ModelPickerState{
		ForProfile:   true,
		AvailableIDs: []string{"openai"},
		Providers: map[string]opencode.Provider{
			"openai": {
				Name: "OpenAI",
				Models: map[string]opencode.Model{
					"gpt-5": {Name: "GPT-5"},
				},
			},
		},
	}
	assignments := map[string]model.ModelAssignment{
		"jd-judge-a": {ProviderID: "openai", ModelID: "gpt-5"},
	}

	output := screens.RenderProfileCreate(1, draft, "", 0, "", true, assignments, picker, 0)

	for _, want := range []string{"--- Judgment Day ---", "jd-judge-a", "jd-judge-b", "jd-fix-agent", "OpenAI / GPT-5", "backspace: clear"} {
		if !strings.Contains(output, want) {
			t.Fatalf("profile model step missing %q; got:\n%s", want, output)
		}
	}
}

// ─── Edit mode ────────────────────────────────────────────────────────────────

func TestRenderProfileCreate_EditMode_ShowsEditHeader(t *testing.T) {
	draft := model.Profile{Name: "cheap"}
	picker := screens.ModelPickerState{}
	// step 0 in edit mode
	output := screens.RenderProfileCreate(0, draft, "cheap", 5, "", true, nil, picker, 0)

	if !strings.Contains(output, "Edit Profile") {
		t.Errorf("expected 'Edit Profile' header in edit mode, got:\n%s", output)
	}
}

func TestRenderProfileCreate_EditMode_Step2_ShowsSaveAndSync(t *testing.T) {
	draft := model.Profile{Name: "cheap"}
	picker := screens.ModelPickerState{}
	output := screens.RenderProfileCreate(2, draft, "", 0, "", true, nil, picker, 0)

	if !strings.Contains(output, "Save & Sync") {
		t.Errorf("expected 'Save & Sync' button in edit mode confirm screen, got:\n%s", output)
	}
}

// ─── ProfileCreateOptionCount ─────────────────────────────────────────────────

func TestProfileCreateOptionCount_Step0(t *testing.T) {
	picker := screens.ModelPickerState{}
	count := screens.ProfileCreateOptionCount(0, picker)

	// Step 0: text input — 0 navigation options (cursor not used for options)
	if count != 0 {
		t.Errorf("expected option count 0 for step 0 (text input), got %d", count)
	}
}

func TestProfileCreateOptionCount_Step2(t *testing.T) {
	picker := screens.ModelPickerState{}
	count := screens.ProfileCreateOptionCount(2, picker)

	// Step 2: "Create & Sync" + "Cancel" = 2
	if count != 2 {
		t.Errorf("expected option count 2 for step 2 (confirm), got %d", count)
	}
}

func TestProfileCreateOptionCount_Step1IncludesJDRows(t *testing.T) {
	picker := screens.ModelPickerState{AvailableIDs: []string{"anthropic"}}
	count := screens.ProfileCreateOptionCount(1, picker)

	want := 2 + len(opencode.SDDPhases()) + 1 + len(opencode.JDPhases()) + 2
	if count != want {
		t.Errorf("expected option count %d for step 1 with JD rows, got %d", want, count)
	}
}

func TestProfileCreateOptionCount_Step1EmptyProvidersIncludesContinueAndBack(t *testing.T) {
	picker := screens.ModelPickerState{}
	count := screens.ProfileCreateOptionCount(1, picker)

	if count != 2 {
		t.Errorf("expected option count 2 for empty-provider profile step (Continue with defaults + Back), got %d", count)
	}
}
