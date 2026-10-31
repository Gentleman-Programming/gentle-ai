//go:build windows

package shellinstaller

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// A scoped update that reuses CodeGraph must not carry the generated
// bin\codegraph.cmd into the stage: userWindowsCodeGraphCommand regenerates it
// with an exclusive create.
func TestUserWindowsReuseCodeGraphDropsCommand(t *testing.T) {
	parent := t.TempDir()
	from := filepath.Join(parent, "installed", "runtime", "codegraph")
	library := filepath.Join(from, "node_modules", "@colbymchenry", "codegraph-win32-x64")
	for _, dir := range []string{library, filepath.Join(from, "bin")} {
		if err := os.MkdirAll(dir, 0700); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(library, "package.json"), []byte("{}"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(from, "bin", "codegraph.cmd"), []byte("@echo off\r\n"), 0600); err != nil {
		t.Fatal(err)
	}
	to := filepath.Join(parent, "stage", "runtime", "codegraph")
	if err := userWindowsReuseCodeGraphTree(context.Background(), from, to); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(to, "node_modules", "@colbymchenry", "codegraph-win32-x64", "package.json")); err != nil {
		t.Fatal("reused CodeGraph library missing:", err)
	}
	if _, err := os.Lstat(filepath.Join(to, "bin")); !os.IsNotExist(err) {
		t.Fatal("reused CodeGraph command survived into the stage:", err)
	}
}
