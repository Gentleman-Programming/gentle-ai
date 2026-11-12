package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v4/internal/components/filemerge"
	"github.com/gentleman-programming/gentle-ai/v4/internal/components/opencodeagentfiles"
	"github.com/gentleman-programming/gentle-ai/v4/internal/model"
)

// TestOpenCodeWritersEmitNativeV2OnNativeDocuments proves the routing writers
// stop writing legacy top-level keys on native OpenCode V2 documents: the
// wholesale-managed subagents ship as individual agent files under the global
// agents directory (their entries leave the settings `agents` map), the
// orchestrator delegation grants stay in the legacy `agent` map by design
// (#4816), and no managed wholesale entry is written into the settings.
func TestOpenCodeWritersEmitNativeV2OnNativeDocuments(t *testing.T) {
	home := t.TempDir()
	settingsPath := filepath.Join(home, ".config", "opencode", "opencode.jsonc")
	if err := os.MkdirAll(filepath.Dir(settingsPath), 0o755); err != nil {
		t.Fatal(err)
	}
	seed := `{
  "$schema": "https://opencode.ai/config.json",
  "providers": {"zai": {"settings": {"baseURL": "https://example.invalid"}}},
  "agents": {"general": {"description": "user-owned agent", "mode": "subagent"}}
}`
	if err := os.WriteFile(settingsPath, []byte(seed), 0o644); err != nil {
		t.Fatal(err)
	}

	if changed, err := installOpenCodeReviewProviderRoles(settingsPath, home, model.AgentOpenCode); err != nil {
		t.Fatalf("review provider roles: %v", err)
	} else if !changed {
		t.Fatal("review provider roles did not change the settings")
	}
	// The parity emitter's change lands on the agents directory (files), not
	// on the settings document: its bool reports settings changes only.
	if _, err := installOpenCodeODDParityAgents(settingsPath, home); err != nil {
		t.Fatalf("parity agents: %v", err)
	}

	raw, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatal(err)
	}
	root, err := filemerge.UnmarshalJSONObject(raw)
	if err != nil {
		t.Fatal(err)
	}

	if _, exists := root["permission"]; exists {
		t.Fatalf("legacy top-level %q key resurrected on a native V2 document", "permission")
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

	agents, _ := root["agents"].(map[string]any)
	if agents == nil {
		t.Fatal("native agents map missing")
	}
	if _, ok := agents["general"]; !ok {
		t.Fatal("pre-existing user-owned native agent was dropped")
	}
	for _, name := range opencodeagentfiles.Manifest {
		if _, ok := agents[name]; ok {
			t.Fatalf("managed subagent %q still present in the settings document; it must ship as an agent file", name)
		}
	}

	agentsDir := filepath.Join(home, ".config", "opencode", "agents")
	for _, name := range opencodeagentfiles.Manifest {
		data, err := os.ReadFile(filepath.Join(agentsDir, name+".md"))
		if err != nil {
			t.Fatalf("agent file %s: %v", name, err)
		}
		content := string(data)
		if !strings.HasPrefix(content, "---\n") {
			t.Fatalf("agent file %s does not open with frontmatter", name)
		}
		for _, want := range []string{"mode: subagent", "hidden: true", "permissions:"} {
			if !strings.Contains(content, want) {
				t.Fatalf("agent file %s misses expected frontmatter field %q", name, want)
			}
		}
	}
	// The validator's targeted-validation allow must survive its wildcard
	// deny: rules are emitted in resource byte order, so "*" precedes the
	// specific grant and the last matching rule keeps the grant effective.
	validator, err := os.ReadFile(filepath.Join(agentsDir, "review-validator.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(validator), `resource: "gentle-ai review inspect-candidate --purpose targeted-validation *"`) || !strings.Contains(string(validator), `effect: "allow"`) {
		t.Fatalf("validator targeted-validation grant lost or misordered")
	}
	if !strings.Contains(string(validator), `effect: "deny"`) {
		t.Fatalf("validator wildcard deny lost")
	}
	// The user's own agent file under a managed name is preserved on later
	// runs: overwrite it and re-install.
	userFile := filepath.Join(agentsDir, "review-risk.md")
	if err := os.WriteFile(userFile, []byte("---\ndescription: user agent\n---\nMine.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if changed, err := installOpenCodeODDParityAgents(settingsPath, home); err != nil {
		t.Fatalf("second parity install: %v", err)
	} else if changed {
		t.Fatal("second parity install changed the settings although the managed entries were already migrated")
	}
	kept, err := os.ReadFile(userFile)
	if err != nil {
		t.Fatal(err)
	}
	if string(kept) != "---\ndescription: user agent\n---\nMine.\n" {
		t.Fatalf("user-authored agent file was overwritten: %s", kept)
	}
}
