package app

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/gentleman-programming/gentle-ai/v3/internal/model"
	"github.com/gentleman-programming/gentle-ai/v3/internal/opencode"
	"github.com/gentleman-programming/gentle-ai/v3/internal/pipeline"
	"github.com/gentleman-programming/gentle-ai/v3/internal/system"
	"github.com/gentleman-programming/gentle-ai/v3/internal/tui"
)

// This exercises the real app/CLI bridge, but deliberately fails Prepare after
// SDK provisioning. It does not claim a complete install or plugin activation.
func TestTUIOpenCodeSDKConsentProvisioningIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test executes an isolated fake npm subprocess")
	}
	if runtime.GOOS == "windows" {
		t.Skip("fake npm fixture requires a POSIX shell")
	}
	for _, tc := range []struct {
		name       string
		version    string
		decline    bool
		stale      bool
		wantSDKerr string
	}{
		{name: "default decline", decline: true},
		{name: "affirmative materializes pinned SDK", version: "2.0.4"},
		{name: "manager succeeds without SDK", wantSDKerr: "completed without materializing @opencode/plugin@2.0.4"},
		{name: "manager succeeds with wrong SDK", version: "2.0.3", wantSDKerr: "completed without materializing @opencode/plugin@2.0.4"},
		{name: "dependency state changes after proposal", stale: true, wantSDKerr: "package.json is present"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			home, config, log := isolateSDKBridgeTest(t, root, tc.version)
			oldHome, oldVersion := appUserHomeDir, opencode.VersionRunnerOverride
			t.Cleanup(func() {
				appUserHomeDir = oldHome
				opencode.VersionRunnerOverride = oldVersion
			})
			appUserHomeDir = func() (string, error) { return home, nil }
			opencode.VersionRunnerOverride = func(context.Context, opencode.Command) (opencode.CommandOutput, error) {
				return opencode.CommandOutput{Stdout: []byte("2.0.18")}, nil
			}

			m := tui.NewModel(system.DetectionResult{}, "test")
			m.Selection = model.Selection{Agents: []model.AgentID{model.AgentOpenCode}}
			m.DependencyPlan.Agents = m.Selection.Agents
			m.BackgroundIntent = model.OpenCodeBackgroundOff
			m.PiBackgroundIntent = model.PiBackgroundOff
			m.ExecuteSDKFn = tuiExecuteWithSDK
			m.Screen, m.Cursor = tui.ScreenReview, 0
			updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
			m = updated.(tui.Model)
			if cmd != nil || m.Screen != tui.ScreenOpenCodeSDKConfirm || m.Cursor != 1 {
				t.Fatalf("Review Enter: screen=%v cursor=%d command=%v err=%v", m.Screen, m.Cursor, cmd != nil, m.Err)
			}
			for _, text := range []string{"@opencode/plugin@2.0.4", "npm", config, "No / Back", "rollback"} {
				if !strings.Contains(m.View(), text) {
					t.Fatalf("SDK confirmation missing %q: %s", text, m.View())
				}
			}
			assertSDKBridgeMissing(t, log)
			assertSDKBridgeMissing(t, filepath.Join(home, ".gentle-ai", "backups"))
			if tc.decline {
				updated, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
				m = updated.(tui.Model)
				if m.Screen != tui.ScreenReview || cmd != nil {
					t.Fatalf("default No did not return to Review: screen=%v command=%v", m.Screen, cmd != nil)
				}
				assertSDKBridgeMissing(t, log)
				assertSDKBridgeMissing(t, filepath.Join(config, "node_modules"))
				assertSDKBridgeMissing(t, filepath.Join(home, ".gentle-ai", "backups"))
				return
			}
			if tc.stale {
				writeSDKBridgeFile(t, filepath.Join(config, "package.json"), "{\"private\":true}\n", 0600)
			}
			updated, cmd = m.Update(tea.KeyMsg{Type: tea.KeyUp})
			m = updated.(tui.Model)
			if cmd != nil || m.Cursor != 0 {
				t.Fatal("Up did not select affirmative SDK confirmation")
			}
			updated, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
			m = updated.(tui.Model)
			if m.Screen != tui.ScreenInstalling || cmd == nil {
				t.Fatalf("affirmative did not schedule installation: screen=%v", m.Screen)
			}
			m = drainSDKBridgeCommands(t, m, cmd)
			if m.Execution.Err == nil || m.Execution.Prepare.Success || len(m.Execution.Apply.Steps) != 0 {
				t.Fatalf("expected terminal Prepare failure without Apply: %+v", m.Execution)
			}
			if !strings.Contains(strings.Join(m.Progress.Logs, "\n"), "pipeline completed with errors") {
				t.Fatalf("terminal failure not surfaced to TUI: %v", m.Progress.Logs)
			}
			wantIDs := []string{"prepare:opencode-plugin-dependency", "prepare:opencode-telemetry", "prepare:check-dependencies", "prepare:backup-snapshot"}
			if len(m.Execution.Prepare.Steps) != len(wantIDs) {
				t.Fatalf("unexpected Prepare steps: %+v", m.Execution.Prepare.Steps)
			}
			for i, step := range m.Execution.Prepare.Steps {
				if step.StepID != wantIDs[i] {
					t.Fatalf("Prepare step %d = %s, want %s", i, step.StepID, wantIDs[i])
				}
				wantErr := ""
				if i == 0 {
					wantErr = tc.wantSDKerr
				} else if i == 1 {
					wantErr = "telemetry runtime ownership conflict"
				}
				if wantErr != "" {
					if step.Status != pipeline.StepStatusFailed || step.Err == nil || !strings.Contains(step.Err.Error(), wantErr) {
						t.Fatalf("%s: want failure %q, got %+v", step.StepID, wantErr, step)
					}
				} else if step.Status != pipeline.StepStatusSucceeded || step.Err != nil {
					t.Fatalf("%s unexpectedly failed: %+v", step.StepID, step)
				}
			}
			if tc.stale {
				assertSDKBridgeMissing(t, log)
				data, err := os.ReadFile(filepath.Join(config, "package.json"))
				if err != nil || string(data) != "{\"private\":true}\n" {
					t.Fatalf("changed dependency state not preserved: %q, %v", data, err)
				}
			} else {
				data, err := os.ReadFile(log)
				if err != nil {
					t.Fatal(err)
				}
				physicalConfig, err := filepath.EvalSymlinks(config)
				if err != nil {
					t.Fatal(err)
				}
				lines := strings.Split(strings.TrimSpace(string(data)), "\n")
				if lines[0] != physicalConfig || strings.Count(string(data), "@opencode/plugin@2.0.4\n") != 1 || strings.Count(string(data), "install\n") != 1 {
					t.Fatalf("fake npm did not receive exactly one pinned install in config: %s", data)
				}
				for _, arg := range []string{"--ignore-scripts", "--registry=https://registry.npmjs.org", "--prefix=" + physicalConfig} {
					if !strings.Contains(string(data), "\n"+arg+"\n") {
						t.Fatalf("missing isolated npm argument %q: %s", arg, data)
					}
				}
			}
			manifest := filepath.Join(config, "node_modules", "@opencode", "plugin", "package.json")
			if tc.version == "" {
				assertSDKBridgeMissing(t, manifest)
			} else if data, err := os.ReadFile(manifest); err != nil || string(data) != fmt.Sprintf("{\"version\":\"%s\"}\n", tc.version) {
				t.Fatalf("materialized SDK = %q, %v", data, err)
			}
			data, err := os.ReadFile(filepath.Join(config, "plugins", "telemetry-runtime.ts"))
			if err != nil || string(data) != "// custom telemetry blocker\n" {
				t.Fatalf("custom telemetry changed: %q, %v", data, err)
			}
			assertSDKBridgeMissing(t, filepath.Join(config, "opencode.json"))
			assertSDKBridgeMissing(t, filepath.Join(config, ".gentle-ai-telemetry-runtime.json"))
		})
	}
}

