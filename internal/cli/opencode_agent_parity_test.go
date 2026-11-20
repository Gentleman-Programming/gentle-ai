package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v4/internal/system"
)

// openCodeAgentConfigAllowedKeys is the explicit allowlist of OpenCode
// AgentConfig fields this install path is allowed to write into any agent
// entry. Anything else -- most notably the retired "__managed_by" marker
// (#4471) -- is forwarded to providers as an unknown request option and
// rejected by strict providers.
var openCodeAgentConfigAllowedKeys = map[string]bool{
	"mode":        true,
	"hidden":      true,
	"description": true,
	"prompt":      true,
	"permission":  true,
}

// openCodeParityAgentNames is the full parity set from #4471: the 10 agents
// ported from Gentle Shell's global agents, plus the two review provider
// roles retained across the SDD retirement.
var openCodeParityAgentNames = []string{
	"gentle-ai-explore", "gentle-ai-verify", "gentle-ai-worker",
	"jd-judge-a", "jd-judge-b", "jd-fix-agent",
	"review-risk", "review-readability", "review-reliability", "review-resilience",
	"review-refuter", "review-validator",
}

// TestOpenCodeInstallWritesParityAgentsWithoutManagedByMarker is the RED test
// for #4471: a fresh install must install the full parity set (JD + the four
// review lenses + the two review provider roles), every subagent entry must
// carry mode/hidden/description/prompt, and no agent entry may carry
// "__managed_by" or any key outside the documented AgentConfig allowlist.
func TestOpenCodeInstallWritesParityAgentsWithoutManagedByMarker(t *testing.T) {
	home := installTestHome(t)
	if _, err := RunInstall([]string{"--agent", "opencode", "--component", "persona"}, system.DetectionResult{}); err != nil {
		t.Fatal(err)
	}
	settingsPath := filepath.Join(home, ".config", "opencode", "opencode.json")
	raw, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatal(err)
	}
	var config struct {
		Agent map[string]map[string]any `json:"agent"`
	}
	if err := json.Unmarshal(raw, &config); err != nil {
		t.Fatalf("decode opencode.json: %v", err)
	}

	for _, name := range openCodeParityAgentNames {
		entry, ok := config.Agent[name]
		if !ok {
			t.Fatalf("opencode.json missing parity agent %q; got agents: %v", name, config.Agent)
		}
		wantHidden := !strings.HasPrefix(name, "gentle-ai-")
		if entry["mode"] != "subagent" || entry["hidden"] != wantHidden {
			t.Fatalf("parity agent %q mode/hidden = (%#v, %#v), want (\"subagent\", %v): %#v", name, entry["mode"], entry["hidden"], wantHidden, entry)
		}
		if desc, _ := entry["description"].(string); desc == "" {
			t.Fatalf("parity agent %q missing description: %#v", name, entry)
		}
		if prompt, _ := entry["prompt"].(string); prompt == "" {
			t.Fatalf("parity agent %q missing prompt: %#v", name, entry)
		}
	}

	for name, entry := range config.Agent {
		for key := range entry {
			if !openCodeAgentConfigAllowedKeys[key] {
				t.Fatalf("agent %q carries non-AgentConfig key %q (forwarded to providers as an unknown option, #4471): %#v", name, key, entry)
			}
		}
	}

	orchestrator, ok := config.Agent["gentle-orchestrator"]
	if !ok {
		t.Fatal("opencode.json missing gentle-orchestrator")
	}
	permission, _ := orchestrator["permission"].(map[string]any)
	task, _ := permission["task"].(map[string]any)
	for _, name := range []string{
		"gentle-ai-explore", "gentle-ai-verify", "gentle-ai-worker",
		"jd-judge-a", "jd-judge-b", "jd-fix-agent",
		"review-risk", "review-readability", "review-reliability", "review-resilience",
	} {
		if task[name] != "allow" {
			t.Fatalf("gentle-orchestrator does not allow delegating to parity agent %q: %#v", name, task)
		}
	}
}

// TestOpenCodeInstallParityAgentsAreIdempotent is the second RED assertion for
// #4471: running the install step twice must yield byte-identical bytes for
// every parity agent entry (no drifting timestamps, no accumulating keys).
func TestOpenCodeInstallParityAgentsAreIdempotent(t *testing.T) {
	home := installTestHome(t)
	if _, err := RunInstall([]string{"--agent", "opencode", "--component", "persona"}, system.DetectionResult{}); err != nil {
		t.Fatal(err)
	}
	settingsPath := filepath.Join(home, ".config", "opencode", "opencode.json")
	first, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatal(err)
	}

	changed, err := installOpenCodeODDParityAgents(settingsPath)
	if err != nil {
		t.Fatal(err)
	}
	if changed {
		t.Fatal("re-running the parity agent install step reported a change on an already-installed config")
	}
	second, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(first) != string(second) {
		t.Fatalf("re-running parity agent install changed opencode.json:\nfirst:\n%s\nsecond:\n%s", first, second)
	}
}

