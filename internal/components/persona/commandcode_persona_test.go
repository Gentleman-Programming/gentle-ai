package persona

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v4/internal/agents/cursor"
	"github.com/gentleman-programming/gentle-ai/v4/internal/model"
)

type commandCodeTestAdapter struct{ *cursor.Adapter }

func (commandCodeTestAdapter) Agent() model.AgentID { return model.AgentID("command-code") }
func (commandCodeTestAdapter) SystemPromptFile(home string) string {
	return filepath.Join(home, ".commandcode", "AGENTS.md")
}
func (commandCodeTestAdapter) SystemPromptStrategy() model.SystemPromptStrategy {
	return model.StrategyMarkdownSections
}

func TestCommandCodePersonaInjection(t *testing.T) {
	home := t.TempDir()
	adapter := commandCodeTestAdapter{cursor.NewAdapter()}
	promptPath := adapter.SystemPromptFile(home)

	if err := os.MkdirAll(filepath.Dir(promptPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(promptPath, []byte("# Guidelines\n\nPreserve this.\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	first, err := Inject(home, adapter, model.PersonaGentleman)
	if err != nil || !first.Changed {
		t.Fatalf("first Inject() = (%v, %v), want (true, nil)", first.Changed, err)
	}

	data, err := os.ReadFile(promptPath)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	if !strings.Contains(text, "<!-- gentle-ai:persona -->") ||
		!strings.Contains(text, "<!-- /gentle-ai:persona -->") {
		t.Fatalf("AGENTS.md missing persona markers: %s", text)
	}
	if !strings.Contains(text, "# Guidelines") {
		t.Fatalf("AGENTS.md must preserve user content: %s", text)
	}

	second, err := Inject(home, adapter, model.PersonaGentleman)
	if err != nil || second.Changed {
		t.Fatalf("second Inject() = (%v, %v), want (false, nil)", second.Changed, err)
	}
}
