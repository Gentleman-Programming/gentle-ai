package opencode

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"

	"github.com/gentleman-programming/gentle-ai/v2/internal/components/filemerge"
	"github.com/gentleman-programming/gentle-ai/v2/internal/system"
)

const (
	// BackgroundSubagentsEnv is the OpenCode-specific runtime switch. The
	// launcher only supplies the value when this variable is absent.
	BackgroundSubagentsEnv = "OPENCODE_EXPERIMENTAL_BACKGROUND_SUBAGENTS"

	// OwnershipMarker is embedded in every launcher written by Gentle AI.
	// It is intentionally stable so deactivation can refuse to remove user files.
	OwnershipMarker = "gentle-ai:managed-opencode-launcher/v1"

	minimumMajor = 1
	minimumMinor = 15
	minimumPatch = 11
)

// CapabilityStatus is the typed result of OpenCode background capability
// resolution.
type CapabilityStatus string

const (
	CapabilityReady       CapabilityStatus = "ready"
	CapabilityUnsupported CapabilityStatus = "unsupported"
	CapabilityUnknown     CapabilityStatus = "unknown"

	// Short aliases keep the status vocabulary convenient at call sites.
	StatusReady       = CapabilityReady
	StatusUnsupported = CapabilityUnsupported
	StatusUnknown     = CapabilityUnknown
)

// Version is a parsed OpenCode semantic version.
type Version struct {
	Major         int
	Minor         int
	Patch         int
	PreRelease    string
	BuildMetadata string
}

// String returns the canonical numeric version with its pre-release and build suffixes.
func (v Version) String() string {
	value := fmt.Sprintf("%d.%d.%d", v.Major, v.Minor, v.Patch)
	if v.PreRelease != "" {
		value += v.PreRelease
	}
	if v.BuildMetadata != "" {
		value += "+" + v.BuildMetadata
	}
	return value
}

// AtLeast reports whether v is greater than or equal to other.
func (v Version) AtLeast(other Version) bool {
	for _, pair := range [][2]int{{v.Major, other.Major}, {v.Minor, other.Minor}, {v.Patch, other.Patch}} {
		if pair[0] != pair[1] {
			return pair[0] > pair[1]
		}
	}
	aPreRelease := strings.TrimPrefix(v.PreRelease, "-")
	bPreRelease := strings.TrimPrefix(other.PreRelease, "-")
	if aPreRelease == bPreRelease {
		return true
	}
	if aPreRelease == "" {
		return true
	}
	if bPreRelease == "" {
		return false
	}
	return comparePreRelease(aPreRelease, bPreRelease) >= 0
}

// MinimumBackgroundVersion is the first version whose specific environment
// override semantics are safe for managed activation.
var MinimumBackgroundVersion = Version{Major: minimumMajor, Minor: minimumMinor, Patch: minimumPatch}

// versionPattern accepts complete SemVer tokens only. Version output can contain
// labels and punctuation, but a path or identifier continuation is not a boundary.
var versionPattern = regexp.MustCompile(`(?i)(?:^|[^0-9a-z.+/_-])v?(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)((?:-[0-9a-z-]+(?:\.[0-9a-z-]+)*)?(?:\+[0-9a-z-]+(?:\.[0-9a-z-]+)*)?)(?:$|[\t\r\n ,;:)\]}])`)

// ParseVersionOutput extracts the first semantic version from `opencode
// --version` output. OpenCode releases have emitted both bare versions and
// "opencode <version>" forms, so the command label is deliberately ignored.
func ParseVersionOutput(output string) (Version, error) {
	match := versionPattern.FindStringSubmatch(output)
	if match == nil {
		return Version{}, fmt.Errorf("OpenCode version output contains no semantic version: %q", strings.TrimSpace(output))
	}
	return versionFromMatch(match)
}

