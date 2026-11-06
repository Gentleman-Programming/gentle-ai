//go:build linux || darwin

package shellinstaller

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A scoped update that reuses CodeGraph copies the installed runtime/codegraph
// tree, including its generated bin/codegraph, and then regenerates that
// command for the destination. The reused command must not survive into the
// stage, or the regeneration fails with "mkdirat bin: file exists".
func TestUserReuseCodeGraphRegeneratesCommand(t *testing.T) {
	ctx := context.Background()
	parent := userUnixTempRoot(t)
	destination := filepath.Join(parent, "final")
	from := filepath.Join(parent, "installed", "runtime", "codegraph")
	library := filepath.Join(from, "node_modules", "@colbymchenry", "codegraph")
	for _, dir := range []string{library, filepath.Join(from, "bin")} {
		if err := os.MkdirAll(dir, 0700); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(library, "package.json"), []byte("{}"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(from, "bin", "codegraph"), []byte("#!/bin/sh\nexec stale\n"), 0700); err != nil {
		t.Fatal(err)
	}
	stage := filepath.Join(parent, "stage")
	to := filepath.Join(stage, "runtime", "codegraph")
	if err := userReuseCodeGraphTree(ctx, from, to); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(to, "node_modules", "@colbymchenry", "codegraph", "package.json")); err != nil {
		t.Fatal("reused CodeGraph library missing:", err)
	}
	if err := userStageCodeGraphExecutableWithNode(ctx, stage, destination, "runtime/node/bin/node"); err != nil {
		t.Fatal("regenerating the command after reuse failed:", err)
	}
	data, err := os.ReadFile(filepath.Join(to, "bin", "codegraph"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "stale") || !strings.Contains(string(data), destination) {
		t.Fatalf("command was not regenerated for the destination: %q", data)
	}
}
