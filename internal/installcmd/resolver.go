package installcmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/gentleman-programming/gentle-ai/internal/model"
	"github.com/gentleman-programming/gentle-ai/internal/system"
)

// cmdLookPath, osStat, osGetenv, and cmdGoVersion are package-level vars for testability.
var cmdLookPath = exec.LookPath
var osStat = os.Stat
var osGetenv = os.Getenv
var cmdGoVersion = func() ([]byte, error) {
	return exec.Command("go", "version").Output()
}

// CommandSequence represents an ordered list of commands to run in sequence.
// Each inner slice is a single command with its arguments (e.g., ["brew", "install", "engram"]).
// Multi-step installs (e.g., tap + install) are expressed as multiple entries.
type CommandSequence = [][]string

type Resolver interface {
	ResolveAgentInstall(profile system.PlatformProfile, agent model.AgentID) (CommandSequence, error)
	ResolveComponentInstall(profile system.PlatformProfile, component model.ComponentID) (CommandSequence, error)
	ResolveDependencyInstall(profile system.PlatformProfile, dependency string) (CommandSequence, error)
}

type profileResolver struct{}

func NewResolver() Resolver {
	return profileResolver{}
}

func (profileResolver) ResolveAgentInstall(profile system.PlatformProfile, agent model.AgentID) (CommandSequence, error) {
	switch agent {
	case model.AgentClaudeCode:
		return resolveClaudeCodeInstall(profile), nil
	case model.AgentOpenCode:
		return resolveOpenCodeInstall(profile)
	case model.AgentKilocode:
		return resolveKilocodeInstall(profile), nil
	case model.AgentKimi:
		return resolveKimiInstall(profile)
	default:
		return nil, fmt.Errorf("install command is not supported for agent %q", agent)
	}
}

// resolveClaudeCodeInstall returns the npm install command sequence for Claude Code.
// On Linux with system npm, sudo is required. With nvm/fnm/volta, it is not.
// On Windows and macOS, sudo is never needed.
func resolveClaudeCodeInstall(profile system.PlatformProfile) CommandSequence {
	if profile.OS == "linux" && !profile.NpmWritable {
		return CommandSequence{{"sudo", "npm", "install", "-g", "@anthropic-ai/claude-code"}}
	}
	return CommandSequence{{"npm", "install", "-g", "@anthropic-ai/claude-code"}}
}

// resolveKilocodeInstall returns the npm install command sequence for Kilocode.
// On Linux with system npm, sudo is required. With nvm/fnm/volta, it is not.
// On Windows and macOS, sudo is never needed.
func resolveKilocodeInstall(profile system.PlatformProfile) CommandSequence {
	if profile.OS == "linux" && !profile.NpmWritable {
		return CommandSequence{{"sudo", "npm", "install", "-g", "@kilocode/cli"}}
	}
	return CommandSequence{{"npm", "install", "-g", "@kilocode/cli"}}
}

// resolveKimiInstall returns the official Kimi install command sequence.
// To avoid the security risks of pipe-to-shell patterns (curl | bash),
// we execute the underlying command that the scripts alias: `uv tool install`.
func resolveKimiInstall(profile system.PlatformProfile) (CommandSequence, error) {
	// Kimi CLI is a python-based tool. We use Astral's `uv` as our deterministic
	// prerequisite manager to ensure secure and isolated installs.
	if !profile.Supported {
		return nil, fmt.Errorf("Kimi is not supported on this platform (%s/%s)", profile.OS, profile.LinuxDistro)
	}

	// We explicitly request python 3.13 as strictly defined by Kimi upstream.
	return CommandSequence{{"uv", "tool", "install", "--python", "3.13", "kimi-cli"}}, nil
}

// ValidateAgentInstallPreflight validates agent-specific prerequisites that must
// exist before running installation commands.
func ValidateAgentInstallPreflight(profile system.PlatformProfile, agent model.AgentID) error {
	switch agent {
	case model.AgentKimi:
		return validateKimiInstallPreflight(profile)
	default:
		return nil
	}
}

func validateKimiInstallPreflight(profile system.PlatformProfile) error {
	if !profile.Supported {
		return fmt.Errorf("Kimi is not supported on this platform (%s/%s)", profile.OS, profile.LinuxDistro)
	}

	if _, err := cmdLookPath("uv"); err != nil {
		return fmt.Errorf(
			"Kimi requires Astral uv, but `uv` was not found in PATH.\n"+
				"Install uv and retry:\n"+
				"  %s",
			uvInstallHint(profile),
		)
	}

	return nil
}

func uvInstallHint(profile system.PlatformProfile) string {
	switch profile.PackageManager {
	case "brew":
		return "brew install uv"
	case "apt":
		return "sudo apt-get install -y uv (or see https://docs.astral.sh/uv/getting-started/installation/)"
	case "pacman":
		return "sudo pacman -S --noconfirm uv"
	case "dnf":
		return "sudo dnf install -y uv"
	case "winget":
		return "winget install --id astral-sh.uv -e --accept-source-agreements --accept-package-agreements"
	default:
		return "https://docs.astral.sh/uv/getting-started/installation/"
	}
}

