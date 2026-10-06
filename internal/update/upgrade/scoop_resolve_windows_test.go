//go:build windows

package upgrade

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestScoopOwnsExecutableThroughCurrentJunction(t *testing.T) {
	if testing.Short() {
		t.Skip("creates a Windows directory junction")
	}

	root := filepath.Join(t.TempDir(), "scoop")
	version := filepath.Join(root, "apps", "gentle-ai", "2.2.0")
	current := filepath.Join(root, "apps", "gentle-ai", "current")
	if err := os.MkdirAll(version, 0o755); err != nil {
		t.Fatal(err)
	}
	active := filepath.Join(current, "gentle-ai.exe")
	if err := os.WriteFile(filepath.Join(version, "gentle-ai.exe"), nil, 0o600); err != nil {
		t.Fatal(err)
	}

	if output, err := exec.Command("cmd.exe", "/c", "mklink", "/J", current, version).CombinedOutput(); err != nil {
		t.Fatalf("create current junction: %v (output: %s)", err, output)
	}

	if !scoopOwnsExecutableWithResolvers(active, root, scoopResolvePath, scoopResolvePath) {
		t.Fatal("scoopOwnsExecutableWithResolvers() = false, want true for active executable under current junction")
	}
}
