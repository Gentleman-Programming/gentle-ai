package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v3/internal/components/filemerge"
	"github.com/gentleman-programming/gentle-ai/v3/internal/components/opencodeagents"
	"github.com/gentleman-programming/gentle-ai/v3/internal/system"
)

// kiloParityAgentNames is the Kilo agent set: the v3.7.0 Judgment Day roles
// (#4471). Kilo never received review-validator (it hosts no provider relay)
// nor the gentle-ai-* ODD trio (its orchestrator routes to native
// delegation), and it receives no review agent at all because receipt-driven
// development ships only to its own runtimes.
var kiloParityAgentNames = []string{
	"jd-judge-a", "jd-judge-b", "jd-fix-agent",
}

// kiloRetiredReviewAgentNames are the RDD agents earlier releases installed on
// Kilo; upgrades remove the entries Gentle AI owns.
var kiloRetiredReviewAgentNames = []string{
	"review-risk", "review-readability", "review-reliability", "review-resilience",
	"review-refuter", "review-validator",
}

func kiloSettingsPath(home string) string {
	return filepath.Join(home, ".config", "kilo", "opencode.json")
}

func kiloAgents(t *testing.T, raw []byte) map[string]any {
	t.Helper()
	root, err := filemerge.UnmarshalJSONObject(raw)
	if err != nil {
		t.Fatalf("decode Kilo settings: %v", err)
	}
	agents, _ := root["agent"].(map[string]any)
	return agents
}

func assertKiloParityAgents(t *testing.T, agents map[string]any) {
	t.Helper()
	for _, name := range kiloParityAgentNames {
		entry, ok := agents[name].(map[string]any)
		if !ok {
			t.Fatalf("Kilo settings missing agent %q; got %v", name, agents)
		}
		if entry["mode"] != "subagent" || entry["hidden"] != true {
			t.Fatalf("Kilo agent %q missing mode/hidden: %#v", name, entry)
		}
		if prompt, _ := entry["prompt"].(string); strings.TrimSpace(prompt) == "" || strings.Contains(prompt, "obsolete") {
			t.Fatalf("Kilo agent %q has no current prompt: %#v", name, entry)
		}
		if _, ok := entry["permission"].(map[string]any); !ok {
			t.Fatalf("Kilo agent %q missing permission: %#v", name, entry)
		}
	}
	for _, name := range append([]string{"gentle-ai-explore", "gentle-ai-verify", "gentle-ai-worker"}, kiloRetiredReviewAgentNames...) {
		if _, ok := agents[name]; ok {
			t.Fatalf("Kilo must not install %q", name)
		}
	}
	for name, raw := range agents {
		entry, _ := raw.(map[string]any)
		if _, marked := entry["__managed_by"]; marked {
			t.Fatalf("Kilo agent %q still carries __managed_by (#4471): %#v", name, entry)
		}
	}
	orchestrator, _ := agents["gentle-orchestrator"].(map[string]any)
	permission, _ := orchestrator["permission"].(map[string]any)
	task, _ := permission["task"].(map[string]any)
	for _, name := range kiloParityAgentNames {
		if task[name] != "allow" {
			t.Fatalf("Kilo gentle-orchestrator does not allow delegating to %q: %#v", name, task)
		}
	}
	for _, name := range kiloRetiredReviewAgentNames {
		if _, ok := task[name]; ok {
			t.Fatalf("Kilo gentle-orchestrator still delegates to RDD agent %q: %#v", name, task)
		}
	}
}

func TestKiloInstallWritesParityAgentsWithoutManagedByMarker(t *testing.T) {
	home := installTestHome(t)
	args := []string{"--agent", "kilocode", "--preset", "full-gentleman"}
	if _, err := RunInstall(args, system.DetectionResult{}); err != nil {
		t.Fatal(err)
	}
	first, err := os.ReadFile(kiloSettingsPath(home))
	if err != nil {
		t.Fatal(err)
	}
	agents := kiloAgents(t, first)
	assertKiloParityAgents(t, agents)
	for _, name := range kiloParityAgentNames {
		for key := range agents[name].(map[string]any) {
			if !openCodeAgentConfigAllowedKeys[key] {
				t.Fatalf("Kilo agent %q carries non-AgentConfig key %q: %#v", name, key, agents[name])
			}
		}
	}
	if _, err := RunInstall(args, system.DetectionResult{}); err != nil {
		t.Fatal(err)
	}
	if second, _ := os.ReadFile(kiloSettingsPath(home)); !bytes.Equal(first, second) {
		t.Fatalf("second Kilo install changed settings:\nfirst:\n%s\nsecond:\n%s", first, second)
	}
}

