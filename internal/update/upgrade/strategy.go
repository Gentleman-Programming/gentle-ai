package upgrade

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/gentleman-programming/gentle-ai/internal/components/engram"
	"github.com/gentleman-programming/gentle-ai/internal/system"
	"github.com/gentleman-programming/gentle-ai/internal/update"
)

// engramDownloadFn is the function used to download the engram binary.
// Package-level var for testability — swapped in tests to avoid real network calls.
var engramDownloadFn = engram.DownloadLatestBinary

// execCommand is a package-level var declared in executor.go (same package).

// scriptHTTPClient is the HTTP client used for downloading install.sh.
// Package-level var for testability.
var scriptHTTPClient = &http.Client{Timeout: 2 * time.Minute}

// maxScriptSize is the maximum number of bytes read from a downloaded install.sh.
// This prevents unbounded memory use if the server returns an unexpectedly large body.
// Note: HTTPS provides transport security but NOT content integrity — a compromised
// server or CDN could still serve a malicious script within this size limit.
const maxScriptSize = 1 * 1024 * 1024 // 1 MB

// runStrategy executes the upgrade for a single tool using the appropriate strategy
// for the given platform profile.
//
// Strategy routing:
//   - brew profile → brewUpgrade (regardless of tool's declared method)
//   - go-install method + apt/pacman/other → goInstallUpgrade
//   - binary method + linux/darwin → binaryUpgrade
//   - binary method + windows → manualFallback (Phase 1: self-replace deferred)
//   - script method + linux/darwin + gga → ggaScriptUpgrade (git clone approach)
//   - script method + linux/darwin + other → scriptUpgrade (curl | bash install.sh)
//   - script method + windows → manualFallback
//   - unknown method → manualFallback with explicit message
func runStrategy(ctx context.Context, r update.UpdateResult, profile system.PlatformProfile) error {
	method := effectiveMethod(r.Tool, profile)

	switch method {
	case update.InstallBrew:
		return brewUpgrade(ctx, r.Tool.Name)
	case update.InstallGoInstall:
		return goInstallUpgrade(ctx, r.Tool, r.LatestVersion)
	case update.InstallBinary:
		return binaryUpgrade(ctx, r, profile)
	case update.InstallScript:
		// GGA's install.sh expects to run from within a cloned repo — it references
		// $SCRIPT_DIR/bin/gga and $SCRIPT_DIR/lib/*.sh. The generic scriptUpgrade
		// only downloads and runs the script in isolation (bash -c <content>), which
		// breaks because those relative paths don't exist. Use the git clone approach
		// (same as the initial install resolver) for GGA specifically.
		if r.Tool.Name == "gga" {
			return ggaScriptUpgrade(ctx, r)
		}
		return scriptUpgrade(ctx, r, profile)
	default:
		return &ManualFallbackError{
			Hint: fmt.Sprintf("upgrade %q: unsupported install method %q — please update manually. See: https://github.com/Gentleman-Programming/%s",
				r.Tool.Name, method, r.Tool.Repo),
		}
	}
}

// brewUpgrade runs `brew update` (non-fatal) then `brew upgrade <toolName>`.
//
// brew update refreshes the local formula cache so that Homebrew is aware of
// new versions published since the user last ran it. If update fails (e.g. no
// network), the upgrade is still attempted using the existing cache — a stale
// cache is better than no upgrade at all.
func brewUpgrade(ctx context.Context, toolName string) error {
	// Update Homebrew formula cache before upgrading.
	// Non-fatal: if update fails (e.g. no network), attempt upgrade with existing cache.
	updateCmd := execCommand("brew", "update")
	updateCmd.Stdin = nil
	_ = updateCmd.Run() // ignore error intentionally

	upgradeCmd := execCommand("brew", "upgrade", toolName)
	upgradeCmd.Stdin = nil
	if out, err := upgradeCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("brew upgrade %s: %w (output: %s)", toolName, err, string(out))
	}
	return nil
}

