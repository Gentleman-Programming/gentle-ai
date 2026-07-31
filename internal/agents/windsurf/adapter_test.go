package windsurf

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/model"
	"github.com/gentleman-programming/gentle-ai/v2/internal/system"
)

const testHome = "/tmp/home"

func TestDetect(t *testing.T) {
	tests := []struct {
		name            string
		devinStat       statResult
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
			name:            "Devin config directory found",
			devinStat:       statResult{isDir: true},
			wantInstalled:   true,
			wantConfigPath:  filepath.Join(testHome, ".codeium", "devin"),
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
			devinPath := filepath.Join(testHome, ".codeium", "devin")
			legacyPath := filepath.Join(testHome, ".codeium", "windsurf")
			a := &Adapter{statPath: func(path string) statResult {
				if path == devinPath {
					return tt.devinStat
				}
				if path != legacyPath {
					t.Errorf("statPath() path = %q, want %q", path, legacyPath)
				}
				return tt.stat
			}}
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
	a := &Adapter{statPath: func(string) statResult { return statResult{err: os.ErrNotExist} }}
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

func TestGlobalConfigDirPrefersDevin(t *testing.T) {
	home := testHome
	devinPath := filepath.Join(home, ".codeium", "devin")
	legacyPath := filepath.Join(home, ".codeium", "windsurf")

	tests := []struct {
		name string
		stat statResult
		want string
	}{
		{name: "Devin directory selected", stat: statResult{isDir: true}, want: devinPath},
		{name: "missing Devin falls back", stat: statResult{err: os.ErrNotExist}, want: legacyPath},
		{name: "non-directory Devin falls back", stat: statResult{}, want: legacyPath},
		{name: "Devin stat error falls back", stat: statResult{err: errors.New("permission denied")}, want: legacyPath},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := &Adapter{statPath: func(path string) statResult {
				if path != devinPath {
					t.Errorf("statPath() path = %q, want %q", path, devinPath)
				}
				return tt.stat
			}}
			if got := a.GlobalConfigDir(home); got != tt.want {
				t.Fatalf("GlobalConfigDir() = %q, want %q", got, tt.want)
			}
		})
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

	platforms := []struct {
		name     string
		goos     string
		env      map[string]string
		basePath string
	}{
		{
			name: "Linux with custom XDG_CONFIG_HOME",
			goos: "linux",
			env: map[string]string{
				"XDG_CONFIG_HOME": filepath.Join(home, "custom-config"),
			},
			basePath: filepath.Join(home, "custom-config"),
		},
		{
			name:     "Linux with default XDG_CONFIG_HOME",
			goos:     "linux",
			env:      map[string]string{},
			basePath: filepath.Join(home, ".config"),
		},
		{
			name: "Windows with custom APPDATA",
			goos: "windows",
			env: map[string]string{
				"APPDATA": filepath.Join(home, "custom-appdata"),
			},
			basePath: filepath.Join(home, "custom-appdata"),
		},
		{
			name:     "Windows with default APPDATA",
			goos:     "windows",
			env:      map[string]string{},
			basePath: filepath.Join(home, "AppData", "Roaming"),
		},
		{
			name:     "macOS",
			goos:     "darwin",
			env:      map[string]string{},
			basePath: filepath.Join(home, "Library", "Application Support"),
		},
	}

	statuses := []struct {
		name string
		stat statResult
		app  string
	}{
		{name: "Devin directory selected", stat: statResult{isDir: true}, app: "Devin"},
		{name: "missing Devin falls back", stat: statResult{err: os.ErrNotExist}, app: "Windsurf"},
		{name: "non-directory Devin falls back", stat: statResult{}, app: "Windsurf"},
		{name: "Devin stat error falls back", stat: statResult{err: errors.New("permission denied")}, app: "Windsurf"},
	}

	for _, platform := range platforms {
		for _, status := range statuses {
			t.Run(platform.name+"/"+status.name, func(t *testing.T) {
				devinPath := filepath.Join(platform.basePath, "Devin", "User")
				a := &Adapter{
					statPath: func(path string) statResult {
						if path != devinPath {
							t.Errorf("statPath() path = %q, want %q", path, devinPath)
						}
						return status.stat
					},
					goos: platform.goos,
					getenv: func(key string) string {
						return platform.env[key]
					},
				}
				want := filepath.Join(platform.basePath, status.app, "User", "settings.json")
				if got := a.SettingsPath(home); got != want {
					t.Fatalf("SettingsPath() = %q, want %q", got, want)
				}
			})
		}
	}
}
