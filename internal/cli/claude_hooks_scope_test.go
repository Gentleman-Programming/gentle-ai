package cli

import (
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v4/internal/model"
	"github.com/gentleman-programming/gentle-ai/v4/internal/pipeline"
	"github.com/gentleman-programming/gentle-ai/v4/internal/system"
)

type claudeHooksRollbackFailureStep struct{ run func() error }

func (claudeHooksRollbackFailureStep) ID() string   { return "test:fail-after-claude-hooks" }
func (s claudeHooksRollbackFailureStep) Run() error { return s.run() }

func TestInstallClaudeHooksRollbackWithoutPersona(t *testing.T) {
	for _, scope := range []InstallScope{ScopeWorkspace, ScopeGlobal} {
		for _, existing := range []bool{false, true} {
			name := string(scope) + "/new settings"
			if existing {
				name = string(scope) + "/existing settings"
			}
			t.Run(name, func(t *testing.T) {
				home, workspace := installTestHome(t), t.TempDir()
				t.Setenv("HOME", home)
				t.Setenv("USERPROFILE", home)
				t.Chdir(workspace)
				target, untouched := workspace, home
				if scope == ScopeGlobal {
					target, untouched = home, workspace
				}
				settings := filepath.Join(target, ".claude", "settings.json")
				otherSettings := filepath.Join(untouched, ".claude", "settings.json")
				const personal = "{\"hooks\":{\"Stop\":[{\"hooks\":[{\"type\":\"command\",\"command\":\"echo personal\"}]}]},\"custom\":true}\n"
				mustWriteFile(t, otherSettings, []byte(personal))
				if existing {
					mustWriteFile(t, settings, []byte(personal))
				}
				before := snapshotTree(t, home)
				priorPlan := installStagePlan
				t.Cleanup(func() { installStagePlan = priorPlan })
				failure := errors.New("injected failure after scoped Claude hooks")
				observed := false
				installStagePlan = func(rt *installRuntime) pipeline.StagePlan {
					if len(rt.selection.Components) != 1 || rt.selection.Components[0] != model.ComponentSkills {
						t.Fatalf("regression must not select a settings-writing component: %v", rt.selection.Components)
					}
					plan := priorPlan(rt)
					plan.Apply = append(plan.Apply, claudeHooksRollbackFailureStep{run: func() error {
						observed = strings.Count(readTextFile(t, settings), "gentle-ai review stop-hook --agent claude-code") == 2
						return failure
					}})
					return plan
				}
				result, err := RunInstall([]string{"--scope=" + string(scope), "--agent=claude-code", "--components=skills", "--skills=go-testing"}, system.DetectionResult{})
				if err == nil || !strings.Contains(err.Error(), failure.Error()) || !result.Execution.Rollback.Success {
					t.Fatalf("RunInstall() = %v, rollback=%+v; want injected failure and successful rollback", err, result.Execution.Rollback)
				}
				if !observed {
					t.Fatal("failure occurred without observing the scoped hook writes")
				}
				if existing {
					if got := readTextFile(t, settings); got != personal {
						t.Errorf("rollback did not restore personal settings: %s", got)
					}
				} else if _, err := os.Stat(settings); !os.IsNotExist(err) {
					t.Errorf("rollback left newly created settings: %v", err)
				}
				if got := readTextFile(t, otherSettings); got != personal {
					t.Error("install/rollback changed opposite-scope settings")
				}
				// Backups are expected, but no install state may be published on failure.
				statePath := filepath.Join(home, ".gentle-ai", "state.json")
				if _, exists := before[filepath.Join(".gentle-ai", "state.json")]; !exists {
					if _, err := os.Stat(statePath); !os.IsNotExist(err) {
						t.Errorf("failed install published state: %v", err)
					}
				}
			})
		}
	}
}

func TestInstallClaudeHooksRespectScope(t *testing.T) {
	for _, scope := range []InstallScope{ScopeWorkspace, ScopeGlobal} {
		t.Run(string(scope), func(t *testing.T) {
			home, workspace := t.TempDir(), t.TempDir()
			t.Setenv("HOME", home)
			t.Setenv("USERPROFILE", home)
			t.Chdir(workspace)
			previousHome, previousCommand, previousLookPath := osUserHomeDir, runCommand, cmdLookPath
			osUserHomeDir = func() (string, error) { return home, nil }
			runCommand = func(string, ...string) error { return nil }
			cmdLookPath = func(string) (string, error) { return "", exec.ErrNotFound }
			t.Cleanup(func() {
				osUserHomeDir, runCommand, cmdLookPath = previousHome, previousCommand, previousLookPath
			})

			const original = "{\"hooks\":{\"Stop\":[{\"hooks\":[{\"type\":\"command\",\"command\":\"echo personal\"}]}]},\"custom\":true}\n"
			target, untouched := workspace, home
			if scope == ScopeGlobal {
				target, untouched = home, workspace
			}
			for _, root := range []string{home, workspace} {
				mustWriteFile(t, filepath.Join(root, ".claude", "settings.json"), []byte(original))
			}
			install := func() {
				t.Helper()
				_, err := RunInstall([]string{"--scope=" + string(scope), "--agent=claude-code", "--components=persona"}, system.DetectionResult{})
				if err != nil {
					t.Fatalf("RunInstall() error = %v", err)
				}
			}
			install()
			path := filepath.Join(target, ".claude", "settings.json")
			first := readTextFile(t, path)
			var settings struct {
				Custom bool `json:"custom"`
				Hooks  map[string][]struct {
					Hooks []struct {
						Command string `json:"command"`
					} `json:"hooks"`
				} `json:"hooks"`
			}
			if err := json.Unmarshal([]byte(first), &settings); err != nil {
				t.Fatal(err)
			}
			got := map[string]int{}
			personal := 0
			for event, entries := range settings.Hooks {
				for _, entry := range entries {
					for _, hook := range entry.Hooks {
						if strings.HasPrefix(hook.Command, "gentle-ai ") || strings.Contains(hook.Command, "gentle-ai skill-registry refresh") {
							got[event]++
						}
						if hook.Command == "echo personal" {
							personal++
						}
					}
				}
			}
			want := map[string]int{"SessionStart": 1, "Stop": 2, "SubagentStop": 1, "UserPromptSubmit": 1}
			for event, count := range want {
				if got[event] != count {
					t.Errorf("managed %s hooks = %d, want %d", event, got[event], count)
				}
			}
			if !settings.Custom || personal != 1 {
				t.Error("install changed personal settings or hooks")
			}
			untouchedPath := filepath.Join(untouched, ".claude", "settings.json")
			if readTextFile(t, untouchedPath) != original {
				t.Error("install modified settings outside the selected scope")
			}
			install()
			if readTextFile(t, path) != first {
				t.Error("second install changed hook settings")
			}
			if readTextFile(t, untouchedPath) != original {
				t.Error("second install modified settings outside the selected scope")
			}
		})
	}
}
