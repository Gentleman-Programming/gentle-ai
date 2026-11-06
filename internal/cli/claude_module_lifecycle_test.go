package cli

import (
	"errors"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v4/internal/components/agentguidance"
	"github.com/gentleman-programming/gentle-ai/v4/internal/model"
	"github.com/gentleman-programming/gentle-ai/v4/internal/planner"
)

// #5256 U2b-1: install and sync continue an installed user-global Claude
// module pilot instead of folding it back into the monolithic orchestrator.
// They never create one; only the global Claude scope looks for its ledger.

// seedClaudeModulePilot installs the pilot the way a pilot-enabled global
// install leaves it and returns the nine paths it plans: core, modules, ledger.
func seedClaudeModulePilot(t *testing.T, home string) []string {
	t.Helper()

	options := agentguidance.RoutingOptions{ClaudeGlobalModules: true, ReviewContract: routingReviewContract}
	if _, err := agentguidance.InjectRoutingWithOptions(home, model.AgentClaudeCode, options); err != nil {
		t.Fatalf("seed Claude module pilot: %v", err)
	}
	paths, err := agentguidance.ClaudeGlobalModulePaths(home)
	if err != nil || len(paths) != 9 {
		t.Fatalf("ClaudeGlobalModulePaths() = %v, %v; want the nine pilot paths", paths, err)
	}
	return paths
}

// changedPilotFile reports the first of paths whose snapshot entry differs,
// including a file that appeared or disappeared.
func changedPilotFile(t *testing.T, home string, paths []string, before, after map[string]string) string {
	t.Helper()

	for _, path := range paths {
		rel, err := filepath.Rel(home, path)
		if err != nil {
			t.Fatal(err)
		}
		if before[rel] != after[rel] {
			return path
		}
	}
	return ""
}

func claudeRoutingStep(home, workspace string, scope InstallScope, changed *[]string) agentRoutingGuidanceStep {
	return agentRoutingGuidanceStep{
		id:           "agent-guidance:" + string(model.AgentClaudeCode),
		agent:        model.AgentClaudeCode,
		homeDir:      home,
		workspaceDir: workspace,
		scope:        scope,
		changedFiles: changed,
	}
}

func TestClaudeInstalledModulePilotPlansNinePathsWithoutWriting(t *testing.T) {
	home := t.TempDir()
	want := seedClaudeModulePilot(t, home)
	before := snapshotTree(t, home)

	paths := routingGuidancePaths(home, "", ScopeGlobal, resolveAdapters([]model.AgentID{model.AgentClaudeCode}))

	if !slices.Equal(paths, want) {
		t.Fatalf("routingGuidancePaths(global Claude pilot) = %v, want the nine pilot paths %v", paths, want)
	}

	// Install and sync both snapshot through this planner before any write.
	selection := model.Selection{Agents: []model.AgentID{model.AgentClaudeCode}}
	install, err := backupTargets(home, "", ScopeGlobal, selection, planner.ResolvedPlan{Agents: selection.Agents})
	if err != nil {
		t.Fatalf("backupTargets() error = %v", err)
	}
	sync, err := syncBackupTargetsScoped(home, "", ScopeGlobal, selection, resolveAdapters(selection.Agents))
	if err != nil {
		t.Fatalf("syncBackupTargetsScoped() error = %v", err)
	}
	for _, path := range want {
		if !containsPath(install, path) || !containsPath(sync, path) {
			t.Fatalf("pilot path %q missing from backup targets\ninstall = %v\nsync = %v", path, install, sync)
		}
	}
	if after := snapshotTree(t, home); !maps.Equal(after, before) {
		t.Fatalf("planning the pilot paths changed the home tree")
	}
}

func TestClaudeInstalledModulePilotLeavesOtherRoutingUnchanged(t *testing.T) {
	prompt := func(t *testing.T, root string, agent model.AgentID) []string {
		return []string{systemPromptFileFor(t, root, agent)}
	}
	for _, tc := range []struct {
		name   string
		pilot  bool
		agents []model.AgentID
		scope  InstallScope
		seed   func(t *testing.T, home string)
		want   func(t *testing.T, home, workspace string) []string
	}{
		{
			name:   "global Claude without ledger",
			agents: []model.AgentID{model.AgentClaudeCode},
			scope:  ScopeGlobal,
			want:   func(t *testing.T, home, _ string) []string { return prompt(t, home, model.AgentClaudeCode) },
		},
		{
			name:   "global Claude with a user file where the module directory would be",
			agents: []model.AgentID{model.AgentClaudeCode},
			scope:  ScopeGlobal,
			seed: func(t *testing.T, home string) {
				mustWriteFile(t, filepath.Join(home, ".claude", "gentle-ai"), []byte("user file\n"))
			},
			want: func(t *testing.T, home, _ string) []string { return prompt(t, home, model.AgentClaudeCode) },
		},
		{
			name:   "workspace Claude beside an installed pilot",
			pilot:  true,
			agents: []model.AgentID{model.AgentClaudeCode},
			scope:  ScopeWorkspace,
			want:   func(t *testing.T, _, workspace string) []string { return prompt(t, workspace, model.AgentClaudeCode) },
		},
		{
			name:   "other global agents beside an installed pilot",
			pilot:  true,
			agents: []model.AgentID{model.AgentCodex, model.AgentGeminiCLI},
			scope:  ScopeGlobal,
			want: func(t *testing.T, home, _ string) []string {
				return append(prompt(t, home, model.AgentCodex), prompt(t, home, model.AgentGeminiCLI)...)
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			home := t.TempDir()
			workspace := t.TempDir()
			if tc.pilot {
				seedClaudeModulePilot(t, home)
			}
			if tc.seed != nil {
				tc.seed(t, home)
			}
			homeBefore := snapshotTree(t, home)

			paths := routingGuidancePaths(home, workspace, tc.scope, resolveAdapters(tc.agents))
			if want := tc.want(t, home, workspace); !slices.Equal(paths, want) {
				t.Fatalf("routingGuidancePaths() = %v, want unchanged default delivery %v", paths, want)
			}
			if !slices.Contains(tc.agents, model.AgentClaudeCode) {
				return
			}

			if err := claudeRoutingStep(home, workspace, tc.scope, nil).Run(); err != nil {
				t.Fatalf("Run() error = %v", err)
			}
			homeAfter := snapshotTree(t, home)
			for rel, entry := range homeBefore {
				if homeAfter[rel] != entry {
					t.Fatalf("routing step changed pre-existing %q: %q -> %q", rel, entry, homeAfter[rel])
				}
			}
			// The step continues a pilot but never creates one.
			moduleDir := filepath.Join(".claude", "gentle-ai", "orchestrator")
			if _, created := homeAfter[moduleDir]; created && !tc.pilot {
				t.Fatalf("routing step created module directory %q under home", moduleDir)
			}
			if _, err := os.Stat(filepath.Join(workspace, moduleDir)); !errors.Is(err, fs.ErrNotExist) {
				t.Fatalf("routing step created module directory %q under the workspace (stat err = %v)", moduleDir, err)
			}
		})
	}
}