// goInstallUpgrade runs `go install <importPath>@v<version>`.
func goInstallUpgrade(ctx context.Context, tool update.ToolInfo, latestVersion string) error {
	if tool.GoImportPath == "" {
		return fmt.Errorf("upgrade %q: GoImportPath is empty — cannot run go install", tool.Name)
	}

	// Pin to the exact release version.
	target := fmt.Sprintf("%s@v%s", tool.GoImportPath, latestVersion)
	cmd := execCommand("go", "install", target)
	cmd.Stdin = nil
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("go install %s: %w (output: %s)", target, err, string(out))
	}
	return nil
}

// binaryUpgrade handles binary-release upgrades via GitHub Releases asset download.
//
// engram has its own cross-platform binary downloader (DownloadLatestBinary) that
// works on all platforms including Windows. For all other tools on Windows,
// self-replace of a running binary is deferred (Phase 1) — a ManualFallbackError
// is returned so the executor surfaces it as UpgradeSkipped with an actionable hint.
func binaryUpgrade(ctx context.Context, r update.UpdateResult, profile system.PlatformProfile) error {
	// engram: always use its dedicated binary downloader regardless of platform
	// (except brew, which is handled by effectiveMethod before we get here).
	if r.Tool.Name == "engram" {
		return engramBinaryUpgrade(profile)
	}

	if profile.OS == "windows" {
		// Phase 1: Windows binary self-replace is deferred for non-engram tools.
		// Return a ManualFallbackError so the executor surfaces this as UpgradeSkipped
		// with an actionable hint — NOT as UpgradeFailed.
		hint := r.UpdateHint
		if hint == "" {
			hint = fmt.Sprintf("Download manually from https://github.com/Gentleman-Programming/%s/releases", r.Tool.Repo)
		}
		return &ManualFallbackError{
			Hint: fmt.Sprintf("upgrade %q on Windows requires manual update: %s", r.Tool.Name, hint),
		}
	}

	// For Linux/macOS binary installs: delegate to the download package.
	return downloadAndReplace(ctx, r, profile)
}

// engramBinaryUpgrade downloads the latest engram binary using its dedicated
// cross-platform downloader and adds the install directory to PATH.
// On Windows the PATH change is also persisted to the user registry via PowerShell.
func engramBinaryUpgrade(profile system.PlatformProfile) error {
	binaryPath, err := engramDownloadFn(profile)
	if err != nil {
		return fmt.Errorf("download engram binary: %w", err)
	}
	// Add install dir to PATH. On Windows this also persists via PowerShell (user registry).
	binDir := filepath.Dir(binaryPath)
	if err := system.AddToUserPath(binDir); err != nil {
		// Non-fatal: the binary was downloaded successfully. Warn and continue.
		fmt.Fprintf(os.Stderr, "WARNING: could not add %s to PATH: %v\n", binDir, err)
	}
	return nil
}

// downloadAndReplace downloads the release asset and atomically replaces the binary.
// Implemented in download.go.
func downloadAndReplace(ctx context.Context, r update.UpdateResult, profile system.PlatformProfile) error {
	return Download(ctx, r, profile)
}

// installScriptURLFn builds the raw GitHub URL for the project's install.sh.
// Package-level var for testability.
var installScriptURLFn = func(owner, repo string) string {
	return fmt.Sprintf("https://raw.githubusercontent.com/%s/%s/main/install.sh",
		owner, repo)
}

// installScriptURL builds the raw GitHub URL for the project's install.sh.
func installScriptURL(owner, repo string) string {
	return installScriptURLFn(owner, repo)
}

