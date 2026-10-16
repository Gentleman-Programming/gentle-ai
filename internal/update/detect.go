package update

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/gentleman-programming/gentle-ai/v4/internal/system"
)

// Package-level vars for testability (swap in tests via t.Cleanup).
var (
	execCommand   = exec.Command
	lookPath      = exec.LookPath
	userHomeDir   = os.UserHomeDir
	osStat        = os.Stat
	osGetenv      = os.Getenv
	runPowerShell = system.NewPowerShellRunner().Run
)

const powerShellCommand = "<PowerShell>"

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
		// LookPath failed — the running process PATH may be stale (common on
		// Windows immediately after install when AddToUserPath updates the
		// registry but has not yet been picked up by the current process).
		// Fall back to checking known install locations on disk.
		fullPath := findFallbackBinary(tool)
		if fullPath == "" {
			return "" // binary not found anywhere
		}
		// Use the full filesystem path to invoke the binary directly, bypassing PATH.
		binary = fullPath
	}

	// Apply a bounded timeout so a hanging binary (e.g. engram stuck on DB
	// lock) cannot block update/upgrade flows forever.
	detectCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	execBinary, execArgs := buildExecCmd(binary, tool.DetectCmd[1:])
	if execBinary == powerShellCommand {
		out, err := runPowerShell(detectCtx, execArgs...)
		if err != nil {
			return ""
		}
		return parseVersionFromOutput(strings.TrimSpace(string(out)))
	}

	cmd := execCommand(execBinary, execArgs...)

	// Start and wait on the command in this execution path so no goroutine reads
	// mutable exec.Cmd state while Start initializes it. The cancellation callback
	// only receives the concurrency-safe Process after Start has completed.
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := detectCtx.Err(); err != nil {
		return ""
	}
	if err := cmd.Start(); err != nil {
		return ""
	}
	process := cmd.Process
	killDone := make(chan struct{})
	stopKill := context.AfterFunc(detectCtx, func() {
		defer close(killDone)
		_ = process.Kill()
	})
	err := cmd.Wait()
	if !stopKill() {
		<-killDone
	}
	if err != nil {
		return "" // command failed or timed out — binary exists but version unknown
	}

	return parseVersionFromOutput(strings.TrimSpace(out.String()))
}

// findFallbackBinary checks the tool's FallbackPaths for a binary that exists
// on disk. It returns the first path that stat succeeds on, or "" if none found.
// This is used when exec.LookPath fails due to a stale process PATH (e.g.,
// Windows immediately after install).
func findFallbackBinary(tool ToolInfo) string {
	if tool.FallbackPaths == nil {
		return ""
	}
	homeDir, _ := userHomeDir()
	localAppData := osGetenv("LOCALAPPDATA")
	for _, path := range tool.FallbackPaths(homeDir, localAppData) {
		if _, err := osStat(path); err == nil {
			return path
		}
	}
	return ""
}

// buildExecCmd marks .ps1 commands for the central PowerShell runner. Other
// executables pass through unchanged.
func buildExecCmd(binary string, remainingArgs []string) (string, []string) {
	if strings.EqualFold(filepath.Ext(binary), ".ps1") {
		args := append([]string{"-NoProfile", "-File", binary}, remainingArgs...)
		return powerShellCommand, args
	}
	return binary, remainingArgs
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
