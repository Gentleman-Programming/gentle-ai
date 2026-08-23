package model

import "fmt"

type AgentID string

// OpenCodeBackgroundIntent is OpenCode's unresolved auto/on/off preference.
type OpenCodeBackgroundIntent string

const (
	OpenCodeBackgroundAuto OpenCodeBackgroundIntent = "auto"
	OpenCodeBackgroundOn   OpenCodeBackgroundIntent = "on"
	OpenCodeBackgroundOff  OpenCodeBackgroundIntent = "off"
)

func (i OpenCodeBackgroundIntent) Valid() bool {
	return i == OpenCodeBackgroundAuto || i == OpenCodeBackgroundOn || i == OpenCodeBackgroundOff
}

// ParseOpenCodeBackgroundIntent validates the persisted and user-supplied
// control vocabulary without selecting a runtime behavior.
func ParseOpenCodeBackgroundIntent(raw string) (OpenCodeBackgroundIntent, error) {
	intent := OpenCodeBackgroundIntent(raw)
	if intent.Valid() {
		return intent, nil
	}
	return "", fmt.Errorf("invalid OpenCode background-subagent intent %q (valid values: auto, on, off)", raw)
}

// PiBackgroundIntent is Pi's unresolved auto/on/off background-subagent
// preference. It deliberately mirrors OpenCodeBackgroundIntent instead of
// generalizing it: each type's JSON state key is baked into persisted state
// files, so sharing one type would couple two independent persistence
// contracts.
type PiBackgroundIntent string

const (
	PiBackgroundAuto PiBackgroundIntent = "auto"
	PiBackgroundOn   PiBackgroundIntent = "on"
	PiBackgroundOff  PiBackgroundIntent = "off"
)

func (i PiBackgroundIntent) Valid() bool {
	return i == PiBackgroundAuto || i == PiBackgroundOn || i == PiBackgroundOff
}

// ParsePiBackgroundIntent validates the persisted and user-supplied control
// vocabulary without selecting a runtime behavior.
func ParsePiBackgroundIntent(raw string) (PiBackgroundIntent, error) {
	intent := PiBackgroundIntent(raw)
	if intent.Valid() {
		return intent, nil
	}
	return "", fmt.Errorf("invalid Pi background-subagent intent %q (valid values: auto, on, off)", raw)
}

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
	AgentPi            AgentID = "pi"
	AgentTrae          AgentID = "trae-ide"
	AgentHermes        AgentID = "hermes"
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
	SkillSDDInit             SkillID = "sdd-init"
	SkillSDDApply            SkillID = "sdd-apply"
	SkillSDDVerify           SkillID = "sdd-verify"
	SkillSDDExplore          SkillID = "sdd-explore"
	SkillSDDPropose          SkillID = "sdd-propose"
	SkillSDDSpec             SkillID = "sdd-spec"
	SkillSDDDesign           SkillID = "sdd-design"
	SkillSDDTasks            SkillID = "sdd-tasks"
	SkillSDDArchive          SkillID = "sdd-archive"
	SkillSDDOnboard          SkillID = "sdd-onboard"
	SkillGoTesting           SkillID = "go-testing"
	SkillCreator             SkillID = "skill-creator"
	SkillImprover            SkillID = "skill-improver"
	SkillJudgmentDay         SkillID = "judgment-day"
	SkillBranchPR            SkillID = "branch-pr"
	SkillIssueCreation       SkillID = "issue-creation"
	SkillSkillRegistry       SkillID = "skill-registry"
	SkillChainedPR           SkillID = "chained-pr"
	SkillCognitiveDoc        SkillID = "cognitive-doc-design"
	SkillCommentWriter       SkillID = "comment-writer"
	SkillWorkUnitCommits     SkillID = "work-unit-commits"
	SkillRDDDefectWorkflow   SkillID = "rdd-defect-workflow"
	SkillSystemicIssueTriage SkillID = "systemic-issue-triage"
	SkillGentleAIBench       SkillID = "gentle-ai-bench"
)

type PersonaID string

const (
	PersonaGentleman PersonaID = "gentleman"
	// PersonaGentlemanNeutralArtifacts is a legacy alias accepted for backward
	// compatibility. The CLI and sync normalization treat it as PersonaNeutral,
	// and it is never offered as a selectable choice.
	PersonaGentlemanNeutralArtifacts PersonaID = "gentleman-neutral-artifacts"
	PersonaNeutral                   PersonaID = "neutral"
	PersonaCustom                    PersonaID = "custom"
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
	// StrategyMergeIntoYAML merges MCP server blocks into a YAML config file using
	// comment-preserving hand-rolled helpers (e.g., Hermes ~/.hermes/config.yaml).
	StrategyMergeIntoYAML
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

type CommunityToolID string

const (
	CommunityToolCodeGraph CommunityToolID = "codegraph"
)

// Profile represents a named SDD orchestrator configuration with model assignments.
// The default profile (Name="" or Name="default") maps to the base sdd-orchestrator.
// Named profiles generate sdd-orchestrator-{Name} + suffixed sub-agents.
type Profile struct {
	Name              string                     // e.g. "cheap", "premium"; empty = default
	OrchestratorModel ModelAssignment            // orchestrator model
	PhaseAssignments  map[string]ModelAssignment // key = phase name (e.g. "sdd-apply")
}
