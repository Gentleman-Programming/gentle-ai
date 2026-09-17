package sdd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v3/internal/assets"
	"github.com/gentleman-programming/gentle-ai/v3/internal/components/filemerge"
	"github.com/gentleman-programming/gentle-ai/v3/internal/model"
)

func TestOpenCodeV2ProfileAndMerge(t *testing.T) {
	path := filepath.Join(t.TempDir(), "opencode.jsonc")
	raw := `{"agents":{"sdd-orchestrator-fast":{"system":"Keep","model":{"providerID":"native","model":"coder","variant":"high"}},"custom":{"system":"Unrelated"}},"agent":{"sdd-orchestrator-fast":{"model":"stale/model","variant":"low"}},"providers":{}}`
	if err := os.WriteFile(path, []byte(raw), 0600); err != nil {
		t.Fatal(err)
	}
	profiles, err := DetectProfiles(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(profiles) != 1 || profiles[0].OrchestratorModel.ProviderID != "native" || profiles[0].OrchestratorModel.Effort != "high" {
		t.Fatalf("profiles = %+v", profiles)
	}
	_, err = mergeOpenCodeJSONFile(path, []byte(`{"agent":{"sdd-orchestrator-fast":{"model":"new/coder","variant":"low","prompt":"Updated"}}}`))
	if err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(path)
	root, err := filemerge.UnmarshalJSONObject(data)
	if err != nil {
		t.Fatal(err)
	}
	native, _ := root["agents"].(map[string]any)
	entry, _ := native["sdd-orchestrator-fast"].(map[string]any)
	if entry["model"] != "new/coder#low" || entry["system"] != "Updated" {
		t.Fatalf("native overlay = %#v", entry)
	}
	if _, ok := native["custom"]; !ok {
		t.Fatal("custom agent lost")
	}
	if err := RemoveProfileAgents(path, "fast"); err != nil {
		t.Fatal(err)
	}
	data, _ = os.ReadFile(path)
	root, _ = filemerge.UnmarshalJSONObject(data)
	for _, key := range []string{"agent", "agents"} {
		entries, _ := root[key].(map[string]any)
		if _, ok := entries["sdd-orchestrator-fast"]; ok {
			t.Fatalf("profile remains in %s", key)
		}
	}
}

func TestOpenCodeV2GeneratedPermissions(t *testing.T) {
	path := filepath.Join(t.TempDir(), "opencode.json")
	if err := os.WriteFile(path, []byte(`{"agents":{"worker":{"system":"Native","permissions":[{"action":"edit","resource":"*","effect":"allow"}]}}}`), 0600); err != nil {
		t.Fatal(err)
	}
	_, err := mergeOpenCodeJSONFile(path, []byte(`{"agent":{"worker":{"permission":{"task":{"__replace__":{"*":"deny","explore":"allow"}},"bash":"ask"}}}}`))
	if err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(path)
	root, _ := filemerge.UnmarshalJSONObject(data)
	entry := root["agents"].(map[string]any)["worker"].(map[string]any)
	rules, ok := entry["permissions"].([]any)
	if !ok || len(rules) != 4 {
		t.Fatalf("permissions=%#v", entry["permissions"])
	}
	if rules[0].(map[string]any)["action"] != "edit" || rules[1].(map[string]any)["action"] != "shell" || rules[2].(map[string]any)["action"] != "subagent" || rules[2].(map[string]any)["resource"] != "*" {
		t.Fatalf("rule order/actions=%#v", rules)
	}
}

func TestOpenCodeV2ShippedPermissionPreservation(t *testing.T) {
	content, err := assets.Read(overlayAssetPath(model.SDDModeMulti))
	if err != nil {
		t.Fatal(err)
	}
	overlay, err := filemerge.UnmarshalJSONObject([]byte(content))
	if err != nil {
		t.Fatal(err)
	}
	agents := overlay["agent"].(map[string]any)
	for _, tt := range []struct {
		name, agent, action, resource string
		rules                         []any
		want                          string
	}{
		{"multiple user task denies", "gentle-orchestrator", "subagent", "explore", []any{map[string]any{"action": "subagent", "resource": "*", "effect": "deny"}, map[string]any{"action": "subagent", "resource": "explore", "effect": "deny"}}, "deny"},
		{"specific deny with generated exception", "gentle-orchestrator", "subagent", "explore", []any{map[string]any{"action": "subagent", "resource": "*", "effect": "deny"}, map[string]any{"action": "subagent", "resource": "sdd-*", "effect": "allow"}, map[string]any{"action": "subagent", "resource": "explore", "effect": "deny"}}, "deny"},
		{"existing generated exception survives", "gentle-orchestrator", "subagent", "explore", []any{map[string]any{"action": "subagent", "resource": "*", "effect": "deny"}, map[string]any{"action": "subagent", "resource": "explore", "effect": "allow"}}, "allow"},
		{"specific ask survives replacement", "gentle-orchestrator", "subagent", "explore", []any{map[string]any{"action": "subagent", "resource": "*", "effect": "deny"}, map[string]any{"action": "subagent", "resource": "explore", "effect": "ask"}}, "ask"},
		{"existing question deny", "gentle-orchestrator", "question", "any", []any{map[string]any{"action": "question", "resource": "*", "effect": "deny"}}, "deny"},
		{"managed allowed scope", "gentle-orchestrator", "subagent", "explore", []any{map[string]any{"action": "subagent", "resource": "untrusted-agent", "effect": "allow"}}, "allow"},
		{"universal user task deny", "gentle-orchestrator", "subagent", "explore", []any{map[string]any{"action": "subagent", "resource": "*", "effect": "deny"}}, "deny"},
		{"validator allowed command", "review-validator", "shell", "gentle-ai review inspect-candidate --purpose targeted-validation public", []any{map[string]any{"action": "shell", "resource": "gentle-ai review inspect-candidate --purpose targeted-validation restricted*", "effect": "deny"}}, "allow"},
		{"managed task replacement", "gentle-orchestrator", "subagent", "untrusted-agent", []any{map[string]any{"action": "subagent", "resource": "untrusted-agent", "effect": "allow"}}, "deny"},
		{"validator restrictive exception", "review-validator", "shell", "gentle-ai review inspect-candidate --purpose targeted-validation restricted", []any{map[string]any{"action": "shell", "resource": "gentle-ai review inspect-candidate --purpose targeted-validation restricted*", "effect": "deny"}}, "deny"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "opencode.json")
			base, _ := json.Marshal(map[string]any{"agents": map[string]any{tt.agent: map[string]any{"system": "Existing", "permissions": tt.rules}}})
			if err := os.WriteFile(path, base, 0600); err != nil {
				t.Fatal(err)
			}
			patch, _ := json.Marshal(map[string]any{"agent": map[string]any{tt.agent: agents[tt.agent]}})
			if _, err := mergeOpenCodeJSONFile(path, patch); err != nil {
				t.Fatal(err)
			}
			first, _ := os.ReadFile(path)
			if _, err := mergeOpenCodeJSONFile(path, patch); err != nil {
				t.Fatal(err)
			}
			second, _ := os.ReadFile(path)
			if string(first) != string(second) {
				t.Fatalf("native permission merge is not idempotent\nfirst=%s\nsecond=%s", first, second)
			}
			data, _ := os.ReadFile(path)
			root, _ := filemerge.UnmarshalJSONObject(data)
			rules := root["agents"].(map[string]any)[tt.agent].(map[string]any)["permissions"].([]any)
			effect := "ask"
			matches := func(pattern, value string) bool {
				return regexp.MustCompile("^" + strings.ReplaceAll(regexp.QuoteMeta(pattern), `\*`, ".*") + "$").MatchString(value)
			}
			for _, raw := range rules {
				rule := raw.(map[string]any)
				if matches(rule["action"].(string), tt.action) && matches(rule["resource"].(string), tt.resource) {
					effect = rule["effect"].(string)
				}
			}
			if effect != tt.want {
				t.Fatalf("effective %s permission = %s, want %s; rules=%#v", tt.action, effect, tt.want, rules)
			}
		})
	}
}
