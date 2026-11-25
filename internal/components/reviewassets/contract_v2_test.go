package reviewassets

import (
	"context"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v5/internal/model"
	"github.com/gentleman-programming/gentle-ai/v5/internal/opencode"
)

func TestOpenCodeV2ContractAuthorizesReviewWithV2ToolNames(t *testing.T) {
	restore := opencode.VersionRunnerOverride
	t.Cleanup(func() { opencode.VersionRunnerOverride = restore })
	opencode.VersionRunnerOverride = func(context.Context, opencode.Command) (opencode.CommandOutput, error) {
		return opencode.CommandOutput{Stdout: []byte("2.0.19\n")}, nil
	}

	contract := ContractFor(model.AgentOpenCode)
	for _, stale := range []string{"unavailable pending", "Do not start a review", "not capability authorization"} {
		if strings.Contains(contract, stale) {
			t.Fatalf("V2 contract still refuses review: found %q", stale)
		}
	}
	if !strings.Contains(contract, "`subagent`") || strings.Contains(contract, "`subagent_type`") {
		t.Fatalf("V2 contract must name the V2 `subagent` tool and `agent` field")
	}
}

func TestRenderedV2ContractsNamePostRecoveryPreflight(t *testing.T) {
	restore := opencode.VersionRunnerOverride
	t.Cleanup(func() { opencode.VersionRunnerOverride = restore })
	opencode.VersionRunnerOverride = func(context.Context, opencode.Command) (opencode.CommandOutput, error) {
		return opencode.CommandOutput{Stdout: []byte("2.0.19\n")}, nil
	}
	for _, agent := range []model.AgentID{model.AgentClaudeCode, model.AgentCodex, model.AgentOpenCode} {
		t.Run(string(agent), func(t *testing.T) {
			instruction := "After successful `review recover` into `reviewing` under the derived successor name, run the same selectorless v2 preflight from the same worktree: " +
				"`gentle-ai review status --contract gentle-ai.review-integration/v2 --agent " + string(agent) + " --next-transition`, retaining the target selectors. " +
				"It binds the successor in that slot; follow only its returned `next_transition`."
			contract, err := ReviewExecutionContractFor(agent)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(contract, instruction) {
				t.Errorf("rendered contract does not name the post-recovery entry and routing rule")
			}
			if strings.Contains(contract, "**Preflight only.**") {
				t.Errorf("rendered contract still describes selectorless STATUS as START-only")
			}
		})
	}
}
