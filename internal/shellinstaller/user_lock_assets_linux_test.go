//go:build linux

package shellinstaller

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	assets "github.com/gentleman-programming/gentle-ai/v4/scripts"
)

// Run the process-boundary regression through the ordinary Go test entrypoint.
func TestUserFrozenLockAcquisition(t *testing.T) {
	if testing.Short() || os.Getuid() == 0 {
		t.Skip("external Node proof requires a non-root process")
	}
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("Node unavailable for process-boundary proof")
	}
	home := t.TempDir()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, node, "--test", "../../e2e/frozen-user-lock-acquisition.test.mjs", "../../e2e/shared-recovery.test.mjs", "../../e2e/frozen-global-materialization.test.mjs", "../../e2e/user-global-graph.test.mjs")
	cmd.Env = []string{"PATH=" + os.Getenv("PATH"), "HOME=" + home, "TMPDIR=" + home, "XDG_CONFIG_HOME=" + home, "XDG_DATA_HOME=" + home,
		"XDG_STATE_HOME=" + home, "XDG_CACHE_HOME=" + home, "PI_CODING_AGENT_DIR=" + home}
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("frozen acquisition/recovery proof: %v\n%s", err, output)
	}
}

func TestUserProvisionAssetsStageAndReadback(t *testing.T) {
	root := t.TempDir()
	privateMust(t, os.Chmod(root, 0700))
	privateMust(t, userProvisionAssets(context.Background(), root, true))
	files, err := assets.ReadUserAssets()
	privateMust(t, err)
	if len(files) != 4 {
		t.Fatalf("asset inventory: %v", files)
	}
	for name, expected := range files {
		filename := filepath.Join(root, name)
		actual, readErr := os.ReadFile(filename)
		privateMust(t, readErr)
		info, statErr := os.Stat(filename)
		privateMust(t, statErr)
		if !bytes.Equal(actual, expected) || info.Mode() != 0400 {
			t.Fatalf("staged bytes or mode differ: %s", name)
		}
	}
	privateMust(t, userProvisionAssets(context.Background(), root, false))
}

func TestUserProvisionAssetsRefusesChangedAssets(t *testing.T) {
	for _, name := range []string{"provision.mjs", "user-global-graph.mjs", "user-locks/modern/package-lock.json", "user-locks/prior/package-lock.json"} {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			privateMust(t, os.Chmod(root, 0700))
			privateMust(t, userProvisionAssets(context.Background(), root, true))
			filename := filepath.Join(root, name)
			data, err := os.ReadFile(filename)
			privateMust(t, err)
			data[0] ^= 1
			privateMust(t, os.Chmod(filename, 0600))
			privateMust(t, os.WriteFile(filename, data, 0400))
			privateMust(t, os.Chmod(filename, 0400))
			if err := userProvisionAssets(context.Background(), root, false); err == nil {
				t.Fatal("changed same-size asset accepted")
			}
		})
	}
}

func TestUserProvisionAssetsRefusesAliasBeforeWritingOutside(t *testing.T) {
	root, outside := t.TempDir(), t.TempDir()
	privateMust(t, os.Chmod(root, 0700))
	privateMust(t, os.Symlink(outside, filepath.Join(root, "user-locks")))
	if err := userProvisionAssets(context.Background(), root, true); err == nil {
		t.Fatal("asset directory alias accepted")
	}
	entries, err := os.ReadDir(outside)
	privateMust(t, err)
	if len(entries) != 0 {
		t.Fatal("staging wrote through the alias")
	}
}
