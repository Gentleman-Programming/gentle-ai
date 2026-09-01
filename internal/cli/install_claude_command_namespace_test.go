package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/backup"
	"github.com/gentleman-programming/gentle-ai/v2/internal/components/sdd"
	"github.com/gentleman-programming/gentle-ai/v2/internal/model"
	"github.com/gentleman-programming/gentle-ai/v2/internal/state"
	"github.com/gentleman-programming/gentle-ai/v2/internal/system"
)

// claudeCommandSkillCollisions lists every file under ~/.claude/commands whose
// basename is also a directory under ~/.claude/skills. Claude Code resolves a
// same-named skill ahead of a command, and every SDD phase skill is
// delegate-only, so such a command never runs when the user types it
// (#2644, #2322).
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

func TestRunInstallClaudeCommandsNeverShareASkillName(t *testing.T) {
	home := installTestHome(t)

	if _, err := RunInstall([]string{"--agents", "claude-code", "--components", "sdd"}, system.DetectionResult{}); err != nil {
		t.Fatalf("RunInstall() error = %v", err)
	}

	if collisions := claudeCommandSkillCollisions(t, home); len(collisions) > 0 {
		t.Fatalf("~/.claude/commands files shadowed by same-named ~/.claude/skills directories: %v", collisions)
	}
	for _, command := range sdd.OpenCodeCommands() {
		path := filepath.Join(home, ".claude", "commands", "gentle-"+command.Name+".md")
		if _, err := os.Stat(path); err != nil {
			t.Errorf("expected namespaced Claude command %q: %v", path, err)
		}
	}
}

func TestRunSyncRetiresUnprefixedClaudeCommands(t *testing.T) {
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
	mustWriteFile(t, legacy, []byte("# pre-#2644 managed command\n"))
	custom := filepath.Join(home, ".claude", "commands", "my-command.md")
	mustWriteFile(t, custom, []byte("keep"))

	result, err := RunSync([]string{"--agents", "claude-code"})
	if err != nil {
		t.Fatalf("RunSync() error = %v", err)
	}

	if _, statErr := os.Stat(legacy); !os.IsNotExist(statErr) {
		t.Fatalf("sync left the retired Claude command %q installed: %v", legacy, statErr)
	}
	if !containsPath(result.ChangedFiles, legacy) {
		t.Errorf("ChangedFiles missing retired command %q\nchanged = %#v", legacy, result.ChangedFiles)
	}
	if _, err := os.Stat(filepath.Join(home, ".claude", "commands", "gentle-sdd-init.md")); err != nil {
		t.Errorf("sync did not write the namespaced command: %v", err)
	}
	if _, err := os.Stat(custom); err != nil {
		t.Errorf("sync removed a user-owned command: %v", err)
	}
	if collisions := claudeCommandSkillCollisions(t, home); len(collisions) > 0 {
		t.Fatalf("~/.claude/commands files shadowed by same-named ~/.claude/skills directories: %v", collisions)
	}
}
