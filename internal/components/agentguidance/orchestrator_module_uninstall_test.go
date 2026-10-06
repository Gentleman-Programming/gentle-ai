package agentguidance

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v4/internal/model"
)

// #5256 U2c covers retiring the Claude module pilot back to the monolithic
// orchestrator, which the uninstall service does for a complete Claude removal.

const claudeNativeAgentSentinel = "native agent sentinel\n"

// installClaudeModulePilot installs the module pilot around user text in a
// fresh home, next to a native agent that retirement must never touch.
func installClaudeModulePilot(t *testing.T) (home, configDir, moduleDir string) {
	t.Helper()
	home = t.TempDir()
	configDir = filepath.Join(home, ".claude")
	moduleDir = filepath.Join(configDir, "gentle-ai", "orchestrator")
	writeModuleTestFile(t, filepath.Join(configDir, "CLAUDE.md"), coreTransactionUserCore, 0o600)
	writeModuleTestFile(t, filepath.Join(configDir, "agents", "review-risk.md"), claudeNativeAgentSentinel, 0o600)
	if _, err := InjectRoutingWithOptions(home, model.AgentClaudeCode, claudeModuleOptions); err != nil {
		t.Fatalf("install pilot: %v", err)
	}
	return home, configDir, moduleDir
}

func assertPathAbsent(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Lstat(path); !os.IsNotExist(err) {
		t.Fatalf("%s still present: %v", path, err)
	}
}

func TestRetireClaudeGlobalModulesRestoresMonolith(t *testing.T) {
	home, configDir, moduleDir := installClaudeModulePilot(t)
	corePath := filepath.Join(configDir, "CLAUDE.md")
	installed, err := os.ReadFile(corePath)
	if err != nil {
		t.Fatal(err)
	}
	paths, err := ClaudeGlobalModulePaths(home)
	if err != nil || !reflect.DeepEqual(paths, plannedClaudeModulePaths(configDir)) {
		t.Fatalf("ClaudeGlobalModulePaths = %v, %v; want the nine planned paths", paths, err)
	}

	result, err := RetireClaudeGlobalModules(home, fakeModuleContract)
	want := []string{corePath}
	for _, module := range buildClaudeTestModules(t).modules {
		want = append(want, filepath.Join(moduleDir, module.file))
	}
	want = append(want, filepath.Join(moduleDir, orchestratorModuleLedgerName))
	if err != nil || !result.Changed || !reflect.DeepEqual(result.Files, want) {
		t.Fatalf("retire = %+v, %v; want changed files %v", result, err, want)
	}
	monolith, err := RenderOrchestratorWithSource(model.AgentClaudeCode, fakeModuleContract, "")
	if err != nil {
		t.Fatal(err)
	}
	// Only the orchestrator section changes; user text, routing and mode stay.
	assertModuleFile(t, corePath, injectOrchestratorSection(string(installed), monolith), 0o600)
	if data, _ := os.ReadFile(corePath); strings.Contains(string(data), moduleDir) {
		t.Fatal("monolithic core still points at the module directory")
	}
	for _, path := range want[1:] {
		assertPathAbsent(t, path)
	}
	assertModuleFile(t, filepath.Join(configDir, "agents", "review-risk.md"), claudeNativeAgentSentinel, 0o600)

	before := snapshotModuleTree(t, home)
	again, err := RetireClaudeGlobalModules(home, fakeModuleContract)
	if paths, _ := ClaudeGlobalModulePaths(home); err != nil || again.Changed || len(again.Files) != 0 || paths != nil {
		t.Fatalf("second retire = %+v, %v (paths %v); want a no-op", again, err, paths)
	}
	if after := snapshotModuleTree(t, home); !reflect.DeepEqual(after, before) {
		t.Fatal("second retire changed the tree")
	}
}

func TestRetireClaudeGlobalModulesKeepsFilesItDoesNotOwn(t *testing.T) {
	home, configDir, moduleDir := installClaudeModulePilot(t)
	modules := buildClaudeTestModules(t).modules
	modified := filepath.Join(moduleDir, modules[0].file)
	var unowned string
	for _, name := range orchestratorModuleNames {
		if !slices.ContainsFunc(modules, func(m orchestratorModule) bool { return m.name == name }) {
			unowned = filepath.Join(moduleDir, orchestratorModuleFile(name))
			break
		}
	}
	unknown := filepath.Join(moduleDir, "notes.md")
	writeModuleTestFile(t, modified, "user edit\n", 0o640)
	writeModuleTestFile(t, unowned, "user module\n", 0o644)
	writeModuleTestFile(t, unknown, "user notes\n", 0o644)

	result, err := RetireClaudeGlobalModules(home, fakeModuleContract)
	if err != nil || !result.Changed || slices.Contains(result.Files, modified) || slices.Contains(result.Files, unowned) {
		t.Fatalf("retire = %+v, %v; want kept files unreported", result, err)
	}
	assertModuleFile(t, modified, "user edit\n", 0o640)
	assertModuleFile(t, unowned, "user module\n", 0o644)
	assertModuleFile(t, unknown, "user notes\n", 0o644)
	for _, module := range modules[1:] {
		assertPathAbsent(t, filepath.Join(moduleDir, module.file))
	}
	assertPathAbsent(t, filepath.Join(moduleDir, orchestratorModuleLedgerName))
	if data, _ := os.ReadFile(filepath.Join(configDir, "CLAUDE.md")); strings.Contains(string(data), moduleDir) {
		t.Fatal("monolithic core still points at the module directory")
	}
}

