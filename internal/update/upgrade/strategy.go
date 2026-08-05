package upgrade

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"

	"github.com/gentleman-programming/gentle-ai/v2/internal/cli"
	"github.com/gentleman-programming/gentle-ai/v2/internal/components/engram"
	"github.com/gentleman-programming/gentle-ai/v2/internal/system"
	"github.com/gentleman-programming/gentle-ai/v2/internal/update"
)

// engramDownloadFn is the function used to download the engram binary on the stable channel.
// Package-level var for testability — swapped in tests to avoid real network calls.
var engramDownloadFn = func(profile system.PlatformProfile) (string, error) {
	return engram.DownloadLatestBinary(profile, false)
}

// engramBetaInstallFn installs engram from HEAD via `go install @main` (beta channel).
// It delegates to engram.DownloadLatestBinary(profile, true), which is the single
// canonical beta path shared with the install-time flow. Package-level var for
// testability — swapped in tests to avoid real go install/network calls.
var engramBetaInstallFn = func(profile system.PlatformProfile) (string, error) {
	return engram.DownloadLatestBinary(profile, true)
}

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
//   - binary method + windows → manualFallback (gentle-ai explains the signed-distribution hold)
//   - script method + linux/darwin + gga → ggaScriptUpgrade (git clone approach)
//   - script method + linux/darwin + other → scriptUpgrade (curl | bash install.sh)
//   - script method + windows → manualFallback
//   - OpenCode plugin method → update materialized package in ~/.config/opencode when possible
//   - unknown method → manualFallback with explicit message
func runStrategy(ctx context.Context, r update.UpdateResult, profile system.PlatformProfile, preflightDestination ...string) (bool, error) {
	ownership := update.HomebrewNone
	if profile.PackageManager == "brew" && r.Tool.InstallMethod != update.InstallOpenCodePlugin {
		var err error
		ownership, err = homebrewOwnershipDetector(r.Tool.Name)
		if err != nil {
			return false, fmt.Errorf("detect Homebrew ownership for %s: %w", r.Tool.Name, err)
		}
	}
	if isBetaGentleAIUpgrade(r) && profile.OS != "windows" && ownership == update.HomebrewNone {
		return false, goInstallMainUpgrade(r.Tool)
	}

	method := effectiveMethod(r.Tool, profile)
	if ownership != update.HomebrewNone {
		method = update.InstallBrew
	}

	switch method {
	case update.InstallBrew:
		return false, brewUpgrade(ctx, r, ownership)
	case update.InstallGoInstall:
		return false, goInstallUpgrade(ctx, r, profile, firstString(preflightDestination))
	case update.InstallBinary:
		return false, binaryUpgrade(ctx, r, profile)
	case update.InstallScript:
		// GGA's install.sh expects to run from within a cloned repo — it references
		// $SCRIPT_DIR/bin/gga and $SCRIPT_DIR/lib/*.sh. The generic scriptUpgrade
		// only downloads and runs the script in isolation (bash -c <content>), which
		// breaks because those relative paths don't exist. Use the git clone approach
		// (same as the initial install resolver) for GGA specifically.
		if r.Tool.Name == "gga" {
			return false, ggaScriptUpgrade(ctx, r)
		}
		return false, scriptUpgrade(ctx, r, profile)
	case update.InstallOpenCodePlugin:
		return false, opencodePluginUpgrade(ctx, r)
	default:
		return false, &ManualFallbackError{
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
		outStr := string(out)
		if pm == "npm" && npmErrorCode(outStr) == "ERESOLVE" {
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
			}
			fmt.Fprintln(os.Stderr, "WARNING: npm reported an ERESOLVE peer dependency conflict; retrying with --legacy-peer-deps")
			retryCmd := execCommand("npm", append([]string{"install", "--save", "--no-audit", "--no-fund", "--legacy-peer-deps"}, targets...)...)
			retryCmd.Dir = opencodeDir
			retryCmd.Stdin = nil
			retryCmd.Env = openCodePluginUpgradeEnv(retryCmd.Env)
			if retryOut, retryErr := retryCmd.CombinedOutput(); retryErr != nil {
				return fmt.Errorf("%s upgrade %s in %s: retry with --legacy-peer-deps failed: %w (retry output: %s); original error: %v (original output: %s)", pm, pkg, opencodeDir, retryErr, string(retryOut), err, outStr)
			}
		} else {
			return fmt.Errorf("%s upgrade %s in %s: %w (output: %s)", pm, pkg, opencodeDir, err, outStr)
		}
	}
	if err := clearOpenCodePluginPackageCache(homeDir, pkg); err != nil {
		return fmt.Errorf("clear OpenCode package cache for %s: %w", pkg, err)
	}
	return nil
}

