package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v3/internal/components/communitytool"
	"github.com/gentleman-programming/gentle-ai/v3/internal/model"
	"github.com/gentleman-programming/gentle-ai/v3/internal/planner"
	"github.com/gentleman-programming/gentle-ai/v3/internal/state"
	"github.com/gentleman-programming/gentle-ai/v3/internal/statecoord"
	"github.com/gentleman-programming/gentle-ai/v3/internal/system"
)

// convergenceTestHome prepares an isolated home where every runtime binary
// resolves as installed, so install and sync run the same managed steps a real
// user machine would.
func convergenceTestHome(t *testing.T) string {
	t.Helper()
	home := installTestHome(t)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	binDir := filepath.Join(home, "bin")
	cmdLookPath = func(name string) (string, error) { return filepath.Join(binDir, name), nil }
	return home
}

// snapshotManagedTree reads every file under root except install state, whose
// sync timestamp is expected to move, and backup snapshots.
func snapshotManagedTree(t *testing.T, root string) map[string]string {
	t.Helper()
	files := map[string]string{}
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, path)
		if d.IsDir() {
			if rel == filepath.Join(".gentle-ai", "backups") {
				return filepath.SkipDir
			}
			return nil
		}
		if rel == filepath.Join(".gentle-ai", "state.json") {
			return nil
		}
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		files[rel] = string(data)
		return nil
	})
	if err != nil {
		t.Fatalf("snapshot %s: %v", root, err)
	}
	return files
}

func assertSameManagedTree(t *testing.T, step string, before, after map[string]string) {
	t.Helper()
	for path, content := range after {
		prev, ok := before[path]
		switch {
		case !ok:
			t.Errorf("%s created %s", step, path)
		case prev != content:
			t.Errorf("%s rewrote %s\n--- before ---\n%s\n--- after ---\n%s", step, path, prev, content)
		}
	}
	for path := range before {
		if _, ok := after[path]; !ok {
			t.Errorf("%s removed %s", step, path)
		}
	}
}

// TestInstallThenSyncConverges pins that the first sync after a full install
// is a no-op: sync must apply components in install order, so persona does not
// replace a prompt after Engram wrote its protocol and MCP blocks keep their
// install layout.
func TestInstallThenSyncConverges(t *testing.T) {
	for _, agent := range []string{
		"claude-code", "opencode", "gemini-cli", "codex", "cursor", "kiro-ide",
		"kilocode", "hermes", "qwen-code", "vscode-copilot",
	} {
		t.Run(agent, func(t *testing.T) {
			home := convergenceTestHome(t)
			if _, err := RunInstall([]string{"--agent", agent, "--preset", "full-gentleman"}, system.DetectionResult{}); err != nil {
				t.Fatalf("RunInstall() error = %v", err)
			}
			installed := snapshotManagedTree(t, home)

			result, err := RunSync(nil)
			if err != nil {
				t.Fatalf("RunSync() error = %v", err)
			}
			assertSameManagedTree(t, "sync after install", installed, snapshotManagedTree(t, home))
			if result.FilesChanged != 0 {
				t.Errorf("sync after install changed %d files: %v", result.FilesChanged, result.ChangedFiles)
			}
		})
	}
}

func TestCodexReinstallKeepsConfigTOML(t *testing.T) {
	home := convergenceTestHome(t)
	args := []string{"--agent", "codex", "--preset", "full-gentleman"}
	if _, err := RunInstall(args, system.DetectionResult{}); err != nil {
		t.Fatalf("first RunInstall() error = %v", err)
	}
	configPath := filepath.Join(home, ".codex", "config.toml")
	first, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := RunInstall(args, system.DetectionResult{}); err != nil {
		t.Fatalf("second RunInstall() error = %v", err)
	}
	second, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(first) != string(second) {
		t.Fatalf("second install rewrote config.toml\n--- first ---\n%s\n--- second ---\n%s", first, second)
	}
}

// TestCodeGraphInstallThenSyncConverges covers the TUI install path, the only
// one that selects CodeGraph. Its guidance must be injected after persona so
// install keeps it, and sync must keep it even when persona replaces the
// prompt file earlier in the same run and no CodeGraph MCP wiring exists.
func TestCodeGraphInstallThenSyncConverges(t *testing.T) {
	for _, tt := range []struct {
		agent        model.AgentID
		wantGuidance bool
	}{
		{model.AgentClaudeCode, true}, {model.AgentGeminiCLI, true}, {model.AgentCodex, true},
		{model.AgentCursor, true}, {model.AgentKiroIDE, true}, {model.AgentHermes, true},
		{model.AgentKilocode, false}, {model.AgentQwenCode, false}, {model.AgentVSCodeCopilot, false},
	} {
		agent := tt.agent
		t.Run(string(agent), func(t *testing.T) {
			home := convergenceTestHome(t)
			previous := installCommunityToolWithHome
			t.Cleanup(func() { installCommunityToolWithHome = previous })
			installCommunityToolWithHome = func(id model.CommunityToolID, _, homeDir string, _ communitytool.Runner, _ communitytool.Detector) (communitytool.Result, error) {
				_, err := communitytool.InjectCodeGraphGuidanceIfSelected(homeDir, []model.CommunityToolID{id})
				return communitytool.Result{Tool: id}, err
			}

			input, err := NormalizeInstallFlags(InstallFlags{Agents: []string{string(agent)}, Preset: "full-gentleman"}, system.DetectionResult{})
			if err != nil {
				t.Fatal(err)
			}
			selection := input.Selection
			selection.CommunityTools = []model.CommunityToolID{model.CommunityToolCodeGraph}
			resolved, err := planner.NewResolver(planner.MVPGraph()).Resolve(selection)
			if err != nil {
				t.Fatal(err)
			}
			execution, _ := executeTUIInstallWithBackground(home, selection, resolved, ResolveInstallProfile(system.DetectionResult{}), "", "", nil)
			if execution.Err != nil {
				t.Fatalf("TUI install error = %v", execution.Err)
			}
			installState := state.InstallState{
				InstalledAgents:          []string{string(agent)},
				CommunityTools:           []string{string(model.CommunityToolCodeGraph)},
				CommunityToolsConfigured: true,
				Persona:                  string(selection.Persona),
			}
			installState.SetSelection(selection)
			// Persist state the way the TUI does after a successful install.
			if err := statecoord.WithLock(home, func() error { return state.WriteReconciled(home, installState) }); err != nil {
				t.Fatal(err)
			}
			if got := communitytool.HasManagedCodeGraphGuidance(home); got != tt.wantGuidance {
				t.Fatalf("CodeGraph guidance after install = %v, want %v", got, tt.wantGuidance)
			}
			installed := snapshotManagedTree(t, home)

			result, err := RunSync(nil)
			if err != nil {
				t.Fatalf("RunSync() error = %v", err)
			}
			assertSameManagedTree(t, "sync after CodeGraph install", installed, snapshotManagedTree(t, home))
			if result.FilesChanged != 0 {
				t.Errorf("sync after CodeGraph install changed %d files: %v", result.FilesChanged, result.ChangedFiles)
			}
		})
	}
}
