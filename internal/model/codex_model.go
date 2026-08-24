package model

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"time"
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

var codexModelDiscoveryTimeout = 3 * time.Second

const codexModelDiscoveryOutputLimit = 1 << 20

var codexLookPath = exec.LookPath
var codexCommand = exec.CommandContext

type codexDiscoveryOutput struct {
	data     strings.Builder
	limit    int
	overflow bool
}

func (w *codexDiscoveryOutput) Write(p []byte) (int, error) {
	remaining := w.limit - w.data.Len()
	if remaining <= 0 {
		w.overflow = true
		return 0, io.ErrShortWrite
	}
	if len(p) > remaining {
		_, _ = w.data.Write(p[:remaining])
		w.overflow = true
		return remaining, io.ErrShortWrite
	}
	return w.data.Write(p)
}

type codexDiscoveredModelCatalog struct {
	Models []codexCatalogModel `json:"models"`
}

type codexCatalogModel struct {
	Slug           string `json:"slug"`
	Visibility     string `json:"visibility"`
	SupportedInAPI *bool  `json:"supported_in_api"`
}

// DiscoverCodexModels returns selectable models from the locally installed Codex
// CLI. It falls back to the curated catalog if Codex is unavailable, times out,
// returns invalid JSON, or reports no selectable models.
func DiscoverCodexModels(ctx context.Context) []string {
	discoveryCtx, cancel := context.WithTimeout(ctx, codexModelDiscoveryTimeout)
	defer cancel()

	codexPath, err := codexLookPath("codex")
	if err != nil {
		return CodexAvailableModels()
	}
	cmd := codexCommand(discoveryCtx, codexPath, "debug", "models")
	output := &codexDiscoveryOutput{limit: codexModelDiscoveryOutputLimit}
	cmd.Stdout = output
	if err := cmd.Run(); err != nil || output.overflow {
		return CodexAvailableModels()
	}

	var catalog codexDiscoveredModelCatalog
	if err := json.Unmarshal([]byte(output.data.String()), &catalog); err != nil {
		return CodexAvailableModels()
	}

	models := make([]string, 0, len(catalog.Models))
	seen := make(map[string]struct{}, len(catalog.Models))
	for _, entry := range catalog.Models {
		slug := strings.TrimSpace(entry.Slug)
		if slug == "" || (entry.Visibility != "" && entry.Visibility != "list") || (entry.SupportedInAPI != nil && !*entry.SupportedInAPI) {
			continue
		}
		if _, ok := seen[slug]; ok {
			continue
		}
		seen[slug] = struct{}{}
		models = append(models, slug)
	}
	if len(models) == 0 {
		return CodexAvailableModels()
	}
	return models
}

// FilterCodexModelList returns the subset of models whose ID contains query as a
// case-insensitive substring. An empty query returns a copy of models.
func FilterCodexModelList(models []string, query string) []string {
	if strings.TrimSpace(query) == "" {
		return append([]string(nil), models...)
	}
	q := strings.ToLower(query)
	out := make([]string, 0, len(models))
	for _, m := range models {
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
		"sdd-strong": {Model: "gpt-5.6-sol", Effort: CodexEffortMedium},
		"sdd-mid":    {Model: "gpt-5.6-terra", Effort: CodexEffortMedium},
		"sdd-cheap":  {Model: "gpt-5.6-luna", Effort: CodexEffortHigh},
	},
	CodexPresetRecommended: {
		"sdd-strong": {Model: "gpt-5.6-sol", Effort: CodexEffortMedium},
		"sdd-mid":    {Model: "gpt-5.6-terra", Effort: CodexEffortHigh},
		"sdd-cheap":  {Model: "gpt-5.6-luna", Effort: CodexEffortHigh},
	},
	CodexPresetPowerful: {
		"sdd-strong": {Model: "gpt-5.6-sol", Effort: CodexEffortXHigh},
		"sdd-mid":    {Model: "gpt-5.6-sol", Effort: CodexEffortHigh},
		"sdd-cheap":  {Model: "gpt-5.6-luna", Effort: CodexEffortHigh},
	},
}