func TestKiloUpgradeRetiresOwnedAgents(t *testing.T) {
	for _, tc := range []struct {
		name string
		run  func(t *testing.T)
	}{
		{"install", func(t *testing.T) {
			t.Helper()
			if _, err := RunInstall([]string{"--agent", "kilocode", "--preset", "full-gentleman"}, system.DetectionResult{}); err != nil {
				t.Fatal(err)
			}
		}},
		{"sync", func(t *testing.T) {
			t.Helper()
			if _, err := RunSync([]string{"--agent", "kilocode"}); err != nil {
				t.Fatal(err)
			}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			home := installTestHome(t)
			path := kiloSettingsPath(home)
			if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
				t.Fatal(err)
			}
			fixture, err := os.ReadFile("testdata/kilo-v3.7.0-upgrade.json")
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, fixture, 0o600); err != nil {
				t.Fatal(err)
			}
			before, err := filemerge.UnmarshalJSONObject(fixture)
			if err != nil {
				t.Fatal(err)
			}
			tc.run(t)
			first, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if bytes.Contains(first, []byte(`"__managed_by"`)) {
				t.Fatalf("legacy marker remains:\n%s", first)
			}
			after, err := filemerge.UnmarshalJSONObject(first)
			if err != nil {
				t.Fatal(err)
			}
			agents := after["agent"].(map[string]any)
			original := before["agent"].(map[string]any)
			assertKiloParityAgents(t, agents)
			for name := range agents {
				if strings.HasPrefix(name, "sdd-") {
					t.Errorf("retired owned agent %s remains", name)
				}
			}
			if !reflect.DeepEqual(after["provider"], before["provider"]) {
				t.Error("provider changed")
			}
			if !reflect.DeepEqual(after["mcp"].(map[string]any)["my-server"], before["mcp"].(map[string]any)["my-server"]) {
				t.Error("user MCP server changed")
			}
			if !reflect.DeepEqual(agents["user-owned"], original["user-owned"]) {
				t.Error("user-owned agent changed")
			}
			if !reflect.DeepEqual(agents["other-marked"], map[string]any{"prompt": "keep this"}) {
				t.Error("unknown marked agent changed beyond marker removal")
			}
			for _, name := range append([]string{"gentle-orchestrator"}, kiloParityAgentNames...) {
				entry := agents[name].(map[string]any)
				if _, stale := entry["tools"]; stale {
					t.Errorf("%s retained stale v3.7.0 tools", name)
				}
			}
			for name, want := range map[string]map[string]any{
				"jd-judge-a":          {"model": "user/judge", "variant": "low"},
				"gentle-orchestrator": {"model": "user/orchestrator"},
			} {
				entry := agents[name].(map[string]any)
				for field, value := range want {
					if entry[field] != value {
						t.Errorf("%s lost user %s: %v", name, field, entry)
					}
				}
			}
			tc.run(t)
			if second, _ := os.ReadFile(path); !bytes.Equal(first, second) {
				t.Errorf("second %s changed settings bytes", tc.name)
			}
		})
	}
}