func versionFromMatch(match []string) (Version, error) {
	preRelease, buildMetadata := match[4], ""
	if index := strings.IndexByte(preRelease, '+'); index >= 0 {
		preRelease, buildMetadata = preRelease[:index], preRelease[index+1:]
	}
	if preRelease != "" && !validSemVerIdentifiers(strings.TrimPrefix(preRelease, "-"), true) ||
		strings.Contains(match[4], "+") && !validSemVerIdentifiers(buildMetadata, false) {
		return Version{}, errors.New("invalid OpenCode semantic version identifiers")
	}
	major, err := strconv.Atoi(match[1])
	if err != nil {
		return Version{}, err
	}
	minor, err := strconv.Atoi(match[2])
	if err != nil {
		return Version{}, err
	}
	patch, err := strconv.Atoi(match[3])
	if err != nil {
		return Version{}, err
	}
	return Version{Major: major, Minor: minor, Patch: patch, PreRelease: preRelease, BuildMetadata: buildMetadata}, nil
}

func validSemVerIdentifiers(value string, rejectLeadingZeroes bool) bool {
	if value == "" {
		return false
	}
	for _, identifier := range strings.Split(value, ".") {
		if identifier == "" || rejectLeadingZeroes && len(identifier) > 1 && identifier[0] == '0' && isNumericIdentifier(identifier) {
			return false
		}
		for _, char := range identifier {
			if char != '-' && (char < '0' || char > '9') && (char < 'a' || char > 'z') && (char < 'A' || char > 'Z') {
				return false
			}
		}
	}
	return true
}

func comparePreRelease(a, b string) int {
	aIDs := strings.Split(a, ".")
	bIDs := strings.Split(b, ".")
	for i := 0; i < len(aIDs) && i < len(bIDs); i++ {
		aID, bID := aIDs[i], bIDs[i]
		aNumeric, bNumeric := isNumericIdentifier(aID), isNumericIdentifier(bID)
		switch {
		case aNumeric && bNumeric:
			if comparison := compareNumericIdentifier(aID, bID); comparison != 0 {
				return comparison
			}
		case aNumeric:
			return -1
		case bNumeric:
			return 1
		case aID < bID:
			return -1
		case aID > bID:
			return 1
		}
	}
	if len(aIDs) < len(bIDs) {
		return -1
	}
	if len(aIDs) > len(bIDs) {
		return 1
	}
	return 0
}

func isNumericIdentifier(value string) bool {
	if value == "" {
		return false
	}
	for _, char := range value {
		if char < '0' || char > '9' {
			return false
		}
	}
	return true
}

func compareNumericIdentifier(a, b string) int {
	if len(a) < len(b) || len(a) == len(b) && a < b {
		return -1
	}
	if len(a) > len(b) || a > b {
		return 1
	}
	return 0
}

// CapabilityResolution records the version/capability decision and the
// restart guidance needed after a successful managed activation.
type CapabilityResolution struct {
	Status          CapabilityStatus
	Version         string
	TargetPath      string
	Reason          string
	RestartRequired bool
	RestartGuidance string
}

// ActivationReport is the public outcome of a managed launcher transaction.
// Capability status remains meaningful when activation is intentionally a
// foreground fallback for an unsupported or unknown runtime.
type ActivationReport struct {
	Capability    CapabilityResolution
	Action        string
	Applied       bool
	ChangedPaths  []string
	LauncherPaths []string
}

// Ready reports whether the runtime is eligible for managed activation.
func (c CapabilityResolution) Ready() bool { return c.Status == CapabilityReady }

// VersionRunner executes `opencode --version` for a resolved real target.
type VersionRunner func(target string) (string, error)

