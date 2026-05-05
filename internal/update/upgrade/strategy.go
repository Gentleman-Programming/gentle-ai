package upgrade

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
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

var (
	openCodeHomeDir = os.UserHomeDir
	lookPathCommand = exec.LookPath
)

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
//   - OpenCode plugin method → update materialized package in ~/.config/opencode when possible
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
	case update.InstallOpenCodePlugin:
		return opencodePluginUpgrade(ctx, r)
	default:
		return &ManualFallbackError{
			Hint: fmt.Sprintf("upgrade %q: unsupported install method %q — please update manually. See: https://github.com/Gentleman-Programming/%s",
				r.Tool.Name, method, r.Tool.Repo),
		}
	}
}

func opencodePluginUpgrade(ctx context.Context, r update.UpdateResult) error {
	pkg := strings.TrimSpace(r.Tool.NpmPackage)
	if pkg == "" {
		return &ManualFallbackError{Hint: openCodePluginManualHint(r)}
	}

	homeDir, err := openCodeHomeDir()
	if err != nil || strings.TrimSpace(homeDir) == "" {
		return &ManualFallbackError{Hint: fmt.Sprintf("%s Could not resolve the user home directory; update %s manually.", openCodePluginManualHint(r), pkg)}
	}

	opencodeDir := filepath.Join(homeDir, ".config", "opencode")
	info, err := os.Stat(opencodeDir)
	if err != nil || !info.IsDir() {
		return &ManualFallbackError{Hint: fmt.Sprintf("%s OpenCode config directory was not found at %s; %s is not installed/materialized yet.", openCodePluginManualHint(r), opencodeDir, pkg)}
	}

	materialized, registered, err := openCodePluginRegisteredOrMaterialized(opencodeDir, pkg)
	if err != nil {
		return fmt.Errorf("inspect OpenCode plugin %s: %w", pkg, err)
	}
	if !materialized && !registered && r.Status != update.RegisteredNotMaterialized {
		return &ManualFallbackError{Hint: fmt.Sprintf("%s %s is not registered in tui.json and is not present in node_modules; start/reload OpenCode first so it materializes the plugin.", openCodePluginManualHint(r), pkg)}
	}

	pm, err := selectOpenCodePackageManager(opencodeDir)
	if err != nil {
		return &ManualFallbackError{Hint: fmt.Sprintf("OpenCode plugin %s can be upgraded from %s, but no supported package manager is available in PATH. Install bun or npm, then run update tools again.", pkg, opencodeDir)}
	}

	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	targets := []string{pkg + "@latest", "@opencode-ai/plugin@latest"}
	var cmd *exec.Cmd
	switch pm {
	case "bun":
		cmd = execCommand("bun", append([]string{"add"}, targets...)...)
	case "npm":
		cmd = execCommand("npm", append([]string{"install", "--save", "--no-audit", "--no-fund"}, targets...)...)
	default:
		return &ManualFallbackError{Hint: fmt.Sprintf("unsupported OpenCode package manager %q for %s", pm, pkg)}
	}
	cmd.Dir = opencodeDir
	cmd.Stdin = nil
	cmd.Env = openCodePluginUpgradeEnv(cmd.Env)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("%s upgrade %s in %s: %w (output: %s)", pm, pkg, opencodeDir, err, string(out))
	}
	if err := clearOpenCodePluginPackageCache(homeDir, pkg); err != nil {
		return fmt.Errorf("clear OpenCode package cache for %s: %w", pkg, err)
	}
	return nil
}

