package copilotcli

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/gentleman-programming/gentle-ai/v2/internal/agents/capabilitymanifest"
	"github.com/gentleman-programming/gentle-ai/v2/internal/copilotconfig"
	"github.com/gentleman-programming/gentle-ai/v2/internal/model"
	"github.com/gentleman-programming/gentle-ai/v2/internal/system"
)

var LookPathOverride = exec.LookPath

type statResult struct {
	isDir bool
	err   error
}

// Adapter implements agents.Adapter for GitHub Copilot CLI.
type Adapter struct {
	lookPath func(string) (string, error)
	statPath func(string) statResult
}

func NewAdapter() *Adapter {
	return &Adapter{lookPath: LookPathOverride, statPath: defaultStat}
}

func (a *Adapter) Agent() model.AgentID    { return model.AgentGitHubCopilotCLI }
func (a *Adapter) Tier() model.SupportTier { return model.TierFull }
func (a *Adapter) CapabilityManifest() capabilitymanifest.AgentCapabilityManifest {
	return capabilitymanifest.MustForAgent(model.AgentGitHubCopilotCLI)
}

func (a *Adapter) Detect(_ context.Context, homeDir string) (bool, string, string, bool, error) {
	configPath := copilotconfig.Root(homeDir)
	binaryPath, err := a.lookPath("copilot")
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

func (a *Adapter) InstallCommand(profile system.PlatformProfile) ([][]string, error) {
	const pkg = "@github/copilot@latest"
	if profile.OS == "linux" && !profile.NpmWritable {
		return [][]string{{"sudo", "npm", "install", "-g", pkg}}, nil
	}
	return [][]string{{"npm", "install", "-g", pkg}}, nil
}

func (a *Adapter) GlobalConfigDir(homeDir string) string { return copilotconfig.Root(homeDir) }
func (a *Adapter) SystemPromptDir(homeDir string) string { return copilotconfig.Root(homeDir) }
func (a *Adapter) SystemPromptFile(homeDir string) string {
	return filepath.Join(copilotconfig.Root(homeDir), "copilot-instructions.md")
}
func (a *Adapter) SkillsDir(homeDir string) string {
	return filepath.Join(copilotconfig.Root(homeDir), "skills")
}
func (a *Adapter) SettingsPath(homeDir string) string {
	return filepath.Join(copilotconfig.Root(homeDir), "settings.json")
}
func (a *Adapter) MCPConfigPath(homeDir string, _ string) string {
	return filepath.Join(copilotconfig.Root(homeDir), "mcp-config.json")
}

func (a *Adapter) SystemPromptStrategy() model.SystemPromptStrategy { return model.StrategyFileReplace }
func (a *Adapter) MCPStrategy() model.MCPStrategy                   { return model.StrategyMCPConfigFile }

func (a *Adapter) SupportsOutputStyles() bool     { return a.CapabilityManifest().Features.OutputStyles }
func (a *Adapter) OutputStyleDir(_ string) string { return "" }
func (a *Adapter) SupportsSlashCommands() bool    { return a.CapabilityManifest().Features.SlashCommands }
func (a *Adapter) CommandsDir(_ string) string    { return "" }
func (a *Adapter) SupportsSubAgents() bool        { return a.CapabilityManifest().Features.FileSubAgents }
func (a *Adapter) SubAgentsDir(_ string) string   { return "" }
func (a *Adapter) EmbeddedSubAgentsDir() string   { return "" }
func (a *Adapter) SupportsSkills() bool           { return a.CapabilityManifest().Features.Skills }
func (a *Adapter) SupportsSystemPrompt() bool     { return a.CapabilityManifest().Features.SystemPrompt }
func (a *Adapter) SupportsMCP() bool              { return a.CapabilityManifest().Features.MCP }

func defaultStat(path string) statResult {
	info, err := os.Stat(path)
	if err != nil {
		return statResult{err: err}
	}
	return statResult{isDir: info.IsDir()}
}
