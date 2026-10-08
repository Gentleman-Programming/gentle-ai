package cli

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v4/internal/components/filemerge"
	"github.com/gentleman-programming/gentle-ai/v4/internal/components/opencodeagents"
	"github.com/gentleman-programming/gentle-ai/v4/internal/model"
	runtimeopencode "github.com/gentleman-programming/gentle-ai/v4/internal/opencode"
	"github.com/gentleman-programming/gentle-ai/v4/internal/system"
)

// assertOpenCodeOrchestratorPermissions pins the permissions the managed
// orchestrator needs at runtime, not only the cleanup of obsolete keys.
// OpenCode denies the native question tool to custom agents unless their own
// permission allows it (#4816), and the orchestrator prompt relays blocking
// prompts through it; v3.7.0 shipped the same rule.
func assertOpenCodeOrchestratorPermissions(t *testing.T, agents map[string]any) {
	t.Helper()
	orchestrator, _ := agents["gentle-orchestrator"].(map[string]any)
	permission, _ := orchestrator["permission"].(map[string]any)
	if permission["question"] != "allow" {
		t.Errorf("gentle-orchestrator permission.question = %#v, want \"allow\" (#4816)", permission["question"])
	}
	task, _ := permission["task"].(map[string]any)
	for _, name := range opencodeagents.Roles(model.AgentOpenCode) {
		if task[name] != "allow" {
			t.Errorf("gentle-orchestrator does not allow delegating to %q: %#v", name, task)
		}
	}
}

// openCodeRuntimeVersions covers both runtime majors: 1.x denies question to
// custom agents by default, while 2.x starts them from allow-all and migrates
// the same agent.<name>.permission.question rule from the 1.x config shape.
var openCodeRuntimeVersions = []string{"1.18.30", "2.0.23"}

