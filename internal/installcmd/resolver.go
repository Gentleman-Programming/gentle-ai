package installcmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/gentleman-programming/gentle-ai/v2/internal/model"
	"github.com/gentleman-programming/gentle-ai/v2/internal/system"
	"github.com/gentleman-programming/gentle-ai/v2/internal/versions"
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

// resolveClaudeCodeInstall returns the npm install command sequence gentle-ai
// shows for Claude Code — display text only, never executed by gentle-ai
// (see agentInstallStep in internal/cli/run.go). On Linux with system npm,
// sudo is required. With nvm/fnm/volta, it is not. On Windows and macOS,
// sudo is never needed.
//
// --ignore-scripts blocks postinstall hooks, the primary supply-chain attack
// vector for npm packages. The version advises "latest" rather than a pin:
// a pin only guarded against a tampered "latest" tag when gentle-ai itself
// ran the command unattended. Now a human reads and runs it, and a stale
// hardcoded version goes wrong the moment a newer release ships (the same
// drift this shape fixed for Codex's GPT-5.6 update advice).
func resolveClaudeCodeInstall(profile system.PlatformProfile) CommandSequence {
	const pkg = "@anthropic-ai/claude-code@latest"
	if profile.OS == "linux" && !profile.NpmWritable {
		return CommandSequence{{"sudo", "npm", "install", "-g", "--ignore-scripts", pkg}}
	}
	return CommandSequence{{"npm", "install", "-g", "--ignore-scripts", pkg}}
}

