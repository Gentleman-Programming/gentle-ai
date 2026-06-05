package screens

import (
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/internal/model"
)

func TestRenderKiroModelPicker_ShowsRequestedCopy(t *testing.T) {
	state := NewKiroModelPickerState()
	out := RenderKiroModelPicker(state, 0)

	if !strings.Contains(out, "Kiro Model Assignments") {
		t.Fatalf("expected title 'Kiro Model Assignments' in output, got:\n%s", out)
	}
	if !strings.Contains(out, "Choose how Kiro models are assigned to each SDD execution phase") {
		t.Fatalf("expected Kiro subtitle in output, got:\n%s", out)
	}
	for _, want := range []string{"balanced", "performance", "economy", "open-weight", "custom"} {
		if !strings.Contains(out, want) {
			t.Fatalf("expected preset %q in output, got:\n%s", want, out)
		}
	}
	if !strings.Contains(out, "Kiro Auto") {
		t.Fatalf("expected Kiro-native Auto copy in output, got:\n%s", out)
	}
}

func TestHandleKiroModelPickerNav_SelectsKiroNativePreset(t *testing.T) {
	state := NewKiroModelPickerState()

	handled, assignments := HandleKiroModelPickerNav("enter", &state, 0)

	if !handled {
		t.Fatal("expected enter on preset to be handled")
	}
	if assignments == nil {
		t.Fatal("expected preset selection to return assignments")
	}
	if got := assignments["default"]; got != model.KiroModelAuto {
		t.Fatalf("default assignment = %q, want %q", got, model.KiroModelAuto)
	}
	if got := assignments["sdd-design"]; got != model.KiroModelOpus {
		t.Fatalf("sdd-design assignment = %q, want %q", got, model.KiroModelOpus)
	}
}

func TestHandleKiroModelPickerNav_CustomCyclesAcrossKiroOptions(t *testing.T) {
	state := NewKiroModelPickerState()

	handled, assignments := HandleKiroModelPickerNav("enter", &state, 4)
	if !handled || assignments != nil || !state.InCustomMode {
		t.Fatalf("expected custom preset to enter custom mode, handled=%v assignments=%v inCustom=%v", handled, assignments, state.InCustomMode)
	}

	handled, assignments = HandleKiroModelPickerNav("enter", &state, 0)
	if !handled || assignments != nil {
		t.Fatalf("expected phase cycle to be handled without confirming, handled=%v assignments=%v", handled, assignments)
	}
	if got := state.CustomAssignments["sdd-explore"]; got != model.KiroModelOpus {
		t.Fatalf("first cycle from auto should become opus, got %q", got)
	}

	for _, want := range []model.KiroModelAlias{
		model.KiroModelSonnet,
		model.KiroModelHaiku,
		model.KiroModelMiniMax,
		model.KiroModelGLM,
		model.KiroModelDeepSeek,
		model.KiroModelQwen,
		model.KiroModelAuto,
	} {
		handled, _ = HandleKiroModelPickerNav("enter", &state, 0)
		if !handled {
			t.Fatal("expected cycle to be handled")
		}
		if got := state.CustomAssignments["sdd-explore"]; got != want {
			t.Fatalf("cycled assignment = %q, want %q", got, want)
		}
	}
}

func TestNewKiroModelPickerStateFromAssignments_PreservesLegacyAliases(t *testing.T) {
	state := NewKiroModelPickerStateFromAssignments(map[string]model.KiroModelAlias{
		"sdd-apply": "sonnet",
		"default":   "haiku",
	})

	if state.Preset != KiroPresetCustom {
		t.Fatalf("Preset = %q, want custom", state.Preset)
	}
	if got := state.CustomAssignments["sdd-apply"]; got != model.KiroModelSonnet {
		t.Fatalf("legacy sonnet assignment = %q, want %q", got, model.KiroModelSonnet)
	}
	if got := state.CustomAssignments["default"]; got != model.KiroModelHaiku {
		t.Fatalf("legacy haiku assignment = %q, want %q", got, model.KiroModelHaiku)
	}
}
