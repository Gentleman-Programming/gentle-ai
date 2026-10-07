package agentguidance

import (
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v4/internal/managedownership"
)

// #5256 U1a covers only the private module install helpers against a
// temporary Claude config directory. Nothing here wires them into a public
// install, a core write or a lifecycle command.

func moduleInstallFixture(t *testing.T) (base, configDir, dir string, modules []orchestratorModule) {
	t.Helper()
	modules = buildClaudeTestModules(t).modules
	if len(modules) != 3 {
		t.Fatalf("Claude bundle has %d modules, want 3", len(modules))
	}
	base = t.TempDir()
	configDir = filepath.Join(base, "claude")
	return base, configDir, filepath.Join(configDir, "gentle-ai", "orchestrator"), modules
}

func writeModuleTestFile(t *testing.T, path, content string, mode fs.FileMode) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), mode); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, mode); err != nil {
		t.Fatal(err)
	}
}

func moduleLedgerJSON(files map[string]string) string {
	data, err := json.MarshalIndent(orchestratorModuleLedger{Version: 1, Files: files}, "", "  ")
	if err != nil {
		panic(err)
	}
	return string(data) + "\n"
}

func moduleHashes(modules []orchestratorModule) map[string]string {
	files := map[string]string{}
	for _, module := range modules {
		files[module.file] = managedownership.Hash([]byte(module.content))
	}
	return files
}

func assertModuleFile(t *testing.T, path, want string, mode fs.FileMode) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	info, err := os.Lstat(path)
	if err != nil || string(data) != want || info.Mode() != mode {
		t.Fatalf("%s = %q mode %v (%v), want %q mode %v", path, data, info.Mode(), err, want, mode)
	}
}

// snapshotModuleTree records every path under root with its mode and either
// its bytes or its symlink target, without following symlinks.
func snapshotModuleTree(t *testing.T, root string) map[string]string {
	t.Helper()
	tree := map[string]string{}
	err := filepath.WalkDir(root, func(path string, _ fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		info, err := os.Lstat(path)
		if err != nil {
			return err
		}
		entry := info.Mode().String()
		if info.Mode()&fs.ModeSymlink != 0 {
			target, err := os.Readlink(path)
			if err != nil {
				return err
			}
			entry += " -> " + target
		} else if info.Mode().IsRegular() {
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			entry += " " + string(data)
		}
		tree[path] = entry
		return nil
	})
	if err != nil {
		t.Fatalf("snapshot %s: %v", root, err)
	}
	return tree
}

func runModuleInstall(t *testing.T, configDir string, modules []orchestratorModule) *orchestratorModuleInstall {
	t.Helper()
	install, err := prepareOrchestratorModuleInstall(configDir, modules)
	if err != nil {
		t.Fatalf("prepare: %v", err)
	}
	if err := install.stage(); err != nil {
		t.Fatalf("stage: %v", err)
	}
	if err := install.finalize(); err != nil {
		t.Fatalf("finalize: %v", err)
	}
	return install
}

