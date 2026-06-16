package screens

import (
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/internal/model"
)

func TestRenderKiloModelPicker_ShowsRequestedCopy(t *testing.T) {
	state := NewKiloModelPickerState()
	out := RenderKiloModelPicker(state, 0)

	if !strings.Contains(out, "Kilo Model Assignments") {
		t.Fatalf("expected title 'Kilo Model Assignments' in output, got:\n%s", out)
	}
	if !strings.Contains(out, "Choose how Kilo models are assigned to each SDD execution phase") {
		t.Fatalf("expected Kilo subtitle in output, got:\n%s", out)
	}
	for _, want := range []string{"balanced", "custom"} {
		if !strings.Contains(out, want) {
			t.Fatalf("expected preset %q in output, got:\n%s", want, out)
		}
	}
}

func TestHandleKiloModelPickerNav_SelectsBalancedPreset(t *testing.T) {
	state := NewKiloModelPickerState()

	handled, assignments := HandleKiloModelPickerNav("enter", &state, 0)

	if !handled {
		t.Fatal("expected enter on preset to be handled")
	}
	if assignments == nil {
		t.Fatal("expected preset selection to return assignments")
	}
	if got := assignments["default"]; got != model.KiloModelAuto {
		t.Fatalf("default assignment = %q, want %q", got, model.KiloModelAuto)
	}
}

func TestHandleKiloModelPickerNav_CustomCyclesAcrossKiloOptions(t *testing.T) {
	state := NewKiloModelPickerState()

	handled, assignments := HandleKiloModelPickerNav("enter", &state, 1)
	if !handled || assignments != nil || !state.InCustomMode {
		t.Fatalf("expected custom preset to enter custom mode, handled=%v assignments=%v inCustom=%v", handled, assignments, state.InCustomMode)
	}

	handled, assignments = HandleKiloModelPickerNav("enter", &state, 0)
	if !handled || assignments != nil {
		t.Fatalf("expected phase cycle to be handled without confirming, handled=%v assignments=%v", handled, assignments)
	}
	if got := state.CustomAssignments["sdd-explore"]; got != model.KiloModelSonnet {
		t.Fatalf("first cycle from auto should become sonnet, got %q", got)
	}

	for _, want := range []model.KiloModelAlias{
		model.KiloModelOpus,
		model.KiloModelHaiku,
		model.KiloModelGateway,
		model.KiloModelAuto,
	} {
		handled, _ = HandleKiloModelPickerNav("enter", &state, 0)
		if !handled {
			t.Fatal("expected cycle to be handled")
		}
		if got := state.CustomAssignments["sdd-explore"]; got != want {
			t.Fatalf("cycled assignment = %q, want %q", got, want)
		}
	}
}

func TestNewKiloModelPickerStateFromAssignments_Presets(t *testing.T) {
	state := NewKiloModelPickerStateFromAssignments(model.KiloModelPresetBalanced())

	if state.Preset != KiloPresetBalanced {
		t.Fatalf("Preset = %q, want balanced", state.Preset)
	}
}

func TestNewKiloModelPickerStateFromAssignments_Custom(t *testing.T) {
	state := NewKiloModelPickerStateFromAssignments(map[string]model.KiloModelAlias{
		"sdd-apply": model.KiloModelSonnet,
		"default":   model.KiloModelHaiku,
	})

	if state.Preset != KiloPresetCustom {
		t.Fatalf("Preset = %q, want custom", state.Preset)
	}
	if got := state.CustomAssignments["sdd-apply"]; got != model.KiloModelSonnet {
		t.Fatalf("custom sdd-apply assignment = %q, want %q", got, model.KiloModelSonnet)
	}
	if got := state.CustomAssignments["default"]; got != model.KiloModelHaiku {
		t.Fatalf("custom default assignment = %q, want %q", got, model.KiloModelHaiku)
	}
}

func TestKiloModelPickerOptionCount_PresetMode(t *testing.T) {
	state := NewKiloModelPickerState()
	count := KiloModelPickerOptionCount(state)
	if count != len(kiloPresetOrder)+1 {
		t.Fatalf("KiloModelPickerOptionCount = %d, want %d", count, len(kiloPresetOrder)+1)
	}
}

func TestKiloModelPickerOptionCount_CustomMode(t *testing.T) {
	state := NewKiloModelPickerState()
	state.InCustomMode = true
	count := KiloModelPickerOptionCount(state)
	if count != len(claudePhases)+2 {
		t.Fatalf("KiloModelPickerOptionCount (custom) = %d, want %d", count, len(claudePhases)+2)
	}
}
