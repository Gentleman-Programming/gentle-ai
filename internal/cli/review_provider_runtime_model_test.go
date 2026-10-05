package cli

import (
	"os"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v4/internal/model"
	"github.com/gentleman-programming/gentle-ai/v4/internal/reviewerprovider"
	"github.com/gentleman-programming/gentle-ai/v4/internal/state"
)

func TestClaudeReviewAdapterUsesSavedModelForEachNativeRole(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	roles := []struct{ role, lens, model string }{
		{reviewProviderRoleLens, "review-risk", "opus"},
		{reviewProviderRoleLens, "review-readability", "haiku"},
		{reviewProviderRoleLens, "review-reliability", "sonnet"},
		{reviewProviderRoleLens, "review-resilience", "fable"},
		{reviewProviderRoleRefuter, "", "opus"},
		{reviewProviderRoleTargetedValidator, "", "haiku"},
	}
	keys := []string{"risk", "readability", "reliability", "resilience", "refuter", "validator"}
	persisted := state.InstallState{ClaudePhaseAssignments: make(map[string]state.ClaudePhaseAssignmentState)}
	for i, key := range keys {
		persisted.ClaudePhaseAssignments[key] = state.ClaudePhaseAssignmentState{Model: roles[i].model}
	}
	if err := state.Write(home, persisted); err != nil {
		t.Fatal(err)
	}
	loaded, err := state.Read(home)
	if err != nil {
		t.Fatal(err)
	}
	for i, key := range keys {
		if loaded.ClaudePhaseAssignments[key].Model != roles[i].model {
			t.Fatalf("persisted role %s was lost", key)
		}
		adapter, err := reviewProviderAdapter(roles[i].role, model.AgentClaudeCode, roles[i].lens)
		if err != nil {
			t.Fatal(err)
		}
		if got := adapter.(*reviewerprovider.ClaudeAdapter).Model; got != model.ClaudeModelAlias(roles[i].model) {
			t.Errorf("%s selected %q, want %q", key, got, roles[i].model)
		}
	}
}

func TestClaudeReviewAdapterMissingAndInvalidAssignmentsUseNativeDefault(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	for _, persisted := range []state.InstallState{
		{},
		{ClaudePhaseAssignments: map[string]state.ClaudePhaseAssignmentState{"risk": {Model: "invalid"}}},
		{ClaudePhaseAssignments: map[string]state.ClaudePhaseAssignmentState{"risk": {Model: "opus", Effort: "invalid"}}},
		{ClaudeModelAssignments: map[string]string{"risk": "invalid"}},
	} {
		if err := state.Write(home, persisted); err != nil {
			t.Fatal(err)
		}
		adapter, err := reviewProviderAdapter(reviewProviderRoleLens, model.AgentClaudeCode, "review-risk")
		if err != nil {
			t.Fatal(err)
		}
		if got := adapter.(*reviewerprovider.ClaudeAdapter).Model; got != "" {
			t.Errorf("invalid or absent assignment selected %q", got)
		}
	}
	if err := state.Write(home, state.InstallState{ClaudeModelAssignments: map[string]string{"refuter": "sonnet"}}); err != nil {
		t.Fatal(err)
	}
	adapter, err := reviewProviderAdapter(reviewProviderRoleRefuter, model.AgentClaudeCode)
	if err != nil || adapter.(*reviewerprovider.ClaudeAdapter).Model != model.ClaudeModelSonnet {
		t.Fatalf("legacy assignment = %v, %v", adapter, err)
	}
	if err := state.Write(home, state.InstallState{
		ClaudePhaseAssignments: map[string]state.ClaudePhaseAssignmentState{"refuter": {Model: "invalid"}},
		ClaudeModelAssignments: map[string]string{"refuter": "sonnet"},
	}); err != nil {
		t.Fatal(err)
	}
	if got := savedClaudeReviewModel("refuter"); got != model.ClaudeModelSonnet {
		t.Fatalf("invalid phase assignment should preserve valid legacy fallback, got %q", got)
	}
	if err := state.Write(home, state.InstallState{
		ClaudePhaseAssignments: map[string]state.ClaudePhaseAssignmentState{"refuter": {Model: "opus", Effort: "invalid"}},
		ClaudeModelAssignments: map[string]string{"refuter": "sonnet"},
	}); err != nil {
		t.Fatal(err)
	}
	if got := savedClaudeReviewModel("refuter"); got != model.ClaudeModelSonnet {
		t.Fatalf("invalid effort should preserve valid legacy fallback, got %q", got)
	}
	if err := state.Write(home, state.InstallState{
		ClaudePhaseAssignments: map[string]state.ClaudePhaseAssignmentState{"refuter": {Model: "opus", Effort: "high"}},
		ClaudeModelAssignments: map[string]string{"refuter": "sonnet"},
	}); err != nil {
		t.Fatal(err)
	}
	adapter, err = reviewProviderAdapter(reviewProviderRoleRefuter, model.AgentClaudeCode)
	if err != nil {
		t.Fatal(err)
	}
	if got := adapter.(*reviewerprovider.ClaudeAdapter).Model; got != model.ClaudeModelOpus {
		t.Fatalf("valid model+effort selected %q, want opus", got)
	}
	if err := os.WriteFile(state.Path(home), []byte("invalid JSON"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := savedClaudeReviewModel("refuter"); got != "" {
		t.Fatalf("invalid persisted state selected %q", got)
	}
}
