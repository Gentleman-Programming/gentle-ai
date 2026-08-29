package sdd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// #3814 / #3778 / #2698: SDD agent contracts hardcoded the plugin-scoped
// Engram tool prefix mcp__plugin_engram_engram__*. That prefix never resolves
// for an Engram registered through `claude mcp add engram`, so the agent was
// granted tools that do not exist and every phase returned an empty result
// with its persistence step unexecuted.
//
// The assets now carry {{ENGRAM_TOOL_PREFIX}} and injection expands it to BOTH
// known shapes, which is the resolution #2698 proposes. Probing the ambient
// config would be order-dependent: the Engram component registers the
// user-scope entry during the same sync that renders these agents.

func injectedAgent(t *testing.T, home, name string) string {
	t.Helper()
	content, err := os.ReadFile(filepath.Join(home, ".claude", "agents", name))
	if err != nil {
		t.Fatalf("ReadFile(%s) error = %v", name, err)
	}
	return string(content)
}

// TestInjectGrantsBothEngramToolShapes pins that every declared Engram tool
// reaches the agent under both registration routes.
func TestInjectGrantsBothEngramToolShapes(t *testing.T) {
	home := t.TempDir()
	if _, err := Inject(home, claudeAdapter(), ""); err != nil {
		t.Fatalf("Inject() error = %v", err)
	}

	text := injectedAgent(t, home, "sdd-apply.md")
	for _, tool := range []string{"mem_search", "mem_get_observation", "mem_save", "mem_update"} {
		for _, prefix := range []string{"mcp__engram__", "mcp__plugin_engram_engram__"} {
			if !strings.Contains(text, prefix+tool) {
				t.Errorf("sdd-apply is missing the %s%s grant", prefix, tool)
			}
		}
	}
}

// TestInjectLeavesNoRawEngramToolPlaceholder pins that the placeholder never
// reaches a rendered agent.
func TestInjectLeavesNoRawEngramToolPlaceholder(t *testing.T) {
	home := t.TempDir()
	if _, err := Inject(home, claudeAdapter(), ""); err != nil {
		t.Fatalf("Inject() error = %v", err)
	}

	agentsDir := filepath.Join(home, ".claude", "agents")
	entries, err := os.ReadDir(agentsDir)
	if err != nil {
		t.Fatalf("ReadDir() error = %v", err)
	}
	for _, entry := range entries {
		if strings.Contains(injectedAgent(t, home, entry.Name()), "{{ENGRAM_TOOL_PREFIX}}") {
			t.Errorf("agent %s kept the raw placeholder", entry.Name())
		}
	}
}

// TestInjectEngramToolExpansionIsAmbientIndependent pins the property that
// made the both-shapes expansion the right answer: the rendered agent must not
// depend on whether a user-scope Engram MCP entry happens to exist yet, because
// the same sync that renders these agents also writes that entry.
func TestInjectEngramToolExpansionIsAmbientIndependent(t *testing.T) {
	withoutEntry := t.TempDir()
	if _, err := Inject(withoutEntry, claudeAdapter(), ""); err != nil {
		t.Fatalf("Inject() error = %v", err)
	}

	withEntry := t.TempDir()
	if err := os.WriteFile(filepath.Join(withEntry, ".claude.json"), []byte(`{"mcpServers":{"engram":{"command":"engram"}}}`), 0o644); err != nil {
		t.Fatalf("seed user config: %v", err)
	}
	if _, err := Inject(withEntry, claudeAdapter(), ""); err != nil {
		t.Fatalf("Inject() error = %v", err)
	}

	if a, b := injectedAgent(t, withoutEntry, "sdd-apply.md"), injectedAgent(t, withEntry, "sdd-apply.md"); a != b {
		t.Error("rendered sdd-apply depends on ambient Engram MCP registration")
	}
}
