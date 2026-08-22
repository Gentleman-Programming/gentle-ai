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
	Capability       CapabilityResolution
	Action           string
	Applied          bool
	Effective        bool
	ActivationReason string
	ChangedPaths     []string
	LauncherPaths    []string
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
	Shell                    string
	RunVersion               VersionRunner
	AddToUserPath            func(string) error
	RemoveFromUserPath       func(string) error
	AddToUserPathWithResult  func(string) (system.UserPathAddition, error)
	RollbackUserPathAddition func(string, system.UserPathAddition) error
	ResolveTarget            func(homeDir, goos, pathValue string) (string, error)
	WriteFile                func(path string, content []byte, mode os.FileMode) (filemerge.WriteResult, error)
	RemoveFile               func(path string) error
}

func (o ActivationOptions) normalized() ActivationOptions {
	if o.OS == "" {
		o.OS = runtime.GOOS
	}
	if o.Path == "" {
		o.Path = os.Getenv("PATH")
	}
	if o.Shell == "" {
		o.Shell = os.Getenv("SHELL")
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
		o.WriteFile = filemerge.WriteFileAtomic
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
	if goos != "windows" {
		return []string{"opencode"}
	}
	names := make([]string, 0, len(windowsExecutableExtensions()))
	for _, extension := range windowsExecutableExtensions() {
		names = append(names, "opencode"+extension)
	}
	return names
}

func windowsExecutableExtensions() []string {
	pathext := os.Getenv("PATHEXT")
	if pathext == "" {
		return []string{".com", ".exe", ".bat", ".cmd"}
	}
	var extensions []string
	for _, extension := range strings.Split(pathext, ";") {
		extension = strings.ToLower(strings.TrimSpace(extension))
		if extension == "" {
			continue
		}
		if !strings.HasPrefix(extension, ".") {
			extension = "." + extension
		}
		extensions = append(extensions, extension)
	}
	return extensions
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

	activationReasonReady        = "OpenCode runtime supports managed activation"
	activationReasonApplied      = "managed OpenCode launcher activation is active"
	activationReasonPathPending  = "managed OpenCode launcher was written but is not currently reachable through PATH"
	activationReasonDeactivation = "deactivation does not require an OpenCode runtime probe"
	profilePendingReason         = "OpenCode managed profile update is pending"
)

type launcherSnapshot struct {
	exists bool
	data   []byte
	mode   os.FileMode
	owned  bool
}

type profileChange struct {
	path          string
	before        launcherSnapshot
	desired       []byte
	changed       bool
	profileReason string
	writeResult   filemerge.WriteResult
	applied       bool
}

// ActivationPlan is a prepared, reversible launcher transaction. Preparation
// resolves the target, checks capability, and rejects collisions before Apply
// mutates any launcher or PATH state.
type ActivationPlan struct {
	homeDir          string
	goos             string
	options          ActivationOptions
	action           activationAction
	capability       CapabilityResolution
	paths            []string
	desired          map[string][]byte
	before           map[string]launcherSnapshot
	changed          []string
	pathAddition     system.UserPathAddition
	pathAdded        bool
	profiles         []profileChange
	effective        bool
	activationReason string
	applied          bool
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
		plan.activationReason = plan.capability.Reason
		return plan, nil
	}
	if options.OS == "windows" && !windowsTargetSafe(target) {
		plan.capability = CapabilityResolution{Status: CapabilityUnsupported, TargetPath: target, Reason: "Windows OpenCode target contains %, which CMD expands; install OpenCode at a path without % and run sync again"}
		plan.activationReason = plan.capability.Reason
		return plan, nil
	}
	plan.capability = ResolveCapability(target, options.RunVersion)
	plan.activationReason = plan.capability.Reason
	if plan.activationReason == "" && plan.capability.Ready() {
		plan.activationReason = activationReasonReady
	}
	if plan.capability.Ready() {
		plan.capability.RestartGuidance = restartGuidance(paths)
	}
	if !plan.capability.Ready() {
		return plan, nil
	}
	if options.OS != "windows" {
		profile, ok := loginProfile(homeDir, options.Shell)
		if !ok {
			plan.activationReason = "managed launcher activation requires zsh, or an existing bash login profile; start a supported login shell and run sync again"
			return plan, nil
		}
		change, err := addProfileChange(profile, BinDir(homeDir))
		if err != nil {
			return nil, err
		}
		plan.profiles = []profileChange{change}
		if change.profileReason != "" {
			plan.activationReason = change.profileReason
		} else {
			plan.effective = pathResolvesTo(options.Path, POSIXLauncherPath(homeDir), options.OS)
			if !plan.effective {
				plan.activationReason = "managed launcher is persisted for future login shells; start a new login shell before running opencode"
			}
		}
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
		homeDir:          homeDir,
		goos:             options.OS,
		options:          options,
		action:           activationActionOff,
		paths:            paths,
		before:           make(map[string]launcherSnapshot, len(paths)),
		activationReason: activationReasonDeactivation,
	}
	for _, path := range paths {
		snapshot, err := readLauncherSnapshot(path)
		if err != nil {
			return nil, err
		}
		plan.before[path] = snapshot
	}
	if options.OS != "windows" {
		for _, profile := range ManagedProfilePaths(homeDir) {
			change, err := removeProfileChange(profile, BinDir(homeDir))
			if err != nil {
				return nil, err
			}
			if change.profileReason != "" {
				plan.activationReason = change.profileReason
			}
			if change.changed || change.profileReason != "" {
				plan.profiles = append(plan.profiles, change)
			}
		}
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
		capability.Reason = activationReasonDeactivation
	}
	return ActivationReport{
		Capability:       capability,
		Action:           string(p.action),
		Applied:          p.applied,
		Effective:        p.Effective(),
		ActivationReason: p.activationReason,
		ChangedPaths:     p.ChangedPaths(),
		LauncherPaths:    append([]string(nil), p.paths...),
	}
}

