package vscode

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/internal/model"
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

func TestSubAgentSupportUsesCopilotUserAgentsDir(t *testing.T) {
	a := NewAdapter()
	home := "/tmp/home"

	if !a.SupportsSubAgents() {
		t.Fatal("VS Code adapter must advertise native sub-agent support")
	}

	got := a.SubAgentsDir(home)
	want := filepath.Join(home, ".copilot", "agents")
	if got != want {
		t.Fatalf("SubAgentsDir() = %q, want %q", got, want)
	}
	if got == filepath.Join(home, ".github", "agents") || strings.Contains(got, string(filepath.Separator)+".github"+string(filepath.Separator)) {
		t.Fatalf("SubAgentsDir() must not target workspace .github/agents, got %q", got)
	}

	if got := a.EmbeddedSubAgentsDir(); got != "vscode/agents" {
		t.Fatalf("EmbeddedSubAgentsDir() = %q, want %q", got, "vscode/agents")
	}
}