func npmErrorCode(output string) string {
	for _, line := range strings.Split(output, "\n") {
		fields := strings.Fields(line)
		if len(fields) == 4 && fields[0] == "npm" && fields[1] == "error" && fields[2] == "code" {
			return fields[3]
		}
	}
	return ""
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
func brewUpgrade(ctx context.Context, r update.UpdateResult, ownership update.HomebrewOwnership) error {
	toolName := r.Tool.Name
	flag := "--" + string(ownership)
	// Ensure the Gentleman-Programming homebrew tap is present before upgrading.
	// Non-fatal: brew tap is a no-op when already present; if it fails for any other
	// reason, the subsequent brew upgrade will surface the real error. See issue #455:
	// without this, a lost tap (untap, machine swap, brew cleanup) makes upgrades fail
	// with "No available formula" for engram/gga/gentle-ai.
	tapCmd := execCommand("brew", "tap", "Gentleman-Programming/homebrew-tap")
	tapCmd.Stdin = nil
	_ = tapCmd.Run()

	// Trust only the Gentleman Programming artifact being upgraded. Homebrew 6 can
	// require explicit trust for non-official taps; this is intentionally scoped to
	// our formula/cask, not the whole tap or third-party taps. Older Homebrew versions
	// may not support `brew trust`, so this is non-fatal and the upgrade output
	// below remains the source of truth.
	trustCmd := execCommand("brew", "trust", flag, gentlemanProgrammingTapRef(toolName))
	trustCmd.Stdin = nil
	_ = trustCmd.Run()

	// Update Homebrew formula cache before upgrading.
	// Non-fatal: if update fails (e.g. no network), attempt upgrade with existing cache.
	updateCmd := execCommand("brew", "update")
	updateCmd.Stdin = nil
	_ = updateCmd.Run() // ignore error intentionally

	upgradeCmd := execCommand("brew", "upgrade", flag, toolName)
	upgradeCmd.Stdin = nil
	if out, err := upgradeCmd.CombinedOutput(); err != nil {
		return formatBrewUpgradeError(toolName, ownership, err, string(out))
	}
	if ownership == update.HomebrewCask {
		return verifyLegacyCaskTarget(r)
	}
	return nil
}

var brewVersionRegexp = regexp.MustCompile(`\d+\.\d+(?:\.\d+)?(?:-[0-9A-Za-z]+(?:[.-][0-9A-Za-z]+)*)?(?:\+[0-9A-Za-z]+(?:[.-][0-9A-Za-z]+)*)?`)

func verifyLegacyCaskTarget(r update.UpdateResult) error {
	migration := legacyCaskMigration(r.Tool.Name)
	if len(r.Tool.DetectCmd) == 0 {
		return fmt.Errorf("cannot verify Homebrew cask %s after upgrade; %s", r.Tool.Name, migration)
	}
	cmd := execCommand(r.Tool.DetectCmd[0], r.Tool.DetectCmd[1:]...)
	out, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("verify Homebrew cask %s after upgrade: %w; %s", r.Tool.Name, err, migration)
	}
	installed := brewVersionRegexp.FindString(string(out))
	target := brewVersionRegexp.FindString(r.LatestVersion)
	if installed == "" || target == "" || installed != target {
		return fmt.Errorf("Homebrew cask %s remains at %q after targeting %q; %s", r.Tool.Name, installed, target, migration)
	}
	return nil
}

