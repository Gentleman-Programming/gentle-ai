package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v4/internal/agents"
	"github.com/gentleman-programming/gentle-ai/v4/internal/assets"
	"github.com/gentleman-programming/gentle-ai/v4/internal/catalog"
	"github.com/gentleman-programming/gentle-ai/v4/internal/components/agentguidance"
	"github.com/gentleman-programming/gentle-ai/v4/internal/components/opencodedefault"
	"github.com/gentleman-programming/gentle-ai/v4/internal/components/reviewassets"
	"github.com/gentleman-programming/gentle-ai/v4/internal/model"
)

const (
	orchestratorOpenMarker       = "<!-- gentle-ai:orchestrator -->"
	legacyOrchestratorOpenMarker = "<!-- gentle-ai:sdd-orchestrator -->"
)

// orchestratorRuntimesForInstall lists every runtime whose installed prompt
// carries the orchestrator; Pi's prompt belongs to the Gentle Shell package.
func orchestratorRuntimesForInstall(t *testing.T) []model.AgentID {
	t.Helper()

	var selected []model.AgentID
	for _, agent := range catalog.AllAgents() {
		if agent.ID == model.AgentPi {
			continue
		}
		adapter, err := agents.NewAdapter(agent.ID)
		if err != nil {
			t.Fatalf("NewAdapter(%q) error = %v", agent.ID, err)
		}
		if adapter.SupportsSystemPrompt() {
			selected = append(selected, agent.ID)
		}
	}
	return selected
}

// installedGuidance returns the path the agent loads guidance from and the
// decoded guidance text inside it.
func installedGuidance(t *testing.T, home string, agent model.AgentID) (string, string) {
	t.Helper()

	adapter, err := agents.NewAdapter(agent)
	if err != nil {
		t.Fatalf("NewAdapter(%q) error = %v", agent, err)
	}
	paths, err := agentguidance.RoutingPathsWithOptions(home, agent, routingGuidanceOptions(home, "", adapter))
	if err != nil || len(paths) == 0 {
		t.Fatalf("RoutingPathsWithOptions(%q) = %v, %v", agent, paths, err)
	}
	raw := readTextFile(t, paths[0])
	if !strings.HasSuffix(paths[0], ".json") {
		return paths[0], raw
	}
	var settings struct {
		Agent map[string]struct {
			Prompt string `json:"prompt"`
		} `json:"agent"`
	}
	if err := json.Unmarshal([]byte(raw), &settings); err != nil {
		t.Fatalf("decode %q error = %v", paths[0], err)
	}
	return paths[0], settings.Agent[opencodedefault.ManagedAgent].Prompt
}

// reconstructCodexInstalledGuidance resolves every installed Codex core
// pointer through the exact module path and section it names. It fails on an
// unreachable pointer, a missing or duplicate section, or an installed module
// that the core never references.
func reconstructCodexInstalledGuidance(t *testing.T, home, core string) string {
	t.Helper()
	paths, err := agentguidance.CodexGlobalModulePaths(home)
	if err != nil || len(paths) < 3 {
		t.Fatalf("CodexGlobalModulePaths() = %v, %v", paths, err)
	}
	known := map[string]bool{}
	for _, path := range paths[1 : len(paths)-1] {
		known[path] = true
	}

	reconstructed := core
	used := map[string]bool{}
	usedPath := map[string]bool{}
	for _, line := range strings.SplitAfter(core, "\n") {
		pathStart := strings.Index(line, "read `")
		sectionStart := strings.Index(line, ", section \"")
		if !strings.HasPrefix(line, "On demand: when ") || pathStart < 0 || sectionStart < 0 {
			continue
		}
		pathStart += len("read `")
		pathEnd := strings.Index(line[pathStart:], "`")
		if pathEnd < 0 {
			t.Fatalf("malformed Codex module pointer %q", line)
		}
		modulePath := line[pathStart : pathStart+pathEnd]
		sectionStart += len(", section \"")
		sectionEnd := strings.Index(line[sectionStart:], "\"")
		if sectionEnd < 0 || !known[modulePath] {
			t.Fatalf("unreachable Codex module pointer %q", line)
		}
		title := line[sectionStart : sectionStart+sectionEnd]
		key := modulePath + "\x00" + title
		if used[key] {
			t.Fatalf("duplicate Codex module pointer for %s section %q", modulePath, title)
		}
		used[key] = true
		usedPath[modulePath] = true

		module := readTextFile(t, modulePath)
		heading := "\n## " + title + "\n\n"
		if strings.Count(module, heading) != 1 {
			t.Fatalf("%s carries section %q %d times, want 1", modulePath, title, strings.Count(module, heading))
		}
		body := strings.SplitN(module, heading, 2)[1]
		if next := strings.Index(body, "\n## "); next >= 0 {
			body = body[:next]
		}
		reconstructed = strings.Replace(reconstructed, line, body, 1)
	}

	for path := range known {
		if _, err := os.Lstat(path); err == nil {
			if !usedPath[path] {
				t.Fatalf("installed Codex module %s has no reachable core pointer", path)
			}
		} else if !os.IsNotExist(err) {
			t.Fatalf("stat Codex module %s: %v", path, err)
		}
	}
	if strings.Contains(reconstructed, "read `"+filepath.Join(filepath.Dir(paths[len(paths)-1]), "orchestrator-")) {
		t.Fatal("reconstructed Codex guidance still carries a module pointer")
	}
	return reconstructed
}

