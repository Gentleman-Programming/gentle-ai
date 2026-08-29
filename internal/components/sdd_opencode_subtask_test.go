package components_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// delegatingSDDCommands lists every OpenCode SDD command whose frontmatter
// routes through the primary `gentle-orchestrator` agent and then delegates
// to a hidden phase sub-agent. Per #2939, none of them may carry
// `subtask: true`: OpenCode treats that field as a forced sub-agent
// invocation even for a primary-mode agent, adding an avoidable orchestrator
// child (or a nested path refused at the default subagent-depth boundary)
// before the real phase worker launches. The parent-owned controls
// (`sdd-status`, `sdd-new`, `sdd-continue`, `sdd-ff`) never delegate and are
// intentionally absent from this list.
//
// `sdd-research` joined this family after the original approval in #2939
// (same root shape: agent + hidden phase sub-agent); it is kept here so the
// ratchet covers the current command set, not the 2026-08 snapshot.
var delegatingSDDCommands = []string{
	"sdd-init.md",
	"sdd-explore.md",
	"sdd-apply.md",
	"sdd-verify.md",
	"sdd-archive.md",
	"sdd-onboard.md",
	"sdd-research.md",
}

// openCodeCommandMeta carries the frontmatter fields the #2939 ratchet
// asserts on. Decoding the YAML (instead of substring-matching the raw file)
// makes the assertions exact: prose or comments that mention the literal
// `agent:` or `subtask:` values cannot produce a false pass or failure.
type openCodeCommandMeta struct {
	Agent   string `yaml:"agent"`
	Subtask bool   `yaml:"subtask"`
}

// decodeCommandFrontmatter reads a command asset and decodes its YAML
// frontmatter block (between the opening and closing `---` delimiters).
func decodeCommandFrontmatter(t *testing.T, path string) openCodeCommandMeta {
	t.Helper()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}

	trimmed := strings.TrimLeft(string(data), "\n")
	const open = "---\n"
	if !strings.HasPrefix(trimmed, open) {
		t.Fatalf("%s: does not open with YAML frontmatter", path)
	}
	rest := trimmed[len(open):]
	end := strings.Index(rest, "\n---")
	if end == -1 {
		t.Fatalf("%s: no closing frontmatter delimiter", path)
	}

	var meta openCodeCommandMeta
	if err := yaml.Unmarshal([]byte(rest[:end]), &meta); err != nil {
		t.Fatalf("%s: invalid frontmatter YAML: %v", path, err)
	}
	return meta
}

// TestNoSubtaskOnSDDOpenCodeCommands is the #2939 source-asset ratchet: the
// delegating SDD command assets must not declare `subtask: true` in their
// YAML frontmatter. The orchestrator's permission `task` allowlist already
// covers every phase worker, so the field only changes session topology —
// it never grants extra delegation capability. The installation-level twin
// of this guard is TestInjectOpenCodeSDDCommandsRemainParentOwned in
// internal/components/sdd.
func TestNoSubtaskOnSDDOpenCodeCommands(t *testing.T) {
	root := filepath.Join("..", "..", "internal", "assets", "opencode", "commands")
	for _, name := range delegatingSDDCommands {
		meta := decodeCommandFrontmatter(t, filepath.Join(root, name))
		if meta.Agent != "gentle-orchestrator" {
			t.Errorf("%s lost `agent: gentle-orchestrator` routing (got %q); the fix must remove only `subtask: true`", name, meta.Agent)
		}
		if meta.Subtask {
			t.Errorf("%s still declares `subtask: true`; OpenCode will force gentle-orchestrator into a sub-agent invocation. Remove the line from the YAML frontmatter (the orchestrator `task` allowlist already covers every phase worker).", name)
		}
	}
}
