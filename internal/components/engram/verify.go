package engram

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
)

var (
	lookPath    = exec.LookPath
	execCommand = exec.Command
)

func VerifyInstalled() error {
	if _, err := lookPath("engram"); err != nil {
		return fmt.Errorf("engram binary not found in PATH: %w", err)
	}

	return nil
}

// runVersionCommand executes `engram version` and returns raw stdout. It is a
// package-level seam (built on the execCommand precedent) so tests can pin
// the parsed version deterministically — without spawning a real process —
// feeding the Decision 1 version-gate boundary (see SetVersionForTest).
const versionProbeTimeout = protocolProbeTimeout

var runVersionCommand = func(ctx context.Context, command string) ([]byte, error) {
	if strings.TrimSpace(command) == "" {
		command = "engram"
	}
	cmd := execCommandContext(ctx, command, "version")
	cmd.Stdin = nil
	return cmd.Output()
}

// VerifyVersion runs "engram version" and returns the trimmed output.
// Returns an error if the command fails or produces no output.
func VerifyVersion() (string, error) {
	return VerifyVersionCommand("engram")
}

// VerifyVersionCommand is like VerifyVersion but probes the provided engram
// command path. It is used by install branches that just resolved or downloaded
// a specific binary, so version gating cannot accidentally read a stale PATH
// shadow.
func VerifyVersionCommand(command string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), versionProbeTimeout)
	defer cancel()

	out, err := runVersionCommand(ctx, command)
	if err != nil {
		return "", fmt.Errorf("engram version command failed: %w", err)
	}

	version := strings.TrimSpace(string(out))
	if version == "" {
		return "", fmt.Errorf("engram version returned empty output")
	}

	return version, nil
}

// SetVersionForTest replaces the underlying VerifyVersion() command with a
// fake that returns the given raw version string, and restores the original
// after the test completes. This lets golden/integration tests pin the
// engram version result feeding the Decision 1 slim/full gate without
// depending on a real installed engram binary.
func SetVersionForTest(t interface {
	Helper()
	Cleanup(func())
}, version string) {
	t.Helper()
	orig := runVersionCommand
	runVersionCommand = func(context.Context, string) ([]byte, error) {
		return []byte(version), nil
	}
	t.Cleanup(func() { runVersionCommand = orig })
}

// CountVersionCallsForTest behaves like SetVersionForTest but also
// increments the returned counter on every underlying `engram version`
// invocation. This lets cross-package integration tests (internal/cli)
// assert the command is shelled out at most once per run (JD-016) without
// depending on a real installed engram binary.
func CountVersionCallsForTest(t interface {
	Helper()
	Cleanup(func())
}, version string) *int {
	t.Helper()
	count := 0
	orig := runVersionCommand
	runVersionCommand = func(context.Context, string) ([]byte, error) {
		count++
		return []byte(version), nil
	}
	t.Cleanup(func() { runVersionCommand = orig })
	return &count
}
