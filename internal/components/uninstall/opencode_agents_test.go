package uninstall

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v3/internal/components/filemerge"
	"github.com/gentleman-programming/gentle-ai/v3/internal/components/opencodeagents"
	"github.com/gentleman-programming/gentle-ai/v3/internal/model"
)

func TestPersonaOnlyUninstallRemovesOnlyManagedGentleman(t *testing.T) {
	for _, agent := range []model.AgentID{model.AgentOpenCode, model.AgentKilocode} {
		for _, modified := range []bool{false, true} {
			t.Run(string(agent)+"/modified="+strconv.FormatBool(modified), func(t *testing.T) {
				home := t.TempDir()
				t.Setenv("HOME", home)
				t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
				svc, err := NewService(home, t.TempDir(), "dev")
				if err != nil {
					t.Fatal(err)
				}
				svc.snapshotter = stubSnapshotter{}
				adapter, _ := svc.registry.Get(agent)
				path := adapter.SettingsPath(home)
				if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
					t.Fatal(err)
				}
				gentleman := map[string]any{"mode": "primary", "description": "Senior Architect mentor - helpful first, challenging when it matters", "prompt": "{file:./AGENTS.md}"}
				if agent == model.AgentKilocode {
					gentleman["tools"] = map[string]any{"write": true, "edit": true}
				}
				if modified {
					gentleman["prompt"] = "user-owned prompt"
				}
				raw, err := json.Marshal(map[string]any{"agent": map[string]any{"gentleman": gentleman, "my-agent": map[string]any{"prompt": "mine"}}})
				if err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, raw, 0600); err != nil {
					t.Fatal(err)
				}
				if _, err := svc.PartialUninstall([]model.AgentID{agent}, []model.ComponentID{model.ComponentPersona}); err != nil {
					t.Fatal(err)
				}
				body, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				var root map[string]any
				if err := json.Unmarshal(body, &root); err != nil {
					t.Fatal(err)
				}
				agents := root["agent"].(map[string]any)
				if _, exists := agents["gentleman"]; exists != modified {
					t.Fatalf("gentleman exists=%v want %v: %s", exists, modified, body)
				}
				if modified && agents["gentleman"].(map[string]any)["prompt"] != "user-owned prompt" {
					t.Fatalf("user prompt changed: %s", body)
				}
				if agents["my-agent"].(map[string]any)["prompt"] != "mine" {
					t.Fatalf("user agent changed: %s", body)
				}
			})
		}
	}
}

func TestKiloUninstallPreservesOpenCodeOnlyShape(t *testing.T) {
	path := filepath.Join(t.TempDir(), "opencode.json")
	var explore map[string]any
	for _, spec := range opencodeagents.Parity(model.AgentOpenCode) {
		if spec.Name == "gentle-ai-explore" {
			var err error
			explore, err = opencodeagents.Entry(spec)
			if err != nil {
				t.Fatal(err)
			}
		}
	}
	if explore == nil {
		t.Fatal("missing explore spec")
	}
	raw, err := json.Marshal(map[string]any{"agent": map[string]any{"gentle-ai-explore": explore}})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, raw, 0600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := removeOpenCodeFamilyAgents(path, model.AgentKilocode).apply(path); err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var root map[string]any
	if err := json.Unmarshal(body, &root); err != nil {
		t.Fatal(err)
	}
	if root["agent"].(map[string]any)["gentle-ai-explore"] == nil {
		t.Fatalf("OpenCode-only agent removed: %s", body)
	}
}

