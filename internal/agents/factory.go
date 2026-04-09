package agents

import (
	"fmt"

	"github.com/gentleman-programming/gentle-ai/internal/agents/antigravity"
	"github.com/gentleman-programming/gentle-ai/internal/agents/claude"
	"github.com/gentleman-programming/gentle-ai/internal/agents/codex"
	cursoradapter "github.com/gentleman-programming/gentle-ai/internal/agents/cursor"
	"github.com/gentleman-programming/gentle-ai/internal/agents/gemini"
	"github.com/gentleman-programming/gentle-ai/internal/agents/kilocode"
	"github.com/gentleman-programming/gentle-ai/internal/agents/opencode"
	"github.com/gentleman-programming/gentle-ai/internal/agents/qwen"
	"github.com/gentleman-programming/gentle-ai/internal/agents/vscode"
	"github.com/gentleman-programming/gentle-ai/internal/agents/windsurf"
	"github.com/gentleman-programming/gentle-ai/internal/model"
)

func NewAdapter(agent model.AgentID) (Adapter, error) {
	switch agent {
	case model.AgentClaudeCode:
		return claude.NewAdapter(), nil
	case model.AgentOpenCode:
		return opencode.NewAdapter(), nil
	case model.AgentKilocode:
		return kilocode.NewAdapter(), nil
	case model.AgentGeminiCLI:
		return gemini.NewAdapter(), nil
	case model.AgentCursor:
		return cursoradapter.NewAdapter(), nil
	case model.AgentVSCodeCopilot:
		return vscode.NewAdapter(), nil
	case model.AgentCodex:
		return codex.NewAdapter(), nil
	case model.AgentAntigravity:
		return antigravity.NewAdapter(), nil
	case model.AgentWindsurf:
		return windsurf.NewAdapter(), nil
	case model.AgentQwenCode:
		return qwen.NewAdapter(), nil
	default:
		return nil, AgentNotSupportedError{Agent: agent}
	}
}

func NewDefaultRegistry() (*Registry, error) {
	adapters := make([]Adapter, 0, 9)

	for _, agent := range []model.AgentID{
		model.AgentClaudeCode,
		model.AgentOpenCode,
		model.AgentKilocode,
		model.AgentGeminiCLI,
		model.AgentCursor,
		model.AgentVSCodeCopilot,
		model.AgentCodex,
		model.AgentAntigravity,
		model.AgentWindsurf,
		model.AgentQwenCode,
	} {
		adapter, err := NewAdapter(agent)
		if err != nil {
			return nil, fmt.Errorf("create %s adapter: %w", agent, err)
		}
		adapters = append(adapters, adapter)
	}

	registry, err := NewRegistry(adapters...)
	if err != nil {
		return nil, fmt.Errorf("create registry: %w", err)
	}

	return registry, nil
}

// NewMVPRegistry creates a registry with only the MVP agents (Claude Code, OpenCode).
// Kept for backward compatibility.
func NewMVPRegistry() (*Registry, error) {
	claudeAdapter, err := NewAdapter(model.AgentClaudeCode)
	if err != nil {
		return nil, fmt.Errorf("create claude adapter: %w", err)
	}

	opencodeAdapter, err := NewAdapter(model.AgentOpenCode)
	if err != nil {
		return nil, fmt.Errorf("create opencode adapter: %w", err)
	}

	registry, err := NewRegistry(claudeAdapter, opencodeAdapter)
	if err != nil {
		return nil, fmt.Errorf("create registry: %w", err)
	}

	return registry, nil
}
