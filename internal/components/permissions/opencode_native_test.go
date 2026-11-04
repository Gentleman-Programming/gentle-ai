package permissions

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	opencodeadapter "github.com/gentleman-programming/gentle-ai/v4/internal/agents/opencode"
)

// TestOpenCodeNativeDocumentsUseOrderedPermissions proves the permission
// injector writes the native ordered `permissions` rule list for native V2
// documents (instead of the legacy `permission` object), preserves rules the
// user already had, and stays idempotent.
func TestOpenCodeNativeDocumentsUseOrderedPermissions(t *testing.T) {
	path := filepath.Join(t.TempDir(), "opencode.jsonc")
	seed := `{
  "providers": {"zai": {}},
  "permissions": [
    {"action": "shell", "resource": "git push", "effect": "ask"}
  ]
}`
	if err := os.WriteFile(path, []byte(seed), 0o644); err != nil {
		t.Fatal(err)
	}

	res, err := InjectAtPath(path, opencodeadapter.NewAdapter())
	if err != nil {
		t.Fatalf("inject: %v", err)
	}
	if !res.Changed {
		t.Fatal("inject did not change a native document")
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var root map[string]any
	if err := json.Unmarshal(raw, &root); err != nil {
		t.Fatal(err)
	}
	if _, exists := root["permission"]; exists {
		t.Fatalf("legacy permission object resurrected on a native document")
	}

	rules, _ := root["permissions"].([]any)
	resolved := map[string]string{}
	userKept := false
	for _, item := range rules {
		rule, _ := item.(map[string]any)
		action, _ := rule["action"].(string)
		resource, _ := rule["resource"].(string)
		effect, _ := rule["effect"].(string)
		resolved[action+"\x00"+resource] = effect
		if action == "shell" && resource == "git push" && effect == "ask" {
			userKept = true
		}
	}
	if !userKept {
		t.Fatalf("user-authored rule dropped: %v", rules)
	}
	if resolved["shell\x00*"] != "allow" {
		t.Fatalf("wildcard shell allow missing: %v", resolved)
	}
	if resolved["shell\x00sudo rm -rf /"] != "deny" {
		t.Fatalf("protected rm deny missing: %v", resolved)
	}
	if resolved["edit\x00*"] != "allow" {
		t.Fatalf("edit wildcard allow missing: %v", resolved)
	}
	if resolved["edit\x00**/.ssh/**"] != "deny" {
		t.Fatalf("sensitive path edit deny missing: %v", resolved)
	}
	if resolved["read\x00**/.ssh/**"] != "deny" {
		t.Fatalf("sensitive path read deny missing: %v", resolved)
	}

	// Second run must be idempotent: identical rules, no duplicates.
	res2, err := InjectAtPath(path, opencodeadapter.NewAdapter())
	if err != nil {
		t.Fatalf("second inject: %v", err)
	}
	if res2.Changed {
		t.Fatal("second inject rewrote identical rules")
	}
	raw2, _ := os.ReadFile(path)
	var root2 map[string]any
	json.Unmarshal(raw2, &root2)
	rules2, _ := root2["permissions"].([]any)
	if len(rules2) != len(rules) {
		t.Fatalf("second inject duplicated rules: %d -> %d", len(rules), len(rules2))
	}
}
