//go:build linux || darwin

package shellinstaller

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
)

func TestUserCodeGraphOwnedLaunchPath(t *testing.T) {
	root := filepath.Join(t.TempDir(), "stage")
	for _, manifest := range []userManifest{{Mode: "separate", CodeGraph: true}, {Mode: "separate"}, {Mode: "shared", CodeGraph: true}} {
		cmd := userLaunchCommand(context.Background(), root, manifest, "/not-executed/cli.js", nil)
		allowed := manifest.Mode == "separate" && manifest.CodeGraph
		found := false
		for _, entry := range cmd.Env {
			if strings.HasPrefix(entry, "PATH=") {
				found = strings.Contains(entry, filepath.Join(root, "runtime/codegraph/bin"))
			}
		}
		if found != allowed {
			t.Fatalf("CodeGraph discovery differs from exact ON manifest: %+v, %v", manifest, cmd.Env)
		}
	}
	ambiguous := filepath.Join(t.TempDir(), "parent:alias", "stage")
	cmd := userLaunchCommand(context.Background(), ambiguous, userManifest{Mode: "separate", CodeGraph: true}, "/not-executed/cli.js", nil)
	if cmd.Err == nil {
		t.Fatal("ambiguous CodeGraph PATH accepted")
	}
}
