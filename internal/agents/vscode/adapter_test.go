package vscode

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/gentleman-programming/gentle-ai/internal/model"
	"github.com/gentleman-programming/gentle-ai/internal/system"
)

func TestStrategies(t *testing.T) {
	a := NewAdapter()

	if got := a.SystemPromptStrategy(); got != model.StrategyInstructionsFile {
		t.Fatalf("SystemPromptStrategy() = %v, want %v", got, model.StrategyInstructionsFile)
	}

	if got := a.MCPStrategy(); got != model.StrategyMCPConfigFile {
		t.Fatalf("MCPStrategy() = %v, want %v", got, model.StrategyMCPConfigFile)
	}
}

func TestSystemPromptFileUsesInstructionsExtension(t *testing.T) {
	a := NewAdapter()
	home := "/tmp/home"

	path := a.SystemPromptFile(home)
	if filepath.Ext(path) != ".md" {
		t.Fatalf("SystemPromptFile() should end with .md: %q", path)
	}

	if filepath.Base(path) != "gentle-ai.instructions.md" {
		t.Fatalf("SystemPromptFile() = %q, want filename gentle-ai.instructions.md", path)
	}
}

func TestSettingsPathUsesVSCodeUserProfile(t *testing.T) {
	a := NewAdapter()
	home := "/tmp/home"

	switch runtime.GOOS {
	case "darwin":
		path := a.SettingsPath(home)
		want := filepath.Join(home, "Library", "Application Support", "Code", "User", "settings.json")
		if path != want {
			t.Fatalf("SettingsPath() = %q, want %q", path, want)
		}
	case "windows":
		appData := filepath.Join(home, "AppData", "Roaming")
		t.Setenv("APPDATA", appData)
		path := a.SettingsPath(home)
		want := filepath.Join(appData, "Code", "User", "settings.json")
		if path != want {
			t.Fatalf("SettingsPath() = %q, want %q", path, want)
		}
	default:
		t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg"))
		path := a.SettingsPath(home)
		want := filepath.Join(home, "xdg", "Code", "User", "settings.json")
		if path != want {
			t.Fatalf("SettingsPath() = %q, want %q", path, want)
		}
	}
}

func TestMCPConfigPathUsesVSCodeUserProfile(t *testing.T) {
	a := NewAdapter()
	home := "/tmp/home"

	switch runtime.GOOS {
	case "darwin":
		path := a.MCPConfigPath(home, "context7")
		want := filepath.Join(home, "Library", "Application Support", "Code", "User", "mcp.json")
		if path != want {
			t.Fatalf("MCPConfigPath() = %q, want %q", path, want)
		}
	case "windows":
		appData := filepath.Join(home, "AppData", "Roaming")
		t.Setenv("APPDATA", appData)
		path := a.MCPConfigPath(home, "context7")
		want := filepath.Join(appData, "Code", "User", "mcp.json")
		if path != want {
			t.Fatalf("MCPConfigPath() = %q, want %q", path, want)
		}
	default:
		t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg"))
		path := a.MCPConfigPath(home, "context7")
		want := filepath.Join(home, "xdg", "Code", "User", "mcp.json")
		if path != want {
			t.Fatalf("MCPConfigPath() = %q, want %q", path, want)
		}
	}
}

func TestAdapterIdentityAndCapabilities(t *testing.T) {
	a := NewAdapter()

	if a.Agent() != model.AgentVSCodeCopilot {
		t.Errorf("Agent() = %q, want %q", a.Agent(), model.AgentVSCodeCopilot)
	}

	if a.Tier() != model.TierFull {
		t.Errorf("Tier() = %q, want %q", a.Tier(), model.TierFull)
	}

	if a.SupportsAutoInstall() {
		t.Error("SupportsAutoInstall() should be false")
	}

	if !a.SupportsSkills() {
		t.Error("SupportsSkills() should be true")
	}

	if !a.SupportsSystemPrompt() {
		t.Error("SupportsSystemPrompt() should be true")
	}

	if !a.SupportsMCP() {
		t.Error("SupportsMCP() should be true")
	}

	if a.SupportsOutputStyles() {
		t.Error("SupportsOutputStyles() should be false")
	}

	if a.OutputStyleDir("") != "" {
		t.Error("OutputStyleDir() should be empty")
	}

	if a.SupportsSlashCommands() {
		t.Error("SupportsSlashCommands() should be false")
	}

	if a.CommandsDir("") != "" {
		t.Error("CommandsDir() should be empty")
	}

	if a.SupportsSubAgents() {
		t.Error("SupportsSubAgents() should be false")
	}

	if a.SubAgentsDir("") != "" {
		t.Error("SubAgentsDir() should be empty")
	}

	if a.EmbeddedSubAgentsDir() != "" {
		t.Error("EmbeddedSubAgentsDir() should be empty")
	}
}