// ResolveCapability parses the target's version without probing a server or
// any other OpenCode runtime endpoint.
func ResolveCapability(target string, runVersion VersionRunner) CapabilityResolution {
	result := CapabilityResolution{TargetPath: target}
	if strings.TrimSpace(target) == "" {
		result.Status = CapabilityUnknown
		result.Reason = "OpenCode target could not be resolved"
		return result
	}
	if runVersion == nil {
		runVersion = RunVersion
	}
	output, err := runVersion(target)
	if err != nil {
		result.Status = CapabilityUnknown
		result.Reason = fmt.Sprintf("run %s --version: %v", target, err)
		return result
	}
	version, err := ParseVersionOutput(output)
	if err != nil {
		result.Status = CapabilityUnknown
		result.Reason = err.Error()
		return result
	}
	result.Version = version.String()
	if !version.AtLeast(MinimumBackgroundVersion) {
		result.Status = CapabilityUnsupported
		result.Reason = fmt.Sprintf("OpenCode %s is below the minimum supported version %s", result.Version, MinimumBackgroundVersion.String())
		return result
	}
	result.Status = CapabilityReady
	result.RestartRequired = true
	result.RestartGuidance = "Restart OpenCode after launching it through the managed launcher so the background-subagent environment is inherited."
	return result
}

// RunVersion executes the only runtime probe used by managed activation.
func RunVersion(target string) (string, error) {
	command := exec.Command(target, "--version")
	system.EnsureCommandDir(command)
	output, err := command.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("%w: %s", err, strings.TrimSpace(string(output)))
	}
	return string(output), nil
}

// ActivationOptions supplies platform and process seams for activation. Zero
// values use the current process and the real system PATH implementation.
type ActivationOptions struct {
	OS                       string
	Path                     string
	RunVersion               VersionRunner
	AddToUserPath            func(string) error
	RemoveFromUserPath       func(string) error
	AddToUserPathWithResult  func(string) (system.UserPathAddition, error)
	RollbackUserPathAddition func(string, system.UserPathAddition) error
	ResolveTarget            func(homeDir, goos, pathValue string) (string, error)
	WriteFile                func(path string, content []byte, mode os.FileMode) error
	RemoveFile               func(path string) error
}

func (o ActivationOptions) normalized() ActivationOptions {
	if o.OS == "" {
		o.OS = runtime.GOOS
	}
	if o.Path == "" {
		o.Path = os.Getenv("PATH")
	}
	if o.RunVersion == nil {
		o.RunVersion = RunVersion
	}
	if o.AddToUserPath == nil {
		o.AddToUserPath = system.AddToUserPath
	}
	if o.RemoveFromUserPath == nil {
		o.RemoveFromUserPath = system.RemoveFromUserPath
	}
	if o.AddToUserPathWithResult == nil {
		o.AddToUserPathWithResult = system.AddToUserPathWithResult
	}
	if o.RollbackUserPathAddition == nil {
		o.RollbackUserPathAddition = system.RollbackUserPathAddition
	}
	if o.ResolveTarget == nil {
		o.ResolveTarget = ResolveTarget
	}
	if o.WriteFile == nil {
		o.WriteFile = func(path string, content []byte, mode os.FileMode) error {
			_, err := filemerge.WriteFileAtomic(path, content, mode)
			return err
		}
	}
	if o.RemoveFile == nil {
		o.RemoveFile = os.Remove
	}
	return o
}

// BinDir returns the Gentle-owned launcher directory.
func BinDir(homeDir string) string { return filepath.Join(homeDir, ".gentle-ai", "bin") }

// POSIXLauncherPath returns the POSIX launcher path.
func POSIXLauncherPath(homeDir string) string { return filepath.Join(BinDir(homeDir), "opencode") }

// WindowsCMDPath returns the Windows cmd launcher path.
func WindowsCMDPath(homeDir string) string { return filepath.Join(BinDir(homeDir), "opencode.cmd") }

// WindowsPS1Path returns the Windows PowerShell launcher path.
func WindowsPS1Path(homeDir string) string { return filepath.Join(BinDir(homeDir), "opencode.ps1") }

