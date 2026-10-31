//go:build windows

package shellinstaller

import (
	"os"
	"path/filepath"
	"testing"
)

// userWindowsCleanupFixtureWrite writes one owned fixture file, creating its
// parent directories first: the owned writer never creates parents, and a
// missing parent surfaces as a misleading alias/reparse-point refusal.
func userWindowsCleanupFixtureWrite(t *testing.T, root, relative string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(relative))
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := userWindowsWrite(path, []byte("x")); err != nil {
		t.Fatal(err)
	}
}

func userWindowsCleanupFixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	if err := userWindowsPrivate(root); err != nil {
		t.Fatal(err)
	}
	for _, relative := range []string{
		`runtime/cache`, `home/go`, `state/go-build`, `tmp`,
		`runtime/node`, `runtime/archives`, `runtime/go`, `runtime/codegraph`,
		`config`, `agent`, `home`, `prefix`,
	} {
		if err := os.MkdirAll(filepath.Join(root, relative), 0700); err != nil {
			t.Fatal(err)
		}
	}
	for _, relative := range []string{
		`runtime/cache/cacache/x`, `home/go/pkg/mod/m`, `state/go-build/x`, `tmp/scratch`,
		`runtime/node/node.exe`, `runtime/archives/node.zip`, `config/persona.json`,
		`agent/mcp.json`, `home/.gentle-ai/state.json`, `prefix/lib/x.js`,
		`selection.json`, `manifest.json`,
	} {
		userWindowsCleanupFixtureWrite(t, root, relative)
	}
	return root
}

func TestUserWindowsEphemeralCleanupRemovesOnlyEphemeralPaths(t *testing.T) {
	root := userWindowsCleanupFixture(t)
	if err := userWindowsEphemeralCleanup(root); err != nil {
		t.Fatalf("ephemeral cleanup failed: %v", err)
	}
	for _, relative := range userWindowsEphemeralRelatives {
		if _, err := os.Lstat(filepath.Join(root, filepath.FromSlash(relative))); !os.IsNotExist(err) {
			t.Fatalf("ephemeral path %s survived cleanup: %v", relative, err)
		}
	}
	for _, relative := range []string{
		`runtime/node/node.exe`, `runtime/archives/node.zip`, `config/persona.json`,
		`agent/mcp.json`, `home/.gentle-ai/state.json`, `prefix/lib/x.js`,
		`selection.json`, `manifest.json`,
	} {
		if _, err := os.Lstat(filepath.Join(root, relative)); err != nil {
			t.Fatalf("retained path %s was removed: %v", relative, err)
		}
	}
}

func TestUserWindowsEphemeralCleanupHandlesAbsentPaths(t *testing.T) {
	root := t.TempDir()
	if err := userWindowsPrivate(root); err != nil {
		t.Fatal(err)
	}
	userWindowsCleanupFixtureWrite(t, root, `config/persona.json`)
	if err := userWindowsEphemeralCleanup(root); err != nil {
		t.Fatalf("cleanup of absent ephemeral paths failed: %v", err)
	}
}

func TestUserWindowsEphemeralCleanupRefusesUncertainEntry(t *testing.T) {
	root := userWindowsCleanupFixture(t)
	outside := t.TempDir()
	if err := os.RemoveAll(filepath.Join(root, "tmp")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "tmp")); err != nil {
		t.Skipf("symlink creation requires privileges: %v", err)
	}
	if err := userWindowsEphemeralCleanup(root); err == nil {
		t.Fatal("cleanup accepted a reparse-point ephemeral entry")
	}
	if _, err := os.Lstat(filepath.Join(root, "tmp")); err != nil {
		t.Fatalf("uncertain entry was removed: %v", err)
	}
	if _, err := os.Lstat(outside); err != nil {
		t.Fatalf("reparse destination was touched: %v", err)
	}
}

func userWindowsPruneFixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	if err := userWindowsPrivate(root); err != nil {
		t.Fatal(err)
	}
	for _, relative := range []string{
		`config`, `agent`, `home`, `home/go`, `runtime/node`, `runtime/archives`, `prefix`,
		`state`, `tmp`, `bin`, `pnpm`, `source`, `project`,
	} {
		if err := os.MkdirAll(filepath.Join(root, relative), 0700); err != nil {
			t.Fatal(err)
		}
	}
	for _, relative := range []string{
		`config/persona.json`, `agent/mcp.json`, `home/.gentle-ai/state.json`,
		`selection.json`, `manifest.json`, `runtime/archives/node.zip`,
		`prefix/lib/x.js`, `state/old`, `tmp/scratch`, `bin/gentle-shell.cmd`,
		`pnpm/store`, `source/package-lock.json`, `provision.mjs`,
		`supervisor.exe`, `complete-generated-lock-sri.mjs`, `main.json`,
		`home/go/pkg/mod/m`,
	} {
		userWindowsCleanupFixtureWrite(t, root, relative)
	}
	return root
}

func TestUserWindowsPruneRemovedBackupKeepsMinimalSet(t *testing.T) {
	backup := userWindowsPruneFixture(t)
	if err := userWindowsPruneRemovedBackup(backup); err != nil {
		t.Fatalf("backup pruning failed: %v", err)
	}
	entries, err := os.ReadDir(backup)
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for _, entry := range entries {
		if !userWindowsBackupKeep[entry.Name()] {
			t.Fatalf("backup retains unpruned entry %q", entry.Name())
		}
		seen[entry.Name()] = true
	}
	for _, name := range []string{"config", "agent", "home", "selection.json", "manifest.json"} {
		if !seen[name] {
			t.Fatalf("minimal backup lost %q", name)
		}
	}
	if _, err := os.Lstat(filepath.Join(backup, `home`, `go`)); !os.IsNotExist(err) {
		t.Fatalf("backup retained the Go module cache inside home: %v", err)
	}
	if _, err := os.Lstat(filepath.Join(backup, `home`, `.gentle-ai`, `state.json`)); err != nil {
		t.Fatalf("pruning lost the retained home state: %v", err)
	}
}

func TestUserWindowsPruneRemovedBackupHandlesAbsentEntries(t *testing.T) {
	backup := t.TempDir()
	if err := userWindowsPrivate(backup); err != nil {
		t.Fatal(err)
	}
	if err := userWindowsWrite(filepath.Join(backup, "manifest.json"), []byte("x")); err != nil {
		t.Fatal(err)
	}
	if err := userWindowsPruneRemovedBackup(backup); err != nil {
		t.Fatalf("pruning of an already minimal backup failed: %v", err)
	}
}

func TestUserWindowsPruneRemovedBackupRefusesUncertainEntry(t *testing.T) {
	backup := userWindowsPruneFixture(t)
	outside := t.TempDir()
	if err := os.Remove(filepath.Join(backup, "provision.mjs")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(backup, "provision.mjs")); err != nil {
		t.Skipf("symlink creation requires privileges: %v", err)
	}
	err := userWindowsPruneRemovedBackup(backup)
	if err == nil {
		t.Fatal("pruning accepted an uncertain backup entry")
	}
	if info, infoErr := os.Lstat(filepath.Join(backup, "provision.mjs")); infoErr != nil || info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("uncertain entry was removed or replaced: %v %v", info, infoErr)
	}
	if _, err := os.Lstat(filepath.Join(backup, "manifest.json")); err != nil {
		t.Fatalf("pruning disturbed the retained manifest: %v", err)
	}
}
