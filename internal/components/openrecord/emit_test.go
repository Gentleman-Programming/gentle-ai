package openrecord

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/model"
)

func stageOpenRecordSkills(t *testing.T, files map[string]string) string {
	t.Helper()
	staging := t.TempDir()
	entries := make([]manifestEntry, 0, len(files))
	for relative, content := range files {
		full := filepath.Join(staging, filepath.FromSlash(relative))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
		entries = append(entries, manifestEntry{Path: relative, Hash: hashContent([]byte(content))})
	}
	writeManifestFile(t, staging, entries)
	return staging
}

// TestFanOutOverRealFilesystem covers the integration case: two agent skills
// directories receive the same emitted set, an agent whose SkillsDir resolves
// empty (Pi) is skipped without error, and re-running the sequence produces
// no duplicates and no failure.
func TestFanOutOverRealFilesystem(t *testing.T) {
	home := t.TempDir()
	staging := stageOpenRecordSkills(t, map[string]string{
		"openrecord-consult/SKILL.md": "consult skill body",
	})

	selected := []model.AgentID{model.AgentClaudeCode, model.AgentGeminiCLI, model.AgentPi}

	fanned, err := fanOut(staging, home, selected)
	if err != nil {
		t.Fatalf("fanOut() error = %v", err)
	}
	if fanned != 2 {
		t.Fatalf("fanOut() fanned = %d, want 2 (Pi has no skills dir)", fanned)
	}

	claudeSkill := filepath.Join(home, ".claude", "skills", "openrecord-consult", "SKILL.md")
	geminiSkill := filepath.Join(home, ".gemini", "skills", "openrecord-consult", "SKILL.md")
	for _, path := range []string{claudeSkill, geminiSkill} {
		content, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("expected %s to exist: %v", path, err)
		}
		if string(content) != "consult skill body" {
			t.Fatalf("%s content = %q, want %q", path, content, "consult skill body")
		}
	}
	claudeManifest := filepath.Join(home, ".claude", "skills", manifestName)
	if _, err := os.Stat(claudeManifest); err != nil {
		t.Fatalf("expected manifest at %s: %v", claudeManifest, err)
	}

	// Re-running must not duplicate anything or fail.
	fannedAgain, err := fanOut(staging, home, selected)
	if err != nil {
		t.Fatalf("second fanOut() error = %v", err)
	}
	if fannedAgain != 2 {
		t.Fatalf("second fanOut() fanned = %d, want 2", fannedAgain)
	}
	entries, err := os.ReadDir(filepath.Join(home, ".claude", "skills", "openrecord-consult"))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("openrecord-consult dir has %d entries, want 1 (no duplicates)", len(entries))
	}
}
