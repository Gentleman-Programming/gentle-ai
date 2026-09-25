package cli

import (
	"io/fs"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v3/internal/assets"
	"github.com/gentleman-programming/gentle-ai/v3/internal/model"
)

// embeddedSharedFileNames returns the names of every file embedded under
// skills/_shared. Sync path coverage is asserted against this listing rather
// than a literal so a newly added shared file cannot silently miss a path.
func embeddedSharedFileNames(t *testing.T) []string {
	t.Helper()

	entries, err := fs.ReadDir(assets.FS, "skills/_shared")
	if err != nil {
		t.Fatalf("ReadDir(assets.FS, skills/_shared) error = %v", err)
	}

	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		names = append(names, entry.Name())
	}
	if len(names) == 0 {
		t.Fatal("embedded skills/_shared listing is empty; the source of truth cannot be empty")
	}
	sort.Strings(names)
	return names
}

// Shared files remain embedded as source material; ordinary skills own their
// installed paths, rather than an implicit SDD-wide shared directory.
func TestComponentPathsSDDCoversEveryEmbeddedSharedFile(t *testing.T) {
	names := embeddedSharedFileNames(t)
	for _, retained := range []string{"engram-convention.md", "odd-orchestrator-sections.md", "skill-resolver.md"} {
		found := false
		for _, name := range names {
			if name == retained {
				found = true
			}
		}
		if !found {
			t.Errorf("retained shared source missing: %s", retained)
		}
	}
	for _, name := range names {
		if strings.HasPrefix(name, "sdd-") || name == "openspec-convention.md" {
			t.Errorf("retired shared source still embedded: %s", name)
		}
	}
	home := t.TempDir()
	selection := model.Selection{Skills: []model.SkillID{model.SkillGoTesting}}
	paths := componentPaths(home, selection, resolveAdapters([]model.AgentID{model.AgentGeminiCLI}), model.ComponentSkills)
	if want := filepath.Join(home, ".gemini", "skills", "go-testing", "SKILL.md"); !containsPath(paths, want) {
		t.Fatalf("retained skill not tracked: %s", want)
	}
}
