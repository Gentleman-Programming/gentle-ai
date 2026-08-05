package sdd

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/assets"
)

// embeddedSharedFileNames returns the names of every file embedded under
// skills/_shared. The embedded listing is the single source of truth for the
// shared skill inventory; tests must derive from it so that adding a file
// cannot silently skip a deployment path.
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

// TestInjectDeploysEveryEmbeddedSharedFile pins the install property: every file
// embedded under skills/_shared reaches the user's machine. Asserting against
// the embedded listing rather than a literal keeps a newly added shared file
// from silently missing this path.
func TestInjectDeploysEveryEmbeddedSharedFile(t *testing.T) {
	mockNoPackageManager(t)
	home := t.TempDir()

	result, err := Inject(home, opencodeAdapter(), "")
	if err != nil {
		t.Fatalf("Inject() error = %v", err)
	}

	reported := make(map[string]struct{}, len(result.Files))
	for _, file := range result.Files {
		reported[file] = struct{}{}
	}

	sharedDir := filepath.Join(home, ".config", "opencode", "skills", "_shared")
	for _, name := range embeddedSharedFileNames(t) {
		path := filepath.Join(sharedDir, name)

		info, statErr := os.Stat(path)
		if statErr != nil {
			t.Errorf("embedded shared file %q not deployed by Inject(): %v", name, statErr)
			continue
		}
		if info.Size() == 0 {
			t.Errorf("embedded shared file %q deployed empty at %s", name, path)
		}
		if _, ok := reported[path]; !ok {
			t.Errorf("embedded shared file %q deployed but not reported in result.Files", name)
		}
	}
}

var sharedRelativeLinkPattern = regexp.MustCompile(`\]\(\.\./_shared/([^)#]+)`)

// TestSkillRelativeSharedLinksResolveAfterInject pins that a skill shipping a
// ../_shared/ relative link resolves on disk after install. judgment-day links
// to review-ledger-contract.md, so a shared file that never deploys turns into
// a dangling reference on every synced machine.
func TestSkillRelativeSharedLinksResolveAfterInject(t *testing.T) {
	mockNoPackageManager(t)
	home := t.TempDir()

	if _, err := Inject(home, opencodeAdapter(), ""); err != nil {
		t.Fatalf("Inject() error = %v", err)
	}

	skillsRoot := filepath.Join(home, ".config", "opencode", "skills")
	skillDoc := filepath.Join(skillsRoot, "judgment-day", "SKILL.md")

	content, err := os.ReadFile(skillDoc)
	if err != nil {
		t.Fatalf("read deployed judgment-day SKILL.md: %v", err)
	}

	matches := sharedRelativeLinkPattern.FindAllStringSubmatch(string(content), -1)
	if len(matches) == 0 {
		t.Fatal("judgment-day SKILL.md has no ../_shared/ relative link; the guard would prove nothing")
	}

	for _, match := range matches {
		target := filepath.Join(skillsRoot, "_shared", filepath.FromSlash(match[1]))
		if _, statErr := os.Stat(target); statErr != nil {
			t.Errorf("judgment-day SKILL.md links to %q but it does not resolve on disk: %v", match[1], statErr)
		}
	}
}
