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
	if strings.Contains(got, "gentle-ai review status") {
		t.Fatal("ReviewExecutionContractFor(pi) exposes raw STATUS")
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

func TestReviewExecutionContractForPiAcknowledgesBeforeBurningAuthority(t *testing.T) {
	contract, err := ReviewExecutionContractFor(model.AgentPi)
	if err != nil {
		t.Fatalf("ReviewExecutionContractFor(pi): %v", err)
	}
	for _, want := range []string{
		"An approved capture awaits acknowledgement; it is not burned.",
		"On `approved`, use bound facade STATUS to obtain or replay the exact provider-issued `acknowledge-approved` continuation, then execute it unchanged.",
		"Only its successful returned envelope burns authority; do not issue STATUS after that burn.",
	} {
		if !strings.Contains(contract, want) {
			t.Errorf("Pi review contract missing acknowledgement rule %q", want)
		}
	}
	if strings.Contains(contract, "On `approved`, authority is already burned") {
		t.Fatal("Pi review contract claims approved authority is already burned")
	}
}

func TestNativeReviewExecutionContractsRetainTheirCLIStatusRoute(t *testing.T) {
	for _, agent := range []model.AgentID{model.AgentClaudeCode, model.AgentCodex, model.AgentOpenCode} {
		t.Run(string(agent), func(t *testing.T) {
			contract, err := ReviewExecutionContractFor(agent)
			if err != nil {
				t.Fatalf("ReviewExecutionContractFor(%s): %v", agent, err)
			}
			status := "gentle-ai review status --cwd <repo> --contract gentle-ai.review-integration/v2 --agent " + string(agent) + " --next-transition"
			if count := strings.Count(contract, status); count != 1 {
				t.Fatalf("native contract contains %d canonical STATUS routes, want 1", count)
			}
			if strings.Contains(contract, "gentle_review_capture") {
				t.Fatal("native contract received a Pi facade route")
			}
		})
	}
}

func TestReviewExecutionContractForPiUsesFacadeLifecycleRoutes(t *testing.T) {
	contract, err := ReviewExecutionContractFor(model.AgentPi)
	if err != nil {
		t.Fatalf("ReviewExecutionContractFor(pi): %v", err)
	}
	for _, want := range []string{
		"`gentle_review` with {\"operation\":\"inspect\"}",
		"`gentle_review` with operation `status`, the exact retained `lineageId`, and `workspaceRoot` only when needed",
		"`gentle_review_capture` for one current returned slot",
		"`gentle_review_capture_group` for the complete current reviewer group",
		"Pi never reconstructs lineage, target, revision, repository context, lens, order, or commands",
		"`gentle_review` with operation `answer-consent` and the exact `consentBinding`",
		"resubmit the same exact binding with `reviewerRunAcknowledged: true`",
		"`gentle-ai review mode enable --scope global`",
	} {
		if !strings.Contains(contract, want) {
			t.Errorf("Pi review contract missing facade route %q", want)
		}
	}
	if strings.Contains(contract, "gentle-ai review status") {
		t.Fatal("Pi review contract instructs raw gentle-ai review status instead of gentle_review")
	}
}
