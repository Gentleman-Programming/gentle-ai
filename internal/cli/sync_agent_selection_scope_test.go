package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/components/communitytool"
	"github.com/gentleman-programming/gentle-ai/v2/internal/model"
	"github.com/gentleman-programming/gentle-ai/v2/internal/state"
)

// Regression test for issue #3473.
//
// The reported machine had Codex and Cursor installed but selected only
// OpenCode in Gentle AI. Because the CodeGraph sync steps discovered agents
// from the filesystem instead of from the persisted selection, every sync
// rewrote ~/.codex/AGENTS.md and ~/.cursor/rules/gentle-ai.mdc. The install-time
// selection is the only authority over what sync writes.
func TestSyncNeverWritesOutsideSelectedAgents(t *testing.T) {
	home := t.TempDir()

	opencodeDir := filepath.Join(home, ".config", "opencode")
	codexPrompt := filepath.Join(home, ".codex", "AGENTS.md")
	cursorPrompt := filepath.Join(home, ".cursor", "rules", "gentle-ai.mdc")

	// OpenCode is selected and already has effective CodeGraph MCP wiring, so
	// the reconcile path stays quiet and no installer subprocess is spawned.
	writeSyncScopeFile(t, filepath.Join(opencodeDir, "opencode.json"), `{
  "mcp": {
    "codegraph": {
      "type": "local",
      "enabled": true,
      "command": ["codegraph", "serve", "--mcp"]
    }
  }
}`)
	// A managed guidance marker makes CodeGraph report as configured.
	writeSyncScopeFile(t, filepath.Join(opencodeDir, "AGENTS.md"),
		"<!-- gentle-ai:codegraph-guidance -->\nstale\n<!-- /gentle-ai:codegraph-guidance -->\n")

	// Codex and Cursor exist on the machine but were never selected.
	codexBefore := "user codex instructions\n"
	cursorBefore := "user cursor rules\n"
	writeSyncScopeFile(t, codexPrompt, codexBefore)
	writeSyncScopeFile(t, cursorPrompt, cursorBefore)

	if err := state.Write(home, state.InstallState{
		InstalledAgents: []string{string(model.AgentOpenCode)},
		Persona:         string(model.PersonaNeutral),
	}); err != nil {
		t.Fatalf("seed state: %v", err)
	}

	origLookPath := cmdLookPath
	cmdLookPath = func(string) (string, error) { return "/bin/codegraph", nil }
	t.Cleanup(func() { cmdLookPath = origLookPath })

	if communitytool.NeedsOpenCodeCodeGraphReconcile(home) {
		t.Fatalf("test setup: OpenCode wiring must already be effective so sync spawns no installer")
	}

	result, err := RunSyncWithSelection(home, model.Selection{
		Agents:         []model.AgentID{model.AgentOpenCode},
		CommunityTools: []model.CommunityToolID{model.CommunityToolCodeGraph},
		Persona:        model.PersonaNeutral,
	})
	if err != nil {
		t.Fatalf("RunSyncWithSelection() error = %v", err)
	}

	for _, path := range result.ChangedFiles {
		if strings.Contains(path, string(filepath.Separator)+".codex"+string(filepath.Separator)) ||
			strings.Contains(path, string(filepath.Separator)+".cursor"+string(filepath.Separator)) {
			t.Errorf("sync reported a change in an unselected agent: %q", path)
		}
	}

	assertSyncScopeUnchanged(t, codexPrompt, codexBefore)
	assertSyncScopeUnchanged(t, cursorPrompt, cursorBefore)
}

func writeSyncScopeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll(%q): %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile(%q): %v", path, err)
	}
}

func assertSyncScopeUnchanged(t *testing.T, path, want string) {
	t.Helper()
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%q): %v", path, err)
	}
	if string(got) != want {
		t.Errorf("sync rewrote unselected agent file %q:\n--- got ---\n%s\n--- want ---\n%s", path, got, want)
	}
}
