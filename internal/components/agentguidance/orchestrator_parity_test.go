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

// Exercises the real installation carrier for every non-Pi runtime, then
// syncs it again. Pi routing is covered by the renderer; its orchestrator is
// owned by Gentle Shell and must not be copied here.
func TestInstalledEvidenceBudgetGuidance(t *testing.T) {
	for _, agent := range orchestratorRuntimes(t) {
		t.Run(string(agent), func(t *testing.T) {
			root := t.TempDir()
			first, err := InjectRoutingWithOptions(root, agent, RoutingOptions{})
			if err != nil {
				t.Fatal(err)
			}
			prompt := deliveredGuidance(t, first.Files[0])
			for _, want := range []string{
				"one parallel batch", "at most 3 calls", "approximately 10k tokens",
				"bounded search/line ranges", "not whole large files",
				"more than approximately 5 sequential lookups", "long-session mapping",
				"one read-only explorer", "at most approximately 2k tokens", "path:line",
				"one parent spot check", "Do not reread the entire mapped evidence",
				"counts, --stat, tail, or summaries", "Delegate full suites and builds",
				"approximately 150k", "not mechanically observed or enforced",
				"2+ non-trivial files", "reading that prepares a write", "broad research",
				"one mechanical, already-understood file", "no research", "unresolved design",
			} {
				if !strings.Contains(prompt, want) {
					t.Errorf("installed guidance missing %q", want)
				}
			}
			for _, stale := range []string{"1–3 files", "1-3 files", "4+ files", "4-file rule", "20 tool calls", "5 exploratory reads", "2 non-mechanical edits"} {
				if strings.Contains(prompt, stale) {
					t.Errorf("installed guidance retains %q", stale)
				}
			}
			second, err := InjectRoutingWithOptions(root, agent, RoutingOptions{})
			if err != nil {
				t.Fatal(err)
			}
			if got := deliveredGuidance(t, second.Files[0]); got != prompt {
				t.Error("identical sync changed guidance")
			}
		})
	}
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
