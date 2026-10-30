//go:build windows

package shellinstaller

import (
	"os"
	"path/filepath"
	"testing"
)

func TestUninstallWindowsLeavesMinimalBackup(t *testing.T) {
	t.Setenv("LOCALAPPDATA", t.TempDir()) // Keep the uninstall hermetic.
	root := t.TempDir()
	if err := userWindowsPrivate(root); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(root, "install")
	for _, relative := range []string{
		`config`, `agent`, `home`, `home/go`, `home/go/pkg/mod`, `home/.gentle-ai`, `runtime/node`, `runtime/archives`, `runtime/cache`,
		`prefix`, `prefix/lib`, `state`, `tmp`, `bin`, `source`, `project`,
	} {
		if err := os.MkdirAll(filepath.Join(target, relative), 0700); err != nil {
			t.Fatal(err)
		}
	}
	for _, relative := range []string{
		`config/persona.json`, `agent/mcp.json`, `home/.gentle-ai/state.json`,
		`selection.json`, `manifest.json`, `runtime/archives/node.zip`,
		`prefix/lib/x.js`, `state/old`, `tmp/scratch`, `bin/gentle-shell.cmd`,
		`source/package-lock.json`, `provision.mjs`, `supervisor.exe`,
		`complete-generated-lock-sri.mjs`, `main.json`, `home/go/pkg/mod/m`,
	} {
		if err := userWindowsWrite(filepath.Join(target, relative), []byte("x")); err != nil {
			t.Fatal(err)
		}
	}
	home := t.TempDir()
	backup, err := UninstallUserShellInstall(home, UserShellInstall{Target: target, Mode: "separate"})
	if err != nil || backup == "" {
		t.Fatalf("Windows uninstall failed: %v %q", err, backup)
	}
	if _, err := os.Lstat(target); !os.IsNotExist(err) {
		t.Fatal("target still exists after Windows uninstall")
	}
	if filepath.Dir(backup) != root || filepath.Base(backup) == "install" {
		t.Fatalf("backup path %q is not the renamed target", backup)
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
		t.Fatalf("uninstall lost the retained home state: %v", err)
	}
}

// userWindowsUninstallFixture builds a minimal owned Separate installation
// that survives pruning: only the retained backup set plus the cache-less
// per-user LOCALAPPDATA override.
func userWindowsUninstallFixture(t *testing.T) (target, cache string) {
	t.Helper()
	t.Setenv("LOCALAPPDATA", t.TempDir())
	root := t.TempDir()
	if err := userWindowsPrivate(root); err != nil {
		t.Fatal(err)
	}
	target = filepath.Join(root, "install")
	for _, relative := range []string{"config", "agent", "home", "home/.gentle-ai"} {
		if err := os.MkdirAll(filepath.Join(target, relative), 0700); err != nil {
			t.Fatal(err)
		}
	}
	for _, relative := range []string{
		`config/persona.json`, `agent/mcp.json`, `home/.gentle-ai/state.json`,
		`selection.json`, `manifest.json`,
	} {
		if err := userWindowsWrite(filepath.Join(target, relative), []byte("x")); err != nil {
			t.Fatal(err)
		}
	}
	return target, filepath.Join(os.Getenv("LOCALAPPDATA"), "gentle-shell", "cache", "downloads")
}

func TestUninstallWindowsRemovesOwnedDownloadCache(t *testing.T) {
	target, cache := userWindowsUninstallFixture(t)
	if err := os.MkdirAll(cache, 0700); err != nil {
		t.Fatal(err)
	}
	if err := userWindowsWrite(filepath.Join(cache, "node.zip"), []byte("x")); err != nil {
		t.Fatal(err)
	}
	backup, err := UninstallUserShellInstall(t.TempDir(), UserShellInstall{Target: target, Mode: "separate"})
	if err != nil || backup == "" {
		t.Fatalf("Windows uninstall failed: %v %q", err, backup)
	}
	if _, err := os.Lstat(cache); !os.IsNotExist(err) {
		t.Fatalf("owned download cache survived uninstall: %v", err)
	}
	if _, err := os.Lstat(filepath.Join(backup, "manifest.json")); err != nil {
		t.Fatalf("uninstall lost the retained backup: %v", err)
	}
}

func TestUninstallWindowsPreservesForeignDownloadCache(t *testing.T) {
	target, cache := userWindowsUninstallFixture(t)
	aliased := filepath.Join(t.TempDir(), "downloads")
	if err := os.MkdirAll(aliased, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(cache), 0700); err != nil {
		t.Fatal(err)
	}
	userWindowsSymlinkOrSkip(t, aliased, cache)
	backup, err := UninstallUserShellInstall(t.TempDir(), UserShellInstall{Target: target, Mode: "separate"})
	if err != nil || backup == "" {
		t.Fatalf("aliased download cache custody must not fail the uninstall: %v %q", err, backup)
	}
	if _, err := os.Lstat(cache); err != nil {
		t.Fatalf("foreign download cache was removed: %v", err)
	}
	if _, err := os.Lstat(filepath.Join(aliased, "keep.txt")); !os.IsNotExist(err) {
		t.Fatalf("aliased cache destination was touched: %v", err)
	}
}

func TestUninstallWindowsRefusesUncertainTargets(t *testing.T) {
	home := t.TempDir()
	if _, err := UninstallUserShellInstall(home, UserShellInstall{Target: filepath.Join(home, "absent"), Mode: "separate"}); err == nil {
		t.Fatal("uninstall accepted an absent target")
	}
	real := t.TempDir()
	if err := userWindowsPrivate(real); err != nil {
		t.Fatal(err)
	}
	link := real + "-link"
	if err := os.Symlink(real, link); err != nil {
		t.Skipf("symlink creation requires privileges: %v", err)
	}
	if _, err := UninstallUserShellInstall(home, UserShellInstall{Target: link, Mode: "separate"}); err == nil {
		t.Fatal("uninstall accepted a reparse-point target")
	}
	if _, err := os.Lstat(real); err != nil {
		t.Fatalf("refused target was modified: %v", err)
	}
	if _, err := UninstallUserShellInstall(home, UserShellInstall{Target: real, Mode: "agent"}); err == nil {
		t.Fatal("uninstall accepted a non-Separate mode")
	}
}
