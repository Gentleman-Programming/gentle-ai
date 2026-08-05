package upgrade

import (
	"io"
	"io/fs"
	"path/filepath"
	"sort"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/agents/opencode"
	"github.com/gentleman-programming/gentle-ai/v2/internal/assets"
)

// embeddedSharedFileNames returns the names of every file embedded under
// skills/_shared. Upgrade backup coverage is asserted against this listing
// rather than a literal so a newly added shared file cannot silently miss it.
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

// TestManagedSkillBackupPathsCoverEveryEmbeddedSharedFile pins the upgrade
// property: every embedded shared file is a managed backup path, so upgrading
// preserves and restores it instead of dropping it.
func TestManagedSkillBackupPathsCoverEveryEmbeddedSharedFile(t *testing.T) {
	home := t.TempDir()
	adapter := opencode.NewAdapter()

	paths := managedSkillBackupPaths(home, adapter, io.Discard)
	got := make(map[string]struct{}, len(paths))
	for _, path := range paths {
		got[path] = struct{}{}
	}

	skillDir := adapter.SkillsDir(home)
	for _, name := range embeddedSharedFileNames(t) {
		want := filepath.Join(skillDir, "_shared", name)
		if _, ok := got[want]; !ok {
			t.Errorf("managedSkillBackupPaths missing embedded shared file %q (%s)", name, want)
		}
	}
}
