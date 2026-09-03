package upgrade

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// setMiseEnv is a test helper that sets an env var and registers cleanup to
// restore its original value (or absence).
func setMiseEnv(t *testing.T, key, value string) {
	t.Helper()
	orig, existed := os.LookupEnv(key)
	if err := os.Setenv(key, value); err != nil {
		t.Fatalf("os.Setenv(%q, %q): %v", key, value, err)
	}
	t.Cleanup(func() {
		if existed {
			os.Setenv(key, orig)
		} else {
			os.Unsetenv(key)
		}
	})
}

// unsetMiseEnv is a test helper that unsets an env var and registers cleanup
// to restore its original value (or absence).
func unsetMiseEnv(t *testing.T, key string) {
	t.Helper()
	orig, existed := os.LookupEnv(key)
	os.Unsetenv(key)
	t.Cleanup(func() {
		if existed {
			os.Setenv(key, orig)
		} else {
			os.Unsetenv(key)
		}
	})
}

// clearMisePrecedenceEnv unsets all precedence inputs so each test case
// starts from a known-empty environment, regardless of what the host machine
// (or a prior subtest) happens to carry.
func clearMisePrecedenceEnv(t *testing.T) {
	t.Helper()
	unsetMiseEnv(t, "MISE_INSTALLS_DIR")
	unsetMiseEnv(t, "MISE_DATA_DIR")
	unsetMiseEnv(t, "XDG_DATA_HOME")
	unsetMiseEnv(t, "LOCALAPPDATA")
}

// swapMiseUserHomeDirFn swaps userHomeDirFn for the duration of the test and
// restores the original on cleanup.
func swapMiseUserHomeDirFn(t *testing.T, home string, err error) {
	t.Helper()
	original := userHomeDirFn
	t.Cleanup(func() { userHomeDirFn = original })
	userHomeDirFn = func() (string, error) { return home, err }
}

// swapMiseCurrentExecutableFn swaps currentExecutableFn for the duration of
// the test and restores the original on cleanup.
func swapMiseCurrentExecutableFn(t *testing.T, exe string, err error) {
	t.Helper()
	original := currentExecutableFn
	t.Cleanup(func() { currentExecutableFn = original })
	currentExecutableFn = func() (string, error) { return exe, err }
}

// mustMkdirAll is a test helper that creates a directory tree and fails the
// test on error.
func mustMkdirAll(t *testing.T, dir string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("os.MkdirAll(%q): %v", dir, err)
	}
}

// mustWriteExecutable is a test helper that writes a fake executable file at
// path (creating its parent directories) and fails the test on error.
func mustWriteExecutable(t *testing.T, path string) {
	t.Helper()
	mustMkdirAll(t, filepath.Dir(path))
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatalf("os.WriteFile(%q): %v", path, err)
	}
}

