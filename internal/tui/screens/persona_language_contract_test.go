package screens

import (
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/internal/model"
)

func TestPersonaOptionsIncludeGentlemanNeutralArtifacts(t *testing.T) {
	options := PersonaOptions()
	found := false
	for _, option := range options {
		if option == model.PersonaGentlemanNeutralArtifacts {
			found = true
		}
	}
	if !found {
		t.Fatalf("PersonaOptions() = %v, missing %q", options, model.PersonaGentlemanNeutralArtifacts)
	}
}

func TestRenderPersonaDescribesGentlemanNeutralArtifacts(t *testing.T) {
	out := RenderPersona(model.PersonaGentlemanNeutralArtifacts, 2)
	for _, want := range []string{
		"gentleman-neutral-artifacts",
		"Gentleman conversation",
		"English technical artifacts",
		"context language",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("RenderPersona() missing %q; output:\n%s", want, out)
		}
	}
}
