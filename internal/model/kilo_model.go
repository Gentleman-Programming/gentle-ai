package model

// KiloModelAlias represents a Kilo Gateway model choice for per-phase custom
// agent assignments.
type KiloModelAlias string

const (
	KiloModelAuto    KiloModelAlias = "auto"
	KiloModelSonnet  KiloModelAlias = "sonnet"
	KiloModelOpus    KiloModelAlias = "opus"
	KiloModelHaiku   KiloModelAlias = "haiku"
	KiloModelGateway KiloModelAlias = "gateway" // Kilo Gateway free routing
)

// Valid reports whether the alias is one of the known Kilo model options.
func (a KiloModelAlias) Valid() bool {
	switch a {
	case KiloModelAuto, KiloModelSonnet, KiloModelOpus, KiloModelHaiku, KiloModelGateway:
		return true
	default:
		return false
	}
}

// KiloModelID maps a KiloModelAlias to the model identifier Kilo Gateway expects
// in the `model:` field of a custom agent frontmatter.
//
// Kilo Gateway model IDs include a provider prefix (e.g. "anthropic/claude-sonnet-4-20250514")
// that differs from Kiro's bare IDs.
//
// NOTE: Model IDs include date stamps (20250514). Update these when Anthropic
// releases new versions. Kilo Gateway may accept dateless aliases — test before
// updating. See: https://docs.kilo.ai/gateway/models
func KiloModelID(alias KiloModelAlias) string {
	switch alias {
	case KiloModelAuto:
		return "kilo/kilo-auto/free"
	case KiloModelSonnet:
		return "anthropic/claude-sonnet-4-20250514"
	case KiloModelOpus:
		return "anthropic/claude-opus-4-20250514"
	case KiloModelHaiku:
		return "anthropic/claude-haiku-4-20250514"
	case KiloModelGateway:
		return "gateway/auto"
	default:
		return "anthropic/claude-sonnet-4-20250514"
	}
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