// scriptUpgrade downloads and executes the project's install.sh via curl | bash.
// This is used for tools that distribute via shell scripts (e.g., GGA) rather than
// pre-built release binary assets.
//
// The script is downloaded to a temp file, then executed with bash and stdin set to nil
// so it runs non-interactively (no prompts). This assumes the install.sh handles the
// non-interactive case gracefully (e.g., auto-reinstalls when already installed).
func scriptUpgrade(ctx context.Context, r update.UpdateResult, profile system.PlatformProfile) error {
	if profile.OS == "windows" {
		hint := r.UpdateHint
		if hint == "" {
			hint = fmt.Sprintf("Download manually from https://github.com/%s/%s/releases", r.Tool.Owner, r.Tool.Repo)
		}
		return &ManualFallbackError{
			Hint: fmt.Sprintf("upgrade %q on Windows requires manual update: %s", r.Tool.Name, hint),
		}
	}

	url := installScriptURL(r.Tool.Owner, r.Tool.Repo)

	// Download install.sh content.
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("download install.sh: build request: %w", err)
	}

	resp, err := scriptHTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("download install.sh: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download install.sh: HTTP %d from %s", resp.StatusCode, url)
	}

	scriptBody, err := io.ReadAll(io.LimitReader(resp.Body, maxScriptSize+1))
	if err != nil {
		return fmt.Errorf("download install.sh: read body: %w", err)
	}
	if int64(len(scriptBody)) > maxScriptSize {
		return fmt.Errorf("download install.sh: response body exceeds %d bytes limit", maxScriptSize)
	}

	// Execute install.sh with bash. Stdin is nil to ensure non-interactive mode.
	cmd := execCommand("bash", "-c", string(scriptBody))
	cmd.Stdin = nil
	if out, err := cmd.CombinedOutput(); err != nil {
		// Provide a helpful hint if the script fails.
		output := strings.TrimSpace(string(out))
		return fmt.Errorf("install.sh failed for %q: %w\nOutput: %s", r.Tool.Name, err, output)
	}

	return nil
}

// ggaMkdirTemp is the function used to create a temporary directory for GGA git clone.
// Package-level var for testability — swapped in tests to control the temp dir path.
var ggaMkdirTemp = func() (string, error) {
	return os.MkdirTemp("", "gentle-ai-gga-*")
}

// ggaScriptUpgrade upgrades GGA by cloning its repository and running install.sh
// from within the cloned repo — the same approach used by the initial install resolver.
//
// This is required because GGA's install.sh references $SCRIPT_DIR/bin/gga and
// $SCRIPT_DIR/lib/*.sh (relative to the cloned repo). The generic scriptUpgrade
// downloads and runs the script in isolation via `bash -c <content>`, which fails
// because those relative paths don't exist without the full repo context.
//
// On Windows, bash is not available — returns ManualFallbackError.
func ggaScriptUpgrade(ctx context.Context, r update.UpdateResult) error {
	return ggaScriptUpgradeForOS(ctx, r, detectOS())
}

// detectOS returns the current runtime OS name. Package-level var for testability.
var detectOS = func() string {
	return runtime.GOOS
}

// ggaScriptUpgradeForOS is the testable version of ggaScriptUpgrade that accepts
// an explicit OS string so tests can simulate Windows without actually running on it.
func ggaScriptUpgradeForOS(ctx context.Context, r update.UpdateResult, osName string) error {
	if osName == "windows" {
		hint := r.UpdateHint
		if hint == "" {
			hint = fmt.Sprintf("Download manually from https://github.com/%s/%s/releases", r.Tool.Owner, r.Tool.Repo)
		}
		return &ManualFallbackError{
			Hint: fmt.Sprintf("upgrade %q on Windows requires manual update: %s", r.Tool.Name, hint),
		}
	}

	// Use an unpredictable temp directory to avoid TOCTOU races on the fixed path.
	tmpDir, err := ggaMkdirTemp()
	if err != nil {
		return fmt.Errorf("create temp dir for gga clone: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	// Clone the full repository — install.sh needs the entire repo context.
	repoURL := fmt.Sprintf("https://github.com/%s/%s.git", r.Tool.Owner, r.Tool.Repo)
	cloneCmd := execCommand("git", "clone", repoURL, tmpDir)
	cloneCmd.Stdin = nil
	if out, err := cloneCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git clone %s: %w (output: %s)", r.Tool.Repo, err, strings.TrimSpace(string(out)))
	}

	// Execute install.sh from within the cloned repo (non-interactive).
	installScript := filepath.Join(tmpDir, "install.sh")
	installCmd := execCommand("bash", installScript)
	installCmd.Stdin = nil
	if out, err := installCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("install.sh failed for %q: %w\nOutput: %s", r.Tool.Name, err, strings.TrimSpace(string(out)))
	}

	return nil
}
