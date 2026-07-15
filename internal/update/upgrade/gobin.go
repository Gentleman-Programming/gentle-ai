package upgrade

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// osExecutableFn resolves the path of the running executable.
// Package-level var for testability — swapped in tests to simulate a binary
// installed in a Go bin directory.
var osExecutableFn = os.Executable

// goEnvFn queries `go env <key>` and returns the trimmed value.
// Package-level var for testability — swapped in tests to avoid real go calls.
var goEnvFn = func(key string) (string, error) {
	out, err := execCommand("go", "env", key).Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// pathCaseInsensitive reports whether filesystem paths compare
// case-insensitively on this platform. Package-level var for testability.
var pathCaseInsensitive = runtime.GOOS == "windows"

// runningFromGoBinDir reports whether the current executable lives in a Go
// binary directory (GOBIN or <GOPATH>/bin), which means it was installed via
// `go install`. goAvailable controls whether `go env` is consulted; process
// env vars and the ~/go/bin default are always checked. Used to preserve the
// original install method on self-upgrade (issue #999).
func runningFromGoBinDir(goAvailable bool) bool {
	exe, err := osExecutableFn()
	if err != nil || strings.TrimSpace(exe) == "" {
		return false
	}
	exeDir := filepath.Dir(exe)
	for _, dir := range goBinDirs(goAvailable) {
		if sameInstallDir(exeDir, dir) {
			return true
		}
	}
	return false
}

// goBinDirs returns every directory where `go install` may place binaries:
// GOBIN (from `go env` or the process env), <GOPATH>/bin for each GOPATH
// entry, and the ~/go/bin default. Empty values are skipped.
func goBinDirs(goAvailable bool) []string {
	var dirs []string
	add := func(dir string) {
		if strings.TrimSpace(dir) != "" {
			dirs = append(dirs, dir)
		}
	}
	addGopathBins := func(gopath string) {
		for _, entry := range filepath.SplitList(gopath) {
			if strings.TrimSpace(entry) != "" {
				add(filepath.Join(entry, "bin"))
			}
		}
	}

	// `go env` is authoritative: it covers values persisted via `go env -w`,
	// which never appear in the process environment.
	if goAvailable {
		if gobin, err := goEnvFn("GOBIN"); err == nil {
			add(gobin)
		}
		if gopath, err := goEnvFn("GOPATH"); err == nil {
			addGopathBins(gopath)
		}
	}
	add(os.Getenv("GOBIN"))
	addGopathBins(os.Getenv("GOPATH"))
	if home, err := os.UserHomeDir(); err == nil {
		add(filepath.Join(home, "go", "bin"))
	}
	return dirs
}

// sameInstallDir reports whether two directory paths are equal after cleaning,
// case-insensitively on Windows.
func sameInstallDir(a, b string) bool {
	a, b = filepath.Clean(a), filepath.Clean(b)
	if pathCaseInsensitive {
		return strings.EqualFold(a, b)
	}
	return a == b
}
