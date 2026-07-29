package cli

import (
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/model"
)

// TestComputeSlugSlimVerdictsSafestWinsAcrossDivergentPair pins JD-017: the
// per-slug forwarding verdict uses AND (safest-wins) semantics across every
// adapter sharing a slug. Gemini CLI and Antigravity share the real
// "gemini-cli" slug (engram.SetupAgentSlug); isSlim is fabricated per case
// so the divergent (one slim, one full) branch is reachable without
// depending on IsVerifiedSlimAdapter, which today never verifies either of
// these two agents as slim.
func TestComputeSlugSlimVerdictsSafestWinsAcrossDivergentPair(t *testing.T) {
	agentIDs := []model.AgentID{model.AgentGeminiCLI, model.AgentAntigravity}

	tests := []struct {
		name        string
		isSlim      func(model.AgentID) bool
		wantVerdict bool
	}{
		{
			name: "both slim -> slim",
			isSlim: func(model.AgentID) bool {
				return true
			},
			wantVerdict: true,
		},
		{
			name: "both full -> full",
			isSlim: func(model.AgentID) bool {
				return false
			},
			wantVerdict: false,
		},
		{
			name: "divergent (one slim, one full) -> safest-wins full",
			isSlim: func(agent model.AgentID) bool {
				return agent == model.AgentGeminiCLI
			},
			wantVerdict: false,
		},
		{
			name: "divergent, other order -> still safest-wins full",
			isSlim: func(agent model.AgentID) bool {
				return agent == model.AgentAntigravity
			},
			wantVerdict: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			verdicts := computeSlugSlimVerdicts(agentIDs, tt.isSlim)
			got, ok := verdicts["gemini-cli"]
			if !ok {
				t.Fatalf("computeSlugSlimVerdicts() missing verdict for shared slug %q", "gemini-cli")
			}
			if got != tt.wantVerdict {
				t.Fatalf("computeSlugSlimVerdicts() verdict = %v, want %v", got, tt.wantVerdict)
			}
		})
	}
}

// TestComputeSlugSlimVerdictsSkipsAgentsWithoutSetupSlug ensures agents
// without an engram setup slug (e.g. Cursor, VS Code Copilot) are excluded
// from the verdict map entirely, matching the pre-extraction inline loop.
func TestComputeSlugSlimVerdictsSkipsAgentsWithoutSetupSlug(t *testing.T) {
	agentIDs := []model.AgentID{model.AgentCursor}

	verdicts := computeSlugSlimVerdicts(agentIDs, func(model.AgentID) bool {
		return true
	})
	if len(verdicts) != 0 {
		t.Fatalf("computeSlugSlimVerdicts() = %v, want empty map for an agent with no setup slug", verdicts)
	}
}
