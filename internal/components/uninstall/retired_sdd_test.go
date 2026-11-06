package uninstall

import (
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v4/internal/model"
	"github.com/gentleman-programming/gentle-ai/v4/internal/state"
)

func releasedSDDSkill(t *testing.T) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "legacyassets", "testdata", "v3.7.0", "skill-sdd-init.md"))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// Retired SDD files belong to no removable component: only an uninstall that
// removes the whole runtime retires them.
func TestPartialComponentUninstallLeavesRetiredSDDFiles(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	skill := filepath.Join(home, ".claude", "skills", "sdd-init", "SKILL.md")
	writeBatchFile(t, skill, releasedSDDSkill(t))
	svc, err := NewService(home, t.TempDir(), "dev")
	if err != nil {
		t.Fatal(err)
	}
	svc.snapshotter = stubSnapshotter{}
	if _, err := svc.PartialUninstall([]model.AgentID{model.AgentClaudeCode}, []model.ComponentID{model.ComponentTheme}); err != nil {
		t.Fatal(err)
	}
	if got, err := os.ReadFile(skill); err != nil || string(got) != releasedSDDSkill(t) {
		t.Fatalf("theme-only uninstall changed the retired SDD skill: %v", err)
	}
}

// A retirement that fails keeps the runtime in state.json, names it in the
// manual actions, and still reports what it already removed.
func TestRetiredSDDFailureKeepsRuntimeInState(t *testing.T) {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("needs POSIX directory permissions enforced")
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	skill := filepath.Join(home, ".claude", "skills", "sdd-init", "SKILL.md")
	writeBatchFile(t, skill, releasedSDDSkill(t))
	command, err := os.ReadFile(filepath.Join("..", "legacyassets", "testdata", "v2.0.0", "claude-sdd-init.md"))
	if err != nil {
		t.Fatal(err)
	}
	commands := filepath.Join(home, ".claude", "commands")
	writeBatchFile(t, filepath.Join(commands, "sdd-init.md"), string(command))
	if err := os.Chmod(commands, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(commands, 0o755) })
	if err := state.Write(home, state.InstallState{InstalledAgents: []string{string(model.AgentClaudeCode)}}); err != nil {
		t.Fatal(err)
	}
	svc, err := NewService(home, t.TempDir(), "dev")
	if err != nil {
		t.Fatal(err)
	}
	svc.snapshotter = stubSnapshotter{}
	result, err := svc.PartialUninstall([]model.AgentID{model.AgentClaudeCode}, nil)
	if err == nil {
		t.Fatal("PartialUninstall() succeeded although a retired SDD command could not be removed")
	}
	if !slices.Contains(result.FailedAgents, model.AgentClaudeCode) || slices.Contains(result.AgentsRemovedFromState, model.AgentClaudeCode) {
		t.Fatalf("failed = %v, removed from state = %v, want claude-code kept in state", result.FailedAgents, result.AgentsRemovedFromState)
	}
	if !slices.Contains(result.RemovedFiles, skill) {
		t.Fatalf("removed files = %v, want the skill retired before the failure", result.RemovedFiles)
	}
	recorded, err := state.Read(home)
	if err != nil || !slices.Contains(recorded.InstalledAgents, string(model.AgentClaudeCode)) {
		t.Fatalf("state = %v, %v, want claude-code still installed", recorded.InstalledAgents, err)
	}
}
