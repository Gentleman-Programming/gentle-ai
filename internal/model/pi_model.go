package model

// PiSubscription represents a supported AI preset ID for Pi subagents.
type PiSubscription string

const (
	// Anthropic API-key tiers
	PiPresetClaudeBalanced PiSubscription = "claude-balanced"
	PiPresetClaudePremium  PiSubscription = "claude-premium"
	PiPresetClaudeEconomy  PiSubscription = "claude-economy"

	// Codex Tiers
	PiPresetCodexBalanced PiSubscription = "codex-balanced"
	PiPresetCodexPremium  PiSubscription = "codex-premium"
	PiPresetCodexEconomy  PiSubscription = "codex-economy"

	// Kiro Tiers
	PiPresetKiroBalanced PiSubscription = "kiro-balanced"
	PiPresetKiroPremium  PiSubscription = "kiro-premium"
	PiPresetKiroEconomy  PiSubscription = "kiro-economy"

	// Budget / Open Source
	PiPresetBudgetBalanced PiSubscription = "budget-balanced"

	// Legacy aliases for backward compatibility
	PiSubscriptionClaude = PiPresetClaudeBalanced
	PiSubscriptionCodex  = PiPresetCodexBalanced
	PiSubscriptionKiro   = PiPresetKiroBalanced
	PiSubscriptionBudget = PiPresetBudgetBalanced
)

// PiAgentModelEntry represents the model and thinking/effort assigned to a Pi agent.
type PiAgentModelEntry struct {
	Model    string `json:"model"`
	Thinking string `json:"thinking,omitempty"`
}

// PiConfigurableAgents returns the ordered list of Pi agent names configurable by presets.
func PiConfigurableAgents() []string {
	return []string{
		"sdd-explore",
		"sdd-research",
		"sdd-proposal",
		"sdd-spec",
		"sdd-design",
		"sdd-tasks",
		"sdd-apply",
		"sdd-verify",
		"sdd-archive",
		"sdd-onboard",
		"sdd-status",
		"sdd-sync",
		"jd-judge-a",
		"jd-judge-b",
		"jd-fix-agent",
		"review-risk",
		"review-readability",
		"review-reliability",
		"review-resilience",
		"default",
	}
}

// PiSubscriptionsOrder returns the display order for Pi subscription presets.
func PiSubscriptionsOrder() []PiSubscription {
	return []PiSubscription{
		PiPresetClaudeBalanced,
		PiPresetClaudePremium,
		PiPresetClaudeEconomy,
		PiPresetCodexBalanced,
		PiPresetCodexPremium,
		PiPresetCodexEconomy,
		PiPresetKiroBalanced,
		PiPresetKiroPremium,
		PiPresetKiroEconomy,
		PiPresetBudgetBalanced,
	}
}

// PiSubscriptionLabel returns the user-facing option label for each preset.
func PiSubscriptionLabel(sub PiSubscription) string {
	switch sub {
	case PiPresetClaudeBalanced, "claude":
		return "Anthropic via API key — Balanced (Recommended)"
	case PiPresetClaudePremium:
		return "Anthropic via API key — Premium (Max Quality / Opus)"
	case PiPresetClaudeEconomy:
		return "Anthropic via API key — Economy (Token Saver / Haiku)"
	case PiPresetCodexBalanced, "codex":
		return "Codex — Balanced (Recommended)"
	case PiPresetCodexPremium:
		return "Codex — Premium (Max Quality / Sol)"
	case PiPresetCodexEconomy:
		return "Codex — Economy (Token Saver / Luna)"
	case PiPresetKiroBalanced, "kiro":
		return "Kiro — Balanced (Recommended)"
	case PiPresetKiroPremium:
		return "Kiro — Premium (Max Quality / 4.8)"
	case PiPresetKiroEconomy:
		return "Kiro — Economy (Token Saver / 4.5)"
	case PiPresetBudgetBalanced, "budget":
		return "Budget — Open Source (DeepSeek Reasoner & Chat)"
	default:
		return string(sub)
	}
}

// PiSubscriptionDescription returns a concise human-readable description for a preset.
func PiSubscriptionDescription(sub PiSubscription) string {
	switch sub {
	case PiPresetClaudeBalanced, "claude":
		return "Anthropic API-key mix: Sonnet for reasoning & code, Haiku for light work (balanced spend)"
	case PiPresetClaudePremium:
		return "Anthropic API-key max quality: Opus for architecture & verification, Sonnet for code"
	case PiPresetClaudeEconomy:
		return "Anthropic API-key cost saver: Sonnet for architecture only, Haiku for code & maintenance"

	case PiPresetCodexBalanced, "codex":
		return "Smart mix: Sol for reasoning, Terra for code, Luna for light work (balanced spend)"
	case PiPresetCodexPremium:
		return "Max quality: Sol with high effort across architecture & code"
	case PiPresetCodexEconomy:
		return "Cost-optimised: Terra for code, Luna for light work with lower effort"

	case PiPresetKiroBalanced, "kiro":
		return "Smart mix: Claude 4.8 for reasoning, Sonnet 4.6 for code, Haiku 4.5 for light work"
	case PiPresetKiroPremium:
		return "Max quality: Claude 4.8 across all critical architecture & coding phases"
	case PiPresetKiroEconomy:
		return "Cost-optimised: Sonnet 4.6 for architecture, Haiku 4.5 for code & maintenance"

	case PiPresetBudgetBalanced, "budget":
		return "Cost-optimised: DeepSeek Reasoner & Chat — high quality at minimal token cost"
	default:
		return ""
	}
}

