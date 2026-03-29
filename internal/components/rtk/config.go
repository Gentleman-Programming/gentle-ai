package rtk

import "github.com/gentleman-programming/gentle-ai/internal/model"

// AgentFlags maps an AgentID to the rtk init flags for that agent.
// Returns the flag string to pass to "rtk init -g <flags>".
// Returns empty string for agents that don't support RTK hooks.
func AgentFlags(agentID model.AgentID) string {
	switch agentID {
	case model.AgentClaudeCode:
		// Default: rtk init -g (no extra flags needed)
		return ""
	case model.AgentOpenCode:
		return "--opencode"
	case model.AgentGeminiCLI:
		return "--gemini"
	case model.AgentCursor:
		return "--agent cursor"
	case model.AgentVSCodeCopilot:
		return "--copilot"
	case model.AgentCodex:
		return "--codex"
	case model.AgentWindsurf:
		return "--agent windsurf"
	case model.AgentAntigravity:
		// Antigravity does not have RTK hook support yet
		return ""
	default:
		return ""
	}
}

// SupportsHook returns true if the agent supports RTK hook installation.
func SupportsHook(agentID model.AgentID) bool {
	// Antigravity has no RTK support; all others do
	return agentID != model.AgentAntigravity
}
