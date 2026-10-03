package catalog

import "github.com/gentleman-programming/gentle-ai/v4/internal/model"

type Agent struct {
	ID         model.AgentID
	Name       string
	Tier       model.SupportTier
	ConfigPath string

	// ReviewNote, when non-empty, is surfaced on the install review screen
	// whenever this agent is selected. It exists for detection/catalog-only
	// integrations whose selection deserves an explicit expectation setting;
	// writable agents must leave it empty.
	ReviewNote string
}

var allAgents = []Agent{
	{ID: model.AgentClaudeCode, Name: "Claude Code", Tier: model.TierFull, ConfigPath: "~/.claude"},
	{ID: model.AgentOpenCode, Name: "OpenCode", Tier: model.TierFull, ConfigPath: "~/.config/opencode"},
	{ID: model.AgentKilocode, Name: "Kilo Code", Tier: model.TierFull, ConfigPath: "~/.config/kilo"},
	{ID: model.AgentGeminiCLI, Name: "Gemini CLI", Tier: model.TierFull, ConfigPath: "~/.gemini"},
	{ID: model.AgentCodex, Name: "Codex", Tier: model.TierFull, ConfigPath: "~/.codex"},
	{ID: model.AgentCursor, Name: "Cursor", Tier: model.TierFull, ConfigPath: "~/.cursor"},
	{ID: model.AgentVSCodeCopilot, Name: "VS Code Copilot", Tier: model.TierFull, ConfigPath: "~/.copilot"},
	{ID: model.AgentAntigravity, Name: "Google Antigravity", Tier: model.TierFull, ConfigPath: "~/.gemini/antigravity-cli"},
	{ID: model.AgentWindsurf, Name: "Windsurf", Tier: model.TierFull, ConfigPath: "~/.codeium/windsurf"},
	{ID: model.AgentKimi, Name: "Kimi Code", Tier: model.TierFull, ConfigPath: "~/.kimi-code (or legacy ~/.kimi)"},
	{ID: model.AgentQwenCode, Name: "Qwen Code", Tier: model.TierFull, ConfigPath: "~/.qwen"},
	{ID: model.AgentKiroIDE, Name: "Kiro IDE", Tier: model.TierFull, ConfigPath: "~/.kiro"},
	{ID: model.AgentOpenClaw, Name: "OpenClaw", Tier: model.TierFull, ConfigPath: "~/.openclaw"},
	{ID: model.AgentPi, Name: "Pi", Tier: model.TierFull, ConfigPath: "~/.pi"},
	{ID: model.AgentTrae, Name: "Trae IDE", Tier: model.TierFull, ConfigPath: "~/.trae"},
	{ID: model.AgentHermes, Name: "Hermes", Tier: model.TierFull, ConfigPath: "~/.hermes"},
	// Conductor is detection/catalog-only: its workspaces inherit Claude Code
	// configuration, and Gentle AI writes no Conductor-specific files. The note
	// sets that expectation before the user confirms the install.
	{ID: model.AgentConductor, Name: "Conductor", Tier: model.TierFull, ConfigPath: "~/.conductor",
		ReviewNote: "Conductor workspaces inherit Claude Code configuration; Gentle AI writes no Conductor-specific files."},
	{ID: model.AgentCommandCode, Name: "Command Code", Tier: model.TierFull, ConfigPath: "~/.commandcode"},
}

// mvpAgents are the original MVP agents (Claude Code, OpenCode).
var mvpAgents = []Agent{
	{ID: model.AgentClaudeCode, Name: "Claude Code", Tier: model.TierFull, ConfigPath: "~/.claude"},
	{ID: model.AgentOpenCode, Name: "OpenCode", Tier: model.TierFull, ConfigPath: "~/.config/opencode"},
}

func AllAgents() []Agent {
	agents := make([]Agent, len(allAgents))
	copy(agents, allAgents)
	return agents
}

func MVPAgents() []Agent {
	agents := make([]Agent, len(mvpAgents))
	copy(agents, mvpAgents)
	return agents
}

func IsMVPAgent(agent model.AgentID) bool {
	for _, current := range mvpAgents {
		if current.ID == agent {
			return true
		}
	}

	return false
}

func IsSupportedAgent(agent model.AgentID) bool {
	for _, current := range allAgents {
		if current.ID == agent {
			return true
		}
	}

	return false
}
