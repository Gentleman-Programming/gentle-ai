package screens

import (
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/model"
)

func TestRenderPersonaClarifiesCustomKeepsExistingPersona(t *testing.T) {
	out := RenderPersona(model.PersonaCustom, 2)

	if !strings.Contains(out, "custom") {
		t.Fatalf("RenderPersona missing custom option; output:\n%s", out)
	}
	if !strings.Contains(out, "Do not install a managed persona; choose themes/logo on the next screens") {
		t.Fatalf("RenderPersona missing custom persona clarification; output:\n%s", out)
	}
	if strings.Contains(out, "Bring your own persona instructions") {
		t.Fatalf("RenderPersona still shows old custom persona wording; output:\n%s", out)
	}
}

func TestRenderPresetClarifiesCustomManualSelection(t *testing.T) {
	out := RenderPreset(model.PresetCustom, 3)

	if !strings.Contains(out, "Choose each component manually") {
		t.Fatalf("RenderPreset missing custom preset clarification; output:\n%s", out)
	}
	if strings.Contains(out, "Pick individual components yourself") {
		t.Fatalf("RenderPreset still shows old custom preset wording; output:\n%s", out)
	}
}