func TestUninstallLegacyMarkedAgents(t *testing.T) {
	for _, tc := range []struct {
		agent   model.AgentID
		fixture string
	}{
		{model.AgentOpenCode, "opencode-v3.7.0-upgrade.json"},
		{model.AgentKilocode, "kilo-v3.7.0-upgrade.json"},
	} {
		t.Run(string(tc.agent), func(t *testing.T) {
			fixture, err := os.ReadFile(filepath.Join("..", "..", "cli", "testdata", tc.fixture))
			if err != nil {
				t.Fatal(err)
			}
			home := t.TempDir()
			t.Setenv("HOME", home)
			t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
			svc, err := NewService(home, t.TempDir(), "dev")
			if err != nil {
				t.Fatal(err)
			}
			svc.snapshotter = stubSnapshotter{}
			adapter, _ := svc.registry.Get(tc.agent)
			path := adapter.SettingsPath(home)
			if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, fixture, 0600); err != nil {
				t.Fatal(err)
			}
			if _, err := svc.PartialUninstall([]model.AgentID{tc.agent}, allManagedComponents); err != nil {
				t.Fatal(err)
			}
			body, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			var root map[string]any
			if err := json.Unmarshal(body, &root); err != nil {
				t.Fatal(err)
			}
			agents := root["agent"].(map[string]any)
			for _, name := range []string{"gentle-orchestrator", "sdd-init", "jd-judge-a", "review-risk", "review-refuter"} {
				if agents[name] != nil {
					t.Errorf("legacy %s retained: %s", name, body)
				}
			}
			if tc.agent == model.AgentOpenCode {
				for _, name := range []string{"general", "explore", "review-validator", "sdd-apply"} {
					if agents[name] != nil {
						t.Errorf("legacy %s retained: %s", name, body)
					}
				}
			}
			if agents["user-owned"].(map[string]any)["prompt"] != "My instructions" {
				t.Fatalf("user-owned changed: %s", body)
			}
			if agents["other-marked"].(map[string]any)["prompt"] != "keep this" {
				t.Fatalf("unknown entry lost: %s", body)
			}
			for name, value := range agents {
				if entry, ok := value.(map[string]any); ok && entry["__managed_by"] != nil {
					t.Errorf("marker remains on %s", name)
				}
			}
		})
	}
}

func TestRemoveOpenCodeOrchestratorOnlyWhenEntireEntryIsManaged(t *testing.T) {
	for _, tc := range []struct {
		name, userPrompt string
		extra            map[string]any
		keep             bool
	}{
		{name: "managed-only"},
		{name: "user prompt", userPrompt: "User instructions\n", keep: true},
		{name: "user option", extra: map[string]any{"temperature": 0.2}, keep: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "opencode.json")
			prompt := filemerge.InjectMarkdownSection(tc.userPrompt, "orchestrator", "Managed instructions")
			prompt = filemerge.InjectMarkdownSection(prompt, "agent-routing", "Managed routing")
			orchestrator := map[string]any{"prompt": prompt, "permission": map[string]any{"task": map[string]any{"jd-judge-b": "allow"}}}
			for key, value := range tc.extra {
				orchestrator[key] = value
			}
			var judge map[string]any
			for _, spec := range opencodeagents.Parity(model.AgentOpenCode) {
				if spec.Name == "jd-judge-b" {
					var entryErr error
					judge, entryErr = opencodeagents.Entry(spec)
					if entryErr != nil {
						t.Fatal(entryErr)
					}
					break
				}
			}
			root := map[string]any{"agent": map[string]any{"gentle-orchestrator": orchestrator, "jd-judge-b": judge}}
			raw, err := json.Marshal(root)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, raw, 0600); err != nil {
				t.Fatal(err)
			}
			op := removeOpenCodeFamilyAgents(path, model.AgentOpenCode)
			changed, _, err := op.apply(path)
			if err != nil || !changed {
				t.Fatalf("rewrite: changed=%v err=%v", changed, err)
			}
			body, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			var after map[string]any
			if err := json.Unmarshal(body, &after); err != nil {
				t.Fatal(err)
			}
			agents := after["agent"].(map[string]any)
			entry, exists := agents["gentle-orchestrator"].(map[string]any)
			if exists != tc.keep {
				t.Fatalf("orchestrator existence=%v, want %v: %s", exists, tc.keep, body)
			}
			if exists {
				remainingPrompt := entry["prompt"].(string)
				if strings.Contains(remainingPrompt, "gentle-ai:") || !strings.Contains(remainingPrompt, tc.userPrompt) {
					t.Fatalf("managed prompt not stripped or user text lost: %s", body)
				}
				if _, ok := entry["permission"]; ok {
					t.Fatalf("managed task retained: %s", body)
				}
			}
		})
	}
}