func gentlemanProgrammingTapRef(toolName string) string {
	return "gentleman-programming/tap/" + strings.TrimSpace(toolName)
}

func homebrewTrustFlag(toolName string) string {
	if strings.TrimSpace(toolName) == "engram" {
		return "--cask"
	}
	return "--formula"
}

func legacyCaskMigration(toolName string) string {
	return fmt.Sprintf("migrate the legacy cask to the current formula:\n  brew uninstall --cask %s\n  brew install --formula %s", strings.TrimSpace(toolName), gentlemanProgrammingTapRef(toolName))
}

func formatBrewUpgradeError(toolName string, ownership update.HomebrewOwnership, err error, output string) error {
	message := fmt.Sprintf("brew upgrade --%s %s: %v (output: %s)", ownership, toolName, err, output)
	if advice := homebrewFailureAdvice(toolName, output, ownership); advice != "" {
		message += "\n\n" + advice
	}
	if ownership == update.HomebrewCask {
		message += "\n\n" + legacyCaskMigration(toolName)
	}
	return errors.New(message)
}

func homebrewFailureAdvice(toolName string, output string, detected ...update.HomebrewOwnership) string {
	lower := strings.ToLower(output)
	ref := gentlemanProgrammingTapRef(toolName)
	flag := homebrewTrustFlag(toolName)
	if len(detected) > 0 {
		flag = "--" + string(detected[0])
	}

	if strings.Contains(lower, "untrusted tap") || strings.Contains(lower, "tap trust is required") || strings.Contains(lower, "homebrew_require_tap_trust") {
		artifact := strings.TrimPrefix(flag, "--")
		if strings.Contains(lower, "--cask") || strings.Contains(lower, "load cask") {
			flag = "--cask"
			artifact = "cask"
		} else if strings.Contains(lower, "--formula") || strings.Contains(lower, "load formula") {
			flag = "--formula"
			artifact = "formula"
		}
		return fmt.Sprintf("Homebrew requires explicit trust for external taps. Trust only this Gentle AI %s, then retry:\n  brew trust %s %s\n  brew upgrade %s %s", artifact, flag, ref, flag, toolName)
	}

	if strings.Contains(lower, "bubblewrap is installed but cannot create a rootless sandbox") ||
		strings.Contains(lower, "rootless sandbox") ||
		strings.Contains(lower, "homebrew_no_sandbox_linux") {
		return "Homebrew on Linux could not create its Bubblewrap rootless sandbox. This requires an explicit admin/security decision: enabling unprivileged user namespaces lets Homebrew use its sandbox but changes host kernel/AppArmor policy. If acceptable, run:\n  sudo sysctl -w kernel.unprivileged_userns_clone=1\n  sudo sysctl -w user.max_user_namespaces=28633\n  sudo sysctl -w kernel.apparmor_restrict_unprivileged_userns=0 || true\n\nFinal workaround if your distro policy forbids this sandbox:\n  HOMEBREW_NO_SANDBOX_LINUX=1 brew upgrade " + flag + " " + toolName
	}

	return ""
}

// goInstallUpgrade runs `go install <importPath>@v<version>`.
//
// Generic Go-managed tools retain the post-install warning because the new
// binary was genuinely written. Windows gentle-ai self-upgrades are different:
// they must prove that Go owns the active executable before writing, or skip to
// a manual recovery instead of creating a second PATH-visible binary.
func goInstallUpgrade(ctx context.Context, r update.UpdateResult, profile system.PlatformProfile, preflightDestination string) error {
	tool := r.Tool
	latestVersion := r.LatestVersion
	if tool.GoImportPath == "" {
		return fmt.Errorf("upgrade %q: GoImportPath is empty — cannot run go install", tool.Name)
	}

	// GOBIN/GOPATH are static Go configuration that `go install` does not
	// change, so they are read up front; the PATH lookup happens afterwards so
	// a first-time install resolves correctly.
	destDir := preflightDestination
	var destErr error
	if destDir == "" {
		destDir, destErr = goInstallDestinationDir()
		if err := preflightWindowsGentleAIGoInstallWithDestination(r, profile, destDir, destErr); err != nil {
			return err
		}
	}

	// Pin to the exact release version.
	target := fmt.Sprintf("%s@v%s", tool.GoImportPath, latestVersion)
	cmd := execCommand("go", "install", target)
	cmd.Stdin = nil
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("go install %s: %w (output: %s)", target, err, string(out))
	}

	warnGoInstallDestination(tool.Name, detectOS(), destDir, destErr)
	return nil
}

