package conductor

import (
	"context"
	"os"
	"path/filepath"

	"github.com/gentleman-programming/gentle-ai/internal/model"
	"github.com/gentleman-programming/gentle-ai/internal/system"
)

// Adapter implements agents.Adapter for Conductor (conductor.build).
//
// Conductor is a macOS workspace orchestrator that runs Claude Code agents in
// parallel isolated git worktrees. It does not have its own skills, MCP, or
// system prompt config — all of that flows through the Claude Code adapter
// (~/.claude/). This adapter exists for detection and catalog purposes only.
//
// Detection: ~/.conductor/ is created by Conductor on first launch.
// No binary appears on PATH (desktop app).
type Adapter struct {
	statPath func(string) (os.FileInfo, error)
}

func NewAdapter() *Adapter {
	return &Adapter{statPath: os.Stat}
}

// --- Identity ---

func (a *Adapter) Agent() model.AgentID    { return model.AgentConductor }
func (a *Adapter) Tier() model.SupportTier { return model.TierFull }

// --- Detection ---

// Detect checks for the ~/.conductor directory, which Conductor creates on first launch.
// No binary appears on PATH (desktop app).
func (a *Adapter) Detect(_ context.Context, homeDir string) (bool, string, string, bool, error) {
	configPath := a.GlobalConfigDir(homeDir)
	info, err := a.statPath(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			return false, "", configPath, false, nil
		}
		return false, "", "", false, err
	}
	installed := info.IsDir()
	return installed, "", configPath, installed, nil
}

// --- Installation ---

func (a *Adapter) SupportsAutoInstall() bool { return false }

func (a *Adapter) InstallCommand(_ system.PlatformProfile) ([][]string, error) {
	return nil, AgentNotInstallableError{Agent: model.AgentConductor}
}

// --- Config paths ---

// GlobalConfigDir returns ~/.conductor, the root Conductor config directory.
func (a *Adapter) GlobalConfigDir(homeDir string) string {
	return filepath.Join(homeDir, ".conductor")
}

// Conductor has no standalone system prompt, skills, settings, or MCP config.
// All config is inherited from the Claude Code adapter (~/.claude/).
func (a *Adapter) SystemPromptDir(_ string) string  { return "" }
func (a *Adapter) SystemPromptFile(_ string) string { return "" }
func (a *Adapter) SkillsDir(_ string) string        { return "" }
func (a *Adapter) SettingsPath(_ string) string     { return "" }

// --- Config strategies ---

func (a *Adapter) SystemPromptStrategy() model.SystemPromptStrategy {
	return model.StrategyMarkdownSections
}

func (a *Adapter) MCPStrategy() model.MCPStrategy {
	return model.StrategyMCPConfigFile
}

// --- MCP ---

func (a *Adapter) MCPConfigPath(_ string, _ string) string { return "" }

// --- Optional capabilities ---

// Conductor has no config files of its own — capabilities are all false.
// Config is inherited from the Claude Code agent (~/.claude/).
func (a *Adapter) SupportsOutputStyles() bool     { return false }
func (a *Adapter) OutputStyleDir(_ string) string { return "" }
func (a *Adapter) SupportsSlashCommands() bool    { return false }
func (a *Adapter) CommandsDir(_ string) string    { return "" }
func (a *Adapter) SupportsSubAgents() bool        { return false }
func (a *Adapter) SubAgentsDir(_ string) string   { return "" }
func (a *Adapter) EmbeddedSubAgentsDir() string   { return "" }
func (a *Adapter) SupportsSkills() bool           { return false }
func (a *Adapter) SupportsSystemPrompt() bool     { return false }
func (a *Adapter) SupportsMCP() bool              { return false }

// AgentNotInstallableError is returned when InstallCommand is called on a desktop-only agent.
type AgentNotInstallableError struct {
	Agent model.AgentID
}

func (e AgentNotInstallableError) Error() string {
	return "agent " + string(e.Agent) + " is a desktop app and cannot be installed via CLI"
}
