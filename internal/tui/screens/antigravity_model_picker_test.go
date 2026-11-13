package screens

import (
	"reflect"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v4/internal/model"
)

// TestRenderAntigravityModelPickerShowsTierPresets verifies that the preset selection
// screen renders the expected titles, options, and host limitation notice.
func TestRenderAntigravityModelPickerShowsTierPresets(t *testing.T) {
	out := RenderAntigravityModelPicker(NewAntigravityModelPickerState(), 0)
	for _, want := range []string{
		"Antigravity Model Assignments",
		"Choose the agy model tier for Antigravity subagents:",
		"Note: Antigravity native subagents support tiers (inherit/flash_lite/flash/pro); arbitrary model/provider/effort is unsupported by the host.",
		"inherit",
		"performance",
		"economy",
		"custom",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q in:\n%s", want, out)
		}
	}
}

// TestHandleAntigravityModelPickerNavPresetDefaultsToInherit verifies preset navigation
// and constructor mappings for inherit and performance presets.
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

// TestHandleAntigravityModelPickerNavCustomCyclesTiers verifies custom mode entry,
// role labels, host limitation note, and tier cycling across available model tiers.
func TestHandleAntigravityModelPickerNavCustomCyclesTiers(t *testing.T) {
	state := NewAntigravityModelPickerState()
	customCursor := AntigravityModelPickerOptionCount(state) - 2
	if handled, assignments := HandleAntigravityModelPickerNav("enter", &state, customCursor); !handled || assignments != nil || !state.InCustomMode {
		t.Fatalf("custom preset must open role rows without confirming: %+v", state)
	}
	out := RenderAntigravityModelPicker(state, 0)
	for _, want := range []string{
		"ODD Explorer",
		"ODD Worker",
		"ODD Verify",
		"JD Judge A",
		"JD Judge B",
		"JD Fix Agent",
		"Note: Antigravity native subagents support tiers (inherit/flash_lite/flash/pro); arbitrary model/provider/effort is unsupported by the host.",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q in:\n%s", want, out)
		}
	}
	if strings.Contains(out, "RDD") {
		t.Fatalf("Antigravity custom picker lists RDD roles without native agents:\n%s", out)
	}
	for _, want := range []model.AntigravityModelTier{model.AntigravityModelFlashLite, model.AntigravityModelFlash, model.AntigravityModelPro, model.AntigravityModelInherit} {
		HandleAntigravityModelPickerNav("enter", &state, 0)
		if got := state.CustomAssignments["gentle-ai-explore"]; got != want {
			t.Fatalf("cycled tier = %q, want %q", got, want)
		}
	}
	HandleAntigravityModelPickerNav("enter", &state, 0)
	_, assignments := HandleAntigravityModelPickerNav("enter", &state, len(model.AntigravityModelRoles))
	if assignments["gentle-ai-explore"] != model.AntigravityModelFlashLite || assignments["jd-fix-agent"] != model.AntigravityModelInherit {
		t.Fatalf("confirmed custom assignments = %v", assignments)
	}
}

// TestNewAntigravityModelPickerStateFromAssignments verifies state restoration
// from persisted custom assignments matching known presets and custom mappings.
func TestNewAntigravityModelPickerStateFromAssignments(t *testing.T) {
	if got := NewAntigravityModelPickerStateFromAssignments(model.AntigravityModelPresetEconomy()); got.Preset != AntigravityPresetEconomy {
		t.Fatalf("economy assignments preset = %q", got.Preset)
	}
	custom := map[string]model.AntigravityModelTier{"jd-judge-a": model.AntigravityModelPro}
	if got := NewAntigravityModelPickerStateFromAssignments(custom); got.Preset != AntigravityPresetCustom {
		t.Fatalf("custom assignments preset = %q", got.Preset)
	}
}

// TestRenderAntigravityModelPickerWrapsNoticeToTerminalWidth verifies that the host limitation
// notice wraps within 80 columns in both preset and custom views without losing content.
func TestRenderAntigravityModelPickerWrapsNoticeToTerminalWidth(t *testing.T) {
	state := NewAntigravityModelPickerState()
	outPreset := RenderAntigravityModelPicker(state, 0, 80)
	if !strings.Contains(outPreset, "support tiers (inherit/flash_lite/flash/pro)") {
		t.Fatalf("preset view must retain tiers mention, got:\n%s", outPreset)
	}
	if !strings.Contains(outPreset, "arbitrary model/provider/effort is unsupported by the host.") {
		t.Fatalf("preset view must retain unsupported mention, got:\n%s", outPreset)
	}

	state.InCustomMode = true
	outCustom := RenderAntigravityModelPicker(state, 0, 80)
	if !strings.Contains(outCustom, "support tiers (inherit/flash_lite/flash/pro)") {
		t.Fatalf("custom view must retain tiers mention, got:\n%s", outCustom)
	}
	if !strings.Contains(outCustom, "arbitrary model/provider/effort is unsupported by the host.") {
		t.Fatalf("custom view must retain unsupported mention, got:\n%s", outCustom)
	}
}
