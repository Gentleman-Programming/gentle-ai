package components_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestNoSubtaskOnSDDOpenCodeCommands ensures the five SDD phase commands in
// internal/assets/opencode/commands/ do not carry `subtask: true` in their
// YAML frontmatter. OpenCode treats `subtask: true` as a forced sub-agent
// invocation, which conflicts with the primary-session routing contract
// (see openspec/changes/2939-keep-sdd-commands-primary/specs/sdd-opencode-subtask-primary/spec.md).
//
// The orchestrator's permission.task allowlist already covers every phase
// worker, so adding `subtask: true` back would only change session topology
// without granting any extra delegation. This is a static, byte-level check;
// it does not parse YAML — simple substring is sufficient because the literal
// `subtask: true` is the only way to express the field in this asset tree.
func TestNoSubtaskOnSDDOpenCodeCommands(t *testing.T) {
	root := filepath.Join("..", "..", "internal", "assets", "opencode", "commands")
	files := []string{
		"sdd-init.md",
		"sdd-explore.md",
		"sdd-apply.md",
		"sdd-verify.md",
		"sdd-archive.md",
	}
	for _, name := range files {
		data, err := os.ReadFile(filepath.Join(root, name))
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		if strings.Contains(string(data), "subtask: true") {
			t.Errorf("%s still declares `subtask: true`; OpenCode will force it into a sub-agent invocation. Remove the line from YAML frontmatter (the orchestrator `task` allowlist already covers every phase worker).", name)
		}
	}
}
