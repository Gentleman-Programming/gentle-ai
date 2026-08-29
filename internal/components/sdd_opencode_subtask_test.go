package components_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
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

// commandFrontmatterText extracts the raw YAML frontmatter block (between
// the opening and closing `---` delimiters) of a command asset. Assertions
// about metadata must be scoped to this block: the literal `subtask: true`
// appearing in a command body (for example, when the body documents the
// field) is prose, not a routing regression.
func commandFrontmatterText(t *testing.T, path string) string {
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
	return rest[:end]
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
		fm := commandFrontmatterText(t, filepath.Join(root, name))
		if !strings.Contains(fm, "agent: gentle-orchestrator") {
			t.Errorf("%s lost `agent: gentle-orchestrator` routing; the fix must remove only `subtask: true`", name)
		}
		if strings.Contains(fm, "subtask: true") {
			t.Errorf("%s still declares `subtask: true`; OpenCode will force gentle-orchestrator into a sub-agent invocation. Remove the line from the YAML frontmatter (the orchestrator `task` allowlist already covers every phase worker).", name)
		}
	}
}
