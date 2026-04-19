package agents

import (
	"context"

	"github.com/gentleman-programming/gentle-ai/internal/model"
	"github.com/gentleman-programming/gentle-ai/internal/system"
)

// Capability tags for adapter feature checks.
type Capability string

const (
	CapabilityAutoInstall Capability = "auto-install"
)

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

	// Optional capabilities — agents declare what they support.
	SupportsOutputStyles() bool
	OutputStyleDir(homeDir string) string

	SupportsSlashCommands() bool
	CommandsDir(homeDir string) string

	SupportsSkills() bool
	SupportsSystemPrompt() bool
	SupportsMCP() bool
}

// ExtendedResourceAdapter is an optional capability for agents that expose
// additional first-class resource directories beyond the baseline Adapter
// contract (extensions, prompt templates, themes, package metadata).
//
// Callers must feature-detect with a type assertion:
//
//	ext, ok := adapter.(agents.ExtendedResourceAdapter)
//
// and only use these paths when ok==true.
//
// This keeps the core Adapter stable while enabling incremental support for
// resource-native runtimes such as pi.
type ExtendedResourceAdapter interface {
	SupportsExtensions() bool
	ExtensionsDir(homeDir string) string

	SupportsPromptTemplates() bool
	PromptsDir(homeDir string) string

	SupportsThemes() bool
	ThemesDir(homeDir string) string

	SupportsPackages() bool
	PackagesStatePath(homeDir string) string
}
