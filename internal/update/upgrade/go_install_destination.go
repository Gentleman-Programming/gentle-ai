package upgrade

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/gentleman-programming/gentle-ai/v3/internal/system"
	"github.com/gentleman-programming/gentle-ai/v3/internal/update"
)

// `go install` writes the built binary to GOBIN, or to the first GOPATH entry's
// bin/ directory when GOBIN is unset. That location is not necessarily the one
// the user's shell resolves for the tool: a stale copy earlier on PATH keeps
// running after a successful install, which looks exactly like an upgrade that
// silently did nothing.
//
// The helpers below resolve both sides and describe the difference. They never
// turn a successful install into a failure — the new binary really was written,
// so the worst honest outcome is a successful upgrade carrying a warning, and
// the second-worst is a successful upgrade the tool could not verify.

// goEnvValue reads a single `go env` variable through the execCommand seam.
func goEnvValue(key string) (string, error) {
	cmd := execCommand("go", "env", key)
	cmd.Stdin = nil
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("go env %s: %w", key, err)
	}
	return strings.TrimSpace(string(out)), nil
}

// goInstallDestinationDir resolves the directory `go install` writes into:
// GOBIN when set, otherwise the first GOPATH entry plus "bin".
func goInstallDestinationDir() (string, error) {
	gobin, err := goEnvValue("GOBIN")
	if err != nil {
		return "", err
	}
	if gobin != "" {
		return gobin, nil
	}

	gopath, err := goEnvValue("GOPATH")
	if err != nil {
		return "", err
	}
	entries := filepath.SplitList(gopath)
	if len(entries) == 0 || strings.TrimSpace(entries[0]) == "" {
		return "", fmt.Errorf("go env reported neither GOBIN nor GOPATH")
	}
	return filepath.Join(entries[0], "bin"), nil
}

// goInstallBinaryName returns the on-disk file name `go install` produces.
func goInstallBinaryName(toolName, osName string) string {
	if osName == "windows" {
		return toolName + ".exe"
	}
	return toolName
}

// sameBinaryPathForOS reports whether two paths designate the same executable.
// Symlinks are resolved when they can be, so a ~/.local/bin shim pointing into
// the go-install directory counts as a match. Windows comparison is
// case-insensitive and tolerates the ".exe" suffix on either side.
//
// osName is an explicit parameter rather than runtime.GOOS so Windows
// comparison semantics stay testable from any host.
func sameBinaryPathForOS(a, b, osName string) bool {
	a = normalizeBinaryPathForOS(a, osName)
	b = normalizeBinaryPathForOS(b, osName)
	if a == "" || b == "" {
		return false
	}
	return a == b
}

func normalizeBinaryPathForOS(path, osName string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	// Windows accepts both separators inside one path, but filepath.Clean only
	// applies the *host* separator rules: on a non-Windows test host a
	// backslash path never collapses onto its slash twin. Fold the separators
	// explicitly whenever the compared platform is Windows so the comparison
	// is byte-identical on every host (osName is a parameter for exactly this).
	if osName == "windows" {
		path = strings.ReplaceAll(path, "\\", "/")
	}
	path = absoluteBinaryPath(path)
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		path = resolved
	}
	path = filepath.Clean(path)
	if osName == "windows" {
		path = strings.ToLower(path)
		path = strings.TrimSuffix(path, ".exe")
	}
	return path
}

func absoluteBinaryPath(path string) string {
	if abs, err := filepath.Abs(path); err == nil {
		return abs
	}
	return path
}

// goInstallDestinationNotice compares where `go install` wrote against the
// binary the shell resolves for toolName, and returns the message the user
// needs. An empty string means the destination was verified as a match.
//
// destDir/destErr come from goInstallDestinationDir, resolved before the
// install runs: GOBIN and GOPATH are static Go configuration that `go install`
// does not change, while the PATH lookup below is deliberately performed after
// the install so a first-time install is resolved correctly.
func goInstallDestinationNotice(toolName, osName, destDir string, destErr error) string {
	if destErr != nil {
		return unverifiedDestinationNotice(toolName, "", destErr.Error())
	}

	installed := absoluteBinaryPath(filepath.Join(destDir, goInstallBinaryName(toolName, osName)))
	if _, err := os.Stat(installed); err != nil {
		// Claiming a mismatch means claiming `go install` wrote a binary here.
		// Without an observable file that claim is unfounded, so report the
		// honest weaker result instead of fabricating either outcome.
		return unverifiedDestinationNotice(toolName, "", fmt.Sprintf("no binary was found at the expected go-install destination %s: %v", installed, err))
	}

	effective, err := lookPathFn(toolName)
	if err != nil {
		return unverifiedDestinationNotice(toolName, installed, fmt.Sprintf("%s could not be resolved on PATH: %v", toolName, err))
	}
	effective = absoluteBinaryPath(effective)

	if sameBinaryPathForOS(installed, effective, osName) {
		return ""
	}

	return fmt.Sprintf(
		"`go install` wrote the new %s to %s, but your shell still resolves %s to %s — that stale binary will keep running. Fix it by putting %s ahead of %s on PATH, or by replacing/relinking %s so it points at %s.",
		toolName, installed,
		toolName, effective,
		filepath.Dir(installed), filepath.Dir(effective),
		effective, installed,
	)
}

