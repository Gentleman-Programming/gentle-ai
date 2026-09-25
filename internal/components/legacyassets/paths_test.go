package legacyassets

import (
	"path/filepath"
	"reflect"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v3/internal/model"
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
