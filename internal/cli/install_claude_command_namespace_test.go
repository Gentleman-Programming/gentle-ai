package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v3/internal/backup"
	"github.com/gentleman-programming/gentle-ai/v3/internal/model"
	"github.com/gentleman-programming/gentle-ai/v3/internal/state"
	"github.com/gentleman-programming/gentle-ai/v3/internal/system"
)

// claudeCommandSkillCollisions lists command names shadowed by skill directories.
// Claude Code resolves same-named skills ahead of commands.
func claudeCommandSkillCollisions(t *testing.T, home string) []string {
	t.Helper()
	entries, err := os.ReadDir(filepath.Join(home, ".claude", "commands"))
	if err != nil {
		t.Fatalf("ReadDir(~/.claude/commands): %v", err)
	}
	var collisions []string
	for _, entry := range entries {
		name := strings.TrimSuffix(entry.Name(), ".md")
		if info, err := os.Stat(filepath.Join(home, ".claude", "skills", name)); err == nil && info.IsDir() {
			collisions = append(collisions, name)
		}
	}
	return collisions
}

func TestRunInstallRejectsRetiredSDDCommands(t *testing.T) {
	home := installTestHome(t)

	if _, err := RunInstall([]string{"--agents", "claude-code", "--components", "sdd"}, system.DetectionResult{}); err == nil {
		t.Fatal("install accepted a retired SDD component")
	}
	if _, err := os.Stat(filepath.Join(home, ".claude", "commands", "gentle-sdd-init.md")); !os.IsNotExist(err) {
		t.Fatalf("retired SDD command installed: %v", err)
	}
}

func TestRunSyncPreservesUserEditedLegacyClaudeCommandWithoutReinstallingSDD(t *testing.T) {
	home := installTestHome(t)
	restoreBackupHome := backup.UserHomeDirFn
	backup.UserHomeDirFn = func() (string, error) { return home, nil }
	t.Cleanup(func() { backup.UserHomeDirFn = restoreBackupHome })
	if err := state.Write(home, state.InstallState{
		InstalledAgents:     []string{string(model.AgentClaudeCode)},
		SelectionConfigured: true,
		Components:          []model.ComponentID{model.ComponentSDD},
		Persona:             "neutral",
	}); err != nil {
		t.Fatalf("state.Write() error = %v", err)
	}
	legacy := filepath.Join(home, ".claude", "commands", "sdd-init.md")
	mustWriteFile(t, legacy, []byte("# user-edited former SDD command\n"))
	custom := filepath.Join(home, ".claude", "commands", "my-command.md")
	mustWriteFile(t, custom, []byte("keep"))

	result, err := RunSync([]string{"--agents", "claude-code"})
	if err != nil {
		t.Fatalf("RunSync() error = %v", err)
	}

	// Sync must preserve user edits and never install another SDD command.
	if data, err := os.ReadFile(legacy); err != nil || string(data) != "# user-edited former SDD command\n" {
		t.Fatalf("sync changed an unverified legacy command: %q, %v", data, err)
	}
	if containsPath(result.ChangedFiles, legacy) {
		t.Errorf("ChangedFiles reports untouched legacy command %q", legacy)
	}
	if _, err := os.Stat(filepath.Join(home, ".claude", "commands", "gentle-sdd-init.md")); !os.IsNotExist(err) {
		t.Errorf("sync installed a retired SDD command: %v", err)
	}
	if _, err := os.Stat(custom); err != nil {
		t.Errorf("sync removed a user-owned command: %v", err)
	}
	if collisions := claudeCommandSkillCollisions(t, home); len(collisions) > 0 {
		t.Fatalf("~/.claude/commands files shadowed by same-named ~/.claude/skills directories: %v", collisions)
	}
}
