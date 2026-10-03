// Package commandcode provides Command Code (commandcode.ai) integration.
//
// Detection is platform-aware: bare "cmd" resolves to cmd.exe on Windows and is
// never probed there. The adapter owns ~/.commandcode and claims only
// surfaces verified against a real install.
package commandcode

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

	"github.com/gentleman-programming/gentle-ai/v4/internal/agents/capabilitymanifest"
	"github.com/gentleman-programming/gentle-ai/v4/internal/model"
	"github.com/gentleman-programming/gentle-ai/v4/internal/system"
)

var LookPathOverride = exec.LookPath

type statResult struct {
	isDir bool
	err   error
}

// detectionAliases returns the executable names probed for Command Code, in
// precedence order, for the given GOOS. The distribution publishes cmd, cmdc,
// command-code and commandcode at the same entrypoint; bare "cmd" is cmd.exe on
// Windows, and "commandcode" adds nothing once "command-code" is probed.
func detectionAliases(goos string) []string {
	if goos == "windows" {
		return []string{"command-code", "cmdc"}
	}
	return []string{"command-code", "cmd"}
}

type Adapter struct {
	lookPath func(string) (string, error)
	statPath func(string) statResult
	goos     string
}

func NewAdapter() *Adapter {
	return &Adapter{
		lookPath: LookPathOverride,
		statPath: defaultStat,
		goos:     runtime.GOOS,
	}
}

// --- Identity ---

func (a *Adapter) Agent() model.AgentID {
	return model.AgentCommandCode
}

func (a *Adapter) Tier() model.SupportTier {
	return model.TierFull
}

// --- Detection ---

// Detect probes the platform-aware aliases in precedence order and reports the
// first resolved binary as-is; pnpm-global and npm-global installs can differ.
func (a *Adapter) Detect(_ context.Context, homeDir string) (bool, string, string, bool, error) {
	configPath := ConfigPath(homeDir)

	var binaryPath string
	installed := false
	for _, alias := range detectionAliases(a.goos) {
		found, err := a.lookPath(alias)
		if err == nil {
			binaryPath = found
			installed = true
			break
		}
	}

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
	return capabilitymanifest.MustForAgent(model.AgentCommandCode)
}

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

func (a *Adapter) SystemPromptFile(homeDir string) string {
	return filepath.Join(ConfigPath(homeDir), "AGENTS.md")
}

func (a *Adapter) SkillsDir(homeDir string) string {
	return filepath.Join(ConfigPath(homeDir), "skills")
}

func (a *Adapter) SettingsPath(homeDir string) string {
	return filepath.Join(ConfigPath(homeDir), "settings.json")
}

// --- Config strategies ---

// SystemPromptStrategy injects a managed section into ~/.commandcode/AGENTS.md
// without clobbering user content.
func (a *Adapter) SystemPromptStrategy() model.SystemPromptStrategy {
	return model.StrategyMarkdownSections
}

// MCPStrategy: Command Code reads every MCP server from one ~/.commandcode/mcp.json.
func (a *Adapter) MCPStrategy() model.MCPStrategy {
	return model.StrategyMCPConfigFile
}

// --- MCP ---

func (a *Adapter) MCPConfigPath(homeDir string, _ string) string {
	return filepath.Join(ConfigPath(homeDir), "mcp.json")
}

// --- Optional capabilities ---
//
// Slash commands (~/.commandcode/commands) and native sub-agents
// (~/.commandcode/agents) are deliberately not claimed until their native
// formats are verified against a real install.

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

func (a *Adapter) SubAgentsDir(_ string) string {
	return ""
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

type AgentNotInstallableError struct {
	Agent model.AgentID
}

func (e AgentNotInstallableError) Error() string {
	return fmt.Sprintf("agent %q must be installed manually before Gentle AI can configure it", e.Agent)
}