func preflightWindowsGentleAIGoInstall(r update.UpdateResult, profile system.PlatformProfile) (string, error) {
	if profile.OS != "windows" || r.Tool.Name != "gentle-ai" {
		return "", nil
	}
	destDir, destErr := goInstallDestinationDir()
	if err := preflightWindowsGentleAIGoInstallWithDestination(r, profile, destDir, destErr); err != nil {
		return "", err
	}
	return destDir, nil
}

func firstString(values []string) string {
	if len(values) == 0 {
		return ""
	}
	return values[0]
}

func preflightWindowsGentleAIGoInstallWithDestination(r update.UpdateResult, profile system.PlatformProfile, destDir string, destErr error) error {
	if profile.OS != "windows" || r.Tool.Name != "gentle-ai" {
		return nil
	}
	if destErr != nil {
		return &ManualFallbackError{Hint: gentleAIWindowsGoInstallProvenanceHint(r, "", "")}
	}

	destination := absoluteBinaryPath(filepath.Join(destDir, goInstallBinaryName(r.Tool.Name, profile.OS)))
	active, err := lookPathFn(r.Tool.Name)
	if err != nil {
		return &ManualFallbackError{Hint: gentleAIWindowsGoInstallProvenanceHint(r, destination, "")}
	}
	active = absoluteBinaryPath(active)
	if !sameBinaryPathForOS(destination, active, profile.OS) {
		return &ManualFallbackError{Hint: gentleAIWindowsGoInstallProvenanceHint(r, destination, active)}
	}
	return nil
}

func gentleAIWindowsGoInstallProvenanceHint(r update.UpdateResult, destination, active string) string {
	details := "could not determine the Go installation destination"
	switch {
	case destination != "" && active == "":
		details = fmt.Sprintf("could not resolve the active gentle-ai executable before Go would write to %s", destination)
	case destination != "" && active != "":
		details = fmt.Sprintf("resolves gentle-ai to %s, but Go would write to %s", active, destination)
	}

	hint := fmt.Sprintf("Windows self-upgrade %s. No files were changed. ", details)
	if active != "" {
		hint += fmt.Sprintf("Keep %s as the active installation, or intentionally migrate to %s with:\n  ", active, destination)
	} else {
		hint += "Confirm the active installation, then intentionally migrate with:\n  "
	}
	hint += update.GentleAISourceInstallCommand(r.LatestVersion)
	if destination != "" {
		hint += fmt.Sprintf("\nAfter a successful migration, ensure only %s resolves for gentle-ai on PATH.", destination)
	}
	return hint
}

func isBetaGentleAIUpgrade(r update.UpdateResult) bool {
	return r.Tool.Name == "gentle-ai" &&
		strings.EqualFold(r.Tool.Owner, "Gentleman-Programming") &&
		r.Tool.Repo == "gentle-ai" &&
		strings.HasPrefix(strings.TrimSpace(r.LatestVersion), "main@")
}

