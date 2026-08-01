package upgrade

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

var scoopGentleAIOwned = defaultScoopGentleAIOwned

const scoopCleanupTimeout = 5 * time.Second

func defaultScoopGentleAIOwned() bool {
	if runtime.GOOS != "windows" {
		return false
	}

	executable, err := os.Executable()
	if err != nil {
		return false
	}

	root := scoopRootWith(os.Getenv, os.UserHomeDir, func(args ...string) ([]byte, error) {
		return scoopCommand(context.Background(), args...)
	})
	if root == "" {
		return false
	}

	return scoopOwnsExecutableWithResolvers(executable, root, scoopResolvePath, scoopResolvePath)
}

func scoopRootWith(getenv func(string) string, userHome func() (string, error), run func(...string) ([]byte, error)) string {
	if output, err := run("config", "root_path"); err == nil {
		root := strings.TrimSpace(string(output))
		if root != "" && !strings.EqualFold(root, "'root_path' is not set") {
			return root
		}
	}

	if root := strings.TrimSpace(getenv("SCOOP")); root != "" {
		return root
	}
	homeDir, err := userHome()
	if err != nil {
		return ""
	}
	return filepath.Join(homeDir, "scoop")
}

func scoopOwnsExecutableWithResolvers(executable, root string, resolveExecutable, resolveCurrent func(string) (string, error)) bool {
	resolvedExecutable, err := resolveExecutable(executable)
	if err != nil {
		return false
	}

	current := filepath.Join(root, "apps", "gentle-ai", "current")
	resolvedCurrent, err := resolveCurrent(current)
	if err != nil {
		return false
	}

	return scoopPathWithin(resolvedExecutable, resolvedCurrent)
}

func scoopPathWithin(path, root string) bool {
	path = strings.ToLower(filepath.Clean(path))
	root = strings.ToLower(filepath.Clean(root))
	return path == root || strings.HasPrefix(path, root+string(filepath.Separator))
}

func scoopUpgrade(ctx context.Context) (err error) {
	previous, err := scoopCommand(ctx, "config", "IGNORE_RUNNING_PROCESSES")
	if err != nil {
		return err
	}

	previousValue := strings.ToLower(strings.TrimSpace(string(previous)))
	restore := []string{"config", "rm", "IGNORE_RUNNING_PROCESSES"}
	if previousValue == "true" || previousValue == "false" {
		restore = []string{"config", "IGNORE_RUNNING_PROCESSES", previousValue}
	} else if !strings.Contains(previousValue, "is not set") {
		return fmt.Errorf("read Scoop IGNORE_RUNNING_PROCESSES: unexpected output %q", strings.TrimSpace(string(previous)))
	}
	defer func() {
		cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), scoopCleanupTimeout)
		defer cancel()
		if _, restoreErr := scoopCommand(cleanupCtx, restore...); restoreErr != nil {
			err = errors.Join(err, fmt.Errorf("restore Scoop IGNORE_RUNNING_PROCESSES: %w", restoreErr))
		}
	}()

	if _, err = scoopCommand(ctx, "config", "IGNORE_RUNNING_PROCESSES", "true"); err != nil {
		return err
	}
	_, err = scoopCommand(ctx, "update", "gentle-ai")
	return err
}

func scoopCommand(ctx context.Context, args ...string) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	cmd := execCommand("scoop", args...)
	cmd.Stdin = nil
	var output bytes.Buffer
	cmd.Stdout = &output
	cmd.Stderr = &output
	if err := cmd.Start(); err != nil {
		out := output.Bytes()
		return out, fmt.Errorf("scoop %s: %w (output: %s)", strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}

	process := cmd.Process
	killDone := make(chan struct{})
	stopKill := context.AfterFunc(ctx, func() {
		defer close(killDone)
		_ = process.Kill()
	})
	err := cmd.Wait()
	if !stopKill() {
		<-killDone
	}

	out := output.Bytes()
	if err != nil {
		return out, fmt.Errorf("scoop %s: %w (output: %s)", strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return out, nil
}
