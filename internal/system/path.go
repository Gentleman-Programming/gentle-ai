package system

import (
	"flag"
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
	if runningInGoTest() {
		// Go tests must not mutate the real Windows user PATH registry. Keep the
		// test process behavior identical for callers that need the new directory
		// available later in the same run.
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

// PrioritizeUserPath moves dir to the front of PATH for the current process and,
// on Windows, the user-scoped persistent PATH. Existing entries are preserved;
// only exact matches for dir are removed before the refreshed dir is prepended.
func PrioritizeUserPath(dir string) error {
	dir = strings.Trim(strings.TrimSpace(dir), `"`)
	if dir == "" {
		return nil
	}

	if err := prioritizeProcessPath(dir); err != nil {
		return err
	}
	if runtime.GOOS != "windows" || runningInGoTest() {
		return nil
	}

	safeDir := escapePowerShellString(dir)
	script := fmt.Sprintf(
		`$dir = '%s'; `+
			`$current = [Environment]::GetEnvironmentVariable('PATH', 'User'); `+
			`$entries = @(); `+
			`if ($current) { $entries = $current.Split(';') | Where-Object { $_ -and ([string]::Compare($_.Trim('"'), $dir, $true) -ne 0) } }; `+
			`[Environment]::SetEnvironmentVariable('PATH', ($dir + ';' + ($entries -join ';')).TrimEnd(';'), 'User')`,
		safeDir,
	)
	cmd := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-Command", script)
	return cmd.Run()
}

// UserPathEntries returns the persistent user-scoped PATH entries for the given
// platform. On Windows it reads the User PATH registry-backed environment value;
// on other platforms it returns the current process PATH entries.
func UserPathEntries(goos string) ([]string, error) {
	if goos != "windows" {
		return filepath.SplitList(os.Getenv("PATH")), nil
	}

	cmd := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-Command", `[Environment]::GetEnvironmentVariable('PATH', 'User')`)
	output, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	return splitWindowsPath(strings.TrimSpace(string(output))), nil
}

func splitWindowsPath(value string) []string {
	if value == "" {
		return nil
	}
	return strings.Split(value, ";")
}

func runningInGoTest() bool {
	return flag.Lookup("test.v") != nil
}

// escapePowerShellString escapes a string for safe use inside a PowerShell
// single-quoted string literal by replacing each ' with ” (PowerShell's escape
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

func prioritizeProcessPath(dir string) error {
	currentPath := os.Getenv("PATH")
	if currentPath == "" {
		return os.Setenv("PATH", dir)
	}

	entries := []string{dir}
	for _, entry := range filepath.SplitList(currentPath) {
		entry = strings.TrimSpace(entry)
		if entry == "" || strings.EqualFold(filepath.Clean(entry), filepath.Clean(dir)) {
			continue
		}
		entries = append(entries, entry)
	}
	return os.Setenv("PATH", strings.Join(entries, string(os.PathListSeparator)))
}
