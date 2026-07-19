package sdd

import (
	"fmt"
	"strings"

	"github.com/gentleman-programming/gentle-ai/v2/internal/model"
)

// RenderTriggerRules renders a TriggerRuleSet as a short, scannable Markdown
// block. The output is marker-free — the caller wraps it via InjectMarkdownSection.
//
// Output format:
//   - Fixed header framing review as one explicit post-implementation operation
//   - Deterministic receipt outcomes for later lifecycle gates
//   - One bullet per binding in declaration order
//
// The function is pure: no I/O, no globals mutated, no goroutines.
func RenderTriggerRules(set model.TriggerRuleSet) string {
	var sb strings.Builder

	sb.WriteString("## Agent Trigger Rules\n\n")
	sb.WriteString("Deterministic bounded-review lifecycle router; apply it as a decision procedure, not advice. ")
	sb.WriteString("Post-apply starts `review/start(target)` only when no valid receipt exists. ")
	sb.WriteString("Pre-commit, pre-push, and pre-PR validate the same content-bound receipt and never create a new review budget or silently start Judgment Day. ")
	sb.WriteString("Release from protected `main` may bypass receipt validation only when the tag targets the current immutable `origin/main` SHA, required CI for that exact SHA is successful, the remote head is rechecked before tag push, and no fresh risk evidence exists; otherwise fail closed through native receipt validation. Major and post-incident releases require explicit extraordinary review.\n\n")
	sb.WriteString("Receipt action table: missing → start explicitly after implementation/post-apply; scope-changed → require explicit maintainer action; invalidated → require explicit maintainer action; escalated → stop. New CI, vulnerability, base, policy, provenance, or release evidence may invalidate/escalate without reopening unchanged code review.\n\n")
	sb.WriteString("Inside explicit `review/start(target)` only, select initial lenses by deterministic risk: **Low** (only documentation, comments, formatting, or typo-only string edits; zero executable-code and configuration changes) → no lens; **Medium** (every remaining change; pure human documentation above 400 authored changed lines is pinned to `review-readability`) → exactly ONE dominant-risk lens; **High** (security/auth/update/payments, data loss or exposure, permission changes, shell/process integration, or more than 400 authored changed lines in code, configuration, prompts, agent rules, workflows, runtime instruction docs, mixed content, or active content) → four initial 4R lens sweeps. Generated goldens are excluded from the authored threshold but remain in snapshot identity. Model, provider, profile, and reasoning effort are never classifier inputs.\n\n")
	sb.WriteString("Risk table: Clear naming, structure, maintainability, or small refactors → `review-readability`; ")
	sb.WriteString("Behavior, state, tests, determinism, or regressions → `review-reliability`; ")
	sb.WriteString("Shell/process integration, partial failures, recovery, or degraded dependencies → `review-resilience`; ")
	sb.WriteString("Security, permissions, data exposure/loss, architecture, or dependencies → `review-risk`.\n\n")

	for _, b := range set.Bindings {
		whenPhrase := renderWhen(b.When)
		directive := renderDirective(b)

		line := fmt.Sprintf("- At **%s**, %s: %s.", b.On, whenPhrase, directive)
		if b.Mode != model.ModeAdvisory && isFull4R(b.Run) && !b.When.Always {
			line = fmt.Sprintf("- At **%s**: %s.", b.On, directive)
		}
		if b.Reason != "" {
			line += fmt.Sprintf(" (%s)", b.Reason)
		}
		sb.WriteString(line)
		sb.WriteString("\n")
	}

	return sb.String()
}

// renderDirective converts a binding's Run list and Mode into the v2 triage
// directive for that event.
//
//   - advisory + a single review lens: the everyday router — trivial diffs run
//     no lens, everything else runs exactly ONE lens (the bound lens is the
//     default row), and the full 4R fan-out is prohibited at that event.
//   - advisory + anything else: trivial diffs are exempt, otherwise run the
//     bound agents.
//   - strong + the full 4R set under a condition: still diff triage, so the
//     trivial exemption applies; the fan-out fires when the condition matches;
//     a standard diff falls back to exactly ONE lens.
//   - strong otherwise (phase-triggered agents such as judgment-day, not diff
//     triage): run the bound agents whenever the condition matches, with no
//     trivial exemption.
func renderDirective(b model.TriggerBinding) string {
	agents := renderAgents(b.Run)
	if len(b.Run) == 1 {
		switch b.Run[0] {
		case "review-receipt-validator":
			return "validate the existing content-bound receipt with native `gentle-ai review validate --gate <gate>`; never start a reviewer or reset its budget"
		case "review-start":
			return "if no valid receipt exists, explicitly run `review/start(target)`; otherwise reuse the receipt"
		}
	}

	if b.Mode == model.ModeAdvisory {
		if len(b.Run) == 1 && isReviewLens(b.Run[0]) {
			return fmt.Sprintf(
				"trivial diff → no lens; otherwise run exactly ONE lens selected by the risk table (default %s); never the full 4R fan-out here",
				agents,
			)
		}
		return fmt.Sprintf("trivial diff → no lens; otherwise run %s", agents)
	}

	// ModeStrong (and any unrecognized mode) renders as a direct directive.
	if isFull4R(b.Run) && !b.When.Always {
		condition := strings.TrimPrefix(renderWhen(b.When), "when ")
		condition = strings.ReplaceAll(condition, " OR when ", " OR ")
		condition = strings.ReplaceAll(condition, " AND when ", " AND ")
		return fmt.Sprintf("trivial diff → no lens; else if %s, run %s using the adapter's execution mode (parallel with dedicated agents; sequential inline); else run exactly ONE lens selected by the risk table", condition, renderAgentList(b.Run))
	}
	return fmt.Sprintf("run %s", agents)
}

// isReviewLens reports whether agent is one of the four 4R review lenses.
func isReviewLens(agent string) bool {
	switch agent {
	case "review-risk", "review-readability", "review-reliability", "review-resilience":
		return true
	}
	return false
}

// isFull4R reports whether run contains all four 4R review lenses.
func isFull4R(run []string) bool {
	found := map[string]bool{}
	for _, r := range run {
		if isReviewLens(r) {
			found[r] = true
		}
	}
	return len(found) == 4
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

// renderAgents formats the list of agent names for a binding.
func renderAgents(run []string) string {
	agents := renderAgentList(run)
	if len(run) > 1 {
		return agents + " in parallel"
	}
	return agents
}

func renderAgentList(run []string) string {
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
	return strings.Join(rest, ", ") + ", and " + last
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
