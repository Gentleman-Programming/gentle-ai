package trae

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/gentleman-programming/gentle-ai/v2/internal/agents/capabilitymanifest"
	"github.com/gentleman-programming/gentle-ai/v2/internal/model"
	"github.com/gentleman-programming/gentle-ai/v2/internal/system"
)

type statResult struct {
	isDir bool
	err   error
}

// Adapter implements agents.Adapter for Trae IDE (by ByteDance).
//
// Config path summary:
//   - Detection / skills: ~/.trae/ (cross-platform, always under home)
//     → skills/                Skill files
//   - Rules / MCP: OS-specific Trae User config dir
//     macOS:   ~/Library/Application Support/Trae/User/
//     Linux:   ~/.config/Trae/User/   (respects XDG_CONFIG_HOME)
//     Windows: %APPDATA%\Trae\User\
//     → user_rules.md          Personal rules (StrategyMarkdownSections)
//     → mcp.json               MCP server configs
//
// Detection: Trae is a desktop app. If ~/.trae exists as a directory, it's installed.
// No binary appears on PATH.
type Adapter struct {
	statPath func(string) statResult
}

func NewAdapter() *Adapter {
	return &Adapter{statPath: defaultStat}
}

// --- Identity ---

func (a *Adapter) Agent() model.AgentID    { return model.AgentTrae }
func (a *Adapter) Tier() model.SupportTier { return model.TierFull }

// --- Detection ---

// Detect checks for the ~/.trae directory, which Trae creates on its first launch.
// No binary appears on PATH (desktop app).
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

func (a *Adapter) CapabilityManifest() capabilitymanifest.AgentCapabilityManifest {
	return capabilitymanifest.MustForAgent(model.AgentTrae)
}

func (a *Adapter) InstallCommand(_ system.PlatformProfile) ([][]string, error) {
	return nil, AgentNotInstallableError{Agent: model.AgentTrae}
}

// --- Config paths ---

// GlobalConfigDir returns ~/.trae, the root of Trae's config directory.
// Trae uses a flat cross-platform layout with no OS-specific split.
func (a *Adapter) GlobalConfigDir(homeDir string) string {
	return filepath.Join(homeDir, ".trae")
}

// SystemPromptDir returns the OS-specific Trae User config directory,
// which is where user_rules.md lives.
func (a *Adapter) SystemPromptDir(homeDir string) string {
	return a.traeUserDir(homeDir)
}

// SystemPromptFile returns the personal rules file that Trae reads.
// gentle-ai injects its sections via StrategyMarkdownSections markers.
func (a *Adapter) SystemPromptFile(homeDir string) string {
	return filepath.Join(a.traeUserDir(homeDir), "user_rules.md")
}

// SkillsDir returns the skills directory for Trae.
func (a *Adapter) SkillsDir(homeDir string) string {
	return filepath.Join(a.GlobalConfigDir(homeDir), "skills")
}

// SettingsPath returns the platform-specific editor settings.json.
func (a *Adapter) SettingsPath(homeDir string) string {
	return filepath.Join(a.traeUserDir(homeDir), "settings.json")
}

// --- Config strategies ---

// SystemPromptStrategy uses MarkdownSections: gentle-ai markers are injected
// into user_rules/gentle-ai.md without clobbering other user content.
func (a *Adapter) SystemPromptStrategy() model.SystemPromptStrategy {
	return model.StrategyMarkdownSections
}

func (a *Adapter) MCPStrategy() model.MCPStrategy {
	return model.StrategyMCPConfigFile
}

// --- MCP ---

// MCPConfigPath returns the MCP servers config file.
// Trae uses {traeUserDir}/mcp.json — same format as Cursor (mcpServers object).
func (a *Adapter) MCPConfigPath(homeDir string, _ string) string {
	return filepath.Join(a.traeUserDir(homeDir), "mcp.json")
}

// traeUserDir returns the OS-specific Trae User config directory.
// Trae follows VS Code conventions substituting "Trae" for "Code".
//
// Environment overrides (XDG_CONFIG_HOME, APPDATA) are honored only when
// homeDir is the real user home: a caller that passes a custom installation
// root (sandboxed installs, tests) must stay contained inside that root, so
// ambient environment can never redirect a write outside it.
func (a *Adapter) traeUserDir(homeDir string) string {
	switch runtime.GOOS {
	case "darwin":
		return filepath.Join(homeDir, "Library", "Application Support", "Trae", "User")
	case "windows":
		if appData := strings.TrimSpace(os.Getenv("APPDATA")); filepath.IsAbs(appData) && isRealUserHome(homeDir) {
			return filepath.Join(appData, "Trae", "User")
		}
		return filepath.Join(homeDir, "AppData", "Roaming", "Trae", "User")
	default: // linux and others
		if xdgConfigHome := strings.TrimSpace(os.Getenv("XDG_CONFIG_HOME")); filepath.IsAbs(xdgConfigHome) && isRealUserHome(homeDir) {
			return filepath.Join(xdgConfigHome, "Trae", "User")
		}
		return filepath.Join(homeDir, ".config", "Trae", "User")
	}
}

// isRealUserHome reports whether homeDir is the current user's actual home
// directory — the only case where process-wide environment overrides may
// legitimately steer config resolution away from homeDir.
func isRealUserHome(homeDir string) bool {
	userHome, err := os.UserHomeDir()
	return err == nil && filepath.Clean(homeDir) == filepath.Clean(userHome)
}

// --- Optional capabilities ---

func (a *Adapter) SupportsOutputStyles() bool {
	return a.CapabilityManifest().Features.OutputStyles
}
func (a *Adapter) OutputStyleDir(_ string) string { return "" }
func (a *Adapter) SupportsSlashCommands() bool {
	return a.CapabilityManifest().Features.SlashCommands
}
func (a *Adapter) CommandsDir(_ string) string { return "" }
func (a *Adapter) SupportsSubAgents() bool {
	return a.CapabilityManifest().Features.FileSubAgents
}
func (a *Adapter) SubAgentsDir(_ string) string { return "" }
func (a *Adapter) EmbeddedSubAgentsDir() string { return "" }
func (a *Adapter) SupportsSkills() bool {
	return a.CapabilityManifest().Features.Skills
}
func (a *Adapter) SupportsSystemPrompt() bool {
	return a.CapabilityManifest().Features.SystemPrompt
}
func (a *Adapter) SupportsMCP() bool {
	return a.CapabilityManifest().Features.MCP
}

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
