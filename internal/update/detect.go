package update

import (
	"context"
	"os/exec"
	"regexp"
	"strings"
	"time"
)

// Package-level vars for testability (swap in tests via t.Cleanup).
var (
	execCommand = exec.Command
	lookPath    = exec.LookPath
)

// versionRegexp extracts a semver-like version from command output.
// Same pattern as internal/system/deps.go for consistency.
var versionRegexp = regexp.MustCompile(`(\d+\.\d+(?:\.\d+)?)`)

// devVersionRegexp matches common unversioned source-build output like
// "engram dev" or "version: dev".
var devVersionRegexp = regexp.MustCompile(`(?i)(?:^|\s)dev(?:$|\s)`)

// detectInstalledVersion determines the installed version of a tool.
// For tools with nil DetectCmd (gentle-ai), returns currentBuildVersion.
// For other tools, checks LookPath then runs the detect command.
func detectInstalledVersion(ctx context.Context, tool ToolInfo, currentBuildVersion string) string {
	if tool.DetectCmd == nil {
		return currentBuildVersion
	}

	if len(tool.DetectCmd) == 0 {
		return ""
	}

	binary := tool.DetectCmd[0]
	if _, err := lookPath(binary); err != nil {
		return "" // binary not found
	}

	// Apply a bounded timeout so a hanging binary (e.g. engram stuck on DB
	// lock) cannot block update/upgrade flows forever.
	detectCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	cmd := execCommand(tool.DetectCmd[0], tool.DetectCmd[1:]...)

	// Kill the subprocess when the context fires. We use a goroutine because
	// the testable execCommand var returns a plain *exec.Cmd (not CommandContext).
	done := make(chan struct{})
	go func() {
		select {
		case <-detectCtx.Done():
			if cmd.Process != nil {
				_ = cmd.Process.Kill()
			}
		case <-done:
		}
	}()

	out, err := cmd.Output()
	close(done)
	if err != nil {
		return "" // command failed or timed out — binary exists but version unknown
	}

	return parseVersionFromOutput(strings.TrimSpace(string(out)))
}

// parseVersionFromOutput extracts the first semver-like pattern from raw output.
func parseVersionFromOutput(output string) string {
	if output == "" {
		return ""
	}

	if devVersionRegexp.MatchString(output) {
		return "dev"
	}

	match := versionRegexp.FindStringSubmatch(output)
	if len(match) >= 2 {
		return match[1]
	}

	return ""
}
