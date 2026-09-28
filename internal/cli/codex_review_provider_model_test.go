package cli

import (
	"testing"

	"github.com/gentleman-programming/gentle-ai/v4/internal/model"
	"github.com/gentleman-programming/gentle-ai/v4/internal/reviewerprovider"
	"github.com/gentleman-programming/gentle-ai/v4/internal/state"
)

func TestCodexReviewAdapterDispatchesSavedRoles(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	roles := []struct{ role, lens, key, id string }{
		{reviewProviderRoleLens, "review-risk", "rdd-risk", "gpt-6-astra"},
		{reviewProviderRoleLens, "review-readability", "rdd-readability", "gpt-6-sol"},
		{reviewProviderRoleLens, "review-reliability", "rdd-reliability", "gpt-6-luna"},
		{reviewProviderRoleLens, "review-resilience", "rdd-resilience", "gpt-5.5"},
		{reviewProviderRoleRefuter, "", "rdd-refuter", "gpt-5.4"},
		{reviewProviderRoleTargetedValidator, "", "rdd-validator", "gpt-5.4-mini"},
	}
	saved := state.InstallState{CodexPhaseModelAssignments: map[string]string{}}
	for _, r := range roles {
		saved.CodexPhaseModelAssignments[r.key] = r.id
	}
	if err := state.Write(home, saved); err != nil {
		t.Fatal(err)
	}
	for _, r := range roles {
		adapter, err := reviewProviderAdapter(r.role, model.AgentCodex, r.lens)
		if err != nil {
			t.Fatal(err)
		}
		if got := adapter.(*reviewerprovider.CodexAdapter).Model; got != r.id {
			t.Errorf("%s model = %q, want %q", r.key, got, r.id)
		}
	}
}

func TestCodexReviewAdapterInvalidOrMissingUsesDefault(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	for _, id := range []string{"", "--danger", "model with spaces", "model\n--config"} {
		if err := state.Write(home, state.InstallState{CodexPhaseModelAssignments: map[string]string{"rdd-risk": id, "risk": "gpt-6-sol", "refuter": "gpt-6-astra"}}); err != nil {
			t.Fatal(err)
		}
		adapter, err := reviewProviderAdapter(reviewProviderRoleLens, model.AgentCodex, "review-risk")
		if err != nil {
			t.Fatal(err)
		}
		if got := adapter.(*reviewerprovider.CodexAdapter).Model; got != "" {
			t.Errorf("invalid %q selected %q", id, got)
		}
		refuter, err := reviewProviderAdapter(reviewProviderRoleRefuter, model.AgentCodex)
		if err != nil {
			t.Fatal(err)
		}
		if got := refuter.(*reviewerprovider.CodexAdapter).Model; got != "" {
			t.Errorf("unrelated short refuter key selected %q", got)
		}
	}
}
