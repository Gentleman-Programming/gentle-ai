package legacyassets

import (
	"path/filepath"
	"reflect"
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

func TestLegacySubAgentPaths(t *testing.T) {
	names := []string{"sdd-init", "sdd-explore", "sdd-research", "sdd-propose", "sdd-spec", "sdd-design", "sdd-tasks", "sdd-apply", "sdd-verify", "sdd-archive", "sdd-onboard"}
	dir := filepath.Join(t.TempDir(), "agents")
	want := make([]string, 0, len(names))
	for _, name := range names {
		want = append(want, filepath.Join(dir, name+".md"))
	}
	if got := SubAgentPaths(model.AgentClaudeCode, dir); !reflect.DeepEqual(got, want) {
		t.Errorf("claude sub-agent paths = %v, want %v", got, want)
	}
	// Only Claude Code kept v3 SDD agents in a native agents directory.
	for _, agent := range []model.AgentID{model.AgentOpenCode, model.AgentKimi, model.AgentCursor} {
		if got := SubAgentPaths(agent, dir); got != nil {
			t.Errorf("%s sub-agent paths = %v, want nil", agent, got)
		}
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
