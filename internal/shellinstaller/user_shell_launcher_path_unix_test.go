//go:build linux || darwin

package shellinstaller

import (
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

const (
	launcherShLine   = `export PATH="$HOME/.local/bin:$PATH" # gentle-ai: gentle-shell launcher`
	launcherFishLine = `set -gx PATH $HOME/.local/bin $PATH # gentle-ai: gentle-shell launcher`
)

// launcherShellEnv pins every input the profile choice reads: the login shell,
// ZDOTDIR, XDG_CONFIG_HOME and the forwarded intents.
func launcherShellEnv(t *testing.T, shell string) {
	t.Helper()
	t.Setenv("SHELL", shell)
	t.Setenv("ZDOTDIR", "")
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("GENTLE_AI_LAUNCHER_DIR_ON_PATH", "")
	t.Setenv("GENTLE_AI_LAUNCHER_PROFILE", "")
}

func readProfile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestUserLauncherProfileForFollowsTheWebInstaller(t *testing.T) {
	home := t.TempDir()
	zdot := filepath.Join(home, "zdot")
	xdg := filepath.Join(home, "xdg")
	for _, test := range []struct {
		name, goos string
		env        map[string]string
		existing   []string
		want       string
	}{
		{"zsh without ZDOTDIR", "linux", map[string]string{"SHELL": "/usr/bin/zsh"}, nil, filepath.Join(home, ".zshrc")},
		{"zsh with ZDOTDIR", "darwin", map[string]string{"SHELL": "/bin/zsh", "ZDOTDIR": zdot}, nil, filepath.Join(zdot, ".zshrc")},
		{"zsh with relative ZDOTDIR", "linux", map[string]string{"SHELL": "/bin/zsh", "ZDOTDIR": "zdot"}, nil, filepath.Join(home, ".zshrc")},
		{"bash on Linux", "linux", map[string]string{"SHELL": "/bin/bash"}, []string{".bash_profile"}, filepath.Join(home, ".bashrc")},
		{"bash on macOS without startup files", "darwin", map[string]string{"SHELL": "/bin/bash"}, nil, filepath.Join(home, ".bash_profile")},
		{"bash on macOS with only .profile", "darwin", map[string]string{"SHELL": "/bin/bash"}, []string{".profile"}, filepath.Join(home, ".profile")},
		{"bash on macOS prefers .bash_login over .profile", "darwin", map[string]string{"SHELL": "/bin/bash"}, []string{".bash_login", ".profile"}, filepath.Join(home, ".bash_login")},
		{"fish without XDG_CONFIG_HOME", "linux", map[string]string{"SHELL": "/usr/bin/fish"}, nil, filepath.Join(home, ".config", "fish", "conf.d", "gentle-ai-shell.fish")},
		{"fish with XDG_CONFIG_HOME", "darwin", map[string]string{"SHELL": "/opt/homebrew/bin/fish", "XDG_CONFIG_HOME": xdg}, nil, filepath.Join(xdg, "fish", "conf.d", "gentle-ai-shell.fish")},
		{"other shell", "linux", map[string]string{"SHELL": "/bin/dash"}, nil, filepath.Join(home, ".profile")},
		{"no shell", "darwin", map[string]string{}, nil, filepath.Join(home, ".profile")},
	} {
		t.Run(test.name, func(t *testing.T) {
			dir := t.TempDir()
			home := home
			want := test.want
			if len(test.existing) != 0 {
				// Existing startup files live in a fresh home per case.
				want = filepath.Join(dir, filepath.Base(test.want))
				home = dir
				for _, name := range test.existing {
					if err := os.WriteFile(filepath.Join(dir, name), nil, 0644); err != nil {
						t.Fatal(err)
					}
				}
			}
			got := userLauncherProfileFor(test.goos, func(key string) string { return test.env[key] }, home)
			if got != want {
				t.Fatalf("profile = %q, want %q", got, want)
			}
		})
	}
}

func TestPublishUserShellLauncherAddsOneMarkedPathLine(t *testing.T) {
	target := launcherTestTarget(t)
	launcherShellEnv(t, "/bin/zsh")
	home := os.Getenv("HOME")
	profile := filepath.Join(home, ".zshrc")
	report, err := PublishUserShellLauncher(target)
	if err != nil {
		t.Fatal(err)
	}
	if got := readProfile(t, profile); got != launcherShLine+"\n" {
		t.Fatalf("profile = %q", got)
	}
	if info, err := os.Lstat(profile); err != nil || info.Mode().Perm() != 0644 {
		t.Fatalf("created profile mode differs: %v %v", info.Mode(), err)
	}
	note := userShellLauncherNote(report)
	want := strings.Join([]string{
		"Commands: " + launcherLink(t, "gentle-shell") + " and " + launcherLink(t, "gs"),
		filepath.Dir(launcherLink(t, "gs")) + " is not on this terminal's PATH.",
		`Current terminal: export PATH="$HOME/.local/bin:$PATH"`,
		"Future terminals: open a new terminal",
		"PATH was added to " + profile,
	}, "\n")
	if note != want {
		t.Fatalf("note =\n%s\nwant\n%s", note, want)
	}
	// Republishing never duplicates the line and says where it already is.
	report, err = PublishUserShellLauncher(target)
	if err != nil {
		t.Fatal(err)
	}
	if got := readProfile(t, profile); got != launcherShLine+"\n" {
		t.Fatalf("profile after republish = %q", got)
	}
	if note := userShellLauncherNote(report); !strings.HasSuffix(note, "Future terminals: open a new terminal\nPATH is already set in "+profile) {
		t.Fatalf("republish note = %s", note)
	}
}

func TestPublishUserShellLauncherAppendsAfterExistingContent(t *testing.T) {
	target := launcherTestTarget(t)
	launcherShellEnv(t, "/bin/zsh")
	profile := filepath.Join(os.Getenv("HOME"), ".zshrc")
	if err := os.WriteFile(profile, []byte("alias ll='ls -l'"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := PublishUserShellLauncher(target); err != nil {
		t.Fatal(err)
	}
	if got := readProfile(t, profile); got != "alias ll='ls -l'\n"+launcherShLine+"\n" {
		t.Fatalf("profile = %q", got)
	}
	if info, _ := os.Lstat(profile); info.Mode().Perm() != 0600 {
		t.Fatalf("existing profile mode changed: %v", info.Mode())
	}
}

func TestPublishUserShellLauncherWritesTheFishLine(t *testing.T) {
	target := launcherTestTarget(t)
	launcherShellEnv(t, "/usr/bin/fish")
	profile := filepath.Join(os.Getenv("HOME"), ".config", "fish", "conf.d", "gentle-ai-shell.fish")
	report, err := PublishUserShellLauncher(target)
	if err != nil {
		t.Fatal(err)
	}
	if got := readProfile(t, profile); got != launcherFishLine+"\n" {
		t.Fatalf("fish profile = %q", got)
	}
	if note := userShellLauncherNote(report); !strings.Contains(note, "Current terminal: set -gx PATH $HOME/.local/bin $PATH\n") {
		t.Fatalf("fish note = %s", note)
	}
}

// A dotfile manager's link (stow, chezmoi, home-manager) is never written
// through: the operator gets the line to add instead.
func TestPublishUserShellLauncherNeverWritesThroughSymlinks(t *testing.T) {
	for _, test := range []struct{ name, linked string }{
		{"linked profile", "file"},
		{"linked profile folder", "folder"},
	} {
		t.Run(test.name, func(t *testing.T) {
			target := launcherTestTarget(t)
			launcherShellEnv(t, "/bin/zsh")
			home := os.Getenv("HOME")
			dotfiles := filepath.Join(home, "dotfiles")
			if err := os.MkdirAll(dotfiles, 0755); err != nil {
				t.Fatal(err)
			}
			real := filepath.Join(dotfiles, ".zshrc")
			if err := os.WriteFile(real, []byte("# mine\n"), 0644); err != nil {
				t.Fatal(err)
			}
			profile := filepath.Join(home, ".zshrc")
			if test.linked == "file" {
				if err := os.Symlink(real, profile); err != nil {
					t.Fatal(err)
				}
			} else {
				zdot := filepath.Join(home, "zdot")
				if err := os.Symlink(dotfiles, zdot); err != nil {
					t.Fatal(err)
				}
				t.Setenv("ZDOTDIR", zdot)
				profile = filepath.Join(zdot, ".zshrc")
			}
			report, err := PublishUserShellLauncher(target)
			if err != nil {
				t.Fatal(err)
			}
			if got := readProfile(t, real); got != "# mine\n" {
				t.Fatalf("linked profile was edited: %q", got)
			}
			note := userShellLauncherNote(report)
			for _, want := range []string{
				"Future terminals: add this line to " + profile + ", then open a new terminal: " + launcherShLine,
				profile + " was not edited: it is a symlink",
			} {
				if !strings.Contains(note, want) {
					t.Fatalf("note misses %q:\n%s", want, note)
				}
			}
		})
	}
}

func TestPublishUserShellLauncherWritesNothingWhenDirOnPath(t *testing.T) {
	target := launcherTestTarget(t)
	launcherShellEnv(t, "/bin/zsh")
	t.Setenv("PATH", filepath.Dir(launcherLink(t, "gs")))
	report, err := PublishUserShellLauncher(target)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(filepath.Join(os.Getenv("HOME"), ".zshrc")); !os.IsNotExist(err) {
		t.Fatalf("profile written while the launcher dir is on PATH: %v", err)
	}
	if note := userShellLauncherNote(report); strings.Contains(note, "PATH") {
		t.Fatalf("PATH guidance shown while the launcher dir is on PATH: %s", note)
	}
}

// The Linux delegated unit has no SHELL or ZDOTDIR: the caller's profile choice
// crosses as one intent, never as the caller's environment.
func TestPublishUserShellLauncherUsesForwardedProfile(t *testing.T) {
	target := launcherTestTarget(t)
	launcherShellEnv(t, "/bin/zsh")
	home := os.Getenv("HOME")
	forwarded := userServiceIntentEnvironment(home)
	if want := "GENTLE_AI_LAUNCHER_PROFILE=" + filepath.Join(home, ".zshrc"); strings.Join(forwarded, "\n") != want {
		t.Fatalf("forwarded = %q, want %q", forwarded, want)
	}
	launcherShellEnv(t, "")
	t.Setenv("PATH", "/usr/bin:/bin")
	for _, entry := range forwarded {
		key, value, _ := strings.Cut(entry, "=")
		t.Setenv(key, value)
	}
	if _, err := PublishUserShellLauncher(target); err != nil {
		t.Fatal(err)
	}
	if got := readProfile(t, filepath.Join(home, ".zshrc")); got != launcherShLine+"\n" {
		t.Fatalf("forwarded profile = %q", got)
	}
	if _, err := os.Lstat(filepath.Join(home, ".profile")); !os.IsNotExist(err) {
		t.Fatalf("unit fell back to ~/.profile: %v", err)
	}
}

// A forwarded profile outside the candidates of the unit's own HOME is never
// edited, not even a ZDOTDIR file: the operator gets the line instead.
func TestPublishUserShellLauncherRejectsForeignForwardedProfile(t *testing.T) {
	target := launcherTestTarget(t)
	launcherShellEnv(t, "")
	home := os.Getenv("HOME")
	zdot := filepath.Join(home, "zdot")
	if err := os.Mkdir(zdot, 0700); err != nil {
		t.Fatal(err)
	}
	for index, intent := range []string{filepath.Join(zdot, ".zshrc"), home + "/x/../.bashrc", "/etc/profile", ".bashrc"} {
		t.Run(strconv.Itoa(index), func(t *testing.T) {
			t.Setenv("GENTLE_AI_LAUNCHER_PROFILE", intent)
			report, err := PublishUserShellLauncher(target)
			if err != nil {
				t.Fatal(err)
			}
			for _, path := range []string{filepath.Join(zdot, ".zshrc"), filepath.Join(home, ".bashrc"), filepath.Join(home, ".profile")} {
				if _, err := os.Lstat(path); !os.IsNotExist(err) {
					t.Fatalf("%s written for a rejected profile: %v", path, err)
				}
			}
			note := userShellLauncherNote(report)
			for _, want := range []string{
				"Future terminals: add this line to your shell startup file, then open a new terminal: " + launcherShLine,
				"No startup file was edited: the requested file is not one this installer edits",
			} {
				if !strings.Contains(note, want) {
					t.Fatalf("note misses %q:\n%s", want, note)
				}
			}
		})
	}
}

func TestUserLauncherProfileIntentIsOneOfTheHomeCandidates(t *testing.T) {
	home := t.TempDir()
	launcherShellEnv(t, "")
	for _, test := range []struct {
		intent string
		ok     bool
	}{
		{filepath.Join(home, ".zshrc"), true},
		{filepath.Join(home, ".bashrc"), runtime.GOOS == "linux"},
		{filepath.Join(home, ".bash_profile"), runtime.GOOS == "darwin"},
		{filepath.Join(home, ".bash_login"), runtime.GOOS == "darwin"},
		{filepath.Join(home, ".profile"), true},
		{filepath.Join(home, ".config", "fish", "conf.d", "gentle-ai-shell.fish"), true},
		{filepath.Join(home, "zdot", ".zshrc"), false},
		{filepath.Join(home, ".config", "fish", "conf.d", "other.fish"), false},
		{home + "/x/../.bashrc", false},
		{"/etc/passwd", false},
		{".bashrc", false},
	} {
		t.Setenv("GENTLE_AI_LAUNCHER_PROFILE", test.intent)
		got, ok := userLauncherProfile(home)
		if ok != test.ok || ok && got != test.intent {
			t.Fatalf("intent %q: profile = %q %v, want ok %v", test.intent, got, ok, test.ok)
		}
	}
	t.Setenv("GENTLE_AI_LAUNCHER_PROFILE", "")
	if got, ok := userLauncherProfile(home); !ok || got != filepath.Join(home, ".profile") {
		t.Fatalf("without an intent the local choice applies: %q %v", got, ok)
	}
}

func TestRemoveUserShellLauncherRemovesThePathLine(t *testing.T) {
	t.Run("keeps the user's content", func(t *testing.T) {
		target := launcherTestTarget(t)
		launcherShellEnv(t, "/bin/zsh")
		profile := filepath.Join(os.Getenv("HOME"), ".zshrc")
		if err := os.WriteFile(profile, []byte("# before\n"), 0644); err != nil {
			t.Fatal(err)
		}
		if _, err := PublishUserShellLauncher(target); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(profile, []byte(readProfile(t, profile)+"# after\n"), 0644); err != nil {
			t.Fatal(err)
		}
		if err := RemoveUserShellLauncher(target); err != nil {
			t.Fatal(err)
		}
		if got := readProfile(t, profile); got != "# before\n# after\n" {
			t.Fatalf("profile after uninstall = %q", got)
		}
	})
	t.Run("deletes a startup file left exactly empty", func(t *testing.T) {
		target := launcherTestTarget(t)
		launcherShellEnv(t, "/bin/zsh")
		profile := filepath.Join(os.Getenv("HOME"), ".zshrc")
		if _, err := PublishUserShellLauncher(target); err != nil {
			t.Fatal(err)
		}
		if err := RemoveUserShellLauncher(target); err != nil {
			t.Fatal(err)
		}
		if _, err := os.Lstat(profile); !os.IsNotExist(err) {
			t.Fatalf("startup file holding only our line kept after uninstall: %v", err)
		}
	})
	t.Run("keeps a startup file with any other content", func(t *testing.T) {
		target := launcherTestTarget(t)
		launcherShellEnv(t, "/bin/zsh")
		profile := filepath.Join(os.Getenv("HOME"), ".zshrc")
		if _, err := PublishUserShellLauncher(target); err != nil {
			t.Fatal(err)
		}
		// A lone blank line is the user's content, not ours.
		if err := os.WriteFile(profile, []byte("\n"+readProfile(t, profile)), 0644); err != nil {
			t.Fatal(err)
		}
		if err := RemoveUserShellLauncher(target); err != nil {
			t.Fatal(err)
		}
		if got := readProfile(t, profile); got != "\n" {
			t.Fatalf("profile after uninstall = %q", got)
		}
	})
	t.Run("deletes our fish file left empty", func(t *testing.T) {
		target := launcherTestTarget(t)
		launcherShellEnv(t, "/usr/bin/fish")
		profile := filepath.Join(os.Getenv("HOME"), ".config", "fish", "conf.d", "gentle-ai-shell.fish")
		if _, err := PublishUserShellLauncher(target); err != nil {
			t.Fatal(err)
		}
		// The uninstalling process may run another shell: every candidate is checked.
		launcherShellEnv(t, "/bin/bash")
		if err := RemoveUserShellLauncher(target); err != nil {
			t.Fatal(err)
		}
		if _, err := os.Lstat(profile); !os.IsNotExist(err) {
			t.Fatalf("emptied profile kept: %v", err)
		}
	})
	t.Run("keeps the line while another gentle-shell launcher remains", func(t *testing.T) {
		target := launcherTestTarget(t)
		launcherShellEnv(t, "/bin/zsh")
		profile := filepath.Join(os.Getenv("HOME"), ".zshrc")
		if _, err := PublishUserShellLauncher(target); err != nil {
			t.Fatal(err)
		}
		if err := RemoveUserShellLauncher(filepath.Join(os.Getenv("HOME"), "other")); err != nil {
			t.Fatal(err)
		}
		if got := readProfile(t, profile); got != launcherShLine+"\n" {
			t.Fatalf("line removed while gentle-shell still links elsewhere: %q", got)
		}
	})
	t.Run("never edits a linked profile", func(t *testing.T) {
		target := launcherTestTarget(t)
		launcherShellEnv(t, "/bin/zsh")
		home := os.Getenv("HOME")
		real := filepath.Join(home, "dotfiles-zshrc")
		if err := os.WriteFile(real, []byte(launcherShLine+"\n"), 0644); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(real, filepath.Join(home, ".zshrc")); err != nil {
			t.Fatal(err)
		}
		if _, err := PublishUserShellLauncher(target); err != nil {
			t.Fatal(err)
		}
		if err := RemoveUserShellLauncher(target); err != nil {
			t.Fatal(err)
		}
		if got := readProfile(t, real); got != launcherShLine+"\n" {
			t.Fatalf("linked profile edited on uninstall: %q", got)
		}
	})
}

// With every name occupied no launcher is served: nothing is written and no
// PATH guidance is shown.
func TestPublishUserShellLauncherAllOccupiedShowsNoPathGuidance(t *testing.T) {
	target := launcherTestTarget(t)
	launcherShellEnv(t, "/bin/zsh")
	for _, name := range userShellLauncherNames {
		if err := os.MkdirAll(filepath.Dir(launcherLink(t, name)), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(launcherLink(t, name), []byte("#!/bin/sh\n"), 0755); err != nil {
			t.Fatal(err)
		}
	}
	report, err := PublishUserShellLauncher(target)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(filepath.Join(os.Getenv("HOME"), ".zshrc")); !os.IsNotExist(err) {
		t.Fatalf("profile written with every name occupied: %v", err)
	}
	note := userShellLauncherNote(report)
	if strings.Contains(note, "PATH") || strings.Contains(note, "startup file") {
		t.Fatalf("PATH guidance shown with every name occupied:\n%s", note)
	}
	if strings.Count(note, "is occupied by") != 2 {
		t.Fatalf("occupied lines missing:\n%s", note)
	}
}

// macOS bash reads exactly one login file; creating ~/.bash_profile would hide
// an existing ~/.profile, so the existing one is chosen.
func TestPublishUserShellLauncherBashFileMatchesThePlatform(t *testing.T) {
	target := launcherTestTarget(t)
	launcherShellEnv(t, "/bin/bash")
	home := os.Getenv("HOME")
	if err := os.WriteFile(filepath.Join(home, ".profile"), nil, 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := PublishUserShellLauncher(target); err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(home, ".bashrc")
	if runtime.GOOS == "darwin" {
		want = filepath.Join(home, ".profile")
	}
	if got := readProfile(t, want); got != launcherShLine+"\n" {
		t.Fatalf("%s = %q", want, got)
	}
}
