package antigravity

import (
	"context"
	"os"
	"path/filepath"

	"github.com/gentleman-programming/gentle-ai/internal/model"
	"github.com/gentleman-programming/gentle-ai/internal/system"
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

// --- Identity ---

func (a *Adapter) Agent() model.AgentID {
	return model.AgentAntigravity
}

func (a *Adapter) Tier() model.SupportTier {
	return model.TierFull
}

// --- Detection ---

func (a *Adapter) Detect(_ context.Context, homeDir string) (bool, string, string, bool, error) {
	// Attempt to detect the new antigravity-ide directory first, then fallback to antigravity-cli
	paths := []string{
		filepath.Join(homeDir, ".gemini", "antigravity-ide"),
		filepath.Join(homeDir, ".gemini", "antigravity-cli"),
	}

	for _, configPath := range paths {
		stat := a.statPath(configPath)
		if stat.err != nil {
			if !os.IsNotExist(stat.err) {
				// Propagate critical errors immediately
				return false, "", "", false, stat.err
			}
			continue
		}
		if stat.isDir {
			return true, "", configPath, true, nil
		}
	}

	// Default path for fresh installations
	defaultPath := filepath.Join(homeDir, ".gemini", "antigravity-ide")
	return false, "", defaultPath, false, nil
}

// --- Installation ---

func (a *Adapter) SupportsAutoInstall() bool {
	return false
}

func (a *Adapter) InstallCommand(_ system.PlatformProfile) ([][]string, error) {
	return nil, AgentNotInstallableError{Agent: model.AgentAntigravity}
}

// --- Config paths ---

func (a *Adapter) resolveConfigDir(homeDir string) string {
	ideDir := filepath.Join(homeDir, ".gemini", "antigravity-ide")
	stat := a.statPath(ideDir)
	if stat.err == nil && stat.isDir {
		return ideDir
	}

	cliDir := filepath.Join(homeDir, ".gemini", "antigravity-cli")
	stat = a.statPath(cliDir)
	if stat.err == nil && stat.isDir {
		return cliDir
	}

	return ideDir // default fallback
}

func (a *Adapter) GlobalConfigDir(homeDir string) string {
	return a.resolveConfigDir(homeDir)
}

func (a *Adapter) SystemPromptDir(homeDir string) string {
	return filepath.Join(homeDir, ".gemini")
}

func (a *Adapter) SystemPromptFile(homeDir string) string {
	return filepath.Join(homeDir, ".gemini", "GEMINI.md")
}

func (a *Adapter) SkillsDir(homeDir string) string {
	return filepath.Join(a.resolveConfigDir(homeDir), "skills")
}

func (a *Adapter) SettingsPath(homeDir string) string {
	return filepath.Join(a.resolveConfigDir(homeDir), "settings.json")
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
	return filepath.Join(a.resolveConfigDir(homeDir), "mcp_config.json")
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
