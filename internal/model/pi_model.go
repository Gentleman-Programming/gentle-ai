package model

// PiSubscription represents a supported AI preset for Pi agents.
type PiSubscription string

const (
	// PiSubscriptionClaude optimizes for Anthropic models via API key.
	PiSubscriptionClaude PiSubscription = "claude"

	// PiSubscriptionCodex optimizes for OpenAI / Codex (via Pi native login or key).
	PiSubscriptionCodex PiSubscription = "codex"

	// PiSubscriptionKiro optimizes for Kiro IDE / frontier model environments.
	PiSubscriptionKiro PiSubscription = "kiro"

	// PiSubscriptionBudget optimizes for lower-cost models (DeepSeek, MiniMax, mini models).
	PiSubscriptionBudget PiSubscription = "budget"
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

// PiSubscriptionsOrder returns the display order for Pi presets.
func PiSubscriptionsOrder() []PiSubscription {
	return []PiSubscription{
		PiSubscriptionClaude,
		PiSubscriptionCodex,
		PiSubscriptionKiro,
		PiSubscriptionBudget,
	}
}

// PiSubscriptionDescription returns a concise human-readable description for a preset.
func PiSubscriptionDescription(sub PiSubscription) string {
	switch sub {
	case PiSubscriptionClaude:
		return "Anthropic via API key: Opus/Sonnet for architecture & verify, Sonnet for code, Haiku for light work"
	case PiSubscriptionCodex:
		return "Codex / ChatGPT: GPT-5.6 Sol for reasoning, Terra for code, Luna for light tasks"
	case PiSubscriptionKiro:
		return "Kiro Frontier: Claude 4.8 for reasoning, Sonnet 4.6 for execution, Haiku 4.5 for light work"
	case PiSubscriptionBudget:
		return "Cost-optimised: DeepSeek / GPT mini mix — high quality at minimal token expense"
	default:
		return ""
	}
}

// PiPresetClaude returns the agent model mapping for Anthropic via API key.
func PiPresetClaude() map[string]PiAgentModelEntry {
	strong := PiAgentModelEntry{Model: "anthropic/claude-sonnet-4", Thinking: "high"}
	code := PiAgentModelEntry{Model: "anthropic/claude-sonnet-4", Thinking: "medium"}
	light := PiAgentModelEntry{Model: "anthropic/claude-haiku-4", Thinking: "low"}

	return mapRoles(strong, code, light)
}

// PiPresetCodex returns the agent model mapping for OpenAI / Codex subscriptions.
func PiPresetCodex() map[string]PiAgentModelEntry {
	strong := PiAgentModelEntry{Model: "openai/gpt-5.6-sol", Thinking: "high"}
	code := PiAgentModelEntry{Model: "openai/gpt-5.6-terra", Thinking: "medium"}
	light := PiAgentModelEntry{Model: "openai/gpt-5.6-luna", Thinking: "low"}

	return mapRoles(strong, code, light)
}

// PiPresetKiro returns the agent model mapping for Kiro subscriptions.
func PiPresetKiro() map[string]PiAgentModelEntry {
	strong := PiAgentModelEntry{Model: "kiro/claude-opus-4.8", Thinking: "high"}
	code := PiAgentModelEntry{Model: "kiro/claude-sonnet-4.6", Thinking: "medium"}
	light := PiAgentModelEntry{Model: "kiro/claude-haiku-4.5", Thinking: "low"}

	return mapRoles(strong, code, light)
}

// PiPresetBudget returns the agent model mapping for cost-effective models.
func PiPresetBudget() map[string]PiAgentModelEntry {
	strong := PiAgentModelEntry{Model: "deepseek/deepseek-reasoner", Thinking: "medium"}
	code := PiAgentModelEntry{Model: "deepseek/deepseek-chat", Thinking: "low"}
	light := PiAgentModelEntry{Model: "openai/gpt-5.4-mini", Thinking: "off"}

	return mapRoles(strong, code, light)
}

// PiPresetForSubscription returns the preset mapping for a given provider preset.
func PiPresetForSubscription(sub PiSubscription) map[string]PiAgentModelEntry {
	switch sub {
	case PiSubscriptionClaude:
		return PiPresetClaude()
	case PiSubscriptionCodex:
		return PiPresetCodex()
	case PiSubscriptionKiro:
		return PiPresetKiro()
	case PiSubscriptionBudget:
		return PiPresetBudget()
	default:
		return PiPresetClaude()
	}
}

func mapRoles(strong, code, light PiAgentModelEntry) map[string]PiAgentModelEntry {
	return map[string]PiAgentModelEntry{
		// Reasoning & Architecture Tier
		"sdd-explore":         strong,
		"sdd-research":        strong,
		"sdd-proposal":        strong,
		"sdd-design":          strong,
		"sdd-verify":          strong,
		"jd-judge-a":          strong,
		"jd-judge-b":          strong,
		"review-risk":         strong,
		"review-reliability":  strong,
		"review-resilience":   strong,

		// Code & Execution Tier
		"sdd-apply":           code,
		"jd-fix-agent":        code,
		"default":             code,

		// Light & Maintenance Tier
		"sdd-spec":            light,
		"sdd-tasks":           light,
		"sdd-archive":         light,
		"sdd-onboard":         light,
		"sdd-status":          light,
		"sdd-sync":            light,
		"review-readability":  light,
	}
}
