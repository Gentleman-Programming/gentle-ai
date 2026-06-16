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
	for _, want := range []string{"balanced", "quality", "custom"} {
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

func TestHandleKiloModelPickerNav_CustomCyclesThroughAliases(t *testing.T) {
	state := NewKiloModelPickerState()

	// Enter custom mode
	handled, assignments := HandleKiloModelPickerNav("enter", &state, 2)
	if !handled || assignments != nil || !state.InCustomMode {
		t.Fatalf("expected custom preset to enter custom mode, handled=%v assignments=%v inCustom=%v", handled, assignments, state.InCustomMode)
	}

	// First cycle: auto → next alias (gateway, since KnownKiloAliases[0]=auto, [1]=gateway)
	handled, assignments = HandleKiloModelPickerNav("enter", &state, 1)
	if !handled || assignments != nil {
		t.Fatalf("expected phase cycle to be handled without confirming, handled=%v assignments=%v", handled, assignments)
	}
	firstCycled := state.CustomAssignments["sdd-explore"]
	if firstCycled == model.KiloModelAuto {
		t.Fatalf("first cycle from auto should change, got %q", firstCycled)
	}

	// Cycle through remaining aliases and verify we return to auto after N total steps.
	// The first cycle already moved from auto→gateway, so we need N-1 more cycles.
	seen := map[model.KiloModelAlias]bool{model.KiloModelAuto: true}
	seen[firstCycled] = true
	current := firstCycled
	for i := 0; i < len(model.KnownKiloAliases)-1; i++ {
		handled, _ = HandleKiloModelPickerNav("enter", &state, 1)
		if !handled {
			t.Fatal("expected cycle to be handled")
		}
		next := state.CustomAssignments["sdd-explore"]
		if next == current {
			t.Fatalf("cycle stuck at %q after %d iterations", next, i)
		}
		current = next
		seen[current] = true
	}

	// After cycling through all known aliases, we should be back to auto
	if current != model.KiloModelAuto {
		t.Fatalf("expected to return to auto after full cycle, got %q", current)
	}

	// Verify we saw at least the first few distinct aliases
	for _, expected := range []model.KiloModelAlias{
		model.KiloModelGateway,
		model.KiloModelOpus,
		model.KiloModelSonnet,
		model.KiloModelHaiku,
	} {
		if !seen[expected] {
			t.Fatalf("expected to see alias %q during cycle", expected)
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
	if count != len(sddPhases)+2 {
		t.Fatalf("KiloModelPickerOptionCount (custom) = %d, want %d", count, len(sddPhases)+2)
	}
}
