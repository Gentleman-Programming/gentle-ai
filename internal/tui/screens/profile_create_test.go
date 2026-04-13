package screens_test

import (
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/internal/model"
	"github.com/gentleman-programming/gentle-ai/internal/tui/screens"
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
	}
	picker := screens.ModelPickerState{}
	output := screens.RenderProfileCreate(2, draft, "", 0, "", false, nil, picker, 0)

	if !strings.Contains(output, "anthropic") {
		t.Errorf("expected orchestrator provider 'anthropic' in confirm screen, got:\n%s", output)
	}
	if !strings.Contains(output, "claude-haiku-4") {
		t.Errorf("expected orchestrator model 'claude-haiku-4' in confirm screen, got:\n%s", output)
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
