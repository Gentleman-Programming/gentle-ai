package agentguidance

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v4/internal/model"
)

var codexModuleOptions = RoutingOptions{CodexGlobalModules: true, ReviewContract: fakeModuleContract}

func TestCodexModuleDeliveryReconstructsAndInstallsReachableModules(t *testing.T) {
	home := t.TempDir()
	configDir := filepath.Join(home, ".codex")
	moduleDir := filepath.Join(configDir, "gentle-ai", "orchestrator")
	bundle, err := buildOrchestratorModules(model.AgentCodex, fakeModuleContract, moduleDir)
	if err != nil {
		t.Fatal(err)
	}
	public, err := RenderOrchestratorWithSource(model.AgentCodex, fakeModuleContract, "")
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("Codex module core: %d bytes; public monolith: %d bytes", len(bundle.core), len(public))
	for _, module := range bundle.modules {
		t.Logf("Codex module %s: %d bytes", module.file, len(module.content))
	}
	reconstructed, err := bundle.reconstruct()
	if err != nil || reconstructed != public {
		t.Fatalf("reconstruct = %d bytes, %v; want %d bytes", len(reconstructed), err, len(public))
	}

	result, err := InjectRoutingWithOptions(home, model.AgentCodex, codexModuleOptions)
	if err != nil || !result.Changed {
		t.Fatalf("inject = %+v, %v; want changed", result, err)
	}
	corePath := filepath.Join(configDir, "AGENTS.md")
	core, err := os.ReadFile(corePath)
	if err != nil {
		t.Fatal(err)
	}
	for _, module := range bundle.modules {
		path := filepath.Join(moduleDir, module.file)
		if !strings.Contains(string(core), "read `"+path+"`") {
			t.Errorf("core has no pointer to %s", path)
		}
		if data, err := os.ReadFile(path); err != nil || string(data) != module.content {
			t.Errorf("module %s = %q, %v", path, data, err)
		}
	}

	before := snapshotModuleTree(t, home)
	again, err := InjectRoutingWithOptions(home, model.AgentCodex, codexModuleOptions)
	if err != nil || again.Changed || !reflect.DeepEqual(snapshotModuleTree(t, home), before) {
		t.Fatalf("second inject = %+v, %v; want byte-identical no-op", again, err)
	}
}

func TestCodexModuleDeliveryRejectsModifiedModuleWithoutChangingData(t *testing.T) {
	home := t.TempDir()
	if _, err := InjectRoutingWithOptions(home, model.AgentCodex, codexModuleOptions); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(home, ".codex", "gentle-ai", "orchestrator", "orchestrator-skills.md")
	if err := os.WriteFile(path, []byte("user customization\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	before := snapshotModuleTree(t, home)
	if _, err := InjectRoutingWithOptions(home, model.AgentCodex, codexModuleOptions); err == nil {
		t.Fatal("inject succeeded with a modified module")
	}
	if after := snapshotModuleTree(t, home); !reflect.DeepEqual(after, before) {
		t.Fatal("rejected injection changed stored data")
	}
}

func TestCodexModuleTransactionRollsBackCoreAndModules(t *testing.T) {
	home := t.TempDir()
	configDir := filepath.Join(home, ".codex")
	corePath := filepath.Join(configDir, "AGENTS.md")
	if err := os.MkdirAll(configDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(corePath, []byte("user rules\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	bundle, err := buildOrchestratorModules(model.AgentCodex, fakeModuleContract, filepath.Join(configDir, "gentle-ai", "orchestrator"))
	if err != nil {
		t.Fatal(err)
	}
	before := snapshotModuleTree(t, home)
	cause := errors.New("fail after publishing Codex core")
	result, err := commitObservedOrchestratorCoreAt(configDir, corePath, func(existing string) string {
		return existing + bundle.core
	}, bundle.modules, func() error { return cause })
	if !errors.Is(err, cause) || result.Changed {
		t.Fatalf("transaction = %+v, %v; want complete rollback", result, err)
	}
	after := snapshotModuleTree(t, home)
	if after[corePath] != before[corePath] {
		t.Fatal("failed Codex transaction did not restore the core bytes and mode")
	}
	for path, entry := range after {
		if path != corePath && strings.HasPrefix(entry, "-") {
			t.Fatalf("failed Codex transaction left file %s", path)
		}
	}
}

func TestRetireCodexGlobalModulesPreservesCustomizedModule(t *testing.T) {
	home := t.TempDir()
	if _, err := InjectRoutingWithOptions(home, model.AgentCodex, codexModuleOptions); err != nil {
		t.Fatal(err)
	}
	paths, err := CodexGlobalModulePaths(home)
	if err != nil || len(paths) != 9 {
		t.Fatalf("CodexGlobalModulePaths = %v, %v", paths, err)
	}
	customized := paths[1]
	if err := os.WriteFile(customized, []byte("user customization\n"), 0o640); err != nil {
		t.Fatal(err)
	}
	if _, err := RetireCodexGlobalModules(home, fakeModuleContract); err != nil {
		t.Fatal(err)
	}
	if got, err := os.ReadFile(customized); err != nil || string(got) != "user customization\n" {
		t.Fatalf("customized module = %q, %v", got, err)
	}
	core, err := os.ReadFile(paths[0])
	if err != nil {
		t.Fatal(err)
	}
	monolith, err := RenderOrchestratorWithSource(model.AgentCodex, fakeModuleContract, "")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(core), strings.TrimSpace(monolith)) || strings.Contains(string(core), filepath.Dir(paths[len(paths)-1])) {
		t.Fatal("retired Codex core is not the monolith or still points to modules")
	}
	for _, path := range paths[2:] {
		if _, err := os.Lstat(path); !os.IsNotExist(err) {
			t.Fatalf("owned Codex module path remains %s: %v", path, err)
		}
	}
}
