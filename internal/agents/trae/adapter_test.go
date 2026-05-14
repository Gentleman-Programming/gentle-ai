package trae

import (
	"context"
	"errors"
	"os"
	"path/filepath"
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

	wantPromptDir := filepath.Join(home, ".trae", "user_rules")
	if got := a.SystemPromptDir(home); got != wantPromptDir {
		t.Fatalf("SystemPromptDir() = %q, want %q", got, wantPromptDir)
	}

	wantPromptFile := filepath.Join(home, ".trae", "user_rules", "gentle-ai.md")
	if got := a.SystemPromptFile(home); got != wantPromptFile {
		t.Fatalf("SystemPromptFile() = %q, want %q", got, wantPromptFile)
	}

	wantSkills := filepath.Join(home, ".trae", "skills")
	if got := a.SkillsDir(home); got != wantSkills {
		t.Fatalf("SkillsDir() = %q, want %q", got, wantSkills)
	}

	wantMCP := filepath.Join(home, ".trae", "mcp.json")
	if got := a.MCPConfigPath(home, "context7"); got != wantMCP {
		t.Fatalf("MCPConfigPath() = %q, want %q", got, wantMCP)
	}

	if got := a.SettingsPath(home); got != "" {
		t.Fatalf("SettingsPath() = %q, want empty string", got)
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
	want := filepath.Join(home, ".trae", "mcp.json")

	if got1 != want {
		t.Fatalf("MCPConfigPath(home, \"\") = %q, want %q", got1, want)
	}
	if got2 != want {
		t.Fatalf("MCPConfigPath(home, \"some-server\") = %q, want %q", got2, want)
	}
}
