package model

import (
	"fmt"
	"strings"
)

// codexAvailableModels is the curated list of Codex model IDs available for
// per-phase custom assignments. Order is intentional: newest/most-capable first.
var codexAvailableModels = []string{
	"gpt-5.5",
	"gpt-5.4",
	"gpt-5.4-mini",
	"gpt-5.2-codex",
	"gpt-5.3-codex",
}

// CodexAvailableModels returns the curated list of Codex model IDs that can be
// assigned per-phase in the Custom picker. The slice is a copy — mutations do
// not affect the canonical list.
func CodexAvailableModels() []string {
	out := make([]string, len(codexAvailableModels))
	copy(out, codexAvailableModels)
	return out
}

// FilterCodexModels returns the subset of CodexAvailableModels whose ID contains
// query as a case-insensitive substring. An empty query returns all models.
func FilterCodexModels(query string) []string {
	all := CodexAvailableModels()
	if strings.TrimSpace(query) == "" {
		return all
	}
	q := strings.ToLower(query)
	out := make([]string, 0, len(all))
	for _, m := range all {
		if strings.Contains(strings.ToLower(m), q) {
			out = append(out, m)
		}
	}
	return out
}

// CodexEffort represents an OpenAI reasoning_effort level used for Codex
// per-phase delegation via spawn_agent.
type CodexEffort string

const (
	CodexEffortLow    CodexEffort = "low"
	CodexEffortMedium CodexEffort = "medium"
	CodexEffortHigh   CodexEffort = "high"
	CodexEffortXHigh  CodexEffort = "xhigh"
)

// Valid reports whether the effort value is one of the four known levels.
func (e CodexEffort) Valid() bool {
	switch e {
	case CodexEffortLow, CodexEffortMedium, CodexEffortHigh, CodexEffortXHigh:
		return true
	default:
		return false
	}
}

// CodexModelPresetRecommended returns the Recommended (ChatGPT Pro $100/mo) preset.
// Carril-aligned effort: Razonamiento=high, Código=medium, Liviano=low.
// Every phase within a carril carries the same effort so that maxEffort over the
// carril's phases yields exactly the carril's intended tier.
func CodexModelPresetRecommended() map[string]CodexEffort {
	return map[string]CodexEffort{
		// Razonamiento (sdd-strong): high
		"sdd-propose": CodexEffortHigh,
		"sdd-design":  CodexEffortHigh,
		"sdd-verify":  CodexEffortHigh,
		"jd-judge-a":  CodexEffortHigh,
		"jd-judge-b":  CodexEffortHigh,
		"default":     CodexEffortHigh,
		// Código (sdd-mid): medium
		"sdd-apply":    CodexEffortMedium,
		"jd-fix-agent": CodexEffortMedium,
		// Liviano (sdd-cheap): low
		"sdd-explore": CodexEffortLow,
		"sdd-spec":    CodexEffortLow,
		"sdd-tasks":   CodexEffortLow,
		"sdd-archive": CodexEffortLow,
		"sdd-onboard": CodexEffortLow,
	}
}

// CodexModelPresetPowerful returns the Powerful (ChatGPT Pro $200/mo) preset.
// Carril-aligned effort: Razonamiento=xhigh, Código=high, Liviano=low.
// Every phase within a carril carries the same effort so that maxEffort over the
// carril's phases yields exactly the carril's intended tier.
func CodexModelPresetPowerful() map[string]CodexEffort {
	return map[string]CodexEffort{
		// Razonamiento (sdd-strong): xhigh
		"sdd-propose": CodexEffortXHigh,
		"sdd-design":  CodexEffortXHigh,
		"sdd-verify":  CodexEffortXHigh,
		"jd-judge-a":  CodexEffortXHigh,
		"jd-judge-b":  CodexEffortXHigh,
		"default":     CodexEffortXHigh,
		// Código (sdd-mid): high
		"sdd-apply":    CodexEffortHigh,
		"jd-fix-agent": CodexEffortHigh,
		// Liviano (sdd-cheap): low
		"sdd-explore": CodexEffortLow,
		"sdd-spec":    CodexEffortLow,
		"sdd-tasks":   CodexEffortLow,
		"sdd-archive": CodexEffortLow,
		"sdd-onboard": CodexEffortLow,
	}
}

