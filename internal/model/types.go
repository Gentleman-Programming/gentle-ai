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
)

// SupportTier indicates how fully an agent supports the Gentleman AI ecosystem.
// All current agents receive the full SDD orchestrator, skill files, MCP config,
// and system prompt injection. The tier is kept as metadata for display purposes.
type SupportTier string

const (
	// TierFull — the agent receives all ecosystem features: SDD orchestrator,
	// skill files, MCP servers, system prompt, and sub-agent delegation.
	TierFull SupportTier = "full"
)

type ComponentID string

const (
	ComponentEngram     ComponentID = "engram"
	ComponentSDD        ComponentID = "sdd"
	ComponentSkills     ComponentID = "skills"
	ComponentContext7   ComponentID = "context7"
	ComponentPersona    ComponentID = "persona"
	ComponentPermission ComponentID = "permissions"
	ComponentGGA        ComponentID = "gga"
	ComponentTheme      ComponentID = "theme"
)

type SkillID string

const (
	SkillSDDInit       SkillID = "sdd-init"
	SkillSDDApply      SkillID = "sdd-apply"
	SkillSDDVerify     SkillID = "sdd-verify"
	SkillSDDExplore    SkillID = "sdd-explore"
	SkillSDDPropose    SkillID = "sdd-propose"
	SkillSDDSpec       SkillID = "sdd-spec"
	SkillSDDDesign     SkillID = "sdd-design"
	SkillSDDTasks      SkillID = "sdd-tasks"
	SkillSDDArchive    SkillID = "sdd-archive"
	SkillSDDOnboard    SkillID = "sdd-onboard"
	SkillGoTesting     SkillID = "go-testing"
	SkillCreator       SkillID = "skill-creator"
	SkillJudgmentDay   SkillID = "judgment-day"
	SkillBranchPR      SkillID = "branch-pr"
	SkillIssueCreation SkillID = "issue-creation"
	SkillSkillRegistry SkillID = "skill-registry"
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

// Profile represents a named SDD orchestrator configuration with model assignments.
// The default profile (Name="" or Name="default") maps to the base sdd-orchestrator.
// Named profiles generate sdd-orchestrator-{Name} + suffixed sub-agents.
type Profile struct {
	Name              string                     // e.g. "cheap", "premium"; empty = default
	OrchestratorModel ModelAssignment            // orchestrator model
	PhaseAssignments  map[string]ModelAssignment // key = phase name (e.g. "sdd-apply")
}
