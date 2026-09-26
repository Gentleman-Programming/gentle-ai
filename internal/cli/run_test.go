package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v3/internal/agents/opencode"
	"github.com/gentleman-programming/gentle-ai/v3/internal/assets"
	"github.com/gentleman-programming/gentle-ai/v3/internal/backup"
	"github.com/gentleman-programming/gentle-ai/v3/internal/components/telemetryruntime"
	"github.com/gentleman-programming/gentle-ai/v3/internal/model"
	"github.com/gentleman-programming/gentle-ai/v3/internal/pipeline"
	"github.com/gentleman-programming/gentle-ai/v3/internal/planner"
	"github.com/gentleman-programming/gentle-ai/v3/internal/telemetry"
)

// themeSettingsFixture puts the effective JSONC in the project and a distinct
// user-owned JSON at the adapter's default global path.
func themeSettingsFixture(t *testing.T) (home, workspace, selected, decoy string, original []byte) {
	t.Helper()
	home, workspace = t.TempDir(), t.TempDir()
	setOpenCodeTestHome(t, home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("OPENCODE_CONFIG_DIR", "")
	if err := os.Mkdir(filepath.Join(workspace, ".git"), 0700); err != nil {
		t.Fatal(err)
	}
	selected = filepath.Join(workspace, "opencode.jsonc")
	decoy = opencode.NewAdapter().SettingsPath(home)
	original = []byte("// project settings\n{\"theme\":\"original\",\"user\":true}\n")
	if err := os.WriteFile(selected, original, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(decoy), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(decoy, []byte(`{"theme":"user-owned"}`), 0600); err != nil {
		t.Fatal(err)
	}
	if got := effectiveOpenCodeSettingsPath(home, workspace, ScopeGlobal, opencode.NewAdapter()); got != selected {
		t.Fatalf("effective path = %q, want %q", got, selected)
	}
	return
}

func TestInstallOpenCodeSettingsWritersUseSelectedJSONC(t *testing.T) {
	for _, component := range []model.ComponentID{model.ComponentPersona, model.ComponentPermission, model.ComponentContext7} {
		t.Run(string(component), func(t *testing.T) {
			home, workspace, selected, decoy, before := themeSettingsFixture(t)
			selection := model.Selection{Agents: []model.AgentID{model.AgentOpenCode}, Components: []model.ComponentID{component}, Persona: model.PersonaGentleman}
			resolved := planner.ResolvedPlan{Agents: selection.Agents, OrderedComponents: selection.Components}
			paths, err := backupTargets(home, workspace, ScopeGlobal, selection, resolved)
			if err != nil || !slices.Contains(paths, selected) {
				t.Fatalf("backup targets = %v, %v; selected missing", paths, err)
			}
			step := componentApplyStep{component: component, homeDir: home, workspaceDir: workspace, scope: ScopeGlobal, agents: selection.Agents, selection: selection}
			if err := step.Run(); err != nil {
				t.Fatal(err)
			}
			assertOpenCodeComponentSelectedOnly(t, selected, decoy, before, component)
		})
	}
}

func assertOpenCodeComponentSelectedOnly(t *testing.T, selected, decoy string, before []byte, component model.ComponentID) {
	t.Helper()
	data, err := os.ReadFile(selected)
	if err != nil || bytes.Equal(data, before) || !bytes.Contains(data, []byte("// project settings")) || !bytes.Contains(data, []byte(`"user":true`)) {
		t.Fatalf("%s selected settings = %s, %v", component, data, err)
	}
	data, err = os.ReadFile(decoy)
	if err != nil || string(data) != `{"theme":"user-owned"}` {
		t.Fatalf("%s changed decoy settings: %s, %v", component, data, err)
	}
}

func TestInstallEngramUsesSelectedOpenCodeJSONC(t *testing.T) {
	home, workspace, selected, decoy, before := themeSettingsFixture(t)
	stubEngramLookPath(t, home)
	selection := model.Selection{Agents: []model.AgentID{model.AgentOpenCode}, Components: []model.ComponentID{model.ComponentEngram}}
	targets, err := backupTargets(home, workspace, ScopeGlobal, selection, planner.ResolvedPlan{Agents: selection.Agents, OrderedComponents: selection.Components})
	if err != nil || !slices.Contains(targets, selected) {
		t.Fatalf("engram backup targets = %v, %v", targets, err)
	}
	step := componentApplyStep{component: model.ComponentEngram, homeDir: home, workspaceDir: workspace, scope: ScopeGlobal, agents: selection.Agents, selection: selection}
	if err := step.Run(); err != nil {
		t.Fatal(err)
	}
	assertOpenCodeComponentSelectedOnly(t, selected, decoy, before, model.ComponentEngram)
}

// TestInstallOpenCodeSettingsWritersWorkspaceScope proves a workspace-scoped
// install still writes the one settings document OpenCode loads (the effective
// global authority) and never strands <workspace>/.config/opencode/opencode.json,
// which OpenCode never reads (issue #1825).
func TestInstallOpenCodeSettingsWritersWorkspaceScope(t *testing.T) {
	components := []model.ComponentID{model.ComponentPersona, model.ComponentPermission, model.ComponentContext7, model.ComponentTheme, model.ComponentEngram}
	for _, component := range components {
		for _, projectFile := range []bool{true, false} {
			name := string(component) + "/home-settings"
			if projectFile {
				name = string(component) + "/project-jsonc"
			}
			t.Run(name, func(t *testing.T) {
				home, workspace, selected, decoy, before := themeSettingsFixture(t)
				loaded := selected
				if !projectFile {
					if err := os.Remove(selected); err != nil {
						t.Fatal(err)
					}
					loaded = decoy
					var err error
					if before, err = os.ReadFile(loaded); err != nil {
						t.Fatal(err)
					}
					if got := effectiveOpenCodeSettingsPath(home, workspace, ScopeGlobal, opencode.NewAdapter()); got != loaded {
						t.Fatalf("effective path = %q, want home settings %q", got, loaded)
					}
				}
				if component == model.ComponentEngram {
					stubEngramLookPath(t, home)
				}
				stranded := opencode.NewAdapter().SettingsPath(workspace)
				selection := model.Selection{Agents: []model.AgentID{model.AgentOpenCode}, Components: []model.ComponentID{component}, Persona: model.PersonaGentleman}
				paths, err := backupTargets(home, workspace, ScopeWorkspace, selection, planner.ResolvedPlan{Agents: selection.Agents, OrderedComponents: selection.Components})
				if err != nil || !slices.Contains(paths, loaded) || slices.Contains(paths, stranded) {
					t.Fatalf("workspace backup targets = %v, %v; want %q and not %q", paths, err, loaded, stranded)
				}
				if declared := componentPathsWithWorkspaceScoped(home, workspace, ScopeWorkspace, selection, resolveAdapters(selection.Agents), component); slices.Contains(declared, stranded) {
					t.Fatalf("%s declares stranded workspace settings: %v", component, declared)
				}
				step := componentApplyStep{component: component, homeDir: home, workspaceDir: workspace, scope: ScopeWorkspace, agents: selection.Agents, selection: selection}
				if err := step.Run(); err != nil {
					t.Fatal(err)
				}
				if projectFile {
					assertOpenCodeComponentSelectedOnly(t, loaded, decoy, before, component)
				} else if got, err := os.ReadFile(loaded); err != nil || bytes.Equal(got, before) {
					t.Fatalf("%s home settings = %s, %v; want a write", component, got, err)
				}
				if _, err := os.Stat(stranded); !os.IsNotExist(err) {
					t.Fatalf("%s stranded a settings document OpenCode never loads at %s (stat err = %v)", component, stranded, err)
				}
			})
		}
	}
}

// stubEngramLookPath resolves engram to a path that is never executed, with
// engram setup disabled, so the Engram component only merges its settings.
func stubEngramLookPath(t *testing.T, home string) {
	t.Helper()
	t.Setenv("GENTLE_AI_ENGRAM_SETUP_MODE", "off")
	originalLookPath := cmdLookPath
	cmdLookPath = func(name string) (string, error) {
		if name == "engram" {
			return filepath.Join(home, "test-engram-not-executed"), nil
		}
		return originalLookPath(name)
	}
	t.Cleanup(func() { cmdLookPath = originalLookPath })
}

func TestInstallThemeUsesSelectedOpenCodeJSONC(t *testing.T) {
	home, workspace, selected, decoy, _ := themeSettingsFixture(t)
	selection := model.Selection{Agents: []model.AgentID{model.AgentOpenCode}, Components: []model.ComponentID{model.ComponentTheme}}
	resolved := planner.ResolvedPlan{Agents: selection.Agents, OrderedComponents: selection.Components}
	paths, err := backupTargets(home, workspace, ScopeGlobal, selection, resolved)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(paths, selected) {
		t.Fatalf("install backup omits selected JSONC: %v", paths)
	}
	if slices.Contains(componentPathsWithWorkspaceScoped(home, workspace, ScopeGlobal, selection, resolveAdapters(selection.Agents), model.ComponentTheme), decoy) {
		t.Fatal("theme declares decoy JSON as its write target")
	}
	step := componentApplyStep{component: model.ComponentTheme, homeDir: home, workspaceDir: workspace, scope: ScopeGlobal, agents: selection.Agents}
	if err := step.Run(); err != nil {
		t.Fatal(err)
	}
	assertThemeSelectedOnly(t, selected, decoy)
}

type failAfterOpenCodeSettingsStep struct {
	selected string
	before   []byte
	cause    error
}

func (s failAfterOpenCodeSettingsStep) ID() string { return "test:fail-after-settings" }
func (s failAfterOpenCodeSettingsStep) Run() error {
	data, err := os.ReadFile(s.selected)
	if err != nil || bytes.Equal(data, s.before) {
		return errors.New("selected settings were not written before failure")
	}
	return s.cause
}

func TestInstallOpenCodeSettingsWritersRollbackSelectedJSONC(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX mode assertions do not apply on Windows")
	}
	for _, component := range []model.ComponentID{model.ComponentPersona, model.ComponentPermission, model.ComponentContext7} {
		t.Run(string(component), func(t *testing.T) {
			home, workspace, selected, decoy, before := themeSettingsFixture(t)
			selection := model.Selection{Agents: []model.AgentID{model.AgentOpenCode}, Components: []model.ComponentID{component}, Persona: model.PersonaGentleman}
			targets, err := backupTargets(home, workspace, ScopeGlobal, selection, planner.ResolvedPlan{Agents: selection.Agents, OrderedComponents: selection.Components})
			if err != nil || !slices.Contains(targets, selected) {
				t.Fatalf("snapshot targets = %v, %v", targets, err)
			}
			state := &runtimeState{}
			cause := errors.New("injected failure after settings write")
			plan := pipeline.StagePlan{
				Prepare: []pipeline.Step{prepareBackupStep{id: "prepare:backup-snapshot", snapshotter: backup.NewSnapshotter(), snapshotDir: filepath.Join(home, "backup"), targets: []string{selected}, state: state}},
				Apply: []pipeline.Step{
					rollbackRestoreStep{id: "apply:rollback-restore", state: state, homeDir: home, workspaceDir: workspace},
					componentApplyStep{component: component, homeDir: home, workspaceDir: workspace, scope: ScopeGlobal, agents: selection.Agents, selection: selection},
					failAfterOpenCodeSettingsStep{selected: selected, before: before, cause: cause},
				},
			}
			result := pipeline.NewOrchestrator(pipeline.DefaultRollbackPolicy()).Execute(plan)
			if !errors.Is(result.Err, cause) || !result.Rollback.Success {
				t.Fatalf("failure = %v, rollback = %#v", result.Err, result.Rollback)
			}
			got, err := os.ReadFile(selected)
			if err != nil || !bytes.Equal(got, before) {
				t.Fatalf("restored bytes = %s, %v", got, err)
			}
			info, err := os.Stat(selected)
			if err != nil || info.Mode().Perm() != 0600 {
				t.Fatalf("restored mode = %v, %v", info, err)
			}
			got, err = os.ReadFile(decoy)
			if err != nil || string(got) != `{"theme":"user-owned"}` {
				t.Fatalf("decoy = %s, %v", got, err)
			}
		})
	}
}

type failAfterThemeStep struct {
	selected string
	observed *bool
	cause    error
}

func (s failAfterThemeStep) ID() string { return "test:fail-after-theme" }
func (s failAfterThemeStep) Run() error {
	data, err := os.ReadFile(s.selected)
	if err != nil || !bytes.Contains(data, []byte(`"theme":"gentleman"`)) {
		return errors.New("theme write was not observed before failure")
	}
	*s.observed = true
	return s.cause
}

func TestInstallThemeRollbackRestoresSelectedJSONCBytesAndMode(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows does not preserve POSIX file modes")
	}
	home, workspace, selected, decoy, before := themeSettingsFixture(t)
	selection := model.Selection{Agents: []model.AgentID{model.AgentOpenCode}, Components: []model.ComponentID{model.ComponentTheme}}
	resolved := planner.ResolvedPlan{Agents: selection.Agents, OrderedComponents: selection.Components}
	targets, err := backupTargets(home, workspace, ScopeGlobal, selection, resolved)
	if err != nil || !slices.Contains(targets, selected) {
		t.Fatalf("theme backup targets = %v, %v", targets, err)
	}
	decoyBefore, err := os.ReadFile(decoy)
	if err != nil {
		t.Fatal(err)
	}
	state := &runtimeState{}
	cause := errors.New("forced failure after theme write")
	observed := false
	plan := pipeline.StagePlan{
		Prepare: []pipeline.Step{prepareBackupStep{
			id: "prepare:backup-snapshot", snapshotter: backup.NewSnapshotter(),
			snapshotDir: filepath.Join(home, "theme-backup"), targets: []string{selected}, state: state,
		}},
		Apply: []pipeline.Step{
			rollbackRestoreStep{id: "apply:rollback-restore", state: state, homeDir: home, workspaceDir: workspace},
			componentApplyStep{component: model.ComponentTheme, homeDir: home, workspaceDir: workspace, scope: ScopeGlobal, agents: selection.Agents},
			failAfterThemeStep{selected: selected, observed: &observed, cause: cause},
		},
	}
	result := pipeline.NewOrchestrator(pipeline.DefaultRollbackPolicy()).Execute(plan)
	if !errors.Is(result.Err, cause) || !observed || !result.Rollback.Success {
		t.Fatalf("post-theme failure = %v, observed=%t, rollback=%#v", result.Err, observed, result.Rollback)
	}
	if len(result.Apply.Steps) != 3 || result.Apply.Steps[1].Status != pipeline.StepStatusSucceeded || result.Apply.Steps[2].Status != pipeline.StepStatusFailed {
		t.Fatalf("theme must succeed before injected failure: %#v", result.Apply.Steps)
	}
	got, err := os.ReadFile(selected)
	if err != nil || !bytes.Equal(got, before) {
		t.Fatalf("rollback JSONC bytes = %q, %v; want %q", got, err, before)
	}
	info, err := os.Stat(selected)
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("rollback JSONC mode = %v, %v; want 0600", info, err)
	}
	got, err = os.ReadFile(decoy)
	if err != nil || !bytes.Equal(got, decoyBefore) {
		t.Fatalf("decoy JSON changed: %q, %v", got, err)
	}
}

