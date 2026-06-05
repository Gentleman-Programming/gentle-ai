package model

import (
	"fmt"
	"strings"
)

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

// codexPhaseOrder defines the canonical order of phases used in render output.
// These 13 keys match the claudePhases list (plus "default") from the TUI screens package.
var codexPhaseOrder = []string{
	"sdd-explore",
	"sdd-propose",
	"sdd-spec",
	"sdd-design",
	"sdd-tasks",
	"sdd-apply",
	"sdd-verify",
	"sdd-archive",
	"sdd-onboard",
	"jd-judge-a",
	"jd-judge-b",
	"jd-fix-agent",
	"default",
}

// CodexModelPresetRecommended returns the Recommended (ChatGPT Pro $100/mo) preset.
// Balanced effort — high for key SDD phases, low for lightweight ones.
func CodexModelPresetRecommended() map[string]CodexEffort {
	return map[string]CodexEffort{
		"sdd-explore":  CodexEffortLow,
		"sdd-propose":  CodexEffortHigh,
		"sdd-spec":     CodexEffortMedium,
		"sdd-design":   CodexEffortHigh,
		"sdd-tasks":    CodexEffortHigh,
		"sdd-apply":    CodexEffortHigh,
		"sdd-verify":   CodexEffortHigh,
		"sdd-archive":  CodexEffortLow,
		"sdd-onboard":  CodexEffortLow,
		"jd-judge-a":   CodexEffortHigh,
		"jd-judge-b":   CodexEffortHigh,
		"jd-fix-agent": CodexEffortMedium,
		"default":      CodexEffortHigh,
	}
}

// CodexModelPresetPowerful returns the Powerful (ChatGPT Pro $200/mo) preset.
// xhigh effort for architecture-heavy and review-heavy phases.
func CodexModelPresetPowerful() map[string]CodexEffort {
	return map[string]CodexEffort{
		"sdd-explore":  CodexEffortMedium,
		"sdd-propose":  CodexEffortXHigh,
		"sdd-spec":     CodexEffortHigh,
		"sdd-design":   CodexEffortXHigh,
		"sdd-tasks":    CodexEffortHigh,
		"sdd-apply":    CodexEffortHigh,
		"sdd-verify":   CodexEffortXHigh,
		"sdd-archive":  CodexEffortLow,
		"sdd-onboard":  CodexEffortLow,
		"jd-judge-a":   CodexEffortXHigh,
		"jd-judge-b":   CodexEffortXHigh,
		"jd-fix-agent": CodexEffortHigh,
		"default":      CodexEffortHigh,
	}
}

// CodexModelPresetLowCost returns the Low-cost (ChatGPT Plus $20/mo) preset.
// Minimal effort to stay within tight Plus plan limits.
func CodexModelPresetLowCost() map[string]CodexEffort {
	return map[string]CodexEffort{
		"sdd-explore":  CodexEffortLow,
		"sdd-propose":  CodexEffortMedium,
		"sdd-spec":     CodexEffortLow,
		"sdd-design":   CodexEffortMedium,
		"sdd-tasks":    CodexEffortLow,
		"sdd-apply":    CodexEffortMedium,
		"sdd-verify":   CodexEffortMedium,
		"sdd-archive":  CodexEffortLow,
		"sdd-onboard":  CodexEffortLow,
		"jd-judge-a":   CodexEffortLow,
		"jd-judge-b":   CodexEffortLow,
		"jd-fix-agent": CodexEffortLow,
		"default":      CodexEffortMedium,
	}
}

// codexTierGroups defines the three CLI profile tiers and which phases they cover.
// The tier effort is derived by taking the maximum effort across phases in the group.
var codexTierGroups = []struct {
	profile string
	phases  []string
}{
	{
		profile: "sdd-strong",
		phases:  []string{"sdd-propose", "sdd-design", "sdd-verify", "jd-judge-a", "jd-judge-b"},
	},
	{
		profile: "sdd-mid",
		phases:  []string{"sdd-spec", "sdd-tasks", "sdd-apply"},
	},
	{
		profile: "sdd-cheap",
		phases:  []string{"sdd-explore", "sdd-archive", "sdd-onboard"},
	},
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
// sdd-orchestrator.md asset. The table maps CLI profile names to their
// reasoning_effort tier and covered SDD phases. The output is deterministic:
// tier groups are always rendered in codexTierGroups order.
//
// When assignments is nil or empty, falls back to CodexModelPresetRecommended.
func RenderCodexPhaseEfforts(assignments map[string]CodexEffort) string {
	if len(assignments) == 0 {
		assignments = CodexModelPresetRecommended()
	}

	tierPhaseLabels := map[string]string{
		"sdd-strong": "propose, design, verify, judge",
		"sdd-mid":    "spec, tasks, apply",
		"sdd-cheap":  "explore, archive, onboard",
	}

	var sb strings.Builder
	sb.WriteString("| Profile (CLI) | `reasoning_effort` (spawn_agent) | SDD phases |\n")
	sb.WriteString("|---------------|----------------------------------|------------|\n")

	for _, tier := range codexTierGroups {
		effort := maxEffort(assignments, tier.phases)
		phases := tierPhaseLabels[tier.profile]
		sb.WriteString(fmt.Sprintf("| `%s` | `%s` | %s |\n",
			tier.profile,
			effort,
			phases,
		))
	}

	return sb.String()
}
