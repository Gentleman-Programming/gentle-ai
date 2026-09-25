package opencodedefault

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestLegacyUninstallOwnership(t *testing.T) {
	for _, tt := range []struct {
		name, current, previousState, previousDefault, want string
		wantDefault, removeRecord                           bool
	}{
		{name: "owned previous default restored", current: ManagedAgent, previousState: "value", previousDefault: "build", want: "build", wantDefault: true, removeRecord: true},
		{name: "owned absent default removed", current: ManagedAgent, previousState: "absent", removeRecord: true},
		{name: "user modified default and metadata preserved", current: "user-agent", previousState: "value", previousDefault: "build", want: "user-agent", wantDefault: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			settings := filepath.Join(t.TempDir(), "opencode.json")
			original := []byte(`{"default_agent":"` + tt.current + `","unrelated":true}`)
			if err := os.WriteFile(settings, original, 0600); err != nil {
				t.Fatal(err)
			}
			ownerPath := OwnershipPath(settings)
			metadata := []byte(`{"schema":"gentle-ai.opencode-default-agent","version":1,"state":"managed","previous_state":"` + tt.previousState + `","previous_default":"` + tt.previousDefault + `"}`)
			if err := os.WriteFile(ownerPath, metadata, 0600); err != nil {
				t.Fatal(err)
			}
			plan, err := PrepareUninstall(settings)
			if err != nil {
				t.Fatal(err)
			}
			if _, _, err := plan.Apply(original, true); err != nil {
				t.Fatal(err)
			}
			body, err := os.ReadFile(settings)
			if err != nil {
				t.Fatal(err)
			}
			var root map[string]any
			if err := json.Unmarshal(body, &root); err != nil {
				t.Fatal(err)
			}
			value, present := root["default_agent"]
			if present != tt.wantDefault || present && value != tt.want || root["unrelated"] != true {
				t.Fatalf("unexpected settings: %s", body)
			}
			info, err := os.Stat(settings)
			if err != nil || info.Mode().Perm() != 0600 {
				t.Fatalf("settings permissions changed: %v, %v", info, err)
			}
			_, err = os.Stat(ownerPath)
			if tt.removeRecord && !os.IsNotExist(err) || !tt.removeRecord && err != nil {
				t.Fatalf("ownership record existence mismatch: %v", err)
			}
		})
	}
}

func TestUninstallWithoutOwnershipPreservesDefault(t *testing.T) {
	settings := filepath.Join(t.TempDir(), "opencode.json")
	original := []byte(`{"default_agent":"gentle-orchestrator","unrelated":true}`)
	if err := os.WriteFile(settings, original, 0600); err != nil {
		t.Fatal(err)
	}
	plan, err := PrepareUninstall(settings)
	if err != nil {
		t.Fatal(err)
	}
	changed, removed, err := plan.Apply(original, true)
	if err != nil || changed || removed {
		t.Fatalf("unowned default changed: changed=%v removed=%v err=%v", changed, removed, err)
	}
	body, err := os.ReadFile(settings)
	if err != nil || !bytes.Equal(body, original) {
		t.Fatalf("unowned settings changed: %q, %v", body, err)
	}
}

func TestMalformedLegacyOwnershipRefusesUninstall(t *testing.T) {
	settings := filepath.Join(t.TempDir(), "opencode.json")
	original := []byte(`{"default_agent":"gentle-orchestrator"}`)
	if err := os.WriteFile(settings, original, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(OwnershipPath(settings), []byte(`{"schema":"wrong"}`), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := PrepareUninstall(settings); err == nil {
		t.Fatal("malformed ownership accepted")
	}
	body, err := os.ReadFile(settings)
	if err != nil || !bytes.Equal(body, original) {
		t.Fatalf("settings changed: %q, %v", body, err)
	}
}
