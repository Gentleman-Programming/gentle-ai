package opencode

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
)

func TestPOSIXActivationPersistsManagedProfileBlockAndDeactivationRemovesIt(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX login profiles are not used on Windows")
	}
	t.Setenv("SHELL", "/bin/zsh")
	t.Setenv("ZDOTDIR", "")
	home := t.TempDir()
	profile := filepath.Join(home, ".zprofile")
	userContent := "export USER_SETTING=1\n"
	if err := os.WriteFile(profile, []byte(userContent), 0o600); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(t.TempDir(), "opencode")
	if err := os.WriteFile(target, []byte("real"), 0o755); err != nil {
		t.Fatal(err)
	}
	options := ActivationOptions{
		OS:            "linux",
		Path:          filepath.Dir(target),
		RunVersion:    func(string) (string, error) { return "1.15.11", nil },
		AddToUserPath: func(string) error { return nil },
		ResolveTarget: func(string, string, string) (string, error) { return target, nil },
	}

	first, err := Activate(home, options)
	if err != nil {
		t.Fatal(err)
	}
	want := userContent + expectedProfileBlock(BinDir(home))
	assertProfile(t, profile, want, 0o600)
	if !slices.Contains(first.ChangedPaths(), profile) {
		t.Fatalf("first activation changed paths = %v, want %s", first.ChangedPaths(), profile)
	}

	second, err := Activate(home, options)
	if err != nil {
		t.Fatal(err)
	}
	if len(second.ChangedPaths()) != 0 {
		t.Fatalf("repeated activation changed paths = %v, want none", second.ChangedPaths())
	}
	assertProfile(t, profile, want, 0o600)

	off, err := Deactivate(home, options)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(off.ChangedPaths(), profile) {
		t.Fatalf("deactivation changed paths = %v, want %s", off.ChangedPaths(), profile)
	}
	assertProfile(t, profile, userContent, 0o600)
}

// expectedProfileBlock spells out the managed block bytes independently of the
// production generator so a format drift fails loudly.
func expectedProfileBlock(binDir string) string {
	return "# >>> gentle-ai managed OpenCode launcher >>>\n" +
		"export PATH='" + binDir + "':\"$PATH\"\n" +
		"# <<< gentle-ai managed OpenCode launcher <<<\n"
}

func assertProfile(t *testing.T, path, want string, mode os.FileMode) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%s): %v", path, err)
	}
	if string(data) != want {
		t.Fatalf("profile %s = %q, want %q", path, data, want)
	}
	info, err := os.Lstat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != mode {
		t.Fatalf("profile %s mode = %04o, want %04o", path, info.Mode().Perm(), mode)
	}
}

func profileTestOptions(t *testing.T, shell string) ActivationOptions {
	t.Helper()
	target := filepath.Join(t.TempDir(), "opencode")
	if err := os.WriteFile(target, []byte("real"), 0o755); err != nil {
		t.Fatal(err)
	}
	return ActivationOptions{
		OS:            "linux",
		Shell:         shell,
		Path:          filepath.Dir(target),
		RunVersion:    func(string) (string, error) { return "1.15.11", nil },
		AddToUserPath: func(string) error { return nil },
		ResolveTarget: func(string, string, string) (string, error) { return target, nil },
	}
}

func skipProfileTestOnWindows(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("POSIX login profiles are not used on Windows")
	}
}

