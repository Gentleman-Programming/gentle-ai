package agentbuilder

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/components/mutationjournal"
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

type writeFailureJournal struct {
	installJournal
	err error
}

func (j writeFailureJournal) WriteWithMode(path string, data []byte, mode os.FileMode) (mutationjournal.OwnedFile, error) {
	owned, err := j.installJournal.WriteWithMode(path, data, mode)
	if err != nil {
		return owned, err
	}
	return owned, j.err
}

type restoreFailureJournal struct {
	installJournal
	err error
}

func (j restoreFailureJournal) Restore() error {
	return j.err
}

func TestInstall_HappyPath_WritesFilesToBothDirs(t *testing.T) {
	dir1 := t.TempDir()
	dir2 := t.TempDir()
	agent := makeAgent("my-agent", "# My Agent\n\nContent here.")

	adapters := []AdapterInfo{
		{AgentID: model.AgentClaudeCode, SkillsDir: dir1},
		{AgentID: model.AgentOpenCode, SkillsDir: dir2},
	}

	results, err := Install(agent, adapters, "")
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

	results, err := Install(agent, adapters, "")
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
	info, err := os.Stat(skillFile)
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if got, want := info.Mode().Perm(), os.FileMode(0644); got != want {
		t.Errorf("file mode = %04o, want %04o", got, want)
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

	results, err := Install(agent, adapters, "")
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

func TestInstall_RollbackOnMkdirFailure_RestoresExistingFile(t *testing.T) {
	dir1 := t.TempDir()
	firstFile := filepath.Join(dir1, "my-agent", "SKILL.md")
	if err := os.MkdirAll(filepath.Dir(firstFile), 0755); err != nil {
		t.Fatalf("setup first skill directory: %v", err)
	}
	before := []byte("# Existing Agent\n\x00preserve these bytes\n")
	if err := os.WriteFile(firstFile, before, 0640); err != nil {
		t.Fatalf("setup first skill: %v", err)
	}
	if err := os.Chmod(firstFile, 0640); err != nil {
		t.Fatalf("chmod first skill: %v", err)
	}

	dir2 := t.TempDir()
	mkdirErr := errors.New("mkdir failed")

	agent := makeAgent("my-agent", "# Agent\n")
	adapters := []AdapterInfo{
		{AgentID: model.AgentClaudeCode, SkillsDir: dir1},
		{AgentID: model.AgentOpenCode, SkillsDir: dir2},
	}

	results, err := install(agent, adapters, installOperations{
		mkdirAll: func(path string, mode os.FileMode) error {
			if path == filepath.Join(dir2, agent.Name) {
				return mkdirErr
			}
			return os.MkdirAll(path, mode)
		},
		newJournal: newInstallJournal,
	})
	if err == nil {
		t.Fatal("expected error when second directory creation fails")
	}
	if !errors.Is(err, mkdirErr) {
		t.Errorf("error = %v, want wrapped %v", err, mkdirErr)
	}

	got, readErr := os.ReadFile(firstFile)
	if readErr != nil {
		t.Fatalf("read restored first skill: %v", readErr)
	}
	if string(got) != string(before) {
		t.Errorf("restored first skill = %q, want %q", got, before)
	}
	info, statErr := os.Stat(firstFile)
	if statErr != nil {
		t.Fatalf("stat restored first skill: %v", statErr)
	}
	if got, want := info.Mode().Perm(), os.FileMode(0640); got != want {
		t.Errorf("restored first skill mode = %04o, want %04o", got, want)
	}
	for _, result := range results {
		if result.Success {
			t.Errorf("result for %s remained successful after rollback", result.AgentID)
		}
	}
}

func TestInstall_RestoresWriteThatLandedBeforeFailure(t *testing.T) {
	dir := t.TempDir()
	skillFile := filepath.Join(dir, "my-agent", "SKILL.md")
	if err := os.MkdirAll(filepath.Dir(skillFile), 0755); err != nil {
		t.Fatalf("setup skill directory: %v", err)
	}
	before := []byte("# Existing Agent\n\x00preserve this too\n")
	if err := os.WriteFile(skillFile, before, 0600); err != nil {
		t.Fatalf("setup skill: %v", err)
	}
	if err := os.Chmod(skillFile, 0600); err != nil {
		t.Fatalf("chmod skill: %v", err)
	}
	writeErr := errors.New("write failed after publication")

	results, err := install(makeAgent("my-agent", "# Replacement\n"), []AdapterInfo{{
		AgentID:   model.AgentClaudeCode,
		SkillsDir: dir,
	}}, installOperations{
		mkdirAll: os.MkdirAll,
		newJournal: func(roots ...string) installJournal {
			return writeFailureJournal{installJournal: newInstallJournal(roots...), err: writeErr}
		},
	})
	if !errors.Is(err, writeErr) {
		t.Fatalf("error = %v, want wrapped %v", err, writeErr)
	}
	if len(results) != 1 || results[0].Success {
		t.Fatalf("results = %#v, want one failed result", results)
	}

	got, readErr := os.ReadFile(skillFile)
	if readErr != nil {
		t.Fatalf("read restored skill: %v", readErr)
	}
	if string(got) != string(before) {
		t.Errorf("restored skill = %q, want %q", got, before)
	}
	info, statErr := os.Stat(skillFile)
	if statErr != nil {
		t.Fatalf("stat restored skill: %v", statErr)
	}
	if got, want := info.Mode().Perm(), os.FileMode(0600); got != want {
		t.Errorf("restored skill mode = %04o, want %04o", got, want)
	}
}

func TestInstall_JoinsRestoreFailure(t *testing.T) {
	dir := t.TempDir()
	writeErr := errors.New("write failed after publication")
	restoreErr := errors.New("restore failed")

	results, err := install(makeAgent("my-agent", "# Replacement\n"), []AdapterInfo{{
		AgentID:   model.AgentClaudeCode,
		SkillsDir: dir,
	}}, installOperations{
		mkdirAll: os.MkdirAll,
		newJournal: func(roots ...string) installJournal {
			return restoreFailureJournal{
				installJournal: writeFailureJournal{installJournal: newInstallJournal(roots...), err: writeErr},
				err:            restoreErr,
			}
		},
	})
	if !errors.Is(err, writeErr) {
		t.Errorf("error = %v, want wrapped %v", err, writeErr)
	}
	if !errors.Is(err, restoreErr) {
		t.Errorf("error = %v, want joined %v", err, restoreErr)
	}
	if len(results) != 1 || results[0].Success {
		t.Errorf("results = %#v, want one failed result", results)
	}
}

func TestInstall_EmptyAdapters_NoErrorAndEmptyResults(t *testing.T) {
	agent := makeAgent("my-agent", "# Agent\n")

	results, err := Install(agent, []AdapterInfo{}, "")
	if err != nil {
		t.Fatalf("unexpected error with empty adapters: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("results len = %d, want 0", len(results))
	}
}

func TestInstall_NilAgent_ReturnsError(t *testing.T) {
	_, err := Install(nil, []AdapterInfo{}, "")
	if err == nil {
		t.Fatal("expected error for nil agent")
	}
}