func TestAdapterDetect(t *testing.T) {
	t.Run("code binary found", func(t *testing.T) {
		a := &Adapter{
			lookPath: func(binary string) (string, error) {
				if binary == "code" {
					return "/usr/bin/code", nil
				}
				return "", os.ErrNotExist
			},
		}

		detected, binaryPath, configPath, hasConfig, err := a.Detect(context.Background(), "/tmp/home")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !detected {
			t.Error("expected detected = true")
		}
		if binaryPath != "/usr/bin/code" {
			t.Errorf("expected binaryPath = %q, got %q", "/usr/bin/code", binaryPath)
		}
		if configPath != "" {
			t.Errorf("expected empty configPath, got %q", configPath)
		}
		if !hasConfig {
			t.Error("expected hasConfig = true")
		}
	})

	t.Run("code binary not found", func(t *testing.T) {
		a := &Adapter{
			lookPath: func(binary string) (string, error) {
				return "", os.ErrNotExist
			},
		}

		detected, binaryPath, configPath, hasConfig, err := a.Detect(context.Background(), "/tmp/home")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if detected {
			t.Error("expected detected = false")
		}
		if binaryPath != "" {
			t.Errorf("expected empty binaryPath, got %q", binaryPath)
		}
		if configPath != "" {
			t.Errorf("expected empty configPath, got %q", configPath)
		}
		if hasConfig {
			t.Error("expected hasConfig = false")
		}
	})
}

func TestAdapterInstallCommand(t *testing.T) {
	a := NewAdapter()
	commands, err := a.InstallCommand(system.PlatformProfile{})
	if err == nil {
		t.Fatal("expected InstallCommand to return error, got nil")
	}
	if commands != nil {
		t.Errorf("expected commands to be nil, got %+v", commands)
	}

	errText := err.Error()
	wantText := "agent vscode-copilot is a desktop app and cannot be installed via CLI"
	if errText != wantText {
		t.Errorf("error = %q, want %q", errText, wantText)
	}
}

func TestAdapterGlobalPaths(t *testing.T) {
	a := NewAdapter()
	home := "/home/user"

	if got := a.GlobalConfigDir(home); got != filepath.Join(home, ".copilot") {
		t.Errorf("GlobalConfigDir = %q, want %q", got, filepath.Join(home, ".copilot"))
	}

	if got := a.SkillsDir(home); got != filepath.Join(home, ".copilot", "skills") {
		t.Errorf("SkillsDir = %q, want %q", got, filepath.Join(home, ".copilot", "skills"))
	}

	if got := a.SystemPromptDir(home); !filepath.HasPrefix(got, home) {
		t.Errorf("SystemPromptDir = %q, expected to start with home %q", got, home)
	}
}

func TestVSCodeUserDirDefault(t *testing.T) {
	a := NewAdapter()
	home := "/home/user"

	if runtime.GOOS != "darwin" && runtime.GOOS != "windows" {
		t.Setenv("XDG_CONFIG_HOME", "")
		got := a.vscodeUserDir(home)
		want := filepath.Join(home, ".config", "Code", "User")
		if got != want {
			t.Errorf("vscodeUserDir default = %q, want %q", got, want)
		}
	} else if runtime.GOOS == "windows" {
		t.Setenv("APPDATA", "")
		got := a.vscodeUserDir(home)
		want := filepath.Join(home, "AppData", "Roaming", "Code", "User")
		if got != want {
			t.Errorf("vscodeUserDir default Windows = %q, want %q", got, want)
		}
	}
}
