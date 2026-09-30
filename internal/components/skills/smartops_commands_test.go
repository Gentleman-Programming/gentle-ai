package skills

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v3/internal/assets"
	"github.com/gentleman-programming/gentle-ai/v3/internal/model"
)

// TestInjectSmartopsUICommandsWritesCommandForOpenCode is the RED/GREEN core
// assertion: with smartops-ui selected, exactly 1 OpenCode slash command file
// is written, with content matching the embedded asset bytes exactly.
func TestInjectSmartopsUICommandsWritesCommandForOpenCode(t *testing.T) {
	home := t.TempDir()

	result, err := InjectSmartopsUICommands(home, opencodeAdapter(), []model.SkillID{model.SkillSmartopsUI})
	if err != nil {
		t.Fatalf("InjectSmartopsUICommands() error = %v", err)
	}
	if !result.Changed {
		t.Fatal("InjectSmartopsUICommands() changed = false, want true")
	}
	if len(result.Files) != 1 {
		t.Fatalf("InjectSmartopsUICommands() files = %v, want exactly 1 entry", result.Files)
	}

	commandsDir := opencodeAdapter().CommandsDir(home)
	path := filepath.Join(commandsDir, "smartops-ui.md")
	if !containsFile(result.Files, path) {
		t.Fatalf("InjectSmartopsUICommands() files = %v, missing %q", result.Files, path)
	}
	got, readErr := os.ReadFile(path)
	if readErr != nil {
		t.Fatalf("ReadFile(%q) error = %v", path, readErr)
	}
	want, assetErr := assets.Read(smartopsCommandAssetDir + "/smartops-ui.md")
	if assetErr != nil {
		t.Fatalf("assets.Read() error = %v", assetErr)
	}
	if string(got) != want {
		t.Fatalf("%s content does not match embedded asset bytes exactly", path)
	}
}

// TestInjectSmartopsUICommandsSkipsClaudeCode confirms Claude Code gets zero
// smartops-ui command files regardless of selection: Claude Code already
// resolves a same-named skill directory as /<skill-name> natively.
func TestInjectSmartopsUICommandsSkipsClaudeCode(t *testing.T) {
	home := t.TempDir()

	result, err := InjectSmartopsUICommands(home, claudeAdapter(), []model.SkillID{model.SkillSmartopsUI})
	if err != nil {
		t.Fatalf("InjectSmartopsUICommands() error = %v", err)
	}
	if result.Changed {
		t.Fatalf("InjectSmartopsUICommands(claude) changed = true, want false")
	}
	if len(result.Files) != 0 {
		t.Fatalf("InjectSmartopsUICommands(claude) files = %v, want none", result.Files)
	}

	commandsDir := claudeAdapter().CommandsDir(home)
	path := filepath.Join(commandsDir, "smartops-ui.md")
	if _, statErr := os.Stat(path); statErr == nil {
		t.Fatalf("Claude Code must never receive a smartops-ui command file, found %q", path)
	}
}

// TestInjectSmartopsUICommandsSelectionGating mirrors the QA commands
// discipline: not selecting smartops-ui writes nothing.
func TestInjectSmartopsUICommandsSelectionGating(t *testing.T) {
	home := t.TempDir()

	result, err := InjectSmartopsUICommands(home, opencodeAdapter(), []model.SkillID{model.SkillQAEvidence})
	if err != nil {
		t.Fatalf("InjectSmartopsUICommands() error = %v", err)
	}
	if result.Changed {
		t.Fatalf("InjectSmartopsUICommands() changed = true, want false")
	}
	if len(result.Files) != 0 {
		t.Fatalf("InjectSmartopsUICommands() files = %v, want none", result.Files)
	}

	commandsDir := opencodeAdapter().CommandsDir(home)
	path := filepath.Join(commandsDir, "smartops-ui.md")
	if _, statErr := os.Stat(path); statErr == nil {
		t.Fatalf("unselected smartops-ui command %q was written", path)
	}
}

// TestSmartopsUICommandPathsMatchesInjectSmartopsUICommands ensures the
// path-listing helper used by install/uninstall backup and rollback tracking
// agrees exactly with what InjectSmartopsUICommands actually writes.
func TestSmartopsUICommandPathsMatchesInjectSmartopsUICommands(t *testing.T) {
	home := t.TempDir()
	selected := []model.SkillID{model.SkillSmartopsUI, model.SkillQAEvidence}

	result, err := InjectSmartopsUICommands(home, opencodeAdapter(), selected)
	if err != nil {
		t.Fatalf("InjectSmartopsUICommands() error = %v", err)
	}

	got := SmartopsUICommandPaths(opencodeAdapter(), home, selected)
	if len(got) != len(result.Files) {
		t.Fatalf("SmartopsUICommandPaths() = %v, want to match InjectSmartopsUICommands() files %v", got, result.Files)
	}
	for _, path := range result.Files {
		if !containsFile(got, path) {
			t.Fatalf("SmartopsUICommandPaths() = %v, missing written path %q", got, path)
		}
	}

	if paths := SmartopsUICommandPaths(claudeAdapter(), home, selected); len(paths) != 0 {
		t.Fatalf("SmartopsUICommandPaths(claude) = %v, want none", paths)
	}
}
