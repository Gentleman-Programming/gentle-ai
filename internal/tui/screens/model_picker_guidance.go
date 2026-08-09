package screens

import (
	"strings"

	"github.com/gentleman-programming/gentle-ai/v2/internal/tui/styles"
)

// ModelSelectionGuidance describes the capabilities, reasoning range, and
// tradeoffs a user should consider when assigning a model to a given SDD,
// Judgment Day, or review role. Guidance is capability-based — it never
// pins to specific model brands or transient model IDs, and reasoning-effort
// guidance is kept separate from Fast / service-tier guidance.
type ModelSelectionGuidance struct {
	// Purpose is a one-line summary of what the role does.
	Purpose string

	// Priorities is a newline-separated list of capabilities to prefer
	// when selecting a model for the role. Rendered as bullet points
	// when the expanded guidance panel is visible.
	Priorities string

	// Reasoning is the suggested reasoning-effort range. Reasoning is
	// separate from Fast mode — see FastMode below.
	Reasoning string

	// FastMode describes when to enable Fast / service-tier processing.
	// Empty means Fast mode is not recommended for the role. Fast mode
	// changes processing speed and pricing only; it does not replace
	// the chosen reasoning effort.
	FastMode string

	// Tradeoffs lists quality / speed / cost / context considerations.
	// Rendered only when the expanded guidance panel is visible.
	Tradeoffs string
}

