package app

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
	"syscall"
	"time"

	"github.com/gentleman-programming/gentle-ai/internal/system"
	"github.com/gentleman-programming/gentle-ai/internal/update"
	"github.com/gentleman-programming/gentle-ai/internal/update/upgrade"
)

// lookPathFn is a package-level var for testability.
var lookPathFn = exec.LookPath

// Environment variable names for self-update control.
const (
	envNoSelfUpdate   = "GENTLE_AI_NO_SELF_UPDATE"
	envSelfUpdateDone = "GENTLE_AI_SELF_UPDATE_DONE"
)

// Self-update timeouts are split into two phases with distinct SLAs:
//
//   - Check phase (API round-trip): must be fast to avoid delaying CLI startup.
//     5 seconds is generous for a single HTTPS GET to api.github.com — if the
//     network is unreachable, the TCP handshake fails well before this.
//
//   - Execution phase (backup + binary download): needs real time for I/O.
//     The gentle-ai tarball is ~9 MB. At 1 Mbps (a slow mobile connection),
//     that takes ~72 seconds. 60 seconds covers most real-world connections
//     while still bounding the worst case.
//
// Previously a single 7-second timeout covered both phases, which caused the
// download to be cancelled mid-stream on slower connections. The binary was
// sometimes partially or fully written to disk before the context fired,
// resulting in a successful upgrade that was falsely reported as failed.
// See: https://github.com/Gentleman-Programming/gentle-ai/issues/230
const (
	selfUpdateCheckTimeout = 5 * time.Second
	selfUpdateExecTimeout  = 60 * time.Second
)

// reExec is swappable for testing — prevents actual syscall.Exec in tests.
var reExec = func(argv0 string, argv []string, envv []string) error {
	return syscall.Exec(argv0, argv, envv)
}

// goOS returns the current operating system name. Package-level var for testing.
var goOS = func() string { return runtime.GOOS }

// selfUpdate checks for and applies a gentle-ai update before normal dispatch.
// Returns nil on success or skip; errors are non-fatal (caller logs and continues).
//
// Guard evaluation order (per spec):
//  1. GENTLE_AI_SELF_UPDATE_DONE=1 → skip (loop guard)
//  2. GENTLE_AI_NO_SELF_UPDATE=1 → skip (opt-out)
//  3. version == "dev" → skip (dev build)
//  4. Proceed with update check
func selfUpdate(ctx context.Context, version string, profile system.PlatformProfile, stdout io.Writer) error {
	// Guard 1: loop prevention — already updated this invocation.
	if os.Getenv(envSelfUpdateDone) == "1" {
		return nil
	}

	// Guard 2: user opt-out.
	if os.Getenv(envNoSelfUpdate) == "1" {
		return nil
	}

	// Guard 3: dev build — no meaningful version to compare.
	if version == "dev" {
		return nil
	}

	// Phase 1: Check for updates (only gentle-ai).
	// Short timeout — fail fast if the network is unreachable so CLI startup
	// is not delayed for users who are offline or behind a slow proxy.
	checkCtx, checkCancel := context.WithTimeout(ctx, selfUpdateCheckTimeout)
	defer checkCancel()

	results := updateCheckFiltered(checkCtx, version, profile, []string{"gentle-ai"})

	// Find the gentle-ai result.
	var target *update.UpdateResult
	for i := range results {
		if results[i].Tool.Name == "gentle-ai" {
			target = &results[i]
			break
		}
	}

	// No result or not an available update — nothing to do.
	if target == nil || target.Status != update.UpdateAvailable {
		return nil
	}

	// Phase 2: Run upgrade (backup + strategy execution).
	// Independent context with a generous timeout — binary downloads need real
	// time on slower connections. This context is derived from the original
	// parent (not from checkCtx) so the check timeout does not bleed into it.
	execCtx, execCancel := context.WithTimeout(ctx, selfUpdateExecTimeout)
	defer execCancel()

	homeDir, err := os.UserHomeDir()
	if err != nil {
		_, _ = fmt.Fprintf(stdout, "self-update: cannot resolve home directory: %v\n", err)
		return nil // non-fatal
	}

	report := upgradeExecute(execCtx, results, profile, homeDir, false, stdout)

	// Check if upgrade succeeded.
	var succeeded bool
	for _, r := range report.Results {
		if r.ToolName == "gentle-ai" && r.Status == upgrade.UpgradeSucceeded {
			succeeded = true
			break
		}
	}

	if !succeeded {
		// Upgrade failed or was skipped — non-fatal, continue with current binary.
		return nil
	}

	// Re-exec on Unix; print message on Windows.
	if goOS() == "windows" {
		_, _ = fmt.Fprintf(stdout, "Updated to v%s — please restart.\n", target.LatestVersion)
		return nil
	}

	// Unix: re-exec with the updated binary.
	//
	// Use exec.LookPath("gentle-ai") rather than os.Executable() because
	// on Homebrew, os.Executable() resolves to the versioned Cellar path
	// (e.g. /opt/homebrew/Cellar/gentle-ai/1.8.5/bin/gentle-ai) which
	// still points to the OLD binary after upgrade. The PATH symlink
	// (/opt/homebrew/bin/gentle-ai) is updated by Homebrew to the new
	// version, so LookPath gives us the correct binary.
	executable, err := lookPathFn("gentle-ai")
	if err != nil {
		// Fallback to os.Executable() if LookPath fails.
		executable, err = os.Executable()
		if err != nil {
			return nil // non-fatal
		}
	}

	// Set loop guard env var before re-exec.
	os.Setenv(envSelfUpdateDone, "1")

	_, _ = fmt.Fprintf(stdout, "Updated to v%s, restarting...\n", target.LatestVersion)

	return reExec(executable, os.Args, os.Environ())
}