// codexPresetOrchestrator is the main-session model per preset. It is no
// longer one shared policy: the low-cost preset runs the orchestrator on
// Terra, because a Plus plan cannot afford Sol in both the main session and
// every strong lane, and the strong lanes are where reasoning actually pays.
// Unknown keys fall back to Recommended, as the carril matrix does.
var codexPresetOrchestrator = map[CodexPresetKey]CodexOrchestratorAssignment{
	CodexPresetLowCost:     {Model: "gpt-5.6-terra", Effort: CodexEffortMedium},
	CodexPresetRecommended: {Model: "gpt-5.6-sol", Effort: CodexEffortMedium},
	CodexPresetPowerful:    {Model: "gpt-5.6-sol", Effort: CodexEffortMedium},
}

// CodexOrchestratorAssignment is the explicit top-level Codex session model
// selected by a Gentle AI preset. It is separate from delegated SDD carriles.
type CodexOrchestratorAssignment struct {
	Model  string
	Effort CodexEffort
}

// CodexPresetOrchestratorAssignment returns the main-session policy for a
// named preset. Every preset runs the orchestrator at medium effort: it plans,
// routes and adjudicates rather than doing the delegated work, so low effort
// made it the weakest link in the chain while medium keeps it responsive and
// still routes correctly. The model does vary — see codexPresetOrchestrator.
// Unknown keys intentionally fall back to Recommended.
func CodexPresetOrchestratorAssignment(preset string) *CodexOrchestratorAssignment {
	assignment, ok := codexPresetOrchestrator[CodexPresetKey(preset)]
	if !ok {
		assignment = codexPresetOrchestrator[CodexPresetRecommended]
	}
	return &assignment
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
	out := make(map[string]CodexEffort, 14)
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
// Phase groupings (Approach C — orthogonal carril axis). Sol reasons, Terra
// writes, Luna transcribes:
//   - sdd-strong (Razonamiento): explore, propose, design, verify, judge-a, judge-b, default
//   - sdd-mid    (Código):       apply, fix-agent
//   - sdd-cheap  (Liviano):      spec, tasks, archive, onboard
//
// codexTierGroups below is the single source of this grouping; the rendered
// table derives its phase column from it via codexTierPhaseLabel.
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
//	sdd-strong  medium   medium       xhigh
//	sdd-mid     medium   high         high
//	sdd-cheap   high     high         high
var codexTierGroups = []CodexTierGroup{
	{
		Profile:       "sdd-strong",
		Model:         codexPresetMatrix[CodexPresetRecommended]["sdd-strong"].Model,
		DefaultEffort: codexPresetMatrix[CodexPresetRecommended]["sdd-strong"].Effort,
		Phases:        []string{"sdd-explore", "sdd-research", "sdd-propose", "sdd-design", "sdd-verify", "jd-judge-a", "jd-judge-b", "default"},
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
		Phases:        []string{"sdd-spec", "sdd-tasks", "sdd-archive", "sdd-onboard"},
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

// codexTierPhaseLabel renders the human-readable "SDD phases" cell for one
// carril row directly from that carril's Phases. It exists so the rendered
// table cannot drift from codexTierGroups: the grouping has exactly one
// source, and moving a phase between carriles updates the table for free.
//
// Three presentation rules shape the label. Runtime prefixes (sdd-, jd-) are
// dropped because the column is already scoped to phases. The two Judgment Day
// judges collapse into a single "judge" entry, since a reader picking a profile
// does not care that there are two blind judges. "default" is omitted because
// it is the fallback binding for anything unlisted, not an SDD phase.
func codexTierPhaseLabel(tier CodexTierGroup) string {
	labels := make([]string, 0, len(tier.Phases))
	seen := make(map[string]bool, len(tier.Phases))
	for _, phase := range tier.Phases {
		if phase == "default" {
			continue
		}
		name := strings.TrimPrefix(strings.TrimPrefix(phase, "sdd-"), "jd-")
		if name == "judge-a" || name == "judge-b" {
			name = "judge"
		}
		if seen[name] {
			continue
		}
		seen[name] = true
		labels = append(labels, name)
	}
	return strings.Join(labels, ", ")
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

	var sb strings.Builder
	sb.WriteString("| Profile (CLI) | Model | `reasoning_effort` (spawn_agent) | SDD phases |\n")
	sb.WriteString("|---------------|-------|----------------------------------|------------|\n")

	for _, tier := range codexTierGroups {
		effort := maxEffort(assignments, tier.Phases)
		phases := codexTierPhaseLabel(tier)
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
	"sdd-explore", "sdd-research", "sdd-propose", "sdd-spec", "sdd-design", "sdd-tasks",
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
