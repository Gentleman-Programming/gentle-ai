package update

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// Package-level vars for testability (swap in tests via t.Cleanup).
var (
	execCommand    = exec.Command
	lookPath       = exec.LookPath
	userHomeDir    = os.UserHomeDir
	osStat         = os.Stat
	osGetenv       = os.Getenv
	powershellPath = "powershell" // overridable in tests
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
	if strings.TrimSpace(tool.NpmPackage) != "" {
		return detectNpmPackageVersion(tool.NpmPackage)
	}

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

	// On Windows, exec.Command (CreateProcess) cannot execute a .ps1 script
	// directly — it is not an executable image. Wrap via powershell -File so
	// the OS can launch the PowerShell host and pass the script to it.
	// Outside Windows, .ps1 files don't exist in fallback paths, so this
	// branch is unreachable in practice on Linux/macOS.
	execBinary, execArgs := buildExecCmd(binary, tool.DetectCmd[1:])
	cmd := execCommand(execBinary, execArgs...)

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

// buildExecCmd returns the executable name and arguments to use when running a
// version-detect command. On Windows, PowerShell scripts (.ps1) cannot be
// passed as argv[0] to CreateProcess — they must be launched via the
// PowerShell host. For a .ps1 binary we therefore rewrite:
//
//	("C:\Users\...\gga.ps1", ["--version"])
//	→ ("powershell", ["-NoProfile", "-File", "C:\Users\...\gga.ps1", "--version"])
//
// For all other binaries (real PE executables on Windows, any file on
// Linux/macOS), the arguments are returned unchanged.
func buildExecCmd(binary string, remainingArgs []string) (string, []string) {
	if strings.EqualFold(filepath.Ext(binary), ".ps1") {
		args := make([]string, 0, 3+len(remainingArgs))
		args = append(args, "-NoProfile", "-File", binary)
		args = append(args, remainingArgs...)
		return powershellPath, args
	}
	return binary, remainingArgs
}

func detectNpmPackageVersion(pkg string) string {
	version, _ := detectOpenCodePluginPackage(pkg)
	return version
}

func detectOpenCodePluginPackage(pkg string) (string, bool) {
	home, err := userHomeDir()
	if err != nil || strings.TrimSpace(home) == "" {
		return "", false
	}
	opencodeDir := filepath.Join(home, ".config", "opencode")
	data, err := os.ReadFile(filepath.Join(opencodeDir, "node_modules", pkg, "package.json"))
	if err != nil {
		if version, ok := openCodePackageJSONDependencyVersion(opencodeDir, pkg); ok {
			return version, false
		}
		return "", isOpenCodePluginRegistered(opencodeDir, pkg)
	}
	var manifest struct {
		Version string `json:"version"`
	}
	if err := json.Unmarshal(data, &manifest); err != nil {
		return "", isOpenCodePluginRegistered(opencodeDir, pkg)
	}
	return parseVersionFromOutput(manifest.Version), false
}

func openCodePackageJSONDependencyVersion(opencodeDir, pkg string) (string, bool) {
	data, err := os.ReadFile(filepath.Join(opencodeDir, "package.json"))
	if err != nil || strings.TrimSpace(string(data)) == "" {
		return "", false
	}

	var manifest struct {
		Dependencies    map[string]string `json:"dependencies"`
		DevDependencies map[string]string `json:"devDependencies"`
	}
	if err := json.Unmarshal(data, &manifest); err != nil {
		return "", false
	}

	if version, ok := manifest.Dependencies[pkg]; ok {
		return parseVersionFromOutput(version), true
	}
	if version, ok := manifest.DevDependencies[pkg]; ok {
		return parseVersionFromOutput(version), true
	}
	return "", false
}

func isOpenCodePluginRegistered(opencodeDir, pkg string) bool {
	data, err := os.ReadFile(filepath.Join(opencodeDir, "tui.json"))
	if err != nil || strings.TrimSpace(string(data)) == "" {
		return false
	}

	var root struct {
		Plugin []string `json:"plugin"`
	}
	if err := json.Unmarshal(data, &root); err != nil {
		return false
	}

	for _, plugin := range root.Plugin {
		if strings.TrimSpace(plugin) == pkg {
			return true
		}
	}
	return false
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
