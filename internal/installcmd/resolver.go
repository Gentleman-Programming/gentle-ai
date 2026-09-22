package installcmd

import (
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/gentleman-programming/gentle-ai/v3/internal/model"
	"github.com/gentleman-programming/gentle-ai/v3/internal/system"
	"github.com/gentleman-programming/gentle-ai/v3/internal/versions"
)

// cmdLookPath, osStat, osGetenv, and cmdGoVersion are package-level vars for testability.
var cmdLookPath = exec.LookPath
var osStat = os.Stat
var osGetenv = os.Getenv
var cmdGoVersion = func() ([]byte, error) {
	return exec.Command("go", "version").Output()
}

// cmdPiVersion and cmdNpmView are the Pi preflight probes; package-level vars
// for testability.
var cmdPiVersion = func() ([]byte, error) { return probe("pi", "--version") }
var cmdNpmView = func(args ...string) ([]byte, error) {
	return probe("npm", slices.Concat([]string{"view"}, args)...)
}

// probeTimeout bounds each preflight probe so a hung `pi` or a slow registry
// cannot stall the install; a timeout reads as an unavailable probe.
const probeTimeout = 30 * time.Second

// probe runs name with args under probeTimeout and returns its stdout.
func probe(name string, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), probeTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.WaitDelay = 5 * time.Second
	system.EnsureCommandDir(cmd)
	return cmd.Output()
}

var (
	semverPattern  = regexp.MustCompile(`\bv?(\d+)\.(\d+)\.(\d+)(?:-([0-9A-Za-z.-]+))?\b`)
	npmNamePattern = regexp.MustCompile(`^(@[a-z0-9._~-]+/)?[a-z0-9._~-]+$`)
)

// gentlePiPackage publishes the peer floor; piPackageSuffix picks the Pi
// package out of its peerDependencies, whatever scope it is published under.
const (
	gentlePiPackage = "gentle-pi"
	piPackageSuffix = "/pi-coding-agent"
)

// semver is one x.y.z[-prerelease] match in comparable form.
type semver struct {
	text  string
	parts [3]int
	pre   string
}

// parseSemver extracts the first whole x.y.z[-prerelease] in s ("Pi v0.87.0",
// ">=0.85.1 <1.0.0", "0.88.0-beta.1"); ok is false when there is none, when
// the match runs into other text ("0.73.1broken") or when a part overflows.
func parseSemver(s string) (v semver, ok bool) {
	match := semverPattern.FindStringSubmatch(s)
	if match == nil {
		return semver{}, false
	}
	v = semver{text: strings.TrimPrefix(match[0], "v"), pre: match[4]}
	for i := range 3 {
		n, err := strconv.Atoi(match[i+1])
		if err != nil {
			return semver{}, false
		}
		v.parts[i] = n
	}
	return v, true
}

// less reports whether v precedes w by SemVer 2.0 precedence: numeric parts,
// then a prerelease before the same release, then identifiers in order.
func (v semver) less(w semver) bool {
	if c := slices.Compare(v.parts[:], w.parts[:]); c != 0 {
		return c < 0
	}
	if (v.pre == "") != (w.pre == "") {
		return v.pre != ""
	}
	return slices.CompareFunc(strings.Split(v.pre, "."), strings.Split(w.pre, "."), compareIdentifier) < 0
}

// floorOf returns the lowest version an npm range admits: the strongest
// inclusive lower bound of each `||` alternative, then the lowest of those.
// An alternative with no inclusive lower bound admits anything, so no floor.
func floorOf(constraint string) (floor semver, ok bool) {
	for alternative := range strings.SplitSeq(constraint, "||") {
		bound, found := semver{}, false
		for token := range strings.FieldsSeq(alternative) {
			if v, isBound := lowerBound(token); isBound && (!found || bound.less(v)) {
				bound, found = v, true
			}
		}
		if !found {
			return semver{}, false
		}
		if !ok || bound.less(floor) {
			floor, ok = bound, true
		}
	}
	return floor, ok
}

// lowerBound reads a token that admits its own version and everything above
// within its comparator set (">=x", "=x", "^x", "~x", "x", "vx").
func lowerBound(token string) (semver, bool) {
	switch token[0] {
	case '^', '~', '=', 'v', '0', '1', '2', '3', '4', '5', '6', '7', '8', '9':
		return parseSemver(token)
	case '>':
		if strings.HasPrefix(token, ">=") {
			return parseSemver(token)
		}
	}
	return semver{}, false
}

// compareIdentifier orders prerelease identifiers: numeric ones numerically
// and before alphanumeric ones, which compare lexically.
func compareIdentifier(a, b string) int {
	an, aErr := strconv.Atoi(a)
	bn, bErr := strconv.Atoi(b)
	switch {
	case aErr == nil && bErr == nil:
		return cmp.Compare(an, bn)
	case aErr == nil:
		return -1
	case bErr == nil:
		return 1
	}
	return cmp.Compare(a, b)
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
		return validatePiInstallPreflight(profile)
	default:
		return nil
	}
}