func TestOrchestratorModuleInstallStagesThenFinalizesLedger(t *testing.T) {
	for _, linked := range []bool{false, true} {
		t.Run(map[bool]string{false: "real config dir", true: "symlinked config dir"}[linked], func(t *testing.T) {
			base, configDir, dir, modules := moduleInstallFixture(t)
			if linked {
				real := filepath.Join(base, "real-claude")
				if err := os.Mkdir(real, 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(real, configDir); err != nil {
					t.Fatal(err)
				}
			}
			install, err := prepareOrchestratorModuleInstall(configDir, modules)
			if err != nil {
				t.Fatalf("prepare: %v", err)
			}
			if err := install.finalize(); err == nil {
				t.Fatal("finalize before stage succeeded")
			}
			if _, err := os.Lstat(filepath.Join(configDir, "gentle-ai")); !os.IsNotExist(err) {
				t.Fatalf("prepare or early finalize created directories: %v", err)
			}
			if err := install.stage(); err != nil {
				t.Fatalf("stage: %v", err)
			}
			ledger := filepath.Join(dir, orchestratorModuleLedgerName)
			if _, err := os.Lstat(ledger); !os.IsNotExist(err) {
				t.Fatalf("ledger exists before finalize: %v", err)
			}
			for _, module := range modules {
				assertModuleFile(t, filepath.Join(dir, module.file), module.content, 0o644)
			}
			for _, created := range []string{filepath.Dir(dir), dir} {
				if info, err := os.Lstat(created); err != nil || !info.IsDir() || info.Mode().Perm()&^0o700 != 0 {
					t.Fatalf("created directory %s = %v (%v), want private directory", created, info, err)
				}
			}
			if err := install.finalize(); err != nil {
				t.Fatalf("finalize: %v", err)
			}
			assertModuleFile(t, ledger, moduleLedgerJSON(moduleHashes(modules)), 0o644)

			before := snapshotModuleTree(t, base)
			again := runModuleInstall(t, configDir, modules)
			if len(again.writes) != 0 || len(again.retire) != 0 || again.writeLedger {
				t.Fatalf("repeated plan writes=%d retire=%d ledger=%v, want no mutation", len(again.writes), len(again.retire), again.writeLedger)
			}
			if after := snapshotModuleTree(t, base); !reflect.DeepEqual(before, after) {
				t.Fatalf("repeated install changed the tree:\nbefore %v\nafter  %v", before, after)
			}
		})
	}
}

func TestOrchestratorModuleInstallUpdatesOwnedAndAdoptsIdentical(t *testing.T) {
	_, configDir, dir, modules := moduleInstallFixture(t)
	owned, identical := filepath.Join(dir, modules[0].file), filepath.Join(dir, modules[1].file)
	writeModuleTestFile(t, owned, "previous release\n", 0o600)
	writeModuleTestFile(t, identical, modules[1].content, 0o640)
	ledger := filepath.Join(dir, orchestratorModuleLedgerName)
	writeModuleTestFile(t, ledger, moduleLedgerJSON(map[string]string{modules[0].file: managedownership.Hash([]byte("previous release\n"))}), 0o600)

	install := runModuleInstall(t, configDir, modules)
	if len(install.writes) != 2 {
		t.Fatalf("writes = %d, want the owned update and the absent module only", len(install.writes))
	}
	assertModuleFile(t, owned, modules[0].content, 0o600)
	assertModuleFile(t, identical, modules[1].content, 0o640)
	assertModuleFile(t, filepath.Join(dir, modules[2].file), modules[2].content, 0o644)
	assertModuleFile(t, ledger, moduleLedgerJSON(moduleHashes(modules)), 0o600)
}

func TestOrchestratorModuleInstallRejectsBeforeAnyWrite(t *testing.T) {
	type fixture struct {
		base, configDir, dir string
		modules              []orchestratorModule
	}
	module := func(f *fixture, name string) string { return filepath.Join(f.dir, orchestratorModuleFile(name)) }
	ledger := func(t *testing.T, f *fixture, body string) {
		writeModuleTestFile(t, filepath.Join(f.dir, orchestratorModuleLedgerName), body, 0o644)
	}
	link := func(t *testing.T, target, path string) {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(target, path); err != nil {
			t.Fatal(err)
		}
	}
	tests := []struct {
		name    string
		arrange func(*testing.T, *fixture)
		want    string
	}{
		{"modified owned module", func(t *testing.T, f *fixture) {
			writeModuleTestFile(t, module(f, "delegation"), "user edit\n", 0o644)
			ledger(t, f, moduleLedgerJSON(map[string]string{"orchestrator-delegation.md": managedownership.Hash([]byte("released\n"))}))
		}, "modified or not owned"},
		{"unowned module", func(t *testing.T, f *fixture) {
			writeModuleTestFile(t, module(f, "writer"), "user file\n", 0o644)
		}, "modified or not owned"},
		{"header-only module", func(t *testing.T, f *fixture) {
			writeModuleTestFile(t, module(f, "delegation"), strings.SplitAfter(f.modules[0].content, "\n")[0], 0o644)
		}, "modified or not owned"},
		{"module directory", func(t *testing.T, f *fixture) {
			if err := os.MkdirAll(module(f, "delegation"), 0o755); err != nil {
				t.Fatal(err)
			}
		}, "not a regular file"},
		{"module symlink", func(t *testing.T, f *fixture) {
			writeModuleTestFile(t, filepath.Join(f.base, "outside.md"), f.modules[0].content, 0o644)
			link(t, filepath.Join(f.base, "outside.md"), module(f, "delegation"))
		}, "not a regular file"},
		{"unused known module symlink", func(t *testing.T, f *fixture) {
			writeModuleTestFile(t, filepath.Join(f.base, "outside.md"), "user memory\n", 0o644)
			link(t, filepath.Join(f.base, "outside.md"), module(f, "memory"))
		}, "not a regular file"},
		{"ledger symlink", func(t *testing.T, f *fixture) {
			writeModuleTestFile(t, filepath.Join(f.base, "ledger.json"), moduleLedgerJSON(map[string]string{}), 0o644)
			link(t, filepath.Join(f.base, "ledger.json"), filepath.Join(f.dir, orchestratorModuleLedgerName))
		}, "ownership ledger is not a regular file"},
		{"malformed ledger", func(t *testing.T, f *fixture) { ledger(t, f, "{") }, "decode ownership ledger"},
		{"ledger version", func(t *testing.T, f *fixture) { ledger(t, f, `{"version":2,"files":{}}`) }, "unsupported ownership ledger"},
		{"ledger unknown name", func(t *testing.T, f *fixture) {
			ledger(t, f, `{"version":1,"files":{"notes.md":"`+strings.Repeat("a", 64)+`"}}`)
		}, "invalid ownership ledger entry"},
		{"ledger bad hash", func(t *testing.T, f *fixture) {
			ledger(t, f, `{"version":1,"files":{"orchestrator-delegation.md":"ABC"}}`)
		}, "invalid ownership ledger entry"},
		{"module dir symlink", func(t *testing.T, f *fixture) {
			if err := os.Mkdir(filepath.Join(f.base, "elsewhere"), 0o755); err != nil {
				t.Fatal(err)
			}
			link(t, filepath.Join(f.base, "elsewhere"), f.dir)
		}, "not a real directory"},
		{"module parent file", func(t *testing.T, f *fixture) {
			writeModuleTestFile(t, filepath.Dir(f.dir), "not a directory\n", 0o644)
		}, "not a real directory"},
		{"module dir file", func(t *testing.T, f *fixture) { writeModuleTestFile(t, f.dir, "not a directory\n", 0o644) }, "not a real directory"},
		{"relative config dir", func(t *testing.T, f *fixture) { f.configDir = "claude" }, "not absolute"},
		{"no modules", func(t *testing.T, f *fixture) { f.modules = nil }, "no orchestrator modules"},
		{"unknown module", func(t *testing.T, f *fixture) {
			f.modules[0].name, f.modules[0].file = "notes", "orchestrator-notes.md"
		}, "invalid orchestrator module"},
		{"mismatched file", func(t *testing.T, f *fixture) { f.modules[0].file = f.modules[1].file }, "invalid orchestrator module"},
		{"empty content", func(t *testing.T, f *fixture) { f.modules[0].content = " \n" }, "invalid orchestrator module"},
		{"duplicate module", func(t *testing.T, f *fixture) { f.modules = append(f.modules, f.modules[0]) }, "invalid orchestrator module"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			base, configDir, dir, modules := moduleInstallFixture(t)
			f := &fixture{base: base, configDir: configDir, dir: dir, modules: slices.Clone(modules)}
			tt.arrange(t, f)
			before := snapshotModuleTree(t, base)
			install, err := prepareOrchestratorModuleInstall(f.configDir, f.modules)
			if err == nil || !strings.Contains(err.Error(), tt.want) || install != nil {
				t.Fatalf("prepare = %v, %v; want nil plan and error containing %q", install, err, tt.want)
			}
			if after := snapshotModuleTree(t, base); !reflect.DeepEqual(before, after) {
				t.Fatalf("rejected prepare changed the tree:\nbefore %v\nafter  %v", before, after)
			}
		})
	}
}

