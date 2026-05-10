package claude

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/gentleman-programming/gentle-ai/internal/installcmd"
	"github.com/gentleman-programming/gentle-ai/internal/model"
	"github.com/gentleman-programming/gentle-ai/internal/system"
)

var LookPathOverride = exec.LookPath

type statResult struct {
	isDir bool
	err   error
}

type Adapter struct {
	lookPath func(string) (string, error)
	statPath func(string) statResult
	resolver installcmd.Resolver
}

func NewAdapter() *Adapter {
	return &Adapter{
		lookPath: LookPathOverride,
		statPath: defaultStat,
		resolver: installcmd.NewResolver(),
	}
}

// --- Identity ---

func (a *Adapter) Agent() model.AgentID {
	return model.AgentClaudeCode
}

func (a *Adapter) Tier() model.SupportTier {
	return model.TierFull
}

// --- Detection ---

func (a *Adapter) Detect(_ context.Context, homeDir string) (bool, string, string, bool, error) {
	configPath := ConfigPath(homeDir)

	binaryPath, err := a.lookPath("claude")
	installed := err == nil

	stat := a.statPath(configPath)
	if stat.err != nil {
		if os.IsNotExist(stat.err) {
			return installed, binaryPath, configPath, false, nil
		}
		return false, "", "", false, stat.err
	}

	return installed, binaryPath, configPath, stat.isDir, nil
}

// --- Installation ---

func (a *Adapter) SupportsAutoInstall() bool {
	return true
}

func (a *Adapter) InstallCommand(profile system.PlatformProfile) ([][]string, error) {
	resolver := a.resolver
	if resolver == nil {
		resolver = installcmd.NewResolver()
	}

	return resolver.ResolveAgentInstall(profile, a.Agent())
}

// --- Config paths ---

func (a *Adapter) GlobalConfigDir(homeDir string) string {
	return filepath.Join(homeDir, ".claude")
}

func (a *Adapter) SystemPromptDir(homeDir string) string {
	return filepath.Join(homeDir, ".claude")
}

func (a *Adapter) SystemPromptFile(homeDir string) string {
	return filepath.Join(homeDir, ".claude", "CLAUDE.md")
}

func (a *Adapter) SkillsDir(homeDir string) string {
	return filepath.Join(homeDir, ".claude", "skills")
}

func (a *Adapter) SettingsPath(homeDir string) string {
	return filepath.Join(homeDir, ".claude", "settings.json")
}

// --- Config strategies ---

func (a *Adapter) SystemPromptStrategy() model.SystemPromptStrategy {
	return model.StrategyMarkdownSections
}

func (a *Adapter) MCPStrategy() model.MCPStrategy {
	return model.StrategySeparateMCPFiles
}

// --- MCP ---

func (a *Adapter) MCPConfigPath(homeDir string, serverName string) string {
	return filepath.Join(homeDir, ".claude", "mcp", serverName+".json")
}

// --- Optional capabilities ---

func (a *Adapter) SupportsOutputStyles() bool {
	return true
}

func (a *Adapter) OutputStyleDir(homeDir string) string {
	return filepath.Join(homeDir, ".claude", "output-styles")
}

func (a *Adapter) SupportsSlashCommands() bool {
	return true
}

func (a *Adapter) CommandsDir(homeDir string) string {
	return filepath.Join(homeDir, ".claude", "commands")
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

// --- Sub-agent support ---
//
// Claude Code loads agent files from ~/.claude/agents/*.md. Each file carries
// frontmatter (name, description, tools, model) and a prompt body. The SDD
// component copies the embedded set at install time, resolving the
// {{CLAUDE_MODEL}} placeholder in each file against the user's model
// assignments so the per-phase model contract is enforced at the agent layer
// rather than relying on orchestrator prose.

func (a *Adapter) SupportsSubAgents() bool {
	return true
}

func (a *Adapter) SubAgentsDir(homeDir string) string {
	return filepath.Join(homeDir, ".claude", "agents")
}

func (a *Adapter) EmbeddedSubAgentsDir() string {
	return "claude/agents"
}

// ClaudeModelID resolves a ClaudeModelAlias to the string Claude Code accepts
// in the `model:` frontmatter field of a sub-agent file. Claude Code uses the
// aliases ("opus", "sonnet", "haiku") verbatim, so this is an identity over
// alias.String(). Implemented as a method so the SDD injector's
// claudeModelResolver type assertion fires for this adapter.
func (a *Adapter) ClaudeModelID(alias model.ClaudeModelAlias) string {
	return alias.String()
}

func defaultStat(path string) statResult {
	info, err := os.Stat(path)
	if err != nil {
		return statResult{err: err}
	}

	return statResult{isDir: info.IsDir()}
}
