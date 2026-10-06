package assets

import (
	"strings"
	"testing"
)

// TestOpenCodeDelegatedSkillsLoadByName pins #4457: installed skills live in
// the OpenCode config directory, outside the workspace, where a delegated
// agent's read needs external_directory approval it may not get. OpenCode's
// native `skill` tool loads an installed skill by name with only the `skill`
// permission, so the OpenCode orchestrator passes names, never absolute paths,
// and the worker loads names through that tool.
func TestOpenCodeDelegatedSkillsLoadByName(t *testing.T) {
	t.Parallel()

	for path, tc := range map[string]struct{ want, retired []string }{
		"opencode/orchestrator.md": {
			want: []string{
				"an installed skill (one listed in `<available_skills>`) by its name",
				"a skill file inside the workspace by its workspace-relative `SKILL.md` path",
				"Never pass an absolute path outside the workspace",
				"a name with the native `skill` tool, a path with `read`",
			},
			retired: []string{"pre-resolved skill paths", "Copy matching `SKILL.md` paths", "_shared/skill-resolver.md", "read those exact files"},
		},
		"opencode/agents/gentle-ai-worker.md": {
			want: []string{
				"Load every skill listed under `## Skills to load before work` in the parent task: a skill name with the native `skill` tool, a workspace path with `read`.",
				"every listed skill was loaded before repository work",
			},
			retired: []string{"Read every exact path under", "exact skill paths"},
		},
	} {
		body, err := FS.ReadFile(path)
		if err != nil {
			t.Fatalf("ReadFile(%s) error = %v", path, err)
		}
		for _, clause := range tc.want {
			if !strings.Contains(string(body), clause) {
				t.Errorf("%s is missing %q", path, clause)
			}
		}
		for _, phrase := range tc.retired {
			if strings.Contains(string(body), phrase) {
				t.Errorf("%s still contains %q", path, phrase)
			}
		}
	}
}
