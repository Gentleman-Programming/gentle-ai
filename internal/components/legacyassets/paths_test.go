package legacyassets

import (
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v4/internal/model"
)

func TestLegacyCommandPaths(t *testing.T) {
	names := []string{"sdd-init", "sdd-new", "sdd-continue", "sdd-status", "sdd-explore", "sdd-research", "sdd-ff", "sdd-apply", "sdd-verify", "sdd-archive", "sdd-onboard"}
	for _, tc := range []struct {
		agent  model.AgentID
		prefix string
		legacy bool
	}{{model.AgentClaudeCode, "gentle-", true}, {model.AgentOpenCode, "", false}} {
		dir := filepath.Join(t.TempDir(), "commands")
		want := []string{}
		for _, name := range names {
			want = append(want, filepath.Join(dir, tc.prefix+name+".md"))
			if tc.legacy {
				want = append(want, filepath.Join(dir, name+".md"))
			}
		}
		if got := SlashCommandPaths(tc.agent, dir); !reflect.DeepEqual(got, want) {
			t.Errorf("%s paths = %v, want %v", tc.agent, got, want)
		}
	}
}

// TestLegacySubAgentsPairEveryNameWithItsTemplateMarker pins the ownership
// evidence: each retired name must carry the v3 sentence, and the two lists must
// never drift apart.
func TestLegacySubAgentsPairEveryNameWithItsTemplateMarker(t *testing.T) {
	agents := SubAgents()
	if len(agents) != 11 {
		t.Fatalf("inventory has %d agents, want 11", len(agents))
	}
	seen := make(map[string]bool, len(agents))
	for _, agent := range agents {
		if agent.Name == "" || agent.Marker == "" {
			t.Errorf("agent %+v is missing a name or marker", agent)
		}
		if seen[agent.Name] {
			t.Errorf("duplicate inventory name %q", agent.Name)
		}
		seen[agent.Name] = true
		// sdd-research is the one v3 template that never used the phase banner.
		if agent.Name == "sdd-research" {
			continue
		}
		if want := "You are the SDD **" + strings.TrimPrefix(agent.Name, "sdd-") + "** executor."; agent.Marker != want {
			t.Errorf("%s marker = %q, want %q", agent.Name, agent.Marker, want)
		}
	}
	// Mutating the copy must not reach the package inventory.
	agents[0].Name = "mutated"
	if SubAgents()[0].Name == "mutated" {
		t.Error("SubAgents returned a shared slice")
	}
}

func TestRetiredManagedPath(t *testing.T) {
	root := t.TempDir()
	for _, tc := range []struct {
		relative string
		want     bool
	}{
		{".claude/commands/sdd-init.md", true},
		{".claude/commands/sdd-onboard.md", true},
		{".claude/commands/gentle-sdd-init.md", false},
		{".claude/commands/sdd-extra.md", false},
		{".other/commands/sdd-init.md", false},
		{".claude/skills/sdd-init.md", false},
	} {
		if got := IsLegacyClaudeCommandPath(filepath.Join(root, tc.relative)); got != tc.want {
			t.Errorf("%s: got %v, want %v", tc.relative, got, tc.want)
		}
	}
}
