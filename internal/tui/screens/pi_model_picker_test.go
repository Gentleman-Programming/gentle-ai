package screens_test

import (
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/model"
	"github.com/gentleman-programming/gentle-ai/v2/internal/tui/screens"
)

func TestPiModelPickerOptionCount(t *testing.T) {
	count := screens.PiModelPickerOptionCount()
	expected := len(model.PiSubscriptionsOrder()) + 1 // presets + Back
	if count != expected {
		t.Fatalf("expected option count %d, got %d", expected, count)
	}
}

func TestRenderPiModelPicker(t *testing.T) {
	state := screens.NewPiModelPickerState(model.PiSubscriptionClaude)
	rendered := screens.RenderPiModelPicker(state, 0)

	if !strings.Contains(rendered, "Configure Pi Agent Models") {
		t.Errorf("expected title in rendered output")
	}
	if !strings.Contains(rendered, "Claude Subscription") {
		t.Errorf("expected Claude Subscription in output")
	}
	if !strings.Contains(rendered, "Codex Subscription") {
		t.Errorf("expected Codex Subscription in output")
	}
	if !strings.Contains(rendered, "Back") {
		t.Errorf("expected Back in output")
	}
}

func TestHandlePiModelPickerNav(t *testing.T) {
	state := screens.NewPiModelPickerState(model.PiSubscriptionClaude)

	// Enter on cursor 1 (Codex)
	handled, selected := screens.HandlePiModelPickerNav("enter", &state, 1)
	if !handled {
		t.Fatalf("expected handled to be true")
	}
	if selected != model.PiSubscriptionCodex {
		t.Errorf("expected %s, got %s", model.PiSubscriptionCodex, selected)
	}
	if state.ActivePreset != model.PiSubscriptionCodex {
		t.Errorf("expected active preset %s, got %s", model.PiSubscriptionCodex, state.ActivePreset)
	}

	// Enter on Back row (cursor 4)
	handled, selected = screens.HandlePiModelPickerNav("enter", &state, 4)
	if handled || selected != "" {
		t.Errorf("expected Back row not to return handled preset")
	}
}
