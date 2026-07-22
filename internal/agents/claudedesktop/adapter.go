package claudedesktop

import (
	"context"
	"os"
	"path/filepath"
	"runtime"

	"github.com/gentleman-programming/gentle-ai/internal/model"
	"github.com/gentleman-programming/gentle-ai/internal/system"
)

type statResult struct {
	isDir bool
	err   error
}

// Adapter implements agents.Adapter for Claude Desktop (by Anthropic).
//
// Config path summary:
//   - OS-specific Claude User config dir
//     macOS:   ~/Library/Application Support/Claude/
//     Linux:   ~/.config/Claude/   (respects XDG_CONFIG_HOME)
//     Windows: %APPDATA%\Claude\
//     → claude_desktop_config.json   MCP server configs and preferences
//
// Detection: Claude Desktop is a desktop app. If the Claude config directory or file exists, it is detected.
type Adapter struct {
	statPath func(string) statResult
}

func NewAdapter() *Adapter {
	return &Adapter{statPath: defaultStat}
}

// --- Identity ---

func (a *Adapter) Agent() model.AgentID    { return model.AgentClaudeDesktop }
func (a *Adapter) Tier() model.SupportTier { return model.TierFull }

// --- Detection ---

// Detect checks for the Claude Desktop config directory or config file.
func (a *Adapter) Detect(_ context.Context, homeDir string) (bool, string, string, bool, error) {
	configDir := a.GlobalConfigDir(homeDir)
	configFile := a.MCPConfigPath(homeDir, "")

	statFile := a.statPath(configFile)
	if statFile.err == nil && !statFile.isDir {
		return true, "", configFile, true, nil
	}

	statDir := a.statPath(configDir)
	if statDir.err != nil {
		if os.IsNotExist(statDir.err) {
			return false, "", configDir, false, nil
		}
		return false, "", "", false, statDir.err
	}
	return statDir.isDir, "", configDir, statDir.isDir, nil
}

// --- Installation ---

func (a *Adapter) SupportsAutoInstall() bool { return false }

func (a *Adapter) InstallCommand(_ system.PlatformProfile) ([][]string, error) {
	return nil, AgentNotInstallableError{Agent: model.AgentClaudeDesktop}
}

// --- Config paths ---

// GlobalConfigDir returns the OS-specific Claude Desktop config directory.
func (a *Adapter) GlobalConfigDir(homeDir string) string {
	return a.claudeUserDir(homeDir)
}

func (a *Adapter) SystemPromptDir(homeDir string) string {
	return a.claudeUserDir(homeDir)
}

func (a *Adapter) SystemPromptFile(homeDir string) string {
	return filepath.Join(a.claudeUserDir(homeDir), "instructions.md")
}

func (a *Adapter) SkillsDir(homeDir string) string {
	return filepath.Join(a.claudeUserDir(homeDir), "skills")
}

func (a *Adapter) SettingsPath(homeDir string) string {
	return filepath.Join(a.claudeUserDir(homeDir), "claude_desktop_config.json")
}

// --- Config strategies ---

func (a *Adapter) SystemPromptStrategy() model.SystemPromptStrategy {
	return model.StrategyInstructionsFile
}

func (a *Adapter) MCPStrategy() model.MCPStrategy {
	return model.StrategyMergeIntoSettings
}

// --- MCP ---

func (a *Adapter) MCPConfigPath(homeDir string, _ string) string {
	return filepath.Join(a.claudeUserDir(homeDir), "claude_desktop_config.json")
}

// claudeUserDir returns the OS-specific Claude Desktop User config directory.
func (a *Adapter) claudeUserDir(homeDir string) string {
	switch runtime.GOOS {
	case "darwin":
		return filepath.Join(homeDir, "Library", "Application Support", "Claude")
	case "windows":
		appData := os.Getenv("APPDATA")
		if appData == "" {
			appData = filepath.Join(homeDir, "AppData", "Roaming")
		}
		return filepath.Join(appData, "Claude")
	default: // linux and others
		xdgConfigHome := os.Getenv("XDG_CONFIG_HOME")
		if xdgConfigHome == "" {
			xdgConfigHome = filepath.Join(homeDir, ".config")
		}
		return filepath.Join(xdgConfigHome, "Claude")
	}
}

// --- Optional capabilities ---

func (a *Adapter) SupportsOutputStyles() bool     { return false }
func (a *Adapter) OutputStyleDir(_ string) string { return "" }
func (a *Adapter) SupportsSlashCommands() bool    { return false }
func (a *Adapter) CommandsDir(_ string) string    { return "" }
func (a *Adapter) SupportsSubAgents() bool        { return false }
func (a *Adapter) SubAgentsDir(_ string) string   { return "" }
func (a *Adapter) EmbeddedSubAgentsDir() string   { return "" }
func (a *Adapter) SupportsSkills() bool           { return false }
func (a *Adapter) SupportsSystemPrompt() bool     { return false }
func (a *Adapter) SupportsMCP() bool              { return true }

type AgentNotInstallableError struct{ Agent model.AgentID }

func (e AgentNotInstallableError) Error() string {
	return "agent " + string(e.Agent) + " is a desktop app and cannot be installed via CLI"
}

func defaultStat(path string) statResult {
	info, err := os.Stat(path)
	return statResult{isDir: err == nil && info.IsDir(), err: err}
}
