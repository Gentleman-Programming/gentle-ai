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

// NewAdapter creates a new Adapter instance for Claude Desktop.
func NewAdapter() *Adapter {
	return &Adapter{statPath: defaultStat}
}

// --- Identity ---

// Agent returns model.AgentClaudeDesktop.
func (a *Adapter) Agent() model.AgentID { return model.AgentClaudeDesktop }

// Tier returns model.TierFull.
func (a *Adapter) Tier() model.SupportTier { return model.TierFull }

// --- Detection ---

// Detect checks for the Claude Desktop config directory or config file.
func (a *Adapter) Detect(_ context.Context, homeDir string) (bool, string, string, bool, error) {
	configDir := a.GlobalConfigDir(homeDir)
	configFile := a.MCPConfigPath(homeDir, "")

	statFile := a.statPath(configFile)
	if statFile.err == nil && !statFile.isDir {
		return true, "", configFile, true, nil
	} else if statFile.err != nil && !os.IsNotExist(statFile.err) {
		return false, "", "", false, statFile.err
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

// SupportsAutoInstall reports whether Claude Desktop supports automated installation.
func (a *Adapter) SupportsAutoInstall() bool { return false }

// InstallCommand returns an error because Claude Desktop is a GUI app and cannot be installed via CLI.
func (a *Adapter) InstallCommand(_ system.PlatformProfile) ([][]string, error) {
	return nil, AgentNotInstallableError{Agent: model.AgentClaudeDesktop}
}

// --- Config paths ---

// GlobalConfigDir returns the OS-specific Claude Desktop User config directory.
func GlobalConfigDir(homeDir string) string {
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

// GlobalConfigDir returns the OS-specific Claude Desktop config directory.
func (a *Adapter) GlobalConfigDir(homeDir string) string {
	return GlobalConfigDir(homeDir)
}

// SystemPromptDir returns the directory containing system prompt instructions for Claude Desktop.
func (a *Adapter) SystemPromptDir(homeDir string) string {
	if !a.SupportsSystemPrompt() {
		return ""
	}
	return GlobalConfigDir(homeDir)
}

// SystemPromptFile returns the path to instructions.md for Claude Desktop.
func (a *Adapter) SystemPromptFile(homeDir string) string {
	if !a.SupportsSystemPrompt() {
		return ""
	}
	return filepath.Join(GlobalConfigDir(homeDir), "instructions.md")
}

// SkillsDir returns the path to the skills directory for Claude Desktop.
func (a *Adapter) SkillsDir(homeDir string) string {
	if !a.SupportsSkills() {
		return ""
	}
	return filepath.Join(GlobalConfigDir(homeDir), "skills")
}

// SettingsPath returns the path to claude_desktop_config.json.
func (a *Adapter) SettingsPath(homeDir string) string {
	return filepath.Join(GlobalConfigDir(homeDir), "claude_desktop_config.json")
}

// --- Config strategies ---

// SystemPromptStrategy returns the system prompt strategy for Claude Desktop.
func (a *Adapter) SystemPromptStrategy() model.SystemPromptStrategy {
	return model.StrategyInstructionsFile
}

// MCPStrategy returns the MCP strategy for Claude Desktop.
func (a *Adapter) MCPStrategy() model.MCPStrategy {
	return model.StrategyMergeIntoSettings
}

// --- MCP ---

// MCPConfigPath returns the path to the MCP configuration file for Claude Desktop.
func (a *Adapter) MCPConfigPath(homeDir string, _ string) string {
	return filepath.Join(GlobalConfigDir(homeDir), "claude_desktop_config.json")
}

// --- Optional capabilities ---

// SupportsOutputStyles reports whether Claude Desktop supports custom output styles.
func (a *Adapter) SupportsOutputStyles() bool { return false }

// OutputStyleDir returns the directory for custom output styles.
func (a *Adapter) OutputStyleDir(_ string) string { return "" }

// SupportsSlashCommands reports whether Claude Desktop supports slash commands.
func (a *Adapter) SupportsSlashCommands() bool { return false }

// CommandsDir returns the directory for slash commands.
func (a *Adapter) CommandsDir(_ string) string { return "" }

// SupportsSubAgents reports whether Claude Desktop supports sub-agents.
func (a *Adapter) SupportsSubAgents() bool { return false }

// SubAgentsDir returns the directory for sub-agents.
func (a *Adapter) SubAgentsDir(_ string) string { return "" }

// EmbeddedSubAgentsDir returns the directory for embedded sub-agents.
func (a *Adapter) EmbeddedSubAgentsDir() string { return "" }

// SupportsSkills reports whether Claude Desktop supports skills.
func (a *Adapter) SupportsSkills() bool { return false }

// SupportsSystemPrompt reports whether Claude Desktop supports system prompts.
func (a *Adapter) SupportsSystemPrompt() bool { return false }

// SupportsMCP reports whether Claude Desktop supports Model Context Protocol.
func (a *Adapter) SupportsMCP() bool { return true }

// AgentNotInstallableError is returned when InstallCommand is called on a desktop-only agent.
type AgentNotInstallableError struct {
	Agent model.AgentID
}

// Error formats the AgentNotInstallableError message.
func (e AgentNotInstallableError) Error() string {
	return "agent " + string(e.Agent) + " is a desktop app and cannot be installed via CLI"
}

func defaultStat(path string) statResult {
	info, err := os.Stat(path)
	return statResult{isDir: err == nil && info.IsDir(), err: err}
}
