package model

type AgentID string

const (
	AgentClaudeCode    AgentID = "claude-code"
	AgentOpenCode      AgentID = "opencode"
	AgentKilocode      AgentID = "kilocode"
	AgentGeminiCLI     AgentID = "gemini-cli"
	AgentCursor        AgentID = "cursor"
	AgentVSCodeCopilot AgentID = "vscode-copilot"
	AgentCodex         AgentID = "codex"
	AgentAntigravity   AgentID = "antigravity"
	AgentWindsurf      AgentID = "windsurf"
	AgentKimi          AgentID = "kimi"
	AgentQwenCode      AgentID = "qwen-code"
	AgentKiroIDE       AgentID = "kiro-ide"
	AgentOpenClaw      AgentID = "openclaw"
	AgentPiCodingAgent AgentID = "pi-coding-agent"
)

// SupportTier indicates how fully an agent supports the Gentleman AI ecosystem.
// It is metadata for display and product guidance; capability flags on each
// adapter remain the source of truth for what is actually injected.
type SupportTier string

const (
	// TierFull — the agent receives the full supported ecosystem for its native
	// integration surface, including SDD, skills, prompt/system instructions, and
	// MCP/sub-agent integration where the agent exposes those capabilities.
	TierFull SupportTier = "full"
	// TierPartial — the agent receives a supported subset because its native
	// integration surface does not currently expose every ecosystem feature.
	TierPartial SupportTier = "partial"
)

type ComponentID string

const (
	ComponentEngram             ComponentID = "engram"
	ComponentSDD                ComponentID = "sdd"
	ComponentSkills             ComponentID = "skills"
	ComponentContext7           ComponentID = "context7"
	ComponentPersona            ComponentID = "persona"
	ComponentPermission         ComponentID = "permissions"
	ComponentGGA                ComponentID = "gga"
	ComponentTheme              ComponentID = "theme"
	ComponentClaudeTheme        ComponentID = "claude-theme"
	ComponentOpenCodeGentleLogo ComponentID = "opencode-gentle-logo"
)

type UninstallMode string

const (
	UninstallModePartial      UninstallMode = "partial"
	UninstallModeFull         UninstallMode = "full"
	UninstallModeFullRemove   UninstallMode = "full-remove"
	UninstallModeCleanInstall UninstallMode = "clean-install"
)

type EngramUninstallScope string

const (
	EngramUninstallScopeGlobal  EngramUninstallScope = "global"
	EngramUninstallScopeProject EngramUninstallScope = "project"
)

type SkillID string

const (
	SkillSDDInit         SkillID = "sdd-init"
	SkillSDDApply        SkillID = "sdd-apply"
	SkillSDDVerify       SkillID = "sdd-verify"
	SkillSDDExplore      SkillID = "sdd-explore"
	SkillSDDPropose      SkillID = "sdd-propose"
	SkillSDDSpec         SkillID = "sdd-spec"
	SkillSDDDesign       SkillID = "sdd-design"
	SkillSDDTasks        SkillID = "sdd-tasks"
	SkillSDDArchive      SkillID = "sdd-archive"
	SkillSDDOnboard      SkillID = "sdd-onboard"
	SkillGoTesting       SkillID = "go-testing"
	SkillCreator         SkillID = "skill-creator"
	SkillJudgmentDay     SkillID = "judgment-day"
	SkillBranchPR        SkillID = "branch-pr"
	SkillIssueCreation   SkillID = "issue-creation"
	SkillSkillRegistry   SkillID = "skill-registry"
	SkillChainedPR       SkillID = "chained-pr"
	SkillCognitiveDoc    SkillID = "cognitive-doc-design"
	SkillCommentWriter   SkillID = "comment-writer"
	SkillWorkUnitCommits SkillID = "work-unit-commits"
)

type PersonaID string

const (
	PersonaGentleman PersonaID = "gentleman"
	PersonaNeutral   PersonaID = "neutral"
	PersonaCustom    PersonaID = "custom"
)

// SystemPromptStrategy defines how an agent's system prompt file is managed.
type SystemPromptStrategy int

const (
	// StrategyMarkdownSections uses <!-- gentle-ai:ID --> markers to inject sections
	// into an existing file without clobbering user content (Claude Code CLAUDE.md).
	StrategyMarkdownSections SystemPromptStrategy = iota
	// StrategyFileReplace replaces the entire system prompt file (OpenCode AGENTS.md).
	StrategyFileReplace
	// StrategyAppendToFile appends content to an existing system prompt file.
	StrategyAppendToFile
	// StrategyInstructionsFile writes a dedicated instructions file (e.g. .instructions.md).
	StrategyInstructionsFile
	// StrategyJinjaModules writes separate module files that are included into a
	// thin Jinja2 template (e.g. Kimi's KIMI.md).
	StrategyJinjaModules
	// StrategySteeringFile writes a Kiro steering file with inclusion: always frontmatter.
	StrategySteeringFile
)

// MCPStrategy defines how MCP server configs are written for an agent.
type MCPStrategy int

const (
	// StrategySeparateMCPFiles writes one JSON file per server in a dedicated directory
	// (e.g., ~/.claude/mcp/context7.json).
	StrategySeparateMCPFiles MCPStrategy = iota
	// StrategyMergeIntoSettings merges mcpServers into a settings.json file
	// (e.g., OpenCode, Gemini CLI).
	StrategyMergeIntoSettings
	// StrategyMCPConfigFile writes to a dedicated mcp.json config file (e.g., Cursor ~/.cursor/mcp.json).
	StrategyMCPConfigFile
	// StrategyTOMLFile writes MCP config to a TOML file (e.g., Codex ~/.codex/config.toml).
	StrategyTOMLFile
)

type PresetID string

const (
	PresetFullGentleman PresetID = "full-gentleman"
	PresetEcosystemOnly PresetID = "ecosystem-only"
	PresetMinimal       PresetID = "minimal"
	PresetCustom        PresetID = "custom"
)

type SDDModeID string

const (
	SDDModeSingle SDDModeID = "single"
	SDDModeMulti  SDDModeID = "multi"
)

// SDDProfileStrategyID defines how sync handles OpenCode SDD profiles.
type SDDProfileStrategyID string

const (
	// SDDProfileStrategyGeneratedMulti is the default/backward-compatible mode:
	// named profiles coexist in opencode.json as suffixed agents and are detected
	// from sdd-orchestrator-{name} keys during regular sync.
	SDDProfileStrategyGeneratedMulti SDDProfileStrategyID = "generated-multi"
	// SDDProfileStrategyExternalSingleActive supports external profile managers
	// that keep profile state outside opencode.json and activate one runtime
	// profile without requiring a restart.
	SDDProfileStrategyExternalSingleActive SDDProfileStrategyID = "external-single-active"
)

type OpenCodeCommunityPluginID string

const (
	OpenCodePluginSubAgentStatusline OpenCodeCommunityPluginID = "sub-agent-statusline"
	OpenCodePluginSDDEngramManage    OpenCodeCommunityPluginID = "sdd-engram-plugin"
	OpenCodePluginGentleLogo         OpenCodeCommunityPluginID = "gentle-logo"
)

// Profile represents a named SDD orchestrator configuration with model assignments.
// The default profile (Name="" or Name="default") maps to the base sdd-orchestrator.
// Named profiles generate sdd-orchestrator-{Name} + suffixed sub-agents.
type Profile struct {
	Name              string                     // e.g. "cheap", "premium"; empty = default
	OrchestratorModel ModelAssignment            // orchestrator model
	PhaseAssignments  map[string]ModelAssignment // key = phase name (e.g. "sdd-apply")
}
