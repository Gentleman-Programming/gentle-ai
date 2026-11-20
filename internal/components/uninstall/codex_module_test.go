package uninstall

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v4/internal/components/agentguidance"
	"github.com/gentleman-programming/gentle-ai/v4/internal/components/reviewassets"
	"github.com/gentleman-programming/gentle-ai/v4/internal/model"
	"github.com/gentleman-programming/gentle-ai/v4/internal/state"
)

func codexModuleUninstallFixture(t *testing.T) (*Service, *recordingSnapshotter, string, []string) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	corePath := filepath.Join(home, ".codex", "AGENTS.md")
	if err := os.MkdirAll(filepath.Dir(corePath), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(corePath, []byte(claudeModuleUserText), 0o600); err != nil {
		t.Fatal(err)
	}
	options := agentguidance.RoutingOptions{CodexGlobalModules: true, ReviewContract: reviewassets.ReviewExecutionContractFor}
	if _, err := agentguidance.InjectRoutingWithOptions(home, model.AgentCodex, options); err != nil {
		t.Fatal(err)
	}
	if err := state.Write(home, state.InstallState{InstalledAgents: []string{string(model.AgentCodex)}}); err != nil {
		t.Fatal(err)
	}
	paths, err := agentguidance.CodexGlobalModulePaths(home)
	if err != nil || len(paths) != 9 {
		t.Fatalf("CodexGlobalModulePaths = %v, %v", paths, err)
	}
	svc, err := NewService(home, t.TempDir(), "dev")
	if err != nil {
		t.Fatal(err)
	}
	snapshots := &recordingSnapshotter{}
	svc.snapshotter = snapshots
	return svc, snapshots, home, paths
}

func TestCodexUninstallRetiresModulesOnlyOnFullRemoval(t *testing.T) {
	t.Run("component only preserves modules", func(t *testing.T) {
		svc, snapshots, _, paths := codexModuleUninstallFixture(t)
		ledger := paths[len(paths)-1]
		before := readTestFile(t, ledger)
		if _, err := svc.PartialUninstall([]model.AgentID{model.AgentCodex}, []model.ComponentID{model.ComponentPersona}); err != nil {
			t.Fatal(err)
		}
		if snapshots.existed[ledger] || readTestFile(t, ledger) != before {
			t.Fatal("component-only uninstall planned or changed Codex modules")
		}
	})

	svc, snapshots, _, paths := codexModuleUninstallFixture(t)
	var installed []string
	for _, path := range paths[1:] {
		if _, err := os.Lstat(path); err == nil {
			installed = append(installed, path)
		}
	}
	result, err := svc.PartialUninstall([]model.AgentID{model.AgentCodex}, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range paths {
		if _, planned := snapshots.existed[path]; !planned {
			t.Fatalf("uninstall snapshot lacks %s", path)
		}
	}
	for _, path := range installed {
		if _, err := os.Lstat(path); !os.IsNotExist(err) || !slices.Contains(result.RemovedFiles, path) {
			t.Fatalf("module path %s was not removed and reported", path)
		}
	}
	monolith, err := agentguidance.RenderOrchestratorWithSource(model.AgentCodex, reviewassets.ReviewExecutionContractFor, "")
	if err != nil {
		t.Fatal(err)
	}
	core := readTestFile(t, paths[0])
	if !strings.HasPrefix(core, claudeModuleUserText) || !strings.Contains(core, strings.TrimSpace(monolith)) || strings.Contains(core, filepath.Dir(paths[len(paths)-1])) {
		t.Fatalf("Codex core was not restored to the monolith around user bytes:\n%s", core)
	}
}
