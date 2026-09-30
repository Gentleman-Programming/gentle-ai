package minimaxcode

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/gentleman-programming/gentle-ai/v4/internal/agents/capabilitymanifest"
	"github.com/gentleman-programming/gentle-ai/v4/internal/model"
	"github.com/gentleman-programming/gentle-ai/v4/internal/system"
)

// LookPathOverride is the exec.LookPath seam used by tests.
var LookPathOverride = exec.LookPath

// binaryName is the public terminal command for the MiniMax Code CLI/TUI.
const binaryName = "mcode"

type statResult struct {
	isDir bool
	err   error
}

type Adapter struct {
	lookPath func(string) (string, error)
	statPath func(string) statResult
}

func NewAdapter() *Adapter {
	return &Adapter{
		lookPath: LookPathOverride,
		statPath: defaultStat,
	}
}

// --- Identity ---

func (a *Adapter) Agent() model.AgentID {
	return model.AgentMiniMaxCode
}

func (a *Adapter) Tier() model.SupportTier {
	return model.TierFull
}

// --- Detection ---

// Detect treats the CLI binary and the agent data directory as two independent
// signals, because MiniMax Code ships as two separate installs that share one
// data directory:
//
//   - Desktop (Electron) writes ~/.minimax but does not put `mcode` on PATH.
//   - CLI/TUI (npm `@minimax-ai/code`) puts `mcode` on PATH and uses the same
//     ~/.minimax directory.
//
// Neither signal alone is sufficient, so a Desktop-only machine still reports
// the agent and a CLI-only machine does too.
func (a *Adapter) Detect(_ context.Context, homeDir string) (bool, string, string, bool, error) {
	configPath := ConfigPath(homeDir)

	binaryPath, err := a.lookPath(binaryName)
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

func (a *Adapter) CapabilityManifest() capabilitymanifest.AgentCapabilityManifest {
	return capabilitymanifest.MustForAgent(model.AgentMiniMaxCode)
}

// InstallCommand reports that the client is installed manually. MiniMax Code is
// distributed through its own desktop installer, npm, and a shell installer;
// Gentle AI configures an agent the user already has rather than installing it.
func (a *Adapter) InstallCommand(_ system.PlatformProfile) ([][]string, error) {
	return nil, AgentNotInstallableError{Agent: a.Agent()}
}

// --- Config paths ---

func (a *Adapter) GlobalConfigDir(homeDir string) string {
	return ConfigPath(homeDir)
}

func (a *Adapter) SystemPromptDir(homeDir string) string {
	return ConfigPath(homeDir)
}

// SystemPromptFile points at the managed agent definition. MiniMax Code loads
// per-agent instructions from ~/.minimax/agents/<agent>/agent.md, the same
// layout the built-in agents use.
func (a *Adapter) SystemPromptFile(homeDir string) string {
	return filepath.Join(ConfigPath(homeDir), "agents", ManagedAgentName, "agent.md")
}

// ManagedAgentName is the agent directory Gentle AI owns inside the shared
// MiniMax data directory. It is namespaced so managed instructions never
// collide with the runtime's built-in agents (.builtin/) or user-authored ones.
const ManagedAgentName = "gentle-ai"

// SkillsDir is the user-level skills location. The runtime's own bundled
// skills live in .builtin-skills/, which is reserved for the runtime and is
// never written by Gentle AI.
func (a *Adapter) SkillsDir(homeDir string) string {
	return filepath.Join(ConfigPath(homeDir), "skills")
}

// SettingsPath is the agent's own configuration file. Gentle AI never writes
// managed keys into it: MCP goes to the dedicated mcp.json, the prompt to
// agents/gentle-ai/agent.md, and skills to skills/. Returning "" tells the
// installer and the uninstaller that there is no managed settings surface,
// which also keeps the uninstall's JSON settings cleaner from being pointed at
// a YAML file it cannot parse.
func (a *Adapter) SettingsPath(_ string) string {
	return ""
}

// --- Config strategies ---

func (a *Adapter) SystemPromptStrategy() model.SystemPromptStrategy {
	return model.StrategyMarkdownSections
}

// MCPStrategy writes to the dedicated mcp.json file. MiniMax Code stores MCP
// servers in ~/.minimax/mcp.json using the same mcpServers object and the same
// transports as Claude Code, so the existing JSON merge applies unchanged.
func (a *Adapter) MCPStrategy() model.MCPStrategy {
	return model.StrategyMCPConfigFile
}

// --- MCP ---

// MCPConfigPath returns ~/.minimax/mcp.json for every server. MiniMax Code
// keeps all MCP servers in one file at the data-directory root, so serverName
// is intentionally ignored. The file is created by the runtime on first use.
func (a *Adapter) MCPConfigPath(homeDir string, _ string) string {
	return filepath.Join(ConfigPath(homeDir), "mcp.json")
}

// --- Optional capabilities ---

func (a *Adapter) SupportsOutputStyles() bool {
	return a.CapabilityManifest().Features.OutputStyles
}

func (a *Adapter) OutputStyleDir(_ string) string {
	return ""
}

func (a *Adapter) SupportsSlashCommands() bool {
	return a.CapabilityManifest().Features.SlashCommands
}

func (a *Adapter) CommandsDir(_ string) string {
	return ""
}

func (a *Adapter) SupportsSubAgents() bool {
	return a.CapabilityManifest().Features.FileSubAgents
}

// SubAgentsDir is the root under which per-agent markdown lives. The adapter
// writes a single namespaced agent, so this is informational.
func (a *Adapter) SubAgentsDir(homeDir string) string {
	return filepath.Join(ConfigPath(homeDir), "agents")
}

func (a *Adapter) EmbeddedSubAgentsDir() string {
	return ""
}

func (a *Adapter) SupportsSkills() bool {
	return a.CapabilityManifest().Features.Skills
}

func (a *Adapter) SupportsSystemPrompt() bool {
	return a.CapabilityManifest().Features.SystemPrompt
}

func (a *Adapter) SupportsMCP() bool {
	return a.CapabilityManifest().Features.MCP
}

func defaultStat(path string) statResult {
	info, err := os.Stat(path)
	if err != nil {
		return statResult{err: err}
	}

	return statResult{isDir: info.IsDir()}
}

// ConfigPath returns the MiniMax Code agent data directory, ~/.minimax. It is
// shared by the Desktop app, the CLI/TUI, and the web surface, which is why one
// adapter covers all of them.
func ConfigPath(homeDir string) string {
	return filepath.Join(homeDir, ".minimax")
}

type AgentNotInstallableError struct {
	Agent model.AgentID
}

func (e AgentNotInstallableError) Error() string {
	return fmt.Sprintf("agent %q must be installed manually before Gentle AI can configure it", e.Agent)
}
