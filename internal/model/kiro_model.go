package model

// KiroModelAlias represents a Kiro-native model choice for role assignments.
type KiroModelAlias string

const (
	KiroModelAuto     KiroModelAlias = "auto"
	KiroModelOpus     KiroModelAlias = "opus"
	KiroModelSonnet   KiroModelAlias = "sonnet"
	KiroModelHaiku    KiroModelAlias = "haiku"
	KiroModelLuna     KiroModelAlias = "luna"
	KiroModelTerra    KiroModelAlias = "terra"
	KiroModelSol      KiroModelAlias = "sol"
	KiroModelMiniMax  KiroModelAlias = "minimax"
	KiroModelGLM      KiroModelAlias = "glm"
	KiroModelDeepSeek KiroModelAlias = "deepseek"
	KiroModelQwen     KiroModelAlias = "qwen"
)

// KiroModelID maps a KiroModelAlias to the model identifier Kiro expects
// in the `model:` field of a custom agent frontmatter.
//
// Kiro model IDs do not include a provider prefix — they are passed directly
// as the `model` key in ~/.kiro/agents/*.md frontmatter. The opus and sonnet
// aliases track the newest Claude generation Kiro offers.
//
// References: https://kiro.dev/docs/models/
func KiroModelID(alias KiroModelAlias) string {
	switch alias {
	case KiroModelAuto:
		return "auto"
	case KiroModelOpus:
		return "claude-opus-5.5"
	case KiroModelHaiku:
		return "claude-haiku-4.5"
	case KiroModelLuna:
		return "gpt-5.6-luna"
	case KiroModelTerra:
		return "gpt-5.6-terra"
	case KiroModelSol:
		return "gpt-5.6-sol"
	case KiroModelMiniMax:
		return "minimax-m2.5"
	case KiroModelGLM:
		return "glm-5"
	case KiroModelDeepSeek:
		return "deepseek-3.2"
	case KiroModelQwen:
		return "qwen3-coder-next"
	default:
		return "claude-sonnet-5.5"
	}
}

// Presets never assign GPT-5.6 Sol or Terra: Sol is among Kiro's most
// expensive models and Terra costs more than Opus 5.5 while trailing it.
// Both stay selectable in the custom picker.

// KiroModelPresetBalanced mixes Claude Sonnet 5.5 and GPT-5.6 Luna so the two
// Judgment Day judges come from different model families at near-Auto cost.
func KiroModelPresetBalanced() map[string]KiroModelAlias {
	return map[string]KiroModelAlias{
		"orchestrator": KiroModelAuto,
		"odd-explorer": KiroModelLuna,
		"odd-worker":   KiroModelLuna,
		"odd-verify":   KiroModelSonnet,
		"jd-judge-a":   KiroModelSonnet,
		"jd-judge-b":   KiroModelLuna,
		"jd-fix-agent": KiroModelSonnet,
		"risk":         KiroModelSonnet,
		"readability":  KiroModelLuna,
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

// KiroModelPresetEconomy uses GPT-5.6 Luna, Kiro's strongest low-credit
// model, for most roles and MiniMax for the reliability and resilience lenses.
func KiroModelPresetEconomy() map[string]KiroModelAlias {
	return map[string]KiroModelAlias{
		"orchestrator": KiroModelAuto,
		"odd-explorer": KiroModelLuna,
		"odd-worker":   KiroModelLuna,
		"odd-verify":   KiroModelLuna,
		"jd-judge-a":   KiroModelLuna,
		"jd-judge-b":   KiroModelLuna,
		"jd-fix-agent": KiroModelLuna,
		"risk":         KiroModelLuna,
		"readability":  KiroModelLuna,
		"reliability":  KiroModelMiniMax,
		"resilience":   KiroModelMiniMax,
		"refuter":      KiroModelLuna,
		"validator":    KiroModelLuna,
		"default":      KiroModelLuna,
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