// goInstallMainUpgrade installs gentle-ai from HEAD on the beta channel. It runs
// the same `go install` mechanism as goInstallUpgrade and therefore carries the
// same risk of writing somewhere the shell does not resolve, so it performs the
// same non-fatal destination verification.
func goInstallMainUpgrade(tool update.ToolInfo) error {
	repository := strings.ToLower(fmt.Sprintf("github.com/%s/%s", strings.TrimSpace(tool.Owner), strings.TrimSpace(tool.Repo)))
	if repository == "github.com//" {
		repository = "github.com/gentleman-programming/gentle-ai"
	}
	// Go derives the module path from the repository plus the major-version
	// suffix: for major 2 and above the module path must end in /vN or the
	// toolchain refuses every resolution of that repository, including the
	// branch pseudo-versions this beta path installs.
	module := repository + "/v2"

	destDir, destErr := goInstallDestinationDir()

	target := module + "/cmd/gentle-ai@main"
	cmd := execCommand("go", "install", target)
	cmd.Stdin = nil
	cmd.Env = goProxyBypassEnv(cmd.Env, module)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("go install %s: %w (output: %s)", target, err, strings.TrimSpace(string(out)))
	}

	warnGoInstallDestination(tool.Name, detectOS(), destDir, destErr)
	return nil
}

func goProxyBypassEnv(base []string, module string) []string {
	if base == nil {
		base = os.Environ()
	}
	env := append([]string{}, base...)
	for _, key := range []string{"GONOSUMDB", "GOPRIVATE", "GONOPROXY"} {
		env = setEnvValue(env, key, prependGoPattern(getEnvValue(env, key), module))
	}
	return env
}

func getEnvValue(env []string, key string) string {
	prefix := key + "="
	for i := len(env) - 1; i >= 0; i-- {
		if strings.HasPrefix(env[i], prefix) {
			return strings.TrimPrefix(env[i], prefix)
		}
	}
	return ""
}

func setEnvValue(env []string, key, value string) []string {
	prefix := key + "="
	for i := len(env) - 1; i >= 0; i-- {
		if strings.HasPrefix(env[i], prefix) {
			env[i] = prefix + value
			return env
		}
	}
	return append(env, prefix+value)
}

func prependGoPattern(existing, pattern string) string {
	pattern = strings.TrimSpace(pattern)
	if pattern == "" {
		return existing
	}
	parts := strings.Split(existing, ",")
	for _, part := range parts {
		if strings.TrimSpace(part) == pattern {
			return existing
		}
	}
	if strings.TrimSpace(existing) == "" {
		return pattern
	}
	return pattern + "," + existing
}

