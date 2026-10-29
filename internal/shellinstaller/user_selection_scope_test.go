//go:build !windows

package shellinstaller

import (
	"strings"
	"testing"
)

// The inspected confirmation binds the component scope: the worker entry must
// carry the scope verbatim, or the worker recomputes a different token and
// refuses the exact selection the operator confirmed (#5789).
func TestUserInstallFromEntryCarriesUpdateScope(t *testing.T) {
	profile := UserExperience{Persona: "gentleman", BackgroundSubagents: "on"}
	profile.SDKSource = "main-preview"
	scope := CanonicalUpdateScope([]string{UserComponentContext7, UserComponentSDK, UserComponentCodeGraph})
	req := UserInstallRequest{Destination: "/owned/fixture", Mode: "separate", Scope: scope, Experience: &profile}
	encoded, err := profile.Encode()
	if err != nil {
		t.Fatal(err)
	}
	const identity = "identity-fixture"
	args := []string{req.Destination, req.Mode, "", "", "token-fixture", "--scope", scope, "--experience", encoded}
	parsed, err := UserInstallFromEntry(args)
	if err != nil {
		t.Fatalf("entry refused the scoped request: %v", err)
	}
	if parsed.Scope != scope {
		t.Fatalf("worker entry lost the component scope: got %q, want %q", parsed.Scope, scope)
	}
	if got, want := userConfirmation(parsed, identity), userConfirmation(req, identity); got != want {
		t.Fatal("worker entry changed the confirmed confirmation preimage through the scope transport")
	}
	if !strings.Contains(parsed.Scope, UserComponentCodeGraph) {
		t.Fatalf("canonical scope order lost: %q", parsed.Scope)
	}
}

// A request without scope keeps the historical entry bytes: no --scope pair
// may appear and the parsed request must keep an empty scope.
func TestUserInstallFromEntryOmitsEmptyScope(t *testing.T) {
	args := []string{"target", "separate", "", "", "token"}
	parsed, err := UserInstallFromEntry(args)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Scope != "" {
		t.Fatalf("empty scope became %q", parsed.Scope)
	}
	for _, extra := range [][]string{
		{"--scope", ""},
		{"--scope", "codegraph,,context7"},
		{"--scope", strings.Repeat("a", 129)},
	} {
		if _, err := UserInstallFromEntry(append(append([]string(nil), args...), extra...)); err == nil {
			t.Fatalf("malformed scope admitted: %q", extra)
		}
	}
	for _, bad := range []string{"node,codegraph,node", "context7,codegraph", "unknown"} {
		if _, err := UserInstallFromEntry(append(append([]string(nil), args...), "--scope", bad)); err == nil {
			t.Fatalf("non-canonical scope admitted: %q", bad)
		}
	}
}
