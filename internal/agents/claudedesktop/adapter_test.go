package claudedesktop

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/gentleman-programming/gentle-ai/internal/model"
)

func TestAdapter_Identity(t *testing.T) {
	adapter := NewAdapter()
	if got := adapter.Agent(); got != model.AgentClaudeDesktop {
		t.Errorf("Agent() = %v, want %v", got, model.AgentClaudeDesktop)
	}
	if got := adapter.Tier(); got != model.TierFull {
		t.Errorf("Tier() = %v, want %v", got, model.TierFull)
	}
}

func TestAdapter_ConfigPaths(t *testing.T) {
	adapter := NewAdapter()
	home := "/home/user"
	if runtime.GOOS == "windows" {
		home = `C:\Users\user`
	}

	configDir := adapter.GlobalConfigDir(home)
	if configDir == "" {
		t.Error("GlobalConfigDir() is empty")
	}

	mcpPath := adapter.MCPConfigPath(home, "")
	if filepath.Base(mcpPath) != "claude_desktop_config.json" {
		t.Errorf("MCPConfigPath() base = %v, want claude_desktop_config.json", filepath.Base(mcpPath))
	}
}

func TestAdapter_Strategies(t *testing.T) {
	adapter := NewAdapter()
	if got := adapter.SystemPromptStrategy(); got != model.StrategyInstructionsFile {
		t.Errorf("SystemPromptStrategy() = %v, want %v", got, model.StrategyInstructionsFile)
	}
	if got := adapter.MCPStrategy(); got != model.StrategyMergeIntoSettings {
		t.Errorf("MCPStrategy() = %v, want %v", got, model.StrategyMergeIntoSettings)
	}
}

func TestAdapter_Capabilities(t *testing.T) {
	adapter := NewAdapter()
	if !adapter.SupportsMCP() {
		t.Error("SupportsMCP() = false, want true")
	}
	if adapter.SupportsSkills() {
		t.Error("SupportsSkills() = true, want false")
	}
	if adapter.SupportsSystemPrompt() {
		t.Error("SupportsSystemPrompt() = true, want false")
	}
	if adapter.SupportsAutoInstall() {
		t.Error("SupportsAutoInstall() = true, want false")
	}
}

func TestAdapter_Detect(t *testing.T) {
	adapter := NewAdapter()
	adapter.statPath = func(p string) statResult {
		if filepath.Base(p) == "claude_desktop_config.json" {
			return statResult{isDir: false, err: nil}
		}
		return statResult{isDir: false, err: os.ErrNotExist}
	}

	installed, _, configPath, found, err := adapter.Detect(context.Background(), "/home/user")
	if err != nil {
		t.Fatalf("Detect() unexpected error: %v", err)
	}
	if !installed || !found {
		t.Errorf("Detect() = (%v, %v), want (true, true)", installed, found)
	}
	if filepath.Base(configPath) != "claude_desktop_config.json" {
		t.Errorf("Detect() configPath = %v, want base claude_desktop_config.json", configPath)
	}
}
