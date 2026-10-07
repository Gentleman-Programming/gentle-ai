package uninstall

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v4/internal/backup"
	"github.com/gentleman-programming/gentle-ai/v4/internal/components/agentguidance"
	"github.com/gentleman-programming/gentle-ai/v4/internal/components/reviewassets"
	"github.com/gentleman-programming/gentle-ai/v4/internal/model"
	"github.com/gentleman-programming/gentle-ai/v4/internal/state"
)

// #5256 U2c: a complete Claude uninstall retires the user-global module pilot
// to the monolithic orchestrator; a narrower uninstall leaves it alone.

const claudeModuleUserText = "# My rules\n\nprefix text\n\nsuffix text\n"

// recordingSnapshotter records whether each planned path existed when the
// snapshot was taken, which precedes every uninstall write.
type recordingSnapshotter struct{ existed map[string]bool }

func (r *recordingSnapshotter) Create(snapshotDir string, paths []string) (backup.Manifest, error) {
	r.existed = map[string]bool{}
	for _, path := range paths {
		_, err := os.Lstat(path)
		r.existed[path] = err == nil
	}
	return backup.Manifest{}, os.MkdirAll(snapshotDir, 0o755)
}

// claudeModuleUninstallFixture installs Claude guidance around user text,
// through the module pilot when modules is set, next to a native agent and
// an install state recording Claude.
func claudeModuleUninstallFixture(t *testing.T, modules bool) (*Service, *recordingSnapshotter, string) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	for path, content := range map[string]string{"CLAUDE.md": claudeModuleUserText, "agents/review-risk.md": "native agent"} {
		path = filepath.Join(home, ".claude", path)
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil || os.WriteFile(path, []byte(content), 0o600) != nil {
			t.Fatalf("write %s", path)
		}
	}
	options := agentguidance.RoutingOptions{ClaudeGlobalModules: modules, ReviewContract: reviewassets.ReviewExecutionContractFor}
	if _, err := agentguidance.InjectRoutingWithOptions(home, model.AgentClaudeCode, options); err != nil {
		t.Fatal(err)
	}
	if err := state.Write(home, state.InstallState{InstalledAgents: []string{string(model.AgentClaudeCode)}}); err != nil {
		t.Fatal(err)
	}
	svc, err := NewService(home, t.TempDir(), "dev")
	if err != nil {
		t.Fatal(err)
	}
	snapshots := &recordingSnapshotter{}
	svc.snapshotter = snapshots
	return svc, snapshots, home
}

func readTestFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// pilotPaths returns the core, the seven module names and the ledger that a
// pilot install under home has planned.
func pilotPaths(t *testing.T, home string) []string {
	t.Helper()
	paths, err := agentguidance.ClaudeGlobalModulePaths(home)
	if err != nil || len(paths) != 9 {
		t.Fatalf("ClaudeGlobalModulePaths = %v, %v; want nine paths", paths, err)
	}
	return paths
}

