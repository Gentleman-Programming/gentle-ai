package opencode

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// POSIX login shells do not read the process PATH of the installer, so the
// managed bin directory is persisted as a Gentle-owned block in the login
// profile the user's shell actually reads. Only the exact block below is ever
// added, replaced, or removed; every other byte of the profile belongs to the
// user and is preserved.
const (
	profileStart = "# >>> gentle-ai managed OpenCode launcher >>>"
	profileEnd   = "# <<< gentle-ai managed OpenCode launcher <<<"
)

// newProfileMode is the mode of a login profile created by activation.
const newProfileMode os.FileMode = 0o644

type profileChange struct {
	path    string
	before  launcherSnapshot
	desired []byte
	changed bool
	applied bool
}

// ManagedProfilePaths returns every login profile that activation may write.
// Deactivation and uninstall scan all of them because the login shell can
// change between activation and removal.
func ManagedProfilePaths(homeDir string) []string {
	return []string{
		filepath.Join(homeDir, ".zprofile"),
		filepath.Join(homeDir, ".bash_profile"),
		filepath.Join(homeDir, ".bash_login"),
		filepath.Join(homeDir, ".profile"),
	}
}

// ProfileExportLine is the PATH export the managed block carries. Users whose
// login profile cannot be updated safely add it themselves.
func ProfileExportLine(binDir string) string {
	return "export PATH=" + shellQuote(binDir) + ":\"$PATH\""
}

func profileBlock(binDir, eol string) string {
	return profileStart + eol + ProfileExportLine(binDir) + eol + profileEnd + eol
}

// loginProfile returns the login profile options.Shell reads, or a reason why
// no profile can be chosen safely.
func loginProfile(homeDir string, options ActivationOptions) (string, string) {
	shell := filepath.Base(options.Shell)
	switch shell {
	case "zsh":
		if options.ZDotDir != "" && !samePath(options.ZDotDir, homeDir, options.OS) {
			return "", fmt.Sprintf("zsh reads its login profile from ZDOTDIR %q, which Gentle AI does not manage", options.ZDotDir)
		}
		return filepath.Join(homeDir, ".zprofile"), ""
	case "bash":
		// A bash login shell reads only the first of these that exists, so a
		// new .bash_profile would silently shadow an existing .profile.
		for _, name := range []string{".bash_profile", ".bash_login", ".profile"} {
			path := filepath.Join(homeDir, name)
			if _, err := os.Lstat(path); err == nil {
				return path, ""
			} else if !os.IsNotExist(err) {
				return "", fmt.Sprintf("inspect login profile %q: %v", path, err)
			}
		}
		return filepath.Join(homeDir, ".profile"), ""
	case "sh", "dash", "ksh":
		return filepath.Join(homeDir, ".profile"), ""
	case ".", "":
		return "", "the login shell is unknown because SHELL is not set"
	default:
		return "", fmt.Sprintf("login shell %q is not supported for automatic PATH persistence", shell)
	}
}

// readProfileSnapshot reads a login profile for mutation. A non-empty reason
// means the profile must not be modified; it is not an error because the user
// can persist PATH manually.
func readProfileSnapshot(path string) (launcherSnapshot, string, error) {
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return launcherSnapshot{}, "", nil
	}
	if err != nil {
		return launcherSnapshot{}, "", fmt.Errorf("inspect OpenCode login profile %q: %w", path, err)
	}
	switch {
	case info.Mode()&os.ModeSymlink != 0:
		return launcherSnapshot{}, fmt.Sprintf("login profile %q is a symlink; refusing to follow or modify its target", path), nil
	case !info.Mode().IsRegular():
		return launcherSnapshot{}, fmt.Sprintf("login profile %q is not a regular file; refusing to modify it", path), nil
	case info.Mode().Perm()&0o200 == 0:
		return launcherSnapshot{}, fmt.Sprintf("login profile %q is read-only (mode %04o); refusing to modify it", path, info.Mode().Perm()), nil
	}
	snapshot, err := readLauncherSnapshot(path)
	if err != nil {
		return launcherSnapshot{}, "", err
	}
	snapshot.owned = false
	return snapshot, "", nil
}

func addProfileChange(path, binDir string) (profileChange, string, error) {
	before, reason, err := readProfileSnapshot(path)
	if err != nil || reason != "" {
		return profileChange{}, reason, err
	}
	desired, err := rewriteProfileBlock(before.data, binDir, false)
	if err != nil {
		return profileChange{}, profileMarkerRefusal(path), nil
	}
	return profileChange{path: path, before: before, desired: desired, changed: !before.exists || !bytes.Equal(desired, before.data)}, "", nil
}