// ManagedLauncherPaths returns the launcher files for the requested platform.
// WSL reports linux and therefore receives the POSIX launcher.
func ManagedLauncherPaths(homeDir, goos string) []string {
	if goos == "windows" {
		return []string{WindowsCMDPath(homeDir), WindowsPS1Path(homeDir)}
	}
	return []string{POSIXLauncherPath(homeDir)}
}

// LauncherPaths is an alias with a concise name for callers that already know
// they are asking for Gentle-owned paths.
func LauncherPaths(homeDir, goos string) []string { return ManagedLauncherPaths(homeDir, goos) }

// ResolveTarget finds the real OpenCode executable while excluding the
// Gentle-owned bin directory. It must be called before that directory is added
// to PATH, and remains safe on repeated activation after it is already there.
func ResolveTarget(homeDir, goos, pathValue string) (string, error) {
	if goos == "" {
		goos = runtime.GOOS
	}
	managedDir, err := filepath.Abs(BinDir(homeDir))
	if err != nil {
		return "", fmt.Errorf("resolve managed OpenCode bin directory: %w", err)
	}
	for _, entry := range splitPath(pathValue, goos) {
		entry = strings.Trim(strings.TrimSpace(entry), `"`)
		if entry == "" || !filepath.IsAbs(entry) {
			continue
		}
		entry, err = filepath.Abs(entry)
		if err != nil {
			continue
		}
		if samePath(entry, managedDir, goos) {
			continue
		}
		for _, name := range targetNames(goos) {
			candidate := filepath.Join(entry, name)
			info, statErr := os.Stat(candidate)
			if statErr != nil || !info.Mode().IsRegular() || goos != "windows" && info.Mode().Perm()&0o111 == 0 {
				continue
			}
			realPath, evalErr := filepath.EvalSymlinks(candidate)
			if evalErr != nil {
				continue
			}
			realPath, absErr := filepath.Abs(realPath)
			if absErr != nil {
				continue
			}
			if pathUnder(realPath, managedDir, goos) {
				continue
			}
			return realPath, nil
		}
	}
	return "", fmt.Errorf("OpenCode target not found outside managed launcher directory %q", managedDir)
}

func splitPath(value, goos string) []string {
	separator := ":"
	if goos == "windows" {
		separator = ";"
	}
	if value == "" {
		return nil
	}
	return strings.Split(value, separator)
}

func targetNames(goos string) []string {
	if goos == "windows" {
		return []string{"opencode.exe", "opencode.cmd", "opencode.bat", "opencode"}
	}
	return []string{"opencode"}
}

func samePath(a, b, goos string) bool {
	a = filepath.Clean(a)
	b = filepath.Clean(b)
	if goos == "windows" {
		return strings.EqualFold(a, b)
	}
	return a == b
}

func pathUnder(path, root, goos string) bool {
	path = filepath.Clean(path)
	root = filepath.Clean(root)
	if samePath(path, root, goos) {
		return true
	}
	prefix := root + string(filepath.Separator)
	if goos == "windows" {
		return strings.HasPrefix(strings.ToLower(path), strings.ToLower(prefix))
	}
	return strings.HasPrefix(path, prefix)
}

type activationAction string

const (
	activationActionOn  activationAction = "on"
	activationActionOff activationAction = "off"
)

type launcherSnapshot struct {
	exists bool
	data   []byte
	mode   os.FileMode
	owned  bool
}

// ActivationPlan is a prepared, reversible launcher transaction. Preparation
// resolves the target, checks capability, and rejects collisions before Apply
// mutates any launcher or PATH state.
type ActivationPlan struct {
	homeDir      string
	goos         string
	options      ActivationOptions
	action       activationAction
	capability   CapabilityResolution
	paths        []string
	desired      map[string][]byte
	before       map[string]launcherSnapshot
	changed      []string
	pathAddition system.UserPathAddition
	pathAdded    bool
	applied      bool
}