// modelSelectionGuidance maps every configurable agent role to capability-
// based guidance. Roles not present in the map fall back to generic
// implementation- or review-agent guidance (see guidanceFor).
//
// The guidance describes what to look for in a model, not which model to
// pick, so the map stays valid as provider catalogs, pricing, and IDs
// change. The Fast / service-tier note is intentionally separate from
// the reasoning-effort note because the two settings control different
// dimensions of model behaviour.
var modelSelectionGuidance = map[string]ModelSelectionGuidance{
	SDDOrchestratorPhase: {
		Purpose:    "Coordinates agents, routes work, and maintains workflow state.",
		Priorities: "Reliable instruction following\nTool and workflow coordination\nBalanced latency and reasoning",
		Reasoning:  "Medium reasoning for normal orchestration.",
		FastMode:   "Use when many small interactions add up to perceptible latency.",
		Tradeoffs:  "Prefer stable JSON output; avoid expensive reasoning unless the workload demands it.",
	},
	"sdd-init": {
		Purpose:    "Initializes SDD context, capabilities cache, and persistence.",
		Priorities: "Reliable instruction following\nStable schema and config output",
		Reasoning:  "Low to medium reasoning.",
		Tradeoffs:  "Prefer cheaper models; boot is frequent and reasoning rarely helps.",
	},
	"sdd-explore": {
		Purpose:    "Explores the problem space before committing to a proposal.",
		Priorities: "Reasoning over ambiguous requirements\nMulti-source synthesis\nOutput that structures findings",
		Reasoning:  "Medium to high reasoning.",
		Tradeoffs:  "Quality of synthesis matters more than speed; sufficient context helps.",
	},
	"sdd-propose": {
		Purpose:    "Drafts the change proposal (intent, scope, approach).",
		Priorities: "Clarity of writing\nScope reasoning\nStructured proposal format",
		Reasoning:  "Medium reasoning.",
		Tradeoffs:  "Editorial quality matters; avoid hallucinating model IDs or external facts.",
	},
	"sdd-spec": {
		Purpose:    "Writes the delta spec: requirements, scenarios, acceptance criteria.",
		Priorities: "Precise requirements writing\nTestable scenarios\nGiven / When / Then discipline",
		Reasoning:  "Medium to high reasoning.",
		Tradeoffs:  "Spec correctness gates downstream work; long context helps related specs.",
	},
	"sdd-design": {
		Purpose:    "Writes the technical design and architecture approach.",
		Priorities: "Architectural reasoning\nTradeoff analysis\nDiagram and code-fence output",
		Reasoning:  "High reasoning.",
		Tradeoffs:  "Quality matters more than speed; strong context capacity helps multi-file designs.",
	},
	"sdd-tasks": {
		Purpose:    "Breaks the change into implementation tasks with review-budget forecasting.",
		Priorities: "Decomposition discipline\nWork-unit clarity\nRollback thinking",
		Reasoning:  "Medium reasoning.",
		Tradeoffs:  "Granularity matters — commits that read alone; avoid premature optimization.",
	},
	"sdd-apply": {
		Purpose:    "Implements approved tasks and modifies project files.",
		Priorities: "Strong coding and tool-use reliability\nSufficient context for specs, design, and tasks\nMedium or high reasoning for non-trivial changes",
		Reasoning:  "Medium or high reasoning for non-trivial changes.",
		FastMode:   "Use when lower latency justifies the additional cost.",
		Tradeoffs:  "Code correctness matters more than cleverness; do not fight the design.",
	},
	"sdd-verify": {
		Purpose:    "Executes tests and proves implementation matches specs, design, and tasks.",
		Priorities: "Test discipline\nEvidence-based pass / fail\nCoverage of acceptance criteria",
		Reasoning:  "Medium reasoning.",
		Tradeoffs:  "Honesty about regressions matters; do not declare done without evidence.",
	},
	"sdd-archive": {
		Purpose:    "Archives the change by syncing delta specs.",
		Priorities: "Stable archive writes\nNo accidental spec deletion\nCheap and fast",
		Reasoning:  "Low reasoning.",
		Tradeoffs:  "Use a cheap model — verification already passed; latency-sensitive final step.",
	},
	"sdd-onboard": {
		Purpose:    "Walks new users through the SDD workflow with skills.",
		Priorities: "Friendly explanation\nContext-rich examples\nStable rendering of skill content",
		Reasoning:  "Low to medium reasoning.",
		Tradeoffs:  "Latency-sensitive (first impression); clarity over depth.",
	},
	"jd-judge-a": {
		Purpose:    "Performs an independent adversarial review.",
		Priorities: "Strong reasoning\nDefect detection\nEvidence analysis\nIndependence from the implementation model",
		Reasoning:  "High reasoning effort.",
		Tradeoffs:  "Prefer review quality over generation speed; diversity from the implementation model improves catch rate.",
	},
	"jd-judge-b": {
		Purpose:    "Second independent adversarial reviewer (Judgment Day diversity).",
		Priorities: "Defect detection from a different angle\nReasoning robustness\nIndependence from judge-a",
		Reasoning:  "High reasoning effort.",
		Tradeoffs:  "Pick a model that disagrees with judge-a often; cheap is fine if quality is comparable.",
	},
	"jd-fix-agent": {
		Purpose:    "Applies judgment-day fixes after the review identifies defects.",
		Priorities: "Targeted code edits\nTest discipline\nMinimal-scope changes",
		Reasoning:  "Medium reasoning.",
		Tradeoffs:  "Do not expand scope beyond the defect; reuse the implementation model's strengths where possible.",
	},
	"review-risk": {
		Purpose:    "Native bounded-review lens: surfaces risk and integrity concerns.",
		Priorities: "Reasoning depth\nDefect detection\nSpecific, actionable findings",
		Reasoning:  "High reasoning effort.",
		Tradeoffs:  "Prefer slower, careful review over speed; cite evidence.",
	},
	"review-readability": {
		Purpose:    "Native bounded-review lens: surfaces readability and clarity issues.",
		Priorities: "Editorial discipline\nStyle consistency\nConstructive framing",
		Reasoning:  "Medium reasoning.",
		Tradeoffs:  "Be specific; avoid taste-based feedback without evidence.",
	},
	"review-reliability": {
		Purpose:    "Native bounded-review lens: surfaces reliability and correctness issues.",
		Priorities: "Edge-case reasoning\nConcurrency and error-path coverage\nTest-evidence analysis",
		Reasoning:  "High reasoning effort.",
		Tradeoffs:  "Quality of evidence matters more than volume; cite specific failure modes.",
	},
	"review-resilience": {
		Purpose:    "Native bounded-review lens: surfaces resilience and failure-handling issues.",
		Priorities: "Failure-mode reasoning\nOperational thinking\nRecovery and rollback paths",
		Reasoning:  "High reasoning effort.",
		Tradeoffs:  "Focus on realistic failure modes; avoid paranoid edge cases that do not occur in production.",
	},
	"review-refuter": {
		Purpose:    "Aggregates lens output and refutes weak claims before sign-off.",
		Priorities: "Strong reasoning\nIndependent judgement\nDefensible verdict",
		Reasoning:  "High reasoning effort.",
		Tradeoffs:  "Adversarial role — do not soften findings; cite evidence.",
	},
}