// stubOpenCodeRuntimeVersion selects the runtime major; for 2.x it also seeds
// the installed plugin SDK the V2 preflight requires before any write.
func stubOpenCodeRuntimeVersion(t *testing.T, home, version string) {
	t.Helper()
	old := runtimeopencode.VersionRunnerOverride
	t.Cleanup(func() { runtimeopencode.VersionRunnerOverride = old })
	runtimeopencode.VersionRunnerOverride = func(context.Context, runtimeopencode.Command) (runtimeopencode.CommandOutput, error) {
		return runtimeopencode.CommandOutput{Stdout: []byte(version)}, nil
	}
	if runtimeopencode.ParseRuntimeMajor(version) != runtimeopencode.RuntimeV2 {
		return
	}
	sdk := filepath.Join(home, ".config", "opencode", "node_modules", "@opencode", "plugin")
	if err := os.MkdirAll(sdk, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sdk, "package.json"), []byte(`{"name":"@opencode/plugin","version":"2.0.23"}`), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestOpenCodeFreshInstallAllowsOrchestratorQuestion(t *testing.T) {
	for _, version := range openCodeRuntimeVersions {
		t.Run(version, func(t *testing.T) {
			home := installTestHome(t)
			stubOpenCodeRuntimeVersion(t, home, version)
			if _, err := RunInstall([]string{"--agent", "opencode", "--component", "persona"}, system.DetectionResult{}); err != nil {
				t.Fatal(err)
			}
			raw, err := os.ReadFile(filepath.Join(home, ".config", "opencode", "opencode.json"))
			if err != nil {
				t.Fatal(err)
			}
			root, err := filemerge.UnmarshalJSONObject(raw)
			if err != nil {
				t.Fatal(err)
			}
			agents, _ := root["agent"].(map[string]any)
			assertOpenCodeOrchestratorPermissions(t, agents)
		})
	}
}

func TestOpenCodeUpgradeRetiresOwnedAgents(t *testing.T) {
	for _, tc := range []struct {
		name string
		run  func(t *testing.T)
	}{
		{"install", func(t *testing.T) {
			t.Helper()
			if _, err := RunInstall([]string{"--agent", "opencode", "--component", "persona"}, system.DetectionResult{}); err != nil {
				t.Fatal(err)
			}
		}},
		{"sync", func(t *testing.T) {
			t.Helper()
			if _, err := RunSync([]string{"--agent", "opencode"}); err != nil {
				t.Fatal(err)
			}
		}},
	} {
		for _, version := range openCodeRuntimeVersions {
			t.Run(tc.name+"/"+version, func(t *testing.T) {
				home := installTestHome(t)
				stubOpenCodeRuntimeVersion(t, home, version)
				path := filepath.Join(home, ".config", "opencode", "opencode.json")
				if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
					t.Fatal(err)
				}
				fixture, err := os.ReadFile("testdata/opencode-v3.7.0-upgrade.json")
				if err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, fixture, 0600); err != nil {
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
					t.Fatal("legacy marker remains")
				}
				after, err := filemerge.UnmarshalJSONObject(first)
				if err != nil {
					t.Fatal(err)
				}
				agents := after["agent"].(map[string]any)
				original := before["agent"].(map[string]any)
				for _, key := range []string{"sdd-init", "sdd-apply", "general", "explore"} {
					if _, ok := agents[key]; ok {
						t.Errorf("retired owned agent %s remains", key)
					}
				}
				if !reflect.DeepEqual(after["provider"], before["provider"]) {
					t.Error("provider changed")
				}
				// Sync also refreshes managed MCP entries; the user's unrelated server stays intact.
				beforeMCP := before["mcp"].(map[string]any)
				afterMCP := after["mcp"].(map[string]any)
				if !reflect.DeepEqual(afterMCP["my-server"], beforeMCP["my-server"]) {
					t.Error("user MCP server changed")
				}
				if !reflect.DeepEqual(agents["user-owned"], original["user-owned"]) {
					t.Error("user-owned agent changed")
				}
				if !reflect.DeepEqual(agents["other-marked"], map[string]any{"prompt": "keep this"}) {
					t.Error("unknown marked agent changed beyond marker removal")
				}
				for _, key := range []string{"jd-judge-a", "review-risk", "review-refuter", "review-validator", "gentle-orchestrator"} {
					entry := agents[key].(map[string]any)
					if _, stale := entry["tools"]; stale {
						t.Errorf("%s retained stale tools", key)
					}
					if strings.Contains(entry["prompt"].(string), "obsolete") || strings.Contains(entry["prompt"].(string), "{file:./AGENTS.md}") {
						t.Errorf("%s retained stale prompt", key)
					}
				}
				assertOpenCodeOrchestratorPermissions(t, agents)
				judge := agents["jd-judge-a"].(map[string]any)
				if judge["model"] != "user/judge" || judge["variant"] != "low" {
					t.Errorf("judge model/variant lost: %v", judge)
				}
				tc.run(t)
				second, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				if !bytes.Equal(first, second) {
					t.Errorf("second %s changed settings bytes", tc.name)
				}
			})
		}
	}
}

func TestOpenCodeUpgradePreservesUnmarkedGeneral(t *testing.T) {
	home := installTestHome(t)
	path := filepath.Join(home, ".config", "opencode", "opencode.json")
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(`{"agent":{"general":{"prompt":"my general","model":"user/model"}}}`), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := RunInstall([]string{"--agent", "opencode", "--component", "persona"}, system.DetectionResult{}); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	root, err := filemerge.UnmarshalJSONObject(raw)
	if err != nil {
		t.Fatal(err)
	}
	general := root["agent"].(map[string]any)["general"]
	if !reflect.DeepEqual(general, map[string]any{"prompt": "my general", "model": "user/model"}) {
		t.Fatalf("unmarked general changed: %v", general)
	}
}

func TestOpenCodeInstallKeepsUserQuestionRule(t *testing.T) {
	for _, rule := range []string{`"ask"`, `"deny"`, `{"*":"ask"}`} {
		t.Run(rule, func(t *testing.T) {
			home := installTestHome(t)
			path := filepath.Join(home, ".config", "opencode", "opencode.json")
			if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, []byte(`{"agent":{"gentle-orchestrator":{"permission":{"question":`+rule+`}}}}`), 0o600); err != nil {
				t.Fatal(err)
			}
			if _, err := RunInstall([]string{"--agent", "opencode", "--component", "persona"}, system.DetectionResult{}); err != nil {
				t.Fatal(err)
			}
			raw, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			root, err := filemerge.UnmarshalJSONObject(raw)
			if err != nil {
				t.Fatal(err)
			}
			want, err := filemerge.UnmarshalJSONObject([]byte(`{"question":` + rule + `}`))
			if err != nil {
				t.Fatal(err)
			}
			permission := root["agent"].(map[string]any)["gentle-orchestrator"].(map[string]any)["permission"].(map[string]any)
			if !reflect.DeepEqual(permission["question"], want["question"]) {
				t.Fatalf("user question rule replaced: %#v", permission["question"])
			}
		})
	}
}

