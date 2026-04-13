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
	SkillSDDInit       SkillID = "sdd-init"
	SkillSDDApply      SkillID = "sdd-apply"
	SkillSDDVerify     SkillID = "sdd-verify"
	SkillSDDExplore    SkillID = "sdd-explore"
	SkillSDDPropose    SkillID = "sdd-propose"
	SkillSDDSpec       SkillID = "sdd-spec"
	SkillSDDDesign     SkillID = "sdd-design"
	SkillSDDTasks      SkillID = "sdd-tasks"
	SkillSDDArchive    SkillID = "sdd-archive"
	SkillGoTesting     SkillID = "go-testing"
	SkillCreator       SkillID = "skill-creator"
	SkillJudgmentDay   SkillID = "judgment-day"
	SkillBranchPR      SkillID = "branch-pr"
	SkillIssueCreation SkillID = "issue-creation"
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

type DelegationModel string

const (
	DelegationSingleAgent DelegationModel = "single"
	DelegationMultiAgent  DelegationModel = "multi"
	DelegationAny         DelegationModel = "any"
)

func (d DelegationModel) IsValid() bool {
	switch d {
	case DelegationSingleAgent, DelegationMultiAgent, DelegationAny:
		return true
	default:
		return false
	}
}

// Skill represents metadata for a skill in the catalog.
type Skill struct {
	ID              SkillID
	Name            string
	Category        string
	Priority        string
	DelegationModel DelegationModel

	// Capabilities is the new capability-based requirements model.
	// When set, the resolver uses this instead of DelegationModel.
	// When nil, DelegationModel is converted to Capabilities for backward compatibility.
	Capabilities *SkillCapabilities
}

// GetCapabilities returns the skill's capabilities, deriving them from
// DelegationModel if not explicitly set. This ensures backward compatibility.
func (s Skill) GetCapabilities() SkillCapabilities {
	if s.Capabilities != nil {
		return *s.Capabilities
	}
	return SkillCapabilitiesFromDelegationModel(s.DelegationModel)
}

var mvpSkills = []Skill{
	// SDD skills
	{ID: SkillSDDInit, Name: "sdd-init", Category: "sdd", Priority: "p0", DelegationModel: DelegationAny},

	{ID: SkillSDDApply, Name: "sdd-apply", Category: "sdd", Priority: "p0", DelegationModel: DelegationAny},
	{ID: SkillSDDVerify, Name: "sdd-verify", Category: "sdd", Priority: "p0", DelegationModel: DelegationAny},
	{ID: SkillSDDExplore, Name: "sdd-explore", Category: "sdd", Priority: "p0", DelegationModel: DelegationAny},
	{ID: SkillSDDPropose, Name: "sdd-propose", Category: "sdd", Priority: "p0", DelegationModel: DelegationAny},
	{ID: SkillSDDSpec, Name: "sdd-spec", Category: "sdd", Priority: "p0", DelegationModel: DelegationAny},
	{ID: SkillSDDDesign, Name: "sdd-design", Category: "sdd", Priority: "p0", DelegationModel: DelegationAny},
	{ID: SkillSDDTasks, Name: "sdd-tasks", Category: "sdd", Priority: "p0", DelegationModel: DelegationAny},
	{ID: SkillSDDArchive, Name: "sdd-archive", Category: "sdd", Priority: "p0", DelegationModel: DelegationAny},
	// Foundation skills
	{ID: SkillGoTesting, Name: "go-testing", Category: "testing", Priority: "p0", DelegationModel: DelegationAny},
	{ID: SkillCreator, Name: "skill-creator", Category: "workflow", Priority: "p0", DelegationModel: DelegationAny},
	{
		ID:              SkillJudgmentDay,
		Name:            "judgment-day",
		Category:        "workflow",
		Priority:        "p0",
		DelegationModel: DelegationMultiAgent,
		Capabilities: &SkillCapabilities{
			RequiredCapabilities: []RuntimeCapability{CapabilitySubAgents},
			ExecutionPattern:     PatternNative,
			FallbackPattern:      PatternSequentialFallback,
			FallbackCapabilities: []RuntimeCapability{CapabilitySequentialExecution, CapabilityFileSystem},
		},
	},
	{ID: SkillBranchPR, Name: "branch-pr", Category: "workflow", Priority: "p0", DelegationModel: DelegationAny},
	{ID: SkillIssueCreation, Name: "issue-creation", Category: "workflow", Priority: "p0", DelegationModel: DelegationAny},
}

// MVPSkills returns a copy of the MVP skill catalog.
func MVPSkills() []Skill {
	skills := make([]Skill, len(mvpSkills))
	copy(skills, mvpSkills)
	return skills
}
