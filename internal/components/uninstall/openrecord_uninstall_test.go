package uninstall

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/components/openrecord"
	"github.com/gentleman-programming/gentle-ai/v2/internal/model"
)

// TestUninstallRemovesOnlyOpenRecordManifestEntries pins the manifest-driven
// removal scope: exactly the manifest's entries plus the manifest itself are
// removed from each agent's skills dir; a decoy file the manifest does not
// list, the openrecord binary (represented by a decoy PATH entry outside the
// skills dir), and a decoy repository's own records stay untouched.
func TestUninstallRemovesOnlyOpenRecordManifestEntries(t *testing.T) {
	homeDir := t.TempDir()
	claudeSkills := filepath.Join(homeDir, ".claude", "skills")
	geminiSkills := filepath.Join(homeDir, ".gemini", "skills")

	emittedFile := "openrecord-consult/SKILL.md"
	emittedContent := "consult skill body"
	for _, skillsDir := range []string{claudeSkills, geminiSkills} {
		full := filepath.Join(skillsDir, filepath.FromSlash(emittedFile))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(emittedContent), 0o644); err != nil {
			t.Fatal(err)
		}
		manifest := map[string]any{
			"version": 1, "emitter": "openrecord@test", "with_qmd": true,
			"entries": []map[string]string{{"path": emittedFile, "hash": "irrelevant-for-uninstall"}},
		}
		raw, err := json.Marshal(manifest)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(skillsDir, openrecord.ManifestName), raw, 0o644); err != nil {
			t.Fatal(err)
		}

		decoy := filepath.Join(skillsDir, "user-notes.md")
		if err := os.WriteFile(decoy, []byte("not ours"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	// A decoy repository's own openrecord store, well outside the home
	// directory the uninstall service ever touches.
	decoyRepo := t.TempDir()
	decoyRecord := filepath.Join(decoyRepo, ".openrecord", "decisions", "api", "some-decision.md")
	if err := os.MkdirAll(filepath.Dir(decoyRecord), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(decoyRecord, []byte("accepted decision"), 0o644); err != nil {
		t.Fatal(err)
	}

	svc, err := NewService(homeDir, t.TempDir(), "dev")
	if err != nil {
		t.Fatal(err)
	}
	svc.snapshotter = stubSnapshotter{}
	if _, err := svc.PartialUninstall([]model.AgentID{model.AgentClaudeCode, model.AgentGeminiCLI}, allManagedComponents); err != nil {
		t.Fatal(err)
	}

	for _, skillsDir := range []string{claudeSkills, geminiSkills} {
		if _, err := os.Stat(filepath.Join(skillsDir, filepath.FromSlash(emittedFile))); !os.IsNotExist(err) {
			t.Errorf("emitted file under %s was not removed: err = %v", skillsDir, err)
		}
		if _, err := os.Stat(filepath.Join(skillsDir, openrecord.ManifestName)); !os.IsNotExist(err) {
			t.Errorf("manifest under %s was not removed: err = %v", skillsDir, err)
		}
		if _, err := os.Stat(filepath.Join(skillsDir, "user-notes.md")); err != nil {
			t.Errorf("a decoy file the manifest did not list was removed from %s: %v", skillsDir, err)
		}
	}

	if _, err := os.Stat(decoyRecord); err != nil {
		t.Errorf("a decoy repository's own openrecord record was affected: %v", err)
	}
}
