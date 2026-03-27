package windsurf

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/gentleman-programming/gentle-ai/internal/model"
	"github.com/gentleman-programming/gentle-ai/internal/system"
)

const testHome = "/tmp/home"

func TestDetect(t *testing.T) {
	tests := []struct {
		name            string
		stat            statResult
		wantInstalled   bool
		wantConfigPath  string
		wantConfigFound bool
		wantErr         bool
	}{
		{
			name:            "config directory found",
			stat:            statResult{isDir: true},
			wantInstalled:   true,
			wantConfigPath:  filepath.Join(testHome, ".codeium", "windsurf"),
			wantConfigFound: true,
		},
		{
			name:            "config missing",
			stat:            statResult{err: os.ErrNotExist},
			wantInstalled:   false,
			wantConfigPath:  filepath.Join(testHome, ".codeium", "windsurf"),
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
				statPath: func(string) statResult { return tt.stat },
			}
			installed, _, configPath, configFound, err := a.Detect(context.Background(), testHome)
			if (err != nil) != tt.wantErr {
				t.Fatalf("Detect() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			if installed != tt.wantInstalled {
				t.Fatalf("Detect() installed = %v, want %v", installed, tt.wantInstalled)
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

func TestConfigPathsCrossPlatform(t *testing.T) {
	a := NewAdapter()
	home := testHome

	wantGlobal := filepath.Join(home, ".codeium", "windsurf")
	if got := a.GlobalConfigDir(home); got != wantGlobal {
		t.Fatalf("GlobalConfigDir() = %q, want %q", got, wantGlobal)
	}

	wantMCP := filepath.Join(home, ".codeium", "windsurf", "mcp_config.json")
	if got := a.MCPConfigPath(home, "ctx7"); got != wantMCP {
		t.Fatalf("MCPConfigPath() = %q, want %q", got, wantMCP)
	}

	wantSkills := filepath.Join(home, ".codeium", "windsurf", "skills")
	if got := a.SkillsDir(home); got != wantSkills {
		t.Fatalf("SkillsDir() = %q, want %q", got, wantSkills)
	}

	wantPrompt := filepath.Join(home, ".codeium", "windsurf", "memories", "global_rules.md")
	if got := a.SystemPromptFile(home); got != wantPrompt {
		t.Fatalf("SystemPromptFile() = %q, want %q", got, wantPrompt)
	}
}

func TestStrategies(t *testing.T) {
	a := NewAdapter()

	if got := a.SystemPromptStrategy(); got != model.StrategyAppendToFile {
		t.Fatalf("SystemPromptStrategy() = %v, want %v", got, model.StrategyAppendToFile)
	}

	if got := a.MCPStrategy(); got != model.StrategyMCPConfigFile {
		t.Fatalf("MCPStrategy() = %v, want %v", got, model.StrategyMCPConfigFile)
	}
}

func TestCapabilities(t *testing.T) {
	a := NewAdapter()

	if !a.SupportsSkills() {
		t.Fatal("Windsurf should support skills")
	}
	if !a.SupportsSystemPrompt() {
		t.Fatal("Windsurf should support system prompt")
	}
	if !a.SupportsMCP() {
		t.Fatal("Windsurf should support MCP")
	}
	if a.SupportsSlashCommands() {
		t.Fatal("Windsurf should NOT support slash commands")
	}
	if a.SupportsAutoInstall() {
		t.Fatal("Windsurf should NOT support auto-install (desktop app)")
	}
	if !a.SupportsWorkflows() {
		t.Fatal("Windsurf should support native workflows")
	}
}

func TestWorkflowsDir(t *testing.T) {
	a := NewAdapter()

	workspace := "/home/user/myproject"
	got := a.WorkflowsDir(workspace)
	want := filepath.Join(workspace, ".windsurf", "workflows")
	if got != want {
		t.Fatalf("WorkflowsDir(%q) = %q, want %q", workspace, got, want)
	}
}

func TestDesktopAppNotAutoInstallable(t *testing.T) {
	a := NewAdapter()

	if a.SupportsAutoInstall() {
		t.Fatal("Windsurf should not support auto-install (desktop app)")
	}

	_, err := a.InstallCommand(system.PlatformProfile{})
	if err == nil {
		t.Fatal("InstallCommand() should return error for desktop app")
	}
}

func TestAgentIdentity(t *testing.T) {
	a := NewAdapter()

	if got := a.Agent(); got != model.AgentWindsurf {
		t.Fatalf("Agent() = %v, want %v", got, model.AgentWindsurf)
	}

	if got := a.Tier(); got != model.TierFull {
		t.Fatalf("Tier() = %v, want %v", got, model.TierFull)
	}
}

func TestSettingsPathMultiplatform(t *testing.T) {
	home := testHome
	a := NewAdapter()

	tests := []struct {
		name    string
		goos    string
		envVars map[string]string
		want    string
	}{
		{
			name: "Linux with custom XDG_CONFIG_HOME",
			goos: "linux",
			envVars: map[string]string{
				"XDG_CONFIG_HOME": "/custom/config",
			},
			want: "/custom/config/Windsurf/User/settings.json",
		},
		{
			name:    "Linux with default XDG_CONFIG_HOME",
			goos:    "linux",
			envVars: map[string]string{},
			want:    filepath.Join(home, ".config", "Windsurf", "User", "settings.json"),
		},
		{
			name: "Windows with custom APPDATA",
			goos: "windows",
			envVars: map[string]string{
				"APPDATA": "C:\\CustomAppData",
			},
			want: "C:\\CustomAppData\\Windsurf\\User\\settings.json",
		},
		{
			name:    "Windows with default APPDATA",
			goos:    "windows",
			envVars: map[string]string{},
			want:    filepath.Join(home, "AppData", "Roaming", "Windsurf", "User", "settings.json"),
		},
		{
			name:    "macOS ignores environment variables",
			goos:    "darwin",
			envVars: map[string]string{},
			want:    filepath.Join(home, "Library", "Application Support", "Windsurf", "User", "settings.json"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Skip tests for other platforms to avoid runtime.GOOS mismatch
			if runtime.GOOS != tt.goos {
				t.Skipf("Skipping %s test on %s", tt.goos, runtime.GOOS)
			}

			// Set environment variables with proper isolation using t.Setenv
			for key, value := range tt.envVars {
				if value != "" {
					t.Setenv(key, value)
				}
			}

			// Ensure clean environment for variables not in the test case
			if _, exists := tt.envVars["XDG_CONFIG_HOME"]; !exists && runtime.GOOS == "linux" {
				t.Setenv("XDG_CONFIG_HOME", "")
			}
			if _, exists := tt.envVars["APPDATA"]; !exists && runtime.GOOS == "windows" {
				t.Setenv("APPDATA", "")
			}

			got := a.SettingsPath(home)
			if got != tt.want {
				t.Fatalf("SettingsPath() = %q, want %q", got, tt.want)
			}
		})
	}
}