// PrepareActivation resolves capability and preflights all owned launcher
// collisions. Unsupported and unknown versions produce a no-op plan with a
// typed foreground result rather than writing an unsafe launcher.
func PrepareActivation(homeDir string, options ActivationOptions) (*ActivationPlan, error) {
	options = options.normalized()
	paths := ManagedLauncherPaths(homeDir, options.OS)
	plan := &ActivationPlan{
		homeDir: homeDir,
		goos:    options.OS,
		options: options,
		action:  activationActionOn,
		paths:   paths,
		desired: make(map[string][]byte, len(paths)),
		before:  make(map[string]launcherSnapshot, len(paths)),
	}

	target, targetErr := options.ResolveTarget(homeDir, options.OS, options.Path)
	if targetErr != nil {
		plan.capability = CapabilityResolution{
			Status: CapabilityUnknown,
			Reason: targetErr.Error(),
		}
		return plan, nil
	}
	plan.capability = ResolveCapability(target, options.RunVersion)
	if plan.capability.Ready() {
		plan.capability.RestartGuidance = restartGuidance(paths)
	}
	if !plan.capability.Ready() {
		return plan, nil
	}
	for _, path := range paths {
		snapshot, err := readLauncherSnapshot(path)
		if err != nil {
			return nil, err
		}
		plan.before[path] = snapshot
		if snapshot.exists && !snapshot.owned {
			return nil, fmt.Errorf("refusing user-owned OpenCode launcher collision at %q", path)
		}
		content := launcherContent(options.OS, target)
		plan.desired[path] = []byte(content[filepath.Base(path)])
	}
	return plan, nil
}

// PrepareDeactivation prepares removal of only marker-owned launchers. It does
// not resolve or execute OpenCode because turning the feature off must remain
// safe even when the real runtime is no longer installed.
func PrepareDeactivation(homeDir string, options ActivationOptions) (*ActivationPlan, error) {
	options = options.normalized()
	paths := ManagedLauncherPaths(homeDir, options.OS)
	plan := &ActivationPlan{
		homeDir: homeDir,
		goos:    options.OS,
		options: options,
		action:  activationActionOff,
		paths:   paths,
		before:  make(map[string]launcherSnapshot, len(paths)),
	}
	for _, path := range paths {
		snapshot, err := readLauncherSnapshot(path)
		if err != nil {
			return nil, err
		}
		plan.before[path] = snapshot
	}
	return plan, nil
}

// Capability returns the prepared typed capability decision.
func (p *ActivationPlan) Capability() CapabilityResolution {
	if p == nil {
		return CapabilityResolution{Status: CapabilityUnknown, Reason: "OpenCode activation plan is nil"}
	}
	return p.capability
}

// Report returns the current typed plan outcome.
func (p *ActivationPlan) Report() ActivationReport {
	if p == nil {
		return ActivationReport{Capability: CapabilityResolution{Status: CapabilityUnknown, Reason: "OpenCode activation plan is nil"}}
	}
	capability := p.capability
	if p.action == activationActionOff && capability.Status == "" {
		capability.Status = CapabilityReady
		capability.Reason = "deactivation does not require an OpenCode runtime probe"
	}
	return ActivationReport{
		Capability:    capability,
		Action:        string(p.action),
		Applied:       p.applied,
		ChangedPaths:  append([]string(nil), p.changed...),
		LauncherPaths: append([]string(nil), p.paths...),
	}
}

// Paths returns the exact managed launcher paths for this plan.
func (p *ActivationPlan) Paths() []string {
	if p == nil {
		return nil
	}
	return append([]string(nil), p.paths...)
}

// RestartRequired reports whether a caller should restart OpenCode after this
// plan's action. Capability readiness is established before any file mutation.
func (p *ActivationPlan) RestartRequired() bool {
	if p == nil {
		return false
	}
	if p.action == activationActionOn {
		return p.capability.Ready()
	}
	for _, snapshot := range p.before {
		if snapshot.exists && snapshot.owned {
			return true
		}
	}
	return false
}

