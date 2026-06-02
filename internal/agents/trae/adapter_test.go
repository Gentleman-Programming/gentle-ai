package trae

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
			wantConfigPath:  filepath.Join(testHome, ".trae"),
			wantConfigFound: true,
		},
		{
			name:            "config missing",
			stat:            statResult{err: os.ErrNotExist},
			wantInstalled:   false,
			wantConfigPath:  filepath.Join(testHome, ".trae"),
			wantConfigFound: false,
		},
		{
			name:            "config exists but is a file not a dir",
			stat:            statResult{isDir: false},
			wantInstalled:   false,
			wantConfigPath:  filepath.Join(testHome, ".trae"),
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

	wantGlobal := filepath.Join(home, ".trae")
	if got := a.GlobalConfigDir(home); got != wantGlobal {
		t.Fatalf("GlobalConfigDir() = %q, want %q", got, wantGlobal)
	}

	wantSkills := filepath.Join(home, ".trae", "skills")
	if got := a.SkillsDir(home); got != wantSkills {
		t.Fatalf("SkillsDir() = %q, want %q", got, wantSkills)
	}
}

func TestOSSpecificPaths(t *testing.T) {
	a := NewAdapter()
	home := testHome

	tests := []struct {
		name    string
		goos    string
		envVars map[string]string
		wantDir string
	}{
		{
			name:    "macOS",
			goos:    "darwin",
			envVars: map[string]string{},
			wantDir: filepath.Join(home, "Library", "Application Support", "Trae", "User"),
		},
		{
			name:    "Linux default XDG",
			goos:    "linux",
			envVars: map[string]string{},
			wantDir: filepath.Join(home, ".config", "Trae", "User"),
		},
		{
			name:    "Linux custom XDG",
			goos:    "linux",
			envVars: map[string]string{"XDG_CONFIG_HOME": "/custom/config"},
			wantDir: "/custom/config/Trae/User",
		},
		{
			name:    "Windows default APPDATA",
			goos:    "windows",
			envVars: map[string]string{},
			wantDir: filepath.Join(home, "AppData", "Roaming", "Trae", "User"),
		},
		{
			name:    "Windows custom APPDATA",
			goos:    "windows",
			envVars: map[string]string{"APPDATA": "C:\\CustomAppData"},
			wantDir: "C:\\CustomAppData\\Trae\\User",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if runtime.GOOS != tt.goos {
				t.Skipf("Skipping %s test on %s", tt.goos, runtime.GOOS)
			}
			for k, v := range tt.envVars {
				if v != "" {
					t.Setenv(k, v)
				}
			}
			if _, ok := tt.envVars["XDG_CONFIG_HOME"]; !ok && runtime.GOOS == "linux" {
				t.Setenv("XDG_CONFIG_HOME", "")
			}
			if _, ok := tt.envVars["APPDATA"]; !ok && runtime.GOOS == "windows" {
				t.Setenv("APPDATA", "")
			}

			gotDir := a.SystemPromptDir(home)
			if gotDir != tt.wantDir {
				t.Fatalf("SystemPromptDir() = %q, want %q", gotDir, tt.wantDir)
			}
			gotFile := a.SystemPromptFile(home)
			wantFile := filepath.Join(tt.wantDir, "user_rules.md")
			if gotFile != wantFile {
				t.Fatalf("SystemPromptFile() = %q, want %q", gotFile, wantFile)
			}
			gotMCP := a.MCPConfigPath(home, "")
			wantMCP := filepath.Join(tt.wantDir, "mcp.json")
			if gotMCP != wantMCP {
				t.Fatalf("MCPConfigPath() = %q, want %q", gotMCP, wantMCP)
			}
			gotSettings := a.SettingsPath(home)
			wantSettings := filepath.Join(tt.wantDir, "settings.json")
			if gotSettings != wantSettings {
				t.Fatalf("SettingsPath() = %q, want %q", gotSettings, wantSettings)
			}
		})
	}
}

func TestStrategies(t *testing.T) {
	a := NewAdapter()

	if got := a.SystemPromptStrategy(); got != model.StrategyMarkdownSections {
		t.Fatalf("SystemPromptStrategy() = %v, want StrategyMarkdownSections", got)
	}

	if got := a.MCPStrategy(); got != model.StrategyMCPConfigFile {
		t.Fatalf("MCPStrategy() = %v, want StrategyMCPConfigFile", got)
	}
}

func TestAgentIdentity(t *testing.T) {
	a := NewAdapter()

	if got := a.Agent(); got != model.AgentTrae {
		t.Fatalf("Agent() = %v, want %v", got, model.AgentTrae)
	}

	if got := a.Tier(); got != model.TierFull {
		t.Fatalf("Tier() = %v, want %v", got, model.TierFull)
	}
}

func TestCapabilities(t *testing.T) {
	a := NewAdapter()

	if !a.SupportsSkills() {
		t.Fatal("Trae should support skills")
	}
	if !a.SupportsSystemPrompt() {
		t.Fatal("Trae should support system prompt")
	}
	if !a.SupportsMCP() {
		t.Fatal("Trae should support MCP")
	}
	if a.SupportsAutoInstall() {
		t.Fatal("Trae should NOT support auto-install (desktop app)")
	}
	if a.SupportsOutputStyles() {
		t.Fatal("Trae should NOT support output styles")
	}
	if a.SupportsSlashCommands() {
		t.Fatal("Trae should NOT support slash commands")
	}
	if a.SupportsSubAgents() {
		t.Fatal("Trae should NOT support sub-agents")
	}
}

func TestDesktopAppNotAutoInstallable(t *testing.T) {
	a := NewAdapter()

	_, err := a.InstallCommand(system.PlatformProfile{})
	if err == nil {
		t.Fatal("InstallCommand() should return error for desktop app")
	}

	notInstallable, ok := err.(AgentNotInstallableError)
	if !ok {
		t.Fatalf("InstallCommand() error type = %T, want AgentNotInstallableError", err)
	}
	if notInstallable.Agent != model.AgentTrae {
		t.Fatalf("AgentNotInstallableError.Agent = %v, want %v", notInstallable.Agent, model.AgentTrae)
	}
}

func TestMCPConfigPathIgnoresServerName(t *testing.T) {
	a := NewAdapter()
	home := testHome

	got1 := a.MCPConfigPath(home, "")
	got2 := a.MCPConfigPath(home, "some-server")

	if got1 != got2 {
		t.Fatalf("MCPConfigPath() should return same path regardless of server name: %q vs %q", got1, got2)
	}
	if filepath.Base(got1) != "mcp.json" {
		t.Fatalf("MCPConfigPath() filename = %q, want mcp.json", filepath.Base(got1))
	}
}