func unverifiedDestinationNotice(toolName, installed, reason string) string {
	notice := fmt.Sprintf("%s was upgraded with `go install`, but its destination could not be verified: %s.", toolName, reason)
	if installed != "" {
		notice += fmt.Sprintf(" `go install` writes to %s.", installed)
	}
	return notice + fmt.Sprintf(" The install itself succeeded; confirm that the directory reported by `go env GOBIN` (or `go env GOPATH` plus /bin) is the one your shell resolves for %s.", toolName)
}

// warnGoInstallDestination emits the destination notice, if any, as a non-fatal
// warning. It never returns an error: the install already succeeded.
func warnGoInstallDestination(toolName, osName, destDir string, destErr error) {
	if notice := goInstallDestinationNotice(toolName, osName, destDir, destErr); notice != "" {
		fmt.Fprintf(os.Stderr, "WARNING: %s\n", notice)
	}
}

// preflightWindowsSelfBinaryWrite is the single provenance gate for every
// upgrade path that writes the self-tool binary on Windows (D-03, T-5).
// It resolves where the write would land (GOBIN/GOPATH) and where the shell
// currently resolves the binary, and refuses the upgrade when they differ —
// preventing a second PATH-visible binary (REQ-22.1).
//
// Returns nil when the tool is not the self-tool or the platform is not
// Windows. On success returns the resolved destination directory.
func preflightWindowsSelfBinaryWrite(tool update.ToolInfo, profile system.PlatformProfile) (string, error) {
	if profile.OS != "windows" || !update.IsSelfTool(tool) {
		return "", nil
	}
	destDir, destErr := goInstallDestinationDir()
	if destErr != nil {
		return "", &ManualFallbackError{Hint: selfBinaryWriteProvenanceHint(tool, "", "")}
	}
	destination := absoluteBinaryPath(filepath.Join(destDir, goInstallBinaryName(tool.Name, profile.OS)))
	active, err := lookPathFn(tool.Name)
	if err != nil {
		return "", &ManualFallbackError{Hint: selfBinaryWriteProvenanceHint(tool, destination, "")}
	}
	active = absoluteBinaryPath(active)
	if !sameBinaryPathForOS(destination, active, profile.OS) {
		return "", &ManualFallbackError{Hint: selfBinaryWriteProvenanceHint(tool, destination, active)}
	}
	return destDir, nil
}

// selfBinaryWriteProvenanceHint builds the manual-fallback message for a
// refused Windows self-binary write. It names both paths and the intentional
// migration command (D-03).
func selfBinaryWriteProvenanceHint(tool update.ToolInfo, destination, active string) string {
	details := "could not determine the Go installation destination"
	switch {
	case destination != "" && active == "":
		details = fmt.Sprintf("could not resolve the active %s executable before Go would write to %s", tool.Name, destination)
	case destination != "" && active != "":
		details = fmt.Sprintf("resolves %s to %s, but Go would write to %s", tool.Name, active, destination)
	}

	hint := fmt.Sprintf("Windows self-upgrade %s. No files were changed. ", details)
	if active != "" {
		hint += fmt.Sprintf("Keep %s as the active installation, or intentionally migrate to %s with:\n  ", active, destination)
	} else {
		hint += "Confirm the active installation, then intentionally migrate with:\n  "
	}
	hint += update.SourceInstallCommand(tool, "")
	if destination != "" {
		hint += fmt.Sprintf("\nAfter a successful migration, ensure only %s resolves for %s on PATH.", destination, tool.Name)
	}
	return hint
}
