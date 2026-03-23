package model

type AgentID string

const (
	AgentClaudeCode    AgentID = "claude-code"
	AgentOpenCode      AgentID = "opencode"
	AgentGeminiCLI     AgentID = "gemini-cli"
	AgentCursor        AgentID = "cursor"
	AgentVSCodeCopilot AgentID = "vscode-copilot"
	AgentCodex         AgentID = "codex"
	AgentAntigravity   AgentID = "antigravity"
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
	SkillSDDInit    SkillID = "sdd-init"
	SkillSDDNew     SkillID = "sdd-new"
	SkillSDDApply   SkillID = "sdd-apply"
	SkillSDDVerify  SkillID = "sdd-verify"
	SkillSDDExplore SkillID = "sdd-explore"
	SkillSDDPropose SkillID = "sdd-propose"
	SkillSDDSpec    SkillID = "sdd-spec"
	SkillSDDDesign  SkillID = "sdd-design"
	SkillSDDTasks   SkillID = "sdd-tasks"
	SkillSDDArchive SkillID = "sdd-archive"
	SkillReact19    SkillID = "react-19"
	SkillNextJS15   SkillID = "nextjs-15"
	SkillTailwind4  SkillID = "tailwind-4"
	SkillZustand5   SkillID = "zustand-5"
	SkillZod4       SkillID = "zod-4"
	SkillAISDK5     SkillID = "ai-sdk-5"
	SkillPlaywright SkillID = "playwright"
	SkillPytest     SkillID = "pytest"
	SkillDjangoDRF  SkillID = "django-drf"
	SkillGoTesting  SkillID = "go-testing"
	SkillCreator    SkillID = "skill-creator"
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
