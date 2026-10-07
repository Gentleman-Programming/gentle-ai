package agentguidance

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v4/internal/catalog"
	"github.com/gentleman-programming/gentle-ai/v4/internal/model"
)

// #5256 U2a covers the default-off Claude global module option of the routing
// injector. No CLI, sync, upgrade or uninstall caller sets it yet.

var claudeModuleOptions = RoutingOptions{ClaudeGlobalModules: true, ReviewContract: fakeModuleContract}

// plannedClaudeModulePaths is the complete backup plan for a Claude config
// directory: the core, every known module name and the ledger.
func plannedClaudeModulePaths(configDir string) []string {
	dir := filepath.Join(configDir, "gentle-ai", "orchestrator")
	paths := []string{filepath.Join(configDir, "CLAUDE.md")}
	for _, name := range orchestratorModuleNames {
		paths = append(paths, filepath.Join(dir, orchestratorModuleFile(name)))
	}
	return append(paths, filepath.Join(dir, orchestratorModuleLedgerName))
}

func TestClaudeModuleDeliveryPlansEveryPathBeforeWriting(t *testing.T) {
	home := t.TempDir()
	paths, err := RoutingPathsWithOptions(home, model.AgentClaudeCode, claudeModuleOptions)
	want := plannedClaudeModulePaths(filepath.Join(home, ".claude"))
	if err != nil || len(want) != 9 || !reflect.DeepEqual(paths, want) {
		t.Fatalf("RoutingPathsWithOptions = %v, %v; want the nine planned paths %v", paths, err, want)
	}
	if entries, err := os.ReadDir(home); err != nil || len(entries) != 0 {
		t.Fatalf("planning touched the disk: %d entries, %v", len(entries), err)
	}
	if _, err := RoutingPathsWithOptions("relative-home", model.AgentClaudeCode, claudeModuleOptions); !errors.Is(err, ErrInvalidTarget) {
		t.Fatalf("relative target error = %v, want ErrInvalidTarget", err)
	}
}

func TestClaudeModuleDeliveryInjectsCoreModulesAndLedger(t *testing.T) {
	home := t.TempDir()
	configDir := filepath.Join(home, ".claude")
	corePath := filepath.Join(configDir, "CLAUDE.md")
	writeModuleTestFile(t, corePath, coreTransactionUserCore, 0o600)
	moduleDir := filepath.Join(configDir, "gentle-ai", "orchestrator")
	bundle, err := buildOrchestratorModules(model.AgentClaudeCode, fakeModuleContract, moduleDir)
	if err != nil {
		t.Fatal(err)
	}

	result, err := InjectRoutingWithOptions(home, model.AgentClaudeCode, claudeModuleOptions)
	if err != nil || !result.Changed {
		t.Fatalf("inject = %+v, %v; want changed", result, err)
	}
	wantFiles := []string{corePath}
	for _, module := range bundle.modules {
		wantFiles = append(wantFiles, filepath.Join(moduleDir, module.file))
	}
	wantFiles = append(wantFiles, filepath.Join(moduleDir, orchestratorModuleLedgerName))
	if !reflect.DeepEqual(result.Files, wantFiles) {
		t.Fatalf("Files = %v, want %v", result.Files, wantFiles)
	}

	data, err := os.ReadFile(corePath)
	if err != nil {
		t.Fatal(err)
	}
	core := string(data)
	for _, want := range []string{
		strings.TrimSpace(bundle.core),
		"<!-- gentle-ai:" + RoutingSectionID + " -->",
		"<!-- gentle-ai:remote-authorization -->",
	} {
		if !strings.Contains(core, want) {
			t.Fatalf("core lacks %.80q", want)
		}
	}
	if !strings.HasPrefix(core, "# My rules\n\nprefix text\n\n") || !strings.Contains(core, "\nsuffix text\n") || strings.Contains(core, "old monolith") {
		t.Fatalf("core does not preserve the user text around the managed sections:\n%s", core)
	}
	for _, module := range bundle.modules {
		path := filepath.Join(moduleDir, module.file)
		if !strings.Contains(core, "`"+path+"`") {
			t.Fatalf("core has no pointer to %s", path)
		}
		assertModuleFile(t, path, module.content, 0o644)
	}
	assertModuleFile(t, filepath.Join(moduleDir, orchestratorModuleLedgerName), moduleLedgerJSON(moduleHashes(bundle.modules)), 0o644)

	before := snapshotModuleTree(t, home)
	again, err := InjectRoutingWithOptions(home, model.AgentClaudeCode, claudeModuleOptions)
	if err != nil || again.Changed {
		t.Fatalf("second inject = %+v, %v; want no-op", again, err)
	}
	if after := snapshotModuleTree(t, home); !reflect.DeepEqual(after, before) {
		t.Fatalf("second inject changed the tree")
	}
}

func TestClaudeModuleDeliveryLeavesDefaultAndOtherAgentsMonolithic(t *testing.T) {
	for _, agent := range catalog.AllAgents() {
		if agent.ID == model.AgentConductor || agent.ID == model.AgentClaudeCode {
			continue
		}
		home := t.TempDir()
		plain, err := RoutingPaths(home, agent.ID)
		opted, optedErr := RoutingPathsWithOptions(home, agent.ID, claudeModuleOptions)
		if err != nil || optedErr != nil || !reflect.DeepEqual(opted, plain) {
			t.Fatalf("%s paths with option = %v, %v; want %v, %v", agent.ID, opted, optedErr, plain, err)
		}
	}

	home := t.TempDir()
	corePath := filepath.Join(home, ".claude", "CLAUDE.md")
	result, err := InjectRoutingWithOptions(home, model.AgentClaudeCode, RoutingOptions{ReviewContract: fakeModuleContract})
	if err != nil || !reflect.DeepEqual(result.Files, []string{corePath}) {
		t.Fatalf("default inject = %+v, %v; want only %s", result, err, corePath)
	}
	public, err := RenderOrchestratorWithSource(model.AgentClaudeCode, fakeModuleContract, "")
	if err != nil {
		t.Fatal(err)
	}
	if data, err := os.ReadFile(corePath); err != nil || !strings.Contains(string(data), strings.TrimSpace(public)) {
		t.Fatalf("default core lacks the public monolith: %v", err)
	}
	if _, err := os.Lstat(filepath.Join(home, ".claude", "gentle-ai")); !os.IsNotExist(err) {
		t.Fatalf("default inject created module storage: %v", err)
	}
}

func TestOrchestratorCoreTransactionReportsRetiredModule(t *testing.T) {
	f := newCoreTransactionFixture(t)
	result, err := commitOrchestratorCore(f.configDir, f.merge, f.bundle.modules)
	if err != nil || !slices.Contains(result.Files, f.unusedPath) {
		t.Fatalf("commit = %+v, %v; want the retired %s reported", result, err, f.unusedPath)
	}
	if _, err := os.Lstat(f.unusedPath); !os.IsNotExist(err) {
		t.Fatalf("retired module still present: %v", err)
	}
	planned := plannedClaudeModulePaths(f.configDir)
	for _, file := range result.Files {
		if !slices.Contains(planned, file) {
			t.Fatalf("reported %s outside the planned paths %v", file, planned)
		}
	}
}
