package model

// KiroModelAlias represents a Kiro-native model choice for per-phase custom
// agent assignments.
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

// KiroModelPresetBalanced returns the default Kiro-native assignment table.
// Auto lets Kiro route most phases while keeping archive/onboard lightweight.
func KiroModelPresetBalanced() map[string]KiroModelAlias {
	return map[string]KiroModelAlias{
		"orchestrator": KiroModelAuto,
		"sdd-explore":  KiroModelAuto,
		"sdd-propose":  KiroModelAuto,
		"sdd-spec":     KiroModelAuto,
		"sdd-design":   KiroModelOpus,
		"sdd-tasks":    KiroModelAuto,
		"sdd-apply":    KiroModelAuto,
		"sdd-verify":   KiroModelAuto,
		"sdd-archive":  KiroModelHaiku,
		"sdd-onboard":  KiroModelHaiku,
		"jd-judge-a":   KiroModelAuto,
		"jd-judge-b":   KiroModelAuto,
		"jd-fix-agent": KiroModelAuto,
		"default":      KiroModelAuto,
	}
}

// KiroModelPresetPerformance prioritizes frontier Claude-family models.
func KiroModelPresetPerformance() map[string]KiroModelAlias {
	return map[string]KiroModelAlias{
		"orchestrator": KiroModelOpus,
		"sdd-explore":  KiroModelSonnet,
		"sdd-propose":  KiroModelOpus,
		"sdd-spec":     KiroModelSonnet,
		"sdd-design":   KiroModelOpus,
		"sdd-tasks":    KiroModelSonnet,
		"sdd-apply":    KiroModelSonnet,
		"sdd-verify":   KiroModelOpus,
		"sdd-archive":  KiroModelHaiku,
		"sdd-onboard":  KiroModelHaiku,
		"jd-judge-a":   KiroModelOpus,
		"jd-judge-b":   KiroModelOpus,
		"jd-fix-agent": KiroModelSonnet,
		"default":      KiroModelSonnet,
	}
}

// KiroModelPresetEconomy prioritizes Kiro's low-credit open-weight options.
func KiroModelPresetEconomy() map[string]KiroModelAlias {
	return map[string]KiroModelAlias{
		"orchestrator": KiroModelAuto,
		"sdd-explore":  KiroModelQwen,
		"sdd-propose":  KiroModelDeepSeek,
		"sdd-spec":     KiroModelQwen,
		"sdd-design":   KiroModelMiniMax,
		"sdd-tasks":    KiroModelQwen,
		"sdd-apply":    KiroModelQwen,
		"sdd-verify":   KiroModelDeepSeek,
		"sdd-archive":  KiroModelQwen,
		"sdd-onboard":  KiroModelQwen,
		"jd-judge-a":   KiroModelDeepSeek,
		"jd-judge-b":   KiroModelQwen,
		"jd-fix-agent": KiroModelQwen,
		"default":      KiroModelQwen,
	}
}

// KiroModelPresetOpenWeight favors Kiro's non-Claude model families.
func KiroModelPresetOpenWeight() map[string]KiroModelAlias {
	base := KiroModelPresetEconomy()
	base["sdd-design"] = KiroModelGLM
	base["sdd-verify"] = KiroModelMiniMax
	base["jd-judge-a"] = KiroModelGLM
	return base
}