func assertThemeSelectedOnly(t *testing.T, selected, decoy string) {
	t.Helper()
	data, err := os.ReadFile(selected)
	if err != nil || !bytes.Contains(data, []byte(`"theme":"gentleman"`)) || !bytes.Contains(data, []byte(`"user":true`)) || !bytes.Contains(data, []byte("// project settings")) {
		t.Fatalf("selected JSONC theme not installed: %s, %v", data, err)
	}
	data, err = os.ReadFile(decoy)
	if err != nil || string(data) != `{"theme":"user-owned"}` {
		t.Fatalf("decoy JSON changed: %s, %v", data, err)
	}
}

func TestOpenCodeTelemetryRollbackPreservesLateEdits(t *testing.T) {
	for _, flow := range []string{"install", "sync"} {
		for _, existing := range []bool{false, true} {
			for _, edit := range []string{"plugin", "manifest", "both", "mode", "symlink", "parent-symlink"} {
				t.Run(fmtRollbackCase(flow, existing, edit), func(t *testing.T) {
					home := t.TempDir()
					setOpenCodeTestHome(t, home)
					t.Setenv("XDG_CONFIG_HOME", t.TempDir())
					if edit == "mode" && runtime.GOOS == "windows" {
						t.Skip("Windows does not preserve POSIX chmod mode mutations")
					}
					adapter := opencode.NewAdapter()
					config := adapter.GlobalConfigDir(home)
					paths := telemetryruntime.ManagedPaths(config)
					before := make([][]byte, 2)
					if existing {
						if _, err := telemetryruntime.Reconcile(config); err != nil {
							t.Fatal(err)
						}
						// A prior valid compact manifest requires a real metadata refresh.
						raw, _ := os.ReadFile(paths[1])
						var compact bytes.Buffer
						if err := json.Compact(&compact, raw); err != nil {
							t.Fatal(err)
						}
						if err := os.WriteFile(paths[1], compact.Bytes(), 0600); err != nil {
							t.Fatal(err)
						}
						for i, path := range paths {
							before[i], _ = os.ReadFile(path)
						}
					}
					unrelated := adapter.SystemPromptFile(home)
					if err := os.MkdirAll(filepath.Dir(unrelated), 0700); err != nil {
						t.Fatal(err)
					}
					if err := os.WriteFile(unrelated, []byte("before"), 0600); err != nil {
						t.Fatal(err)
					}
					selection := model.Selection{Agents: []model.AgentID{model.AgentOpenCode}}
					var plan pipeline.StagePlan
					if flow == "install" {
						rt := &installRuntime{homeDir: home, workspaceDir: t.TempDir(), backupRoot: filepath.Join(home, "backups"), scope: ScopeGlobal, selection: selection, resolved: planner.ResolvedPlan{Agents: selection.Agents}, state: &runtimeState{}}
						plan = rt.stagePlan()
					} else {
						rt, err := newSyncRuntime(home, selection)
						if err != nil {
							t.Fatal(err)
						}
						plan = rt.stagePlan()
					}
					for _, step := range plan.Prepare {
						if snapshot, ok := step.(prepareBackupStep); ok {
							snapshot.targets = append(snapshot.targets, unrelated)
							if err := snapshot.Run(); err != nil {
								t.Fatal(err)
							}
						}
					}
					for _, step := range plan.Apply {
						if strings.HasSuffix(step.ID(), "opencode:telemetry-runtime") {
							if err := step.Run(); err != nil {
								t.Fatal(err)
							}
						}
					}
					if err := os.WriteFile(unrelated, []byte("pipeline write"), 0600); err != nil {
						t.Fatal(err)
					}
					edited := map[int]bool{}
					if edit == "plugin" || edit == "both" {
						edited[0] = true
					}
					if edit == "manifest" || edit == "both" {
						edited[1] = true
					}
					for i := range edited {
						if err := os.WriteFile(paths[i], []byte("custom late edit"), 0600); err != nil {
							t.Fatal(err)
						}
					}
					if edit == "mode" {
						edited[0] = true
						if err := os.Chmod(paths[0], 0600); err != nil {
							t.Fatal(err)
						}
					}
					if edit == "symlink" || edit == "parent-symlink" {
						edited[0] = true
						source := paths[0]
						if edit == "parent-symlink" {
							source = filepath.Dir(source)
						}
						target := filepath.Join(config, "moved-plugin")
						if err := os.Rename(source, target); err != nil {
							t.Fatal(err)
						}
						if err := os.Symlink(target, source); err != nil {
							t.Skip(err)
						}
					}
					var rollbackErr error
					for _, step := range plan.Apply {
						if restore, ok := step.(rollbackRestoreStep); ok {
							rollbackErr = restore.Rollback()
						}
					}
					if rollbackErr == nil {
						t.Error("late edit was not a rollback conflict")
					}
					for i, path := range paths {
						data, err := os.ReadFile(path)
						if edited[i] {
							if err != nil {
								t.Errorf("edited file removed: %s", path)
								continue
							}
							if edit == "mode" {
								info, _ := os.Lstat(path)
								if info.Mode().Perm() != 0600 {
									t.Error("edited mode overwritten")
								}
							} else if edit == "symlink" || edit == "parent-symlink" {
								link := path
								if edit == "parent-symlink" {
									link = filepath.Dir(path)
								}
								info, _ := os.Lstat(link)
								if info.Mode()&os.ModeSymlink == 0 {
									t.Error("symlink replaced")
								}
							} else if string(data) != "custom late edit" {
								t.Error("edited bytes overwritten")
							}
						} else if existing {
							if err != nil || !bytes.Equal(data, before[i]) {
								t.Error("unaffected pair member not restored")
							}
						} else if !os.IsNotExist(err) {
							t.Error("unaffected new pair member remains")
						}
					}
					if data, _ := os.ReadFile(unrelated); string(data) != "before" {
						t.Error("safe unrelated rollback did not complete")
					}
				})
			}
		}
	}
}
func fmtRollbackCase(flow string, existing bool, edit string) string {
	if existing {
		return flow + "/existing/" + edit
	}
	return flow + "/fresh/" + edit
}

