package model

// ClaudeModelAlias represents one of the three Claude model tiers used for
// per-phase model assignments in the SDD orchestrator.
//
// Only three values are valid: ClaudeModelOpus, ClaudeModelSonnet, ClaudeModelHaiku.
type ClaudeModelAlias string

const (
	// ClaudeModelOpus is the high-capability tier, best for architectural decisions
	// and orchestration. Maps to the current claude-opus-* family.
	ClaudeModelOpus ClaudeModelAlias = "opus"

	// ClaudeModelSonnet is the balanced tier, suitable for most SDD phases.
	// Maps to the current claude-sonnet-* family.
	ClaudeModelSonnet ClaudeModelAlias = "sonnet"

	// ClaudeModelHaiku is the lightweight tier, ideal for mechanical tasks like
	// archiving or simple copy work. Maps to the current claude-haiku-* family.
	ClaudeModelHaiku ClaudeModelAlias = "haiku"
)

// String returns the string representation of the alias.
func (a ClaudeModelAlias) String() string {
	return string(a)
}

// Valid reports whether the alias is one of the three known Claude model tiers.
func (a ClaudeModelAlias) Valid() bool {
	switch a {
	case ClaudeModelOpus, ClaudeModelSonnet, ClaudeModelHaiku:
		return true
	default:
		return false
	}
}

// ClaudeModelPresetBalanced returns the default model assignment table.
// It balances cost and capability: orchestration and architecture phases use opus;
// implementation and validation use sonnet; archiving uses haiku.
func ClaudeModelPresetBalanced() map[string]ClaudeModelAlias {
	return map[string]ClaudeModelAlias{
		"orchestrator": ClaudeModelOpus,
		"sdd-explore":  ClaudeModelSonnet,
		"sdd-propose":  ClaudeModelOpus,
		"sdd-spec":     ClaudeModelSonnet,
		"sdd-design":   ClaudeModelOpus,
		"sdd-tasks":    ClaudeModelSonnet,
		"sdd-apply":    ClaudeModelSonnet,
		"sdd-verify":   ClaudeModelSonnet,
		"sdd-archive":  ClaudeModelHaiku,
		"default":      ClaudeModelSonnet,
	}
}

// ClaudeModelPresetPerformance returns a model assignment table optimised for
// output quality. Architecture, planning, and verification phases all use opus.
func ClaudeModelPresetPerformance() map[string]ClaudeModelAlias {
	return map[string]ClaudeModelAlias{
		"orchestrator": ClaudeModelOpus,
		"sdd-explore":  ClaudeModelSonnet,
		"sdd-propose":  ClaudeModelOpus,
		"sdd-spec":     ClaudeModelSonnet,
		"sdd-design":   ClaudeModelOpus,
		"sdd-tasks":    ClaudeModelSonnet,
		"sdd-apply":    ClaudeModelSonnet,
		"sdd-verify":   ClaudeModelOpus,
		"sdd-archive":  ClaudeModelHaiku,
		"default":      ClaudeModelSonnet,
	}
}

// ClaudeModelPresetEconomy returns a model assignment table optimised for cost.
// Every phase uses sonnet except archive, which uses haiku.
func ClaudeModelPresetEconomy() map[string]ClaudeModelAlias {
	return map[string]ClaudeModelAlias{
		"orchestrator": ClaudeModelSonnet,
		"sdd-explore":  ClaudeModelSonnet,
		"sdd-propose":  ClaudeModelSonnet,
		"sdd-spec":     ClaudeModelSonnet,
		"sdd-design":   ClaudeModelSonnet,
		"sdd-tasks":    ClaudeModelSonnet,
		"sdd-apply":    ClaudeModelSonnet,
		"sdd-verify":   ClaudeModelSonnet,
		"sdd-archive":  ClaudeModelHaiku,
		"default":      ClaudeModelSonnet,
	}
}
