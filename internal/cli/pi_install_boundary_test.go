package cli

import (
	"crypto/sha256"
	"errors"
	"fmt"
	piagent "github.com/gentleman-programming/gentle-ai/v4/internal/agents/pi"
	"github.com/gentleman-programming/gentle-ai/v4/internal/installcmd"
	"github.com/gentleman-programming/gentle-ai/v4/internal/model"
	"github.com/gentleman-programming/gentle-ai/v4/internal/pipeline"
	"github.com/gentleman-programming/gentle-ai/v4/internal/planner"
	"github.com/gentleman-programming/gentle-ai/v4/internal/state"
	"github.com/gentleman-programming/gentle-ai/v4/internal/system"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func piInstallTestHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("PI_CODING_AGENT_DIR", "")
	return home
}

func piInstallHomeBytes(t *testing.T, home string) map[string]string {
	t.Helper()
	entries := map[string]string{}
	if err := filepath.WalkDir(home, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(home, path)
		if err != nil {
			return err
		}
		if entry.IsDir() {
			entries[rel] = "directory"
			return nil
		}
		data, err := os.ReadFile(path)
		entries[rel] = fmt.Sprintf("%x", sha256.Sum256(data))
		return err
	}); err != nil {
		t.Fatal(err)
	}
	return entries
}

func TestPiPublicInstallRefusesBeforeAnyMutation(t *testing.T) {
	for _, settings := range []string{
		`{"packages":["npm:gentle-engram@0.1.16","npm:gentle-engram@0.2.0"]}`,
		`{"packages":[{"source":"npm:gentle-engram@0.1.16","extensions":[]},{"source":"npm:gentle-engram@0.1.16","extensions":["index.ts"]}]}`,
		`invalid`,
	} {
		for _, boundary := range []string{"CLI", "TUI"} {
			t.Run(boundary+settings, func(t *testing.T) {
				home := piInstallTestHome(t)
				// Isolate external binaries even on the pre-fix RED path.
				t.Setenv("PATH", t.TempDir())
				path := piagent.NewAdapter().SettingsPath(home)
				if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, []byte(settings), 0o644); err != nil {
					t.Fatal(err)
				}
				if err := state.Write(home, state.InstallState{InstalledAgents: []string{"opencode"}}); err != nil {
					t.Fatal(err)
				}
				before := piInstallHomeBytes(t, home)
				previousHome, previousCommand := osUserHomeDir, runCommand
				t.Cleanup(func() { osUserHomeDir, runCommand = previousHome, previousCommand })
				osUserHomeDir = func() (string, error) { return home, nil }
				commands, progress := 0, 0
				runCommand = func(string, ...string) error { commands++; return errors.New("unexpected install command") }
				t.Cleanup(installcmd.OverrideLookPath(func(string) (string, error) { return "pi", nil }))
				outputPath := filepath.Join(t.TempDir(), "output")
				output, err := os.Create(outputPath)
				if err != nil {
					t.Fatal(err)
				}
				stdout, stderr := os.Stdout, os.Stderr
				os.Stdout, os.Stderr = output, output
				t.Cleanup(func() { os.Stdout, os.Stderr = stdout, stderr; output.Close() })
				if boundary == "CLI" {
					_, err = RunInstall([]string{"--agent", "pi", "--component", "persona"}, system.DetectionResult{})
				} else {
					selection := model.Selection{Agents: []model.AgentID{model.AgentPi}, Components: []model.ComponentID{model.ComponentPersona}}
					resolved, resolveErr := planner.NewResolver(planner.MVPGraph()).Resolve(selection)
					if resolveErr != nil {
						t.Fatal(resolveErr)
					}
					result, orchestrator, _ := ExecuteTUIInstallRecordingCodexServiceTier(home, selection, resolved, system.PlatformProfile{}, "", "", func(pipeline.ProgressEvent) { progress++ })
					err = result.Err
					if orchestrator != nil {
						progress++
					}
				}
				os.Stdout, os.Stderr = stdout, stderr
				if err == nil {
					t.Fatal("invalid Pi configuration was accepted")
				}
				if settings == "invalid" {
					want := fmt.Sprintf("preflight for agent %q: unmarshal pi json file %q: invalid character 'i' looking for beginning of value", model.AgentPi, path)
					if err.Error() != want {
						t.Errorf("error = %q, want %q", err, want)
					}
				} else {
					want := fmt.Sprintf("preflight for agent %q: conflicting Engram package declarations in %q; resolve them before installing Pi packages", model.AgentPi, path)
					if err.Error() != want {
						t.Errorf("error = %q, want %q", err, want)
					}
				}
				if commands != 0 || progress != 0 {
					t.Errorf("commands=%d progress=%d, want zero", commands, progress)
				}
				captured, readErr := os.ReadFile(outputPath)
				if readErr != nil {
					t.Fatal(readErr)
				}
				if len(captured) != 0 {
					t.Errorf("unexpected stdout/stderr: %q", captured)
				}
				if after := piInstallHomeBytes(t, home); !reflect.DeepEqual(after, before) {
					t.Errorf("rejected install changed stored files/directories: before=%v after=%v", before, after)
				}
			})
		}
	}
}
