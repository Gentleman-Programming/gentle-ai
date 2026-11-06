//go:build windows

package shellinstaller

import (
	"encoding/json"
	"testing"
)

// The staged selection.json is the six-field provision ABI (Channel,
// Confirmation, Destination, Mode, SharedAgent, SharedPrefix). A component-
// scoped update marshals a seventh key, and the provisioner refuses the whole
// update with "publication channel/selection" — the exact failure a scoped
// Windows update hit after Scope joined the request (#5917 follow-up). Both
// optional fields must drop from the staged request.
func TestUserWindowsStagedSelectionKeepsSixFieldABI(t *testing.T) {
	req := UserInstallRequest{
		Destination:  `C:\owned\shell`,
		Mode:         "separate",
		Channel:      "stable",
		Confirmation: "fixture-token",
		Scope:        "supervisor,sdk",
	}
	req.Experience = &UserExperience{Persona: "gentleman"}
	data := userWindowsStagedSelection(req)
	var selection map[string]any
	if err := json.Unmarshal(data, &selection); err != nil {
		t.Fatalf("staged selection is not JSON: %v", err)
	}
	if len(selection) != 6 {
		t.Fatalf("staged selection carries %d keys, want exactly 6: %v", len(selection), selection)
	}
	for _, key := range []string{"Channel", "Confirmation", "Destination", "Mode", "SharedAgent", "SharedPrefix"} {
		if _, ok := selection[key]; !ok {
			t.Fatalf("staged selection is missing the required key %q: %v", key, selection)
		}
	}
	for _, leaked := range []string{"Scope", "Experience"} {
		if _, ok := selection[leaked]; ok {
			t.Fatalf("staged selection leaks the optional field %q into the provision ABI", leaked)
		}
	}
	// The staged helper composes the full tree fresh, so the scope never
	// reaches the provisioner through any other field either.
	var parsed UserInstallRequest
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("staged selection does not round-trip: %v", err)
	}
	if parsed.Scope != "" || parsed.Experience != nil {
		t.Fatalf("staged selection round-trips with optional fields: %+v", parsed)
	}
}
