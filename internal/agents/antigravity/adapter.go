package antigravity

import (
	"context"
	"os"
	"path/filepath"

	"github.com/gentleman-programming/gentle-ai/v2/internal/model"
	"github.com/gentleman-programming/gentle-ai/v2/internal/system"
)

type statResult struct {
	isDir bool
	err   error
}

type Adapter struct {
	statPath func(string) statResult
}

func NewAdapter() *Adapter {
	return &Adapter{
		statPath: defaultStat,
	}
}

// antigravityVariantDir retains Gentle AI's legacy settings/skills selection.
func (a *Adapter) antigravityVariantDir(homeDir string) string {
	desktop := filepath.Join(homeDir, ".gemini", "antigravity-desktop")
	if stat := a.statPath(desktop); stat.err == nil {
		return desktop
	}
	return filepath.Join(homeDir, ".gemini", "antigravity-cli")
}

func (a *Adapter) antigravityRoot(homeDir string) string {
	return filepath.Join(homeDir, ".gemini", "antigravity")
}

func (a *Adapter) migratedMarker(homeDir string) string {
	return filepath.Join(homeDir, ".gemini", "config", ".migrated")
}

// --- Identity ---

func (a *Adapter) Agent() model.AgentID {
	return model.AgentAntigravity
}

func (a *Adapter) Tier() model.SupportTier {
	return model.TierFull
}

// --- Detection ---

func (a *Adapter) Detect(_ context.Context, homeDir string) (bool, string, string, bool, error) {
	configPath := a.GlobalConfigDir(homeDir)
	for _, evidence := range []string{a.antigravityRoot(homeDir), a.migratedMarker(homeDir), a.antigravityVariantDir(homeDir)} {
		stat := a.statPath(evidence)
		if stat.err == nil {
			return true, "", configPath, true, nil
		}
		if !os.IsNotExist(stat.err) {
			return false, "", "", false, stat.err
		}
	}
	return false, "", configPath, false, nil
}

// --- Installation ---

func (a *Adapter) SupportsAutoInstall() bool {
	return false
}

func (a *Adapter) InstallCommand(_ system.PlatformProfile) ([][]string, error) {
	return nil, AgentNotInstallableError{Agent: model.AgentAntigravity}
}

// --- Config paths ---

func (a *Adapter) GlobalConfigDir(homeDir string) string {
	if stat := a.statPath(a.migratedMarker(homeDir)); stat.err == nil {
		return filepath.Join(homeDir, ".gemini", "config")
	}
	if stat := a.statPath(a.antigravityRoot(homeDir)); stat.err == nil {
		return a.antigravityRoot(homeDir)
	}
	if stat := a.statPath(a.antigravityVariantDir(homeDir)); stat.err == nil {
		return a.antigravityVariantDir(homeDir)
	}
	return a.antigravityRoot(homeDir)
}

func (a *Adapter) SystemPromptDir(homeDir string) string {
	return filepath.Join(homeDir, ".gemini")
}

func (a *Adapter) SystemPromptFile(homeDir string) string {
	return filepath.Join(homeDir, ".gemini", "GEMINI.md")
}

func (a *Adapter) SkillsDir(homeDir string) string {
	return filepath.Join(a.antigravityVariantDir(homeDir), "skills")
}

func (a *Adapter) SettingsPath(homeDir string) string {
	return filepath.Join(a.antigravityVariantDir(homeDir), "settings.json")
}

// --- Config strategies ---

func (a *Adapter) SystemPromptStrategy() model.SystemPromptStrategy {
	return model.StrategyAppendToFile
}

func (a *Adapter) MCPStrategy() model.MCPStrategy {
	return model.StrategyMCPConfigFile
}

// --- MCP ---

func (a *Adapter) MCPConfigPath(homeDir string, _ string) string {
	return filepath.Join(a.antigravityVariantDir(homeDir), "mcp_config.json")
}

// --- Optional capabilities ---

func (a *Adapter) SupportsOutputStyles() bool {
	return false
}

func (a *Adapter) OutputStyleDir(_ string) string {
	return ""
}

func (a *Adapter) SupportsSlashCommands() bool {
	return false
}

func (a *Adapter) CommandsDir(_ string) string {
	return ""
}

func (a *Adapter) SupportsSubAgents() bool {
	return false
}

func (a *Adapter) SubAgentsDir(_ string) string {
	return ""
}

func (a *Adapter) EmbeddedSubAgentsDir() string {
	return ""
}

func (a *Adapter) SupportsSkills() bool {
	return true
}

func (a *Adapter) SupportsSystemPrompt() bool {
	return true
}

func (a *Adapter) SupportsMCP() bool {
	return true
}

type AgentNotInstallableError struct {
	Agent model.AgentID
}

func (e AgentNotInstallableError) Error() string {
	return "agent " + string(e.Agent) + " is managed by Antigravity and cannot be auto-installed"
}

func defaultStat(path string) statResult {
	info, err := os.Stat(path)
	if err != nil {
		return statResult{err: err}
	}

	return statResult{isDir: info.IsDir()}
}
