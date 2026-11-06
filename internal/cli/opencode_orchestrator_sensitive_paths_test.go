package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v4/internal/system"
)

// TestOpenCodeAgentPermissionsNeverShadowSensitivePathDenies pins #787.
// OpenCode merges each agent's permission block with the global one and lets
// agent rules take precedence, so the top-level sensitive-path denies (.env
// and friends) protect every agent only while no generated agent carries its
// own allow for the tools those denies cover. The orchestrator's generated
// block holds task and question rules only; this keeps any future read, edit,
// write, or patch allow from silently re-opening the protected paths.
func TestOpenCodeAgentPermissionsNeverShadowSensitivePathDenies(t *testing.T) {
	home := installTestHome(t)
	if _, err := RunInstall([]string{"--agent", "opencode", "--component", "persona", "--component", "permissions"}, system.DetectionResult{}); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(home, ".config", "opencode", "opencode.json"))
	if err != nil {
		t.Fatal(err)
	}
	var config struct {
		Permission map[string]any            `json:"permission"`
		Agent      map[string]map[string]any `json:"agent"`
	}
	if err := json.Unmarshal(raw, &config); err != nil {
		t.Fatalf("decode opencode.json: %v", err)
	}

	for _, tool := range []string{"read", "edit"} {
		rules, _ := config.Permission[tool].(map[string]any)
		if rules["**/.env"] != "deny" {
			t.Fatalf("top-level permission.%s[\"**/.env\"] = %#v, want \"deny\"", tool, rules["**/.env"])
		}
	}
	if _, ok := config.Agent["gentle-orchestrator"]; !ok {
		t.Fatal("opencode.json missing gentle-orchestrator")
	}

	for name, entry := range config.Agent {
		permission, _ := entry["permission"].(map[string]any)
		for _, tool := range []string{"read", "edit", "write", "patch"} {
			if grantsAllow(permission[tool]) {
				t.Fatalf("agent %q permission.%s grants allow, which takes precedence over the top-level sensitive-path deny: %#v", name, tool, permission[tool])
			}
		}
		if permission["*"] == "allow" {
			t.Fatalf("agent %q permission[\"*\"] = \"allow\" re-opens every tool, including the sensitive-path denies: %#v", name, permission)
		}
	}
}

// grantsAllow reports whether an OpenCode permission value allows anything:
// the bare "allow" action or any pattern rule mapped to "allow".
func grantsAllow(value any) bool {
	switch v := value.(type) {
	case string:
		return v == "allow"
	case map[string]any:
		for _, action := range v {
			if action == "allow" {
				return true
			}
		}
	}
	return false
}