func isolateSDKBridgeTest(t *testing.T, root, version string) (home, config, log string) {
	t.Helper()
	home = filepath.Join(root, "home")
	config = filepath.Join(root, "config", "opencode")
	log = filepath.Join(root, "npm-install.log")
	// Do not inherit host-specific config roots or runtime injection settings.
	for _, env := range os.Environ() {
		name, _, _ := strings.Cut(env, "=")
		if strings.HasPrefix(name, "OPENCODE_") || strings.HasPrefix(name, "GENTLE_AI_") || strings.HasPrefix(name, "XDG_") {
			t.Setenv(name, "")
		}
	}
	for name, dir := range map[string]string{
		"HOME": home, "USERPROFILE": home, "XDG_CONFIG_HOME": filepath.Dir(config),
		"XDG_DATA_HOME": filepath.Join(root, "data"), "XDG_STATE_HOME": filepath.Join(root, "state"),
		"XDG_CACHE_HOME": filepath.Join(root, "cache"), "XDG_RUNTIME_DIR": filepath.Join(root, "run"),
		"TMPDIR": filepath.Join(root, "tmp"), "TMP": filepath.Join(root, "tmp"), "TEMP": filepath.Join(root, "tmp"),
		"APPDATA": filepath.Join(root, "appdata"), "LOCALAPPDATA": filepath.Join(root, "localappdata"),
	} {
		if err := os.MkdirAll(dir, 0700); err != nil {
			t.Fatal(err)
		}
		t.Setenv(name, dir)
	}
	t.Setenv("GENTLE_AI_NO_ANIMATION", "1")
	work := filepath.Join(root, "workspace")
	if err := os.MkdirAll(work, 0700); err != nil {
		t.Fatal(err)
	}
	t.Chdir(work)
	bin := filepath.Join(root, "bin")
	t.Setenv("PATH", bin)
	// ContinueOnError still probes dependencies. Only controlled version-only
	// scripts are discoverable, and npm rejects every other unexpected command.
	for _, name := range []string{"git", "curl", "node", "go", "brew"} {
		writeSDKBridgeFile(t, filepath.Join(bin, name), "#!/bin/sh\ncase \"$1\" in --version|version) printf '22.0.0\\n';; *) exit 97;; esac\n", 0700)
	}
	quote := func(s string) string { return "'" + strings.ReplaceAll(s, "'", "'\"'\"'") + "'" }
	script := "#!/bin/sh\nset -eu\nif [ \"$1\" = --version ]; then printf '10.0.0\\n'; exit 0; fi\n[ \"$1\" = install ] || exit 98\n{ pwd -P; printf '%s\\n' \"$@\"; } >> " + quote(log) + "\n"
	if version != "" {
		script += "/bin/mkdir -p node_modules/@opencode/plugin\nprintf '%s\\n' " + quote(fmt.Sprintf(`{"version":%q}`, version)) + " > node_modules/@opencode/plugin/package.json\n"
	}
	writeSDKBridgeFile(t, filepath.Join(bin, "npm"), script, 0700)
	writeSDKBridgeFile(t, filepath.Join(config, "plugins", "telemetry-runtime.ts"), "// custom telemetry blocker\n", 0600)
	return home, config, log
}

