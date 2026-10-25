package uninstall

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v4/internal/components/filemerge"
	"github.com/gentleman-programming/gentle-ai/v4/internal/components/opencodeagents"
	"github.com/gentleman-programming/gentle-ai/v4/internal/model"
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

// TestRemoveOpenCodeOrchestratorQuestionPermission covers the question rule the
// routing owner writes (#4816): the exact "allow" is Gentle AI's and leaves no
// residue, while any other user value keeps the orchestrator alive.
func TestRemoveOpenCodeOrchestratorQuestionPermission(t *testing.T) {
	for _, agent := range []model.AgentID{model.AgentOpenCode, model.AgentKilocode} {
		for _, tc := range []struct {
			name     string
			question any
			keep     bool
		}{
			{name: "managed allow", question: "allow"},
			{name: "user ask", question: "ask", keep: true},
			{name: "user deny", question: "deny", keep: true},
			{name: "user rules", question: map[string]any{"*": "allow"}, keep: true},
		} {
			t.Run(string(agent)+"/"+tc.name, func(t *testing.T) {
				path := filepath.Join(t.TempDir(), "opencode.json")
				prompt := filemerge.InjectMarkdownSection("", "orchestrator", "Managed instructions")
				prompt = filemerge.InjectMarkdownSection(prompt, "agent-routing", "Managed routing")
				orchestrator := map[string]any{"prompt": prompt, "permission": map[string]any{
					"question": tc.question,
					"task":     map[string]any{"jd-judge-b": "allow"},
				}}
				var judge map[string]any
				for _, spec := range opencodeagents.Parity(agent) {
					if spec.Name == "jd-judge-b" {
						var entryErr error
						judge, entryErr = opencodeagents.Entry(spec)
						if entryErr != nil {
							t.Fatal(entryErr)
						}
					}
				}
				raw, err := json.Marshal(map[string]any{"agent": map[string]any{"gentle-orchestrator": orchestrator, "jd-judge-b": judge}})
				if err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, raw, 0600); err != nil {
					t.Fatal(err)
				}
				if _, _, err := removeOpenCodeFamilyAgents(path, agent).apply(path); err != nil {
					t.Fatal(err)
				}
				body, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				var after map[string]any
				if err := json.Unmarshal(body, &after); err != nil {
					t.Fatal(err)
				}
				entry, exists := after["agent"].(map[string]any)["gentle-orchestrator"].(map[string]any)
				if exists != tc.keep {
					t.Fatalf("orchestrator existence=%v, want %v: %s", exists, tc.keep, body)
				}
				if !exists {
					return
				}
				permission, _ := entry["permission"].(map[string]any)
				if !reflect.DeepEqual(permission, map[string]any{"question": tc.question}) {
					t.Fatalf("user question permission not preserved alone: %s", body)
				}
			})
		}
	}
}

// TestUninstallKeepsUnprovenOrchestratorQuestionAllow: question "allow" is
// removed only inside the permission object install writes, where every other
// rule is a Gentle AI delegation grant. Anywhere else the same value may be
// the user's, so uninstall keeps it and reports it.
func TestUninstallKeepsUnprovenOrchestratorQuestionAllow(t *testing.T) {
	for _, agent := range []model.AgentID{model.AgentOpenCode, model.AgentKilocode} {
		for _, tc := range []struct {
			name       string
			permission map[string]any
			owned      bool
		}{
			{name: "install shape", permission: map[string]any{"question": "allow", "task": map[string]any{"jd-judge-b": "allow", "*": "deny"}}, owned: true},
			{name: "user rule beside it", permission: map[string]any{"question": "allow", "bash": "ask", "task": map[string]any{"jd-judge-b": "allow"}}},
			{name: "user delegation target", permission: map[string]any{"question": "allow", "task": map[string]any{"my-agent": "allow"}}},
			{name: "no delegation grants", permission: map[string]any{"question": "allow"}},
		} {
			t.Run(string(agent)+"/"+tc.name, func(t *testing.T) {
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
				raw, err := json.Marshal(map[string]any{"agent": map[string]any{"gentle-orchestrator": map[string]any{
					"prompt":     "My orchestrator notes",
					"permission": tc.permission,
				}}})
				if err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, raw, 0600); err != nil {
					t.Fatal(err)
				}
				result, err := svc.PartialUninstall([]model.AgentID{agent}, allManagedComponents)
				if err != nil {
					t.Fatal(err)
				}
				body, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				var after map[string]any
				if err := json.Unmarshal(body, &after); err != nil {
					t.Fatal(err)
				}
				permission, _ := after["agent"].(map[string]any)["gentle-orchestrator"].(map[string]any)["permission"].(map[string]any)
				reported := slices.ContainsFunc(result.ManualActions, func(action string) bool {
					return strings.Contains(action, path) && strings.Contains(action, "question")
				})
				if tc.owned {
					if _, kept := permission["question"]; kept || reported {
						t.Fatalf("Gentle AI question rule kept=%v reported=%v: %s %v", kept, reported, body, result.ManualActions)
					}
					return
				}
				if permission["question"] != "allow" {
					t.Fatalf("unproven question rule removed: %s", body)
				}
				if !reported {
					t.Fatalf("kept question rule not reported: %v", result.ManualActions)
				}
			})
		}
	}
}
