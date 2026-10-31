//go:build linux || darwin

package shellinstaller

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"syscall"

	"golang.org/x/sys/unix"
)

// The launcher dir reaches new terminals through one marked line in the shell
// startup file, chosen like the gentle-pi web installer's profileFor (#5758).
// The line names $HOME, so no path is ever quoted into a shell file.
const (
	userLauncherProfileIntent = "GENTLE_AI_LAUNCHER_PROFILE"
	userLauncherPathMark      = "# gentle-ai: gentle-shell launcher"
	userLauncherFishFile      = "gentle-ai-shell.fish"
	userLauncherShCommand     = `export PATH="$HOME/.local/bin:$PATH"`
	userLauncherFishCommand   = `set -gx PATH $HOME/.local/bin $PATH`
)

const (
	userLauncherProfileAdded   = "added"
	userLauncherProfilePresent = "present"
	userLauncherProfileManual  = "manual"
)

func userLauncherPathCommand(fish bool) string {
	if fish {
		return userLauncherFishCommand
	}
	return userLauncherShCommand
}

func userLauncherPathLine(fish bool) string {
	return userLauncherPathCommand(fish) + " " + userLauncherPathMark
}

// userLauncherProfileFiles lists every startup file the choice can name, per
// shell, in order of preference.
func userLauncherProfileFiles(goos string, getenv func(string) string, home string) map[string][]string {
	config := getenv("XDG_CONFIG_HOME")
	if !filepath.IsAbs(config) {
		config = filepath.Join(home, ".config")
	}
	zdot := getenv("ZDOTDIR")
	if !filepath.IsAbs(zdot) {
		zdot = home
	}
	bash := []string{filepath.Join(home, ".bashrc")}
	if goos == "darwin" {
		bash = []string{filepath.Join(home, ".bash_profile"), filepath.Join(home, ".bash_login"), filepath.Join(home, ".profile")}
	}
	return map[string][]string{
		"zsh":   {filepath.Join(zdot, ".zshrc")},
		"bash":  bash,
		"fish":  {filepath.Join(config, "fish", "conf.d", userLauncherFishFile)},
		"other": {filepath.Join(home, ".profile")},
	}
}

// userLauncherProfileFor is the startup file the user's shell reads for new
// terminals: zsh reads $ZDOTDIR/.zshrc; macOS terminals start login bash,
// which reads exactly one of ~/.bash_profile, ~/.bash_login and ~/.profile, so
// the first existing one is used and ~/.bash_profile is created only when none
// exists; a Linux terminal starts non-login bash, which reads ~/.bashrc; fish
// gets our own conf.d file; anything else reads ~/.profile.
func userLauncherProfileFor(goos string, getenv func(string) string, home string) string {
	files := userLauncherProfileFiles(goos, getenv, home)
	shell := filepath.Base(getenv("SHELL"))
	if shell == "bash" && goos == "darwin" {
		for _, path := range files["bash"] {
			if _, err := os.Stat(path); err == nil {
				return path
			}
		}
	}
	if paths, ok := files[shell]; ok {
		return paths[0]
	}
	return files["other"][0]
}

// userLauncherHomeCandidates are the only files a forwarded profile may name:
// the choices for the unit's own HOME, without ZDOTDIR or XDG_CONFIG_HOME.
func userLauncherHomeCandidates(home string) []string {
	var candidates []string
	for _, paths := range userLauncherProfileFiles(runtime.GOOS, func(string) string { return "" }, home) {
		candidates = append(candidates, paths...)
	}
	return candidates
}

// userLauncherProfile is the startup file for this install. A delegated unit
// has no SHELL or ZDOTDIR and receives the caller's choice as an intent; one
// that is not a candidate of its HOME is reported, never edited.
func userLauncherProfile(home string) (string, bool) {
	if intent := os.Getenv(userLauncherProfileIntent); intent != "" {
		return intent, slices.Contains(userLauncherHomeCandidates(home), intent)
	}
	return userLauncherProfileFor(runtime.GOOS, os.Getenv, home), true
}

