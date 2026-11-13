package model

// KiroModelAlias represents a Kiro-native model choice for role assignments.
type KiroModelAlias string

const (
	KiroModelAuto     KiroModelAlias = "auto"
	KiroModelOpus     KiroModelAlias = "opus"
	KiroModelSonnet   KiroModelAlias = "sonnet"
	KiroModelHaiku    KiroModelAlias = "haiku"
	KiroModelMiniMax  KiroModelAlias = "minimax"
	KiroModelGLM      KiroModelAlias = "glm"
	KiroModelDeepSeek KiroModelAlias = "deepseek"
	KiroModelQwen     KiroModelAlias = "qwen"
)

// Valid reports whether the alias is one of the known Kiro model options.
func (a KiroModelAlias) Valid() bool {
	switch a {
	case KiroModelAuto, KiroModelOpus, KiroModelSonnet, KiroModelHaiku,
		KiroModelMiniMax, KiroModelGLM, KiroModelDeepSeek, KiroModelQwen:
		return true
	default:
		return false
	}
}

// KiroModelID maps a KiroModelAlias to the model identifier Kiro expects
// in the `model:` field of a custom agent frontmatter.
//
// Kiro model IDs do not include a provider prefix — they are passed directly
// as the `model` key in ~/.kiro/agents/*.md frontmatter. Every non-auto ID
// below appears in `kiro-cli chat --list-models`; the Claude 5.5 IDs are
// listed only for paid plans, so Kiro Free users should pick the open-weight
// preset or open-weight custom assignments.
//
// Kiro rejects an agent whose pinned model is unknown or unavailable instead
// of falling back, so unknown aliases resolve to auto.
//
// References: https://kiro.dev/docs/models/
func KiroModelID(alias KiroModelAlias) string {
	switch alias {
	case KiroModelOpus:
		return "claude-opus-5.5"
	case KiroModelSonnet:
		return "claude-sonnet-5.5"
	case KiroModelHaiku:
		return "claude-haiku-5.5"
	case KiroModelMiniMax:
		return "minimax-m2.5"
	case KiroModelGLM:
		return "glm-5"
	case KiroModelDeepSeek:
		return "deepseek-3.2"
	case KiroModelQwen:
		return "qwen3-coder-next"
	default:
		return "auto"
	}
}

// Paid presets use only Claude 5.5. GPT-5.6 Sol (4.40x credits) and Terra
// (2.20x) cost more than Opus 5.5 (2.00x), and Haiku 5.5 (0.06x) is both
// cheaper and stronger than GPT-5.6 Luna (0.60x) and the open-weight models.

// KiroModelPresetBalanced uses Claude Sonnet 5.5 for reasoning-heavy roles and
// Claude Haiku 5.5 for exploration, implementation, and the second judge.
func KiroModelPresetBalanced() map[string]KiroModelAlias {
	return map[string]KiroModelAlias{
		"orchestrator": KiroModelAuto,
		"odd-explorer": KiroModelHaiku,
		"odd-worker":   KiroModelHaiku,
		"odd-verify":   KiroModelSonnet,
		"jd-judge-a":   KiroModelSonnet,
		"jd-judge-b":   KiroModelHaiku,
		"jd-fix-agent": KiroModelSonnet,
		"risk":         KiroModelSonnet,
		"readability":  KiroModelHaiku,
		"reliability":  KiroModelSonnet,
		"resilience":   KiroModelSonnet,
		"refuter":      KiroModelSonnet,
		"validator":    KiroModelSonnet,
		"default":      KiroModelAuto,
	}
}

// KiroModelPresetPerformance prioritizes Claude Opus 5.5 and Sonnet 5.5.
func KiroModelPresetPerformance() map[string]KiroModelAlias {
	return map[string]KiroModelAlias{
		"orchestrator": KiroModelOpus,
		"odd-explorer": KiroModelSonnet,
		"odd-worker":   KiroModelSonnet,
		"odd-verify":   KiroModelOpus,
		"jd-judge-a":   KiroModelOpus,
		"jd-judge-b":   KiroModelOpus,
		"jd-fix-agent": KiroModelSonnet,
		"risk":         KiroModelOpus,
		"readability":  KiroModelSonnet,
		"reliability":  KiroModelOpus,
		"resilience":   KiroModelOpus,
		"refuter":      KiroModelOpus,
		"validator":    KiroModelOpus,
		"default":      KiroModelSonnet,
	}
}

// KiroModelPresetEconomy uses Claude Haiku 5.5, Kiro's cheapest strong model,
// for every role except the auto-routed orchestrator.
func KiroModelPresetEconomy() map[string]KiroModelAlias {
	return map[string]KiroModelAlias{
		"orchestrator": KiroModelAuto,
		"odd-explorer": KiroModelHaiku,
		"odd-worker":   KiroModelHaiku,
		"odd-verify":   KiroModelHaiku,
		"jd-judge-a":   KiroModelHaiku,
		"jd-judge-b":   KiroModelHaiku,
		"jd-fix-agent": KiroModelHaiku,
		"risk":         KiroModelHaiku,
		"readability":  KiroModelHaiku,
		"reliability":  KiroModelHaiku,
		"resilience":   KiroModelHaiku,
		"refuter":      KiroModelHaiku,
		"validator":    KiroModelHaiku,
		"default":      KiroModelHaiku,
	}
}

// KiroModelPresetOpenWeight uses only Kiro's open-weight model families.
func KiroModelPresetOpenWeight() map[string]KiroModelAlias {
	return map[string]KiroModelAlias{
		"orchestrator": KiroModelGLM,
		"odd-explorer": KiroModelQwen,
		"odd-worker":   KiroModelQwen,
		"odd-verify":   KiroModelMiniMax,
		"jd-judge-a":   KiroModelGLM,
		"jd-judge-b":   KiroModelQwen,
		"jd-fix-agent": KiroModelQwen,
		"risk":         KiroModelGLM,
		"readability":  KiroModelQwen,
		"reliability":  KiroModelMiniMax,
		"resilience":   KiroModelMiniMax,
		"refuter":      KiroModelDeepSeek,
		"validator":    KiroModelDeepSeek,
		"default":      KiroModelQwen,
	}
}