func TestInstallAndSyncSelectGenericOrchestratorVariant(t *testing.T) {
	asset, err := assets.Read("generic/orchestrator.md")
	if err != nil {
		t.Fatal(err)
	}
	for _, agent := range []model.AgentID{model.AgentVSCodeCopilot, model.AgentOpenClaw, model.AgentTrae} {
		for _, route := range []string{"fresh", "install-upgrade", "sync-upgrade"} {
			t.Run(string(agent)+"/"+route, func(t *testing.T) {
				home := t.TempDir()
				selection := model.Selection{Agents: []model.AgentID{agent}}
				if route != "fresh" {
					path := systemPromptFileFor(t, home, agent)
					legacy := "User prefix\n" + legacyOrchestratorOpenMarker + "\n" + asset + "\n<!-- /gentle-ai:sdd-orchestrator -->\nUser suffix\n"
					mustWriteFile(t, path, []byte(legacy))
				}
				if route == "sync-upgrade" {
					runSyncInjectionSteps(t, home, selection)
				} else {
					runInstallInjectionSteps(t, newTestInstallRuntime(t, home, selection))
				}
				path, prompt := installedGuidance(t, home, agent)
				if strings.Count(prompt, "# Agent Teams Lite — Orchestrator Instructions") != 1 || strings.Contains(prompt, "Orchestrator Instructions (Small Model)") || strings.Contains(prompt, "section:model-") {
					t.Fatal("installed carrier must contain only the default capable variant")
				}
				for _, marker := range []string{"Lossless Blocking Prompts", "Delegated Verification Gate", "Sub-Agent Launch Deduplication", "Skill Resolution Feedback", "Sub-Agent Context Protocol", "Remote operation authorization"} {
					if !strings.Contains(prompt, marker) {
						t.Errorf("installed carrier lost %q", marker)
					}
				}
				if route != "fresh" && (!strings.HasPrefix(prompt, "User prefix\n") || !strings.Contains(prompt, "\nUser suffix\n")) {
					t.Fatal("upgrade changed user bytes")
				}
				installed := readTextFile(t, path)
				runInstallInjectionSteps(t, newTestInstallRuntime(t, home, selection))
				if readTextFile(t, path) != installed {
					t.Fatal("repeat install changed carrier")
				}
				runSyncInjectionSteps(t, home, selection)
				if readTextFile(t, path) != installed {
					t.Fatal("repeat sync changed carrier")
				}
			})
		}
	}
}

