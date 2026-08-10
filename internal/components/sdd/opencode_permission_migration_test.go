package sdd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/assets"
	"github.com/gentleman-programming/gentle-ai/v2/internal/model"
	openconfig "github.com/gentleman-programming/gentle-ai/v2/internal/opencode"
)

func TestOpenCodeManagedAgentPermissionMigration(t *testing.T) {
	for _, mode := range []model.SDDModeID{model.SDDModeSingle, model.SDDModeMulti} {
		t.Run(string(mode), func(t *testing.T) {
			home := t.TempDir()
			path := filepath.Join(home, "opencode.json")
			global := map[string]any{"read": map[string]any{"*": "allow", "**/.env": "deny"}, "bash": map[string]any{"*": "ask"}}
			baseKeys := append([]string{"gentle-orchestrator"}, ProfilePhaseOrder()...)
			baseKeys = append(baseKeys, openconfig.JDPhases()...)
			baseKeys = append(baseKeys, openconfig.ReviewPhases()...)
			profileKeys := ProfileAgentKeys("fast")
			agents := map[string]any{"user": map[string]any{"tools": map[string]any{"read": true, "custom": true}}}
			for _, key := range append(baseKeys, profileKeys...) {
				agents[key] = map[string]any{"tools": map[string]any{"read": true, "bash": true}}
			}
			delete(agents, "sdd-orchestrator-fast")
			base, err := json.Marshal(map[string]any{"permission": global, "agent": agents})
			must(t, err)
			must(t, os.WriteFile(path, append([]byte("// JSONC remains accepted\n"), base...), 0o644))
			overlay, err := assets.Read(overlayAssetPath(mode))
			must(t, err)
			profile, err := GenerateProfileOverlay(model.Profile{Name: "fast", PhaseAssignments: map[string]model.ModelAssignment{
				"jd-judge-a": {ProviderID: "test", ModelID: "judge-a"}, "jd-judge-b": {ProviderID: "test", ModelID: "judge-b"}, "jd-fix-agent": {ProviderID: "test", ModelID: "fix"},
			}}, home, path, nil, "")
			must(t, err)
			for _, content := range [][]byte{[]byte(overlay), profile} {
				_, err := mergeOpenCodeJSONFile(path, content)
				must(t, err)
			}
			first, err := os.ReadFile(path)
			must(t, err)
			config := map[string]any{}
			must(t, json.Unmarshal(first, &config))
			if !reflect.DeepEqual(config["permission"], global) {
				t.Fatalf("global sensitive-read policy changed: %#v", config["permission"])
			}
			result := config["agent"].(map[string]any)
			if !reflect.DeepEqual(result["user"].(map[string]any)["tools"], map[string]any{"read": true, "custom": true}) {
				t.Fatal("user-owned tools changed")
			}
			for _, key := range append(baseKeys, profileKeys...) {
				if _, exists := result[key].(map[string]any)["tools"]; exists {
					t.Errorf("managed agent %q retained tools", key)
				}
			}
			for key, want := range map[string]map[string]any{
				"gentle-orchestrator": {"edit": "deny", "write": "deny", "bash": "deny", "question": "allow"}, "sdd-orchestrator-fast": {"edit": "deny", "write": "deny", "bash": "deny", "question": "allow"},
				"sdd-apply": {"task": "deny"}, "jd-fix-agent": {"task": "deny"},
				"review-risk": {"read": "deny", "edit": "deny", "write": "deny", "bash": "deny", "task": "deny"}, "review-readability": {"read": "deny", "edit": "deny", "write": "deny", "bash": "deny", "task": "deny"},
				"review-reliability": {"read": "deny", "edit": "deny", "write": "deny", "bash": "deny", "task": "deny"}, "review-resilience": {"read": "deny", "edit": "deny", "write": "deny", "bash": "deny", "task": "deny"},
			} {
				permission := result[key].(map[string]any)["permission"].(map[string]any)
				for name, value := range want {
					if permission[name] != value {
						t.Errorf("%s permission[%s] = %#v, want %#v", key, name, permission[name], value)
					}
				}
				if _, executor := want["task"]; executor && (key == "sdd-apply" || key == "jd-fix-agent") && (permission["read"] != nil || permission["bash"] != nil) {
					t.Errorf("%s emitted broad local read/bash permission", key)
				}
			}
			for _, content := range [][]byte{[]byte(overlay), profile} {
				_, err := mergeOpenCodeJSONFile(path, content)
				must(t, err)
			}
			second, err := os.ReadFile(path)
			must(t, err)
			if string(second) != string(first) {
				t.Fatal("merge is not idempotent")
			}
		})
	}
}

func TestMergeJSONFileDoesNotApplyOpenCodeMigration(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	base := []byte(`{"agent":{"gentle-orchestrator":{"tools":{"read":true}}}}`)
	must(t, os.WriteFile(path, base, 0o644))

	_, err := mergeJSONFile(path, []byte(`{}`))
	must(t, err)

	settings, err := os.ReadFile(path)
	must(t, err)
	config := map[string]any{}
	must(t, json.Unmarshal(settings, &config))
	tools := config["agent"].(map[string]any)["gentle-orchestrator"].(map[string]any)["tools"]
	if !reflect.DeepEqual(tools, map[string]any{"read": true}) {
		t.Fatalf("generic JSON merge changed managed OpenCode tools: %#v", tools)
	}
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}
