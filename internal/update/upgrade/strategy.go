package upgrade

import (
	"context"
	"fmt"

	"github.com/gentleman-programming/gentle-ai/internal/system"
	"github.com/gentleman-programming/gentle-ai/internal/update"
)

// execCommand is a package-level var declared in executor.go (same package).

// runStrategy executes the upgrade for a single tool using the appropriate strategy
// for the given platform profile.
//
// Strategy routing:
//   - brew profile → brewUpgrade (regardless of tool's declared method)
//   - go-install method + apt/pacman/other → goInstallUpgrade
//   - binary method + linux/darwin → binaryUpgrade
//   - binary method + windows → manualFallback (Phase 1: self-replace deferred)
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
// On Windows, self-replace of a running binary is not safe — we return a ManualFallbackError.
func binaryUpgrade(ctx context.Context, r update.UpdateResult, profile system.PlatformProfile) error {
	if profile.OS == "windows" {
		// Phase 1: Windows binary self-replace is deferred.
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

// downloadAndReplace downloads the release asset and atomically replaces the binary.
// Implemented in download.go.
func downloadAndReplace(ctx context.Context, r update.UpdateResult, profile system.PlatformProfile) error {
	return Download(ctx, r, profile)
}
