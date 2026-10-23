package screens

import (
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v4/internal/model"
)

func TestRenderKiroModelPicker_ShowsRequestedCopy(t *testing.T) {
	state := NewKiroModelPickerState()
	out := RenderKiroModelPicker(state, 0)

	if !strings.Contains(out, "Kiro Model Assignments") {
		t.Fatalf("expected title 'Kiro Model Assignments' in output, got:\n%s", out)
	}
	if !strings.Contains(out, "Choose how Kiro models are assigned to ODD and review roles") {
		t.Fatalf("expected Kiro subtitle in output, got:\n%s", out)
	}
	for _, want := range []string{"balanced", "performance", "economy", "open-weight", "custom"} {
		if !strings.Contains(out, want) {
			t.Fatalf("expected preset %q in output, got:\n%s", want, out)
		}
	}
	for _, want := range []string{"Sonnet 5.5 and GPT-5.6 Luna", "Opus 5.5", "GPT-5.6 Luna for most roles"} {
		if !strings.Contains(out, want) {
			t.Fatalf("expected preset description %q in output, got:\n%s", want, out)
		}
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
	for key := range assignments {
		if strings.HasPrefix(key, "sdd-") {
			t.Fatalf("preset exposes retired role %q", key)
		}
	}
	if got := assignments["odd-explorer"]; got != model.KiroModelLuna {
		t.Fatalf("odd-explorer assignment = %q, want luna", got)
	}
	if got := assignments["jd-judge-a"]; got != model.KiroModelSonnet {
		t.Fatalf("jd-judge-a assignment = %q, want sonnet", got)
	}
}

func TestKiroCustomRowsContainOnlyActiveRoles(t *testing.T) {
	state := NewKiroModelPickerState()
	HandleKiroModelPickerNav("enter", &state, 4)
	out := RenderKiroModelPicker(state, 0)
	for _, want := range []string{"ODD Explorer", "ODD Worker", "RDD Risk", "RDD Validator"} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing active role %q", want)
		}
	}
	if strings.Contains(strings.ToLower(out), "sdd") {
		t.Fatal("retired SDD role in Kiro custom picker")
	}
}

func TestHandleKiroModelPickerNav_CustomCyclesAcrossKiroOptions(t *testing.T) {
	state := NewKiroModelPickerState()

	handled, assignments := HandleKiroModelPickerNav("enter", &state, 4)
	if !handled || assignments != nil || !state.InCustomMode {
		t.Fatalf("expected custom preset to enter custom mode, handled=%v assignments=%v inCustom=%v", handled, assignments, state.InCustomMode)
	}
	state.CustomAssignments["odd-explorer"] = model.KiroModelAuto

	handled, assignments = HandleKiroModelPickerNav("enter", &state, 0)
	if !handled || assignments != nil {
		t.Fatalf("expected phase cycle to be handled without confirming, handled=%v assignments=%v", handled, assignments)
	}
	if got := state.CustomAssignments["odd-explorer"]; got != model.KiroModelOpus {
		t.Fatalf("first cycle from auto should become opus, got %q", got)
	}

	for _, want := range []model.KiroModelAlias{
		model.KiroModelSonnet,
		model.KiroModelHaiku,
		model.KiroModelLuna,
		model.KiroModelTerra,
		model.KiroModelSol,
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
		if got := state.CustomAssignments["odd-explorer"]; got != want {
			t.Fatalf("cycled assignment = %q, want %q", got, want)
		}
	}
}

func TestRenderKiroCustomPicker_ShowsKiroModelIDs(t *testing.T) {
	state := NewKiroModelPickerState()
	HandleKiroModelPickerNav("enter", &state, 4)
	state.CustomAssignments["odd-explorer"] = model.KiroModelTerra
	out := RenderKiroModelPicker(state, 0)

	if !strings.Contains(out, "auto → opus → sonnet → haiku → luna → terra → sol → minimax → glm → deepseek → qwen") {
		t.Fatalf("expected full Kiro cycle in help text, got:\n%s", out)
	}
	for _, want := range []string{"[gpt-5.6-luna]", "[claude-sonnet-5.5]", "[gpt-5.6-terra]"} {
		if !strings.Contains(out, want) {
			t.Fatalf("expected model tag %q in output, got:\n%s", want, out)
		}
	}
}

func TestKiroNamedPresetPreservesPersistedLegacyKeys(t *testing.T) {
	state := NewKiroModelPickerStateFromAssignments(map[string]model.KiroModelAlias{
		"sdd-design": model.KiroModelGLM,
		"default":    model.KiroModelHaiku,
	})
	_, saved := HandleKiroModelPickerNav("enter", &state, 1)
	if saved["sdd-design"] != model.KiroModelGLM {
		t.Fatalf("legacy assignment lost when selecting preset: %v", saved)
	}
	if saved["odd-explorer"] != model.KiroModelSonnet {
		t.Fatalf("performance preset lost: %v", saved)
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
	HandleKiroModelPickerNav("enter", &state, 4)
	_, saved := HandleKiroModelPickerNav("enter", &state, KiroModelPickerOptionCount(state)-2)
	if got := saved["sdd-apply"]; got != model.KiroModelSonnet {
		t.Fatalf("saved legacy alias = %q, want sonnet", got)
	}
}