func TestOrchestratorModuleInstallRetiresOnlyOwnedUnusedAfterFinalize(t *testing.T) {
	_, configDir, dir, modules := moduleInstallFixture(t)
	module := func(name string) string { return filepath.Join(dir, orchestratorModuleFile(name)) }
	writeModuleTestFile(t, module("memory"), "owned old memory\n", 0o644)
	writeModuleTestFile(t, module("tracking"), "user edited tracking\n", 0o644)
	writeModuleTestFile(t, module("prompts"), "user prompts\n", 0o644)
	notes := filepath.Join(dir, "notes.md")
	writeModuleTestFile(t, notes, "user notes\n", 0o644)
	ledger := filepath.Join(dir, orchestratorModuleLedgerName)
	writeModuleTestFile(t, ledger, moduleLedgerJSON(map[string]string{
		"orchestrator-memory.md":       managedownership.Hash([]byte("owned old memory\n")),
		"orchestrator-tracking.md":     managedownership.Hash([]byte("released tracking\n")),
		"orchestrator-verification.md": managedownership.Hash([]byte("already gone\n")),
	}), 0o644)

	install, err := prepareOrchestratorModuleInstall(configDir, modules)
	if err != nil {
		t.Fatalf("prepare: %v", err)
	}
	if err := install.stage(); err != nil {
		t.Fatalf("stage: %v", err)
	}
	assertModuleFile(t, module("memory"), "owned old memory\n", 0o644)
	if err := install.finalize(); err != nil {
		t.Fatalf("finalize: %v", err)
	}
	if _, err := os.Lstat(module("memory")); !os.IsNotExist(err) {
		t.Fatalf("owned unused module survived finalize: %v", err)
	}
	assertModuleFile(t, module("tracking"), "user edited tracking\n", 0o644)
	assertModuleFile(t, module("prompts"), "user prompts\n", 0o644)
	assertModuleFile(t, notes, "user notes\n", 0o644)
	assertModuleFile(t, ledger, moduleLedgerJSON(moduleHashes(modules)), 0o644)
}

