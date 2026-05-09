package assets

import "github.com/gentleman-programming/gentle-ai/internal/model"

// SDDCommandsAssetDir returns the embedded slash-command asset directory for an
// agent. Claude uses Claude-native frontmatter, Pi uses prompt templates, and
// agents without a dedicated command set fall back to OpenCode-compatible assets.
func SDDCommandsAssetDir(agent model.AgentID) string {
	switch agent {
	case model.AgentClaudeCode:
		return "claude/commands"
	case model.AgentPiCodingAgent:
		return "pi/prompts"
	default:
		return "opencode/commands"
	}
}