func TestMiseInstallsRoot(t *testing.T) {
	for _, tt := range []struct {
		name            string
		goos            string
		miseInstallsDir string
		setInstallsDir  bool
		miseDataDir     string
		setDataDir      bool
		xdgDataHome     string
		setXDGDataHome  bool
		localAppData    string
		setLocalAppData bool
		homeDir         string
		homeErr         error
		want            string
	}{
		{
			name:            "MISE_INSTALLS_DIR wins over every other rung",
			goos:            "linux",
			miseInstallsDir: "/custom/installs",
			setInstallsDir:  true,
			miseDataDir:     "/mise/data",
			setDataDir:      true,
			xdgDataHome:     "/xdg/data",
			setXDGDataHome:  true,
			homeDir:         "/home/user",
			want:            "/custom/installs",
		},
		{
			name:           "MISE_DATA_DIR/installs wins when MISE_INSTALLS_DIR is unset",
			goos:           "linux",
			miseDataDir:    "/mise/data",
			setDataDir:     true,
			xdgDataHome:    "/xdg/data",
			setXDGDataHome: true,
			homeDir:        "/home/user",
			want:           filepath.Join("/mise/data", "installs"),
		},
		{
			name:           "XDG_DATA_HOME/mise/installs wins when higher rungs are unset",
			goos:           "linux",
			xdgDataHome:    "/xdg/data",
			setXDGDataHome: true,
			homeDir:        "/home/user",
			want:           filepath.Join("/xdg/data", "mise", "installs"),
		},
		{
			name:    "falls through to ~/.local/share/mise/installs when nothing is set",
			goos:    "linux",
			homeDir: "/home/user",
			want:    filepath.Join("/home/user", ".local", "share", "mise", "installs"),
		},
		{
			name:    "missing HOME resolves an empty root",
			goos:    "linux",
			homeErr: errors.New("$HOME is not defined"),
			want:    "",
		},
		// Threat-matrix case (a): an empty MISE_INSTALLS_DIR must NOT resolve a
		// cwd-relative root — it must be treated exactly as unset and fall
		// through to the next rung.
		{
			name:            "empty MISE_INSTALLS_DIR must not resolve a cwd-relative root",
			goos:            "linux",
			miseInstallsDir: "",
			setInstallsDir:  true,
			homeDir:         "/home/user",
			want:            filepath.Join("/home/user", ".local", "share", "mise", "installs"),
		},
		// Threat-matrix case (a), whitespace variant.
		{
			name:            "whitespace-only MISE_INSTALLS_DIR must not resolve a cwd-relative root",
			goos:            "linux",
			miseInstallsDir: "   ",
			setInstallsDir:  true,
			homeDir:         "/home/user",
			want:            filepath.Join("/home/user", ".local", "share", "mise", "installs"),
		},
		// Windows: XDG_DATA_HOME still wins over LOCALAPPDATA when set — this
		// package doesn't special-case it, mirroring mise's own precedence.
		{
			name:            "windows: XDG_DATA_HOME still wins over LOCALAPPDATA",
			goos:            "windows",
			xdgDataHome:     `C:\xdg\data`,
			setXDGDataHome:  true,
			localAppData:    `C:\Users\user\AppData\Local`,
			setLocalAppData: true,
			homeDir:         `C:\Users\user`,
			want:            filepath.Join(`C:\xdg\data`, "mise", "installs"),
		},
		// Windows: with every env-var rung unset, LOCALAPPDATA is the next
		// resolution step — never the Unix ~/.local/share default.
		{
			name:            "windows: LOCALAPPDATA wins when no env-var rung above it is set",
			goos:            "windows",
			localAppData:    `C:\Users\user\AppData\Local`,
			setLocalAppData: true,
			homeDir:         `C:\Users\user`,
			want:            filepath.Join(`C:\Users\user\AppData\Local`, "mise", "installs"),
		},
		// Windows: with nothing set at all, the platform default is
		// %HOME%\AppData\Local\mise\installs, not ~/.local/share/mise/installs.
		{
			name:    "windows: falls through to HOME/AppData/Local/mise/installs when nothing is set",
			goos:    "windows",
			homeDir: `C:\Users\user`,
			want:    filepath.Join(`C:\Users\user`, "AppData", "Local", "mise", "installs"),
		},
		// Windows threat-matrix parity: an empty/whitespace LOCALAPPDATA must
		// not resolve either, same rule as every other rung.
		{
			name:            "windows: whitespace-only LOCALAPPDATA must not resolve a cwd-relative root",
			goos:            "windows",
			localAppData:    "   ",
			setLocalAppData: true,
			homeDir:         `C:\Users\user`,
			want:            filepath.Join(`C:\Users\user`, "AppData", "Local", "mise", "installs"),
		},
		// Non-Windows GOOS must ignore LOCALAPPDATA entirely, even if it
		// happens to be set (e.g. under Wine or a stray env var).
		{
			name:            "non-windows GOOS ignores LOCALAPPDATA even when set",
			goos:            "darwin",
			localAppData:    `C:\Users\user\AppData\Local`,
			setLocalAppData: true,
			homeDir:         "/Users/user",
			want:            filepath.Join("/Users/user", ".local", "share", "mise", "installs"),
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			clearMisePrecedenceEnv(t)
			if tt.setInstallsDir {
				setMiseEnv(t, "MISE_INSTALLS_DIR", tt.miseInstallsDir)
			}
			if tt.setDataDir {
				setMiseEnv(t, "MISE_DATA_DIR", tt.miseDataDir)
			}
			if tt.setXDGDataHome {
				setMiseEnv(t, "XDG_DATA_HOME", tt.xdgDataHome)
			}
			if tt.setLocalAppData {
				setMiseEnv(t, "LOCALAPPDATA", tt.localAppData)
			}
			swapMiseUserHomeDirFn(t, tt.homeDir, tt.homeErr)

			got := miseInstallsRoot(tt.goos)
			if got != tt.want {
				t.Fatalf("miseInstallsRoot(%q) = %q, want %q", tt.goos, got, tt.want)
			}

			// Extra guard for the empty/whitespace cases: the naive bug this
			// threat case exists to catch is `"" + cwd` resolving to a
			// cwd-relative "installs" directory instead of falling through.
			if tt.setInstallsDir && got != "" {
				cwd, err := os.Getwd()
				if err != nil {
					t.Fatalf("os.Getwd(): %v", err)
				}
				wrongCwdRelative := filepath.Join(cwd, "installs")
				if got == wrongCwdRelative {
					t.Fatalf("miseInstallsRoot() = %q resolved a cwd-relative root from an empty/whitespace override", got)
				}
			}
		})
	}
}

