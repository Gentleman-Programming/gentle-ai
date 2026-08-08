//go:build windows

package engram

import (
	"context"
	"errors"
	"os/exec"
	"testing"
)

func TestProbeStdio_MissingRelativeWindowsExecutableIsNotInstalled(t *testing.T) {
	orig := stdioHandshakeFn
	stdioHandshakeFn = func(context.Context, string, ...string) error { return exec.ErrNotFound }
	t.Cleanup(func() { stdioHandshakeFn = orig })

	err := ProbeStdio(context.Background(), "engram.exe", "mcp", "--tools=agent")
	if !errors.Is(err, ErrNotInstalled) {
		t.Fatalf("ProbeStdio() error = %v, want ErrNotInstalled for missing relative engram.exe", err)
	}
}
