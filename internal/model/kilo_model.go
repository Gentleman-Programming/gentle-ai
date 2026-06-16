package model

// KiloModelAlias represents a Kilo Gateway model choice for per-phase custom
// agent assignments. Known aliases are mapped to full model IDs via KiloModelID().
// Unknown aliases are passed through as-is, allowing any provider/model string
// that Kilo Gateway accepts (e.g. "openai/gpt-4o", "meta-llama/llama-4-maverick").
type KiloModelAlias string

// --- Kilo Gateway aliases ---
const (
	KiloModelAuto    KiloModelAlias = "auto"
	KiloModelGateway KiloModelAlias = "gateway"
)

// --- Anthropic models ---
const (
	KiloModelOpus    KiloModelAlias = "opus"
	KiloModelSonnet  KiloModelAlias = "sonnet"
	KiloModelHaiku   KiloModelAlias = "haiku"
	KiloModelSonnet4 KiloModelAlias = "claude-sonnet-4"
	KiloModelOpus4   KiloModelAlias = "claude-opus-4"
)

// --- OpenAI models ---
const (
	KiloModelGPT4o     KiloModelAlias = "gpt-4o"
	KiloModelGPT4oMini KiloModelAlias = "gpt-4o-mini"
	KiloModelO1        KiloModelAlias = "o1"
	KiloModelO3        KiloModelAlias = "o3"
	KiloModelO3Mini    KiloModelAlias = "o3-mini"
	KiloModelO4Mini    KiloModelAlias = "o4-mini"
)

// --- Google models ---
const (
	KiloModelGemini25Pro   KiloModelAlias = "gemini-2.5-pro"
	KiloModelGemini25Flash KiloModelAlias = "gemini-2.5-flash"
	KiloModelGemini20Flash KiloModelAlias = "gemini-2.0-flash"
)

// --- Open-weight models ---
const (
	KiloModelDeepSeek   KiloModelAlias = "deepseek"
	KiloModelDeepSeekR1 KiloModelAlias = "deepseek-r1"
	KiloModelQwen       KiloModelAlias = "qwen"
	KiloModelQwen3      KiloModelAlias = "qwen3"
	KiloModelLlama      KiloModelAlias = "llama"
	KiloModelMistral    KiloModelAlias = "mistral"
)

// KnownKiloAliases is the set of pre-defined Kilo model aliases.
// Used by the picker UI to populate the cycling list.
var KnownKiloAliases = []KiloModelAlias{
	// Kilo Gateway
	KiloModelAuto, KiloModelGateway,
	// Anthropic
	KiloModelOpus, KiloModelSonnet, KiloModelHaiku,
	KiloModelSonnet4, KiloModelOpus4,
	// OpenAI
	KiloModelGPT4o, KiloModelGPT4oMini,
	KiloModelO1, KiloModelO3, KiloModelO3Mini, KiloModelO4Mini,
	// Google
	KiloModelGemini25Pro, KiloModelGemini25Flash, KiloModelGemini20Flash,
	// Open-weight
	KiloModelDeepSeek, KiloModelDeepSeekR1,
	KiloModelQwen, KiloModelQwen3,
	KiloModelLlama, KiloModelMistral,
}

// KnownKiloModelLabels provides human-readable labels for the picker UI.
var KnownKiloModelLabels = map[KiloModelAlias]string{
	KiloModelAuto:          "Kilo Auto (free routing)",
	KiloModelGateway:       "Gateway Auto",
	KiloModelOpus:          "Claude Opus 4",
	KiloModelSonnet:        "Claude Sonnet 4",
	KiloModelHaiku:         "Claude Haiku",
	KiloModelSonnet4:       "Claude Sonnet 4 (latest)",
	KiloModelOpus4:         "Claude Opus 4 (latest)",
	KiloModelGPT4o:         "GPT-4o",
	KiloModelGPT4oMini:     "GPT-4o Mini",
	KiloModelO1:            "OpenAI o1",
	KiloModelO3:            "OpenAI o3",
	KiloModelO3Mini:        "OpenAI o3-mini",
	KiloModelO4Mini:        "OpenAI o4-mini",
	KiloModelGemini25Pro:   "Gemini 2.5 Pro",
	KiloModelGemini25Flash: "Gemini 2.5 Flash",
	KiloModelGemini20Flash: "Gemini 2.0 Flash",
	KiloModelDeepSeek:      "DeepSeek",
	KiloModelDeepSeekR1:    "DeepSeek R1",
	KiloModelQwen:          "Qwen",
	KiloModelQwen3:         "Qwen3",
	KiloModelLlama:         "Meta Llama",
	KiloModelMistral:       "Mistral",
}

// IsKnownKiloAlias reports whether the alias is one of the pre-defined Kilo model options.
func IsKnownKiloAlias(alias KiloModelAlias) bool {
	_, ok := KnownKiloModelLabels[alias]
	return ok
}