// Issue #3452: WSL Ubuntu ships ~/.profile without ~/.bash_profile; a new
// .bash_profile would shadow .profile, so the block goes into .profile.
func TestBashActivationWritesFirstExistingLoginProfile(t *testing.T) {
	skipProfileTestOnWindows(t)
	home := t.TempDir()
	profile := filepath.Join(home, ".profile")
	userContent := "if [ -n \"$BASH_VERSION\" ]; then . \"$HOME/.bashrc\"; fi\n"
	if err := os.WriteFile(profile, []byte(userContent), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Activate(home, profileTestOptions(t, "/usr/bin/bash")); err != nil {
		t.Fatal(err)
	}
	assertProfile(t, profile, userContent+expectedProfileBlock(BinDir(home)), 0o644)
	for _, name := range []string{".bash_profile", ".bash_login", ".zprofile"} {
		if _, err := os.Lstat(filepath.Join(home, name)); !os.IsNotExist(err) {
			t.Fatalf("%s stat error = %v, want absent", name, err)
		}
	}
}

func TestBashActivationPrefersBashProfileAndCreatesProfileWhenNoneExist(t *testing.T) {
	skipProfileTestOnWindows(t)
	t.Run("existing bash_profile", func(t *testing.T) {
		home := t.TempDir()
		for _, name := range []string{".bash_profile", ".profile"} {
			if err := os.WriteFile(filepath.Join(home, name), []byte("# "+name+"\n"), 0o644); err != nil {
				t.Fatal(err)
			}
		}
		if _, err := Activate(home, profileTestOptions(t, "/bin/bash")); err != nil {
			t.Fatal(err)
		}
		assertProfile(t, filepath.Join(home, ".bash_profile"), "# .bash_profile\n"+expectedProfileBlock(BinDir(home)), 0o644)
		assertProfile(t, filepath.Join(home, ".profile"), "# .profile\n", 0o644)
	})
	t.Run("no login profile", func(t *testing.T) {
		home := t.TempDir()
		plan, err := Activate(home, profileTestOptions(t, "/bin/bash"))
		if err != nil {
			t.Fatal(err)
		}
		profile := filepath.Join(home, ".profile")
		assertProfile(t, profile, expectedProfileBlock(BinDir(home)), 0o644)
		if report := plan.Report(); report.Status != ActivationStatusReady || !strings.Contains(report.ActivationReason, profile) {
			t.Fatalf("activation report = %#v, want ready naming profile path", report)
		}
	})
}

// Profiles that cannot be updated safely are never modified; the launcher is
// still written and the activation reason tells the user exactly what to add.
func TestActivationRefusesUnsafeLoginProfilesWithManualGuidance(t *testing.T) {
	skipProfileTestOnWindows(t)
	for _, tt := range []struct {
		name   string
		shell  string
		setup  func(t *testing.T, home string) (path string, content string)
		reason string
		// wantReady marks refused profiles that still put the managed bin
		// directory on PATH: activation is effective even though Gentle AI
		// will not rewrite them.
		wantReady bool
		// wantUnknown marks shells whose startup files cannot be modeled.
		wantUnknown bool
	}{
		{name: "unsupported shell", shell: "/usr/bin/fish", reason: "not supported", wantUnknown: true},
		{name: "unknown shell", shell: "", reason: "SHELL is not set", wantUnknown: true},
		{name: "symlinked profile", shell: "/bin/zsh", reason: "symlink", setup: func(t *testing.T, home string) (string, string) {
			target := filepath.Join(t.TempDir(), "dotfiles-zprofile")
			if err := os.WriteFile(target, []byte("export DOTFILES=1\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(target, filepath.Join(home, ".zprofile")); err != nil {
				t.Fatal(err)
			}
			return target, "export DOTFILES=1\n"
		}},
		{name: "read-only profile", shell: "/bin/zsh", reason: "read-only", setup: func(t *testing.T, home string) (string, string) {
			path := filepath.Join(home, ".zprofile")
			if err := os.WriteFile(path, []byte("export LOCKED=1\n"), 0o444); err != nil {
				t.Fatal(err)
			}
			return path, "export LOCKED=1\n"
		}},
		{name: "edited managed block", shell: "/bin/zsh", reason: "malformed, edited, or multiple", setup: func(t *testing.T, home string) (string, string) {
			path := filepath.Join(home, ".zprofile")
			content := profileStart + "\nexport PATH=\"$HOME/custom:$PATH\"\n" + profileEnd + "\n"
			if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
				t.Fatal(err)
			}
			return path, content
		}},
		{name: "duplicated managed block", shell: "/bin/zsh", wantReady: true, setup: func(t *testing.T, home string) (string, string) {
			path := filepath.Join(home, ".zprofile")
			content := expectedProfileBlock(BinDir(home)) + expectedProfileBlock(BinDir(home))
			if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
				t.Fatal(err)
			}
			return path, content
		}},
		{name: "zsh ZDOTDIR elsewhere", shell: "/bin/zsh", reason: "ZDOTDIR", setup: func(t *testing.T, home string) (string, string) {
			t.Setenv("ZDOTDIR", filepath.Join(home, ".config", "zsh"))
			return "", ""
		}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			home := t.TempDir()
			var path, content string
			if tt.setup != nil {
				path, content = tt.setup(t, home)
			}
			options := profileTestOptions(t, tt.shell)
			if tt.shell == "" {
				t.Setenv("SHELL", "")
			}
			plan, err := Activate(home, options)
			if err != nil {
				t.Fatal(err)
			}
			report := plan.Report()
			if tt.wantReady {
				if report.Status != ActivationStatusReady || !strings.Contains(report.ActivationReason, path) {
					t.Fatalf("activation report = %#v, want ready naming %s", report, path)
				}
			} else if wantStatus := map[bool]ActivationStatus{false: ActivationStatusPending, true: ActivationStatusUnknown}[tt.wantUnknown]; report.Status != wantStatus || !strings.Contains(report.ActivationReason, "PATH persistence is pending") || !strings.Contains(report.ActivationReason, tt.reason) || !strings.Contains(report.ActivationReason, ProfileExportLine(BinDir(home))) {
				t.Fatalf("activation report = %#v, want pending reason %q and manual export line", report, tt.reason)
			}
			if _, err := os.Stat(POSIXLauncherPath(home)); err != nil {
				t.Fatalf("launcher stat error = %v, want written", err)
			}
			if path != "" {
				if data, err := os.ReadFile(path); err != nil || string(data) != content {
					t.Fatalf("profile %s = %q, %v; want untouched %q", path, data, err, content)
				}
			}
			for _, managed := range ManagedProfilePaths(home) {
				if managed == path || filepath.Join(home, ".zprofile") == managed && path != "" {
					continue
				}
				if _, err := os.Lstat(managed); !os.IsNotExist(err) {
					t.Fatalf("%s stat error = %v, want absent", managed, err)
				}
			}
			for _, changed := range plan.ChangedPaths() {
				if changed != POSIXLauncherPath(home) {
					t.Fatalf("changed paths = %v, want only the launcher", plan.ChangedPaths())
				}
			}
		})
	}
}

