package upgrade

import (
	"slices"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v5/internal/components/agentguidance"
	"github.com/gentleman-programming/gentle-ai/v5/internal/model"
	"github.com/gentleman-programming/gentle-ai/v5/internal/state"
)

func TestCodexModulePathsAreIncludedInUpgradeBackup(t *testing.T) {
	home := t.TempDir()
	if err := state.Write(home, state.InstallState{InstalledAgents: []string{string(model.AgentCodex)}, SelectionConfigured: true}); err != nil {
		t.Fatal(err)
	}
	want, err := agentguidance.RoutingPathsWithOptions(home, model.AgentCodex, agentguidance.RoutingOptions{CodexGlobalModules: true})
	if err != nil {
		t.Fatal(err)
	}
	got := configPathsForBackup(home)
	for _, path := range want {
		if !slices.Contains(got, path) {
			t.Errorf("upgrade backup is missing Codex module path %q", path)
		}
	}
}
