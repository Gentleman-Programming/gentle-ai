package cli

import (
	"io/fs"
	"path/filepath"
	"sort"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/assets"
	"github.com/gentleman-programming/gentle-ai/v2/internal/model"
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

// TestComponentPathsSDDCoversEveryEmbeddedSharedFile pins the sync property:
// every embedded shared file is a tracked SDD component path, so a sync backs
// it up and verifies it instead of leaving it unmanaged.
func TestComponentPathsSDDCoversEveryEmbeddedSharedFile(t *testing.T) {
	home := t.TempDir()
	adapters := resolveAdapters([]model.AgentID{model.AgentGeminiCLI})

	paths := componentPaths(home, model.Selection{}, adapters, model.ComponentSDD)
	skillDir := adapters[0].SkillsDir(home)

	for _, name := range embeddedSharedFileNames(t) {
		want := filepath.Join(skillDir, "_shared", name)
		if !containsPath(paths, want) {
			t.Errorf("componentPaths(sdd) missing embedded shared file %q (%s)", name, want)
		}
	}
}
