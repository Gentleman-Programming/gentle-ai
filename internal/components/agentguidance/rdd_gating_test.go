package agentguidance

import (
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v4/internal/catalog"
	"github.com/gentleman-programming/gentle-ai/v4/internal/model"
)

// rddOnlyHeadings are the sections receipt-driven development owns. They reach
// the prompt of an RDD runtime exactly once and never any other runtime.
var rddOnlyHeadings = []string{
	"Gentle AI Provider Defect Handoff",
	"Native Compact Review Orchestration",
	"Receipt-driven development is user-owned",
}

// rddVocabulary is every phrase that would hand a non-RDD runtime a piece of
// the review lifecycle: its commands, consent envelope, receipts, lenses, the
// refuter, or an RDD on/off branch. Matching is case-insensitive, except for
// the acronyms, which are matched exactly so ordinary words never collide.
var rddVocabulary = []string{
	"Native Compact Review Orchestration",
	"Review Execution Contract",
	"Provider Defect Handoff",
	"receipt-driven development",
	"Receipt-driven development is user-owned",
	"gentle-ai review",
	"review assess",
	"review_due",
	"review-integration",
	"consent/v3",
	"receipt",
	"refuter",
	"lens",
	"native review",
}

var rddAcronyms = []string{"RDD", "RAR"}

// assertNoRDDContent fails for every RDD phrase content carries.
func assertNoRDDContent(t *testing.T, content string) {
	t.Helper()

	lower := strings.ToLower(content)
	for _, forbidden := range rddVocabulary {
		if strings.Contains(lower, strings.ToLower(forbidden)) {
			t.Errorf("non-RDD runtime carries RDD content %q", forbidden)
		}
	}
	for _, forbidden := range rddAcronyms {
		if strings.Contains(content, forbidden) {
			t.Errorf("non-RDD runtime carries RDD content %q", forbidden)
		}
	}
}

// oddContractClauses are ODD instructions every runtime keeps, RDD or not.
var oddContractClauses = []string{
	"### ODD protocol (MANDATORY, in this order, on every request)",
	"Use at most one scoped independent read-only assumption challenge for a high-consequence unproven premise",
	"Deterministic failures need fixes, not model debate.",
	"Organic Driven Development Is The Default Workflow",
	"Delegated Verification Gate",
	"Native Checking Contract",
	"Lossless Blocking Prompts",
	"Delegation Rules",
	"Mandatory Delegation Triggers",
	"Language Domain Contract",
	"Cost and Context Balance",
	"Delivery follows work units",
	"The parent spot check",
	"independent verifier",
}

func TestRDDRuntimeSetMatchesReviewTransportManifest(t *testing.T) {
	t.Parallel()

	for _, agent := range catalog.AllAgents() {
		if got, want := model.SupportsReceiptDrivenDevelopment(agent.ID), advertisesReviewTransport(t, agent.ID); got != want {
			t.Errorf("SupportsReceiptDrivenDevelopment(%q) = %v, but the capability manifest advertises review transport = %v", agent.ID, got, want)
		}
	}
}

func TestInstalledPromptCarriesRDDOnlyOnRDDRuntimes(t *testing.T) {
	t.Parallel()

	for _, agent := range orchestratorRuntimes(t) {
		t.Run(string(agent), func(t *testing.T) {
			t.Parallel()

			result, err := InjectRoutingWithOptions(t.TempDir(), agent, RoutingOptions{})
			if err != nil {
				t.Fatalf("InjectRouting(%q) error = %v", agent, err)
			}
			prompt := deliveredGuidance(t, result.Files[0])

			for _, clause := range oddContractClauses {
				if !strings.Contains(prompt, clause) {
					t.Errorf("installed prompt is missing ODD clause %q", clause)
				}
			}

			if model.SupportsReceiptDrivenDevelopment(agent) {
				for _, heading := range rddOnlyHeadings {
					if got := headingCount(prompt, heading); got != 1 {
						t.Errorf("RDD heading %q appears %d times, want 1", heading, got)
					}
				}
				for _, want := range []string{
					"The native RDD refuter owns native review claims; never duplicate or bypass it.",
					"gentle-ai review mode enable|disable|status",
					"gentle-ai review assess --cwd <repo> --json",
					"**RDD on**",
				} {
					if !strings.Contains(prompt, want) {
						t.Errorf("RDD runtime prompt is missing %q", want)
					}
				}
				return
			}

			assertNoRDDContent(t, prompt)
		})
	}
}

func TestRenderRoutingGatesRDDByRuntime(t *testing.T) {
	t.Parallel()

	for _, agent := range catalog.AllAgents() {
		t.Run(string(agent.ID), func(t *testing.T) {
			t.Parallel()

			rendered, err := RenderRouting(agent.ID)
			if err != nil {
				t.Fatalf("RenderRouting(%q) error = %v", agent.ID, err)
			}
			for _, want := range []string{
				"### ODD protocol (MANDATORY, in this order, on every request)",
				"Use at most one scoped independent read-only assumption challenge for a high-consequence unproven premise, even in a small security-critical change. Name the premise, evidence, and consequence; do not start a debate loop. Deterministic failures need fixes, not model debate.",
				"Run applicable functional checks per task",
				"Never skip an existing delivery gate.",
			} {
				if !strings.Contains(rendered, want) {
					t.Errorf("routing is missing ODD clause %q", want)
				}
			}

			hasRDD := strings.Contains(rendered, "Receipt-driven development is user-owned")
			if want := model.SupportsReceiptDrivenDevelopment(agent.ID); hasRDD != want {
				t.Fatalf("routing carries the RDD switch = %v, want %v", hasRDD, want)
			}
			if model.SupportsReceiptDrivenDevelopment(agent.ID) {
				return
			}
			assertNoRDDContent(t, rendered)
		})
	}
}

func TestStripReceiptDrivenDevelopmentFailsClosedOnUnknownReviewWording(t *testing.T) {
	t.Parallel()

	content := "### Delegation Rules\n\nRun `gentle-ai review status` after every task.\n"
	if _, err := stripReceiptDrivenDevelopment(content); err == nil {
		t.Fatal("stripReceiptDrivenDevelopment kept unknown review wording without failing")
	}
}
