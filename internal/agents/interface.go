package agents

import (
	"context"

	"github.com/gentleman-programming/gentle-ai/v2/internal/model"
	"github.com/gentleman-programming/gentle-ai/v2/internal/system"
)

// Capability tags for adapter feature checks.
type Capability string

const (
	CapabilityAutoInstall Capability = "auto-install"
)

// SettingsFileFormat describes the syntax of an adapter's settings file.
// Unknown formats must not be mutated as JSON objects.
type SettingsFileFormat string

const (
	SettingsFileFormatUnknown    SettingsFileFormat = "unknown"
	SettingsFileFormatJSONObject SettingsFileFormat = "json-object"
	SettingsFileFormatTOML       SettingsFileFormat = "toml"
	SettingsFileFormatYAML       SettingsFileFormat = "yaml"
)

var settingsFileFormats = map[model.AgentID]SettingsFileFormat{
	model.AgentAntigravity:   SettingsFileFormatJSONObject,
	model.AgentClaudeCode:    SettingsFileFormatJSONObject,
	model.AgentCursor:        SettingsFileFormatJSONObject,
	model.AgentGeminiCLI:     SettingsFileFormatJSONObject,
	model.AgentHermes:        SettingsFileFormatYAML,
	model.AgentKilocode:      SettingsFileFormatJSONObject,
	model.AgentKimi:          SettingsFileFormatTOML,
	model.AgentKiroIDE:       SettingsFileFormatJSONObject,
	model.AgentOpenClaw:      SettingsFileFormatJSONObject,
	model.AgentOpenCode:      SettingsFileFormatJSONObject,
	model.AgentPi:            SettingsFileFormatJSONObject,
	model.AgentQwenCode:      SettingsFileFormatJSONObject,
	model.AgentTrae:          SettingsFileFormatJSONObject,
	model.AgentVSCodeCopilot: SettingsFileFormatJSONObject,
	model.AgentWindsurf:      SettingsFileFormatJSONObject,
}

// SettingsFileFormatFor returns the known settings syntax for an agent. A
// missing entry intentionally remains unknown until its mutation semantics are
// explicitly established.
func SettingsFileFormatFor(agent model.AgentID) SettingsFileFormat {
	if format, ok := settingsFileFormats[agent]; ok {
		return format
	}
	return SettingsFileFormatUnknown
}

// SupportsJSONSettingsObjectMutation reports whether a settings file may be
// safely treated as a JSON object by shared components.
func SupportsJSONSettingsObjectMutation(adapter Adapter) bool {
	return SettingsFileFormatFor(adapter.Agent()) == SettingsFileFormatJSONObject
}

// Adapter is the core abstraction for AI agent integration. Components use
// adapter methods instead of switch statements on AgentID, making it trivial
// to add new agents without modifying component code.
type Adapter interface {
	// Identity
	Agent() model.AgentID
	Tier() model.SupportTier

	// Detection
	Detect(ctx context.Context, homeDir string) (installed bool, binaryPath string, configPath string, configFound bool, err error)

	// Installation
	SupportsAutoInstall() bool
	InstallCommand(profile system.PlatformProfile) ([][]string, error)

	// Config paths — components use these instead of hardcoding paths per agent.
	GlobalConfigDir(homeDir string) string
	SystemPromptDir(homeDir string) string
	SystemPromptFile(homeDir string) string
	SkillsDir(homeDir string) string
	SettingsPath(homeDir string) string

	// Config strategies — HOW to inject content, not WHERE (that's paths above).
	SystemPromptStrategy() model.SystemPromptStrategy
	MCPStrategy() model.MCPStrategy

	// MCP path resolution
	MCPConfigPath(homeDir string, serverName string) string

	// Optional capabilities — compatibility projections of the adapter's
	// canonical AgentCapabilityManifest.
	SupportsOutputStyles() bool
	OutputStyleDir(homeDir string) string

	SupportsSlashCommands() bool
	CommandsDir(homeDir string) string

	SupportsSubAgents() bool
	SubAgentsDir(homeDir string) string
	EmbeddedSubAgentsDir() string

	SupportsSkills() bool
	SupportsSystemPrompt() bool
	SupportsMCP() bool
}

// EffectiveCodeGraphWiringDetector is an optional adapter capability for agents
// whose configuration format requires semantic validation beyond marker checks.
type EffectiveCodeGraphWiringDetector interface {
	EffectiveCodeGraphWiring(homeDir string) (path string, configured bool)
}