func writeSDKBridgeFile(t *testing.T, path, content string, mode os.FileMode) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), mode); err != nil {
		t.Fatal(err)
	}
}

func assertSDKBridgeMissing(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Lstat(path); !os.IsNotExist(err) {
		t.Fatalf("expected absent %s, got %v", path, err)
	}
}

func drainSDKBridgeCommands(t *testing.T, m tui.Model, initial tea.Cmd) tui.Model {
	t.Helper()
	messages := make(chan tea.Msg, 16)
	stop := make(chan struct{})
	defer close(stop)
	launch := func(cmd tea.Cmd) {
		if cmd != nil {
			go func() {
				msg := cmd()
				select {
				case messages <- msg:
				case <-stop:
				}
			}()
		}
	}
	launch(initial)
	deadline := time.NewTimer(10 * time.Second)
	defer deadline.Stop()
	for count := 0; count < 128; count++ {
		select {
		case msg := <-messages:
			if batch, ok := msg.(tea.BatchMsg); ok {
				for _, cmd := range batch {
					launch(cmd)
				}
				continue
			}
			updated, cmd := m.Update(msg)
			m = updated.(tui.Model)
			if _, done := msg.(tui.PipelineDoneMsg); done {
				if cmd != nil {
					t.Fatal("failed pipeline scheduled unexpected follow-up work")
				}
				return m
			}
			launch(cmd)
		case <-deadline.C:
			t.Fatal("TUI bridge did not deliver PipelineDoneMsg within 10 seconds")
		}
	}
	t.Fatal("TUI bridge exceeded 128 command/progress messages")
	return m
}