func (p *ActivationPlan) Effective() bool {
	if p == nil || !p.applied || !p.capability.Ready() {
		return false
	}
	return p.goos == "windows" || p.effective
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
	if p.profileBlocked() {
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
	if p.profileBlocked() {
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
func (p *ActivationPlan) profileBlocked() bool {
	for _, change := range p.profiles {
		if change.profileReason != "" {
			return true
		}
	}
	return false
}

func (p *ActivationPlan) ChangedPaths() []string {
	if p == nil {
		return nil
	}
	paths := append([]string(nil), p.changed...)
	if !p.applied {
		return paths
	}
	for _, change := range p.profiles {
		if change.changed && change.applied {
			paths = append(paths, change.path)
		}
	}
	return paths
}

// Apply writes or removes the prepared owned launchers. A failure restores all
// changes made by this plan before returning.
func (p *ActivationPlan) Apply() error {
	if p == nil {
		return errors.New("OpenCode activation plan is nil")
	}
	p.changed = nil
	p.applied = false
	for i := range p.profiles {
		p.profiles[i].writeResult = filemerge.WriteResult{}
		p.profiles[i].applied = false
	}
	if p.profileBlocked() {
		p.applied = true
		return nil
	}
	if p.action == activationActionOn {
		if !p.capability.Ready() {
			p.applied = true
			return nil
		}
		if p.goos != "windows" && len(p.profiles) == 0 {
			p.applied = true
			return nil
		}
		for _, path := range p.paths {
			before := p.before[path]
			desired := p.desired[path]
			if before.exists && (p.goos == "windows" || before.mode == 0o755) && bytes.Equal(before.data, desired) {
				continue
			}
			if err := requireSnapshot(path, before); err != nil {
				return p.failAndRollback(fmt.Errorf("revalidate managed OpenCode launcher %q before write: %w", path, err))
			}
			result, err := p.options.WriteFile(path, desired, 0o755)
			if result.Changed {
				p.changed = append(p.changed, path)
			}
			if err != nil {
				return p.failAndRollback(fmt.Errorf("write managed OpenCode launcher %q: %w", path, err))
			}
			if p.goos != "windows" {
				if err := os.Chmod(path, 0o755); err != nil {
					return p.failAndRollback(fmt.Errorf("set managed OpenCode launcher mode %q: %w", path, err))
				}
			}
			if !result.Changed {
				p.changed = append(p.changed, path)
			}
		}
		if err := p.applyProfiles(); err != nil {
			return p.failAndRollback(err)
		}
		// options.Path is the snapshot of PATH before this transaction. Resolve
		// against it before AddToUserPath can mutate the current process; a
		// process-local PATH update is not evidence that a fresh login shell has
		// inherited the profile block.
		if p.goos != "windows" {
			p.effective = ManagedLauncherEffective(p.options.Path, POSIXLauncherPath(p.homeDir), p.goos)
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
		if p.goos != "windows" {
			if p.effective {
				p.activationReason = activationReasonApplied
			} else {
				p.activationReason = activationReasonPathPending
			}
		} else {
			p.activationReason = activationReasonApplied
		}
		p.applied = true
		return nil
	}

	for _, path := range p.paths {
		snapshot := p.before[path]
		if !snapshot.exists || !snapshot.owned {
			continue
		}
		if err := requireSnapshot(path, snapshot); err != nil {
			return p.failAndRollback(fmt.Errorf("revalidate managed OpenCode launcher %q before removal: %w", path, err))
		}
		if err := p.options.RemoveFile(path); err != nil {
			if os.IsNotExist(err) {
				return p.failAndRollback(fmt.Errorf("managed OpenCode launcher %q disappeared after revalidation during removal: %w", path, err))
			}
			return p.failAndRollback(fmt.Errorf("remove managed OpenCode launcher %q: %w", path, err))
		}
		p.changed = append(p.changed, path)
	}
	if err := p.applyProfiles(); err != nil {
		return p.failAndRollback(err)
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
		if err := p.requireRollbackLauncher(path); err != nil {
			rollbackErr = errors.Join(rollbackErr, fmt.Errorf("rollback preserve changed OpenCode launcher %q: %w", path, err))
			continue
		}
		if !snapshot.exists {
			if err := p.options.RemoveFile(path); err != nil && !os.IsNotExist(err) {
				rollbackErr = errors.Join(rollbackErr, fmt.Errorf("rollback remove managed OpenCode launcher %q: %w", path, err))
			}
			continue
		}
		result, err := p.options.WriteFile(path, snapshot.data, snapshot.mode)
		if err != nil {
			if result.Changed {
				if chmodErr := os.Chmod(path, snapshot.mode); chmodErr != nil {
					rollbackErr = errors.Join(rollbackErr, fmt.Errorf("rollback restore managed OpenCode launcher mode %q: %w", path, chmodErr))
				}
			}
			rollbackErr = errors.Join(rollbackErr, fmt.Errorf("rollback restore managed OpenCode launcher %q (changed=%t): %w", path, result.Changed, err))
			continue
		}
		if err := os.Chmod(path, snapshot.mode); err != nil {
			rollbackErr = errors.Join(rollbackErr, fmt.Errorf("rollback restore managed OpenCode launcher mode %q: %w", path, err))
		}
	}
	for i := len(p.profiles) - 1; i >= 0; i-- {
		change := &p.profiles[i]
		if !change.applied {
			continue
		}
		if err := requireSnapshot(change.path, launcherSnapshot{exists: true, data: change.desired, mode: profileMode(change)}); err != nil {
			rollbackErr = errors.Join(rollbackErr, fmt.Errorf("rollback preserve changed OpenCode shell profile %q: %w", change.path, err))
			continue
		}
		if !change.before.exists {
			if err := p.options.RemoveFile(change.path); err != nil && !os.IsNotExist(err) {
				rollbackErr = errors.Join(rollbackErr, fmt.Errorf("rollback remove managed OpenCode shell profile block %q: %w", change.path, err))
			}
			continue
		}
		result, err := p.options.WriteFile(change.path, change.before.data, change.before.mode)
		if err != nil {
			if result.Changed {
				if chmodErr := os.Chmod(change.path, change.before.mode); chmodErr != nil {
					rollbackErr = errors.Join(rollbackErr, fmt.Errorf("rollback restore OpenCode shell profile mode %q: %w", change.path, chmodErr))
				}
			}
			rollbackErr = errors.Join(rollbackErr, fmt.Errorf("rollback restore OpenCode shell profile %q (changed=%t): %w", change.path, result.Changed, err))
			continue
		}
		if err := os.Chmod(change.path, change.before.mode); err != nil {
			rollbackErr = errors.Join(rollbackErr, fmt.Errorf("rollback restore OpenCode shell profile mode %q: %w", change.path, err))
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

func (p *ActivationPlan) requireRollbackLauncher(path string) error {
	if p.action == activationActionOff {
		_, err := os.Lstat(path)
		if os.IsNotExist(err) {
			return nil
		}
		if err != nil {
			return err
		}
		return errors.New("path changed after activation removal")
	}
	return requireWrittenLauncher(path, p.desired[path], p.goos)
}

const (
	profileStart = "# >>> gentle-ai managed OpenCode launcher >>>"
	profileEnd   = "# <<< gentle-ai managed OpenCode launcher <<<"
)

func ManagedProfilePaths(homeDir string) []string {
	return []string{filepath.Join(homeDir, ".zprofile"), filepath.Join(homeDir, ".bash_profile")}
}

func loginProfile(homeDir, shell string) (string, bool) {
	switch filepath.Base(shell) {
	case "zsh":
		return filepath.Join(homeDir, ".zprofile"), true
	case "bash":
		path := filepath.Join(homeDir, ".bash_profile")
		_, err := os.Lstat(path)
		return path, err == nil
	default:
		return "", false
	}
}

func profileBlock(binDir string, endings ...string) string {
	eol := "\n"
	if len(endings) > 0 {
		eol = endings[0]
	}
	return profileStart + eol + "export PATH=" + shellQuote(binDir) + ":\"$PATH\"" + eol + profileEnd + eol
}

func addProfileChange(path, binDir string) (profileChange, error) {
	before, reason, err := readProfileSnapshot(path)
	if err != nil {
		return profileChange{}, err
	}
	if reason != "" {
		return profileChange{path: path, before: before, profileReason: reason}, nil
	}
	desired, changed, err := rewriteProfileBlock(before.data, binDir, false)
	if err != nil {
		return profileChange{path: path, before: before, profileReason: profileMarkerRefusal(path)}, nil
	}
	return profileChange{path: path, before: before, desired: desired, changed: changed}, nil
}

func removeProfileChange(path, _ string) (profileChange, error) {
	before, reason, err := readProfileSnapshot(path)
	if err != nil || !before.exists {
		return profileChange{}, err
	}
	if reason != "" {
		return profileChange{path: path, before: before, profileReason: reason}, nil
	}
	desired, changed, err := rewriteProfileBlock(before.data, "", true)
	if err != nil {
		return profileChange{path: path, before: before, profileReason: profileMarkerRefusal(path)}, nil
	}
	return profileChange{path: path, before: before, desired: desired, changed: changed}, nil
}

func profileMarkerRefusal(path string) string {
	return fmt.Sprintf("%s: login profile %q contains malformed, nested, unbalanced, or multiple managed launcher markers; no profile content was changed", profilePendingReason, path)
}

func profileSafetyReason(path string, info os.FileInfo) string {
	switch {
	case info.Mode()&os.ModeSymlink != 0:
		return fmt.Sprintf("%s: login profile %q uses unsupported symlink topology; refusing to follow or mutate its target", profilePendingReason, path)
	case !info.Mode().IsRegular():
		return fmt.Sprintf("%s: login profile %q uses unsupported non-regular topology; refusing to modify it", profilePendingReason, path)
	case info.Mode().Perm()&0o222 == 0:
		return fmt.Sprintf("%s: login profile %q is read-only (mode %04o); refusing to modify it", profilePendingReason, path, info.Mode().Perm())
	default:
		return ""
	}
}

func readProfileSnapshot(path string) (launcherSnapshot, string, error) {
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return launcherSnapshot{}, "", nil
	}
	if err != nil {
		return launcherSnapshot{}, "", fmt.Errorf("inspect OpenCode shell profile %q: %w", path, err)
	}
	if reason := profileSafetyReason(path, info); reason != "" {
		return launcherSnapshot{exists: true, mode: info.Mode().Perm()}, reason, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return launcherSnapshot{}, "", fmt.Errorf("read OpenCode shell profile %q: %w", path, err)
	}
	return launcherSnapshot{exists: true, data: data, mode: info.Mode().Perm()}, "", nil
}

type profileBlockRange struct {
	start, end int
	eol        string
}

var (
	errMalformedProfile = errors.New("managed profile markers are malformed")
	profileBlockPattern = regexp.MustCompile(`(?m)^` + regexp.QuoteMeta(profileStart) + `\r?\nexport PATH='(([^'\r\n]|'\\'')*)':"\$PATH"\r?\n` + regexp.QuoteMeta(profileEnd) + `\r?\n`)
)

func parseManagedProfileBlock(data []byte) (*profileBlockRange, error) {
	text := string(data)
	if strings.Count(text, profileStart) == 0 && strings.Count(text, profileEnd) == 0 {
		return nil, nil
	}
	if strings.Count(text, profileStart) != 1 || strings.Count(text, profileEnd) != 1 {
		return nil, errMalformedProfile
	}
	matches := profileBlockPattern.FindStringSubmatchIndex(text)
	if len(matches) < 4 {
		return nil, errMalformedProfile
	}
	start, end := matches[0], matches[1]
	raw := text[matches[2]:matches[3]]
	binDir, ok := parseProfileExport("export PATH='" + raw + "':\"$PATH\"")
	if !ok {
		return nil, errMalformedProfile
	}
	eol := "\n"
	if strings.HasPrefix(text[start+len(profileStart):], "\r\n") {
		eol = "\r\n"
	}
	if text[start:end] != profileBlock(binDir, eol) {
		return nil, errMalformedProfile
	}
	return &profileBlockRange{start: start, end: end, eol: eol}, nil
}

func parseProfileExport(line string) (string, bool) {
	const prefix = "export PATH='"
	const suffix = "':\"$PATH\""
	if !strings.HasPrefix(line, prefix) || !strings.HasSuffix(line, suffix) {
		return "", false
	}
	raw := line[len(prefix) : len(line)-len(suffix)]
	if raw == "" {
		return "", false
	}
	parts := strings.Split(raw, "'\\''")
	for _, part := range parts {
		if strings.Contains(part, "'") {
			return "", false
		}
	}
	binDir := strings.Join(parts, "'")
	return binDir, shellQuote(binDir) == "'"+raw+"'"
}

func rewriteProfileBlock(data []byte, binDir string, remove bool) ([]byte, bool, error) {
	block, err := parseManagedProfileBlock(data)
	if err != nil {
		return nil, false, err
	}
	if block == nil {
		if remove {
			return append([]byte(nil), data...), false, nil
		}
		desired := append([]byte(nil), data...)
		eol := profileLineEnding(desired)
		if len(desired) > 0 && !bytes.HasSuffix(desired, []byte(eol)) {
			desired = append(desired, eol...)
		}
		desired = append(desired, profileBlock(binDir, eol)...)
		return desired, true, nil
	}
	replacement := []byte(nil)
	if !remove {
		replacement = []byte(profileBlock(binDir, block.eol))
	}
	desired := make([]byte, 0, len(data)-(block.end-block.start)+len(replacement))
	desired = append(desired, data[:block.start]...)
	desired = append(desired, replacement...)
	desired = append(desired, data[block.end:]...)
	return desired, !bytes.Equal(desired, data), nil
}

func RemoveManagedProfileBlock(path, binDir string) (bool, error) {
	change, err := removeProfileChange(path, binDir)
	if err != nil || !change.changed {
		return false, err
	}
	result, err := writeProfile(change)
	return result.Changed, err
}

func (p *ActivationPlan) applyProfiles() error {
	for i := range p.profiles {
		change := &p.profiles[i]
		if !change.changed {
			continue
		}
		if err := requireSnapshot(change.path, change.before); err != nil {
			return fmt.Errorf("revalidate OpenCode shell profile %q before write: %w", change.path, err)
		}
		result, err := p.options.WriteFile(change.path, change.desired, profileMode(change))
		change.writeResult = result
		change.applied = result.Changed
		if result.Changed {
			if chmodErr := os.Chmod(change.path, profileMode(change)); chmodErr != nil {
				if err != nil {
					return errors.Join(
						fmt.Errorf("write managed OpenCode shell profile block %q: %w", change.path, err),
						fmt.Errorf("set managed OpenCode shell profile mode %q: %w", change.path, chmodErr),
					)
				}
				return fmt.Errorf("set managed OpenCode shell profile mode %q: %w", change.path, chmodErr)
			}
		}
		if err != nil {
			return fmt.Errorf("write managed OpenCode shell profile block %q (changed=%t): %w", change.path, result.Changed, err)
		}
		if !result.Changed {
			return fmt.Errorf("write managed OpenCode shell profile block %q: replacement did not change the destination", change.path)
		}
	}
	return nil
}

func writeProfile(change profileChange) (filemerge.WriteResult, error) {
	result, err := filemerge.WriteFileAtomic(change.path, change.desired, profileMode(&change))
	if result.Changed {
		if chmodErr := os.Chmod(change.path, profileMode(&change)); chmodErr != nil {
			if err != nil {
				return result, errors.Join(err, fmt.Errorf("set managed OpenCode shell profile mode %q: %w", change.path, chmodErr))
			}
			return result, fmt.Errorf("set managed OpenCode shell profile mode %q: %w", change.path, chmodErr)
		}
	}
	return result, err
}

func profileLineEnding(data []byte) string {
	if bytes.Contains(data, []byte("\r\n")) {
		return "\r\n"
	}
	return "\n"
}

func profileMode(change *profileChange) os.FileMode {
	if !change.before.exists {
		return 0o644
	}
	return change.before.mode
}
func requireSnapshot(path string, expected launcherSnapshot) error {
	current, err := readLauncherSnapshot(path)
	if err != nil {
		if !expected.exists && os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if current.exists != expected.exists || current.mode != expected.mode || !bytes.Equal(current.data, expected.data) {
		return errors.New("path changed after preparation")
	}
	return nil
}
func requireWrittenLauncher(path string, desired []byte, goos string) error {
	current, err := readLauncherSnapshot(path)
	if err != nil {
		return err
	}
	if !current.exists || goos != "windows" && current.mode != 0o755 || !bytes.Equal(current.data, desired) {
		return errors.New("path changed after activation write")
	}
	return nil
}

func readLauncherSnapshot(path string) (launcherSnapshot, error) {
	info, data, owned, err := inspectLauncher(path)
	if os.IsNotExist(err) {
		return launcherSnapshot{}, nil
	}
	if err != nil {
		return launcherSnapshot{}, fmt.Errorf("inspect managed OpenCode launcher %q: %w", path, err)
	}
	return launcherSnapshot{exists: true, data: data, mode: info.Mode().Perm(), owned: owned}, nil
}

func inspectLauncher(path string) (os.FileInfo, []byte, bool, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, nil, false, err
	}
	if !info.Mode().IsRegular() {
		return nil, nil, false, fmt.Errorf("refusing non-regular OpenCode launcher collision")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, false, err
	}
	return info, data, IsManagedLauncher(path, data), nil
}

func IsManagedLauncher(path string, data []byte) bool {
	base := strings.ToLower(filepath.Base(path))
	text := string(data)
	switch base {
	case "opencode":
		prefix := "#!/bin/sh\n# " + OwnershipMarker + "\nset -eu\nif [ -z \"${" + BackgroundSubagentsEnv + "+x}\" ]; then\n  export " + BackgroundSubagentsEnv + "=true\nfi\nexec "
		target, ok := canonicalTarget(text, prefix, " \"$@\"\n", "'", "'\\''")
		return ok && text == posixLauncher(target)
	case "opencode.cmd":
		prefix := "@echo off\r\nrem " + OwnershipMarker + "\r\nsetlocal\r\nif not defined " + BackgroundSubagentsEnv + " set \"" + BackgroundSubagentsEnv + "=true\"\r\n"
		target, ok := canonicalCMDTarget(text, prefix)
		return ok && windowsTargetSafe(target) && text == windowsCMDLauncher(target)
	case "opencode.ps1":
		prefix := "# " + OwnershipMarker + "\r\n$ErrorActionPreference = 'Stop'\r\nif (-not (Test-Path Env:" + BackgroundSubagentsEnv + ")) { $env:" + BackgroundSubagentsEnv + " = 'true' }\r\n& "
		target, ok := canonicalTarget(text, prefix, " @args\r\nexit $LASTEXITCODE\r\n", "'", "''")
		return ok && text == windowsPS1Launcher(target)
	default:
		return false
	}
}

func windowsTargetSafe(target string) bool { return !strings.Contains(target, "%") }

// ResolveManagedLauncher resolves the first executable OpenCode command in the
// supplied PATH snapshot and verifies that it is the expected Gentle-owned
// launcher. It follows exec.LookPath's PATH semantics without consulting or
// mutating the process-global PATH: POSIX empty entries mean the current
// directory, Windows empty entries are skipped, relative results fail closed
// with ErrDot, and PATH entries are never trimmed or quote-normalized.
func ResolveManagedLauncher(pathValue, launcher, goos string) (string, error) {
	if goos == "" {
		goos = runtime.GOOS
	}
	if goos == "windows" {
		return resolveManagedLauncherWindows(pathValue, launcher)
	}
	for _, entry := range splitPath(pathValue, goos) {
		// exec.LookPath treats an empty POSIX PATH entry as the current directory.
		// Keep that entry relative so an executable found there produces ErrDot.
		if entry == "" {
			entry = "."
		}
		candidate, ok := firstManagedExecutable(entry, goos)
		if !ok {
			continue
		}
		if !filepath.IsAbs(candidate) {
			return "", fmt.Errorf("managed OpenCode launcher resolved through relative PATH entry %q: %w", entry, exec.ErrDot)
		}
		return validateManagedLauncherCandidate(candidate, launcher, goos)
	}
	return "", fmt.Errorf("managed OpenCode launcher %q is not the first executable on PATH", launcher)
}

func resolveManagedLauncherWindows(pathValue, launcher string) (string, error) {
	if _, disabled := os.LookupEnv("NoDefaultCurrentDirectoryInExePath"); !disabled {
		if dotPath, _ := firstManagedExecutable(".", "windows"); dotPath != "" {
			return "", fmt.Errorf("managed OpenCode launcher resolved through relative PATH entry %q: %w", dotPath, exec.ErrDot)
		}
	}
	for _, entry := range splitPath(pathValue, "windows") {
		// Windows LookPath intentionally skips empty PATH entries; its implicit
		// current-directory probe above is controlled separately.
		if entry == "" {
			continue
		}
		candidate, ok := firstManagedExecutable(entry, "windows")
		if !ok {
			continue
		}
		if !filepath.IsAbs(candidate) {
			return "", fmt.Errorf("managed OpenCode launcher resolved through relative PATH entry %q: %w", candidate, exec.ErrDot)
		}
		return validateManagedLauncherCandidate(candidate, launcher, "windows")
	}
	return "", fmt.Errorf("managed OpenCode launcher %q is not the first executable on PATH", launcher)
}

func firstManagedExecutable(entry, goos string) (string, bool) {
	for _, name := range targetNames(goos) {
		candidate := filepath.Join(entry, name)
		if pathEntryExecutable(candidate, goos) {
			return candidate, true
		}
	}
	return "", false
}

func validateManagedLauncherCandidate(candidate, launcher, goos string) (string, error) {
	if !samePath(candidate, launcher, goos) {
		return "", fmt.Errorf("managed OpenCode launcher is shadowed by executable %q", candidate)
	}
	info, _, owned, err := inspectLauncher(candidate)
	if err != nil {
		return "", fmt.Errorf("inspect managed OpenCode launcher %q: %w", candidate, err)
	}
	if info == nil || goos != "windows" && info.Mode().Perm()&0o111 == 0 {
		return "", fmt.Errorf("managed OpenCode launcher %q has an unsafe mode", candidate)
	}
	if !owned {
		return "", fmt.Errorf("managed OpenCode launcher %q is not Gentle-owned", candidate)
	}
	return candidate, nil
}

// ManagedLauncherEffective reports whether launcher is the first executable
// matching the managed OpenCode name in pathValue. It is intentionally based on
// the supplied PATH snapshot, so callers can evaluate a pre-mutation or
// independently resolved fresh-shell environment without consulting a mutated
// process PATH.
func ManagedLauncherEffective(pathValue, launcher, goos string) bool {
	_, err := ResolveManagedLauncher(pathValue, launcher, goos)
	return err == nil
}

func pathResolvesTo(pathValue, launcher, goos string) bool {
	return ManagedLauncherEffective(pathValue, launcher, goos)
}

func pathEntryExecutable(path, goos string) bool {
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() {
		return false
	}
	return goos == "windows" || info.Mode().Perm()&0o111 != 0
}

func canonicalTarget(text, prefix, suffix, quote, escapedQuote string) (string, bool) {
	if !strings.HasPrefix(text, prefix) || !strings.HasSuffix(text, suffix) {
		return "", false
	}
	raw := strings.TrimSuffix(strings.TrimPrefix(text, prefix), suffix)
	if len(raw) < 2 || !strings.HasPrefix(raw, quote) || !strings.HasSuffix(raw, quote) {
		return "", false
	}
	parts := strings.Split(raw[len(quote):len(raw)-len(quote)], escapedQuote)
	for _, part := range parts {
		if strings.Contains(part, quote) {
			return "", false
		}
	}
	return strings.Join(parts, quote), true
}

func canonicalCMDTarget(text, prefix string) (string, bool) {
	const suffix = "\r\nexit /b %ERRORLEVEL%\r\n"
	if !strings.HasPrefix(text, prefix) || !strings.HasSuffix(text, suffix) {
		return "", false
	}
	body := strings.TrimSuffix(strings.TrimPrefix(text, prefix), suffix)
	for _, form := range []string{"\"", "powershell -NoProfile -ExecutionPolicy Bypass -File \""} {
		if !strings.HasPrefix(body, form) || !strings.HasSuffix(body, "\" %*") {
			continue
		}
		raw := strings.TrimSuffix(strings.TrimPrefix(body, form), "\" %*")
		target := strings.ReplaceAll(raw, `""`, `"`)
		if windowsCMDLauncher(target) == text {
			return target, true
		}
	}
	return "", false
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