func TestClaudeUninstallRetiresModulePilotOnlyOnFullRemoval(t *testing.T) {
	t.Run("component only", func(t *testing.T) {
		svc, snapshots, home := claudeModuleUninstallFixture(t, true)
		paths := pilotPaths(t, home)
		ledger := paths[len(paths)-1]
		before := readTestFile(t, ledger)
		if _, err := svc.PartialUninstall([]model.AgentID{model.AgentClaudeCode}, []model.ComponentID{model.ComponentPersona}); err != nil {
			t.Fatal(err)
		}
		if _, planned := snapshots.existed[ledger]; planned || readTestFile(t, ledger) != before {
			t.Fatal("a component-only uninstall planned or retired the module pilot")
		}
	})

	svc, snapshots, home := claudeModuleUninstallFixture(t, true)
	paths := pilotPaths(t, home)
	corePath, ledger := paths[0], paths[len(paths)-1]
	moduleDir := filepath.Dir(ledger)
	var installed []string
	for _, path := range paths[1 : len(paths)-1] {
		if _, err := os.Lstat(path); err == nil {
			installed = append(installed, path)
		}
	}
	if len(installed) < 2 {
		t.Fatalf("pilot installed %v, want several modules", installed)
	}
	modified := installed[0]
	if err := os.WriteFile(modified, []byte("user edit\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	routing := readTestFile(t, corePath)
	routing = routing[strings.Index(routing, "<!-- gentle-ai:agent-routing -->"):]

	result, err := svc.PartialUninstall([]model.AgentID{model.AgentClaudeCode}, nil)
	if err != nil {
		t.Fatalf("PartialUninstall: %v", err)
	}
	for _, path := range paths {
		if _, planned := snapshots.existed[path]; !planned {
			t.Fatalf("uninstall snapshot lacks %s", path)
		}
	}
	if !snapshots.existed[ledger] || !snapshots.existed[modified] {
		t.Fatal("uninstall snapshot was taken after the pilot changed")
	}
	monolith, err := agentguidance.RenderOrchestratorWithSource(model.AgentClaudeCode, reviewassets.ReviewExecutionContractFor, "")
	if err != nil {
		t.Fatal(err)
	}
	core := readTestFile(t, corePath)
	if !strings.HasPrefix(core, claudeModuleUserText) || !strings.Contains(core, strings.TrimSpace(monolith)) ||
		!strings.HasSuffix(core, routing) || strings.Contains(core, moduleDir) {
		t.Fatalf("core is not user text, monolith and unchanged routing:\n%s", core)
	}
	if info, err := os.Stat(corePath); err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("core mode changed: %v, %v", info.Mode(), err)
	}
	for _, path := range append(installed[1:], ledger) {
		if _, err := os.Lstat(path); !os.IsNotExist(err) || !slices.Contains(result.RemovedFiles, path) {
			t.Fatalf("%s not removed and reported (%v): %v", path, err, result.RemovedFiles)
		}
	}
	if readTestFile(t, modified) != "user edit\n" || slices.Contains(result.RemovedFiles, modified) ||
		!slices.ContainsFunc(result.ManualActions, func(action string) bool { return strings.Contains(action, modified) }) {
		t.Fatalf("modified module not kept and reported: %v", result.ManualActions)
	}
	if readTestFile(t, filepath.Join(home, ".claude", "agents", "review-risk.md")) != "native agent" {
		t.Fatal("native agent changed")
	}
	if !slices.Equal(result.AgentsRemovedFromState, []model.AgentID{model.AgentClaudeCode}) {
		t.Fatalf("AgentsRemovedFromState = %v", result.AgentsRemovedFromState)
	}
}

func TestClaudeUninstallLeavesMonolithicInstallUnchanged(t *testing.T) {
	svc, snapshots, home := claudeModuleUninstallFixture(t, false)
	corePath := filepath.Join(home, ".claude", "CLAUDE.md")
	before := readTestFile(t, corePath)
	if _, err := svc.PartialUninstall([]model.AgentID{model.AgentClaudeCode}, nil); err != nil {
		t.Fatal(err)
	}
	for path := range snapshots.existed {
		if strings.Contains(path, filepath.Join(".claude", "gentle-ai")) {
			t.Fatalf("monolithic uninstall planned module path %s", path)
		}
	}
	if readTestFile(t, corePath) != before {
		t.Fatal("monolithic orchestrator guidance changed")
	}
	if _, err := os.Lstat(filepath.Join(home, ".claude", "gentle-ai")); !os.IsNotExist(err) {
		t.Fatalf("monolithic uninstall created module storage: %v", err)
	}
}

func TestClaudeUninstallKeepsClaudeInstalledWhenPilotRetirementFails(t *testing.T) {
	svc, _, home := claudeModuleUninstallFixture(t, true)
	paths := pilotPaths(t, home)
	ledger := paths[len(paths)-1]
	if err := os.WriteFile(ledger, []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	core := readTestFile(t, paths[0])

	result, err := svc.PartialUninstall([]model.AgentID{model.AgentClaudeCode}, nil)
	if err == nil || !slices.Equal(result.FailedAgents, []model.AgentID{model.AgentClaudeCode}) || len(result.AgentsRemovedFromState) != 0 {
		t.Fatalf("uninstall = %+v, %v; want a Claude failure", result, err)
	}
	recorded, stateErr := state.Read(home)
	if stateErr != nil || !slices.Contains(recorded.InstalledAgents, string(model.AgentClaudeCode)) {
		t.Fatalf("state = %+v, %v; want Claude still installed", recorded, stateErr)
	}
	if readTestFile(t, ledger) != "{not json" || readTestFile(t, paths[0]) != core {
		t.Fatal("failed retirement changed the ledger or the core")
	}
}
