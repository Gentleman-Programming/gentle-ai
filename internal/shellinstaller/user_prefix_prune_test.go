package shellinstaller

import (
	"os"
	"path/filepath"
	"testing"
)

func pruneWrite(t *testing.T, root, rel string) string {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestUserPruneNonRuntimeFilesRemovesOnlyTypesAndSourceMaps(t *testing.T) {
	root := t.TempDir()
	gone := []string{
		"openai/index.d.ts", "openai/index.d.mts", "openai/index.d.cts",
		"openai/index.js.map", "openai/index.mjs.map", "openai/index.cjs.map",
		"openai/index.d.ts.map", "@smithy/core/dist/types/a.d.ts",
	}
	kept := []string{
		"openai/index.js", "openai/index.mjs", "openai/index.cjs",
		"openai/package.json", "openai/README.md", "openai/src/index.ts",
		"@shikijs/langs/go.json", "openai/data.map", "openai/types.d.json",
	}
	for _, rel := range append(append([]string{}, gone...), kept...) {
		pruneWrite(t, root, rel)
	}

	removed, err := userPruneNonRuntimeFiles(root)
	if err != nil {
		t.Fatal(err)
	}
	if removed != len(gone) {
		t.Fatalf("removed = %d, want %d", removed, len(gone))
	}
	for _, rel := range gone {
		if _, err := os.Lstat(filepath.Join(root, filepath.FromSlash(rel))); !os.IsNotExist(err) {
			t.Errorf("%s should be removed (err=%v)", rel, err)
		}
	}
	for _, rel := range kept {
		if _, err := os.Lstat(filepath.Join(root, filepath.FromSlash(rel))); err != nil {
			t.Errorf("%s must be preserved: %v", rel, err)
		}
	}
}

func TestUserPruneNonRuntimeFilesPreservesGentlePiTree(t *testing.T) {
	root := t.TempDir()
	// Main-channel verification compares gentle-pi byte for byte.
	top := pruneWrite(t, root, "gentle-pi/index.d.ts")
	nested := pruneWrite(t, root, "gentle-pi/node_modules/dep/index.d.ts")
	other := pruneWrite(t, root, "other/gentle-pi/index.d.ts")

	removed, err := userPruneNonRuntimeFiles(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{top, nested} {
		if _, err := os.Lstat(path); err != nil {
			t.Errorf("%s must be preserved: %v", path, err)
		}
	}
	if _, err := os.Lstat(other); !os.IsNotExist(err) {
		t.Errorf("only the top-level gentle-pi tree is protected; %s should be removed", other)
	}
	if removed != 1 {
		t.Fatalf("removed = %d, want 1", removed)
	}
}

func TestUserPruneNonRuntimeFilesPreservesTypescriptAndMatchesCaseInsensitively(t *testing.T) {
	root := t.TempDir()
	lib := pruneWrite(t, root, "typescript/lib/lib.dom.d.ts")
	upper := pruneWrite(t, root, "dep/INDEX.D.TS")
	lookalike := []string{
		pruneWrite(t, root, "dep/a.d.ts.bak"),
		pruneWrite(t, root, "dep/b.map"),
		pruneWrite(t, root, "dep/c.d.json"),
	}
	removed, err := userPruneNonRuntimeFiles(root)
	if err != nil || removed != 1 {
		t.Fatalf("removed=%d err=%v, want 1 <nil>", removed, err)
	}
	if _, err := os.Lstat(lib); err != nil {
		t.Errorf("typescript lib must be preserved: %v", err)
	}
	if _, err := os.Lstat(upper); !os.IsNotExist(err) {
		t.Errorf("uppercase declaration file should be removed (err=%v)", err)
	}
	for _, path := range lookalike {
		if _, err := os.Lstat(path); err != nil {
			t.Errorf("%s must be preserved: %v", path, err)
		}
	}
}

func TestUserPruneNonRuntimeFilesNeverFollowsOrRemovesSymlinks(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	victim := pruneWrite(t, outside, "keep.d.ts")
	link := filepath.Join(root, "linked")
	if err := os.Symlink(outside, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if _, err := userPruneNonRuntimeFiles(root); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(victim); err != nil {
		t.Fatalf("file behind a symlink must never be touched: %v", err)
	}
}

func TestUserPruneNonRuntimeFilesMissingRootIsNoop(t *testing.T) {
	removed, err := userPruneNonRuntimeFiles(filepath.Join(t.TempDir(), "absent"))
	if err != nil || removed != 0 {
		t.Fatalf("removed=%d err=%v, want 0 <nil>", removed, err)
	}
}
