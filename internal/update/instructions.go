package update

import (
	"fmt"
	"strings"

	"github.com/gentleman-programming/gentle-ai/v3/internal/system"
)

const WindowsDistributionHoldMessage = "Windows binary distribution and Scoop are temporarily unavailable until publicly trusted Authenticode signing is enforced."

// SourceInstallCommand returns the actionable install/update instruction for a
// tool at an exact release, a beta main build, or the latest release when
// version is empty. It emits a `go install` ONLY when the tool's declared module
// makes that command resolvable (REQ-22.2); otherwise it emits a clone-and-build
// instruction naming the tool's own Owner/Repo and never names upstream (D-04).
func SourceInstallCommand(tool ToolInfo, version string) string {
	target := sourceInstallTarget(version)
	if tool.GoInstallResolvable() {
		return "go install " + tool.GoImportPath + "@" + target
	}
	return sourceCloneBuildCommand(tool, target)
}

// sourceInstallTarget maps a checked version onto the install target token:
// "main" for a beta main-head build, "v<X>" for an exact release, and "latest"
// when no version is advertised.
func sourceInstallTarget(version string) string {
	version = strings.TrimSpace(version)
	if strings.HasPrefix(version, "main@") {
		return "main"
	}
	if version != "" {
		return "v" + strings.TrimPrefix(version, "v")
	}
	return "latest"
}

// sourceCloneBuildCommand returns the manual clone-and-build instruction used
// when `go install` is not resolvable. It names the tool's own repository and
// ./cmd/<Name> package and contains zero `go install` (REQ-22.2). The empty
// "latest" target clones the repository's default branch; exact releases and
// the beta channel pin the branch explicitly.
func sourceCloneBuildCommand(tool ToolInfo, target string) string {
	clone := "git clone "
	if target != "latest" {
		clone += "--branch " + target + " "
	}
	clone += "https://github.com/" + tool.Owner + "/" + tool.Repo
	return clone + " && cd " + tool.Repo + " && go build -o " + tool.Name + " ./cmd/" + tool.Name
}

// updateHint returns a platform-specific instruction string for updating the given tool.
func updateHint(tool ToolInfo, profile system.PlatformProfile) string {
	if IsSelfTool(tool) {
		return gentleAIHint(tool, profile)
	}
	switch tool.Name {
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
// hint with SourceInstallCommand so the printed instruction installs
// the advertised target instead of the latest stable release. Every value is
// derived from tool; no upstream coordinate is hardcoded (D-04).
func gentleAIHint(tool ToolInfo, profile system.PlatformProfile) string {
	if profile.PackageManager == "brew" && homebrewPackageInstalled(tool.Name) {
		return "brew upgrade " + tool.Name
	}

	switch profile.OS {
	case "linux":
		return "curl -fsSL https://raw.githubusercontent.com/" + tool.Owner + "/" + tool.Repo + "/main/scripts/install.sh | bash"
	case "darwin":
		return tool.Name + " upgrade (downloads pre-built binary)"
	case "windows":
		return WindowsDistributionHoldMessage + " Install/update from source with Go 1.25.10+: " + SourceInstallCommand(tool, "")
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