// CodexModelPresetLowCost returns the Low-cost (ChatGPT Plus $20/mo) preset.
// Carril-aligned effort: Razonamiento=medium, Código=medium, Liviano=low.
// Every phase within a carril carries the same effort so that maxEffort over the
// carril's phases yields exactly the carril's intended tier.
func CodexModelPresetLowCost() map[string]CodexEffort {
	return map[string]CodexEffort{
		// Razonamiento (sdd-strong): medium
		"sdd-propose": CodexEffortMedium,
		"sdd-design":  CodexEffortMedium,
		"sdd-verify":  CodexEffortMedium,
		"jd-judge-a":  CodexEffortMedium,
		"jd-judge-b":  CodexEffortMedium,
		"default":     CodexEffortMedium,
		// Código (sdd-mid): medium
		"sdd-apply":    CodexEffortMedium,
		"jd-fix-agent": CodexEffortMedium,
		// Liviano (sdd-cheap): low
		"sdd-explore": CodexEffortLow,
		"sdd-spec":    CodexEffortLow,
		"sdd-tasks":   CodexEffortLow,
		"sdd-archive": CodexEffortLow,
		"sdd-onboard": CodexEffortLow,
	}
}

// CodexTierGroup defines one CLI profile tier: the profile filename (without
// extension), the canonical default model id for that carril, the default
// reasoning_effort tier, and the SDD phases covered.
//
// Phase groupings (Approach C — orthogonal carril axis):
//   - sdd-strong (Razonamiento): propose, design, verify, judge-a, judge-b, default
//   - sdd-mid    (Código):       apply, fix-agent
//   - sdd-cheap  (Liviano):      explore, spec, tasks, archive, onboard
type CodexTierGroup struct {
	Profile       string
	Model         string
	DefaultEffort CodexEffort
	Phases        []string
}

// codexTierGroups defines the three CLI profile tiers and which phases they cover.
//
// Invariant: within each carril, ALL phases carry the same effort value in every
// preset constructor (CodexModelPresetLowCost, CodexModelPresetRecommended,
// CodexModelPresetPowerful). This guarantees that maxEffort over a carril's phases
// always yields the carril's intended effort tier — never an accidental max from a
// stale per-phase value.
//
// DefaultEffort values match CodexModelPresetRecommended so that the nil-input
// fallback in RenderCodexPhaseEfforts and the nil-input fallback in
// resolveProfileAssignments agree on the same canonical tier values:
//
//	Carril      LowCost(+$20)  Recommended($100)  Powerful($200)
//	sdd-strong  medium         high               xhigh
//	sdd-mid     medium         medium             high
//	sdd-cheap   low            low                low
var codexTierGroups = []CodexTierGroup{
	{
		Profile:       "sdd-strong",
		Model:         "gpt-5.5",
		DefaultEffort: CodexEffortHigh,
		Phases:        []string{"sdd-propose", "sdd-design", "sdd-verify", "jd-judge-a", "jd-judge-b", "default"},
	},
	{
		Profile:       "sdd-mid",
		Model:         "gpt-5.5",
		DefaultEffort: CodexEffortMedium,
		Phases:        []string{"sdd-apply", "jd-fix-agent"},
	},
	{
		Profile:       "sdd-cheap",
		Model:         "gpt-5.4-mini",
		DefaultEffort: CodexEffortLow,
		Phases:        []string{"sdd-explore", "sdd-spec", "sdd-tasks", "sdd-archive", "sdd-onboard"},
	},
}

// CodexTierGroups returns the canonical tier group definitions used by the
// three SDD profile carriles. Callers (e.g. the inject layer) should derive
// profile assignments from this slice rather than maintaining a separate table.
func CodexTierGroups() []CodexTierGroup {
	return codexTierGroups
}

// DefaultCarrilModels returns the canonical default model id for each carril.
// Used when state.CodexCarrilModelAssignments is absent (old state files).
func DefaultCarrilModels() map[string]string {
	m := make(map[string]string, len(codexTierGroups))
	for _, g := range codexTierGroups {
		m[g.Profile] = g.Model
	}
	return m
}

// codexEffortRank maps effort levels to a numeric rank for max-derivation.
var codexEffortRank = map[CodexEffort]int{
	CodexEffortLow:    0,
	CodexEffortMedium: 1,
	CodexEffortHigh:   2,
	CodexEffortXHigh:  3,
}

