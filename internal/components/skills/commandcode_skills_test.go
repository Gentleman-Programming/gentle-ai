package skills

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v4/internal/agents/cursor"
	"github.com/gentleman-programming/gentle-ai/v4/internal/model"
)

type commandCodeTestAdapter struct{ *cursor.Adapter }

func (commandCodeTestAdapter) Agent() model.AgentID { return model.AgentID("command-code") }
func (commandCodeTestAdapter) SkillsDir(home string) string {
	return filepath.Join(home, ".commandcode", "skills")
}

func TestCommandCodeSkillsInjection(t *testing.T) {
	home := t.TempDir()
	adapter := commandCodeTestAdapter{cursor.NewAdapter()}

	skillIDs := []model.SkillID{model.SkillGoTesting}
	first, err := Inject(home, adapter, skillIDs)
	if err != nil || !first.Changed {
		t.Fatalf("first Inject() = (%v, %v), want (true, nil)", first.Changed, err)
	}

	skillPath := filepath.Join(adapter.SkillsDir(home), "go-testing", "SKILL.md")
	info, err := os.Stat(skillPath)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if info.Size() == 0 {
		t.Fatalf("Stat(%q) size = 0", skillPath)
	}

	second, err := Inject(home, adapter, skillIDs)
	if err != nil || second.Changed {
		t.Fatalf("second Inject() = (%v, %v), want (false, nil)", second.Changed, err)
	}
}