// KiloModelID maps a KiloModelAlias to the model identifier Kilo Gateway expects
// in the `model:` field of a custom agent frontmatter.
//
// Known aliases are mapped to their full provider/model IDs.
// Unknown aliases are passed through as-is, allowing any custom model string
// that Kilo Gateway accepts (e.g. "openai/gpt-4o", "anthropic/claude-sonnet-4-20250514").
//
// NOTE: Model IDs with date stamps (e.g. 20250514) should be updated when providers
// release new versions. Kilo Gateway may accept dateless aliases — test before updating.
func KiloModelID(alias KiloModelAlias) string {
	switch alias {
	// Kilo Gateway
	case KiloModelAuto:
		return "kilo/kilo-auto/free"
	case KiloModelGateway:
		return "gateway/auto"
	// Anthropic
	case KiloModelOpus:
		return "anthropic/claude-opus-4-20250514"
	case KiloModelSonnet:
		return "anthropic/claude-sonnet-4-20250514"
	case KiloModelHaiku:
		return "anthropic/claude-haiku-4-20250514"
	case KiloModelSonnet4:
		return "anthropic/claude-sonnet-4-20250514"
	case KiloModelOpus4:
		return "anthropic/claude-opus-4-20250514"
	// OpenAI
	case KiloModelGPT4o:
		return "openai/gpt-4o"
	case KiloModelGPT4oMini:
		return "openai/gpt-4o-mini"
	case KiloModelO1:
		return "openai/o1"
	case KiloModelO3:
		return "openai/o3"
	case KiloModelO3Mini:
		return "openai/o3-mini"
	case KiloModelO4Mini:
		return "openai/o4-mini"
	// Google
	case KiloModelGemini25Pro:
		return "google/gemini-2.5-pro"
	case KiloModelGemini25Flash:
		return "google/gemini-2.5-flash"
	case KiloModelGemini20Flash:
		return "google/gemini-2.0-flash"
	// Open-weight
	case KiloModelDeepSeek:
		return "deepseek/deepseek-chat"
	case KiloModelDeepSeekR1:
		return "deepseek/deepseek-reasoner"
	case KiloModelQwen:
		return "qwen/qwen-2.5-72b-instruct"
	case KiloModelQwen3:
		return "qwen/qwen3-235b-a22b"
	case KiloModelLlama:
		return "meta-llama/llama-4-maverick"
	case KiloModelMistral:
		return "mistral/mistral-large-latest"
	default:
		// Pass-through: unknown alias is used as-is as the model ID.
		// This allows users to type any provider/model string that Kilo Gateway accepts.
		return string(alias)
	}
}

// Valid reports whether the alias is recognized or is a custom pass-through string.
// Always returns true since unknown aliases pass through as model IDs.
func (a KiloModelAlias) Valid() bool {
	return true
}

// KiloModelPresetFree returns the default assignment table for Kilo free tier.
// Most phases use auto routing; archive/onboard use Haiku for cost efficiency.
func KiloModelPresetFree() map[string]KiloModelAlias {
	return map[string]KiloModelAlias{
		"orchestrator": KiloModelAuto,
		"sdd-explore":  KiloModelAuto,
		"sdd-propose":  KiloModelAuto,
		"sdd-spec":     KiloModelAuto,
		"sdd-design":   KiloModelAuto,
		"sdd-tasks":    KiloModelAuto,
		"sdd-apply":    KiloModelAuto,
		"sdd-verify":   KiloModelAuto,
		"sdd-archive":  KiloModelHaiku,
		"sdd-onboard":  KiloModelHaiku,
		"default":      KiloModelAuto,
	}
}

// KiloModelPresetBalanced is a deprecated alias for KiloModelPresetFree.
// Deprecated: Use KiloModelPresetFree for free tier or KiloModelPresetQuality for paid.
func KiloModelPresetBalanced() map[string]KiloModelAlias {
	return KiloModelPresetFree()
}

// KiloModelPresetQuality returns assignments optimized for Kilo paid tier.
// Uses Sonnet for reasoning-heavy phases (design, spec, apply) and
// Haiku for lightweight phases (archive, onboard, init).
func KiloModelPresetQuality() map[string]KiloModelAlias {
	return map[string]KiloModelAlias{
		"orchestrator": KiloModelAuto,
		"sdd-explore":  KiloModelSonnet,
		"sdd-propose":  KiloModelSonnet,
		"sdd-spec":     KiloModelSonnet,
		"sdd-design":   KiloModelOpus,
		"sdd-tasks":    KiloModelSonnet,
		"sdd-apply":    KiloModelSonnet,
		"sdd-verify":   KiloModelSonnet,
		"sdd-archive":  KiloModelHaiku,
		"sdd-onboard":  KiloModelHaiku,
		"default":      KiloModelAuto,
	}
}
