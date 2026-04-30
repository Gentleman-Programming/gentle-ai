package kiro

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

	"github.com/gentleman-programming/gentle-ai/internal/model"
	"github.com/gentleman-programming/gentle-ai/internal/system"
)

type Adapter struct {
	lookPath func(string) (string, error)
	statPath func(string) (os.FileInfo, error)
}

func NewAdapter() *Adapter {
	return &Adapter{
		lookPath: exec.LookPath,
		statPath: os.Stat,
	}
}

// --- Identity ---

func (a *Adapter) Agent() model.AgentID {
	return model.AgentKiroIDE
}

func (a *Adapter) Tier() model.SupportTier {
	return model.TierFull
}

// --- Detection ---

func (a *Adapter) Detect(_ context.Context, homeDir string) (bool, string, string, bool, error) {
	// Kiro IDE is a VS Code fork available as a desktop application.
	// Official website: https://kiro.dev/
	// Detection uses two signals:
	//   1. "kiro" binary on PATH — primary indicator that Kiro is installed.
	//   2. ~/.kiro config dir — returned as configPath so callers/UI can
	//      show the managed directory and configFound reflects filesystem reality.
	//
	// Note: configPath is ~/.kiro (the home-based root where all managed
	// artifacts live), NOT GlobalConfigDir() which points to the OS app-config
	// dir (%APPDATA%\kiro\User on Windows) used only for settings.json.
	configPath := filepath.Join(homeDir, ".kiro")

	binaryPath, err := a.lookPath("kiro")
	if err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			// Binary not found — Kiro is not installed.
			return false, "", configPath, false, nil
		}
		// Unexpected error (permission / IO) — surface it so callers can distinguish.
		return false, "", configPath, false, err
	}

	// Binary found — check whether the config dir already exists.
	info, statErr := a.statPath(configPath)
	configFound := statErr == nil && info.IsDir()

	return true, binaryPath, configPath, configFound, nil
}

// --- Installation ---

func (a *Adapter) SupportsAutoInstall() bool {
	return false // Kiro IDE is a desktop app, installed via official downloads or package managers
}

func (a *Adapter) InstallCommand(_ system.PlatformProfile) ([][]string, error) {
	return nil, AgentNotInstallableError{Agent: model.AgentKiroIDE}
}

// --- Config paths ---
// Kiro IDE (VS Code fork) uses a split-root layout:
//   - Steering/skills/agents/MCP: ~/.kiro/ (home-based, all platforms)
//   - Settings:  macOS: ~/Library/Application Support/Kiro/User/
//               Linux: ~/.config/kiro/user/ (respects XDG_CONFIG_HOME)
//               Windows: %APPDATA%/kiro/User/
// Steering content is written to ~/.kiro/steering/gentle-ai.md via StrategySteeringFile.

func (a *Adapter) GlobalConfigDir(homeDir string) string {
	return a.kiroConfigDir(homeDir)
}

func (a *Adapter) SystemPromptDir(homeDir string) string {
	return filepath.Join(homeDir, ".kiro", "steering")
}

func (a *Adapter) SystemPromptFile(homeDir string) string {
	return filepath.Join(a.SystemPromptDir(homeDir), "gentle-ai.md")
}

func (a *Adapter) SkillsDir(homeDir string) string {
	// Skills are always stored in ~/.kiro/skills/ on all platforms.
	// This is intentionally independent from GlobalConfigDir() — Kiro uses a split-root
	// layout where settings live in the OS app-config dir (e.g. %APPDATA%\kiro\User on
	// Windows) but the IDE reads skills, steering, agents, and MCP from the home-based
	// ~/.kiro/ root. Using GlobalConfigDir() here would make skills invisible in the IDE
	// on Windows.
	return filepath.Join(homeDir, ".kiro", "skills")
}

func (a *Adapter) SettingsPath(homeDir string) string {
	return filepath.Join(a.kiroConfigDir(homeDir), "settings.json")
}

// --- Sub-agent support (Kiro native agents in ~/.kiro/agents/) ---

func (a *Adapter) SupportsSubAgents() bool {
	return true
}

func (a *Adapter) SubAgentsDir(homeDir string) string {
	return filepath.Join(homeDir, ".kiro", "agents")
}

func (a *Adapter) EmbeddedSubAgentsDir() string {
	return "kiro/agents"
}

// KiroModelID resolves a ClaudeModelAlias to a Kiro-native model identifier.
// Used by the SDD injector to stamp the `model:` field in agent frontmatter.
func (a *Adapter) KiroModelID(alias model.ClaudeModelAlias) string {
	return model.KiroModelID(alias)
}

// --- Config strategies ---

func (a *Adapter) SystemPromptStrategy() model.SystemPromptStrategy {
	return model.StrategySteeringFile
}

func (a *Adapter) MCPStrategy() model.MCPStrategy {
	return model.StrategyMCPConfigFile
}

// --- MCP ---

// MCPConfigPath returns the user-level MCP config file.
// Kiro reads MCP configuration from ~/.kiro/settings/mcp.json (user level)
// or .kiro/settings/mcp.json (workspace level). This is separate from the
// app config dir (%APPDATA%/kiro/User on Windows) used for settings and prompts.
func (a *Adapter) MCPConfigPath(homeDir string, _ string) string {
	return filepath.Join(homeDir, ".kiro", "settings", "mcp.json")
}

func (a *Adapter) kiroConfigDir(homeDir string) string {
	switch runtime.GOOS {
	case "darwin":
		// macOS: ~/Library/Application Support/Kiro/User/
		return filepath.Join(homeDir, "Library", "Application Support", "Kiro", "User")
	case "windows":
		// Windows: %APPDATA%/kiro/User/
		appData := os.Getenv("APPDATA")
		if appData == "" {
			appData = filepath.Join(homeDir, "AppData", "Roaming")
		}
		return filepath.Join(appData, "kiro", "User")
	default:
		// Linux and others: ~/.config/kiro/user (respects XDG_CONFIG_HOME)
		xdgConfigHome := os.Getenv("XDG_CONFIG_HOME")
		if xdgConfigHome == "" {
			xdgConfigHome = filepath.Join(homeDir, ".config")
		}
		return filepath.Join(xdgConfigHome, "kiro", "user")
	}
}

// --- Optional capabilities ---

func (a *Adapter) SupportsOutputStyles() bool {
	return false // Kiro IDE output style support not documented
}

func (a *Adapter) OutputStyleDir(_ string) string {
	return ""
}

func (a *Adapter) SupportsSlashCommands() bool {
	return false // Would need to verify if Kiro IDE has slash command support
}

func (a *Adapter) CommandsDir(_ string) string {
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