func TestActivationReplacesStaleBlockAndPreservesCRLF(t *testing.T) {
	skipProfileTestOnWindows(t)
	home := t.TempDir()
	profile := filepath.Join(home, ".zprofile")
	stale := strings.ReplaceAll(expectedProfileBlock("/old/home/.gentle-ai/bin"), "\n", "\r\n")
	if err := os.WriteFile(profile, []byte("export A=1\r\n"+stale+"export B=1\r\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Activate(home, profileTestOptions(t, "/bin/zsh")); err != nil {
		t.Fatal(err)
	}
	fresh := strings.ReplaceAll(expectedProfileBlock(BinDir(home)), "\n", "\r\n")
	assertProfile(t, profile, "export A=1\r\n"+fresh+"export B=1\r\n", 0o600)
	if _, err := Deactivate(home, profileTestOptions(t, "/bin/zsh")); err != nil {
		t.Fatal(err)
	}
	assertProfile(t, profile, "export A=1\r\nexport B=1\r\n", 0o600)
}

func TestActivationRevalidatesProfileBeforeWrite(t *testing.T) {
	skipProfileTestOnWindows(t)
	home := t.TempDir()
	profile := filepath.Join(home, ".zprofile")
	if err := os.WriteFile(profile, []byte("export A=1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	plan, err := PrepareActivation(home, profileTestOptions(t, "/bin/zsh"))
	if err != nil {
		t.Fatal(err)
	}
	concurrent := "export A=1\nexport CONCURRENT=1\n"
	if err := os.WriteFile(profile, []byte(concurrent), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := plan.Apply(); err == nil || !strings.Contains(err.Error(), "revalidate OpenCode login profile") {
		t.Fatalf("Apply() error = %v, want profile revalidation failure", err)
	}
	assertProfile(t, profile, concurrent, 0o644)
	if _, err := os.Stat(POSIXLauncherPath(home)); !os.IsNotExist(err) {
		t.Fatalf("launcher after failed activation = %v, want rolled back", err)
	}
}

func TestActivationRollbackRestoresProfiles(t *testing.T) {
	skipProfileTestOnWindows(t)
	pathErr := errors.New("path update failed")
	t.Run("existing profile restored", func(t *testing.T) {
		home := t.TempDir()
		profile := filepath.Join(home, ".zprofile")
		if err := os.WriteFile(profile, []byte("export A=1"), 0o600); err != nil {
			t.Fatal(err)
		}
		options := profileTestOptions(t, "/bin/zsh")
		options.AddToUserPath = func(string) error { return pathErr }
		if _, err := Activate(home, options); !errors.Is(err, pathErr) {
			t.Fatalf("Activate() error = %v, want %v", err, pathErr)
		}
		assertProfile(t, profile, "export A=1", 0o600)
	})
	t.Run("created profile removed", func(t *testing.T) {
		home := t.TempDir()
		options := profileTestOptions(t, "/bin/zsh")
		options.AddToUserPath = func(string) error { return pathErr }
		plan, err := Activate(home, options)
		if !errors.Is(err, pathErr) {
			t.Fatalf("Activate() error = %v, want %v", err, pathErr)
		}
		if _, err := os.Lstat(filepath.Join(home, ".zprofile")); !os.IsNotExist(err) {
			t.Fatalf("created profile stat error = %v, want removed", err)
		}
		if len(plan.ChangedPaths()) != 0 {
			t.Fatalf("changed paths after rollback = %v, want none", plan.ChangedPaths())
		}
	})
	t.Run("concurrent edit after write preserved", func(t *testing.T) {
		home := t.TempDir()
		profile := filepath.Join(home, ".zprofile")
		if err := os.WriteFile(profile, []byte("export A=1\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		options := profileTestOptions(t, "/bin/zsh")
		concurrent := "export A=1\nexport EDITED_AFTER_WRITE=1\n"
		options.AddToUserPath = func(string) error {
			if err := os.WriteFile(profile, []byte(concurrent), 0o644); err != nil {
				t.Fatal(err)
			}
			return pathErr
		}
		_, err := Activate(home, options)
		if !errors.Is(err, pathErr) || !strings.Contains(err.Error(), "rollback preserve changed OpenCode login profile") {
			t.Fatalf("Activate() error = %v, want path failure joined with preserved profile", err)
		}
		assertProfile(t, profile, concurrent, 0o644)
	})
}

func TestDeactivationRemovesBlocksFromEveryManagedProfileOnly(t *testing.T) {
	skipProfileTestOnWindows(t)
	home := t.TempDir()
	block := expectedProfileBlock(BinDir(home))
	files := map[string]string{
		".zprofile":     "export Z=1\n" + block,
		".profile":      block + "export P=1\n",
		".bash_profile": "export UNRELATED=1\n",
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(home, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	plan, err := Deactivate(home, profileTestOptions(t, "/usr/bin/fish"))
	if err != nil {
		t.Fatal(err)
	}
	assertProfile(t, filepath.Join(home, ".zprofile"), "export Z=1\n", 0o644)
	assertProfile(t, filepath.Join(home, ".profile"), "export P=1\n", 0o644)
	assertProfile(t, filepath.Join(home, ".bash_profile"), "export UNRELATED=1\n", 0o644)
	if len(plan.ChangedPaths()) != 2 {
		t.Fatalf("changed paths = %v, want the two profiles carrying the block", plan.ChangedPaths())
	}
}

func TestRemoveManagedProfileBlockLeavesUnsafeProfilesUntouched(t *testing.T) {
	skipProfileTestOnWindows(t)
	home := t.TempDir()
	block := expectedProfileBlock(BinDir(home))
	edited := filepath.Join(home, ".zprofile")
	editedContent := strings.Replace(block, "export PATH=", "export PATH=/user:", 1)
	if err := os.WriteFile(edited, []byte(editedContent), 0o644); err != nil {
		t.Fatal(err)
	}
	readOnly := filepath.Join(home, ".profile")
	if err := os.WriteFile(readOnly, []byte(block), 0o444); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{edited, readOnly, filepath.Join(home, ".bash_profile")} {
		if HasManagedProfileBlock(path) {
			t.Fatalf("HasManagedProfileBlock(%s) = true, want false", path)
		}
		if changed, err := RemoveManagedProfileBlock(path); err != nil || changed {
			t.Fatalf("RemoveManagedProfileBlock(%s) = %t, %v; want untouched", path, changed, err)
		}
	}
	assertProfile(t, edited, editedContent, 0o644)
	assertProfile(t, readOnly, block, 0o444)
}

func TestManagedProfileBlockRoundTripsQuotedBinDir(t *testing.T) {
	binDir := "/home/o'brien/.gentle-ai/bin"
	data := []byte("export A=1\n" + profileBlock(binDir, "\n"))
	block, err := parseManagedProfileBlock(data)
	if err != nil || block == nil || block.binDir != binDir {
		t.Fatalf("parseManagedProfileBlock() = %#v, %v; want binDir %q", block, err, binDir)
	}
	removed, err := rewriteProfileBlock(data, "", true)
	if err != nil || string(removed) != "export A=1\n" {
		t.Fatalf("rewriteProfileBlock(remove) = %q, %v", removed, err)
	}
}

// One pasted CRLF line in an LF profile must not make the managed block CRLF:
// a trailing CR on the export line corrupts the last PATH entry in bash.
func TestProfileBlockFollowsTheLastLineEnding(t *testing.T) {
	binDir := "/home/u/.gentle-ai/bin"
	mixed := []byte("export A=1\r\nexport B=2\n")
	got, err := rewriteProfileBlock(mixed, binDir, false)
	if err != nil {
		t.Fatal(err)
	}
	if want := string(mixed) + profileBlock(binDir, "\n"); string(got) != want {
		t.Fatalf("mixed-EOL profile = %q, want LF block %q", got, want)
	}
	crlf := []byte("export A=1\r\n")
	got, err = rewriteProfileBlock(crlf, binDir, false)
	if err != nil {
		t.Fatal(err)
	}
	if want := string(crlf) + profileBlock(binDir, "\r\n"); string(got) != want {
		t.Fatalf("CRLF profile = %q, want CRLF block %q", got, want)
	}
}

// Only the startup files the current shell reads count: a block left in
// another shell's profile does not put the launcher on a new shell's PATH.
func TestResolveLoginShellActivationReadsTheCurrentShellsProfile(t *testing.T) {
	skipProfileTestOnWindows(t)
	home := t.TempDir()
	binDir := BinDir(home)
	writeExecutable(t, POSIXLauncherPath(home), posixLauncher("/opt/opencode/bin/opencode"))
	profile := filepath.Join(home, ".profile")
	if err := os.WriteFile(profile, []byte(profileBlock(binDir, "\n")), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("ZDOTDIR", "")
	t.Setenv("SHELL", "/bin/zsh")
	options := ActivationOptions{OS: "linux", Path: "/usr/bin"}
	if got := ResolveLoginShellActivation(home, options); got.Status != ActivationStatusPending {
		t.Fatalf("zsh: block only in .profile = %#v, want pending", got)
	}
	t.Setenv("SHELL", "/bin/sh")
	if got := ResolveLoginShellActivation(home, options); got.Status != ActivationStatusReady || got.Source != profile {
		t.Fatalf("sh: ResolveLoginShellActivation = %#v, want ready from %s", got, profile)
	}
}