// resolveKilocodeInstall returns the npm install command sequence gentle-ai
// shows for Kilocode — display text only, never executed by gentle-ai. On
// Linux with system npm, sudo is required. With nvm/fnm/volta, it is not.
// On Windows and macOS, sudo is never needed.
func resolveKilocodeInstall(profile system.PlatformProfile) CommandSequence {
	const pkg = "@kilocode/cli@latest"
	if profile.OS == "linux" && !profile.NpmWritable {
		return CommandSequence{{"sudo", "npm", "install", "-g", "--ignore-scripts", pkg}}
	}
	return CommandSequence{{"npm", "install", "-g", "--ignore-scripts", pkg}}
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

// npmBasedAgents is the set of agents whose auto-install runs npm commands.
// When any of these agents is selected, npm (and therefore Node.js) must be
// present before the pipeline reaches the agent install step.
//
// AgentPi is included because InstallCommand always runs engramInitCommand(),
// which executes either `pnpm dlx` or `npm exec` (both require Node.js). The
// npm-presence check is a sound proxy for Node.js availability.
var npmBasedAgents = map[model.AgentID]struct{}{
	model.AgentClaudeCode: {},
	model.AgentOpenCode:   {},
	model.AgentKilocode:   {},
	model.AgentGeminiCLI:  {},
	model.AgentCodex:      {},
	model.AgentQwenCode:   {},
	model.AgentPi:         {},
}

// ValidateAgentInstallPreflight validates agent-specific prerequisites that must
// exist before running installation commands.
func ValidateAgentInstallPreflight(profile system.PlatformProfile, agent model.AgentID) error {
	if _, ok := npmBasedAgents[agent]; ok {
		if err := validateNpmInstallPreflight(profile); err != nil {
			return err
		}
	}
	switch agent {
	case model.AgentKimi:
		return validateKimiInstallPreflight(profile)
	case model.AgentPi:
		return validatePiInstallPreflight()
	default:
		return nil
	}
}

func validatePiInstallPreflight() error {
	if _, err := cmdLookPath("pi"); err != nil {
		return fmt.Errorf("Pi requires the `pi` executable in PATH before installing Gentle AI Pi packages")
	}

	return nil
}

// validateNpmInstallPreflight ensures npm (and therefore Node.js) is available
// before attempting any npm-based agent install. Called for all agents in
// npmBasedAgents so the user gets a clear, actionable error instead of a
// cryptic "exec: npm: executable file not found in PATH" mid-pipeline.
func validateNpmInstallPreflight(profile system.PlatformProfile) error {
	if _, err := cmdLookPath("npm"); err != nil {
		hint := system.InstallHintForDep("node", profile)
		return fmt.Errorf(
			"Node.js / npm is required but `npm` was not found in PATH.\n"+
				"Install Node.js (npm is included) and retry:\n"+
				"  %s",
			hint,
		)
	}
	return nil
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

// resolveOpenCodeInstall returns the display-only install command sequence
// gentle-ai shows for OpenCode per platform — never executed by gentle-ai.
// - darwin: brew install anomalyco/tap/opencode (official OpenCode tap)
// - linux: npm install -g opencode-ai (official npm package)
// See https://opencode.ai/docs for official install methods.
//
// This deliberately advises "latest" rather than versions.OpenCode: that
// constant pins the exact OpenCode build the organic-runtime E2E installs
// and CI asserts against (TestOrganicRuntimeE2EUsesInstalledOpenCodePin in
// internal/assets/formatter_ordering_test.go, and organic_runtime_test.go's
// pinnedOpenCodeVersion) — a real, separate owner that must keep its exact
// pin. This display string has no such owner, so it is decoupled from that
// constant for the same reason the other five install commands are: a human
// reads and runs it, and an old hardcoded version goes stale the moment a
// newer OpenCode ships.
func resolveOpenCodeInstall(profile system.PlatformProfile) (CommandSequence, error) {
	const pkg = "opencode-ai@latest"
	switch profile.PackageManager {
	case "brew":
		return CommandSequence{
			{"brew", "install", "anomalyco/tap/opencode"},
		}, nil
	case "winget":
		// On Windows, npm global installs do not require sudo.
		return CommandSequence{{"npm", "install", "-g", "--ignore-scripts", pkg}}, nil
	default:
		// Any package manager the system probe accepted is enough here: the
		// install runs through npm, never through the manager itself, so
		// re-enumerating managers would silently narrow the probe's list
		// (issue #2499). The gate keeps a probe-rejected Linux profile
		// (empty PackageManager) on the unsupported arm.
		if profile.OS == "linux" && profile.PackageManager != "" {
			if profile.NpmWritable {
				return CommandSequence{{"npm", "install", "-g", "--ignore-scripts", pkg}}, nil
			}
			return CommandSequence{{"sudo", "npm", "install", "-g", "--ignore-scripts", pkg}}, nil
		}
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
	case "winget":
		// On Windows, use Git Bash explicitly to avoid bare "bash" resolving to
		// C:\Windows\System32\bash.exe (WSL), which cannot run the script.
		// Runtime cleanup is handled through system.PowerShellRunner before this
		// sequence so pwsh launch failures can safely fall back.
		cloneDst := filepath.Join(os.TempDir(), "gentleman-guardian-angel")
		bash := gitBashPath()
		return CommandSequence{
			{"git", "clone", "--depth=1", "--branch", "v" + versions.GGAVersion, "https://github.com/Gentleman-Programming/gentleman-guardian-angel.git", cloneDst},
			{bash, bashScriptPath(profile, filepath.Join(cloneDst, "install.sh"))},
		}, nil
	default:
		// Any package manager the system probe accepted is enough here: the
		// Linux install is git clone + install.sh and never touches the
		// manager, so re-enumerating managers would silently narrow the
		// probe's list (issue #2499). The gate keeps a probe-rejected Linux
		// profile (empty PackageManager) on the unsupported arm.
		if profile.OS == "linux" && profile.PackageManager != "" {
			const tmpDir = "/tmp/gentleman-guardian-angel"
			tagRef := "refs/tags/v" + versions.GGAVersion
			return CommandSequence{
				{"rm", "-rf", tmpDir},
				{"mkdir", "-p", tmpDir},
				{"git", "init", tmpDir},
				{"git", "-C", tmpDir, "fetch", "--depth=1", "https://github.com/Gentleman-Programming/gentleman-guardian-angel.git", tagRef + ":" + tagRef},
				{"git", "-C", tmpDir, "checkout", "-f", tagRef},
				{"bash", tmpDir + "/install.sh"},
			}, nil
		}
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
