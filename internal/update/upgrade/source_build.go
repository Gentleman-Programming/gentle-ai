package upgrade

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/gentleman-programming/gentle-ai/v3/internal/system"
	"github.com/gentleman-programming/gentle-ai/v3/internal/update"
)

// sourceBuildMkdirTemp creates the unpredictable temp directory for the source
// clone. Package-level var for testability.
var sourceBuildMkdirTemp = func() (string, error) {
	return os.MkdirTemp("", "axiom-srcbuild-*")
}

// sourceBuildUpgrade compiles the self-tool from a controlled source clone and
// atomically replaces the active binary (REQ-22.1, D-02). It is the resilient
// Windows path when `go install` is not resolvable.
//
// Steps:
//  1. Provenance preflight (preflightWindowsSelfBinaryWrite).
//  2. Shallow clone of the exact target ref into an unpredictable temp dir.
//  3. go build -trimpath into the temp dir.
//  4. Atomic replace of the active binary.
//
// Any failure before step 4 commits returns a ManualFallbackError naming the
// fork and the correct manual path, with zero files touched (T-2, T-7).
func sourceBuildUpgrade(ctx context.Context, r update.UpdateResult, profile system.PlatformProfile, targetRef string) error {
	tool := r.Tool

	// Step 1: provenance preflight — refuse before touching anything (D-03).
	if _, err := preflightWindowsSelfBinaryWrite(tool, profile); err != nil {
		return err
	}

	if err := ctx.Err(); err != nil {
		return sourceBuildManualFallback(tool, r.LatestVersion, "context cancelled before clone")
	}

	// Pre-flight tool availability so we fail without side effects.
	if _, err := lookPathCommand("git"); err != nil {
		return sourceBuildManualFallback(tool, r.LatestVersion, "git is not available on PATH")
	}
	if _, err := lookPathCommand("go"); err != nil {
		return sourceBuildManualFallback(tool, r.LatestVersion, "go is not available on PATH")
	}

	// Step 2: shallow clone into an unpredictable temp dir (T-2).
	tmpDir, err := sourceBuildMkdirTemp()
	if err != nil {
		return sourceBuildManualFallback(tool, r.LatestVersion, fmt.Sprintf("create temp dir: %v", err))
	}
	defer os.RemoveAll(tmpDir)

	repoURL := fmt.Sprintf("https://github.com/%s/%s.git", tool.Owner, tool.Repo)

	initCmd := execCommand("git", "init", tmpDir)
	initCmd.Stdin = nil
	if out, err := initCmd.CombinedOutput(); err != nil {
		return sourceBuildManualFallback(tool, r.LatestVersion,
			fmt.Sprintf("git init: %v (output: %s)", err, strings.TrimSpace(string(out))))
	}

	fetchCmd := execCommand("git", "-C", tmpDir, "fetch", "--depth=1", repoURL, targetRef+":"+targetRef)
	fetchCmd.Stdin = nil
	if out, err := fetchCmd.CombinedOutput(); err != nil {
		return sourceBuildManualFallback(tool, r.LatestVersion,
			fmt.Sprintf("git fetch %s: %v (output: %s)", targetRef, err, strings.TrimSpace(string(out))))
	}

	checkoutCmd := execCommand("git", "-C", tmpDir, "checkout", "-f", targetRef)
	checkoutCmd.Stdin = nil
	if out, err := checkoutCmd.CombinedOutput(); err != nil {
		return sourceBuildManualFallback(tool, r.LatestVersion,
			fmt.Sprintf("git checkout %s: %v (output: %s)", targetRef, err, strings.TrimSpace(string(out))))
	}

	if err := ctx.Err(); err != nil {
		return sourceBuildManualFallback(tool, r.LatestVersion, "context cancelled before build")
	}

	// Step 3: build inside the clone. Binary lands in the temp dir — never in
	// the user's tree and never on PATH until the atomic replace (T-2).
	binaryName := goInstallBinaryName(tool.Name, profile.OS)
	stagedPath := filepath.Join(tmpDir, binaryName)

	buildCmd := execCommand("go", "build", "-trimpath", "-o", stagedPath, "./cmd/"+tool.Name)
	buildCmd.Dir = tmpDir
	buildCmd.Stdin = nil
	if out, err := buildCmd.CombinedOutput(); err != nil {
		// Remove any partial output before reporting (no half-built binary).
		_ = os.Remove(stagedPath)
		return sourceBuildManualFallback(tool, r.LatestVersion,
			fmt.Sprintf("go build: %v (output: %s)", err, strings.TrimSpace(string(out))))
	}

	if err := ctx.Err(); err != nil {
		return sourceBuildManualFallback(tool, r.LatestVersion, "context cancelled before replace")
	}

	// Step 4: atomic replace of the active binary — the only mutation (D-03).
	active, err := lookPathFn(tool.Name)
	if err != nil {
		return sourceBuildManualFallback(tool, r.LatestVersion,
			fmt.Sprintf("resolve active %s executable: %v", tool.Name, err))
	}
	active = absoluteBinaryPath(active)

	if err := atomicReplace(stagedPath, active); err != nil {
		return fmt.Errorf("replace %s: %w", active, err)
	}
	return nil
}

// sourceBuildManualFallback builds the ManualFallbackError for a failed source
// build. It names the fork and the correct manual path (REQ-22.1).
func sourceBuildManualFallback(tool update.ToolInfo, version, reason string) error {
	return &ManualFallbackError{
		Hint: fmt.Sprintf(
			"source build upgrade for %s failed: %s. No files were changed. Update manually with:\n  %s",
			tool.Name, reason, update.SourceInstallCommand(tool, version),
		),
	}
}
