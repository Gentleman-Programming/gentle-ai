package sdd

import (
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/model"
)

// TestReviewExecutionContractForPiMatchesBoundOrchestratorContract locks the
// exported entry point the provider contract bundle uses (issue #4056) to the
// exact bytes composeOrchestratorPrompt would have spliced into Pi's system
// prompt, had Pi supported one. A duplicate rendering path here is exactly how
// the bundled orchestration text could drift from what every other runtime's
// installed contract says.
func TestReviewExecutionContractForPiMatchesBoundOrchestratorContract(t *testing.T) {
	got, err := ReviewExecutionContractFor(model.AgentPi)
	if err != nil {
		t.Fatalf("ReviewExecutionContractFor(pi): %v", err)
	}
	want := bindRuntimeAgentIdentity(boundedReviewContractFor(model.AgentPi), model.AgentPi)
	if got != want {
		t.Fatalf("ReviewExecutionContractFor(pi) diverged from the bound orchestrator contract")
	}
	if strings.Contains(got, runtimeAgentIDPlaceholder) {
		t.Fatal("ReviewExecutionContractFor(pi) left the runtime identity placeholder unbound")
	}
	if count := strings.Count(got, "--agent pi"); count != 1 {
		t.Fatalf("ReviewExecutionContractFor(pi) contains %d occurrences of `--agent pi`, want 1", count)
	}
	if !strings.Contains(got, "## Entry rule") {
		t.Fatal("ReviewExecutionContractFor(pi) is missing the `## Entry rule` heading")
	}
}

// TestReviewExecutionContractForRejectsRuntimesThatDoNotRenderTheLifecycle
// guards the same gate renderBoundedReviewAssetBody already applies: a runtime
// outside the closed RDD set must never receive review-transport prose, bundled
// or otherwise.
func TestReviewExecutionContractForRejectsRuntimesThatDoNotRenderTheLifecycle(t *testing.T) {
	if _, err := ReviewExecutionContractFor(model.AgentKilocode); err == nil {
		t.Fatal("ReviewExecutionContractFor(kilocode) succeeded, want an error")
	}
}