// removeProfileChange prepares removal of the managed block. Profiles that are
// absent, unsafe to modify, or carry malformed markers are left untouched:
// activation never writes such a profile, so any block there is user content.
func removeProfileChange(path string) (profileChange, error) {
	before, reason, err := readProfileSnapshot(path)
	if err != nil || reason != "" || !before.exists {
		return profileChange{}, err
	}
	desired, err := rewriteProfileBlock(before.data, "", true)
	if err != nil {
		return profileChange{}, nil
	}
	return profileChange{path: path, before: before, desired: desired, changed: !bytes.Equal(desired, before.data)}, nil
}

func profileMarkerRefusal(path string) string {
	return fmt.Sprintf("login profile %q contains malformed, edited, or multiple Gentle AI launcher markers; refusing to modify it", path)
}

var (
	errMalformedProfile = errors.New("managed profile markers are malformed")
	profileBlockPattern = regexp.MustCompile(`(?m)^` + regexp.QuoteMeta(profileStart) + `(\r?\n)export PATH='((?:[^'\r\n]|'\\'')*)':"\$PATH"\r?\n` + regexp.QuoteMeta(profileEnd) + `\r?\n`)
)

type profileBlockRange struct {
	start, end int
	binDir     string
	eol        string
}

// parseManagedProfileBlock locates the single canonical managed block. Any
// other use of the markers is malformed so a hand-edited block is never
// rewritten or removed.
func parseManagedProfileBlock(data []byte) (*profileBlockRange, error) {
	text := string(data)
	starts, ends := strings.Count(text, profileStart), strings.Count(text, profileEnd)
	if starts == 0 && ends == 0 {
		return nil, nil
	}
	if starts != 1 || ends != 1 {
		return nil, errMalformedProfile
	}
	match := profileBlockPattern.FindStringSubmatchIndex(text)
	if match == nil {
		return nil, errMalformedProfile
	}
	block := &profileBlockRange{start: match[0], end: match[1], eol: text[match[2]:match[3]]}
	raw := text[match[4]:match[5]]
	block.binDir = strings.ReplaceAll(raw, `'\''`, "'")
	if block.binDir == "" || text[block.start:block.end] != profileBlock(block.binDir, block.eol) {
		return nil, errMalformedProfile
	}
	return block, nil
}

func rewriteProfileBlock(data []byte, binDir string, remove bool) ([]byte, error) {
	block, err := parseManagedProfileBlock(data)
	if err != nil {
		return nil, err
	}
	if block == nil {
		if remove {
			return append([]byte(nil), data...), nil
		}
		// Follow the file's last line ending: one pasted CRLF line in an LF
		// profile must not append a CR that corrupts the PATH entry in bash.
		eol := "\n"
		if last := bytes.LastIndexByte(data, '\n'); last > 0 && data[last-1] == '\r' {
			eol = "\r\n"
		}
		desired := append([]byte(nil), data...)
		if len(desired) > 0 && !bytes.HasSuffix(desired, []byte("\n")) {
			desired = append(desired, eol...)
		}
		return append(desired, profileBlock(binDir, eol)...), nil
	}
	var replacement []byte
	if !remove {
		replacement = []byte(profileBlock(binDir, block.eol))
	}
	desired := make([]byte, 0, len(data)-(block.end-block.start)+len(replacement))
	desired = append(desired, data[:block.start]...)
	desired = append(desired, replacement...)
	return append(desired, data[block.end:]...), nil
}

// ManagedProfileWithBinDir returns the first managed login profile whose block
// persists binDir. Doctor uses it to diagnose missing PATH persistence.
func ManagedProfileWithBinDir(homeDir, binDir string) (string, bool) {
	// Only the login profile the current $SHELL actually reads counts: a
	// block left in a shadowed or another shell's profile does not persist
	// PATH for the user's login shells.
	path, reason := loginProfile(homeDir, ActivationOptions{}.normalized())
	if reason != "" {
		return "", false
	}
	snapshot, reason, err := readProfileSnapshot(path)
	if err != nil || reason != "" || !snapshot.exists {
		return "", false
	}
	block, err := parseManagedProfileBlock(snapshot.data)
	if err == nil && block != nil && samePath(block.binDir, binDir, "linux") {
		return path, true
	}
	return "", false
}

// HasManagedProfileBlock reports whether RemoveManagedProfileBlock would change
// path.
func HasManagedProfileBlock(path string) bool {
	change, err := removeProfileChange(path)
	return err == nil && change.changed
}

