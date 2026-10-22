package agentguidance_test

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v4/internal/agents"
	"github.com/gentleman-programming/gentle-ai/v4/internal/catalog"
	"github.com/gentleman-programming/gentle-ai/v4/internal/components/agentguidance"
	"github.com/gentleman-programming/gentle-ai/v4/internal/components/communitytool"
	"github.com/gentleman-programming/gentle-ai/v4/internal/components/engram"
	"github.com/gentleman-programming/gentle-ai/v4/internal/components/persona"
	"github.com/gentleman-programming/gentle-ai/v4/internal/components/reviewassets"
	"github.com/gentleman-programming/gentle-ai/v4/internal/model"
)

// retiredWorkflowReference matches SDD and OpenSpec, both retired in v4.0.0.
var retiredWorkflowReference = regexp.MustCompile(`(?i)sdd|openspec`)

// TestRenderedGuidanceHasNoRetiredWorkflowReferences is a ratchet over what
// Gentle AI writes into each runtime: guidance and orchestrator blocks, strict
// TDD, personas and output styles, Engram protocol, CodeGraph guidance, and
// native review/Judgment Day agents. Every scan starts from an empty home, so
// any match was rendered by Gentle AI itself. There is no allowlist: retirement
// inventories live in Go cleanup code and are never rendered.
func TestRenderedGuidanceHasNoRetiredWorkflowReferences(t *testing.T) {
	engram.SetLookPathForTest(t, "/usr/local/bin/engram", "")
	codeGraph := communitytool.CodeGraphGuidanceMarkdown()
	assertNoRetiredWorkflowReference(t, "CodeGraphGuidanceMarkdown", codeGraph)

	for _, agent := range catalog.AllAgents() {
		if agent.ID == model.AgentConductor {
			continue // Catalog-only: no standalone guidance target.
		}
		t.Run(string(agent.ID), func(t *testing.T) {
			adapter, err := agents.NewAdapter(agent.ID)
			if err != nil {
				t.Fatal(err)
			}
			for _, capability := range []string{"", "small"} {
				options := agentguidance.RoutingOptions{
					OrchestratorCapability: capability,
					// Upgraded Codex installs still carry legacy carril keys.
					CodexCarrilModelAssignments: map[string]string{"sdd-cheap": "gpt-cheap", "sdd-mid": "gpt-mid", "sdd-strong": "gpt-strong"},
				}
				home := t.TempDir()
				routing, err := agentguidance.InjectRoutingWithOptions(home, agent.ID, options)
				if err != nil {
					t.Fatalf("InjectRoutingWithOptions(capability=%q): %v", capability, err)
				}
				tdd, err := agentguidance.InjectStrictTDDWithOptions(home, agent.ID, true, options)
				if err != nil {
					t.Fatalf("InjectStrictTDDWithOptions(capability=%q): %v", capability, err)
				}
				files := append(routing.Files, tdd.Files...)
				if agent.ID == model.AgentOpenCode || agent.ID == model.AgentKilocode {
					background := 0
					for _, path := range routing.Files {
						if filepath.Ext(path) != ".json" {
							continue
						}
						policy, err := agentguidance.ApplyOpenCodeBackgroundPolicy(path, true)
						if err != nil {
							t.Fatalf("ApplyOpenCodeBackgroundPolicy(%s): %v", path, err)
						}
						background += len(policy.Files)
						files = append(files, policy.Files...)
					}
					if agent.ID == model.AgentOpenCode && background == 0 {
						t.Fatal("OpenCode background subagent policy was not rendered")
					}
				}
				assertRenderedFilesClean(t, files)
				if agent.ID != model.AgentPi {
					rendered, err := agentguidance.RenderOrchestratorWithSource(agent.ID, reviewassets.ReviewExecutionContractFor, capability)
					if err != nil {
						t.Fatalf("RenderOrchestratorWithSource(capability=%q): %v", capability, err)
					}
					assertNoRetiredWorkflowReference(t, "orchestrator capability="+capability, rendered)
					// Shared-section markers bind the composer only; they never ship.
					if strings.Contains(rendered, "orchestrator-section:") {
						t.Errorf("orchestrator capability=%q leaks a shared-section marker", capability)
					}
				}
			}

			for _, selected := range []model.PersonaID{model.PersonaGentleman, model.PersonaNeutral} {
				home := t.TempDir()
				t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
				result, err := persona.Inject(home, adapter, selected)
				if err != nil {
					t.Fatalf("persona.Inject(%s): %v", selected, err)
				}
				assertRenderedFilesClean(t, result.Files)
			}

			for _, version := range []string{"", "engram 1.18.0"} {
				home := t.TempDir()
				t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
				result, err := engram.InjectWithOptions(home, adapter, engram.InjectOptions{Version: version})
				if err != nil {
					t.Fatalf("engram.InjectWithOptions(version=%q): %v", version, err)
				}
				assertRenderedFilesClean(t, result.Files)
			}

			if reviewassets.NativeAgentsSupported(agent.ID) {
				home := t.TempDir()
				result, err := reviewassets.InstallNativeAgents(home, adapter, reviewassets.InstallOptions{CodeGraphGuidanceMarkdown: codeGraph})
				if err != nil {
					t.Fatalf("InstallNativeAgents: %v", err)
				}
				assertRenderedFilesClean(t, result.Files)
			}
		})
	}

	t.Run("codegraph-guidance-files", func(t *testing.T) {
		home := t.TempDir()
		result, err := communitytool.InjectCodeGraphGuidance(home)
		if err != nil {
			t.Fatal(err)
		}
		assertRenderedFilesClean(t, result.Files)
	})
}

func assertRenderedFilesClean(t *testing.T, paths []string) {
	t.Helper()
	for _, path := range paths {
		content, err := os.ReadFile(path)
		if os.IsNotExist(err) {
			continue // Reported paths can include cleanup targets.
		}
		if err != nil {
			t.Fatalf("read rendered %s: %v", path, err)
		}
		assertNoRetiredWorkflowReference(t, path, string(content))
	}
}

func assertNoRetiredWorkflowReference(t *testing.T, source, content string) {
	t.Helper()
	for index, line := range strings.Split(content, "\n") {
		if retiredWorkflowReference.MatchString(line) {
			t.Errorf("%s:%d renders retired workflow reference: %s", source, index+1, strings.TrimSpace(line))
		}
	}
}
