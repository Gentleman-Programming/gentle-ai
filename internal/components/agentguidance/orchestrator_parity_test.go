package agentguidance

import (
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v4/internal/model"
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
				"more than approximately 5 sequential lookups", "re-evaluate task size",
				"one read-only explorer", "at most approximately 2k tokens", "path:line",
				"one parent spot check", "Do not reread the entire mapped evidence",
				"counts, --stat, tail, or summaries", "delegate long suites and builds",
				"approximately 150k", "not mechanically observed or enforced",
				"delegate a writer only for a named reason", "reading that prepares a write", "broad research",
				"a small task's writes stay inline", "File count never fires this rule",
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

// gentle-shell#1731: every installed orchestrator delegates a writer only for a
// named reason, routes parallel writers through one rule, explores only to
// decide or route, and bounds verification timing and corrections.
func TestInstalledOrchestratorDelegatesForReason(t *testing.T) {
	t.Parallel()

	for _, agent := range orchestratorRuntimes(t) {
		t.Run(string(agent), func(t *testing.T) {
			t.Parallel()

			result, err := InjectRoutingWithOptions(t.TempDir(), agent, RoutingOptions{})
			if err != nil {
				t.Fatalf("InjectRouting(%q) error = %v", agent, err)
			}
			prompt := deliveredGuidance(t, result.Files[0])

			required := []string{
				// S1 writer by reason.
				"**Write rule**: a small task's writes stay inline, even across files; delegate a writer only for a named reason",
				"Never for size, a large task alone, file count, or a price ratio; without a reason the parent writes inline, following its logbook.",
				"Write a large (tracked) task with no Write rule reason",
				"Write a unit with a Write rule reason",
				"one bounded writer per unit",
				"implementing a unit with a writer reason (writer)",
				// S2 parallel writers.
				"parallel writers follow the **Parallel writers** rule under `## Implementation Routing`",
				"Parallel writers follow the **Parallel writers** rule under `## Implementation Routing`: another repository's work goes in a fresh worktree based on its main",
				// S5 explore only to decide or route.
				"never explore files you will read anyway before writing inline",
				// S8 verification timing and bounds.
				"**Verification timing**: when the same model wrote several deliveries of one feature inline, run one independent verifier at the feature's end",
				"verify per unit only when that unit is really high risk",
				"always when a smaller-model profile wrote the code",
				"**Correction bounds**: verifier blockers get one correction batch that fixes every reported blocker, then one recheck limited to those blockers",
				"A second correction runs only when the recheck shows the same blocker still failing; a new finding never earns one.",
				"A writer's self-review follows the same bound, then reports `partial`.",
				// S6 test discipline reaches every handoff.
				"the applicable test-first policy and runner from `## Implementation Routing`",
				// S7 verify handoff.
				"**Verify handoff**: give the verifier the whole feature document (every `S#`, never one task), the baseline commit, and the probe command forms",
				"a fresh scratch copy created with `mktemp -d` under the system temp dir, never inside the workspace, and leaves no new file in the workspace",
				"the data hash is unchanged after a rejected command, and prior commands' output is identical to the baseline",
				"silently ignoring an explicit option with success, or changing existing output nobody asked to change, is always a blocker",
				"The writer commits the probes as regression tests.",
			}
			if model.SupportsReceiptDrivenDevelopment(agent) {
				// Native RDD review allows exactly one correction attempt
				// (reviewtransaction.MaxCompactCorrectionAttempts); the
				// same-blocker second correction applies to the verifier only.
				required = append(required, "One immutable candidate permits at most one scoped correction; there is no loop-until-clean behavior.")
				if strings.Contains(prompt, "One immutable candidate permits at most one scoped correction, and a second") {
					t.Errorf("installed orchestrator for %q grants native RDD review a second correction", agent)
				}
			} else {
				required = append(required, "One verified change permits at most one scoped correction, and a second only when the recheck shows the same blocker still failing")
			}
			for _, want := range required {
				if !strings.Contains(prompt, want) {
					t.Errorf("installed orchestrator for %q is missing %q", agent, want)
				}
			}
			for _, retired := range []string{
				"a large task delegates one writer per task",
				"one writer per task",
				"Use a single writer thread",
				"Keep writes single-threaded",
				"Preserve one writer thread",
				"implementing a large tracked task",
				"configured TDD mode",
				"strict TDD is active",
				"writer implements authorized fixes",
				"COORDINATOR, not an executor",
				"not the default executor",
				"bounded workers execute.",
			} {
				if strings.Contains(prompt, retired) {
					t.Errorf("installed orchestrator for %q keeps retired %q", agent, retired)
				}
			}
		})
	}
}

// TestInstalledSmallGenericOrchestratorCarriesVerificationDiscipline pins the
// small-model generic variant, which replaces the capable sections and would
// otherwise ship without the verification gate.
func TestInstalledSmallGenericOrchestratorCarriesVerificationDiscipline(t *testing.T) {
	t.Parallel()

	result, err := InjectRoutingWithOptions(t.TempDir(), model.AgentVSCodeCopilot, RoutingOptions{OrchestratorCapability: "small"})
	if err != nil {
		t.Fatalf("InjectRouting(small) error = %v", err)
	}
	prompt := deliveredGuidance(t, result.Files[0])
	if !strings.Contains(prompt, "Orchestrator Instructions (Small Model)") {
		t.Fatal("small capability did not select the small-model variant")
	}
	for _, want := range []string{
		// S1 the parent works inline unless a named reason fires.
		"You work inline by default, following your logbook",
		// S6 test discipline.
		"Write one RED test per requested rule",
		"the applicable test-first policy and runner from `## Implementation Routing`",
		// S7 verify handoff.
		"**Verify handoff**",
		"mktemp -d",
		"never inside the workspace",
		"silently ignoring an explicit option",
		// S8 timing and bounds.
		"**Verification timing**: when the same model wrote several deliveries of one feature inline, run one independent verifier at the feature's end",
		"**Correction bounds**: verifier blockers get one correction batch that fixes every reported blocker, then one recheck limited to those blockers",
		"A second correction runs only when the recheck shows the same blocker still failing; a new finding never earns one.",
	} {
		if !strings.Contains(prompt, want) {
			t.Errorf("small generic orchestrator is missing %q", want)
		}
	}
	for _, retired := range []string{"COORDINATOR, not an executor", "configured TDD mode", "strict TDD is active"} {
		if strings.Contains(prompt, retired) {
			t.Errorf("small generic orchestrator keeps retired %q", retired)
		}
	}
}
