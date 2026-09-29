package agentguidance

import (
	"os"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v4/internal/agents"
	"github.com/gentleman-programming/gentle-ai/v4/internal/agents/capabilitymanifest"
	"github.com/gentleman-programming/gentle-ai/v4/internal/catalog"
	"github.com/gentleman-programming/gentle-ai/v4/internal/model"
)

// TestInjectRoutingNoOpsForCatalogOnlyConductor makes the Conductor product
// contract explicit rather than a skipped test: Conductor is detection and
// catalog only, and its workspaces inherit Claude Code configuration, so
// install/sync must neither write standalone guidance for it nor report a
// path for the backup snapshot. Failing closed like an unknown delivery would
// break every Conductor install; silently writing one would violate the
// no-Conductor-specific-files contract.
func TestInjectRoutingNoOpsForCatalogOnlyConductor(t *testing.T) {
	t.Parallel()

	targetDir := t.TempDir()

	paths, err := RoutingPaths(targetDir, model.AgentConductor)
	if err != nil {
		t.Fatalf("RoutingPaths(%q) error = %v", model.AgentConductor, err)
	}
	if len(paths) != 0 {
		t.Fatalf("RoutingPaths(%q) = %v, want no guidance target", model.AgentConductor, paths)
	}

	result, err := InjectRoutingWithOptions(targetDir, model.AgentConductor, RoutingOptions{})
	if err != nil {
		t.Fatalf("InjectRouting(%q) error = %v", model.AgentConductor, err)
	}
	if result.Changed || len(result.Files) != 0 {
		t.Fatalf("InjectRouting(%q) = %+v, want a clean no-op", model.AgentConductor, result)
	}

	entries, err := os.ReadDir(targetDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("InjectRouting(%q) created %d entries under the target dir", model.AgentConductor, len(entries))
	}
}

// TestConductorIsTheOnlyCatalogOnlyGuidanceTarget guards every test loop that
// skips Conductor: the capability manifest is the canonical claim that
// Conductor has no managed system prompt, so this fails if another writable
// agent loses that claim (and the guidance skips above silently under-test it)
// or if Conductor ever grows a writable prompt surface.
func TestConductorIsTheOnlyCatalogOnlyGuidanceTarget(t *testing.T) {
	t.Parallel()

	for _, agent := range catalog.AllAgents() {
		manifest := capabilitymanifest.MustForAgent(agent.ID)
		// Pi claims no managed system prompt feature but keeps its own
		// package-owned prompt delivery, so it is not catalog-only.
		catalogOnly := !manifest.Features.SystemPrompt && agent.ID != model.AgentPi
		if want := agent.ID == model.AgentConductor; catalogOnly != want {
			t.Fatalf("agent %q catalog-only = %v, want %v", agent.ID, catalogOnly, want)
		}

		adapter, err := agents.NewAdapter(agent.ID)
		if err != nil {
			t.Fatalf("NewAdapter(%q) error = %v", agent.ID, err)
		}
		if catalogOnly != (adapter.SystemPromptFile("/home") == "") {
			t.Fatalf("agent %q adapter prompt file disagrees with its catalog-only status", agent.ID)
		}
	}
}
