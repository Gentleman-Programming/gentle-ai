package system

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

var pathGOOS = runtime.GOOS

// AddToUserPath adds a directory to the user PATH persistently.
//
// On Windows, it modifies the user-scoped PATH in the registry via PowerShell,
// surviving terminal restarts without admin privileges.
//
// On non-Windows platforms, the directory is added to the current process PATH.
// On Termux (Android), it is also persisted to ~/.bashrc or ~/.zshrc.
//
// This is safe to call on all platforms — build tags are NOT used.
func AddToUserPath(dir string) error {
	if pathGOOS != "windows" {
		// Still add to the current process PATH on non-Windows (harmless for callers).
		if err := addToProcessPath(dir); err != nil {
			return err
		}

		// Handle Termux persistence
		if isTermux() {
			return persistPathTermux(dir)
		}

		return nil
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

func isTermux() bool {
	return os.Getenv("TERMUX_VERSION") != ""
}

func persistPathTermux(dir string) error {
	home := os.Getenv("HOME")
	if home == "" {
		return fmt.Errorf("HOME environment variable not set")
	}

	shell := os.Getenv("SHELL")
	var rcFile string
	if strings.Contains(shell, "zsh") {
		rcFile = filepath.Join(home, ".zshrc")
	} else {
		rcFile = filepath.Join(home, ".bashrc")
	}

	// Check if already present in file
	content, err := os.ReadFile(rcFile)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	if strings.Contains(string(content), dir) {
		return nil
	}

	// Avoid leading blank line when writing to a new (empty) rc file.
	prefix := "\n"
	if len(content) == 0 {
		prefix = ""
	}
	exportCmd := fmt.Sprintf("%sexport PATH=\"%s:$PATH\"\n", prefix, dir)

	f, err := os.OpenFile(rcFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()

	_, err = f.WriteString(exportCmd)
	return err
}
