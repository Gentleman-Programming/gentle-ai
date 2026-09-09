package screens_test

import (
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/model"
	"github.com/gentleman-programming/gentle-ai/v2/internal/tui/screens"
)

func TestPiModelPickerOptionCount(t *testing.T) {
	count := screens.PiModelPickerOptionCount()
	expected := len(model.PiSubscriptionsOrder()) + 1 // 10 presets + Back
	if count != expected {
		t.Fatalf("expected option count %d, got %d", expected, count)
	}
}

func TestRenderPiModelPicker(t *testing.T) {
	state := screens.NewPiModelPickerState(model.PiPresetClaudeBalanced)
	rendered := screens.RenderPiModelPicker(state, 0)

	if !strings.Contains(rendered, "Configure Pi Agent Models") {
		t.Errorf("expected title in rendered output")
	}
	if !strings.Contains(rendered, "Anthropic via API key — Balanced") {
		t.Errorf("expected Anthropic via API key Balanced in output")
	}
	if !strings.Contains(rendered, "Anthropic via API key — Premium") {
		t.Errorf("expected Anthropic via API key Premium in output")
	}
	if !strings.Contains(rendered, "Anthropic via API key — Economy") {
		t.Errorf("expected Anthropic via API key Economy in output")
	}
	if !strings.Contains(rendered, "Codex — Balanced") {
		t.Errorf("expected Codex Balanced in output")
	}
	if !strings.Contains(rendered, "Configuration Preview") {
		t.Errorf("expected Configuration Preview box in output")
	}
	if !strings.Contains(rendered, "Back") {
		t.Errorf("expected Back in output")
	}
}

func TestHandlePiModelPickerNav(t *testing.T) {
	state := screens.NewPiModelPickerState(model.PiPresetClaudeBalanced)

	// Enter on cursor 3 (Codex Balanced)
	handled, selected := screens.HandlePiModelPickerNav("enter", &state, 3)
	if !handled {
		t.Fatalf("expected handled to be true")
	}
	if selected != model.PiPresetCodexBalanced {
		t.Errorf("expected %s, got %s", model.PiPresetCodexBalanced, selected)
	}
	if state.ActivePreset != model.PiPresetCodexBalanced {
		t.Errorf("expected active preset %s, got %s", model.PiPresetCodexBalanced, state.ActivePreset)
	}

	// Enter on Back row (cursor 10)
	handled, selected = screens.HandlePiModelPickerNav("enter", &state, 10)
	if handled || selected != "" {
		t.Errorf("expected Back row not to return handled preset")
	}
}