func openCodePluginRegisteredOrMaterialized(opencodeDir, pkg string) (bool, bool, error) {
	if info, err := os.Stat(filepath.Join(opencodeDir, "node_modules", pkg)); err == nil && info.IsDir() {
		return true, false, nil
	}
	if _, ok := openCodePackageJSONDependencyVersion(opencodeDir, pkg); ok {
		return false, true, nil
	}

	data, err := os.ReadFile(filepath.Join(opencodeDir, "tui.json"))
	if err != nil {
		if os.IsNotExist(err) {
			return false, false, nil
		}
		return false, false, err
	}
	if strings.TrimSpace(string(data)) == "" {
		return false, false, nil
	}

	var root struct {
		Plugin []string `json:"plugin"`
	}
	if err := json.Unmarshal(data, &root); err != nil {
		return false, false, fmt.Errorf("parse tui.json: %w", err)
	}
	for _, plugin := range root.Plugin {
		if strings.TrimSpace(plugin) == pkg {
			return false, true, nil
		}
	}
	return false, false, nil
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
		return version, true
	}
	if version, ok := manifest.DevDependencies[pkg]; ok {
		return version, true
	}
	return "", false
}

func clearOpenCodePluginPackageCache(homeDir, pkg string) error {
	return os.RemoveAll(filepath.Join(homeDir, ".cache", "opencode", "packages", pkg+"@latest"))
}

func selectOpenCodePackageManager(opencodeDir string) (string, error) {
	candidates := make([]string, 0, 2)
	if pm := openCodePackageManagerFromMetadata(opencodeDir); pm != "" {
		candidates = append(candidates, pm)
	}
	for _, pm := range []string{"bun", "npm"} {
		if !stringInSlice(candidates, pm) {
			candidates = append(candidates, pm)
		}
	}

	for _, pm := range candidates {
		if _, err := lookPathCommand(pm); err == nil {
			return pm, nil
		}
	}
	return "", fmt.Errorf("bun/npm not found")
}

func openCodePackageManagerFromMetadata(opencodeDir string) string {
	packageJSON := filepath.Join(opencodeDir, "package.json")
	if data, err := os.ReadFile(packageJSON); err == nil {
		var manifest struct {
			PackageManager string `json:"packageManager"`
		}
		if json.Unmarshal(data, &manifest) == nil {
			pm := strings.ToLower(strings.TrimSpace(manifest.PackageManager))
			switch {
			case strings.HasPrefix(pm, "bun@") || pm == "bun":
				return "bun"
			case strings.HasPrefix(pm, "npm@") || pm == "npm":
				return "npm"
			}
		}
	}

	for _, lockfile := range []string{"bun.lock", "bun.lockb"} {
		if _, err := os.Stat(filepath.Join(opencodeDir, lockfile)); err == nil {
			return "bun"
		}
	}
	for _, lockfile := range []string{"package-lock.json", "npm-shrinkwrap.json"} {
		if _, err := os.Stat(filepath.Join(opencodeDir, lockfile)); err == nil {
			return "npm"
		}
	}
	return ""
}

func openCodePluginUpgradeEnv(existing []string) []string {
	env := existing
	if len(env) == 0 {
		env = os.Environ()
	}
	return append(env,
		"CI=1",
		"npm_config_yes=true",
		"npm_config_audit=false",
		"npm_config_fund=false",
	)
}

func stringInSlice(items []string, item string) bool {
	for _, candidate := range items {
		if candidate == item {
			return true
		}
	}
	return false
}

func openCodePluginManualHint(r update.UpdateResult) string {
	if strings.TrimSpace(r.UpdateHint) != "" {
		return r.UpdateHint
	}
	if strings.TrimSpace(r.Tool.NpmPackage) != "" {
		return fmt.Sprintf("OpenCode manages %s from tui.json. Restart or reload OpenCode so it refreshes the plugin package.", r.Tool.NpmPackage)
	}
	return "OpenCode manages TUI plugin packages from tui.json. Restart or reload OpenCode so it refreshes plugins."
}

func openCodePluginRegisteredPendingHint(pkg string) string {
	return fmt.Sprintf("OpenCode plugin %s is registered in ~/.config/opencode/tui.json but is not materialized in node_modules yet. Restart or reload OpenCode to materialize it; if it remains pending, check OpenCode logs for package or peer dependency errors before retrying upgrade.", pkg)
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
