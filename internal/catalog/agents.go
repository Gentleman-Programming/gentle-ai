package catalog

import "github.com/gentleman-programming/gentle-ai/internal/model"

type Agent struct {
	ID         model.AgentID
	Name       string
	Tier       model.SupportTier
	ConfigPath string
}

var allAgents = []Agent{
	{ID: model.AgentClaudeCode, Name: "Claude Code", Tier: model.TierFull, ConfigPath: "~/.claude"},
	{ID: model.AgentOpenCode, Name: "OpenCode", Tier: model.TierFull, ConfigPath: "~/.config/opencode"},
	{ID: model.AgentKilocode, Name: "Kilo Code", Tier: model.TierFull, ConfigPath: "~/.config/kilo"},
	{ID: model.AgentGeminiCLI, Name: "Gemini CLI", Tier: model.TierFull, ConfigPath: "~/.gemini"},
	{ID: model.AgentCodex, Name: "Codex", Tier: model.TierFull, ConfigPath: "~/.codex"},
	{ID: model.AgentCursor, Name: "Cursor", Tier: model.TierFull, ConfigPath: "~/.cursor"},
	{ID: model.AgentVSCodeCopilot, Name: "VS Code Copilot", Tier: model.TierFull, ConfigPath: "~/.copilot"},
	{ID: model.AgentAntigravity, Name: "Antigravity", Tier: model.TierFull, ConfigPath: "~/.gemini/antigravity"},
	{ID: model.AgentWindsurf, Name: "Windsurf", Tier: model.TierFull, ConfigPath: "~/.codeium/windsurf"},
	{ID: model.AgentQwenCode, Name: "Qwen Code", Tier: model.TierFull, ConfigPath: "~/.qwen"},
	{ID: model.AgentKiroIDE, Name: "Kiro IDE", Tier: model.TierFull, ConfigPath: "~/.kiro"},
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