// The orchestrator delegates only to Gentle AI agents: a wildcard deny keeps
// OpenCode's built-in explore/general subagents (which lack the skill tool)
// from replacing gentle-ai-explore. OpenCode applies the last matching rule,
// so the wildcard must precede the grants in the written document.
func TestOpenCodeOrchestratorDeniesNonGentleSubagents(t *testing.T) {
	for _, command := range []string{"install", "sync"} {
		for _, version := range openCodeRuntimeVersions {
			t.Run(command+"/"+version, func(t *testing.T) {
				home := installTestHome(t)
				stubOpenCodeRuntimeVersion(t, home, version)
				var err error
				if command == "install" {
					_, err = RunInstall([]string{"--agent", "opencode", "--component", "persona"}, system.DetectionResult{})
				} else {
					if _, err = RunInstall([]string{"--agent", "opencode", "--component", "persona"}, system.DetectionResult{}); err == nil {
						_, err = RunSync([]string{"--agent", "opencode"})
					}
				}
				if err != nil {
					t.Fatal(err)
				}
				raw, err := os.ReadFile(filepath.Join(home, ".config", "opencode", "opencode.json"))
				if err != nil {
					t.Fatal(err)
				}
				root, err := filemerge.UnmarshalJSONObject(raw)
				if err != nil {
					t.Fatal(err)
				}
				agents, _ := root["agent"].(map[string]any)
				assertOpenCodeOrchestratorPermissions(t, agents)
				task := agents["gentle-orchestrator"].(map[string]any)["permission"].(map[string]any)["task"].(map[string]any)
				if task["*"] != "deny" {
					t.Fatalf("orchestrator task permission = %#v, want \"*\": \"deny\" before the Gentle AI grants", task)
				}
				text := string(raw)
				wildcard := strings.Index(text, `"*": "deny"`)
				grant := strings.Index(text, `"gentle-ai-explore": "allow"`)
				if wildcard < 0 || grant < 0 || wildcard > grant {
					t.Fatalf("wildcard deny must precede the grants (last match wins):\n%s", text)
				}
			})
		}
	}
}

func TestOpenCodeInstallKeepsUserTaskWildcardRule(t *testing.T) {
	home := installTestHome(t)
	path := filepath.Join(home, ".config", "opencode", "opencode.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(`{"agent":{"gentle-orchestrator":{"permission":{"task":{"*":"ask"}}}}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := RunInstall([]string{"--agent", "opencode", "--component", "persona"}, system.DetectionResult{}); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	root, err := filemerge.UnmarshalJSONObject(raw)
	if err != nil {
		t.Fatal(err)
	}
	task := root["agent"].(map[string]any)["gentle-orchestrator"].(map[string]any)["permission"].(map[string]any)["task"].(map[string]any)
	if task["*"] != "ask" {
		t.Fatalf("user task wildcard rule replaced: %#v", task["*"])
	}
}

func TestOpenCodeSyncAddsWildcardDenyBeforeExistingGrants(t *testing.T) {
	home := installTestHome(t)
	stubOpenCodeRuntimeVersion(t, home, "2.0.23")
	path := filepath.Join(home, ".config", "opencode", "opencode.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	seed := `{"agent":{"gentle-orchestrator":{"permission":{"question":"allow","task":{"gentle-ai-explore":"allow","review-risk":"allow"}}}}}`
	if err := os.WriteFile(path, []byte(seed), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := RunInstall([]string{"--agent", "opencode", "--component", "persona"}, system.DetectionResult{}); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)
	wildcard := strings.Index(text, `"*": "deny"`)
	grant := strings.Index(text, `"gentle-ai-explore": "allow"`)
	if wildcard < 0 || grant < 0 || wildcard > grant {
		t.Fatalf("wildcard deny must precede existing grants (last match wins):\n%s", text)
	}
}