// TestKiloUpgradeRemovesOwnedReviewAgentsAndKeepsUserOnes seeds the review
// agents an earlier release wrote to Kilo in the managed shape, next to a
// user's own review agent under an RDD name, and proves install and sync
// remove only Gentle AI's entries and their orchestrator task permissions.
func TestKiloUpgradeRemovesOwnedReviewAgentsAndKeepsUserOnes(t *testing.T) {
	for _, tc := range []struct {
		name string
		run  func(t *testing.T)
	}{
		{"install", func(t *testing.T) {
			t.Helper()
			if _, err := RunInstall([]string{"--agent", "kilocode", "--preset", "full-gentleman"}, system.DetectionResult{}); err != nil {
				t.Fatal(err)
			}
		}},
		{"sync", func(t *testing.T) {
			t.Helper()
			if _, err := RunSync([]string{"--agent", "kilocode"}); err != nil {
				t.Fatal(err)
			}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			home := installTestHome(t)
			path := kiloSettingsPath(home)
			if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
				t.Fatal(err)
			}

			agents := map[string]any{}
			task := map[string]any{}
			for _, name := range kiloRetiredReviewAgentNames {
				shape, ok := opencodeagents.ReviewShape(name)
				if !ok {
					t.Fatalf("no managed shape for %q", name)
				}
				agents[name] = shape
				task[name] = "allow"
			}
			// A user's model choice on a managed entry does not make it theirs.
			withModel := agents["review-risk"].(map[string]any)
			withModel["model"] = "user/risk"
			// A user's own agent under an RDD name is never ours.
			agents["review-readability"] = map[string]any{"mode": "subagent", "prompt": "My own readability reviewer", "model": "user/readability"}
			agents["gentle-orchestrator"] = map[string]any{"model": "user/orchestrator", "permission": map[string]any{"task": task}}
			seed, err := json.MarshalIndent(map[string]any{"agent": agents}, "", "  ")
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, seed, 0o600); err != nil {
				t.Fatal(err)
			}

			tc.run(t)
			first, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			after := kiloAgents(t, first)
			for _, name := range []string{"review-risk", "review-reliability", "review-resilience", "review-refuter", "review-validator"} {
				if _, ok := after[name]; ok {
					t.Errorf("owned Kilo review agent %q survived: %#v", name, after[name])
				}
			}
			user, _ := after["review-readability"].(map[string]any)
			if user["prompt"] != "My own readability reviewer" || user["model"] != "user/readability" {
				t.Errorf("user-owned review-readability changed: %#v", after["review-readability"])
			}
			orchestrator, _ := after["gentle-orchestrator"].(map[string]any)
			permission, _ := orchestrator["permission"].(map[string]any)
			gotTask, _ := permission["task"].(map[string]any)
			for _, name := range []string{"review-risk", "review-reliability", "review-resilience", "review-refuter", "review-validator"} {
				if _, ok := gotTask[name]; ok {
					t.Errorf("task permission for removed %q survived: %#v", name, gotTask)
				}
			}
			if gotTask["review-readability"] != "allow" {
				t.Errorf("task permission for the user's own review-readability was dropped: %#v", gotTask)
			}
			for _, name := range kiloParityAgentNames {
				if gotTask[name] != "allow" {
					t.Errorf("Kilo gentle-orchestrator does not allow delegating to %q: %#v", name, gotTask)
				}
			}
			if orchestrator["model"] != "user/orchestrator" {
				t.Errorf("gentle-orchestrator lost the user model: %#v", orchestrator)
			}
			tc.run(t)
			if second, _ := os.ReadFile(path); !bytes.Equal(first, second) {
				t.Errorf("second %s changed settings bytes:\nfirst:\n%s\nsecond:\n%s", tc.name, first, second)
			}
		})
	}
}