func TestOrchestratorModuleInstallRestoreUndoesStageAndFinalize(t *testing.T) {
	t.Run("existing tree", func(t *testing.T) {
		base, configDir, dir, modules := moduleInstallFixture(t)
		writeModuleTestFile(t, filepath.Join(dir, modules[0].file), "previous release\n", 0o600)
		writeModuleTestFile(t, filepath.Join(dir, "orchestrator-memory.md"), "owned old memory\n", 0o644)
		writeModuleTestFile(t, filepath.Join(dir, orchestratorModuleLedgerName), moduleLedgerJSON(map[string]string{
			modules[0].file:          managedownership.Hash([]byte("previous release\n")),
			"orchestrator-memory.md": managedownership.Hash([]byte("owned old memory\n")),
		}), 0o600)
		before := snapshotModuleTree(t, base)
		if err := runModuleInstall(t, configDir, modules).restore(); err != nil {
			t.Fatalf("restore: %v", err)
		}
		if after := snapshotModuleTree(t, base); !reflect.DeepEqual(before, after) {
			t.Fatalf("restore did not reinstate before-images:\nbefore %v\nafter  %v", before, after)
		}
	})
	t.Run("fresh tree", func(t *testing.T) {
		_, configDir, dir, modules := moduleInstallFixture(t)
		if err := runModuleInstall(t, configDir, modules).restore(); err != nil {
			t.Fatalf("restore: %v", err)
		}
		// Directories are not journaled: restore removes created files only.
		if entries, err := os.ReadDir(dir); err != nil || len(entries) != 0 {
			t.Fatalf("restore left %v (%v), want an empty module directory", entries, err)
		}
	})
	t.Run("failed stage", func(t *testing.T) {
		_, configDir, dir, modules := moduleInstallFixture(t)
		owned, ledger := filepath.Join(dir, modules[0].file), filepath.Join(dir, orchestratorModuleLedgerName)
		writeModuleTestFile(t, owned, "previous release\n", 0o600)
		ledgerBody := moduleLedgerJSON(map[string]string{modules[0].file: managedownership.Hash([]byte("previous release\n"))})
		writeModuleTestFile(t, ledger, ledgerBody, 0o644)
		install, err := prepareOrchestratorModuleInstall(configDir, modules)
		if err != nil {
			t.Fatalf("prepare: %v", err)
		}
		late := filepath.Join(dir, modules[2].file)
		writeModuleTestFile(t, late, "user file created after preflight\n", 0o644)
		if err := install.stage(); err == nil || !strings.Contains(err.Error(), "mutation journal conflict") {
			t.Fatalf("stage = %v, want a journal conflict on %s", err, late)
		}
		if err := install.restore(); err != nil {
			t.Fatalf("restore: %v", err)
		}
		assertModuleFile(t, owned, "previous release\n", 0o600)
		assertModuleFile(t, late, "user file created after preflight\n", 0o644)
		assertModuleFile(t, ledger, ledgerBody, 0o644)
		if _, err := os.Lstat(filepath.Join(dir, modules[1].file)); !os.IsNotExist(err) {
			t.Fatalf("restore kept a staged module: %v", err)
		}
	})
}