func (profileResolver) ResolveComponentInstall(profile system.PlatformProfile, component model.ComponentID) (CommandSequence, error) {
	switch component {
	case model.ComponentEngram:
		return resolveEngramInstall(profile)
	case model.ComponentGGA:
		return resolveGGAInstall(profile)
	default:
		return nil, fmt.Errorf("install command is not supported for component %q", component)
	}
}

func (profileResolver) ResolveDependencyInstall(profile system.PlatformProfile, dependency string) (CommandSequence, error) {
	if dependency == "" {
		return nil, fmt.Errorf("dependency name is required")
	}

	switch profile.PackageManager {
	case "brew":
		return CommandSequence{{"brew", "install", dependency}}, nil
	case "apt":
		return CommandSequence{{"sudo", "apt-get", "install", "-y", dependency}}, nil
	case "pacman":
		return CommandSequence{{"sudo", "pacman", "-S", "--noconfirm", dependency}}, nil
	case "dnf":
		return CommandSequence{{"sudo", "dnf", "install", "-y", dependency}}, nil
	case "winget":
		return CommandSequence{{"winget", "install", "--id", dependency, "-e", "--accept-source-agreements", "--accept-package-agreements"}}, nil
	default:
		return nil, fmt.Errorf(
			"unsupported package manager %q for os=%q distro=%q",
			profile.PackageManager,
			profile.OS,
			profile.LinuxDistro,
		)
	}
}

// resolveOpenCodeInstall returns the correct install command sequence for OpenCode per platform.
// - darwin: brew install anomalyco/tap/opencode (official OpenCode tap)
// - linux: npm install -g opencode-ai (official npm package)
// See https://opencode.ai/docs for official install methods.
func resolveOpenCodeInstall(profile system.PlatformProfile) (CommandSequence, error) {
	switch profile.PackageManager {
	case "brew":
		return CommandSequence{
			{"brew", "install", "anomalyco/tap/opencode"},
		}, nil
	case "apt", "pacman", "dnf":
		if profile.NpmWritable {
			return CommandSequence{{"npm", "install", "-g", "opencode-ai"}}, nil
		}
		return CommandSequence{{"sudo", "npm", "install", "-g", "opencode-ai"}}, nil
	case "winget":
		// On Windows, npm global installs do not require sudo.
		return CommandSequence{{"npm", "install", "-g", "opencode-ai"}}, nil
	default:
		return nil, fmt.Errorf(
			"unsupported platform for opencode: os=%q distro=%q pm=%q",
			profile.OS, profile.LinuxDistro, profile.PackageManager,
		)
	}
}

// resolveGGAInstall returns the correct install command sequence for GGA per platform.
// - darwin: brew tap + brew install (via Gentleman-Programming/homebrew-tap)
// - linux: git clone + install.sh (GGA is a pure Bash project, NOT a Go module)
func resolveGGAInstall(profile system.PlatformProfile) (CommandSequence, error) {
	switch profile.PackageManager {
	case "brew":
		return CommandSequence{
			{"brew", "tap", "Gentleman-Programming/homebrew-tap"},
			{"brew", "reinstall", "gga"},
		}, nil
	case "apt", "pacman", "dnf":
		const tmpDir = "/tmp/gentleman-guardian-angel"
		return CommandSequence{
			{"rm", "-rf", tmpDir},
			{"git", "clone", "https://github.com/Gentleman-Programming/gentleman-guardian-angel.git", tmpDir},
			{"bash", tmpDir + "/install.sh"},
		}, nil
	case "winget":
		// On Windows, use Git Bash explicitly to avoid bare "bash" resolving to
		// C:\Windows\System32\bash.exe (WSL), which cannot run the script.
		// Clean up any leftover directory from a previous run before cloning.
		// PowerShell is used for cleanup to avoid cmd.exe quoting issues with
		// embedded double quotes in the "if exist ... rmdir" approach.
		cloneDst := filepath.Join(os.TempDir(), "gentleman-guardian-angel")
		bash := gitBashPath()
		return CommandSequence{
			{"powershell", "-NoProfile", "-Command", fmt.Sprintf("Remove-Item -Recurse -Force -ErrorAction SilentlyContinue '%s'; exit 0", cloneDst)},
			{"git", "clone", "https://github.com/Gentleman-Programming/gentleman-guardian-angel.git", cloneDst},
			{bash, bashScriptPath(profile, filepath.Join(cloneDst, "install.sh"))},
		}, nil
	default:
		return nil, fmt.Errorf(
			"unsupported platform for gga: os=%q distro=%q pm=%q",
			profile.OS, profile.LinuxDistro, profile.PackageManager,
		)
	}
}

