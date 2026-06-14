package sdd

import (
	"fmt"
	"strings"

	"github.com/gentleman-programming/gentle-ai/internal/model"
)

// RenderTriggerRules renders a TriggerRuleSet as a short, scannable Markdown
// block. The output is marker-free — the caller wraps it via InjectMarkdownSection.
//
// Output format:
//   - Fixed header + organic-not-a-gate note
//   - One bullet per binding in declaration order
//
// The function is pure: no I/O, no globals mutated, no goroutines.
func RenderTriggerRules(set model.TriggerRuleSet) string {
	var sb strings.Builder

	sb.WriteString("## Agent Trigger Rules\n\n")
	sb.WriteString("These are organic recommendations, not enforced checkpoints. ")
	sb.WriteString("gentle-ai only renders this text; the AI orchestrator decides when to act on it.\n\n")

	for _, b := range set.Bindings {
		whenPhrase := renderWhen(b.When)
		modePhrase := renderMode(b.Mode)
		agentsPhrase := renderAgents(b.Run)

		line := fmt.Sprintf("- At **%s**, %s, %s %s.", b.On, whenPhrase, modePhrase, agentsPhrase)
		if b.Reason != "" {
			line += fmt.Sprintf(" (%s)", b.Reason)
		}
		sb.WriteString(line)
		sb.WriteString("\n")
	}

	return sb.String()
}

// renderWhen converts a TriggerWhen condition into a natural-language phrase.
func renderWhen(w model.TriggerWhen) string {
	if w.Always {
		return "always"
	}

	var parts []string

	if len(w.Phases) > 0 {
		phaseList := joinPhases(w.Phases)
		return fmt.Sprintf("after the %s phase completes", phaseList)
	}

	if len(w.PathGlobs) > 0 {
		quoted := make([]string, len(w.PathGlobs))
		for i, g := range w.PathGlobs {
			quoted[i] = "`" + g + "`"
		}
		parts = append(parts, "when the diff touches "+strings.Join(quoted, ", "))
	}

	if w.MinDiffLines > 0 {
		parts = append(parts, fmt.Sprintf("when the diff exceeds %d changed lines", w.MinDiffLines))
	}

	if len(parts) == 0 {
		return "when conditions are met"
	}

	combinator := "OR"
	if w.Combine == "and" {
		combinator = "AND"
	}

	return strings.Join(parts, " "+combinator+" ")
}

// renderMode returns the mode wording for the binding.
func renderMode(mode model.TriggerMode) string {
	switch mode {
	case model.ModeStrong:
		return "**strongly recommend** running"
	default: // ModeAdvisory
		return "consider running"
	}
}

// renderAgents formats the list of agent names for a binding.
func renderAgents(run []string) string {
	if len(run) == 0 {
		return "(no agents)"
	}
	if len(run) == 1 {
		return fmt.Sprintf("`%s`", run[0])
	}
	quoted := make([]string, len(run))
	for i, a := range run {
		quoted[i] = "`" + a + "`"
	}
	last := quoted[len(quoted)-1]
	rest := quoted[:len(quoted)-1]
	return strings.Join(rest, ", ") + ", and " + last + " in parallel"
}

// joinPhases joins phase names with "or" for the when-phrase.
func joinPhases(phases []string) string {
	if len(phases) == 0 {
		return ""
	}
	if len(phases) == 1 {
		return phases[0]
	}
	last := phases[len(phases)-1]
	rest := phases[:len(phases)-1]
	return strings.Join(rest, ", ") + " or " + last
}
