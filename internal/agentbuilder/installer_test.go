package agentbuilder

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/model"
)

// makeAgent creates a minimal GeneratedAgent for testing.
func makeAgent(name, content string) *GeneratedAgent {
	return &GeneratedAgent{
		Name:    name,
		Title:   "Test Agent",
		Content: content,
	}
}

func TestInstall_HappyPath_WritesFilesToBothDirs(t *testing.T) {
	dir1 := t.TempDir()
	dir2 := t.TempDir()
	agent := makeAgent("my-agent", "# My Agent\n\nContent here.")

	adapters := []AdapterInfo{
		{AgentID: model.AgentClaudeCode, SkillsDir: dir1},
		{AgentID: model.AgentOpenCode, SkillsDir: dir2},
	}

	results, err := Install(agent, adapters, nil)
	if err != nil {
		t.Fatalf("Install error: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("results len = %d, want 2", len(results))
	}
	for _, r := range results {
		if !r.Success {
			t.Errorf("result for %s: Success=false, err=%v", r.AgentID, r.Err)
		}
	}
}

func TestInstall_FileContentMatchesAgentContent(t *testing.T) {
	dir := t.TempDir()
	content := "# My Agent\n\n## Description\nDoes things.\n"
	agent := makeAgent("my-agent", content)

	adapters := []AdapterInfo{
		{AgentID: model.AgentClaudeCode, SkillsDir: dir},
	}

	results, err := Install(agent, adapters, nil)
	if err != nil {
		t.Fatalf("Install error: %v", err)
	}

	skillFile := filepath.Join(dir, "my-agent", "SKILL.md")
	data, err := os.ReadFile(skillFile)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(data) != content {
		t.Errorf("file content = %q, want %q", string(data), content)
	}

	if results[0].Path != skillFile {
		t.Errorf("Path = %q, want %q", results[0].Path, skillFile)
	}
}

func TestInstall_MissingDirectory_CreatedAutomatically(t *testing.T) {
	base := t.TempDir()
	// nested subdirectory that doesn't exist yet
	skillsDir := filepath.Join(base, "does", "not", "exist")

	agent := makeAgent("auto-dir-agent", "# Agent\n")
	adapters := []AdapterInfo{
		{AgentID: model.AgentClaudeCode, SkillsDir: skillsDir},
	}

	results, err := Install(agent, adapters, nil)
	if err != nil {
		t.Fatalf("Install error: %v", err)
	}
	if !results[0].Success {
		t.Errorf("expected success even for missing parent dir")
	}

	skillFile := filepath.Join(skillsDir, "auto-dir-agent", "SKILL.md")
	if _, statErr := os.Stat(skillFile); statErr != nil {
		t.Errorf("SKILL.md not created: %v", statErr)
	}
}

func TestInstall_RollbackOnSecondWriteFailure(t *testing.T) {
	// dir1: writable — first write succeeds
	dir1 := t.TempDir()

	// dir2: make it read-only so the mkdir inside it fails
	dir2 := t.TempDir()
	// Create a file with the same name as the agent skill dir so MkdirAll fails
	blocker := filepath.Join(dir2, "my-agent")
	if err := os.WriteFile(blocker, []byte("block"), 0444); err != nil {
		t.Fatalf("setup blocker: %v", err)
	}
	// Make blocker read-only to prevent overwrite
	if err := os.Chmod(blocker, 0444); err != nil {
		t.Fatalf("chmod blocker: %v", err)
	}

	agent := makeAgent("my-agent", "# Agent\n")
	adapters := []AdapterInfo{
		{AgentID: model.AgentClaudeCode, SkillsDir: dir1},
		{AgentID: model.AgentOpenCode, SkillsDir: dir2},
	}

	_, err := Install(agent, adapters, nil)
	if err == nil {
		t.Fatal("expected error when second write fails")
	}

	// Verify rollback: the first file should be removed.
	firstFile := filepath.Join(dir1, "my-agent", "SKILL.md")
	if _, statErr := os.Stat(firstFile); statErr == nil {
		t.Errorf("expected first file to be rolled back, but it still exists: %s", firstFile)
	}
}

func TestInstall_RollbackRestoresOverwrittenFile(t *testing.T) {
	dir1 := t.TempDir()
	dir2 := t.TempDir()
	firstFile := filepath.Join(dir1, "my-agent", "SKILL.md")
	if err := os.MkdirAll(filepath.Dir(firstFile), 0755); err != nil {
		t.Fatalf("create existing skill directory: %v", err)
	}
	const original = "# User-authored skill\n"
	if err := os.WriteFile(firstFile, []byte(original), 0600); err != nil {
		t.Fatalf("write existing skill: %v", err)
	}

	blocker := filepath.Join(dir2, "my-agent")
	if err := os.WriteFile(blocker, []byte("block"), 0444); err != nil {
		t.Fatalf("setup blocker: %v", err)
	}

	_, err := Install(makeAgent("my-agent", "# Generated skill\n"), []AdapterInfo{
		{AgentID: model.AgentClaudeCode, SkillsDir: dir1},
		{AgentID: model.AgentOpenCode, SkillsDir: dir2},
	}, nil)
	if err == nil {
		t.Fatal("expected error when second adapter cannot be installed")
	}

	data, readErr := os.ReadFile(firstFile)
	if readErr != nil {
		t.Fatalf("read restored skill: %v", readErr)
	}
	if string(data) != original {
		t.Errorf("restored content = %q, want %q", data, original)
	}
	info, statErr := os.Stat(firstFile)
	if statErr != nil {
		t.Fatalf("stat restored skill: %v", statErr)
	}
	if got, want := info.Mode().Perm(), os.FileMode(0600); got != want {
		t.Errorf("restored mode = %04o, want %04o", got, want)
	}
}

func TestInstall_ReturnsRollbackRemovalFailure(t *testing.T) {
	dir1 := t.TempDir()
	dir2 := t.TempDir()
	firstFile := filepath.Join(dir1, "my-agent", "SKILL.md")
	blocker := filepath.Join(dir2, "my-agent")
	if err := os.WriteFile(blocker, []byte("block"), 0444); err != nil {
		t.Fatalf("setup blocker: %v", err)
	}

	rollbackErr := errors.New("rollback removal failed")
	originalRemove := installerRemoveFile
	installerRemoveFile = func(path string) error {
		if path == firstFile {
			return rollbackErr
		}
		return originalRemove(path)
	}
	t.Cleanup(func() { installerRemoveFile = originalRemove })

	_, err := Install(makeAgent("my-agent", "# Agent\n"), []AdapterInfo{
		{AgentID: model.AgentClaudeCode, SkillsDir: dir1},
		{AgentID: model.AgentOpenCode, SkillsDir: dir2},
	}, nil)
	if !errors.Is(err, rollbackErr) {
		t.Fatalf("Install error = %v, want rollback error %v", err, rollbackErr)
	}
}

func TestInstall_EmptyAdapters_NoErrorAndEmptyResults(t *testing.T) {
	agent := makeAgent("my-agent", "# Agent\n")

	results, err := Install(agent, []AdapterInfo{}, nil)
	if err != nil {
		t.Fatalf("unexpected error with empty adapters: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("results len = %d, want 0", len(results))
	}
}

func TestInstall_NilAgent_ReturnsError(t *testing.T) {
	_, err := Install(nil, []AdapterInfo{}, nil)
	if err == nil {
		t.Fatal("expected error for nil agent")
	}
}

func TestInstall_FinalizationFailureRollsBack(t *testing.T) {
	for _, tt := range []struct {
		name string
		err  error
	}{
		{name: "registry persistence", err: errors.New("registry is read-only")},
		{name: "SDD wiring", err: errors.New("system prompt is read-only")},
	} {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			skillFile := filepath.Join(dir, "my-agent", "SKILL.md")
			if err := os.MkdirAll(filepath.Dir(skillFile), 0755); err != nil {
				t.Fatalf("create skill directory: %v", err)
			}
			const original = "# User-authored skill\n"
			if err := os.WriteFile(skillFile, []byte(original), 0600); err != nil {
				t.Fatalf("write existing skill: %v", err)
			}

			results, err := Install(makeAgent("my-agent", "# Generated skill\n"), []AdapterInfo{
				{AgentID: model.AgentClaudeCode, SkillsDir: dir},
			}, func([]InstallResult) error {
				return tt.err
			})
			if !errors.Is(err, tt.err) {
				t.Fatalf("Install error = %v, want finalization error %v", err, tt.err)
			}
			if len(results) != 1 || results[0].Success {
				t.Fatalf("results = %#v, want one failed result", results)
			}

			data, readErr := os.ReadFile(skillFile)
			if readErr != nil {
				t.Fatalf("read restored skill: %v", readErr)
			}
			if string(data) != original {
				t.Errorf("restored content = %q, want %q", data, original)
			}
		})
	}
}
