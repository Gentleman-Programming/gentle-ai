package shellinstaller

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
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

	removed, err := userPruneNonRuntimeFiles(context.Background(), root)
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

	removed, err := userPruneNonRuntimeFiles(context.Background(), root)
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
	removed, err := userPruneNonRuntimeFiles(context.Background(), root)
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
	if _, err := userPruneNonRuntimeFiles(context.Background(), root); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(victim); err != nil {
		t.Fatalf("file behind a symlink must never be touched: %v", err)
	}
}

func TestUserPruneNonRuntimeFilesMissingRootIsNoop(t *testing.T) {
	removed, err := userPruneNonRuntimeFiles(context.Background(), filepath.Join(t.TempDir(), "absent"))
	if err != nil || removed != 0 {
		t.Fatalf("removed=%d err=%v, want 0 <nil>", removed, err)
	}
}

func TestUserPruneProtectsScopedGentlePiAndNestedTypescript(t *testing.T) {
	root := t.TempDir()
	scoped := pruneWrite(t, root, "@acme/gentle-pi/index.d.ts")
	nestedTS := pruneWrite(t, root, "dep/node_modules/typescript/lib/lib.es5.d.ts")
	deepGentle := pruneWrite(t, root, "dep/node_modules/gentle-pi/index.d.ts")
	other := pruneWrite(t, root, "@acme/other/index.d.ts")

	removed, err := userPruneNonRuntimeFiles(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{scoped, nestedTS} {
		if _, err := os.Lstat(path); err != nil {
			t.Errorf("%s must be preserved: %v", path, err)
		}
	}
	for _, path := range []string{deepGentle, other} {
		if _, err := os.Lstat(path); !os.IsNotExist(err) {
			t.Errorf("%s should be removed (err=%v)", path, err)
		}
	}
	if removed != 2 {
		t.Fatalf("removed = %d, want 2", removed)
	}
}

func TestUserPruneStopsWhenContextIsCancelled(t *testing.T) {
	root := t.TempDir()
	for i := 0; i < 5; i++ {
		pruneWrite(t, root, "dep/f"+strings.Repeat("a", i+1)+".d.ts")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	removed, err := userPruneNonRuntimeFiles(ctx, root)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
	if removed != 0 {
		t.Fatalf("removed = %d, want 0 after cancellation", removed)
	}
}

func TestUserPruneKeepsGoingAndJoinsErrorsWhenARemovalFails(t *testing.T) {
	root := t.TempDir()
	locked := pruneWrite(t, root, "dep/locked.d.ts")
	freed := pruneWrite(t, root, "dep/freed.d.ts")
	real := userPruneRemove
	userPruneRemove = func(path string) error {
		if path == locked {
			return errors.New("access denied")
		}
		return real(path)
	}
	t.Cleanup(func() { userPruneRemove = real })

	removed, err := userPruneNonRuntimeFiles(context.Background(), root)
	if err == nil || !strings.Contains(err.Error(), "access denied") {
		t.Fatalf("err = %v, want it to carry the removal failure", err)
	}
	if removed != 1 {
		t.Fatalf("removed = %d, want 1", removed)
	}
	if _, statErr := os.Lstat(freed); !os.IsNotExist(statErr) {
		t.Errorf("a failure on one file must not stop the others (err=%v)", statErr)
	}
}

func TestUserPrunePrefixSkipsWhenTheSDKComponentIsReused(t *testing.T) {
	stage := t.TempDir()
	file := pruneWrite(t, stage, "prefix/node_modules/dep/index.d.ts")
	var log bytes.Buffer
	if err := userPrunePrefix(context.Background(), stage, true, &log); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(file); err != nil {
		t.Errorf("a reused prefix must not be pruned again: %v", err)
	}
	if log.Len() != 0 {
		t.Errorf("a skipped prune must not log, got %q", log.String())
	}
}

func TestUserPrunePrefixPrunesAndLogsElapsedTime(t *testing.T) {
	stage := t.TempDir()
	file := pruneWrite(t, stage, "prefix/node_modules/dep/index.d.ts")
	var log bytes.Buffer
	if err := userPrunePrefix(context.Background(), stage, false, &log); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(file); !os.IsNotExist(err) {
		t.Errorf("declaration file should be pruned (err=%v)", err)
	}
	if got := log.String(); !strings.Contains(got, "pruned 1 non-runtime files in ") {
		t.Errorf("log = %q, want the count and the elapsed time", got)
	}
}

func TestUserPrunePrefixWarnsButNeverFailsTheInstall(t *testing.T) {
	stage := t.TempDir()
	pruneWrite(t, stage, "prefix/node_modules/dep/index.d.ts")
	real := userPruneRemove
	userPruneRemove = func(string) error { return errors.New("access denied") }
	t.Cleanup(func() { userPruneRemove = real })

	var log bytes.Buffer
	if err := userPrunePrefix(context.Background(), stage, false, &log); err != nil {
		t.Fatalf("a prune failure must not fail the install: %v", err)
	}
	got := log.String()
	if !strings.Contains(got, "warning: prefix prune incomplete after 0 files in ") || !strings.Contains(got, "access denied") {
		t.Errorf("log = %q, want a warning with the count, elapsed time and cause", got)
	}
}

func TestUserPrunePrefixReturnsTheContextErrorWhenCancelled(t *testing.T) {
	stage := t.TempDir()
	pruneWrite(t, stage, "prefix/node_modules/dep/index.d.ts")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var log bytes.Buffer
	if err := userPrunePrefix(ctx, stage, false, &log); !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
	if !strings.Contains(log.String(), "cancelled after 0 files in ") {
		t.Errorf("log = %q, want a cancellation line", log.String())
	}
}
