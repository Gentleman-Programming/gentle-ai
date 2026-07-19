package model

import (
	"fmt"
	"strings"
)

// codexModelCatalog is Gentle AI's curated selectable Codex model catalog for
// per-phase custom assignments. It is a UI/configuration catalog, not a runtime
// availability probe; the Codex CLI remains the source of truth at execution
// time. Order is intentional: newest/most-capable first.
var codexModelCatalog = []string{
	"gpt-5.6-sol",
	"gpt-5.6-terra",
	"gpt-5.6-luna",
	"gpt-5.5",
	"gpt-5.4",
	"gpt-5.4-mini",
	"gpt-5.3-codex",
	"gpt-5.2-codex",
}

// CodexAvailableModels returns Gentle AI's curated selectable Codex model
// catalog for per-phase Custom picker assignments. The slice is a copy —
// mutations do not affect the canonical catalog.
func CodexAvailableModels() []string {
	out := make([]string, len(codexModelCatalog))
	copy(out, codexModelCatalog)
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

type CodexCarrilDefault struct {
	Model  string
	Effort CodexEffort
}

type CodexPresetKey string

const (
	CodexPresetLowCost     CodexPresetKey = "low-cost"
	CodexPresetRecommended CodexPresetKey = "recommended"
	CodexPresetPowerful    CodexPresetKey = "powerful"
)

var codexPresetMatrix = map[CodexPresetKey]map[string]CodexCarrilDefault{
	CodexPresetLowCost: {
		"sdd-strong": {Model: "gpt-5.6-terra", Effort: CodexEffortMedium},
		"sdd-mid":    {Model: "gpt-5.6-terra", Effort: CodexEffortMedium},
		"sdd-cheap":  {Model: "gpt-5.6-luna", Effort: CodexEffortLow},
	},
	CodexPresetRecommended: {
		"sdd-strong": {Model: "gpt-5.6-sol", Effort: CodexEffortMedium},
		"sdd-mid":    {Model: "gpt-5.6-terra", Effort: CodexEffortMedium},
		"sdd-cheap":  {Model: "gpt-5.6-luna", Effort: CodexEffortLow},
	},
	CodexPresetPowerful: {
		"sdd-strong": {Model: "gpt-5.6-sol", Effort: CodexEffortHigh},
		"sdd-mid":    {Model: "gpt-5.6-terra", Effort: CodexEffortHigh},
		"sdd-cheap":  {Model: "gpt-5.6-luna", Effort: CodexEffortLow},
	},
}

// CodexOrchestratorAssignment is the explicit top-level Codex session model
// selected by a Gentle AI preset. It is separate from delegated SDD carriles.
type CodexOrchestratorAssignment struct {
	Model  string
	Effort CodexEffort
}

// CodexPresetOrchestratorAssignment returns the main-session policy for a
// named preset. All curated presets keep orchestration responsive at low effort.
// Unknown keys intentionally fall back to Recommended.
func CodexPresetOrchestratorAssignment(preset string) *CodexOrchestratorAssignment {
	if _, ok := codexPresetMatrix[CodexPresetKey(preset)]; !ok {
		preset = string(CodexPresetRecommended)
	}
	return &CodexOrchestratorAssignment{Model: "gpt-5.6-sol", Effort: CodexEffortLow}
}

// CodexPresetCarrilDefaults returns a defensive copy of the selected preset's
// carril defaults. The string boundary preserves compatibility with persisted
// state; unknown keys intentionally fall back to Recommended.
func CodexPresetCarrilDefaults(preset string) map[string]CodexCarrilDefault {
	defaults, ok := codexPresetMatrix[CodexPresetKey(preset)]
	if !ok {
		defaults = codexPresetMatrix[CodexPresetRecommended]
	}
	out := make(map[string]CodexCarrilDefault, len(defaults))
	for carril, value := range defaults {
		out[carril] = value
	}
	return out
}

// CodexCarrilModelsForPreset returns the model portion of a preset's carril
// defaults. Unknown persisted keys inherit the Recommended fallback policy.
func CodexCarrilModelsForPreset(preset string) map[string]string {
	defaults := CodexPresetCarrilDefaults(preset)
	out := make(map[string]string, len(defaults))
	for carril, value := range defaults {
		out[carril] = value.Model
	}
	return out
}

// MigrateLegacyCodexCarrilDefaults replaces the exact historical implicit
// default tuple with the current Recommended models. Every other persisted map
// is custom and is returned unchanged as a defensive copy.
func MigrateLegacyCodexCarrilDefaults(assignments map[string]string) map[string]string {
	if len(assignments) == 3 &&
		assignments["sdd-strong"] == "gpt-5.5" &&
		assignments["sdd-mid"] == "gpt-5.5" &&
		assignments["sdd-cheap"] == "gpt-5.4-mini" {
		return CodexCarrilModelsForPreset(string(CodexPresetRecommended))
	}

	out := make(map[string]string, len(assignments))
	for carril, modelID := range assignments {
		out[carril] = modelID
	}
	return out
}

func codexPresetEfforts(preset string) map[string]CodexEffort {
	defaults := CodexPresetCarrilDefaults(preset)
	out := make(map[string]CodexEffort, 13)
	for _, tier := range codexTierGroups {
		effort := defaults[tier.Profile].Effort
		for _, phase := range tier.Phases {
			out[phase] = effort
		}
	}
	return out
}

// CodexModelPresetRecommended returns the Recommended preset.
func CodexModelPresetRecommended() map[string]CodexEffort {
	return codexPresetEfforts(string(CodexPresetRecommended))
}

// CodexModelPresetPowerful returns the Powerful preset.
func CodexModelPresetPowerful() map[string]CodexEffort {
	return codexPresetEfforts(string(CodexPresetPowerful))
}

// CodexModelPresetLowCost returns the Low-cost preset.
func CodexModelPresetLowCost() map[string]CodexEffort {
	return codexPresetEfforts(string(CodexPresetLowCost))
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
// These efforts are Gentle AI workload policy, not Codex defaults.
//
//	Carril      LowCost  Recommended  Powerful
//	sdd-strong  medium   medium       high
//	sdd-mid     medium         medium             high
//	sdd-cheap   low            low                low
var codexTierGroups = []CodexTierGroup{
	{
		Profile:       "sdd-strong",
		Model:         codexPresetMatrix[CodexPresetRecommended]["sdd-strong"].Model,
		DefaultEffort: codexPresetMatrix[CodexPresetRecommended]["sdd-strong"].Effort,
		Phases:        []string{"sdd-propose", "sdd-design", "sdd-verify", "jd-judge-a", "jd-judge-b", "default"},
	},
	{
		Profile:       "sdd-mid",
		Model:         codexPresetMatrix[CodexPresetRecommended]["sdd-mid"].Model,
		DefaultEffort: codexPresetMatrix[CodexPresetRecommended]["sdd-mid"].Effort,
		Phases:        []string{"sdd-apply", "jd-fix-agent"},
	},
	{
		Profile:       "sdd-cheap",
		Model:         codexPresetMatrix[CodexPresetRecommended]["sdd-cheap"].Model,
		DefaultEffort: codexPresetMatrix[CodexPresetRecommended]["sdd-cheap"].Effort,
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
	return codexPresetMatrix[CodexPresetRecommended]["sdd-strong"].Model // ultimate fallback
}

// RenderCodexPhaseEffortsByPhase renders a per-phase Markdown table for the
// Codex sdd-orchestrator.md asset when Custom per-phase model assignments are
// active. Each row shows: phase | model | reasoning_effort.
//
// phaseModels maps phase names to custom model IDs. Phases not present in
// phaseModels fall back to carrilModels, preserving the selected or explicitly
// saved carril assignments. efforts maps phase names to CodexEffort values
// (typically from a preset + user overrides). When efforts is nil,
// CodexModelPresetRecommended is used. When carrilModels is nil, the canonical
// Recommended carril models are used.
//
// The output is deterministic: phases are always rendered in codexPhaseOrder.
func RenderCodexPhaseEffortsByPhase(phaseModels map[string]string, efforts map[string]CodexEffort, carrilModels map[string]string) string {
	if len(efforts) == 0 {
		efforts = CodexModelPresetRecommended()
	}
	if len(carrilModels) == 0 {
		carrilModels = DefaultCarrilModels()
	}

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
