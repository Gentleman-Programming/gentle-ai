package antigravity

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
		stat            statResult
		wantInstalled   bool
		wantBinaryPath  string
		wantConfigPath  string
		wantConfigFound bool
		wantErr         bool
	}{
		{
			name:            "config directory found",
			stat:            statResult{isDir: true},
			wantInstalled:   true,
			wantBinaryPath:  "",
			wantConfigPath:  filepath.Join("/tmp/home", ".gemini", "antigravity"),
			wantConfigFound: true,
		},
		{
			name:            "config directory missing",
			stat:            statResult{err: os.ErrNotExist},
			wantInstalled:   false,
			wantBinaryPath:  "",
			wantConfigPath:  filepath.Join("/tmp/home", ".gemini", "antigravity"),
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

func TestInstallCommand(t *testing.T) {
	a := NewAdapter()

	_, err := a.InstallCommand(system.PlatformProfile{OS: "darwin"})
	if err == nil {
		t.Fatal("InstallCommand() expected error for desktop IDE, got nil")
	}

	var notInstallable AgentNotInstallableError
	if !errors.As(err, &notInstallable) {
		t.Fatalf("InstallCommand() error type = %T, want AgentNotInstallableError", err)
	}

	if notInstallable.Agent != model.AgentAntigravity {
		t.Fatalf("AgentNotInstallableError.Agent = %q, want %q", notInstallable.Agent, model.AgentAntigravity)
	}
}

func TestSupportsAutoInstall(t *testing.T) {
	a := NewAdapter()

	if a.SupportsAutoInstall() {
		t.Fatal("SupportsAutoInstall() = true, want false for desktop IDE")
	}
}

func TestConfigPathsCrossPlatform(t *testing.T) {
	a := NewAdapter()
	home := "/tmp/home"

	if got := a.GlobalConfigDir(home); got != filepath.Join(home, ".gemini", "antigravity") {
		t.Fatalf("GlobalConfigDir() = %q, want %q", got, filepath.Join(home, ".gemini", "antigravity"))
	}

	if got := a.SkillsDir(home); got != filepath.Join(home, ".gemini", "antigravity", "skills") {
		t.Fatalf("SkillsDir() = %q, want %q", got, filepath.Join(home, ".gemini", "antigravity", "skills"))
	}

	if got := a.MCPConfigPath(home, "ctx7"); got != filepath.Join(home, ".gemini", "antigravity", "mcp_config.json") {
		t.Fatalf("MCPConfigPath() = %q, want %q", got, filepath.Join(home, ".gemini", "antigravity", "mcp_config.json"))
	}

	if got := a.SystemPromptFile(home); got != filepath.Join(home, ".gemini", "GEMINI.md") {
		t.Fatalf("SystemPromptFile() = %q, want %q", got, filepath.Join(home, ".gemini", "GEMINI.md"))
	}

	if got := a.SettingsPath(home); got != filepath.Join(home, ".gemini", "antigravity", "settings.json") {
		t.Fatalf("SettingsPath() = %q, want %q", got, filepath.Join(home, ".gemini", "antigravity", "settings.json"))
	}

	if got := a.SystemPromptDir(home); got != filepath.Join(home, ".gemini") {
		t.Fatalf("SystemPromptDir() = %q, want %q", got, filepath.Join(home, ".gemini"))
	}
}

func TestCapabilities(t *testing.T) {
	a := NewAdapter()

	if !a.SupportsSkills() {
		t.Fatal("SupportsSkills() = false, want true")
	}

	if !a.SupportsSystemPrompt() {
		t.Fatal("SupportsSystemPrompt() = false, want true")
	}

	if !a.SupportsMCP() {
		t.Fatal("SupportsMCP() = false, want true")
	}

	if a.SupportsOutputStyles() {
		t.Fatal("SupportsOutputStyles() = true, want false")
	}

	if a.SupportsSlashCommands() {
		t.Fatal("SupportsSlashCommands() = true, want false")
	}

	if got := a.OutputStyleDir("/tmp/home"); got != "" {
		t.Fatalf("OutputStyleDir() = %q, want empty string", got)
	}

	if got := a.CommandsDir("/tmp/home"); got != "" {
		t.Fatalf("CommandsDir() = %q, want empty string", got)
	}
}

func TestStrategies(t *testing.T) {
	a := NewAdapter()

	if got := a.SystemPromptStrategy(); got != model.StrategyAppendToFile {
		t.Fatalf("SystemPromptStrategy() = %v, want StrategyAppendToFile", got)
	}

	if got := a.MCPStrategy(); got != model.StrategyMCPConfigFile {
		t.Fatalf("MCPStrategy() = %v, want StrategyMCPConfigFile", got)
	}
}

func TestIdentity(t *testing.T) {
	a := NewAdapter()

	if got := a.Agent(); got != model.AgentAntigravity {
		t.Fatalf("Agent() = %q, want %q", got, model.AgentAntigravity)
	}

	if got := a.Tier(); got != model.TierFull {
		t.Fatalf("Tier() = %q, want %q", got, model.TierFull)
	}
}