// RemoveManagedProfileBlock removes only the canonical managed block from
// path, revalidating the prepared bytes immediately before the rewrite.
func RemoveManagedProfileBlock(path string) (bool, error) {
	change, err := removeProfileChange(path)
	if err != nil || !change.changed {
		return false, err
	}
	plan := &ActivationPlan{action: activationActionOff, options: ActivationOptions{}.normalized(), profiles: []profileChange{change}}
	if err := plan.applyProfiles(); err != nil {
		return false, plan.failAndRollback(err)
	}
	return true, nil
}

// prepareProfileActivation selects and preflights the login profile for an
// activation plan and extends the restart guidance with the outcome.
func (p *ActivationPlan) prepareProfileActivation() error {
	binDir := BinDir(p.homeDir)
	profile, reason := loginProfile(p.homeDir, p.options)
	var change profileChange
	if reason == "" {
		var err error
		change, reason, err = addProfileChange(profile, binDir)
		if err != nil {
			return err
		}
	}
	if reason != "" {
		p.capability.RestartGuidance += fmt.Sprintf(" PATH persistence is pending: %s. Add %s to your login profile, then start a new login shell.", reason, ProfileExportLine(binDir))
		return nil
	}
	p.profiles = []profileChange{change}
	p.capability.RestartGuidance += fmt.Sprintf(" Login profile %s persists %s on PATH; start a new login shell so bare opencode resolves through the managed launcher.", profile, binDir)
	return nil
}

// prepareProfileDeactivation preflights removal of every managed block.
func (p *ActivationPlan) prepareProfileDeactivation() error {
	for _, path := range ManagedProfilePaths(p.homeDir) {
		change, err := removeProfileChange(path)
		if err != nil {
			return err
		}
		if change.changed {
			p.profiles = append(p.profiles, change)
		}
	}
	return nil
}

// applyProfiles writes prepared profile changes, revalidating each profile
// against its prepared snapshot immediately before the write.
func (p *ActivationPlan) applyProfiles() error {
	for i := range p.profiles {
		change := &p.profiles[i]
		change.applied = false
		if !change.changed {
			continue
		}
		if err := requireSnapshot(change.path, change.before); err != nil {
			return fmt.Errorf("revalidate OpenCode login profile %q before write: %w", change.path, err)
		}
		// Marked before the write so a partial failure is still rolled back.
		change.applied = true
		if err := p.options.WriteFile(change.path, change.desired, change.mode()); err != nil {
			return fmt.Errorf("write managed OpenCode login profile block %q: %w", change.path, err)
		}
	}
	return nil
}

// rollbackProfiles restores profiles this plan wrote. A profile edited after
// this plan's write belongs to a concurrent writer and is preserved.
func (p *ActivationPlan) rollbackProfiles() error {
	var rollbackErr error
	for i := len(p.profiles) - 1; i >= 0; i-- {
		change := &p.profiles[i]
		if !change.applied {
			continue
		}
		current, err := readLauncherSnapshot(change.path)
		if err != nil {
			rollbackErr = errors.Join(rollbackErr, fmt.Errorf("rollback inspect OpenCode login profile %q: %w", change.path, err))
			continue
		}
		if sameSnapshot(current, change.before) {
			change.applied = false
			continue
		}
		if !sameSnapshot(current, launcherSnapshot{exists: true, data: change.desired, mode: change.mode()}) {
			rollbackErr = errors.Join(rollbackErr, fmt.Errorf("rollback preserve changed OpenCode login profile %q: path changed after this plan's mutation", change.path))
			continue
		}
		if !change.before.exists {
			desired := change.desired
			result, err := removeOwnedFile(change.path, func(data []byte) bool { return bytes.Equal(data, desired) })
			if err != nil {
				rollbackErr = errors.Join(rollbackErr, fmt.Errorf("rollback remove OpenCode login profile %q: %w", change.path, err))
				continue
			}
			if !result.Removed() && result.Status != ManagedLauncherRemovalAbsent {
				rollbackErr = errors.Join(rollbackErr, fmt.Errorf("rollback remove OpenCode login profile %q: %s", change.path, result.Status))
				continue
			}
		} else if err := p.options.WriteFile(change.path, change.before.data, change.before.mode); err != nil {
			rollbackErr = errors.Join(rollbackErr, fmt.Errorf("rollback restore OpenCode login profile %q: %w", change.path, err))
			continue
		}
		change.applied = false
	}
	return rollbackErr
}

// appliedProfilePaths returns profiles changed by the last Apply call.
func (p *ActivationPlan) appliedProfilePaths() []string {
	var paths []string
	for _, change := range p.profiles {
		if change.applied {
			paths = append(paths, change.path)
		}
	}
	return paths
}

func (c *profileChange) mode() os.FileMode {
	if !c.before.exists {
		return newProfileMode
	}
	return c.before.mode
}
