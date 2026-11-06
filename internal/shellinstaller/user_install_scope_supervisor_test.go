package shellinstaller

import "testing"

// Update detection reports the supervisor whenever the running build differs
// from the recorded one, which is exactly after a gentle-ai upgrade. Every
// publication copies the supervisor from the running build, so a scoped update
// must admit it, never reuse it, and reuse every other unrequested component.
func TestUserInstallScopeAdmitsSupervisor(t *testing.T) {
	only, err := userInstallScopeOf(UserInstallRequest{Scope: UserComponentSupervisor})
	if err != nil {
		t.Fatalf("supervisor-only update refused: %v", err)
	}
	if only.full || only.reused[UserComponentSupervisor] {
		t.Fatalf("supervisor-only update must stay scoped and never reuse the supervisor: %+v", only)
	}
	for _, name := range []string{UserComponentNode, UserComponentSDK, UserComponentCodeGraph, UserComponentContext7} {
		if !only.reused[name] {
			t.Fatalf("supervisor-only update must reuse %q: %+v", name, only)
		}
	}

	mixed, err := userInstallScopeOf(UserInstallRequest{Scope: CanonicalUpdateScope([]string{UserComponentSupervisor, UserComponentSDK})})
	if err != nil {
		t.Fatalf("detection scope with the supervisor refused: %v", err)
	}
	if mixed.reused[UserComponentSDK] || mixed.reused[UserComponentSupervisor] {
		t.Fatalf("requested components must not be reused: %+v", mixed)
	}
	for _, name := range []string{UserComponentNode, UserComponentCodeGraph, UserComponentContext7} {
		if !mixed.reused[name] {
			t.Fatalf("unrequested component %q must be reused: %+v", name, mixed)
		}
	}
	if _, err := userInstallScopeOf(UserInstallRequest{Scope: "supervisor,supervisor"}); err == nil {
		t.Fatal("duplicate supervisor must stay refused")
	}
}

// The worker entry must carry the same detection scope the TUI confirmed.
func TestUserInstallFromEntryCarriesSupervisorScope(t *testing.T) {
	scope := CanonicalUpdateScope([]string{UserComponentSDK, UserComponentSupervisor})
	encoded, err := (UserExperience{Persona: "gentleman", BackgroundSubagents: "on"}).Encode()
	if err != nil {
		t.Fatal(err)
	}
	req, err := UserInstallFromEntry([]string{"target", "separate", "", "", "token", "--scope", scope, "--experience", encoded})
	if err != nil {
		t.Fatalf("worker entry refused the detection scope %q: %v", scope, err)
	}
	if req.Scope != scope {
		t.Fatalf("worker entry changed the scope: got %q, want %q", req.Scope, scope)
	}
	for _, bad := range []string{"supervisor,sdk", "supervisor,supervisor"} {
		if _, err := UserInstallFromEntry([]string{"target", "separate", "", "", "token", "--scope", bad}); err == nil {
			t.Fatalf("non-canonical scope %q admitted", bad)
		}
	}
}
