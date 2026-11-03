package antigravity

import (
	"context"
	"os"
	"path/filepath"

	"github.com/gentleman-programming/gentle-ai/v4/internal/agents/capabilitymanifest"
	"github.com/gentleman-programming/gentle-ai/v4/internal/model"
	"github.com/gentleman-programming/gentle-ai/v4/internal/system"
)

type statResult struct {
	isDir bool
	err   error
}

type Adapter struct {
	statPath func(string) statResult
}

func NewAdapter() *Adapter {
	return &Adapter{
		statPath: defaultStat,
	}
}

// antigravityVariantDir selects the variant-private directory that holds
// settings.json. Customizations live in the shared root instead.
func (a *Adapter) antigravityVariantDir(homeDir string) string {
	desktop := filepath.Join(homeDir, ".gemini", "antigravity-desktop")
	if stat := a.statPath(desktop); stat.err == nil {
		return desktop
	}
	return filepath.Join(homeDir, ".gemini", "antigravity-cli")
}

func (a *Adapter) antigravityRoot(homeDir string) string {
	return filepath.Join(homeDir, ".gemini", "antigravity")
}

// sharedConfigDir is the customization root shared by the Antigravity CLI
// (agy 1.4+) and desktop app: rules/, skills/, plugins/, agents/ and
// mcp_config.json are loaded from here. The variant directories only keep
// private settings and caches, and on migrated layouts their mcp_config.json
// is a symlink into this root.
func (a *Adapter) sharedConfigDir(homeDir string) string {
	return filepath.Join(homeDir, ".gemini", "config")
}

// LegacyVariantDirs returns the variant directories that held skills and
// mcp_config.json before Gentle AI targeted the shared ~/.gemini/config root.
// Older installs may still carry managed files there.
func LegacyVariantDirs(homeDir string) []string {
	return []string{
		filepath.Join(homeDir, ".gemini", "antigravity-cli"),
		filepath.Join(homeDir, ".gemini", "antigravity-desktop"),
	}
}

func (a *Adapter) migratedMarker(homeDir string) string {
	return filepath.Join(a.sharedConfigDir(homeDir), ".migrated")
}

// --- Identity ---

func (a *Adapter) Agent() model.AgentID {
	return model.AgentAntigravity
}

func (a *Adapter) Tier() model.SupportTier {
	return model.TierFull
}

// --- Detection ---

func (a *Adapter) Detect(_ context.Context, homeDir string) (bool, string, string, bool, error) {
	configPath := a.GlobalConfigDir(homeDir)
	for _, evidence := range []string{a.antigravityRoot(homeDir), a.migratedMarker(homeDir), a.antigravityVariantDir(homeDir)} {
		stat := a.statPath(evidence)
		if stat.err == nil {
			return true, "", configPath, true, nil
		}
		if !os.IsNotExist(stat.err) {
			return false, "", "", false, stat.err
		}
	}
	return false, "", configPath, false, nil
}

// --- Installation ---

func (a *Adapter) CapabilityManifest() capabilitymanifest.AgentCapabilityManifest {
	return capabilitymanifest.MustForAgent(model.AgentAntigravity)
}

func (a *Adapter) InstallCommand(_ system.PlatformProfile) ([][]string, error) {
	return nil, AgentNotInstallableError{Agent: model.AgentAntigravity}
}

// --- Config paths ---

func (a *Adapter) GlobalConfigDir(homeDir string) string {
	return a.sharedConfigDir(homeDir)
}

// ConfigDirs lists every directory whose presence shows an Antigravity
// configuration, in preference order: the shared root, the legacy root, and
// the variant directory that holds settings. Hosts that predate the shared
// root carry only the latter two.
func (a *Adapter) ConfigDirs(homeDir string) []string {
	return append([]string{a.GlobalConfigDir(homeDir), a.antigravityRoot(homeDir)}, LegacyVariantDirs(homeDir)...)
}

func (a *Adapter) SystemPromptDir(homeDir string) string {
	return filepath.Join(homeDir, ".gemini")
}

func (a *Adapter) SystemPromptFile(homeDir string) string {
	return filepath.Join(homeDir, ".gemini", "GEMINI.md")
}

func (a *Adapter) SkillsDir(homeDir string) string {
	return filepath.Join(a.sharedConfigDir(homeDir), "skills")
}

func (a *Adapter) SettingsPath(homeDir string) string {
	return filepath.Join(a.antigravityVariantDir(homeDir), "settings.json")
}

// --- Config strategies ---

func (a *Adapter) SystemPromptStrategy() model.SystemPromptStrategy {
	return model.StrategyAppendToFile
}

func (a *Adapter) MCPStrategy() model.MCPStrategy {
	return model.StrategyMCPConfigFile
}

// --- MCP ---

func (a *Adapter) MCPConfigPath(homeDir string, _ string) string {
	return filepath.Join(a.sharedConfigDir(homeDir), "mcp_config.json")
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

// SubAgentsDir is agy's shared custom agents directory. agy 1.4 reads
// ~/.gemini/config/agents on every layout, so it does not follow the variant.
func (a *Adapter) SubAgentsDir(homeDir string) string {
	return filepath.Join(homeDir, ".gemini", "config", "agents")
}

func (a *Adapter) EmbeddedSubAgentsDir() string {
	return "antigravity/agents"
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

type AgentNotInstallableError struct {
	Agent model.AgentID
}

func (e AgentNotInstallableError) Error() string {
	return "agent " + string(e.Agent) + " is managed by Antigravity and cannot be auto-installed"
}

func defaultStat(path string) statResult {
	info, err := os.Stat(path)
	if err != nil {
		return statResult{err: err}
	}

	return statResult{isDir: info.IsDir()}
}