func TestRetireClaudeGlobalModulesWithoutLedgerChangesNothing(t *testing.T) {
	home := t.TempDir()
	if _, err := InjectRoutingWithOptions(home, model.AgentClaudeCode, RoutingOptions{ReviewContract: fakeModuleContract}); err != nil {
		t.Fatal(err)
	}
	// A module-named file without a ledger is never claimed.
	stray := filepath.Join(home, ".claude", "gentle-ai", "orchestrator", orchestratorModuleFile("delegation"))
	writeModuleTestFile(t, stray, "user file\n", 0o644)
	before := snapshotModuleTree(t, home)

	paths, pathsErr := ClaudeGlobalModulePaths(home)
	result, err := RetireClaudeGlobalModules(home, fakeModuleContract)
	if paths != nil || pathsErr != nil || err != nil || result.Changed || len(result.Files) != 0 {
		t.Fatalf("paths %v, %v; retire = %+v, %v; want a no-op", paths, pathsErr, result, err)
	}
	if after := snapshotModuleTree(t, home); !reflect.DeepEqual(after, before) {
		t.Fatal("retire without a ledger changed the tree")
	}
}

// A user file where a module directory would be means there is no ledger; it
// must not fail, or the uninstall of every agent is cancelled.
func TestClaudeGlobalModulesUserFileAncestorMeansNoLedger(t *testing.T) {
	cases := []struct {
		ancestor string
		managed  bool
	}{
		{ancestor: "gentle-ai", managed: true},
		{ancestor: filepath.Join("gentle-ai", "orchestrator")},
	}
	for _, tc := range cases {
		t.Run(tc.ancestor, func(t *testing.T) {
			home := t.TempDir()
			writeModuleTestFile(t, filepath.Join(home, ".claude", "CLAUDE.md"), coreTransactionUserCore, 0o600)
			if tc.managed {
				if _, err := InjectRoutingWithOptions(home, model.AgentClaudeCode, RoutingOptions{ReviewContract: fakeModuleContract}); err != nil {
					t.Fatal(err)
				}
			}
			writeModuleTestFile(t, filepath.Join(home, ".claude", tc.ancestor), "user file\n", 0o644)
			before := snapshotModuleTree(t, home)

			paths, pathsErr := ClaudeGlobalModulePaths(home)
			result, err := RetireClaudeGlobalModules(home, fakeModuleContract)
			if paths != nil || pathsErr != nil || err != nil || result.Changed || len(result.Files) != 0 {
				t.Fatalf("paths %v, %v; retire = %+v, %v; want a no-op", paths, pathsErr, result, err)
			}
			if after := snapshotModuleTree(t, home); !reflect.DeepEqual(after, before) {
				t.Fatal("retire changed the user file or the core")
			}
		})
	}
}

func TestRetireClaudeGlobalModulesFailsClosedBeforeAnyWrite(t *testing.T) {
	ledgerOf := func(dir string) string { return filepath.Join(dir, orchestratorModuleLedgerName) }
	cases := map[string]func(t *testing.T, moduleDir string){
		"invalid ledger": func(t *testing.T, dir string) { writeModuleTestFile(t, ledgerOf(dir), "{not json", 0o644) },
		"ledger is a directory": func(t *testing.T, dir string) {
			if err := os.Remove(ledgerOf(dir)); err != nil || os.Mkdir(ledgerOf(dir), 0o755) != nil {
				t.Fatal("replace ledger with a directory")
			}
		},
		"symlinked module directory": func(t *testing.T, dir string) {
			if err := os.Rename(dir, dir+".real"); err != nil || os.Symlink(dir+".real", dir) != nil {
				t.Fatal("replace module directory with a symlink")
			}
		},
	}
	for name, corrupt := range cases {
		t.Run(name, func(t *testing.T) {
			home, _, moduleDir := installClaudeModulePilot(t)
			corrupt(t, moduleDir)
			before := snapshotModuleTree(t, home)
			result, err := RetireClaudeGlobalModules(home, fakeModuleContract)
			if err == nil || result.Changed {
				t.Fatalf("retire = %+v, %v; want a failure without changes", result, err)
			}
			if after := snapshotModuleTree(t, home); !reflect.DeepEqual(after, before) {
				t.Fatal("failed retire changed the tree")
			}
		})
	}
	if _, err := RetireClaudeGlobalModules("relative-home", fakeModuleContract); !errors.Is(err, ErrInvalidTarget) {
		t.Fatalf("relative home error = %v, want ErrInvalidTarget", err)
	}
}
