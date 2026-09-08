package assets

import (
	"strings"
	"testing"
)

// #2130: a workspace-scope install writes the lazy SDD workflow under the
// workspace's .claude, so every pointer names that location first, then home.
func TestClaudeWorkflowPointersNameWorkspaceAndHomeLocations(t *testing.T) {
	for _, asset := range []string{"claude/commands/gentle-sdd-new.md", "claude/commands/gentle-sdd-continue.md", "claude/commands/gentle-sdd-ff.md", "claude/sdd-orchestrator.md"} {
		content := MustRead(asset)
		workspace := strings.Index(content, "`.claude/skills/_shared/sdd-orchestrator-workflow.md`")
		home := strings.Index(content, "`~/.claude/skills/_shared/sdd-orchestrator-workflow.md`")
		if workspace < 0 || home < 0 || workspace > home {
			t.Errorf("%s must name the workspace-scope workflow path before the home-scope one (workspace=%d home=%d)", asset, workspace, home)
		}
	}
}

func TestClaudeMetaCommandsDelegatePolicy(t *testing.T) {
	for _, name := range []string{"new", "continue", "ff"} {
		t.Run(name, func(t *testing.T) {
			content := MustRead("claude/commands/gentle-sdd-" + name + ".md")
			for _, required := range []string{"description:", "Intent:", "$ARGUMENTS", "under the workspace first; if absent", "authoritative lazy workflow"} {
				if !strings.Contains(content, required) {
					t.Errorf("entrypoint missing %q", required)
				}
			}
			for _, forbidden := range []string{"WORKFLOW:", "STATUS CONTRACT:", "preflight", "sdd-init", "sdd-propose", "sdd-spec", "sdd-apply", "nextRecommended", "blockedReasons", "artifact store", "review", "400", "mem_search", "AskUserQuestion", "`interactive`", "`auto`"} {
				if strings.Contains(content, forbidden) {
					t.Errorf("entrypoint duplicates policy %q", forbidden)
				}
			}
		})
	}
}

// #2480: regenerating tasks.md must keep the existing list and numbering.
func TestTasksSkillPreservesExistingTaskListOnRegeneration(t *testing.T) {
	if content := MustRead("skills/sdd-tasks/SKILL.md"); !strings.Contains(content, "never rewrite") || !strings.Contains(content, "numbering") {
		t.Fatal("skills/sdd-tasks/SKILL.md must say regeneration preserves the existing task list and numbering, never rewriting from scratch")
	}
}
