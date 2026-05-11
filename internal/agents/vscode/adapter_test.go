package vscode

import (
	"path/filepath"
	"runtime"
	"testing"

	"github.com/gentleman-programming/gentle-ai/internal/model"
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

func TestSupportsSubAgents_ReturnsTrue(t *testing.T) {
	a := NewAdapter()
	if !a.SupportsSubAgents() {
		t.Fatal("SupportsSubAgents() = false, want true")
	}
}

func TestSubAgentsDir_CrossPlatform(t *testing.T) {
	a := NewAdapter()

	tests := []struct {
		name    string
		homeDir string
		want    string
	}{
		{
			name:    "macOS",
			homeDir: "/Users/alice",
			want:    filepath.Join("/Users/alice", ".copilot", "agents"),
		},
		{
			name:    "Linux with default home",
			homeDir: "/home/bob",
			want:    filepath.Join("/home/bob", ".copilot", "agents"),
		},
		{
			name:    "Windows with home dir",
			homeDir: `C:\Users\charlie`,
			want:    filepath.Join(`C:\Users\charlie`, ".copilot", "agents"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := a.SubAgentsDir(tt.homeDir)
			if got != tt.want {
				t.Fatalf("SubAgentsDir(%q) = %q, want %q", tt.homeDir, got, tt.want)
			}
		})
	}
}

func TestEmbeddedSubAgentsDir(t *testing.T) {
	a := NewAdapter()
	got := a.EmbeddedSubAgentsDir()
	if got != "vscode/agents" {
		t.Fatalf("EmbeddedSubAgentsDir() = %q, want %q", got, "vscode/agents")
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