// validatePiInstallPreflight refuses a Pi older than the floor gentle-pi
// declares, so a deprecated Pi does not accept the packages and then fail to
// load them. An unreadable version or an unreachable registry never blocks.
func validatePiInstallPreflight(profile system.PlatformProfile) error {
	if _, err := cmdLookPath("pi"); err != nil {
		return fmt.Errorf("Pi requires the `pi` executable in PATH before installing Gentle AI Pi packages")
	}
	out, err := cmdPiVersion()
	installed, ok := parseSemver(string(out))
	if err != nil || !ok {
		return nil
	}
	pkg, constraint := piPeerFloor()
	floor, ok := floorOf(constraint)
	if pkg == "" || !ok || !installed.less(floor) {
		return nil
	}
	return fmt.Errorf(
		"Pi %s is older than the %s gentle-pi requires.\nUpgrade Pi and retry:\n  %s",
		installed.text, floor.text, npmGlobalInstallHint(profile, pkg+"@latest"),
	)
}

// piPeerFloor reads the Pi package and its version constraint from gentle-pi's
// peerDependencies on the registry; both are empty when npm cannot answer or
// the name is not a valid npm package name.
func piPeerFloor() (pkg, constraint string) {
	out, err := cmdNpmView(gentlePiPackage, "peerDependencies", "--json")
	if err != nil {
		return "", ""
	}
	var peers map[string]string
	if err := json.Unmarshal(out, &peers); err != nil {
		return "", ""
	}
	for _, name := range slices.Sorted(maps.Keys(peers)) {
		if strings.HasSuffix(name, piPackageSuffix) && npmNamePattern.MatchString(name) {
			return name, peers[name]
		}
	}
	return "", ""
}

// npmGlobalInstallHint is the global npm install a user can paste; Linux with
// a system npm needs sudo, version managers and other platforms do not.
func npmGlobalInstallHint(profile system.PlatformProfile, pkg string) string {
	if profile.OS == "linux" && !profile.NpmWritable {
		return "sudo npm install -g " + pkg
	}
	return "npm install -g " + pkg
}

// validateNpmInstallPreflight ensures npm (and therefore Node.js) is available
// before attempting any npm-based agent install. Called for all agents in
// npmBasedAgents so the user gets a clear, actionable error instead of a
// cryptic "exec: npm: executable file not found in PATH" mid-pipeline.
func validateNpmInstallPreflight(profile system.PlatformProfile) error {
	if _, err := cmdLookPath("npm"); err != nil {
		hint := system.InstallHintForDep("node", profile)
		msg := fmt.Sprintf(
			"Node.js / npm is required but `npm` was not found in PATH.\n"+
				"Install Node.js (npm is included) and retry:\n"+
				"  %s",
			hint,
		)
		if profile.PackageManager == "rpm-ostree" {
			msg += "\nIf a pending deployment prevents --apply-live, stage without it and reboot:\n  rpm-ostree install -y nodejs npm && systemctl reboot"
		}
		return fmt.Errorf("%s", msg)
	}
	return nil
}

func validateKimiInstallPreflight(profile system.PlatformProfile) error {
	if !profile.Supported {
		return fmt.Errorf("Kimi is not supported on this platform (%s/%s)", profile.OS, profile.LinuxDistro)
	}

	if _, err := cmdLookPath("uv"); err != nil {
		msg := fmt.Sprintf(
			"Kimi requires Astral uv, but `uv` was not found in PATH.\n"+
				"Install uv and retry:\n"+
				"  %s",
			uvInstallHint(profile),
		)
		if profile.PackageManager == "rpm-ostree" {
			msg += "\nIf a pending deployment prevents --apply-live, stage without it and reboot:\n  rpm-ostree install -y uv && systemctl reboot"
		}
		return fmt.Errorf("%s", msg)
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
	case "rpm-ostree":
		return "rpm-ostree install -y --apply-live uv (or see https://docs.astral.sh/uv/getting-started/installation/)"
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
	case "rpm-ostree":
		return CommandSequence{{"rpm-ostree", "install", "-y", "--apply-live", dependency}}, nil
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

// resolveOpenCodeInstall is display-only advice for new V2 installations.
// Released @opencode/cli needs its postinstall binary setup: never add
// --ignore-scripts. Existing V1 installations are not upgraded by this advice.
// Source: https://opencode.ai/v2/docs/migrate-v1/
func resolveOpenCodeInstall(profile system.PlatformProfile) (CommandSequence, error) {
	const pkg = "@opencode/cli@latest"
	switch profile.PackageManager {
	case "brew":
		return CommandSequence{
			{"npm", "install", "-g", pkg},
		}, nil
	case "winget":
		// On Windows, npm global installs do not require sudo.
		return CommandSequence{{"npm", "install", "-g", pkg}}, nil
	default:
		// Any package manager the system probe accepted is enough here: the
		// install runs through npm, never through the manager itself, so
		// re-enumerating managers would silently narrow the probe's list
		// (issue #2499). The gate keeps a probe-rejected Linux profile
		// (empty PackageManager) on the unsupported arm.
		if profile.OS == "linux" && profile.PackageManager != "" {
			if profile.NpmWritable {
				return CommandSequence{{"npm", "install", "-g", pkg}}, nil
			}
			return CommandSequence{{"sudo", "npm", "install", "-g", pkg}}, nil
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
