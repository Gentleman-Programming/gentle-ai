//go:build linux

package shellinstaller

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

// rootServiceEnvironment builds the sealed environment for a system-unit run.
// Root has no user manager and no bus socket requirement; the unit inherits
// only the explicit entries below, plus the operator's acceptance binding.
func rootServiceEnvironment() ([]string, error) {
	home := os.Getenv("HOME")
	if err := privateNativeHome(home); err != nil {
		return nil, err
	}
	return []string{"HOME=" + home, "PATH=/usr/bin:/bin", "TERM=xterm-256color", AcceptRootRiskEnvironment()}, nil
}

func rootSystemManagerProbe(ctx context.Context) *exec.Cmd {
	return exec.CommandContext(ctx, "/usr/bin/systemctl", "--system", "show", "--property=Version", "--value")
}

// rootSystemService runs the accepted root workload inside a transient system
// unit with the identical sandbox properties, then requires the same physical
// receipt readback as the user path. No delegation is created and nothing
// runs outside the bounded unit.
func rootSystemService(ctx context.Context, self string, args []string, stdin io.Reader, stdout, stderr io.Writer) (resultErr error) {
	if ctx == nil || ctx.Err() != nil || os.Getuid() != 0 || !RootRiskAccepted() {
		return privateError("unavailable", errors.New("accepted root risk with a running system manager required"))
	}
	interactive, err := userInteractive(stdin)
	if err != nil {
		return err
	}
	if _, err := userSupervisorSHA(ctx, self); err != nil {
		return err
	}
	// Drop inherited/ambient capabilities before starting Go, not on one of its threads.
	if _, err := userSupervisorSHA(ctx, userCapabilityDrop); err != nil {
		return fmt.Errorf("qualify capability-clearing helper %s: %w", userCapabilityDrop, err)
	}
	env, err := rootServiceEnvironment()
	if err != nil {
		return err
	}
	probeCtx, probeCancel := context.WithTimeout(ctx, 2*time.Second)
	defer probeCancel()
	probe := rootSystemManagerProbe(probeCtx)
	probe.Env = env
	version, err := probe.Output()
	if err != nil {
		return fmt.Errorf("read system manager version: %w", err)
	}
	if err := userManagerVersion(version); err != nil {
		return err
	}
	nonce := make([]byte, 8)
	if _, err := rand.Read(nonce); err != nil {
		return err
	}
	unit := fmt.Sprintf("gentle-shell-%x", nonce)
	receiptDir, err := os.MkdirTemp("/tmp", ".gentle-unit-")
	if err != nil {
		return err
	}
	receiptIdentity, err := privateDirectory(receiptDir)
	if err != nil {
		return err
	}
	terminated := false
	defer func() {
		if !terminated {
			failure := privateError("uncertain", resultErr)
			failure.Workspace, failure.Destination = receiptDir, unit
			resultErr = failure
			return
		}
		if cleanupErr := privateCleanup(receiptDir, receiptIdentity); cleanupErr != nil {
			failure := privateError("uncertain", errors.Join(resultErr, cleanupErr))
			failure.Workspace, failure.Destination = receiptDir, unit
			resultErr = failure
		}
	}()
	env = append(env, "GENTLE_SHELL_UNIT="+unit, "GENTLE_SHELL_UNIT_RECEIPT="+filepath.Join(receiptDir, "kernel.json"))
	env = append(env, userProgressEnvironment(ctx)...)
	// The unit's PATH is the fixed system one: the launcher step needs the
	// caller terminal's decision, never its PATH.
	env = append(env, userLauncherIntentEnvironment(os.Getenv("HOME"))...)
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	cmd := exec.CommandContext(ctx, "/usr/bin/systemd-run", rootServiceArgs(unit, interactive, self, cwd, args, env)...)
	cmd.Env, cmd.Stdin, cmd.Stdout, cmd.Stderr = env, stdin, stdout, stderr
	cmd.WaitDelay = 3 * time.Second
	cmd.Cancel = func() error {
		stopCtx, stopCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer stopCancel()
		stop := exec.CommandContext(stopCtx, "/usr/bin/systemctl", "stop", unit)
		stop.Env = env
		stop.Stdout, stop.Stderr = io.Discard, io.Discard
		if err := stop.Run(); err != nil {
			return privateError("uncertain", errors.New("unit stop failed; preserve owned files and inspect unit"))
		}
		return nil
	}
	runErr := cmd.Run()
	readbackErr := userUnitReadback(filepath.Join(receiptDir, "kernel.json"))
	terminated = readbackErr == nil
	return userServiceResult(runErr, readbackErr)
}
