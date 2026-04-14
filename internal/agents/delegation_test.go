package agents_test

import (
	"testing"

	"github.com/gentleman-programming/gentle-ai/internal/agents"
	"github.com/gentleman-programming/gentle-ai/internal/model"
)

func TestDelegationModels(t *testing.T) {
	tests := []struct {
		agent model.AgentID
		want  model.DelegationModel
	}{
		{model.AgentClaudeCode, model.DelegationMultiAgent},
		{model.AgentOpenCode, model.DelegationMultiAgent},
		{model.AgentKilocode, model.DelegationMultiAgent},
		{model.AgentKiroIDE, model.DelegationMultiAgent},
		{model.AgentGeminiCLI, model.DelegationMultiAgent},
		{model.AgentCursor, model.DelegationMultiAgent},
		{model.AgentVSCodeCopilot, model.DelegationMultiAgent},
		{model.AgentCodex, model.DelegationSingleAgent},
		{model.AgentQwenCode, model.DelegationSingleAgent},
		{model.AgentAntigravity, model.DelegationSingleAgent},
		{model.AgentWindsurf, model.DelegationSingleAgent},
	}

	for _, tt := range tests {
		t.Run(string(tt.agent), func(t *testing.T) {
			adapter, err := agents.NewAdapter(tt.agent)
			if err != nil {
				t.Fatalf("failed to create adapter for %s: %v", tt.agent, err)
			}

			got := adapter.DelegationModel()
			if got != tt.want {
				t.Errorf("DelegationModel() for %s = %v, want %v", tt.agent, got, tt.want)
			}
		})
	}
}