// setOpenCodeTestHome keeps XDG resolution bound to the test home on Windows,
// where os.UserHomeDir reads USERPROFILE rather than HOME.
func setOpenCodeTestHome(t *testing.T, home string) {
	t.Helper()
	t.Setenv("HOME", home)
	if runtime.GOOS == "windows" {
		t.Setenv("USERPROFILE", home)
	}
}

func TestOpenCodeTelemetryInstallRollbackOutsideHome(t *testing.T) {
	home := t.TempDir()
	setOpenCodeTestHome(t, home)
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	agents := []model.AgentID{model.AgentOpenCode}
	rt := &installRuntime{homeDir: home, workspaceDir: t.TempDir(), backupRoot: filepath.Join(home, "backups"), scope: ScopeGlobal, selection: model.Selection{Agents: agents}, resolved: planner.ResolvedPlan{Agents: agents}, state: &runtimeState{}}
	plan := rt.stagePlan()
	for _, step := range plan.Prepare {
		if _, ok := step.(prepareBackupStep); ok {
			if err := step.Run(); err != nil {
				t.Fatal(err)
			}
		}
	}
	for _, step := range plan.Apply {
		if step.ID() == "opencode:telemetry-runtime" {
			if err := step.Run(); err != nil {
				t.Fatal(err)
			}
		}
	}
	for _, step := range plan.Apply {
		if restore, ok := step.(rollbackRestoreStep); ok {
			if err := restore.Rollback(); err != nil {
				t.Fatal(err)
			}
		}
	}
	for _, path := range telemetryruntime.ManagedPaths(opencode.NewAdapter().GlobalConfigDir(home)) {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatal("rollback left new runtime artifact", path, err)
		}
	}
}

