package persona

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/model"
)

func TestInjectGentlemanNeutralArtifactsRoutesToNeutralContent(t *testing.T) {
	home := t.TempDir()

	result, err := Inject(home, opencodeAdapter(), model.PersonaGentlemanNeutralArtifacts)
	if err != nil {
		t.Fatalf("Inject() error = %v", err)
	}
	if !result.Changed {
		t.Fatalf("Inject() changed = false")
	}

	content, err := os.ReadFile(filepath.Join(home, ".config", "opencode", "AGENTS.md"))
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	text := string(content)

	// The alias routes to neutral content, not gentleman
	if strings.Contains(text, "Rioplatense") {
		t.Fatalf("alias should route to neutral — found gentleman tone marker 'Rioplatense'")
	}

	// Verify neutral content is present
	for _, want := range []string{
		"Generated technical artifacts default to English",
		"Public/contextual comments follow the target context language",
		"If the selected reply language is English, every part of the direct reply must be English",
		"Prompts starting with or dominated by hi, hello, hey, or similar English greetings are English prompts",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("installed persona missing %q; content:\n%s", want, text)
		}
	}
}

func TestInjectPersonaAxiomRoutesToAxiomContent(t *testing.T) {
	home := t.TempDir()

	result, err := Inject(home, opencodeAdapter(), model.PersonaAxiom)
	if err != nil {
		t.Fatalf("Inject() error = %v", err)
	}
	if !result.Changed {
		t.Fatalf("Inject() changed = false")
	}

	content, err := os.ReadFile(filepath.Join(home, ".config", "opencode", "AGENTS.md"))
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	text := string(content)

	// Prohibiciones explícitas: no voseo ni regionalismos rioplatenses
	for _, forbidden := range []string{"Rioplatense", "voseo", "tenés", "podés", "hacé"} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("PersonaAxiom contains forbidden term %q; content:\n%s", forbidden, text)
		}
	}

	// Verificación de directivas de Axiom
	for _, want := range []string{
		"Castellano de España (peninsular)",
		"all technical artifacts",
		"Spanish (castellano peninsular)",
		"English is strictly preserved for code syntax and source identifiers",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("PersonaAxiom missing expected directive %q; content:\n%s", want, text)
		}
	}
}