func TestRetireKiloReviewAgentsPreservesSettingsMode(t *testing.T) {
	path := filepath.Join(t.TempDir(), "opencode.json")
	shape, ok := opencodeagents.ReviewShape("review-risk")
	if !ok {
		t.Fatal("no managed shape for review-risk")
	}
	seed, err := json.Marshal(map[string]any{"agent": map[string]any{
		"review-risk":         shape,
		"gentle-orchestrator": map[string]any{"permission": map[string]any{"task": map[string]any{"review-risk": "allow", "jd-judge-a": "allow"}}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, seed, 0o600); err != nil {
		t.Fatal(err)
	}
	changed, err := retireOpenCodeFamilyReviewAgents(path, "kilocode", nil)
	if err != nil || !changed {
		t.Fatalf("retireOpenCodeFamilyReviewAgents = %v, %v; want a change", changed, err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("settings mode = %v, want 0600 preserved", info.Mode().Perm())
	}
	raw, _ := os.ReadFile(path)
	agents := kiloAgents(t, raw)
	if _, ok := agents["review-risk"]; ok {
		t.Fatalf("managed review-risk survived: %s", raw)
	}
	task := agents["gentle-orchestrator"].(map[string]any)["permission"].(map[string]any)["task"].(map[string]any)
	if _, ok := task["review-risk"]; ok || task["jd-judge-a"] != "allow" {
		t.Fatalf("task permissions = %#v, want only review-risk dropped", task)
	}
	if changed, err := retireOpenCodeFamilyReviewAgents(path, "opencode", nil); err != nil || changed {
		t.Fatalf("OpenCode keeps its review agents: changed=%v err=%v", changed, err)
	}
}

// TestKiloReviewTaskPermissionCleanupRequiresOwnershipProof proves install and
// sync drop an orchestrator task permission for a review agent only when this
// run removed that agent as Gentle AI's: in the managed shape, or under the
// v3.7.0 __managed_by marker. A permission whose agent was already absent is the
// user's and survives.
func TestKiloReviewTaskPermissionCleanupRequiresOwnershipProof(t *testing.T) {
	runs := []struct {
		name string
		run  func(t *testing.T)
	}{
		{"install", func(t *testing.T) {
			t.Helper()
			if _, err := RunInstall([]string{"--agent", "kilocode", "--preset", "full-gentleman"}, system.DetectionResult{}); err != nil {
				t.Fatal(err)
			}
		}},
		{"sync", func(t *testing.T) {
			t.Helper()
			if _, err := RunSync([]string{"--agent", "kilocode"}); err != nil {
				t.Fatal(err)
			}
		}},
	}
	managedRisk, ok := opencodeagents.ReviewShape("review-risk")
	if !ok {
		t.Fatal("no managed shape for review-risk")
	}
	for _, tc := range []struct {
		name      string
		agents    map[string]any
		wantTask  bool
		wantAgent bool
	}{
		{"user permission without agent entry survives", map[string]any{}, true, false},
		{"managed entry and its permission are removed", map[string]any{"review-risk": managedRisk}, false, false},
		{"legacy marked entry and its permission are removed", map[string]any{
			"review-risk": map[string]any{"__managed_by": "gentle-ai/sdd", "mode": "subagent", "prompt": "legacy"},
		}, false, false},
	} {
		for _, run := range runs {
			t.Run(tc.name+"/"+run.name, func(t *testing.T) {
				home := installTestHome(t)
				path := kiloSettingsPath(home)
				if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
					t.Fatal(err)
				}
				agents := map[string]any{
					"gentle-orchestrator": map[string]any{"permission": map[string]any{"task": map[string]any{"review-risk": "allow"}}},
				}
				for name, entry := range tc.agents {
					agents[name] = entry
				}
				seed, err := json.MarshalIndent(map[string]any{"agent": agents}, "", "  ")
				if err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, seed, 0o600); err != nil {
					t.Fatal(err)
				}

				run.run(t)
				first, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				after := kiloAgents(t, first)
				if _, got := after["review-risk"]; got != tc.wantAgent {
					t.Errorf("review-risk agent present = %v, want %v:\n%s", got, tc.wantAgent, first)
				}
				orchestrator, _ := after["gentle-orchestrator"].(map[string]any)
				permission, _ := orchestrator["permission"].(map[string]any)
				task, _ := permission["task"].(map[string]any)
				if _, got := task["review-risk"]; got != tc.wantTask {
					t.Errorf("review-risk task permission present = %v, want %v:\n%s", got, tc.wantTask, first)
				}
				run.run(t)
				if second, _ := os.ReadFile(path); !bytes.Equal(first, second) {
					t.Errorf("second %s changed settings bytes:\nfirst:\n%s\nsecond:\n%s", run.name, first, second)
				}
			})
		}
	}
}
