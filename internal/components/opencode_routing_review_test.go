package components_test

import (
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v3/internal/assets"
)

// OpenCode retains the parent orchestrator and provider-bound review route,
// even though phase commands and their subtask topology have been retired.
func TestOpenCodeRetainsParentRoutingAndReview(t *testing.T) {
	content := assets.MustRead("opencode/orchestrator.md")
	for _, required := range []string{
		"Organic Driven Development Is The Default Workflow",
		"Mandatory Delegation Triggers",
		"Review Execution Contract",
		"Lossless Blocking Prompts",
	} {
		if !strings.Contains(content, required) {
			t.Errorf("OpenCode orchestrator missing %q", required)
		}
	}
	for _, retired := range []string{"sdd-init", "sdd-apply", "SDD Workflow"} {
		if strings.Contains(content, retired) {
			t.Errorf("OpenCode orchestrator retains %q", retired)
		}
	}
	provider := assets.MustRead("opencode/plugins/opencode-review-transport.ts")
	if !strings.Contains(provider, "gentle-ai") {
		t.Error("OpenCode review transport no longer routes to Gentle AI")
	}
}