// TestOrchestratorModuleInstallClassifiesCapturedBytes changes one file after
// prepare checked it and before the journal captured it. Ownership must come
// from the captured bytes, the image every later compare-and-swap uses, so a
// user edit in that window is never overwritten, retired or re-owned.
func TestOrchestratorModuleInstallClassifiesCapturedBytes(t *testing.T) {
	const edit = "user edit inside the preflight window\n"
	ownedLedger := func(file string) string {
		return moduleLedgerJSON(map[string]string{file: managedownership.Hash([]byte("previous release\n"))})
	}
	tests := []struct {
		name, target string
		arrange      func(t *testing.T, dir string, modules []orchestratorModule)
		want         string // prepare error; empty when the plan proceeds
	}{
		{"owned module", "orchestrator-delegation.md", func(t *testing.T, dir string, _ []orchestratorModule) {
			writeModuleTestFile(t, filepath.Join(dir, "orchestrator-delegation.md"), "previous release\n", 0o644)
			writeModuleTestFile(t, filepath.Join(dir, orchestratorModuleLedgerName), ownedLedger("orchestrator-delegation.md"), 0o644)
		}, "modified or not owned"},
		{"absent module", "orchestrator-delegation.md", func(*testing.T, string, []orchestratorModule) {}, "modified or not owned"},
		{"identical module", "orchestrator-delegation.md", func(t *testing.T, dir string, modules []orchestratorModule) {
			writeModuleTestFile(t, filepath.Join(dir, modules[0].file), modules[0].content, 0o644)
		}, "modified or not owned"},
		{"ledger", orchestratorModuleLedgerName, func(t *testing.T, dir string, _ []orchestratorModule) {
			writeModuleTestFile(t, filepath.Join(dir, "orchestrator-delegation.md"), "previous release\n", 0o644)
			writeModuleTestFile(t, filepath.Join(dir, orchestratorModuleLedgerName), ownedLedger("orchestrator-delegation.md"), 0o644)
		}, "changed during preflight"},
		{"owned unused module", "orchestrator-memory.md", func(t *testing.T, dir string, _ []orchestratorModule) {
			writeModuleTestFile(t, filepath.Join(dir, "orchestrator-memory.md"), "previous release\n", 0o644)
			writeModuleTestFile(t, filepath.Join(dir, orchestratorModuleLedgerName), ownedLedger("orchestrator-memory.md"), 0o644)
		}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			base, configDir, dir, modules := moduleInstallFixture(t)
			tt.arrange(t, dir, modules)
			target := filepath.Join(dir, tt.target)
			body := edit
			if tt.target == orchestratorModuleLedgerName {
				body = moduleLedgerJSON(map[string]string{})
			}
			var edited map[string]string
			install, err := prepareObservedOrchestratorModuleInstall(configDir, modules, func(path string) {
				if path == target {
					writeModuleTestFile(t, target, body, 0o644)
					edited = snapshotModuleTree(t, base)
				}
			})
			if tt.want != "" {
				if err == nil || !strings.Contains(err.Error(), tt.want) || install != nil {
					t.Fatalf("prepare plan=%t err=%v; want nil plan and error containing %q", install != nil, err, tt.want)
				}
				if after := snapshotModuleTree(t, base); !reflect.DeepEqual(edited, after) {
					t.Fatalf("rejected prepare changed the edited tree:\nedited %v\nafter  %v", edited, after)
				}
				return
			}
			if err != nil {
				t.Fatalf("prepare: %v", err)
			}
			if err := install.stage(); err != nil {
				t.Fatalf("stage: %v", err)
			}
			if err := install.finalize(); err != nil {
				t.Fatalf("finalize: %v", err)
			}
			assertModuleFile(t, target, edit, 0o644)
			assertModuleFile(t, filepath.Join(dir, orchestratorModuleLedgerName), moduleLedgerJSON(moduleHashes(modules)), 0o644)
		})
	}
}

func TestOrchestratorModuleInstallStageRejectsEditedAdoption(t *testing.T) {
	_, configDir, dir, modules := moduleInstallFixture(t)
	adopted := filepath.Join(dir, modules[0].file)
	writeModuleTestFile(t, adopted, modules[0].content, 0o644)
	install, err := prepareOrchestratorModuleInstall(configDir, modules)
	if err != nil {
		t.Fatalf("prepare: %v", err)
	}
	writeModuleTestFile(t, adopted, "user edit after prepare\n", 0o644)
	if err := install.stage(); err == nil || !strings.Contains(err.Error(), "mutation journal conflict") {
		t.Fatalf("stage = %v, want a journal conflict on the edited adopted module", err)
	}
	assertModuleFile(t, adopted, "user edit after prepare\n", 0o644)
}
