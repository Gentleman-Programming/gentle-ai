package system

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

var powerShellHosts = []string{"pwsh", "powershell.exe", "powershell"}

// PowerShellUnavailableError reports every host lookup failure so callers can
// distinguish a missing prerequisite from a script failure.
type PowerShellUnavailableError struct {
	Causes []error
}

func (e *PowerShellUnavailableError) Error() string {
	return "PowerShell is required but neither pwsh nor Windows PowerShell (powershell.exe/powershell) was found on PATH; install PowerShell 7 or enable Windows PowerShell: " + errors.Join(e.Causes...).Error()
}
func (e *PowerShellUnavailableError) Unwrap() []error { return e.Causes }

// PowerShellRunner is the injectable resolution and execution boundary for
// PowerShell scripts. It retries a legacy host only when the preferred host did
// not start, never after a script returned a non-zero exit status.
type PowerShellRunner struct {
	LookPath   func(string) (string, error)
	RunCommand func(context.Context, string, ...string) ([]byte, error)
}

func NewPowerShellRunner() PowerShellRunner {
	return PowerShellRunner{
		LookPath: exec.LookPath,
		RunCommand: func(ctx context.Context, name string, args ...string) ([]byte, error) {
			cmd := exec.CommandContext(ctx, name, args...)
			EnsureCommandDir(cmd)
			return cmd.CombinedOutput()
		},
	}
}
func (r PowerShellRunner) Run(ctx context.Context, args ...string) ([]byte, error) {
	if r.LookPath == nil || r.RunCommand == nil {
		r = NewPowerShellRunner()
	}
	if ctx == nil {
		ctx = context.Background()
	}
	var lookups, attempts []error
	seen := map[string]struct{}{}
	for _, host := range powerShellHosts {
		path, err := r.LookPath(host)
		if err != nil {
			lookups = append(lookups, fmt.Errorf("locate %s: %w", host, err))
			continue
		}
		key := strings.ToLower(path)
		if _, duplicate := seen[key]; duplicate {
			continue
		}
		seen[key] = struct{}{}
		out, err := r.RunCommand(ctx, path, args...)
		if err == nil {
			return out, nil
		}
		attempts = append(attempts, fmt.Errorf("run %s (%s): %w (output: %s)", host, path, err, strings.TrimSpace(string(out))))
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return out, fmt.Errorf("PowerShell command failed: %w", errors.Join(append(attempts, lookups...)...))
		}
	}
	if len(attempts) == 0 {
		return nil, &PowerShellUnavailableError{Causes: lookups}
	}
	return nil, fmt.Errorf("PowerShell hosts could not be launched: %w", errors.Join(append(attempts, lookups...)...))
}
func PowerShellSingleQuoted(value string) string {
	return strings.ReplaceAll(value, "'", "''")
}