// userLauncherProfileState says whether path may be edited: "file" (a regular,
// writable file in real folders) or "absent" (creatable in a real, writable
// folder); otherwise why not. A link, or a folder under home reached through
// one (stow, chezmoi, home-manager), is never written through.
func userLauncherProfileState(path, home string) string {
	realHome, err := filepath.EvalSymlinks(home)
	if err != nil {
		return "not-a-file"
	}
	realFolder := func(folder string) bool {
		relative, err := filepath.Rel(home, folder)
		if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
			return true
		}
		resolved, err := filepath.EvalSymlinks(folder)
		return err == nil && resolved == filepath.Join(realHome, relative)
	}
	writable := func(target string) bool { return unix.Access(target, unix.W_OK) == nil }
	if info, err := os.Lstat(path); err == nil {
		switch {
		case info.Mode()&os.ModeSymlink != 0:
			return "symlink"
		case !info.Mode().IsRegular():
			return "not-a-file"
		case !realFolder(filepath.Dir(path)):
			return "symlink"
		case !writable(path):
			return "not-writable"
		}
		return "file"
	} else if !os.IsNotExist(err) {
		return "not-a-file"
	}
	folder := filepath.Dir(path)
	for {
		if _, err := os.Lstat(folder); err == nil {
			break
		}
		if filepath.Dir(folder) == folder {
			return "not-a-file"
		}
		folder = filepath.Dir(folder)
	}
	info, _ := os.Lstat(folder)
	switch {
	case info.Mode()&os.ModeSymlink != 0:
		return "symlink"
	case !info.IsDir():
		return "not-a-file"
	case !realFolder(folder):
		return "symlink"
	case !writable(folder):
		return "not-writable"
	}
	return "absent"
}

var userLauncherStateReasons = map[string]string{
	"symlink":      "it is a symlink",
	"not-a-file":   "it is not a regular file",
	"not-writable": "it is not writable",
}

func userHoldsLine(path, line string) bool {
	data, err := os.ReadFile(path)
	return err == nil && slices.Contains(strings.Split(string(data), "\n"), line)
}

// userApplyLauncherPath puts the marked line in the startup file once. It is
// convenience, never installation authority: anything it may not or cannot
// write is reported as the line to add by hand.
func userApplyLauncherPath(report *userShellLauncherReport, home string) {
	profile, ok := userLauncherProfile(home)
	report.Fish = strings.HasSuffix(profile, ".fish")
	report.ProfileState = userLauncherProfileManual
	if !ok {
		report.ProfileReason = "the requested file is not one this installer edits"
		return
	}
	report.Profile = profile
	line := userLauncherPathLine(report.Fish)
	if userHoldsLine(profile, line) {
		report.ProfileState = userLauncherProfilePresent
		return
	}
	state := userLauncherProfileState(profile, home)
	var err error
	switch state {
	case "file":
		err = userAppendLauncherLine(profile, line)
	case "absent":
		err = userCreateLauncherProfile(profile, line)
	default:
		report.ProfileReason = userLauncherStateReasons[state]
		return
	}
	if err != nil {
		report.ProfileReason = err.Error()
		return
	}
	report.ProfileState = userLauncherProfileAdded
}

func userAppendLauncherLine(path, line string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	separator := ""
	if len(data) > 0 && data[len(data)-1] != '\n' {
		separator = "\n"
	}
	// O_NOFOLLOW: a link swapped in after the state check is refused.
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND|syscall.O_NOFOLLOW, 0)
	if err != nil {
		return err
	}
	_, err = file.WriteString(separator + line + "\n")
	return errors.Join(err, file.Close())
}

func userCreateLauncherProfile(path, line string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL|syscall.O_NOFOLLOW, 0644)
	if err != nil {
		return err
	}
	// The delegated unit runs with UMask=0077; a startup file stays 0644.
	err = file.Chmod(0644)
	if err == nil {
		_, err = file.WriteString(line + "\n")
	}
	return errors.Join(err, file.Close())
}

// userRemoveLauncherPath removes exactly our marked lines from every startup
// file the choice could have named. A file left exactly empty (0 bytes) is
// deleted, and so is our own fish file once nothing else is left in it; a file
// with any other content is always kept.
func userRemoveLauncherPath(home string) error {
	candidates := userLauncherHomeCandidates(home)
	for _, paths := range userLauncherProfileFiles(runtime.GOOS, os.Getenv, home) {
		candidates = append(candidates, paths...)
	}
	slices.Sort(candidates)
	lines := []string{userLauncherPathLine(false), userLauncherPathLine(true)}
	for _, path := range slices.Compact(candidates) {
		if userLauncherProfileState(path, home) != "file" {
			continue
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		original := strings.Split(string(data), "\n")
		kept := slices.DeleteFunc(slices.Clone(original), func(line string) bool { return slices.Contains(lines, line) })
		if len(kept) == len(original) {
			continue
		}
		text := strings.Join(kept, "\n")
		if text == "" || filepath.Base(path) == userLauncherFishFile && strings.TrimSpace(text) == "" {
			if err := os.Remove(path); err != nil {
				return err
			}
			continue
		}
		if err := userReplaceProfile(path, text); err != nil {
			return err
		}
	}
	return nil
}

func userReplaceProfile(path, text string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	temporary, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".gentle-ai-*")
	if err != nil {
		return err
	}
	_, err = temporary.WriteString(text)
	err = errors.Join(err, temporary.Chmod(info.Mode().Perm()), temporary.Sync(), temporary.Close())
	if err == nil {
		err = os.Rename(temporary.Name(), path)
	}
	if err != nil {
		_ = os.Remove(temporary.Name())
	}
	return err
}
