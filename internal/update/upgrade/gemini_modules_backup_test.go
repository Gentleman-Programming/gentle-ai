package upgrade

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v4/internal/components/geminimodules"
	"github.com/gentleman-programming/gentle-ai/v4/internal/state"
)

// TestConfigPathsForBackup_GeminiModules covers the files the upgraded
// binary's sync rewrites for Gemini CLI: the GEMINI.md root, the staging file
// and every published module.
func TestConfigPathsForBackup_GeminiModules(t *testing.T) {
	home := t.TempDir()
	if err := state.Write(home, state.InstallState{InstalledAgents: []string{"gemini-cli"}, SelectionConfigured: true}); err != nil {
		t.Fatal(err)
	}
	module := filepath.Join(geminimodules.ModulesDir(home), "01-persona.md")
	files := []string{module, geminimodules.StagingPath(home), geminimodules.RootPath(home)}
	for _, path := range files {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("content\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	paths := configPathsForBackup(home)
	for _, path := range files {
		if !slices.Contains(paths, path) {
			t.Errorf("%s missing from backup: %v", path, paths)
		}
	}
}
