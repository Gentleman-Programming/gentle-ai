package openclaw

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/internal/model"
	"github.com/gentleman-programming/gentle-ai/internal/system"
)

func TestAdapterIdentityAndStrategies(t *testing.T) {
	a := NewAdapter()
	homeDir := filepath.Join(string(filepath.Separator), "tmp", "home")

	if got := a.Agent(); got != model.AgentOpenClaw {
		t.Fatalf("Agent() = %q, want %q", got, model.AgentOpenClaw)
	}

	if got := a.Tier(); got != model.TierFull {
		t.Fatalf("Tier() = %q, want %q", got, model.TierFull)
	}

	if got := a.SystemPromptStrategy(); got != model.StrategyMarkdownSections {
		t.Fatalf("SystemPromptStrategy() = %v, want %v", got, model.StrategyMarkdownSections)
	}

	if got := a.MCPStrategy(); got != model.StrategyMergeIntoSettings {
		t.Fatalf("MCPStrategy() = %v, want %v", got, model.StrategyMergeIntoSettings)
	}

	if !a.SupportsSystemPrompt() {
		t.Fatalf("SupportsSystemPrompt() = false, want true")
	}

	if !a.SupportsMCP() {
		t.Fatalf("SupportsMCP() = false, want true")
	}

	if a.SupportsOutputStyles() {
		t.Fatalf("SupportsOutputStyles() = true, want false because OpenClaw uses SOUL.md instead of output-style files")
	}

	if got := a.SystemPromptFile(homeDir); got != filepath.Join(homeDir, "AGENTS.md") {
		t.Fatalf("SystemPromptFile() = %q, want workspace AGENTS.md", got)
	}

	if got := a.OutputStyleDir(homeDir); got != "" {
		t.Fatalf("OutputStyleDir() = %q, want empty because OpenClaw persona injection targets SOUL.md", got)
	}
}

func TestInstallCommandRequiresManualInstall(t *testing.T) {
	a := NewAdapter()

	commands, err := a.InstallCommand(system.PlatformProfile{})
	if err == nil {
		t.Fatalf("InstallCommand() error = nil, want manual install error")
	}
	if commands != nil {
		t.Fatalf("InstallCommand() commands = %v, want nil", commands)
	}
	if got := err.Error(); !strings.Contains(got, "must be installed manually") {
		t.Fatalf("InstallCommand() error = %q, want actionable manual install message", got)
	}
}

func TestAdapterConfigPaths(t *testing.T) {
	a := NewAdapter()
	homeDir := filepath.Join(string(filepath.Separator), "tmp", "home")
	configDir := filepath.Join(homeDir, ".openclaw")
	configPath := filepath.Join(configDir, "openclaw.json")

	paths := map[string]string{
		"GlobalConfigDir": a.GlobalConfigDir(homeDir),
		"SettingsPath":    a.SettingsPath(homeDir),
		"MCPConfigPath":   a.MCPConfigPath(homeDir, "engram"),
		"SkillsDir":       a.SkillsDir(homeDir),
	}

	if got := paths["GlobalConfigDir"]; got != configDir {
		t.Fatalf("GlobalConfigDir() = %q, want %q", got, configDir)
	}

	if got := paths["SettingsPath"]; got != configPath {
		t.Fatalf("SettingsPath() = %q, want %q", got, configPath)
	}

	if got := paths["MCPConfigPath"]; got != configPath {
		t.Fatalf("MCPConfigPath() = %q, want %q", got, configPath)
	}

	if got := paths["SkillsDir"]; got != filepath.Join(configDir, "skills") {
		t.Fatalf("SkillsDir() = %q, want OpenClaw skills dir", got)
	}
}

func TestDetect(t *testing.T) {
	tests := []struct {
		name            string
		lookPathPath    string
		lookPathErr     error
		stat            statResult
		wantInstalled   bool
		wantBinaryPath  string
		wantConfigFound bool
		wantErr         bool
	}{
		{
			name:            "binary and config directory found",
			lookPathPath:    "/opt/homebrew/bin/openclaw",
			stat:            statResult{isDir: true},
			wantInstalled:   true,
			wantBinaryPath:  "/opt/homebrew/bin/openclaw",
			wantConfigFound: true,
		},
		{
			name:            "binary missing and config missing",
			lookPathErr:     errors.New("missing"),
			stat:            statResult{err: os.ErrNotExist},
			wantInstalled:   false,
			wantBinaryPath:  "",
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
			homeDir := filepath.Join(string(filepath.Separator), "tmp", "home")

			installed, binaryPath, configPath, configFound, err := a.Detect(context.Background(), homeDir)
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

			wantConfigPath := filepath.Join(homeDir, ".openclaw")
			if configPath != wantConfigPath {
				t.Fatalf("Detect() configPath = %q, want %q", configPath, wantConfigPath)
			}

			if configFound != tt.wantConfigFound {
				t.Fatalf("Detect() configFound = %v, want %v", configFound, tt.wantConfigFound)
			}
		})
	}
}
