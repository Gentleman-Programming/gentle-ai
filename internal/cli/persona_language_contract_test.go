package cli

import (
	"testing"

	"github.com/gentleman-programming/gentle-ai/internal/model"
)

func TestNormalizePersonaAcceptsGentlemanNeutralArtifacts(t *testing.T) {
	got, err := normalizePersona("gentleman-neutral-artifacts")
	if err != nil {
		t.Fatalf("normalizePersona() error = %v", err)
	}
	if got != model.PersonaGentlemanNeutralArtifacts {
		t.Fatalf("normalizePersona() = %q, want %q", got, model.PersonaGentlemanNeutralArtifacts)
	}
}
