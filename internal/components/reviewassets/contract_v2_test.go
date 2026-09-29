package reviewassets

import (
	"context"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v3/internal/model"
	"github.com/gentleman-programming/gentle-ai/v3/internal/opencode"
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