func TestOpenCodeUpgradeAndUninstallLegacyHiddenODDAgents(t *testing.T) {
	oddNames := []string{"gentle-ai-explore", "gentle-ai-verify", "gentle-ai-worker"}

	setLegacyHidden := func(t *testing.T, settingsPath string, withModel bool) {
		t.Helper()
		raw, err := os.ReadFile(settingsPath)
		if err != nil {
			t.Fatal(err)
		}
		var root map[string]any
		if err := json.Unmarshal(raw, &root); err != nil {
			t.Fatal(err)
		}
		agents, _ := root["agent"].(map[string]any)
		for _, name := range oddNames {
			entry, ok := agents[name].(map[string]any)
			if !ok {
				t.Fatalf("missing agent %q in %s", name, settingsPath)
			}
			entry["hidden"] = true
			if withModel && name == "gentle-ai-explore" {
				entry["model"] = "anthropic/claude-sonnet-4"
			}
		}
		encoded, err := json.MarshalIndent(root, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(settingsPath, append(encoded, '\n'), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	t.Run("install and sync converge legacy hidden ODD trio to false", func(t *testing.T) {
		for _, command := range []string{"install", "sync"} {
			t.Run(command, func(t *testing.T) {
				home := installTestHome(t)
				t.Setenv("HOME", home)
				t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
				stubOpenCodeRuntimeVersion(t, home, "2.0.23")

				if _, err := RunInstall([]string{"--agent", "opencode", "--component", "persona"}, system.DetectionResult{}); err != nil {
					t.Fatal(err)
				}
				settingsPath := filepath.Join(home, ".config", "opencode", "opencode.json")
				setLegacyHidden(t, settingsPath, true)

				if command == "install" {
					if _, err := RunInstall([]string{"--agent", "opencode", "--component", "persona"}, system.DetectionResult{}); err != nil {
						t.Fatal(err)
					}
				} else {
					if _, err := RunSync([]string{"--agent", "opencode"}); err != nil {
						t.Fatal(err)
					}
				}

				raw, err := os.ReadFile(settingsPath)
				if err != nil {
					t.Fatal(err)
				}
				var config struct {
					Agent map[string]map[string]any `json:"agent"`
				}
				if err := json.Unmarshal(raw, &config); err != nil {
					t.Fatalf("decode opencode.json: %v", err)
				}
				for _, name := range openCodeParityAgentNames {
					entry, ok := config.Agent[name]
					if !ok {
						t.Fatalf("%s missing parity agent %q", command, name)
					}
					wantHidden := !strings.HasPrefix(name, "gentle-ai-")
					if entry["hidden"] != wantHidden {
						t.Fatalf("%s agent %q hidden = %#v, want %v", command, name, entry["hidden"], wantHidden)
					}
				}
				if got := config.Agent["gentle-ai-explore"]["model"]; got != "anthropic/claude-sonnet-4" {
					t.Fatalf("%s did not preserve gentle-ai-explore model = %#v, want %q", command, got, "anthropic/claude-sonnet-4")
				}
			})
		}
	})

	t.Run("uninstall removes legacy hidden ODD trio", func(t *testing.T) {
		home := installTestHome(t)
		t.Setenv("HOME", home)
		t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
		stubOpenCodeRuntimeVersion(t, home, "2.0.23")

		if _, err := RunInstall([]string{"--agent", "opencode", "--component", "persona"}, system.DetectionResult{}); err != nil {
			t.Fatal(err)
		}
		settingsPath := filepath.Join(home, ".config", "opencode", "opencode.json")
		setLegacyHidden(t, settingsPath, false)

		if _, err := RunUninstall([]string{"--agent", "opencode", "--yes"}, &bytes.Buffer{}); err != nil {
			t.Fatal(err)
		}
		raw, err := os.ReadFile(settingsPath)
		if err != nil {
			t.Fatal(err)
		}
		var root map[string]any
		if err := json.Unmarshal(raw, &root); err != nil {
			t.Fatal(err)
		}
		agents, _ := root["agent"].(map[string]any)
		for _, name := range oddNames {
			if _, stillPresent := agents[name]; stillPresent {
				t.Fatalf("legacy hidden ODD agent %q was not removed by uninstall: %#v", name, agents[name])
			}
		}
	})
}