// RestartGuidance returns the exact user-facing restart instruction.
func (p *ActivationPlan) RestartGuidance() string {
	if p == nil {
		return ""
	}
	if p.capability.RestartGuidance != "" {
		return p.capability.RestartGuidance
	}
	if p.RestartRequired() {
		return restartGuidance(p.paths)
	}
	return ""
}

// ChangedPaths returns paths changed by the last successful Apply call.
func (p *ActivationPlan) ChangedPaths() []string {
	if p == nil {
		return nil
	}
	return append([]string(nil), p.changed...)
}

// Apply writes or removes the prepared owned launchers. A failure restores all
// changes made by this plan before returning.
func (p *ActivationPlan) Apply() error {
	if p == nil {
		return errors.New("OpenCode activation plan is nil")
	}
	p.changed = nil
	p.applied = false
	if p.action == activationActionOn {
		if !p.capability.Ready() {
			p.applied = true
			return nil
		}
		for _, path := range p.paths {
			before := p.before[path]
			desired := p.desired[path]
			if before.exists && bytes.Equal(before.data, desired) {
				continue
			}
			p.changed = append(p.changed, path)
			if err := p.options.WriteFile(path, desired, 0o755); err != nil {
				return p.failAndRollback(fmt.Errorf("write managed OpenCode launcher %q: %w", path, err))
			}
		}
		if p.goos == "windows" {
			addition, err := p.options.AddToUserPathWithResult(BinDir(p.homeDir))
			p.pathAddition = addition
			p.pathAdded = addition.ProcessAdded || addition.PersistentAdded
			if err != nil {
				return p.failAndRollback(fmt.Errorf("add managed OpenCode bin directory %q to PATH: %w", BinDir(p.homeDir), err))
			}
		} else if err := p.options.AddToUserPath(BinDir(p.homeDir)); err != nil {
			return p.failAndRollback(fmt.Errorf("add managed OpenCode bin directory %q to PATH: %w", BinDir(p.homeDir), err))
		}
		p.applied = true
		return nil
	}

	for _, path := range p.paths {
		snapshot := p.before[path]
		if !snapshot.exists || !snapshot.owned {
			continue
		}
		p.changed = append(p.changed, path)
		if err := p.options.RemoveFile(path); err != nil && !os.IsNotExist(err) {
			return p.failAndRollback(fmt.Errorf("remove managed OpenCode launcher %q: %w", path, err))
		}
	}
	p.applied = true
	return nil
}

func (p *ActivationPlan) failAndRollback(cause error) error {
	if rollbackErr := p.Rollback(); rollbackErr != nil {
		return errors.Join(cause, rollbackErr)
	}
	return cause
}

// Rollback restores only paths changed by this plan and removes a user PATH
// entry only when this plan added it.
func (p *ActivationPlan) Rollback() error {
	if p == nil {
		return nil
	}
	var rollbackErr error
	for i := len(p.changed) - 1; i >= 0; i-- {
		path := p.changed[i]
		snapshot := p.before[path]
		if !snapshot.exists {
			if err := p.options.RemoveFile(path); err != nil && !os.IsNotExist(err) {
				rollbackErr = errors.Join(rollbackErr, fmt.Errorf("rollback remove managed OpenCode launcher %q: %w", path, err))
			}
			continue
		}
		if err := p.options.WriteFile(path, snapshot.data, snapshot.mode); err != nil {
			rollbackErr = errors.Join(rollbackErr, fmt.Errorf("rollback restore managed OpenCode launcher %q: %w", path, err))
		}
	}
	if p.pathAdded {
		if err := p.options.RollbackUserPathAddition(BinDir(p.homeDir), p.pathAddition); err != nil {
			rollbackErr = errors.Join(rollbackErr, fmt.Errorf("rollback remove managed OpenCode bin directory %q from PATH: %w", BinDir(p.homeDir), err))
		} else {
			p.pathAdded = false
		}
	}
	if rollbackErr == nil {
		p.changed = nil
	}
	return rollbackErr
}