func TestOpenCodeTelemetryInstallRefusesCustomBeforeSnapshot(t *testing.T) {
	home := t.TempDir()
	setOpenCodeTestHome(t, home)
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	agents := []model.AgentID{model.AgentOpenCode}
	path := telemetryruntime.ManagedPaths(opencode.NewAdapter().GlobalConfigDir(home))[0]
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("custom"), 0600); err != nil {
		t.Fatal(err)
	}
	rt := &installRuntime{homeDir: home, workspaceDir: t.TempDir(), scope: ScopeGlobal, selection: model.Selection{Agents: agents}, resolved: planner.ResolvedPlan{Agents: agents}, state: &runtimeState{}}
	plan := rt.stagePlan()
	if err := plan.Prepare[0].Run(); err == nil {
		t.Fatal("custom plugin not refused before snapshot")
	}
	if data, err := os.ReadFile(path); err != nil || string(data) != "custom" {
		t.Fatal("custom file changed", err)
	}
}

func TestOpenCodeTelemetryOrdinaryInstall(t *testing.T) {
	for _, selected := range []bool{true, false} {
		t.Run(map[bool]string{true: "selected", false: "absent"}[selected], func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg"))
			t.Setenv("DO_NOT_TRACK", "1")
			if err := telemetry.Save(home, telemetry.State{InstallID: "existing", Enabled: false, NoticeShown: true}); err != nil {
				t.Fatal(err)
			}
			before, _ := os.ReadFile(telemetry.Path(home))
			agents := []model.AgentID{model.AgentClaudeCode}
			if selected {
				agents = []model.AgentID{model.AgentOpenCode}
			}
			selection := model.Selection{Agents: agents}
			resolved := planner.ResolvedPlan{Agents: agents}
			rt := &installRuntime{homeDir: home, workspaceDir: t.TempDir(), scope: ScopeGlobal, selection: selection, resolved: resolved, state: &runtimeState{}}
			plan := rt.stagePlan()
			// Execute the ordinary plan's runtime-asset step, not an installer or an SDD helper.
			found := false
			for _, step := range plan.Apply {
				if step.ID() == "opencode:telemetry-runtime" {
					found = true
					if err := step.Run(); err != nil {
						t.Fatal(err)
					}
				}
			}
			if found != selected {
				t.Fatalf("runtime step present=%v selected=%v", found, selected)
			}
			path := filepath.Join(opencode.NewAdapter().GlobalConfigDir(home), "plugins", "telemetry-runtime.ts")
			data, err := os.ReadFile(path)
			if selected {
				if err != nil || string(data) != assets.MustRead("opencode/plugins/telemetry-runtime.ts") {
					t.Fatal("missing embedded runtime asset", err)
				}
				paths, err := backupTargets(home, rt.workspaceDir, ScopeGlobal, selection, resolved)
				if err != nil || !slices.Contains(paths, path) || !slices.Contains(paths, filepath.Join(filepath.Dir(filepath.Dir(path)), ".gentle-ai-telemetry-runtime.json")) {
					t.Fatal("runtime pair absent from install backup", err)
				}
			} else if !os.IsNotExist(err) {
				t.Fatal("OpenCode absent but plugin installed")
			}
			after, _ := os.ReadFile(telemetry.Path(home))
			if string(before) != string(after) {
				t.Fatal("installation changed telemetry opt-out")
			}
		})
	}
}
