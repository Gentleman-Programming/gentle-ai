package system

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// AddToUserPath adds a directory to the Windows user PATH persistently.
// Uses PowerShell to modify the user-scoped environment variable in the registry,
// which survives terminal restarts without requiring admin privileges.
//
// On non-Windows platforms this is a no-op (returns nil immediately after adding
// to the current process PATH). This is safe to call on all platforms since the
// binary is cross-compiled — build tags are NOT used.
func AddToUserPath(dir string) error {
	if runtime.GOOS != "windows" {
		// Still add to the current process PATH on non-Windows (harmless for callers).
		return addToProcessPath(dir)
	}

	// Check whether dir is already present in PATH (case-insensitive on Windows).
	currentPath := os.Getenv("PATH")
	for _, p := range filepath.SplitList(currentPath) {
		if strings.EqualFold(filepath.Clean(p), filepath.Clean(dir)) {
			return nil // already present — nothing to do
		}
	}

	// 1. Update the current process PATH so subsequent commands in this run can
	//    find the newly installed binary immediately.
	if err := addToProcessPath(dir); err != nil {
		return err
	}

	// 2. Persist via PowerShell: modifies the user-scoped PATH in the registry.
	//    This change survives terminal restarts and applies to all future processes
	//    for this user without requiring admin privileges.
	//
	//    escapePowerShellString replaces ' with '' (PowerShell's escape for single quotes
	//    within single-quoted strings) to prevent injection via path names like C:\O'Brien.
	safeDir := escapePowerShellString(dir)
	script := fmt.Sprintf(
		`$current = [Environment]::GetEnvironmentVariable('PATH', 'User'); `+
			`if (($current.Split(';')) -notcontains '%s') { `+
			`[Environment]::SetEnvironmentVariable('PATH', '%s;' + $current, 'User') }`,
		safeDir, safeDir,
	)
	cmd := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-Command", script)
	return cmd.Run()
}

// escapePowerShellString escapes a string for safe use inside a PowerShell
// single-quoted string literal by replacing each ' with '' (PowerShell's escape
// sequence for a literal single quote within single-quoted strings).
func escapePowerShellString(s string) string {
	return strings.ReplaceAll(s, "'", "''")
}

// addToProcessPath prepends dir to the current process PATH if it is not already
// present. This is a low-level helper called by AddToUserPath.
func addToProcessPath(dir string) error {
	currentPath := os.Getenv("PATH")

	// Already present in process PATH? Skip.
	for _, p := range filepath.SplitList(currentPath) {
		if strings.EqualFold(filepath.Clean(p), filepath.Clean(dir)) {
			return nil
		}
	}

	if currentPath == "" {
		return os.Setenv("PATH", dir)
	}
	return os.Setenv("PATH", dir+string(os.PathListSeparator)+currentPath)
}
