package cli

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v4/internal/model"
)

// stubEngramSetupRunner resolves engram to a fake binary with setup enabled and
// records every `engram setup` invocation instead of executing it.
func stubEngramSetupRunner(t *testing.T, home string) *[][]string {
	t.Helper()
	t.Setenv("GENTLE_AI_ENGRAM_SETUP_MODE", "supported")
	t.Setenv("GENTLE_AI_ENGRAM_SETUP_STRICT", "")
	enginePath := filepath.Join(home, "test-engram-not-executed")
	originalLookPath, originalRun := cmdLookPath, runCommand
	originalVersion, originalProbe := verifyEngramVersionCommand, probeEngramProtocolFlagCommand
	t.Cleanup(func() {
		cmdLookPath, runCommand = originalLookPath, originalRun
		verifyEngramVersionCommand, probeEngramProtocolFlagCommand = originalVersion, originalProbe
	})
	cmdLookPath = func(name string) (string, error) {
		if name == "engram" {
			return enginePath, nil
		}
		return originalLookPath(name)
	}
	verifyEngramVersionCommand = func(string) (string, error) { return "engram 1.20.0", nil }
	probeEngramProtocolFlagCommand = func(context.Context, string) (string, error) { return "Usage: engram setup <slug>", nil }
	var setupCalls [][]string
	runCommand = func(name string, args ...string) error {
		if name == enginePath && len(args) > 0 && args[0] == "setup" {
			setupCalls = append(setupCalls, args)
		}
		return nil
	}
	return &setupCalls
}

// TestInstallEngramRefusesUnsafeOpenCodeSettingsBeforeSetup proves the selected
// OpenCode settings pass the Engram merge refusals before the external
// `engram setup opencode` runs, so a refusal leaves no setup side effects.
func TestInstallEngramRefusesUnsafeOpenCodeSettingsBeforeSetup(t *testing.T) {
	tests := []struct {
		name    string
		content string
		mode    os.FileMode
		symlink bool
		want    string
	}{
		{name: "duplicate keys", content: "// project settings\n{\"mcp\":{},\"mcp\":{}}\n", mode: 0o600, want: "duplicate"},
		{name: "escaped mcp key", content: "// project settings\n{\"\\u006dcp\":{}}\n", mode: 0o600, want: "escaped key spelling"},
		{name: "comment inside mcp", content: "// project settings\n{\"mcp\":{\n// keep\n\"other\":{}}}\n", mode: 0o600, want: "nested comments"},
		{name: "locked file", content: "// project settings\n{\"user\":true}\n", mode: 0, want: "locked settings"},
		{name: "symlink", content: "// project settings\n{\"user\":true}\n", mode: 0o600, symlink: true, want: "symlink"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if runtime.GOOS == "windows" && (tt.symlink || tt.mode == 0) {
				t.Skip("POSIX file modes and symlinks")
			}
			home, workspace, selected, decoy, _ := themeSettingsFixture(t)
			setupCalls := stubEngramSetupRunner(t, home)
			if tt.symlink {
				target := filepath.Join(t.TempDir(), "target.jsonc")
				if err := os.WriteFile(target, []byte(tt.content), tt.mode); err != nil {
					t.Fatal(err)
				}
				if err := os.Remove(selected); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(target, selected); err != nil {
					t.Fatal(err)
				}
			} else if err := os.WriteFile(selected, []byte(tt.content), 0o600); err != nil {
				t.Fatal(err)
			}
			if err := os.Chmod(selected, tt.mode); err != nil && !tt.symlink {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = os.Chmod(selected, 0o600) })
			decoyBefore, err := os.ReadFile(decoy)
			if err != nil {
				t.Fatal(err)
			}

			selection := model.Selection{Agents: []model.AgentID{model.AgentOpenCode}, Components: []model.ComponentID{model.ComponentEngram}}
			step := componentApplyStep{component: model.ComponentEngram, homeDir: home, workspaceDir: workspace, scope: ScopeGlobal, agents: selection.Agents, selection: selection}
			if err := step.Run(); err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("Run() error = %v, want an unsafe settings refusal containing %q", err, tt.want)
			}
			if len(*setupCalls) != 0 {
				t.Fatalf("engram setup ran before the settings refusal: %v", *setupCalls)
			}
			_ = os.Chmod(selected, 0o600)
			if got, err := os.ReadFile(selected); err != nil || string(got) != tt.content {
				t.Fatalf("selected settings = %q, %v; want unchanged %q", got, err, tt.content)
			}
			if got, err := os.ReadFile(decoy); err != nil || !bytes.Equal(got, decoyBefore) {
				t.Fatalf("decoy settings = %q, %v; want unchanged", got, err)
			}
		})
	}
}

// TestInstallEngramRunsSetupForSafeOpenCodeSettings is the control for the
// refusal test: safe selected settings still reach `engram setup opencode`.
func TestInstallEngramRunsSetupForSafeOpenCodeSettings(t *testing.T) {
	home, workspace, _, _, _ := themeSettingsFixture(t)
	setupCalls := stubEngramSetupRunner(t, home)
	selection := model.Selection{Agents: []model.AgentID{model.AgentOpenCode}, Components: []model.ComponentID{model.ComponentEngram}}
	step := componentApplyStep{component: model.ComponentEngram, homeDir: home, workspaceDir: workspace, scope: ScopeGlobal, agents: selection.Agents, selection: selection}
	if err := step.Run(); err != nil {
		t.Fatal(err)
	}
	if len(*setupCalls) != 1 || (*setupCalls)[0][1] != "opencode" {
		t.Fatalf("setup calls = %v, want one `engram setup opencode`", *setupCalls)
	}
}
