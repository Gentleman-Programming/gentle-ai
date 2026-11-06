package screens

import (
	"reflect"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v4/internal/model"
)

func TestRenderAntigravityModelPickerShowsTierPresets(t *testing.T) {
	out := RenderAntigravityModelPicker(NewAntigravityModelPickerState(), 0)
	for _, want := range []string{"Antigravity Model Assignments", "Choose the agy model tier for Antigravity subagents:", "inherit", "performance", "economy", "custom"} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q in:\n%s", want, out)
		}
	}
}

func TestHandleAntigravityModelPickerNavPresetDefaultsToInherit(t *testing.T) {
	state := NewAntigravityModelPickerState()
	handled, assignments := HandleAntigravityModelPickerNav("enter", &state, 0)
	if !handled || !reflect.DeepEqual(assignments, model.AntigravityModelPresetInherit()) {
		t.Fatalf("inherit preset = %v (handled %t)", assignments, handled)
	}
	state = NewAntigravityModelPickerState()
	if _, assignments = HandleAntigravityModelPickerNav("enter", &state, 1); assignments["jd-judge-a"] != model.AntigravityModelPro {
		t.Fatalf("performance preset = %v", assignments)
	}
}

func TestHandleAntigravityModelPickerNavCustomCyclesTiers(t *testing.T) {
	state := NewAntigravityModelPickerState()
	customCursor := AntigravityModelPickerOptionCount(state) - 2
	if handled, assignments := HandleAntigravityModelPickerNav("enter", &state, customCursor); !handled || assignments != nil || !state.InCustomMode {
		t.Fatalf("custom preset must open role rows without confirming: %+v", state)
	}
	out := RenderAntigravityModelPicker(state, 0)
	for _, want := range []string{"JD Judge A", "JD Judge B", "JD Fix Agent"} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing role %q in:\n%s", want, out)
		}
	}
	if strings.Contains(out, "ODD") || strings.Contains(out, "RDD") {
		t.Fatalf("Antigravity custom picker lists roles without native agents:\n%s", out)
	}
	for _, want := range []model.AntigravityModelTier{model.AntigravityModelFlashLite, model.AntigravityModelFlash, model.AntigravityModelPro, model.AntigravityModelInherit} {
		HandleAntigravityModelPickerNav("enter", &state, 0)
		if got := state.CustomAssignments["jd-judge-a"]; got != want {
			t.Fatalf("cycled tier = %q, want %q", got, want)
		}
	}
	HandleAntigravityModelPickerNav("enter", &state, 0)
	_, assignments := HandleAntigravityModelPickerNav("enter", &state, len(model.AntigravityModelRoles))
	if assignments["jd-judge-a"] != model.AntigravityModelFlashLite || assignments["jd-fix-agent"] != model.AntigravityModelInherit {
		t.Fatalf("confirmed custom assignments = %v", assignments)
	}
}

func TestNewAntigravityModelPickerStateFromAssignments(t *testing.T) {
	if got := NewAntigravityModelPickerStateFromAssignments(model.AntigravityModelPresetEconomy()); got.Preset != AntigravityPresetEconomy {
		t.Fatalf("economy assignments preset = %q", got.Preset)
	}
	custom := map[string]model.AntigravityModelTier{"jd-judge-a": model.AntigravityModelPro}
	if got := NewAntigravityModelPickerStateFromAssignments(custom); got.Preset != AntigravityPresetCustom {
		t.Fatalf("custom assignments preset = %q", got.Preset)
	}
}
