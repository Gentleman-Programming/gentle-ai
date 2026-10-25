package agentguidance

import (
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v4/internal/assets"
	"github.com/gentleman-programming/gentle-ai/v4/internal/model"
)

// TestRDDNeverReplacesTheRiskTierVerification is verify-always-rdd-high S1:
// on every RDD runtime the installed guidance verifies a delegated writer's
// work by the same rule whether receipt-driven development is on or off. The
// native review adds an outside view; it never makes the writer's report the
// verification of record or turns the verifier on-demand. The wanted clauses
// changed with pain-point-iteration S9 C1: a native high or an unassessable
// diff no longer adds the independent verifier by itself, only a named
// high-risk item does.
func TestRDDNeverReplacesTheRiskTierVerification(t *testing.T) {
	t.Parallel()

	for _, agent := range orchestratorRuntimes(t) {
		if !model.SupportsReceiptDrivenDevelopment(agent) {
			continue
		}
		t.Run(string(agent), func(t *testing.T) {
			t.Parallel()

			result, err := InjectRoutingWithOptions(t.TempDir(), agent, RoutingOptions{})
			if err != nil {
				t.Fatalf("InjectRouting(%q) error = %v", agent, err)
			}
			prompt := deliveredGuidance(t, result.Files[0])
			for _, want := range []string{
				"follows the same rule whether receipt-driven development (RDD) is on or off",
				"high through a named high-risk item: writer self-verification plus one independent verifier",
				"never replaces or skips the tier's verification",
			} {
				if !strings.Contains(prompt, want) {
					t.Errorf("RDD runtime prompt is missing %q", want)
				}
			}
			for _, banned := range []string{"verification of record", "stays on-demand only"} {
				if strings.Contains(prompt, banned) {
					t.Errorf("RDD runtime prompt still lets the native review replace verification (%q)", banned)
				}
			}
		})
	}
}

// TestVerifierRunsOnlyForANamedHighRiskItem is pain-point-iteration S9 C1:
// the independent verifier is for concrete occasions only. Every installed
// orchestrator adds it only when the parent names a Task Size high-risk item;
// a native high tier, an unclear or failed assessment, and the writer's model
// or effort (the retired small-model bias) never add one by themselves.
func TestVerifierRunsOnlyForANamedHighRiskItem(t *testing.T) {
	t.Parallel()

	for _, agent := range orchestratorRuntimes(t) {
		t.Run(string(agent), func(t *testing.T) {
			t.Parallel()

			result, err := InjectRoutingWithOptions(t.TempDir(), agent, RoutingOptions{})
			if err != nil {
				t.Fatalf("InjectRouting(%q) error = %v", agent, err)
			}
			prompt := deliveredGuidance(t, result.Files[0])
			for _, want := range []string{
				"The independent verifier is for concrete occasions only",
				"6. **Verification rule**: only a change that names a Task Size high-risk item gets an independent verifier after the change's own checks, as rarely as possible; a native tier, an unclear or failed assessment, and the writer's model or effort never add one by themselves",
				"**Verification trigger:** only a change whose Risk line names a high-risk item gets an independent verifier after the change's own checks, as rarely as possible",
				"verify per unit only when that unit names a high-risk item (its Risk line or the escalation over its actual diff), never because of the model or effort that wrote the code",
				"separate verifier, whatever model wrote the change",
			} {
				if !strings.Contains(prompt, want) {
					t.Errorf("installed orchestrator for %q is missing %q", agent, want)
				}
			}
			for _, retired := range []string{
				"small-model", "smaller-model", "a mini model", "low effort or",
				"high or unassessable", "**High or unclear**", "that tier wins",
			} {
				if strings.Contains(prompt, retired) {
					t.Errorf("installed orchestrator for %q keeps retired verifier trigger %q", agent, retired)
				}
			}
		})
	}
}

// TestOpenCodeWorkerReportIsNeverTheVerificationOfRecord keeps the writer's
// own contract consistent with S1: its report is self-verification, and the
// risk tier decides whether an independent verifier also runs.
func TestOpenCodeWorkerReportIsNeverTheVerificationOfRecord(t *testing.T) {
	t.Parallel()

	worker := assets.MustRead("opencode/agents/gentle-ai-worker.md")
	if strings.Contains(worker, "verification of record") {
		t.Fatal("OpenCode worker still calls its report the verification of record when RDD is on")
	}
	if !strings.Contains(worker, "a named high-risk item, not the risk tier alone, decides whether an independent verifier also runs") {
		t.Fatal("OpenCode worker does not say a named high-risk item decides the independent verifier (S9 C1: the tier alone never adds one)")
	}
}
