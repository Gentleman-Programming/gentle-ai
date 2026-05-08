package copilotcli

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/gentleman-programming/gentle-ai/internal/model"
	"github.com/gentleman-programming/gentle-ai/internal/system"
)

func TestDetect(t *testing.T) {
	tests := []struct {
		name            string
		lookPathPath    string
		lookPathErr     error
		stat            statResult
		wantInstalled   bool
		wantBinaryPath  string
		wantConfigPath  string
		wantConfigFound bool
		wantErr         bool
	}{
		{
			name:            "binary and config directory found",
			lookPathPath:    "/usr/local/bin/copilot",
			stat:            statResult{isDir: true},
			wantInstalled:   true,
			wantBinaryPath:  "/usr/local/bin/copilot",
			wantConfigPath:  filepath.Join("/tmp/home", ".copilot"),
			wantConfigFound: true,
		},
		{
			name:            "binary missing and config missing",
			lookPathErr:     errors.New("missing"),
			stat:            statResult{err: os.ErrNotExist},
			wantInstalled:   false,
			wantBinaryPath:  "",
			wantConfigPath:  filepath.Join("/tmp/home", ".copilot"),
			wantConfigFound: false,
		},
		{
			name:    "stat error bubbles up",
			stat:    statResult{err: errors.New("permission denied")},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := &Adapter{
				lookPath: func(string) (string, error) {
					return tt.lookPathPath, tt.lookPathErr
				},
				statPath: func(string) statResult {
					return tt.stat
				},
			}

			installed, binaryPath, configPath, configFound, err := a.Detect(context.Background(), "/tmp/home")
			if (err != nil) != tt.wantErr {
				t.Fatalf("Detect() error = %v, wantErr %v", err, tt.wantErr)
			}

			if tt.wantErr {
				return
			}

			if installed != tt.wantInstalled {
				t.Fatalf("Detect() installed = %v, want %v", installed, tt.wantInstalled)
			}

			if binaryPath != tt.wantBinaryPath {
				t.Fatalf("Detect() binaryPath = %q, want %q", binaryPath, tt.wantBinaryPath)
			}

			if configPath != tt.wantConfigPath {
				t.Fatalf("Detect() configPath = %q, want %q", configPath, tt.wantConfigPath)
			}

			if configFound != tt.wantConfigFound {
				t.Fatalf("Detect() configFound = %v, want %v", configFound, tt.wantConfigFound)
			}
		})
	}
}

func TestInstallCommandNotSupported(t *testing.T) {
	a := NewAdapter()

	if a.SupportsAutoInstall() {
		t.Fatal("SupportsAutoInstall() = true, want false")
	}

	_, err := a.InstallCommand(system.PlatformProfile{OS: "linux"})
	if err == nil {
		t.Fatal("InstallCommand() error = nil, want error")
	}
}

func TestConfigPaths(t *testing.T) {
	a := NewAdapter()
	home := "/tmp/home"

	if got := a.GlobalConfigDir(home); got != filepath.Join(home, ".copilot") {
		t.Fatalf("GlobalConfigDir() = %q, want %q", got, filepath.Join(home, ".copilot"))
	}

	if got := a.SkillsDir(home); got != filepath.Join(home, ".copilot", "skills") {
		t.Fatalf("SkillsDir() = %q, want %q", got, filepath.Join(home, ".copilot", "skills"))
	}

	if got := a.SystemPromptFile(home); got != filepath.Join(home, ".copilot", "AGENTS.md") {
		t.Fatalf("SystemPromptFile() = %q, want %q", got, filepath.Join(home, ".copilot", "AGENTS.md"))
	}

	if got := a.SettingsPath(home); got != filepath.Join(home, ".copilot", "settings.json") {
		t.Fatalf("SettingsPath() = %q, want %q", got, filepath.Join(home, ".copilot", "settings.json"))
	}

	if got := a.MCPConfigPath(home, "engram"); got != filepath.Join(home, ".copilot", "settings.json") {
		t.Fatalf("MCPConfigPath() = %q, want %q", got, filepath.Join(home, ".copilot", "settings.json"))
	}
}

func TestCapabilities(t *testing.T) {
	a := NewAdapter()

	if got := a.Agent(); got != model.AgentCopilotCLI {
		t.Fatalf("Agent() = %q, want %q", got, model.AgentCopilotCLI)
	}
	if got := a.SupportsMCP(); !got {
		t.Fatal("SupportsMCP() = false, want true")
	}
	if got := a.SupportsSkills(); !got {
		t.Fatal("SupportsSkills() = false, want true")
	}
	if got := a.SupportsSystemPrompt(); !got {
		t.Fatal("SupportsSystemPrompt() = false, want true")
	}
	if got := a.MCPStrategy(); got != model.StrategyMergeIntoSettings {
		t.Fatalf("MCPStrategy() = %v, want StrategyMergeIntoSettings", got)
	}
	if got := a.SystemPromptStrategy(); got != model.StrategyFileReplace {
		t.Fatalf("SystemPromptStrategy() = %v, want StrategyFileReplace", got)
	}
}
