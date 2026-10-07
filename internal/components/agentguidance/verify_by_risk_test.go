package agentguidance

import (
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v4/internal/assets"
	"github.com/gentleman-programming/gentle-ai/v4/internal/model"
)

// TestRDDNeverReplacesTheRiskTierVerification is verify-always-rdd-high S1:
// on every RDD runtime the installed guidance verifies a delegated writer's
// work by the native risk tier whether receipt-driven development is on or
// off. The native review adds an outside view; it never makes the writer's
// report the verification of record or turns the verifier on-demand.
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
				"follows the native risk tier whether receipt-driven development (RDD) is on or off",
				"high or unassessable: writer self-verification plus an independent verifier",
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

// TestOpenCodeWorkerReportIsNeverTheVerificationOfRecord keeps the writer's
// own contract consistent with S1: its report is self-verification, and the
// risk tier decides whether an independent verifier also runs.
func TestOpenCodeWorkerReportIsNeverTheVerificationOfRecord(t *testing.T) {
	t.Parallel()

	worker := assets.MustRead("opencode/agents/gentle-ai-worker.md")
	if strings.Contains(worker, "verification of record") {
		t.Fatal("OpenCode worker still calls its report the verification of record when RDD is on")
	}
	if !strings.Contains(worker, "the risk tier decides whether an independent verifier also runs") {
		t.Fatal("OpenCode worker does not say the risk tier decides the independent verifier")
	}
}
