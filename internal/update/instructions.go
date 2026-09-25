package update

import (
	"errors"
	"fmt"
	"strings"

	"github.com/gentleman-programming/gentle-ai/v3/internal/system"
)

const WindowsDistributionHoldMessage = "Windows binary distribution and Scoop are temporarily unavailable until publicly trusted Authenticode signing is enforced."

// GentleAISourceInstallCommand returns the safe source-install fallback for an
// exact release or the latest release when version is empty. Beta builds
// require validated commit metadata; use ValidatedBetaSourceInstallCommand.
// The /vN suffix for releases is derived via ModulePathForVersion.
func GentleAISourceInstallCommand(version string) string {
	target := "latest"
	version = strings.TrimSpace(version)
	if strings.HasPrefix(version, "main@") {
		return ""
	}
	if version != "" {
		target = "v" + strings.TrimPrefix(version, "v")
	}
	module := ModulePathForVersion("github.com/gentleman-programming/gentle-ai/cmd/gentle-ai", "gentle-ai", version)
	return "go install " + module + "@" + target
}

// BetaSourceInstallCommand composes a source install from metadata already
// validated against the exact checked commit. It never follows mutable main.
func BetaSourceInstallCommand(module, commit string) string {
	return "go install " + module + "/cmd/gentle-ai@" + commit
}

// ValidatedBetaSourceInstallCommand rejects unchecked or inconsistent beta metadata.
func ValidatedBetaSourceInstallCommand(r UpdateResult) (string, error) {
	if !isGentleAIRepo(r.Tool) || !validBetaCommit(r.BetaCommit) ||
		!validBetaModulePath(r.BetaModulePath, r.Tool.Owner, r.Tool.Repo) ||
		r.LatestVersion != "main@"+shortCommit(r.BetaCommit) {
		return "", errors.New("beta target metadata is missing or inconsistent; rerun the update check")
	}
	return BetaSourceInstallCommand(r.BetaModulePath, r.BetaCommit), nil
}

// updateHint returns a platform-specific instruction string for updating the given tool.
func updateHint(tool ToolInfo, profile system.PlatformProfile) string {
	switch tool.Name {
	case "gentle-ai":
		return gentleAIHint(profile)
	case "engram":
		return engramHint(profile)
	case "gga":
		return ggaHint(profile)
	case "opencode-subagent-statusline", "opencode-sdd-engram-manage":
		return "gentle-ai upgrade updates ~/.config/opencode npm deps, clears this plugin's @latest cache, then requires OpenCode restart/reload"
	default:
		return ""
	}
}

func updateHintForOwnership(tool ToolInfo, profile system.PlatformProfile, ownership HomebrewOwnership) string {
	if profile.PackageManager == "brew" && ownership != HomebrewNone {
		return fmt.Sprintf("brew upgrade --%s %s", ownership, tool.Name)
	}
	return updateHint(tool, profile)
}

func openCodeRegisteredNotMaterializedHint(tool ToolInfo) string {
	pkg := strings.TrimSpace(tool.NpmPackage)
	if pkg == "" {
		pkg = tool.Name
	}
	return fmt.Sprintf("registered in ~/.config/opencode/tui.json; pending npm dependency materialization for %s. Run gentle-ai upgrade to install/update ~/.config/opencode dependencies, then restart or reload OpenCode; if it stays pending, check OpenCode logs for package or peer dependency errors.", pkg)
}

// gentleAIHint is the stable-channel instruction only. When the checker
// resolves a main-head beta target, applyBetaMainHeadStatus overrides this
// hint with GentleAISourceInstallCommand so the printed instruction installs
// the advertised target instead of the latest stable release.
func gentleAIHint(profile system.PlatformProfile) string {
	if profile.PackageManager == "brew" && homebrewPackageInstalled("gentle-ai") {
		return "brew upgrade gentle-ai"
	}

	switch profile.OS {
	case "linux":
		return "curl -fsSL https://raw.githubusercontent.com/Gentleman-Programming/gentle-ai/main/scripts/install.sh | bash"
	case "darwin":
		return "gentle-ai upgrade (downloads pre-built binary)"
	case "windows":
		return WindowsDistributionHoldMessage + " Install/update from source with Go 1.25.10+: " + GentleAISourceInstallCommand("")
	default:
		return ""
	}
}

func engramHint(profile system.PlatformProfile) string {
	if profile.PackageManager == "brew" && homebrewPackageInstalled("engram") {
		return "brew upgrade engram"
	}
	return "gentle-ai upgrade (downloads pre-built binary)"
}

func ggaHint(profile system.PlatformProfile) string {
	if profile.PackageManager == "brew" && homebrewPackageInstalled("gga") {
		return "brew upgrade gga"
	}
	return "See https://github.com/Gentleman-Programming/gentleman-guardian-angel"
}
