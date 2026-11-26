//go:build linux || darwin

package cli

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

// A cancelled direct worker gets SIGTERM first, so its own rollback runs
// before any kill (gentle-ai#6177: a SIGKILL left agent/shared provisioning
// orphaned without rollback).
func TestShellStopGracefullySendsTermBeforeKill(t *testing.T) {
	marker := filepath.Join(t.TempDir(), "rolled-back")
	ctx, cancel := context.WithCancel(context.Background())
	cmd := exec.CommandContext(ctx, "/bin/sh", "-c", `trap 'echo done > "$0"; exit 0' TERM; while :; do sleep 0.05; done`, marker)
	shellStopGracefully(cmd)
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	time.Sleep(200 * time.Millisecond)
	cancel()
	_ = cmd.Wait()
	if _, err := os.Stat(marker); err != nil {
		t.Fatalf("worker did not get SIGTERM before being stopped: %v", err)
	}
}