func TestRunningBinaryIsMiseManaged(t *testing.T) {
	for _, tt := range []struct {
		name  string
		setup func(t *testing.T) (wantManaged bool)
	}{
		{
			name: "executable under the resolved mise installs root is mise-managed",
			setup: func(t *testing.T) bool {
				root := filepath.Join(t.TempDir(), "installs")
				exe := filepath.Join(root, "go", "1.25.10", "bin", "gentle-ai")
				mustWriteExecutable(t, exe)

				clearMisePrecedenceEnv(t)
				setMiseEnv(t, "MISE_INSTALLS_DIR", root)
				swapMiseCurrentExecutableFn(t, exe, nil)
				return true
			},
		},
		{
			name: "executable outside every resolved mise root is not mise-managed",
			setup: func(t *testing.T) bool {
				base := t.TempDir()
				root := filepath.Join(base, "installs")
				mustMkdirAll(t, root)
				exe := filepath.Join(base, "usr-local-bin", "gentle-ai")
				mustWriteExecutable(t, exe)

				clearMisePrecedenceEnv(t)
				setMiseEnv(t, "MISE_INSTALLS_DIR", root)
				swapMiseCurrentExecutableFn(t, exe, nil)
				return false
			},
		},
		{
			// A path under a sibling directory that merely shares a string
			// prefix with the installs root (".../installsX/") must never be
			// treated as contained — Contains climbs real filesystem
			// ancestors, not string prefixes.
			name: "sibling-prefix path is not treated as contained",
			setup: func(t *testing.T) bool {
				base := t.TempDir()
				root := filepath.Join(base, "installs")
				sibling := filepath.Join(base, "installsX")
				mustMkdirAll(t, root)
				exe := filepath.Join(sibling, "gentle-ai")
				mustWriteExecutable(t, exe)

				clearMisePrecedenceEnv(t)
				setMiseEnv(t, "MISE_INSTALLS_DIR", root)
				swapMiseCurrentExecutableFn(t, exe, nil)
				return false
			},
		},
		{
			// Threat-matrix case (b): a relative-path override must be
			// answered by pathidentity.Contains' own filepath.Abs handling,
			// never a raw string prefix comparison. MISE_INSTALLS_DIR is set
			// to a path that is relative to the test's working directory.
			name: "relative-path override is resolved via Contains, not a string prefix",
			setup: func(t *testing.T) bool {
				base := t.TempDir()
				root := filepath.Join(base, "installs")
				exe := filepath.Join(root, "go", "1.25.10", "bin", "gentle-ai")
				mustWriteExecutable(t, exe)

				t.Chdir(base)
				clearMisePrecedenceEnv(t)
				setMiseEnv(t, "MISE_INSTALLS_DIR", "installs")
				swapMiseCurrentExecutableFn(t, exe, nil)
				return true
			},
		},
		{
			// Threat-matrix case (c): a set-but-nonexistent root must report
			// "not mise-managed" (false), not error the upgrade.
			name: "set-but-nonexistent root reports not mise-managed, not an error",
			setup: func(t *testing.T) bool {
				base := t.TempDir()
				root := filepath.Join(base, "does-not-exist")
				exe := filepath.Join(base, "gentle-ai")
				mustWriteExecutable(t, exe)

				clearMisePrecedenceEnv(t)
				setMiseEnv(t, "MISE_INSTALLS_DIR", root)
				swapMiseCurrentExecutableFn(t, exe, nil)
				return false
			},
		},
		{
			name: "currentExecutableFn error is treated as not mise-managed",
			setup: func(t *testing.T) bool {
				root := t.TempDir()

				clearMisePrecedenceEnv(t)
				setMiseEnv(t, "MISE_INSTALLS_DIR", root)
				swapMiseCurrentExecutableFn(t, "", errors.New("os.Executable: not implemented"))
				return false
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			wantManaged := tt.setup(t)
			got := runningBinaryIsMiseManaged()
			if got != wantManaged {
				t.Fatalf("runningBinaryIsMiseManaged() = %v, want %v", got, wantManaged)
			}
		})
	}
}
