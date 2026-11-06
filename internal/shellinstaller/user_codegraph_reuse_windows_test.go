//go:build windows

package shellinstaller

import (
	"context"
	"os"
	"path/filepath"
	"strings"
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

// The codegraph-acquire phase of a scoped update that reuses CodeGraph must
// regenerate bin\codegraph.cmd, not fail it: the component-reuse phase copies
// the library without its command, and the exclusive regeneration refuses a
// stale copy carried into the stage with "The file exists" (issue #5847).
func TestUserWindowsCodeGraphAcquireRegeneratesReusedCommand(t *testing.T) {
	parent := t.TempDir()
	if err := userWindowsPrivate(parent); err != nil {
		t.Fatal(err)
	}
	backup := filepath.Join(parent, "installed", "runtime", "codegraph")
	library := filepath.Join(backup, "node_modules", "@colbymchenry", "codegraph-win32-x64")
	if err := os.MkdirAll(library, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(library, "package.json"), []byte("{}"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(backup, "bin"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(backup, "bin", "codegraph.cmd"), []byte("stale\r\n"), 0600); err != nil {
		t.Fatal(err)
	}
	stage := filepath.Join(parent, "stage")
	// The component-reuse phase copies the reused library without its command.
	if err := userWindowsReuseCodeGraphTree(context.Background(), backup, filepath.Join(stage, "runtime", "codegraph")); err != nil {
		t.Fatal(err)
	}
	scope := userInstallScope{
		requested: map[string]bool{UserComponentSDK: true},
		reused:    map[string]bool{UserComponentCodeGraph: true},
	}
	req := UserInstallRequest{Experience: &UserExperience{CodeGraph: "on"}}
	if err := userWindowsCodeGraphAcquire(context.Background(), stage, req, scope); err != nil {
		t.Fatal("scoped CodeGraph reuse failed the acquire phase:", err)
	}
	body, err := os.ReadFile(filepath.Join(stage, "runtime", "codegraph", "bin", "codegraph.cmd"))
	if err != nil {
		t.Fatal("regenerated CodeGraph command missing:", err)
	}
	if string(body) == "stale\r\n" {
		t.Fatal("stale CodeGraph command survived the update")
	}
	if !strings.Contains(string(body), "codegraph.js") {
		t.Fatalf("regenerated CodeGraph command differs: %q", body)
	}
}
