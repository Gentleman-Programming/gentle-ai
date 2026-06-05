package persona

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/internal/model"
)

func TestInjectGentlemanNeutralArtifactsUsesGentlemanConversationWithArtifactBoundary(t *testing.T) {
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
	for _, want := range []string{
		"Rioplatense",
		"Generated technical artifacts default to English",
		"Public/contextual comments follow the target context language",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("installed persona missing %q; content:\n%s", want, text)
		}
	}
}
