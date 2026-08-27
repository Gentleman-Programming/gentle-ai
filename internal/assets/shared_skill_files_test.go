package assets

import (
	"io/fs"
	"sort"
	"strings"
	"testing"
)

// collectSharedFilesByReadDir independently derives the shared inventory with
// manual recursion over fs.ReadDir, so the helper is compared against a
// separately written traversal rather than against itself.
func collectSharedFilesByReadDir(t *testing.T, dir, prefix string) []string {
	t.Helper()

	entries, err := fs.ReadDir(FS, dir)
	if err != nil {
		t.Fatalf("ReadDir(FS, %q) error = %v", dir, err)
	}

	var names []string
	for _, entry := range entries {
		if entry.IsDir() {
			names = append(names, collectSharedFilesByReadDir(t, dir+"/"+entry.Name(), prefix+entry.Name()+"/")...)
			continue
		}
		names = append(names, prefix+entry.Name())
	}
	return names
}

// TestSharedSkillFileNamesMatchesEmbeddedListing pins that the derivation
// helper reproduces the embedded skills/_shared listing exactly. Deployment
// call sites read this helper instead of maintaining literal inventories.
func TestSharedSkillFileNamesMatchesEmbeddedListing(t *testing.T) {
	want := collectSharedFilesByReadDir(t, SharedSkillDir, "")
	sort.Strings(want)

	got, err := SharedSkillFileNames()
	if err != nil {
		t.Fatalf("SharedSkillFileNames() error = %v", err)
	}

	if len(got) != len(want) {
		t.Fatalf("SharedSkillFileNames() = %v (%d files), want %v (%d files)", got, len(got), want, len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("SharedSkillFileNames()[%d] = %q, want %q (full: %v)", i, got[i], want[i], got)
		}
	}
}

func TestSharedSupportDirectoryIsDocumentedButNotInvokable(t *testing.T) {
	names, err := SharedSkillFileNames()
	if err != nil {
		t.Fatalf("SharedSkillFileNames() error = %v", err)
	}

	foundReadme := false
	for _, name := range names {
		if name == "SKILL.md" {
			t.Fatal("shared support directory must not embed an invokable SKILL.md")
		}
		if name == "README.md" {
			foundReadme = true
		}
	}
	if !foundReadme {
		t.Fatal("shared support directory must embed README.md")
	}

	readme := MustRead(SharedSkillDir + "/README.md")
	if strings.HasPrefix(readme, "---") {
		t.Fatal("shared README.md must not contain skill frontmatter")
	}
	if !strings.Contains(readme, "not an invokable skill") {
		t.Fatal("shared README.md must explain that the directory is support-only")
	}
}

// TestSharedSkillFileNamesIsSortedAndReadable pins deterministic ordering and
// that every reported name resolves as an embedded asset, so derived path
// lists stay stable and every entry is deployable.
func TestSharedSkillFileNamesIsSortedAndReadable(t *testing.T) {
	got, err := SharedSkillFileNames()
	if err != nil {
		t.Fatalf("SharedSkillFileNames() error = %v", err)
	}
	if len(got) == 0 {
		t.Fatal("SharedSkillFileNames() returned no files; the shared inventory cannot be empty")
	}
	if !sort.StringsAreSorted(got) {
		t.Fatalf("SharedSkillFileNames() = %v, want sorted order", got)
	}

	for _, name := range got {
		content, readErr := Read(SharedSkillDir + "/" + name)
		if readErr != nil {
			t.Errorf("SharedSkillFileNames() reported %q but it is not readable: %v", name, readErr)
			continue
		}
		if len(content) == 0 {
			t.Errorf("SharedSkillFileNames() reported %q but the embedded asset is empty", name)
		}
	}
}