// PiPresetForSubscription returns the preset mapping for a given subscription and tier.
func PiPresetForSubscription(sub PiSubscription) map[string]PiAgentModelEntry {
	switch sub {
	// Anthropic via API key
	case PiPresetClaudeBalanced, "claude":
		return mapRoles(
			PiAgentModelEntry{Model: "anthropic/claude-sonnet-4", Thinking: "high"},
			PiAgentModelEntry{Model: "anthropic/claude-sonnet-4", Thinking: "medium"},
			PiAgentModelEntry{Model: "anthropic/claude-haiku-4", Thinking: "low"},
		)
	case PiPresetClaudePremium:
		return mapRoles(
			PiAgentModelEntry{Model: "anthropic/claude-opus-4", Thinking: "high"},
			PiAgentModelEntry{Model: "anthropic/claude-sonnet-4", Thinking: "high"},
			PiAgentModelEntry{Model: "anthropic/claude-sonnet-4", Thinking: "medium"},
		)
	case PiPresetClaudeEconomy:
		return mapRoles(
			PiAgentModelEntry{Model: "anthropic/claude-sonnet-4", Thinking: "medium"},
			PiAgentModelEntry{Model: "anthropic/claude-haiku-4", Thinking: "medium"},
			PiAgentModelEntry{Model: "anthropic/claude-haiku-4", Thinking: "off"},
		)

	// Codex
	case PiPresetCodexBalanced, "codex":
		return mapRoles(
			PiAgentModelEntry{Model: "openai/gpt-5.6-sol", Thinking: "high"},
			PiAgentModelEntry{Model: "openai/gpt-5.6-terra", Thinking: "medium"},
			PiAgentModelEntry{Model: "openai/gpt-5.6-luna", Thinking: "low"},
		)
	case PiPresetCodexPremium:
		return mapRoles(
			PiAgentModelEntry{Model: "openai/gpt-5.6-sol", Thinking: "xhigh"},
			PiAgentModelEntry{Model: "openai/gpt-5.6-sol", Thinking: "high"},
			PiAgentModelEntry{Model: "openai/gpt-5.6-terra", Thinking: "medium"},
		)
	case PiPresetCodexEconomy:
		return mapRoles(
			PiAgentModelEntry{Model: "openai/gpt-5.6-terra", Thinking: "medium"},
			PiAgentModelEntry{Model: "openai/gpt-5.6-terra", Thinking: "low"},
			PiAgentModelEntry{Model: "openai/gpt-5.6-luna", Thinking: "off"},
		)

	// Kiro
	case PiPresetKiroBalanced, "kiro":
		return mapRoles(
			PiAgentModelEntry{Model: "kiro/claude-opus-4.8", Thinking: "high"},
			PiAgentModelEntry{Model: "kiro/claude-sonnet-4.6", Thinking: "medium"},
			PiAgentModelEntry{Model: "kiro/claude-haiku-4.5", Thinking: "low"},
		)
	case PiPresetKiroPremium:
		return mapRoles(
			PiAgentModelEntry{Model: "kiro/claude-opus-4.8", Thinking: "xhigh"},
			PiAgentModelEntry{Model: "kiro/claude-opus-4.8", Thinking: "high"},
			PiAgentModelEntry{Model: "kiro/claude-sonnet-4.6", Thinking: "medium"},
		)
	case PiPresetKiroEconomy:
		return mapRoles(
			PiAgentModelEntry{Model: "kiro/claude-sonnet-4.6", Thinking: "medium"},
			PiAgentModelEntry{Model: "kiro/claude-haiku-4.5", Thinking: "medium"},
			PiAgentModelEntry{Model: "kiro/claude-haiku-4.5", Thinking: "off"},
		)

	// Budget
	case PiPresetBudgetBalanced, "budget":
		return mapRoles(
			PiAgentModelEntry{Model: "deepseek/deepseek-reasoner", Thinking: "medium"},
			PiAgentModelEntry{Model: "deepseek/deepseek-chat", Thinking: "low"},
			PiAgentModelEntry{Model: "openai/gpt-5.4-mini", Thinking: "off"},
		)

	default:
		return PiPresetForSubscription(PiPresetClaudeBalanced)
	}
}

func mapRoles(strong, code, light PiAgentModelEntry) map[string]PiAgentModelEntry {
	return map[string]PiAgentModelEntry{
		// Reasoning & Architecture Tier
		"sdd-explore":        strong,
		"sdd-research":       strong,
		"sdd-proposal":       strong,
		"sdd-design":         strong,
		"sdd-verify":         strong,
		"jd-judge-a":         strong,
		"jd-judge-b":         strong,
		"review-risk":        strong,
		"review-reliability": strong,
		"review-resilience":  strong,

		// Code & Execution Tier
		"sdd-apply":    code,
		"jd-fix-agent": code,
		"default":      code,

		// Light & Maintenance Tier
		"sdd-spec":           light,
		"sdd-tasks":          light,
		"sdd-archive":        light,
		"sdd-onboard":        light,
		"sdd-status":         light,
		"sdd-sync":           light,
		"review-readability": light,
	}
}
