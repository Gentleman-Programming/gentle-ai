//go:build linux || darwin

package shellinstaller

import (
	"os"
	"path/filepath"
	"testing"
)

func userWriteTestFile(t *testing.T, path, data string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(data), 0600); err != nil {
		t.Fatal(err)
	}
}

func userCleanupFixture(t *testing.T) string {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(root, 0700); err != nil {
		t.Fatal(err)
	}
	for _, relative := range []string{
		"runtime/cache/cacache/x", "home/go/pkg/mod/m", "state/go-build/x", "tmp/scratch",
		"runtime/node/bin/node", "runtime/tools/fd.tgz", "runtime/codegraph/bin/x",
		"config/user.npmrc", "agent/mcp.json", "home/.gentle-ai/state.json", "prefix/lib/x.js",
		"installation.json",
	} {
		userWriteTestFile(t, filepath.Join(root, relative), "x")
	}
	return root
}

func TestUserInstallEphemeralCleanupRemovesOnlyEphemeralPaths(t *testing.T) {
	root := userCleanupFixture(t)
	if err := userInstallEphemeralCleanup(root); err != nil {
		t.Fatalf("ephemeral cleanup failed: %v", err)
	}
	for _, relative := range userEphemeralRelatives {
		if _, err := os.Lstat(filepath.Join(root, relative)); !os.IsNotExist(err) {
			t.Fatalf("ephemeral path %s survived cleanup: %v", relative, err)
		}
	}
	for _, relative := range []string{
		"runtime/node/bin/node", "runtime/tools/fd.tgz", "runtime/codegraph/bin/x",
		"config/user.npmrc", "agent/mcp.json", "home/.gentle-ai/state.json",
		"prefix/lib/x.js", "installation.json",
	} {
		if _, err := os.Lstat(filepath.Join(root, relative)); err != nil {
			t.Fatalf("retained path %s was removed: %v", relative, err)
		}
	}
}

func TestUserInstallEphemeralCleanupHandlesAbsentPaths(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(root, 0700); err != nil {
		t.Fatal(err)
	}
	userWriteTestFile(t, filepath.Join(root, "config/user.npmrc"), "x")
	if err := userInstallEphemeralCleanup(root); err != nil {
		t.Fatalf("cleanup of absent ephemeral paths failed: %v", err)
	}
}

func TestUserInstallEphemeralCleanupRefusesSymlinkTarget(t *testing.T) {
	real, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(real, 0700); err != nil {
		t.Fatal(err)
	}
	userWriteTestFile(t, filepath.Join(real, "runtime/cache/x"), "x")
	link := filepath.Join(filepath.Dir(real), filepath.Base(real)+"-link")
	if err := os.Symlink(real, link); err != nil {
		t.Skipf("symlink creation unavailable: %v", err)
	}
	if err := userInstallEphemeralCleanup(link); err == nil {
		t.Fatal("cleanup accepted a symlinked target")
	}
	if _, err := os.Lstat(filepath.Join(real, "runtime/cache/x")); err != nil {
		t.Fatalf("symlinked target was modified: %v", err)
	}
}

func TestUserInstallEphemeralCleanupRefusesUncertainEntries(t *testing.T) {
	root := userCleanupFixture(t)
	outside, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(filepath.Join(root, "tmp")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "tmp")); err != nil {
		t.Skipf("symlink creation unavailable: %v", err)
	}
	if err := userInstallEphemeralCleanup(root); err == nil {
		t.Fatal("cleanup accepted a symlinked ephemeral entry")
	}
	if _, err := os.Lstat(filepath.Join(root, "tmp")); err != nil {
		t.Fatalf("uncertain entry was removed: %v", err)
	}
	if _, err := os.Lstat(outside); err != nil {
		t.Fatalf("symlink destination was touched: %v", err)
	}
	if err := os.Remove(filepath.Join(root, "tmp")); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(root, "state/go-build"), 0777); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(filepath.Join(root, "state/go-build"), 0777); err != nil {
		t.Fatal(err)
	}
	if err := userInstallEphemeralCleanup(root); err == nil {
		t.Fatal("cleanup accepted a world-writable ephemeral directory")
	}
	if _, err := os.Lstat(filepath.Join(root, "state/go-build")); err != nil {
		t.Fatalf("uncertain directory was removed: %v", err)
	}
}

func userPruneFixture(t *testing.T) string {
	t.Helper()
	backup, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(backup, 0700); err != nil {
		t.Fatal(err)
	}
	for _, relative := range []string{
		"config/user.npmrc", "agent/mcp.json", "home/.gentle-ai/state.json",
		"installation.json", "selection.json",
		"runtime/node/bin/node", "runtime/archives/node.tgz", "prefix/lib/x.js",
		"state/old", "tmp/scratch", "bin/gentle-shell", "pnpm/store", "source/package-lock.json",
		"provision.mjs", "supervisor", "main.json", "complete-generated-lock-sri.mjs",
		"normalize-private-optional-platform-closure.mjs", "bootstrap-gentle-shell-private-node.sh",
		"home/go/pkg/mod/m",
	} {
		userWriteTestFile(t, filepath.Join(backup, relative), "x")
	}
	return backup
}

func TestUserPruneRemovedBackupKeepsMinimalSet(t *testing.T) {
	backup := userPruneFixture(t)
	if err := userPruneRemovedBackup(backup); err != nil {
		t.Fatalf("backup pruning failed: %v", err)
	}
	entries, err := os.ReadDir(backup)
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for _, entry := range entries {
		if !userUnixBackupKeep[entry.Name()] {
			t.Fatalf("backup retains unpruned entry %q", entry.Name())
		}
		seen[entry.Name()] = true
	}
	for _, name := range []string{"config", "agent", "home", "installation.json", "selection.json"} {
		if !seen[name] {
			t.Fatalf("minimal backup lost %q", name)
		}
	}
	if _, err := os.Lstat(filepath.Join(backup, "home", "go")); !os.IsNotExist(err) {
		t.Fatalf("backup retained the Go module cache inside home: %v", err)
	}
	if _, err := os.Lstat(filepath.Join(backup, "home", ".gentle-ai", "state.json")); err != nil {
		t.Fatalf("pruning lost the retained home state: %v", err)
	}
	if _, err := os.Lstat(filepath.Join(backup, "agent", "mcp.json")); err != nil {
		t.Fatalf("pruning lost the retained agent: %v", err)
	}
}

func TestUserPruneRemovedBackupHandlesAbsentEntries(t *testing.T) {
	backup, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(backup, 0700); err != nil {
		t.Fatal(err)
	}
	userWriteTestFile(t, filepath.Join(backup, "installation.json"), "x")
	if err := userPruneRemovedBackup(backup); err != nil {
		t.Fatalf("pruning of an already minimal backup failed: %v", err)
	}
}

func TestUserPruneRemovedBackupRefusesUncertainEntry(t *testing.T) {
	backup := userPruneFixture(t)
	outside, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(backup, "provision.mjs")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(backup, "provision.mjs")); err != nil {
		t.Skipf("symlink creation unavailable: %v", err)
	}
	if err := userPruneRemovedBackup(backup); err == nil {
		t.Fatal("pruning accepted an uncertain backup entry")
	}
	if info, err := os.Lstat(filepath.Join(backup, "provision.mjs")); err != nil || info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("uncertain entry was removed or replaced: %v %v", info, err)
	}
	if _, err := os.Lstat(filepath.Join(backup, "installation.json")); err != nil {
		t.Fatalf("pruning disturbed the retained manifest: %v", err)
	}
}
