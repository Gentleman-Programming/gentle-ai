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
// as the `model` key in ~/.kiro/agents/*.md frontmatter.
//
// References: https://kiro.dev/docs/models/
func KiroModelID(alias KiroModelAlias) string {
	switch alias {
	case KiroModelAuto:
		return "auto"
	case KiroModelOpus:
		return "claude-opus-4.8"
	case KiroModelHaiku:
		return "claude-haiku-4.5"
	case KiroModelMiniMax:
		return "minimax-m2.5"
	case KiroModelGLM:
		return "glm-5"
	case KiroModelDeepSeek:
		return "deepseek-3.2"
	case KiroModelQwen:
		return "qwen3-coder-next"
	default:
		return "claude-sonnet-4.6"
	}
}

// KiroModelPresetBalanced lets Kiro route ODD and review roles automatically.
func KiroModelPresetBalanced() map[string]KiroModelAlias {
	return map[string]KiroModelAlias{
		"orchestrator": KiroModelAuto,
		"odd-explorer": KiroModelAuto,
		"odd-worker":   KiroModelAuto,
		"odd-verify":   KiroModelAuto,
		"jd-judge-a":   KiroModelAuto,
		"jd-judge-b":   KiroModelAuto,
		"jd-fix-agent": KiroModelAuto,
		"risk":         KiroModelAuto,
		"readability":  KiroModelAuto,
		"reliability":  KiroModelAuto,
		"resilience":   KiroModelAuto,
		"refuter":      KiroModelAuto,
		"validator":    KiroModelAuto,
		"default":      KiroModelAuto,
	}
}

// KiroModelPresetPerformance prioritizes frontier Claude-family models.
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

// KiroModelPresetEconomy prioritizes Kiro's low-credit open-weight options.
func KiroModelPresetEconomy() map[string]KiroModelAlias {
	return map[string]KiroModelAlias{
		"orchestrator": KiroModelAuto,
		"odd-explorer": KiroModelQwen,
		"odd-worker":   KiroModelQwen,
		"odd-verify":   KiroModelDeepSeek,
		"jd-judge-a":   KiroModelDeepSeek,
		"jd-judge-b":   KiroModelQwen,
		"jd-fix-agent": KiroModelQwen,
		"risk":         KiroModelDeepSeek,
		"readability":  KiroModelQwen,
		"reliability":  KiroModelMiniMax,
		"resilience":   KiroModelMiniMax,
		"refuter":      KiroModelDeepSeek,
		"validator":    KiroModelDeepSeek,
		"default":      KiroModelQwen,
	}
}

// KiroModelPresetOpenWeight favors Kiro's non-Claude model families.
func KiroModelPresetOpenWeight() map[string]KiroModelAlias {
	base := KiroModelPresetEconomy()
	base["orchestrator"] = KiroModelGLM
	base["odd-verify"] = KiroModelMiniMax
	base["jd-judge-a"] = KiroModelGLM
	base["risk"] = KiroModelGLM
	return base
}
