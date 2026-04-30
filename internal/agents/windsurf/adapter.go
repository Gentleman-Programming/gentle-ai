package windsurf

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

// Adapter implements agents.Adapter for Windsurf IDE (by Codeium).
//
// Config path summary:
//   - Global AI config (MCP, memories): ~/.codeium/windsurf/
//     → mcp_config.json
//     → memories/global_rules.md
//   - Editor settings (platform-specific):
//     macOS:   ~/Library/Application Support/Windsurf/User/settings.json
//     Linux:   ~/.config/Windsurf/User/settings.json   (respects XDG_CONFIG_HOME)
//     Windows: %APPDATA%\Windsurf\User\settings.json
//
// Detection: Windsurf is a desktop app. We detect it by the presence of
// ~/.codeium/windsurf (the AI config dir), which is created on first launch.
type Adapter struct {
	statPath func(string) statResult
}

func NewAdapter() *Adapter {
	return &Adapter{statPath: defaultStat}
}

// --- Identity ---

func (a *Adapter) Agent() model.AgentID    { return model.AgentWindsurf }
func (a *Adapter) Tier() model.SupportTier { return model.TierFull }

// --- Detection ---

// Detect checks for the ~/.codeium/windsurf directory, which Windsurf creates
// on its first launch. No binary appears on PATH (desktop app).
func (a *Adapter) Detect(_ context.Context, homeDir string) (bool, string, string, bool, error) {
	configPath := a.GlobalConfigDir(homeDir)
	stat := a.statPath(configPath)
	if stat.err != nil {
		if os.IsNotExist(stat.err) {
			return false, "", configPath, false, nil
		}
		return false, "", "", false, stat.err
	}
	return stat.isDir, "", configPath, stat.isDir, nil
}

// --- Installation ---

func (a *Adapter) SupportsAutoInstall() bool { return false }

func (a *Adapter) InstallCommand(_ system.PlatformProfile) ([][]string, error) {
	return nil, AgentNotInstallableError{Agent: model.AgentWindsurf}
}

// --- Config paths ---

// GlobalConfigDir returns the root of Windsurf's AI configuration directory.
// This is cross-platform and always lives under the user's home directory.
func (a *Adapter) GlobalConfigDir(homeDir string) string {
	return filepath.Join(homeDir, ".codeium", "windsurf")
}

// SystemPromptDir returns the directory for global rules/memories.
func (a *Adapter) SystemPromptDir(homeDir string) string {
	return filepath.Join(a.GlobalConfigDir(homeDir), "memories")
}

// SystemPromptFile returns the global rules file that Windsurf Cascade reads.
func (a *Adapter) SystemPromptFile(homeDir string) string {
	return filepath.Join(a.SystemPromptDir(homeDir), "global_rules.md")
}

// SkillsDir returns the skills directory for Windsurf.
func (a *Adapter) SkillsDir(homeDir string) string {
	return filepath.Join(a.GlobalConfigDir(homeDir), "skills")
}

// SettingsPath returns the platform-specific editor settings.json.
func (a *Adapter) SettingsPath(homeDir string) string {
	return filepath.Join(a.windsurfUserDir(homeDir), "settings.json")
}

// windsurfUserDir returns the platform-specific Windsurf User config directory.
// Windsurf follows the same platform conventions as VS Code, substituting
// the application name "Windsurf" for "Code".
func (a *Adapter) windsurfUserDir(homeDir string) string {
	switch runtime.GOOS {
	case "darwin":
		return filepath.Join(homeDir, "Library", "Application Support", "Windsurf", "User")
	case "windows":
		appData := os.Getenv("APPDATA")
		if appData == "" {
			appData = filepath.Join(homeDir, "AppData", "Roaming")
		}
		return filepath.Join(appData, "Windsurf", "User")
	default: // linux and others
		xdgConfigHome := os.Getenv("XDG_CONFIG_HOME")
		if xdgConfigHome == "" {
			xdgConfigHome = filepath.Join(homeDir, ".config")
		}
		return filepath.Join(xdgConfigHome, "Windsurf", "User")
	}
}

// --- Config strategies ---

// SystemPromptStrategy uses AppendToFile: Windsurf's global_rules.md is an
// append-friendly Markdown file rather than a section-marker file.
func (a *Adapter) SystemPromptStrategy() model.SystemPromptStrategy {
	return model.StrategyAppendToFile
}

func (a *Adapter) MCPStrategy() model.MCPStrategy {
	return model.StrategyMCPConfigFile
}

// --- MCP ---

// MCPConfigPath returns the dedicated MCP servers config file.
// Windsurf uses ~/.codeium/windsurf/mcp_config.json across all platforms.
func (a *Adapter) MCPConfigPath(homeDir string, _ string) string {
	return filepath.Join(a.GlobalConfigDir(homeDir), "mcp_config.json")
}

// --- Optional capabilities ---

func (a *Adapter) SupportsOutputStyles() bool     { return false }
func (a *Adapter) OutputStyleDir(_ string) string { return "" }
func (a *Adapter) SupportsSlashCommands() bool    { return false }
func (a *Adapter) CommandsDir(_ string) string    { return "" }
func (a *Adapter) SupportsSubAgents() bool        { return false }
func (a *Adapter) SubAgentsDir(_ string) string   { return "" }
func (a *Adapter) EmbeddedSubAgentsDir() string   { return "" }
func (a *Adapter) SupportsSkills() bool           { return true }
func (a *Adapter) SupportsSystemPrompt() bool     { return true }
func (a *Adapter) SupportsMCP() bool              { return true }

// SupportsWorkflows reports that Windsurf can consume native workflow files
// placed in .windsurf/workflows/ inside the active workspace.
func (a *Adapter) SupportsWorkflows() bool { return true }

// WorkflowsDir returns the target directory for Windsurf native workflows.
// Windsurf reads *.md files from .windsurf/workflows/ in the workspace root.
func (a *Adapter) WorkflowsDir(workspaceDir string) string {
	return filepath.Join(workspaceDir, ".windsurf", "workflows")
}

// EmbeddedWorkflowsDir returns the embedded asset path for Windsurf workflows.
// Implements the workflowInjector interface so sdd.Inject does not need to
// hardcode the agent name when reading from the embedded FS.
func (a *Adapter) EmbeddedWorkflowsDir() string { return "windsurf/workflows" }

// AgentNotInstallableError is returned when InstallCommand is called on a desktop-only agent.
type AgentNotInstallableError struct {
	Agent model.AgentID
}

func (e AgentNotInstallableError) Error() string {
	return "agent " + string(e.Agent) + " is a desktop app and cannot be installed via CLI"
}

func defaultStat(path string) statResult {
	info, err := os.Stat(path)
	if err != nil {
		return statResult{err: err}
	}
	return statResult{isDir: info.IsDir()}
}
