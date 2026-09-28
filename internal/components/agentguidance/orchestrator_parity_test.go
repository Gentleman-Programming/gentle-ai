package agentguidance

import (
	"strings"
	"testing"
)

// gentleShellParityHeadings lists every orchestrator section each non-Pi
// runtime must install exactly once. It mirrors the runtime-agnostic contract
// of Gentle Shell's orchestrator prompt (gentle-pi `assets/orchestrator.md`,
// `assets/orchestrator-delegation.md`, `assets/orchestrator-memory.md`, and
// `assets/orchestrator-skills.md` at 89b8de3b5). When Gentle Shell adds a
// runtime-agnostic section, port it into
// `internal/assets/skills/_shared/odd-orchestrator-sections.md` and add its
// heading here deliberately.
//
// The list carries ODD and orchestration sections only: receipt-driven
// development applies to a subset of runtimes, so RDD sections (Provider Defect
// Handoff, the user-owned switch, and the review lifecycle) are asserted where
// RDD ships (rdd_gating_test.go), not here.
//
// Deliberately absent: Organic feature continuity and the memory lifecycle
// rule (owned by the routing block's ODD protocol and the Engram protocol),
// Gentle AI RDD ownership (RDD-specific), Judgment Day dispatch (owned by the
// judgment-day skill), and every Pi-only binding (phase signaling, subagent
// model routing, background policy, and runtime overlays).
var gentleShellParityHeadings = []string{
	// Orchestrator core restored from v3.7.0.
	"Lossless Blocking Prompts",
	"Language Domain Contract",
	"Delegation Rules",
	// assets/orchestrator.md
	"Identity Contract",
	"Core Role",
	"Mental Model",
	"Safety",
	// assets/orchestrator-delegation.md
	"Work Routing Ladder",
	"Canonical Lightweight Workflows",
	"Allowed edit surfaces",
	"Key Learnings closing block",
	"Delivery strategy",
	// assets/orchestrator-skills.md
	"Intent-Driven Skill Discovery",
}

// gentleShellOnlyContent must never reach a non-Pi runtime: Pi tool names and
// the Judgment Day correction-batch contract.
var gentleShellOnlyContent = []string{
	"gentle_odd_phase",
	"subagent_run",
	"subagent_status",
	"subagent_result",
	"ask_user_choice",
	"gentle_review",
	"jd-fix-agent",
	"Judgment Day activation",
	"Judgment Day correction batch",
	"Exact authorized severe IDs",
	"Exact frozen finding rows",
	"Pi Subagent Model Routing",
	"Background Subagent Policy",
	"Pi Runtime Overlays",
}

func TestInstalledOrchestratorHasGentleShellParity(t *testing.T) {
	t.Parallel()

	for _, agent := range orchestratorRuntimes(t) {
		t.Run(string(agent), func(t *testing.T) {
			t.Parallel()

			result, err := InjectRoutingWithOptions(t.TempDir(), agent, RoutingOptions{})
			if err != nil {
				t.Fatalf("InjectRouting(%q) error = %v", agent, err)
			}
			prompt := deliveredGuidance(t, result.Files[0])

			for _, heading := range gentleShellParityHeadings {
				if got := headingCount(prompt, heading); got != 1 {
					t.Errorf("heading %q appears %d times, want 1", heading, got)
				}
			}

			// Every runtime already resolves skills under its own heading
			// (Sub-Agent Launch Pattern, Skill Resolver Protocol, Skill Loading
			// for Delegation); the shared Skill Registry Protocol fills the gap
			// only where none exists. The contract, not the heading, is required.
			if !strings.Contains(prompt, "paths-injected") {
				t.Error("installed prompt carries no skill resolution feedback contract")
			}

			for _, forbidden := range gentleShellOnlyContent {
				if strings.Contains(prompt, forbidden) {
					t.Errorf("installed prompt carries Gentle Shell-only content %q", forbidden)
				}
			}
		})
	}
}
