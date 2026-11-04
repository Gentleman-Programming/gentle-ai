package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v4/internal/components/filemerge"
	"github.com/gentleman-programming/gentle-ai/v4/internal/model"
)

// TestOpenCodeWritersEmitNativeV2OnNativeDocuments proves the routing writers
// stop writing legacy V1 keys (`agent`, `permission`) once the settings
// document is native OpenCode V2: managed content lands in `agents` entries
// with `system`/`permissions`, and rule precedence follows the same byte order
// the V1 object evaluator used.
func TestOpenCodeWritersEmitNativeV2OnNativeDocuments(t *testing.T) {
	settingsPath := filepath.Join(t.TempDir(), "opencode.jsonc")
	seed := `{
  "$schema": "https://opencode.ai/config.json",
  "providers": {"zai": {"settings": {"baseURL": "https://example.invalid"}}},
  "agents": {"general": {"description": "user-owned agent", "mode": "subagent"}}
}`
	if err := os.WriteFile(settingsPath, []byte(seed), 0o644); err != nil {
		t.Fatal(err)
	}

	if changed, err := installOpenCodeODDParityAgents(settingsPath); err != nil {
		t.Fatalf("parity agents: %v", err)
	} else if !changed {
		t.Fatal("parity agents did not change the file")
	}
	if changed, err := installOpenCodeReviewProviderRoles(settingsPath, model.AgentOpenCode); err != nil {
		t.Fatalf("provider roles: %v", err)
	} else if !changed {
		t.Fatal("provider roles did not change the file")
	}

	raw, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatal(err)
	}
	root, err := filemerge.UnmarshalJSONObject(raw)
	if err != nil {
		t.Fatal(err)
	}

	for _, legacy := range []string{"permission"} {
		if _, exists := root[legacy]; exists {
			t.Fatalf("legacy top-level %q key resurrected on a native V2 document", legacy)
		}
	}
	// Parity agents, refuter and validator are managed wholesale, so they are
	// emitted as native `agents` entries; the orchestrator delegation grants
	// stay in the legacy `agent` map by design (#4816) and must not carry any
	// of the managed wholesale entries.
	agents, _ := root["agents"].(map[string]any)
	if agents == nil {
		t.Fatal("native agents map missing")
	}
	if _, ok := agents["general"]; !ok {
		t.Fatal("pre-existing user-owned native agent was dropped")
	}
	for _, name := range []string{"gentle-ai-explore", "review-risk", "review-refuter", "review-validator"} {
		entry, _ := agents[name].(map[string]any)
		if entry == nil {
			t.Fatalf("native agent %q missing", name)
		}
		if _, ok := entry["prompt"]; ok {
			t.Fatalf("agent %q uses the legacy prompt field", name)
		}
		if _, ok := entry["system"]; !ok {
			t.Fatalf("agent %q missing native system field", name)
		}
		if _, ok := entry["permissions"].([]any); !ok {
			t.Fatalf("agent %q missing native permissions list", name)
		}
	}
	legacyAgents, _ := root["agent"].(map[string]any)
	if legacyAgents == nil {
		t.Fatal("legacy agent map missing: orchestrator delegation grants must land there (#4816)")
	}
	for name := range legacyAgents {
		if name != "gentle-orchestrator" {
			t.Fatalf("legacy agent map carries managed wholesale entry %q", name)
		}
	}
	legacyOrchestrator, _ := legacyAgents["gentle-orchestrator"].(map[string]any)
	legacyPermission, _ := legacyOrchestrator["permission"].(map[string]any)
	legacyTask, _ := legacyPermission["task"].(map[string]any)
	if legacyTask["*"] != "deny" {
		t.Fatalf("orchestrator wildcard subagent deny missing: %v", legacyPermission)
	}
	if legacyTask["review-refuter"] != "allow" {
		t.Fatalf("orchestrator refuter grant missing: %v", legacyPermission)
	}
	if legacyPermission["question"] != "allow" {
		t.Fatalf("orchestrator question allowance missing: %v", legacyPermission)
	}
}
