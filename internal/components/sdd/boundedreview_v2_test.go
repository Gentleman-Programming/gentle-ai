package sdd

import (
	"context"
	"github.com/gentleman-programming/gentle-ai/v3/internal/model"
	"github.com/gentleman-programming/gentle-ai/v3/internal/opencode"
	"strings"
	"testing"
)

func TestOpenCodeV2ReviewToolRendering(t *testing.T) {
	old := opencode.VersionRunnerOverride
	t.Cleanup(func() { opencode.VersionRunnerOverride = old })
	opencode.VersionRunnerOverride = func(context.Context, opencode.Command) (opencode.CommandOutput, error) {
		return opencode.CommandOutput{Stdout: []byte("2.0.4")}, nil
	}
	got := boundedReviewContractFor(model.AgentOpenCode)
	if !strings.HasPrefix(got, "OpenCode V2 review transport is unavailable") {
		t.Fatal("V2 staging does not disclose unavailable capability")
	}
	if strings.Contains(got, "`subagent_type`") || !strings.Contains(got, "`subagent` tool-call") || !strings.Contains(got, "copy `provider_task.agent` exactly as `agent`") {
		t.Fatal("V2 renders V1 tool inputs")
	}
}
