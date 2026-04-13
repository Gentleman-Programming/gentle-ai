package model

// KiroModelID maps a ClaudeModelAlias to the model identifier Kiro expects
// in the `model:` field of a custom agent frontmatter.
//
// Kiro model IDs do not include a provider prefix — they are passed directly
// as the `model` key in ~/.kiro/agents/*.md frontmatter.
//
// References: https://kiro.dev/docs/models/
func KiroModelID(alias ClaudeModelAlias) string {
	switch alias {
	case ClaudeModelOpus:
		return "claude-opus-4.6"
	case ClaudeModelHaiku:
		return "claude-haiku-4.5"
	default:
		return "claude-sonnet-4.6"
	}
}