func readLauncherSnapshot(path string) (launcherSnapshot, error) {
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return launcherSnapshot{}, nil
	}
	if err != nil {
		return launcherSnapshot{}, fmt.Errorf("inspect managed OpenCode launcher %q: %w", path, err)
	}
	if !info.Mode().IsRegular() {
		return launcherSnapshot{}, fmt.Errorf("refusing non-regular OpenCode launcher collision at %q", path)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return launcherSnapshot{}, fmt.Errorf("read managed OpenCode launcher %q: %w", path, err)
	}
	return launcherSnapshot{exists: true, data: data, mode: info.Mode().Perm(), owned: bytes.Contains(data, []byte(OwnershipMarker))}, nil
}

func launcherContent(goos, target string) map[string]string {
	if goos == "windows" {
		return map[string]string{
			WindowsCMDPathPlaceholder: windowsCMDLauncher(target),
			WindowsPS1PathPlaceholder: windowsPS1Launcher(target),
		}
	}
	return map[string]string{POSIXLauncherPathPlaceholder: posixLauncher(target)}
}

// Placeholders let launcherContent stay platform-testable without constructing
// a second home-dependent map. PrepareActivation indexes the relevant value.
const (
	POSIXLauncherPathPlaceholder = "opencode"
	WindowsCMDPathPlaceholder    = "opencode.cmd"
	WindowsPS1PathPlaceholder    = "opencode.ps1"
)

func posixLauncher(target string) string {
	return "#!/bin/sh\n" +
		"# " + OwnershipMarker + "\n" +
		"set -eu\n" +
		"if [ -z \"${" + BackgroundSubagentsEnv + "+x}\" ]; then\n" +
		"  export " + BackgroundSubagentsEnv + "=true\n" +
		"fi\n" +
		"exec " + shellQuote(target) + " \"$@\"\n"
}

func windowsCMDLauncher(target string) string {
	// cmd expands %VAR% in target paths; resolved targets containing % are unsupported.
	quotedTarget := strings.ReplaceAll(target, `"`, `""`)
	invoke := `"` + quotedTarget + `" %*`
	if strings.EqualFold(filepath.Ext(target), ".ps1") {
		// Bypass applies only to this already-resolved target, never arbitrary input.
		invoke = `powershell -NoProfile -ExecutionPolicy Bypass -File "` + quotedTarget + `" %*`
	}
	return "@echo off\r\n" +
		"rem " + OwnershipMarker + "\r\n" +
		"setlocal\r\n" +
		"if not defined " + BackgroundSubagentsEnv + " set \"" + BackgroundSubagentsEnv + "=true\"\r\n" +
		invoke + "\r\n" +
		"exit /b %ERRORLEVEL%\r\n"
}

func windowsPS1Launcher(target string) string {
	return "# " + OwnershipMarker + "\r\n" +
		"$ErrorActionPreference = 'Stop'\r\n" +
		"if (-not (Test-Path Env:" + BackgroundSubagentsEnv + ")) { $env:" + BackgroundSubagentsEnv + " = 'true' }\r\n" +
		"& " + powershellQuote(target) + " @args\r\n" +
		"exit $LASTEXITCODE\r\n"
}

func shellQuote(value string) string { return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'" }

func powershellQuote(value string) string { return "'" + strings.ReplaceAll(value, "'", "''") + "'" }

func restartGuidance(paths []string) string {
	if len(paths) == 0 {
		return "Restart OpenCode after managed activation so the background-subagent environment is inherited."
	}
	copyPaths := append([]string(nil), paths...)
	sort.Strings(copyPaths)
	return fmt.Sprintf("Managed OpenCode launcher path: %s. Restart OpenCode after launching through it so the background-subagent environment is inherited; restart the shell if PATH has not refreshed.", strings.Join(copyPaths, ", "))
}