func TestInstallAndSyncDeliverOrchestratorOnceForEveryRuntime(t *testing.T) {
	for _, agent := range orchestratorRuntimesForInstall(t) {
		t.Run(string(agent), func(t *testing.T) {
			home := t.TempDir()
			selection := model.Selection{
				Agents:     []model.AgentID{agent},
				Components: []model.ComponentID{model.ComponentPersona},
				Persona:    model.PersonaGentleman,
			}

			runInstallInjectionSteps(t, newTestInstallRuntime(t, home, selection))
			path, prompt := installedGuidance(t, home, agent)
			installed := readTextFile(t, path)

			rendered, err := agentguidance.RenderOrchestratorWithSource(agent, reviewassets.ReviewExecutionContractFor, "")
			if err != nil {
				t.Fatalf("RenderOrchestrator(%q) error = %v", agent, err)
			}
			guidance := prompt
			if agent == model.AgentCodex {
				guidance = reconstructCodexInstalledGuidance(t, home, prompt)
			}
			if got := strings.Count(guidance, strings.TrimSpace(rendered)); got != 1 {
				t.Fatalf("installed prompt carries the orchestrator %d times, want 1:\n%s", got, prompt)
			}
			if got := strings.Count(prompt, orchestratorOpenMarker); got != 1 {
				t.Fatalf("orchestrator marker count = %d, want 1", got)
			}
			if strings.Count("\n"+prompt, "\n### ODD protocol") != 1 {
				t.Fatal("installed prompt must carry the ODD protocol exactly once")
			}
			if strings.Index(prompt, orchestratorOpenMarker) > strings.Index(prompt, routingOpenMarker) {
				t.Fatal("orchestrator block must precede the routing block")
			}
			if strings.Contains(prompt, legacyOrchestratorOpenMarker) {
				t.Fatal("install wrote the retired sdd-orchestrator marker")
			}
			if !strings.Contains(prompt, "Use at most one scoped independent read-only assumption challenge") {
				t.Fatal("installed prompt lost the ODD assumption challenge")
			}
			// Receipt-driven development reaches only its runtimes.
			rdd := model.SupportsReceiptDrivenDevelopment(agent)
			for _, marker := range []string{
				"Native Compact Review Orchestration",
				"Gentle AI Provider Defect Handoff",
				"Receipt-driven development is user-owned",
				"gentle-ai review mode enable|disable|status",
				"The native RDD refuter owns native review claims",
			} {
				if got := strings.Contains(prompt, marker); got != rdd {
					t.Errorf("installed prompt carries %q = %v, want %v", marker, got, rdd)
				}
			}
			if !rdd {
				for _, forbidden := range []string{"gentle-ai review", "RDD", "receipt", "refuter", "native review"} {
					if strings.Contains(prompt, forbidden) {
						t.Errorf("non-RDD runtime prompt carries %q", forbidden)
					}
				}
			}

			runInstallInjectionSteps(t, newTestInstallRuntime(t, home, selection))
			if got := readTextFile(t, path); got != installed {
				t.Fatalf("second install changed %s:\n%s", path, got)
			}
			runSyncInjectionSteps(t, home, selection)
			if got := readTextFile(t, path); got != installed {
				t.Fatalf("sync after install changed %s:\n%s", path, got)
			}
			runSyncInjectionSteps(t, home, selection)
			if got := readTextFile(t, path); got != installed {
				t.Fatalf("second sync changed %s:\n%s", path, got)
			}
		})
	}
}

// A v3.7.0 CLAUDE.md carried the orchestrator under the retired SDD marker.
// Upgrading must leave exactly one orchestrator block and every user byte.
func TestInstallUpgradesV370OrchestratorBlockAndKeepsUserText(t *testing.T) {
	for _, run := range []struct {
		name  string
		apply func(t *testing.T, home string, selection model.Selection)
	}{
		{"install", func(t *testing.T, home string, selection model.Selection) {
			runInstallInjectionSteps(t, newTestInstallRuntime(t, home, selection))
		}},
		{"sync", func(t *testing.T, home string, selection model.Selection) {
			runSyncInjectionSteps(t, home, selection)
		}},
	} {
		t.Run(run.name, func(t *testing.T) {
			home := t.TempDir()
			promptPath := systemPromptFileFor(t, home, model.AgentClaudeCode)
			legacy := "# My own rules\n\nAlways answer briefly.\n\n" +
				legacyOrchestratorOpenMarker + "\n# Agent Teams Lite — Orchestrator Instructions\n\n### SDD Workflow\n\nArtifact Store Mode\n<!-- /gentle-ai:sdd-orchestrator -->\n\n" +
				"<!-- gentle-ai:agent-routing -->\nstale routing\n<!-- /gentle-ai:agent-routing -->\n\n" +
				"User footer.\n"
			mustWriteFile(t, promptPath, []byte(legacy))
			selection := model.Selection{Agents: []model.AgentID{model.AgentClaudeCode}}

			run.apply(t, home, selection)
			got := readTextFile(t, promptPath)

			if !strings.HasPrefix(got, "# My own rules\n\nAlways answer briefly.\n\n") || !strings.HasSuffix(got, "User footer.\n") {
				t.Fatalf("upgrade lost user text:\n%s", got)
			}
			if strings.Contains(got, legacyOrchestratorOpenMarker) || strings.Contains(got, "SDD Workflow") || strings.Contains(got, "Artifact Store Mode") {
				t.Fatalf("legacy SDD orchestrator survived the upgrade:\n%s", got)
			}
			if count := strings.Count(got, orchestratorOpenMarker); count != 1 {
				t.Fatalf("orchestrator block count = %d, want 1:\n%s", count, got)
			}
			for _, heading := range []string{"Lossless Blocking Prompts", "Delegated Verification Gate", "Native Checking Contract", "Native Compact Review Orchestration"} {
				if !strings.Contains(got, heading) {
					t.Errorf("upgraded prompt is missing %q", heading)
				}
			}

			run.apply(t, home, selection)
			if again := readTextFile(t, promptPath); again != got {
				t.Fatalf("second %s after upgrade changed the prompt:\n%s", run.name, again)
			}
		})
	}
}
