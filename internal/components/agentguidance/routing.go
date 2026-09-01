// Package agentguidance owns the always-installed organic routing guidance
// projected into every configured agent. Routing is unconditional: it must not
// depend on the optional SDD component, on SDD mode, or on any SDD asset being
// present, because an agent without routing guidance has no way to choose
// between direct, delegated, and proposed work.
package agentguidance

import (
	"errors"
	"fmt"
	"strings"

	"github.com/gentleman-programming/gentle-ai/v2/internal/agents/capabilitymanifest"
	"github.com/gentleman-programming/gentle-ai/v2/internal/model"
)

// ErrUnknownRoutingPolicy fails closed when the canonical manifest carries a
// selection policy this renderer was not written for. Rendering stale prose
// over a changed policy would silently misinform every installed agent.
var ErrUnknownRoutingPolicy = errors.New("unknown sdd selection policy")

// RenderRouting projects the canonical implementation-routing facts of one
// supported adapter into its guidance block.
//
// The rendered block deliberately carries routing semantics only. It states no
// runtime observation, issues no lifecycle authority, and activates no remote
// mechanism, so an offline agent reading it can still route work correctly.
func RenderRouting(agent model.AgentID) (string, error) {
	manifest, err := capabilitymanifest.ForAgent(agent)
	if err != nil {
		return "", fmt.Errorf("render routing guidance for %q: %w", agent, err)
	}

	routing := manifest.ImplementationRouting
	if routing.SDD.SelectionPolicy != capabilitymanifest.SDDSelectionExplicitRequestOrAcceptedProposal {
		return "", fmt.Errorf("%w: %q", ErrUnknownRoutingPolicy, routing.SDD.SelectionPolicy)
	}

	var output strings.Builder
	output.WriteString("## Implementation Routing\n\n")
	output.WriteString("First establish whether the requested outcome explicitly authorizes a change. Investigation, explanation, review, audit, comparison, and solution-proposal or planning-only requests are read-only unless the user explicitly requests implementation or another mutation.\n")
	output.WriteString("- Read-only work may inspect, explain, compare, and recommend, but must not write or edit files, delegate a writer, invoke apply, or create implementation artifacts.\n")
	output.WriteString("- If change intent is ambiguous or conditional, ask one clarification and remain read-only until answered.\n\n")
	output.WriteString("After explicit change intent is established, route work for the requested outcome with the smallest useful topology. Every authorized change takes exactly one implementation route: direct inline, delegated direct, or optional SDD.\n\n")

	_, _ = fmt.Fprintf(
		&output,
		"- **Direct inline:** decide or verify from %d–%d files inline. Keep one mechanical, already-understood file change inline only when it needs no research and has no unresolved design decision.\n",
		routing.DirectInline.MinUnderstandingFiles,
		routing.DirectInline.MaxUnderstandingFiles,
	)
	_, _ = fmt.Fprintf(
		&output,
		"- **Delegated direct:** delegate one narrow exploration when understanding needs %d+ files; delegate one writer for %d+ non-trivial files. Reading that prepares a write and broad research also delegate.\n",
		routing.DelegatedDirect.MappingMinUnderstandingFiles,
		routing.DelegatedDirect.WriterMinNonTrivialFiles,
	)
	output.WriteString("- **Optional SDD:** propose SDD only when durable proposal, spec, design, and tasks would materially reduce substantial ambiguity. SDD is selected only by an explicit request or an accepted proposal.\n")
	output.WriteString("- File count, changed lines, size, or perceived risk alone never selects SDD and never forces a heavier route.\n")
	output.WriteString("- Automatic SDD pace is not mutation authorization; once implementation is explicitly authorized, it continues under the selected route.\n")
	output.WriteString("- These are implementation routes, not a ban on per-action delegation. Tests, builds, installs, and review actors may still use fresh workers without changing the selected route.\n")
	output.WriteString("- Direct and delegated work never create SDD artifacts, prompts, phase attempts, or synthetic SDD runs.\n")

	// The kill switch ships in the routing block, not in the optional SDD
	// assets, for the same reason routing itself is unconditional: it is
	// installed for every configured agent. A switch the agent cannot name does
	// not exist for the user, who would otherwise ask to stop using
	// receipt-driven development and be argued with instead of obeyed.
	output.WriteString("\n### Receipt-driven development is user-owned\n\n")
	output.WriteString("The user controls receipt-driven development with a switch: `gentle-ai review mode enable|disable|status`.\n\n")
	output.WriteString("- It is **opt-in and off by default**. Until the user explicitly enables it, reviews do not run and delivery follows ordinary repository policy. Do not treat that as a fault to diagnose or work around.\n")
	output.WriteString("- `status` is read-only. It reports the deciding source and the effective mode, and changes nothing. A `default` deciding source means nobody has chosen, so the effective mode is off.\n")
	output.WriteString("- When the user asks to stop using receipt-driven development, run `disable`. Do not argue, do not work around it, and do not propose alternatives first.\n")
	output.WriteString("- While it is disabled, keep implementing organically through direct inline, delegated direct, or optional SDD: do not start reviews, do not retry, do not reactivate it, and do not fall back to any retired path.\n")
	output.WriteString("- Delivery under a disabled switch follows ordinary repository policy and reports `disabled/unmanaged`, never a fabricated approval.\n")
	output.WriteString("- Never enable receipt-driven development on the user's behalf unless the user explicitly asks for it.\n")

	return output.String(), nil
}
