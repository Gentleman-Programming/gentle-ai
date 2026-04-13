package engram

import (
	"strings"

	"github.com/gentleman-programming/gentle-ai/internal/model"
)

const (
	SetupModeEnvVar   = "GENTLE_AI_ENGRAM_SETUP_MODE"
	SetupStrictEnvVar = "GENTLE_AI_ENGRAM_SETUP_STRICT"
)

type SetupMode string

const (
	SetupModeOff       SetupMode = "off"
	SetupModeOpenCode  SetupMode = "opencode"
	SetupModeSupported SetupMode = "supported"
)

func ParseSetupMode(value string) SetupMode {
	switch strings.TrimSpace(strings.ToLower(value)) {
	case string(SetupModeOff):
		return SetupModeOff
	case string(SetupModeOpenCode):
		return SetupModeOpenCode
	case "", string(SetupModeSupported):
		return SetupModeSupported
	default:
		return SetupModeSupported
	}
}

func ParseSetupStrict(value string) bool {
	v := strings.TrimSpace(strings.ToLower(value))
	return v == "1" || v == "true" || v == "yes" || v == "on"
}

func SetupAgentSlug(agent model.AgentID) (string, bool) {
	switch agent {
	case model.AgentOpenCode:
		return "opencode", true
	case model.AgentKilocode:
		return "kilocode", true
	case model.AgentClaudeCode:
		return "claude-code", true
	case model.AgentGeminiCLI:
		return "gemini-cli", true
	case model.AgentCodex:
		// Codex slug registered for future MCP support; ShouldAttemptSetup gates on SupportsMCP().
		return "codex", true
	case model.AgentAntigravity:
		return "antigravity", true
	case model.AgentWindsurf:
		return "windsurf", true
	case model.AgentQwenCode:
		return "qwen-code", true
	case model.AgentCursor, model.AgentVSCodeCopilot:
		// Cursor and VS Code Copilot do not use `engram setup` — their MCP
		// config is injected directly by the engram component. Returning false
		// here is intentional, not an omission.
		return "", false
	default:
		return "", false
	}
}

func ShouldAttemptSetup(mode SetupMode, agent model.AgentID) bool {
	slug, ok := SetupAgentSlug(agent)
	if !ok {
		return false
	}

	switch mode {
	case SetupModeOff:
		return false
	case SetupModeSupported:
		return true
	case SetupModeOpenCode:
		return slug == "opencode"
	default:
		return slug == "opencode"
	}
}