func TestClaudeInstalledModulePilotSurvivesGlobalRoutingStep(t *testing.T) {
	home := t.TempDir()
	seedClaudeModulePilot(t, home)
	pilot := snapshotTree(t, filepath.Join(home, ".claude", "gentle-ai"))
	core := readTextFile(t, systemPromptFileFor(t, home, model.AgentClaudeCode))

	for _, run := range []string{"reinjection", "identical second run"} {
		var changed []string
		if err := claudeRoutingStep(home, "", ScopeGlobal, &changed).Run(); err != nil {
			t.Fatalf("%s: Run() error = %v", run, err)
		}
		if got := readTextFile(t, systemPromptFileFor(t, home, model.AgentClaudeCode)); got != core {
			t.Fatalf("%s rewrote the pilot core CLAUDE.md:\n%s", run, got)
		}
		if got := snapshotTree(t, filepath.Join(home, ".claude", "gentle-ai")); !maps.Equal(got, pilot) {
			t.Fatalf("%s changed the pilot modules or ledger", run)
		}
		if run == "identical second run" && len(changed) != 0 {
			t.Fatalf("identical second run reported changes: %v", changed)
		}
	}
}

func TestClaudeInstalledModulePilotFailsClosedBeforeAnyWrite(t *testing.T) {
	t.Run("modified referenced module", func(t *testing.T) {
		home := t.TempDir()
		paths := seedClaudeModulePilot(t, home)
		var module string
		for _, path := range paths[1 : len(paths)-1] {
			if _, err := os.Stat(path); err == nil {
				module = path
				break
			}
		}
		if module == "" {
			t.Fatal("seeded pilot wrote no module; the test proves nothing")
		}
		mustWriteFile(t, module, []byte(readTextFile(t, module)+"user edit\n"))
		before := snapshotTree(t, home)

		if err := claudeRoutingStep(home, "", ScopeGlobal, nil).Run(); err == nil {
			t.Fatal("Run() over a modified pilot module error = nil, want fail closed")
		}
		if changed := changedPilotFile(t, home, paths, before, snapshotTree(t, home)); changed != "" {
			t.Fatalf("failed routing step changed pilot file %q", changed)
		}
	})

	t.Run("uninspectable ledger", func(t *testing.T) {
		if runtime.GOOS == "windows" || os.Geteuid() == 0 {
			t.Skip("needs POSIX permissions enforced for the current user")
		}
		home := t.TempDir()
		paths := seedClaudeModulePilot(t, home)
		before := snapshotTree(t, home)
		dir := filepath.Dir(paths[len(paths)-1])
		info, err := os.Stat(dir)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(dir, 0); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = os.Chmod(dir, info.Mode().Perm()) })

		err = claudeRoutingStep(home, "", ScopeGlobal, nil).Run()
		planned := routingGuidancePaths(home, "", ScopeGlobal, resolveAdapters([]model.AgentID{model.AgentClaudeCode}))
		if chmodErr := os.Chmod(dir, info.Mode().Perm()); chmodErr != nil {
			t.Fatal(chmodErr)
		}
		if !errors.Is(err, fs.ErrPermission) {
			t.Fatalf("Run() with an uninspectable ledger error = %v, want permission failure", err)
		}
		if len(planned) != 0 {
			t.Fatalf("routingGuidancePaths() planned %v for a delivery the step refuses", planned)
		}
		if after := snapshotTree(t, home); !maps.Equal(after, before) {
			t.Fatal("failed routing step changed the home tree")
		}
	})

	t.Run("relative home", func(t *testing.T) {
		cwd := t.TempDir()
		t.Chdir(cwd)

		err := claudeRoutingStep("relative-home", "", ScopeGlobal, nil).Run()
		if !errors.Is(err, agentguidance.ErrInvalidTarget) {
			t.Fatalf("Run() with a relative home error = %v, want ErrInvalidTarget", err)
		}
		if entries, _ := os.ReadDir(cwd); len(entries) != 0 {
			t.Fatalf("failed routing step wrote relative to the working directory: %v", entries)
		}
	})
}
