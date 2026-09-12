package vscode

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/model"
)

func TestDetectRequiresVSCodeAndCopilotExtension(t *testing.T) {
	home := t.TempDir()
	a := NewAdapter()
	a.lookPath = func(string) (string, error) { return filepath.Join(home, "bin", "code"), nil }

	if err := os.MkdirAll(filepath.Join(home, ".copilot", "skills"), 0o755); err != nil {
		t.Fatal(err)
	}
	installed, _, _, _, err := a.Detect(context.Background(), home)
	if err != nil || installed {
		t.Fatalf("Detect() with skills only = %v, %v; want false, nil", installed, err)
	}

	if err := os.MkdirAll(filepath.Join(home, ".vscode", "extensions", "github.copilot-1.2.3"), 0o755); err != nil {
		t.Fatal(err)
	}
	installed, _, _, evidence, err := a.Detect(context.Background(), home)
	if err != nil || !installed || !evidence {
		t.Fatalf("Detect() with Copilot extension = %v, %v, %v; want true, true, nil", installed, evidence, err)
	}
}

func TestDetectIgnoresStaleManagedConfiguration(t *testing.T) {
	home := t.TempDir()
	a := NewAdapter()
	a.lookPath = func(string) (string, error) { return filepath.Join(home, "bin", "code"), nil }
	for _, path := range []string{a.SystemPromptFile(home), a.MCPConfigPath(home, "codegraph")} {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("stale"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	installed, _, _, _, err := a.Detect(context.Background(), home)
	if err != nil || installed {
		t.Fatalf("Detect() with stale managed config = %v, %v; want false, nil", installed, err)
	}
}

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
		// A custom root never follows ambient XDG_CONFIG_HOME: containment wins.
		t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg"))
		path := a.SettingsPath(home)
		want := filepath.Join(home, ".config", "Code", "User", "settings.json")
		if path != want {
			t.Fatalf("SettingsPath() = %q, want %q", path, want)
		}

		// The real user home is the only root that honors XDG_CONFIG_HOME.
		realHome, err := os.UserHomeDir()
		if err != nil {
			t.Fatalf("UserHomeDir() error = %v", err)
		}
		path = a.SettingsPath(realHome)
		want = filepath.Join(home, "xdg", "Code", "User", "settings.json")
		if path != want {
			t.Fatalf("SettingsPath(realHome) = %q, want %q", path, want)
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
		// A custom root never follows ambient XDG_CONFIG_HOME: containment wins.
		t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg"))
		path := a.MCPConfigPath(home, "context7")
		want := filepath.Join(home, ".config", "Code", "User", "mcp.json")
		if path != want {
			t.Fatalf("MCPConfigPath() = %q, want %q", path, want)
		}

		// The real user home is the only root that honors XDG_CONFIG_HOME.
		realHome, err := os.UserHomeDir()
		if err != nil {
			t.Fatalf("UserHomeDir() error = %v", err)
		}
		path = a.MCPConfigPath(realHome, "context7")
		want = filepath.Join(home, "xdg", "Code", "User", "mcp.json")
		if path != want {
			t.Fatalf("MCPConfigPath(realHome) = %q, want %q", path, want)
		}
	}
}

func TestCapabilities(t *testing.T) {
	a := NewAdapter()
	home := "/tmp/home"

	if !a.SupportsSkills() {
		t.Fatal("SupportsSkills() = false, want true")
	}
	if !a.SupportsSystemPrompt() {
		t.Fatal("SupportsSystemPrompt() = false, want true")
	}
	if !a.SupportsMCP() {
		t.Fatal("SupportsMCP() = false, want true")
	}
	if a.SupportsSubAgents() {
		t.Fatal("SupportsSubAgents() = true, want false (VS Code Copilot runs inline solo-agent)")
	}
	if a.SupportsSlashCommands() {
		t.Fatal("SupportsSlashCommands() = true, want false")
	}
	if a.SupportsOutputStyles() {
		t.Fatal("SupportsOutputStyles() = true, want false")
	}
	if got := a.SubAgentsDir(home); got != "" {
		t.Fatalf("SubAgentsDir() = %q, want empty", got)
	}
	if got := a.EmbeddedSubAgentsDir(); got != "" {
		t.Fatalf("EmbeddedSubAgentsDir() = %q, want empty", got)
	}
	if got := a.CommandsDir(home); got != "" {
		t.Fatalf("CommandsDir() = %q, want empty", got)
	}
	if got := a.OutputStyleDir(home); got != "" {
		t.Fatalf("OutputStyleDir() = %q, want empty", got)
	}
}