// binaryUpgrade handles binary-release upgrades via GitHub Releases asset download.
//
// engram has its own cross-platform binary downloader (DownloadLatestBinary) that
// works on all platforms including Windows. Other Windows binary upgrades return
// ManualFallbackError so the executor surfaces them as UpgradeSkipped.
func binaryUpgrade(ctx context.Context, r update.UpdateResult, profile system.PlatformProfile) error {
	if profile.OS == "windows" && r.Tool.Name == "gentle-ai" {
		return &ManualFallbackError{Hint: gentleAIWindowsSourceInstallHint(r)}
	}

	// engram: always use its dedicated binary downloader regardless of platform
	// (except brew, which is handled by effectiveMethod before we get here).
	if r.Tool.Name == "engram" {
		return engramBinaryUpgrade(profile)
	}

	if profile.OS == "windows" {
		// Windows binary auto-upgrade is not supported for generic tools yet.
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

func gentleAIWindowsSourceInstallHint(r update.UpdateResult) string {
	return update.WindowsDistributionHoldMessage + " " +
		"No binary or remote script was downloaded or executed. Install/update from source with Go 1.25.10+:\n  " +
		update.GentleAISourceInstallCommand(r.LatestVersion)
}

// engramBinaryUpgrade downloads or installs the latest engram binary.
// It honors GENTLE_AI_CHANNEL: when the channel is beta, engram is installed
// from source via `go install @main`. For stable (the default when the env var
// is unset or unknown), the pre-built release binary is downloaded via
// engramDownloadFn. On Windows, PATH changes are persisted to the user registry
// via PowerShell.
func engramBinaryUpgrade(profile system.PlatformProfile) error {
	// Resolve the install channel from the environment. Unknown values fall back
	// to stable (ResolveInstallChannel returns an error for truly unrecognized
	// values; we treat those as stable and emit a warning so users are not silently
	// misrouted).
	channel, err := cli.ResolveInstallChannel("")
	if err != nil {
		fmt.Fprintf(os.Stderr, "WARNING: unrecognized GENTLE_AI_CHANNEL value (%v); defaulting to stable\n", err)
		channel = cli.ChannelStable
	}

	var binaryPath string
	if channel.IsBeta() {
		// Beta channel: install engram from HEAD via engramBetaInstallFn, which
		// delegates to engram.DownloadLatestBinary(profile, true). This is the
		// single canonical beta path shared with the install-time flow in
		// internal/cli/run.go (installBetaEngramFromMain). The previous inline
		// `go install` block is removed — all beta logic lives in download.go.
		binaryPath, err = engramBetaInstallFn(profile)
		if err != nil {
			return fmt.Errorf("install engram from main (beta): %w", err)
		}
	} else {
		// Stable channel (default): download the latest release binary.
		binaryPath, err = engramDownloadFn(profile)
		if err != nil {
			return fmt.Errorf("download engram binary: %w", err)
		}
	}

	// Add install dir to PATH. On Windows this also persists via PowerShell (user registry).
	binDir := filepath.Dir(binaryPath)
	if err := system.AddToUserPath(binDir); err != nil {
		// Non-fatal: the binary was downloaded or installed successfully. Warn and continue.
		fmt.Fprintf(os.Stderr, "WARNING: could not add %s to PATH: %v\n", binDir, err)
	}
	return nil
}

// downloadAndReplace downloads the release asset and atomically replaces the binary.
// Implemented in download.go.
func downloadAndReplace(ctx context.Context, r update.UpdateResult, profile system.PlatformProfile) error {
	return Download(ctx, r, profile)
}

// installScriptURLFn builds the raw GitHub URL for the project's install.sh,
// pinned to the given release tag (e.g. "1.31.0" → ref "v1.31.0").
// Package-level var for testability.
var installScriptURLFn = func(owner, repo, version string) (string, error) {
	if strings.TrimSpace(version) == "" {
		return "", fmt.Errorf("install script URL: target version must not be empty")
	}
	return fmt.Sprintf("https://raw.githubusercontent.com/%s/%s/v%s/install.sh",
		owner, repo, version), nil
}

// installScriptURL builds the raw GitHub URL for the project's install.sh,
// pinned to the given release tag so the upgrade path never pulls from main.
func installScriptURL(owner, repo, version string) (string, error) {
	return installScriptURLFn(owner, repo, version)
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

	url, err := installScriptURL(r.Tool.Owner, r.Tool.Repo, r.LatestVersion)
	if err != nil {
		return fmt.Errorf("download install.sh: %w", err)
	}
	fmt.Fprintf(os.Stderr, "INFO: downloading install script from %s\n", url)

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

	// Fetch and check out the exact release tag so the install.sh
	// executed here matches the version the user is upgrading TO, not whatever
	// is on main at the moment of the upgrade or a homonymous branch ref.
	targetTagRef := "refs/tags/v" + r.LatestVersion
	repoURL := fmt.Sprintf("https://github.com/%s/%s.git", r.Tool.Owner, r.Tool.Repo)

	initCmd := execCommand("git", "init", tmpDir)
	initCmd.Stdin = nil
	if out, err := initCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git init %s: %w (output: %s)", r.Tool.Repo, err, strings.TrimSpace(string(out)))
	}

	fetchCmd := execCommand("git", "-C", tmpDir, "fetch", "--depth=1", repoURL, targetTagRef+":"+targetTagRef)
	fetchCmd.Stdin = nil
	if out, err := fetchCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git fetch %s: %w (output: %s)", r.Tool.Repo, err, strings.TrimSpace(string(out)))
	}

	checkoutCmd := execCommand("git", "-C", tmpDir, "checkout", "-f", targetTagRef)
	checkoutCmd.Stdin = nil
	if out, err := checkoutCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git checkout %s: %w (output: %s)", r.Tool.Repo, err, strings.TrimSpace(string(out)))
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
