package engram

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/gentleman-programming/gentle-ai/v5/internal/system"
)

func TestVerifyInstalled(t *testing.T) {
	original := lookPath
	t.Cleanup(func() { lookPath = original })

	lookPath = func(string) (string, error) { return "/opt/homebrew/bin/engram", nil }
	if err := VerifyInstalled(); err != nil {
		t.Fatalf("VerifyInstalled() error = %v", err)
	}

	lookPath = func(string) (string, error) { return "", errors.New("missing") }
	if err := VerifyInstalled(); err == nil {
		t.Fatalf("VerifyInstalled() expected missing binary error")
	}
}

func TestVerifyPiInstanceIdentityBoundsInheritedStdout(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX launcher passes the child an inherited pipe")
	}
	t.Setenv("ENGRAM_URL", "")
	t.Setenv(system.EngramBinEnv, "/bin/sh")
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	defer writer.Close()
	// Release the child eventually even if the probe hangs draining stdout.
	release := time.AfterFunc(3*protocolProbeTimeout, func() { _ = writer.Close() })
	defer release.Stop()
	original := execCommandContext
	t.Cleanup(func() { execCommandContext = original })
	execCommandContext = func(ctx context.Context, _ string, _ ...string) *exec.Cmd {
		cmd := exec.CommandContext(ctx, "/bin/sh", "-c", `(read -r line <&3) & echo 'unknown command "instance-id"' >&2; exit 1`)
		cmd.ExtraFiles = []*os.File{reader}
		return cmd
	}
	start := time.Now()
	if err := VerifyPiInstanceIdentity(t.Context()); err != nil || time.Since(start) >= 2*protocolProbeTimeout {
		t.Fatalf("probe took %v, error = %v; want a bounded, inconclusive probe", time.Since(start), err)
	}
}

func TestVerifyPiInstanceIdentityBoundsTheVersionLookupPipes(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX launcher passes the child an inherited pipe")
	}
	t.Setenv("ENGRAM_URL", "")
	t.Setenv(system.EngramBinEnv, "/bin/sh")
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	defer writer.Close()
	// Release the child eventually even if the version lookup hangs draining stdout.
	release := time.AfterFunc(3*versionProbeTimeout, func() { _ = writer.Close() })
	defer release.Stop()
	original := execCommandContext
	t.Cleanup(func() { execCommandContext = original })
	execCommandContext = func(ctx context.Context, _ string, args ...string) *exec.Cmd {
		if len(args) > 0 && args[0] == "version" {
			cmd := exec.CommandContext(ctx, "/bin/sh", "-c", `(read -r line <&3) & echo 'engram 1.20.0'`)
			cmd.ExtraFiles = []*os.File{reader}
			return cmd
		}
		return exec.CommandContext(ctx, "/bin/sh", "-c", `echo 'unknown command "instance-id"' >&2; exit 1`)
	}
	start := time.Now()
	err = VerifyPiInstanceIdentity(t.Context())
	if err == nil || !strings.Contains(err.Error(), "does not support instance-id") || time.Since(start) >= 2*versionProbeTimeout {
		t.Fatalf("probe took %v, error = %v; want the confirmed diagnostic within the version probe bound", time.Since(start), err)
	}
}

func TestVerifyVersionCommandUsesTimeoutAndProvidedBinary(t *testing.T) {
	original := runVersionCommand
	t.Cleanup(func() { runVersionCommand = original })

	var gotCommand string
	var sawDeadline bool
	runVersionCommand = func(ctx context.Context, command string) ([]byte, error) {
		gotCommand = command
		_, sawDeadline = ctx.Deadline()
		return []byte("engram 1.18.0\n"), nil
	}

	version, err := VerifyVersionCommand("/tmp/beta/engram")
	if err != nil {
		t.Fatalf("VerifyVersionCommand() error = %v", err)
	}
	if version != "engram 1.18.0" {
		t.Fatalf("version = %q, want trimmed output", version)
	}
	if gotCommand != "/tmp/beta/engram" {
		t.Fatalf("command = %q, want provided binary", gotCommand)
	}
	if !sawDeadline {
		t.Fatal("VerifyVersionCommand() context had no deadline")
	}
}

func TestVerifyVersionCommandFallsBackOnTimeoutError(t *testing.T) {
	original := runVersionCommand
	t.Cleanup(func() { runVersionCommand = original })

	runVersionCommand = func(ctx context.Context, _ string) ([]byte, error) {
		if _, ok := ctx.Deadline(); !ok {
			t.Fatal("VerifyVersionCommand() context had no deadline")
		}
		return nil, context.DeadlineExceeded
	}

	_, err := VerifyVersionCommand("/tmp/hung/engram")
	if err == nil {
		t.Fatal("VerifyVersionCommand() error = nil, want timeout error")
	}
	if !strings.Contains(err.Error(), "engram version command failed") {
		t.Fatalf("VerifyVersionCommand() error = %v, want wrapped version command failure", err)
	}
}
