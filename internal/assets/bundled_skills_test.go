package assets

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestPublicBundledSkillsMatchEmbeddedAssets(t *testing.T) {
	for _, skill := range []string{"systemic-issue-triage", "gentle-ai-bench"} {
		t.Run(skill, func(t *testing.T) {
			source, err := os.ReadFile(filepath.Join("..", "..", "skills", skill, "SKILL.md"))
			if err != nil {
				t.Fatalf("ReadFile(public source) error = %v", err)
			}

			embedded, err := Read("skills/" + skill + "/SKILL.md")
			if err != nil {
				t.Fatalf("Read(embedded asset) error = %v", err)
			}
			if !bytes.Equal(source, []byte(embedded)) {
				t.Fatal("public source skill and embedded distribution asset differ")
			}
		})
	}
}
