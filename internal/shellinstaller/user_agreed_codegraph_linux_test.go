//go:build linux && amd64 && shell_defaults_codegraph

package shellinstaller

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// This explicit data-only qualifier stages the real pinned archives. It neither
// runs their code nor installs a runtime, and is not PNPM end-to-end acceptance.
func TestUserAgreedCodeGraphPrepared(t *testing.T) {
	if testing.Short() {
		t.Skip("requires the existing fixed CodeGraph source acquisition")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 80*time.Second)
	defer cancel()
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "runtime"), 0700); err != nil {
		t.Fatal(err)
	}
	archives, err := userAcquireCodeGraphArchives(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := userStageCodeGraphLibrary(ctx, root, archives); err != nil {
		t.Fatal(err)
	}
	const node = "pnpm/nodejs/24.21.0/bin/node"
	if err := userStageCodeGraphExecutableWithNode(ctx, root, root, node); err != nil {
		t.Fatal(err)
	}
	if err := userCodeGraphReadbackWithNode(ctx, root, node); err != nil {
		t.Fatal("prepared library or PNPM Node binding rejected", err)
	}
	if err := userCodeGraphReadback(ctx, root); err == nil {
		t.Fatal("library accepted a different Node binding")
	}
	cmd := &exec.Cmd{Env: []string{"PATH=/owned/pnpm/bin:/usr/bin:/bin"}}
	userCodeGraphLaunchEnv(root, userManifest{Mode: "separate", CodeGraph: true}, cmd)
	if cmd.Err != nil || !strings.Contains(strings.Join(cmd.Env, "\n"), "CODEGRAPH_NO_DOWNLOAD=1") || cmd.Env[0] != "PATH="+root+"/runtime/codegraph/bin:/owned/pnpm/bin:/usr/bin:/bin" {
		t.Fatal("prepared library was not exposed by the shared launcher", cmd.Env, cmd.Err)
	}
	path := filepath.Join(root, "runtime/codegraph/sources/codegraph.0.tgz-part")
	if err := os.WriteFile(path, []byte("tampered"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := userCodeGraphReadbackWithNode(ctx, root, node); err == nil {
		t.Fatal("modified retained CodeGraph source was admitted")
	}
}
