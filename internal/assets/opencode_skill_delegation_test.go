package assets

import (
	"strings"
	"testing"
)

// TestOpenCodeDelegatedSkillsLoadByName pins #4457: installed skills live in
// the OpenCode config directory, outside the workspace, where a delegated
// agent's read needs external_directory approval it may not get. OpenCode's
// native `skill` tool loads an installed skill with only the `skill`
// permission, so the OpenCode orchestrator passes the identifier that tool
// takes, never absolute paths, and the worker loads it through that tool.
// OpenCode 1.x takes the skill's <name>; 2.x takes its <id>, the skill's
// directory name, which <available_skills> lists beside the name.
func TestOpenCodeDelegatedSkillsLoadByName(t *testing.T) {
	t.Parallel()

	for path, tc := range map[string]struct{ want, retired []string }{
		"opencode/orchestrator.md": {
			want: []string{
				"an installed skill (one listed in `<available_skills>`) by the identifier the native `skill` tool takes: its `<id>` when `<available_skills>` lists one (OpenCode 2.x), otherwise its `<name>` (OpenCode 1.x)",
				"a skill file inside the workspace by its workspace-relative `SKILL.md` path",
				"Never pass an absolute path outside the workspace",
				"a skill identifier with the native `skill` tool, a path with `read`",
			},
			retired: []string{"pre-resolved skill paths", "Copy matching `SKILL.md` paths", "_shared/skill-resolver.md", "read those exact files", "by its name", "loads installed skills by name"},
		},
		"opencode/agents/gentle-ai-worker.md": {
			want: []string{
				"Load every skill listed under `## Skills to load before work` in the parent task: a skill identifier with the native `skill` tool, a workspace path with `read`.",
				"every listed skill was loaded before repository work",
			},
			retired: []string{"Read every exact path under", "exact skill paths", "a skill name with the native"},
		},
		// The shared routing section is injected into the OpenCode
		// orchestrator, whose skills list carries identifiers, not paths.
		"skills/_shared/odd-orchestrator-sections.md": {
			retired: []string{"exact-path form as `## Skills to load before work`"},
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