func bashScriptPath(profile system.PlatformProfile, path string) string {
	if profile.OS == "windows" {
		return strings.ReplaceAll(path, `\`, "/")
	}
	return path
}

// GitBashPath is the exported wrapper so other packages (e.g. cli) can
// resolve the Git Bash binary without duplicating the detection logic.
func GitBashPath() string { return gitBashPath() }

// gitBashPath returns the path to Git Bash on Windows.
// It resolves git on PATH, then finds bash.exe relative to it
// (Git for Windows always installs both in the same bin/ directory).
// Falls back to well-known locations, then to bare "bash" as last resort.
func gitBashPath() string {
	// Strategy 1: find git on PATH and derive bash.exe from it.
	if gitPath, err := cmdLookPath("git"); err == nil {
		// gitPath is e.g. "C:\Program Files\Git\cmd\git.exe"
		// bash.exe lives in the sibling bin/ directory.
		gitDir := filepath.Dir(gitPath) // .../cmd or .../bin
		parent := filepath.Dir(gitDir)  // .../Git

		candidate := filepath.Join(parent, "bin", "bash.exe")
		if _, err := osStat(candidate); err == nil {
			return candidate
		}

		// git might already be in bin/ (not cmd/).
		candidate = filepath.Join(gitDir, "bash.exe")
		if _, err := osStat(candidate); err == nil {
			return candidate
		}
	}

	// Strategy 2: well-known locations.
	candidates := []string{
		filepath.Join(os.Getenv("ProgramFiles"), "Git", "bin", "bash.exe"),
		filepath.Join(os.Getenv("ProgramFiles(x86)"), "Git", "bin", "bash.exe"),
		`C:\Program Files\Git\bin\bash.exe`,
	}

	for _, c := range candidates {
		if c == "" {
			continue
		}
		if _, err := osStat(c); err == nil {
			return c
		}
	}

	// Last resort — bare "bash" and hope it's Git Bash, not WSL.
	return "bash"
}

// validateGoForModuleInstall checks that Go ≥1.24 is installed and GO111MODULE is not
// disabled before attempting `go install`. Returns an actionable error if any check fails.
// MUST NOT be called for brew-based installs (brew manages Go transitively).
func validateGoForModuleInstall(profile system.PlatformProfile) error {
	if _, err := cmdLookPath("go"); err != nil {
		return fmt.Errorf(
			"Go 1.24+ is required to install Engram but was not found in PATH.\n" +
				"Please install Go from https://go.dev/dl/ and restart your terminal.")
	}

	out, err := cmdGoVersion()
	if err != nil {
		return fmt.Errorf(
			"Go 1.24+ is required but could not verify the installed version.\n" +
				"Please ensure Go is properly installed: https://go.dev/dl/")
	}

	// Parse "go version go1.XX.Y platform/arch"
	parts := strings.Fields(string(out))
	if len(parts) >= 3 {
		versionStr := strings.TrimPrefix(parts[2], "go")
		versionParts := strings.SplitN(versionStr, ".", 3)
		if len(versionParts) >= 2 {
			major, _ := strconv.Atoi(versionParts[0])
			minor, _ := strconv.Atoi(versionParts[1])
			if major < 1 || (major == 1 && minor < 24) {
				return fmt.Errorf(
					"Go 1.24+ is required to install Engram, but found go%s.\n"+
						"Please update Go: https://go.dev/dl/", versionStr)
			}
		}
	}

	if osGetenv("GO111MODULE") == "off" {
		fix := "export GO111MODULE=on  # then retry"
		if profile.OS == "windows" {
			fix = `$env:GO111MODULE = "on"  # PowerShell, then retry`
		}
		return fmt.Errorf("Go modules are disabled (GO111MODULE=off).\nRun: %s", fix)
	}

	return nil
}

// resolveEngramInstall returns the correct install command sequence for Engram per platform.
// - darwin (brew): brew tap + brew install (via Gentleman-Programming/homebrew-tap)
// - linux/windows: returns an error — callers must use engram.DownloadLatestBinary() instead.
//
// The go install method has been removed because it required Go 1.24+ which most
// users on Linux/Windows don't have. Pre-built binaries are available at:
// https://github.com/Gentleman-Programming/engram/releases
func resolveEngramInstall(profile system.PlatformProfile) (CommandSequence, error) {
	switch profile.PackageManager {
	case "brew":
		// macOS (or Linux with Homebrew): brew manages Go transitively — no preflight needed.
		return CommandSequence{
			{"brew", "tap", "Gentleman-Programming/homebrew-tap"},
			{"brew", "install", "engram"},
		}, nil
	default:
		return nil, fmt.Errorf(
			"engram on %q/%q uses direct binary download — use engram.DownloadLatestBinary() instead of CommandSequence",
			profile.OS, profile.PackageManager,
		)
	}
}