func maxEffort(assignments map[string]CodexEffort, phases []string) CodexEffort {
	best := CodexEffortLow
	for _, phase := range phases {
		e, ok := assignments[phase]
		if !ok {
			continue
		}
		if codexEffortRank[e] > codexEffortRank[best] {
			best = e
		}
	}
	return best
}

// RenderCodexPhaseEfforts renders the Model Profiles table for the Codex
// sdd-orchestrator.md asset. The table maps CLI profile names to their model,
// reasoning_effort tier, and covered SDD phases. The output is deterministic:
// tier groups are always rendered in codexTierGroups order.
//
// When assignments is nil or empty, falls back to CodexModelPresetRecommended.
// When carrilModels is nil or empty, falls back to DefaultCarrilModels.
func RenderCodexPhaseEfforts(assignments map[string]CodexEffort, carrilModels map[string]string) string {
	if len(assignments) == 0 {
		assignments = CodexModelPresetRecommended()
	}
	if len(carrilModels) == 0 {
		carrilModels = DefaultCarrilModels()
	}

	tierPhaseLabels := map[string]string{
		"sdd-strong": "propose, design, verify, judge",
		"sdd-mid":    "apply, fix-agent",
		"sdd-cheap":  "explore, spec, tasks, archive, onboard",
	}

	var sb strings.Builder
	sb.WriteString("| Profile (CLI) | Model | `reasoning_effort` (spawn_agent) | SDD phases |\n")
	sb.WriteString("|---------------|-------|----------------------------------|------------|\n")

	for _, tier := range codexTierGroups {
		effort := maxEffort(assignments, tier.Phases)
		phases := tierPhaseLabels[tier.Profile]
		modelID := carrilModels[tier.Profile]
		if modelID == "" {
			modelID = tier.Model
		}
		sb.WriteString(fmt.Sprintf("| `%s` | `%s` | `%s` | %s |\n",
			tier.Profile,
			modelID,
			effort,
			phases,
		))
	}

	return sb.String()
}

// codexPhaseOrder is the canonical phase ordering for the per-phase table,
// matching codexTierGroups phase groupings.
var codexPhaseOrder = []string{
	"sdd-explore", "sdd-propose", "sdd-spec", "sdd-design", "sdd-tasks",
	"sdd-apply", "sdd-verify", "sdd-archive", "sdd-onboard",
	"jd-judge-a", "jd-judge-b", "jd-fix-agent", "default",
}

// phaseToCarrilModel returns the default model id for a phase by looking up its
// carril via codexTierGroups.
func phaseToCarrilModel(phase string, carrilModels map[string]string) string {
	for _, tier := range codexTierGroups {
		for _, p := range tier.Phases {
			if p == phase {
				if m := carrilModels[tier.Profile]; m != "" {
					return m
				}
				return tier.Model
			}
		}
	}
	return "gpt-5.5" // ultimate fallback
}

// RenderCodexPhaseEffortsByPhase renders a per-phase Markdown table for the
// Codex sdd-orchestrator.md asset when Custom per-phase model assignments are
// active. Each row shows: phase | model | reasoning_effort.
//
// phaseModels maps phase names to custom model IDs. Phases not present in
// phaseModels fall back to the carril default model. efforts maps phase names to
// CodexEffort values (typically from a preset + user overrides). When efforts is
// nil, CodexModelPresetRecommended is used.
//
// The output is deterministic: phases are always rendered in codexPhaseOrder.
func RenderCodexPhaseEffortsByPhase(phaseModels map[string]string, efforts map[string]CodexEffort) string {
	if len(efforts) == 0 {
		efforts = CodexModelPresetRecommended()
	}
	carrilModels := DefaultCarrilModels()

	var sb strings.Builder
	sb.WriteString("| Phase | Model | `reasoning_effort` |\n")
	sb.WriteString("|-------|-------|--------------------|\n")

	for _, phase := range codexPhaseOrder {
		// Resolve model: custom per-phase override takes priority over carril default.
		modelID := ""
		if phaseModels != nil {
			modelID = phaseModels[phase]
		}
		if modelID == "" {
			modelID = phaseToCarrilModel(phase, carrilModels)
		}

		effort := efforts[phase]
		if effort == "" {
			effort = CodexEffortMedium // safe fallback
		}

		sb.WriteString(fmt.Sprintf("| `%s` | `%s` | `%s` |\n", phase, modelID, effort))
	}

	return sb.String()
}
