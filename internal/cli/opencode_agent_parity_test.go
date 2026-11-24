package cli

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v5/internal/components/opencodeagents"
	"github.com/gentleman-programming/gentle-ai/v5/internal/model"
	"github.com/gentleman-programming/gentle-ai/v5/internal/system"
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
		if entry["mode"] != "subagent" || entry["hidden"] != true {
			t.Fatalf("parity agent %q missing mode/hidden: %#v", name, entry)
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

func TestOpenCodeFamilyWorkerUpgradeAndUninstallPreserveOwnership(t *testing.T) {
	const worker = "gentle-ai-worker"
	const judge = "jd-judge-a"
	makeEntry := func(name string) map[string]any {
		t.Helper()
		for _, spec := range opencodeagents.Parity(model.AgentOpenCode) {
			if spec.Name == name {
				entry, err := opencodeagents.Entry(spec)
				if err != nil {
					t.Fatal(err)
				}
				return entry
			}
		}
		t.Fatalf("missing parity role %s", name)
		return nil
	}
	currentPrompt := makeEntry(worker)["prompt"].(string)
	releasedPrompt := v5WorkerPrompt(t, currentPrompt)
	for _, tc := range []struct {
		agent     model.AgentID
		directory string
		state     string
		upgrade   bool
	}{
		{model.AgentOpenCode, "opencode", "absent", true},
		{model.AgentOpenCode, "opencode", "released", true},
		{model.AgentOpenCode, "opencode", "released", false},
		{model.AgentOpenCode, "opencode", "user-edited", true},
		{model.AgentKilocode, "kilo", "absent", true},
		{model.AgentKilocode, "kilo", "released", true},
		{model.AgentKilocode, "kilo", "user-edited", true},
	} {
		name := string(tc.agent) + "/" + tc.state
		if !tc.upgrade {
			name += "-direct-uninstall"
		}
		t.Run(name, func(t *testing.T) {
			home := installTestHome(t)
			t.Setenv("HOME", home)
			t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
			t.Chdir(home)
			path := filepath.Join(home, ".config", tc.directory, "opencode.json")
			if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
				t.Fatal(err)
			}
			judgeEntry := makeEntry(judge)
			judgePrompt := judgeEntry["prompt"]
			judgeEntry["prompt"] = "Refresh the existing parity role as before"
			judgeEntry["variant"] = "high"
			agents := map[string]map[string]any{judge: judgeEntry}
			seedPrompt := releasedPrompt
			if tc.state != "absent" {
				if tc.state == "user-edited" {
					seedPrompt += "\r\nMy worker instruction.  \n"
				}
				entry := makeEntry(worker)
				entry["prompt"] = seedPrompt
				entry["model"] = "local/worker"
				entry["variant"] = "high"
				agents[worker] = entry
			}
			var config struct {
				Agent map[string]map[string]any `json:"agent"`
				Theme string                    `json:"theme"`
			}
			config.Agent, config.Theme = agents, "my-theme"
			raw, err := json.Marshal(config)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, raw, 0o600); err != nil {
				t.Fatal(err)
			}
			readAgents := func(afterUpgrade bool) map[string]map[string]any {
				t.Helper()
				raw, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				config.Agent, config.Theme = nil, ""
				if err := json.Unmarshal(raw, &config); err != nil {
					t.Fatal(err)
				}
				if afterUpgrade && config.Theme != "my-theme" {
					t.Errorf("theme = %q, want unchanged", config.Theme)
				}
				return config.Agent
			}
			assertWorker := func(phase string, entry map[string]any, wantPresent bool, wantPrompt string) {
				t.Helper()
				if !wantPresent {
					if entry != nil {
						t.Errorf("%s: worker present; want absent", phase)
					}
					return
				}
				if entry["prompt"] != wantPrompt {
					t.Errorf("%s: worker prompt changed (present=%v); want exact prompt bytes", phase, entry != nil)
				}
				if tc.state != "absent" && (entry["model"] != "local/worker" || entry["variant"] != "high") {
					t.Errorf("%s: worker model or variant changed", phase)
				}
			}
			if tc.upgrade {
				if changed, err := installOpenCodeFamilyParityAgents(path, tc.agent); err != nil || !changed {
					t.Fatalf("parity upgrade: changed=%v, err=%v", changed, err)
				}
				upgraded := readAgents(true)
				managed := tc.agent == model.AgentOpenCode && tc.state != "user-edited"
				wantPrompt := seedPrompt
				if managed {
					wantPrompt = currentPrompt
				}
				assertWorker("upgrade", upgraded[worker], managed || tc.state != "absent", wantPrompt)
				if upgraded[judge]["prompt"] != judgePrompt || upgraded[judge]["variant"] != "high" {
					t.Error("judge parity refresh or user variant changed behavior")
				}
			}
			if result, err := RunUninstall([]string{"--agent", string(tc.agent), "--yes"}, io.Discard); err != nil || len(result.FailedAgents) != 0 {
				t.Fatalf("uninstall: failed agents=%v, err=%v", result.FailedAgents, err)
			}
			remaining := readAgents(false)
			preserved := tc.state == "user-edited" || (tc.agent == model.AgentKilocode && tc.state != "absent")
			assertWorker("uninstall", remaining[worker], preserved, seedPrompt)
			if tc.upgrade && remaining[judge] != nil {
				t.Error("uninstall retained the refreshed managed judge")
			}
		})
	}
}

// Reinsert the only changed v5.0.0 paragraph; pin the entire released prompt so
// changes elsewhere cannot accidentally turn this into a current-prompt test.
func v5WorkerPrompt(t *testing.T, current string) string {
	t.Helper()
	old, err := os.ReadFile(filepath.Join("..", "components", "opencodeagents", "testdata", "v5.0.0", "worker-test-discipline.md"))
	if err != nil {
		t.Fatal(err)
	}
	prefix, rest, found := strings.Cut(current, "Run focused tests first.")
	_, suffix, ended := strings.Cut(rest, "\n## Verification\n")
	if !found || !ended {
		t.Fatal("worker test discipline markers not found")
	}
	prompt := prefix + string(old) + "\n## Verification\n" + suffix
	const releasedSHA = "49533cb3262609334e14edf626d3041626ec642c733ecbf201b8570e660ea6ff"
	if got := fmt.Sprintf("%x", sha256.Sum256([]byte(prompt))); got != releasedSHA {
		t.Fatalf("worker fixture hash = %s, want v5.0.0 %s", got, releasedSHA)
	}
	return prompt
}
