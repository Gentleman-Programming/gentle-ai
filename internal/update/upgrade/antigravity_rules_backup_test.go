package upgrade

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v4/internal/components/antigravityrules"
	"github.com/gentleman-programming/gentle-ai/v4/internal/state"
)

// TestConfigPathsForBackup_AntigravityRules covers the files the upgraded
// binary's sync rewrites for Antigravity: published rules and the legacy
// GEMINI.md whose Antigravity sections it retires.
func TestConfigPathsForBackup_AntigravityRules(t *testing.T) {
	tests := []struct {
		name      string
		agents    []string
		wantRules bool
	}{
		{name: "antigravity", agents: []string{"antigravity"}, wantRules: true},
		{name: "gemini-cli only", agents: []string{"gemini-cli"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			home := t.TempDir()
			if err := state.Write(home, state.InstallState{InstalledAgents: tt.agents, SelectionConfigured: true}); err != nil {
				t.Fatal(err)
			}
			rule := filepath.Join(antigravityrules.RulesDir(home), "gentle-ai-01-persona.md")
			legacy := antigravityrules.LegacyPromptPath(home)
			for path, content := range map[string]string{rule: "---\ntrigger: always_on\n---\n\npersona\n", legacy: "# rules\n"} {
				if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
					t.Fatal(err)
				}
			}

			paths := configPathsForBackup(home)
			if got := slices.Contains(paths, rule); got != tt.wantRules {
				t.Fatalf("published rule in backup = %v, want %v; paths: %v", got, tt.wantRules, paths)
			}
			if tt.wantRules && !slices.Contains(paths, antigravityrules.StagingPath(home)) {
				t.Fatalf("staging file missing from backup: %v", paths)
			}
			// GEMINI.md is covered either as the legacy Antigravity prompt or
			// as Gemini CLI's own system prompt.
			if !slices.Contains(paths, legacy) {
				t.Fatalf("GEMINI.md missing from backup: %v", paths)
			}
		})
	}
}