// guidanceFor returns the guidance entry for a given role. Unknown roles
// fall back to a generic guidance that describes the implementation /
// review responsibilities without claiming any specific SDD or JD phase.
func guidanceFor(role string) ModelSelectionGuidance {
	if g, ok := modelSelectionGuidance[role]; ok {
		return g
	}
	if strings.HasPrefix(role, "jd-") {
		return ModelSelectionGuidance{
			Purpose:    "Judgment Day sub-agent that participates in adversarial review.",
			Priorities: "Strong reasoning\nDefect detection\nIndependence from the implementation model",
			Reasoning:  "High reasoning effort.",
			Tradeoffs:  "Prefer review quality over generation speed.",
		}
	}
	if strings.HasPrefix(role, "review-") {
		return ModelSelectionGuidance{
			Purpose:    "Native bounded-review sub-agent.",
			Priorities: "Reasoning depth\nDefect detection\nSpecific, actionable findings",
			Reasoning:  "Medium to high reasoning effort.",
			Tradeoffs:  "Cite evidence; prefer careful review over speed.",
		}
	}
	return ModelSelectionGuidance{
		Purpose:    "SDD sub-agent that implements approved tasks.",
		Priorities: "Coding and tool-use reliability\nSufficient context\nMedium reasoning for non-trivial work",
		Reasoning:  "Medium reasoning.",
		FastMode:   "Use when lower latency justifies the additional cost.",
		Tradeoffs:  "Code correctness matters more than cleverness.",
	}
}

// renderCompactGuidance renders the single-line summary shown by default
// below the help line. The "i / ?" hint invites the user to expand the
// full guidance panel.
func renderCompactGuidance(role string) string {
	g := guidanceFor(role)
	if g.Purpose == "" {
		return ""
	}
	return styles.SubtextStyle.Render("Role: "+role+" — "+g.Purpose) +
		"\n" +
		styles.HelpStyle.Render("i or ?: show full guidance") +
		"\n"
}

// renderExpandedGuidance renders the multi-line capability panel. The
// panel always ends with a hint that the user can collapse it again.
func renderExpandedGuidance(role string) string {
	g := guidanceFor(role)
	if g.Purpose == "" {
		return renderCompactGuidance(role)
	}

	var b strings.Builder
	b.WriteString(styles.SubtextStyle.Render("Role: " + role))
	b.WriteString("\n")
	b.WriteString(styles.SelectedStyle.Render(g.Purpose))
	b.WriteString("\n\n")

	b.WriteString(styles.SubtextStyle.Render("Consider:"))
	b.WriteString("\n")
	for _, line := range strings.Split(g.Priorities, "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		b.WriteString(styles.SubtextStyle.Render("  • " + line))
		b.WriteString("\n")
	}

	if g.Reasoning != "" {
		b.WriteString("\n")
		b.WriteString(styles.SubtextStyle.Render("Reasoning: " + g.Reasoning))
		b.WriteString("\n")
	}

	if g.FastMode != "" {
		b.WriteString(styles.SubtextStyle.Render("Fast mode: " + g.FastMode))
		b.WriteString("\n")
		b.WriteString(styles.SubtextStyle.Render("Fast changes processing speed and pricing."))
		b.WriteString("\n")
		b.WriteString(styles.SubtextStyle.Render("It does not replace the selected reasoning effort."))
		b.WriteString("\n")
	}

	if g.Tradeoffs != "" {
		b.WriteString("\n")
		b.WriteString(styles.SubtextStyle.Render("Tradeoffs:"))
		b.WriteString("\n")
		for _, line := range strings.Split(g.Tradeoffs, "\n") {
			if strings.TrimSpace(line) == "" {
				continue
			}
			b.WriteString(styles.SubtextStyle.Render("  - " + line))
			b.WriteString("\n")
		}
	}

	b.WriteString("\n")
	b.WriteString(styles.HelpStyle.Render("i or ?: hide guidance"))
	b.WriteString("\n")
	return b.String()
}

// ToggleModelPickerGuidance flips the GuidanceExpanded flag when the user
// presses the i or ? key. Returns true if the key was handled so the
// caller can short-circuit normal navigation.
func ToggleModelPickerGuidance(state *ModelPickerState, key string) bool {
	if state == nil {
		return false
	}
	if key != "i" && key != "?" {
		return false
	}
	state.GuidanceExpanded = !state.GuidanceExpanded
	return true
}
