package opencode

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/components/filemerge"
	"github.com/gentleman-programming/gentle-ai/v2/internal/system"
)

func TestResolveCapabilityVersionTable(t *testing.T) {
	tests := []struct {
		name   string
		output string
		status CapabilityStatus
		ready  bool
	}{
		{name: "baseline", output: "1.15.11\n", status: CapabilityReady, ready: true},
		{name: "newer", output: "opencode 1.18.18", status: CapabilityReady, ready: true},
		{name: "older patch", output: "1.15.10", status: CapabilityUnsupported},
		{name: "older minor", output: "1.14.99", status: CapabilityUnsupported},
		{name: "pre release baseline", output: "1.15.11-beta.1", status: CapabilityUnsupported},
		{name: "build metadata baseline", output: "opencode v1.15.11+build.1\n", status: CapabilityReady, ready: true},
		{name: "valid adjacent punctuation", output: "opencode 1.15.11, installed", status: CapabilityReady, ready: true},
		{name: "malformed prerelease empty identifier", output: "1.15.11-alpha..1", status: CapabilityUnknown},
		{name: "malformed prerelease numeric leading zero", output: "1.15.11-01", status: CapabilityUnknown},
		{name: "malformed build metadata empty identifier", output: "1.15.11+build..1", status: CapabilityUnknown},
		{name: "malformed build metadata invalid identifier character", output: "1.15.11+build_1", status: CapabilityUnknown},
		{name: "malformed prerelease path continuation", output: "opencode 1.15.11-rc.1/evil", status: CapabilityUnknown},
		{name: "malformed build path continuation", output: "opencode 1.15.11+build.1/evil", status: CapabilityUnknown},
		{name: "malformed identifier continuation", output: "opencode 1.15.11-rc.1@bad", status: CapabilityUnknown},
		{name: "valid closing parenthesis", output: "opencode (1.15.11)", status: CapabilityReady, ready: true},
		{name: "unknown output", output: "development build", status: CapabilityUnknown},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ResolveCapability("/real/opencode", func(string) (string, error) {
				return tt.output, nil
			})
			if got.Status != tt.status || got.Ready() != tt.ready {
				t.Fatalf("resolution = %#v, want status=%q ready=%t", got, tt.status, tt.ready)
			}
		})
	}

	unknown := ResolveCapability("/real/opencode", func(string) (string, error) {
		return "", errors.New("not runnable")
	})
	if unknown.Status != CapabilityUnknown || unknown.Ready() {
		t.Fatalf("command failure resolution = %#v, want unknown foreground", unknown)
	}
}

func TestVersionPreReleasePrecedenceIgnoresBuildMetadata(t *testing.T) {
	parse := func(t *testing.T, raw string) Version {
		t.Helper()
		version, err := ParseVersion(raw)
		if err != nil {
			t.Fatalf("ParseVersion(%q): %v", raw, err)
		}
		return version
	}

	if !parse(t, "1.15.11-rc.1").AtLeast(parse(t, "1.15.11-beta.2")) {
		t.Fatal("release candidate ranked below beta")
	}
	if parse(t, "1.15.11-1").AtLeast(parse(t, "1.15.11-alpha")) {
		t.Fatal("numeric prerelease ranked above non-numeric prerelease")
	}
	if !parse(t, "1.15.11+build.1").AtLeast(MinimumBackgroundVersion) || !MinimumBackgroundVersion.AtLeast(parse(t, "1.15.11+build.1")) {
		t.Fatal("build metadata changed stable version precedence")
	}
	if !parse(t, "1.15.11-rc.1+build.1").AtLeast(parse(t, "1.15.11-beta.2+build.2")) {
		t.Fatal("build metadata changed prerelease precedence")
	}
}

// resolveTargetCandidateName is the host-native candidate filename: Windows
// has no execute bit, so the fixture must be PATHEXT-shaped there (#3209).
func resolveTargetCandidateName() string {
	if runtime.GOOS == "windows" {
		return "opencode.cmd"
	}
	return "opencode"
}

func TestResolveTargetSkipsManagedBinAndPreventsRecursion(t *testing.T) {
	home := t.TempDir()
	managed := BinDir(home)
	real := filepath.Join(t.TempDir(), resolveTargetCandidateName())
	if err := os.MkdirAll(managed, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(managed, resolveTargetCandidateName()), []byte("managed"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(real, []byte("real"), 0o755); err != nil {
		t.Fatal(err)
	}

	got, err := ResolveTarget(home, runtime.GOOS, managed+string(os.PathListSeparator)+filepath.Dir(real))
	if err != nil {
		t.Fatal(err)
	}
	if got != normalizedCandidatePath(t, real) {
		t.Fatalf("ResolveTarget() = %q, want %q", got, real)
	}
}

// normalizedCandidatePath resolves the fixture path exactly the way
// ResolveTarget resolves candidates (EvalSymlinks + Abs): on Windows
// t.TempDir() hands out 8.3 short names that production expands (#3209),
// and on macOS /tmp itself is a symlink.
func normalizedCandidatePath(t *testing.T, path string) string {
	t.Helper()
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		t.Fatal(err)
	}
	absolute, err := filepath.Abs(resolved)
	if err != nil {
		t.Fatal(err)
	}
	return absolute
}

func TestResolveTargetRejectsEmptyAndRelativeEntries(t *testing.T) {
	home := t.TempDir()
	if _, err := ResolveTarget(home, "linux", ""); err == nil {
		t.Fatal("ResolveTarget(empty PATH) error = nil, want unsafe PATH entry rejected")
	}
	workingDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(workingDir, "opencode"), []byte("cwd"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Chdir(workingDir)
	if _, err := ResolveTarget(home, "linux", "."); err == nil {
		t.Fatal("ResolveTarget(relative PATH) error = nil, want cwd executable rejected")
	}
}

func TestResolveTargetContinuesAfterBrokenCandidate(t *testing.T) {
	home := t.TempDir()
	brokenDir := t.TempDir()
	validDir := t.TempDir()
	// A directory with the candidate name is a broken candidate on every OS:
	// stat succeeds and IsRegular fails, so resolution must continue. A broken
	// symlink would prove the same only where symlinks are creatable (#3209).
	if err := os.MkdirAll(filepath.Join(brokenDir, resolveTargetCandidateName()), 0o755); err != nil {
		t.Fatal(err)
	}
	valid := filepath.Join(validDir, resolveTargetCandidateName())
	if err := os.WriteFile(valid, []byte("valid"), 0o755); err != nil {
		t.Fatal(err)
	}

	got, err := ResolveTarget(home, runtime.GOOS, strings.Join([]string{brokenDir, validDir}, string(os.PathListSeparator)))
	if err != nil {
		t.Fatal(err)
	}
	if got != normalizedCandidatePath(t, valid) {
		t.Fatalf("ResolveTarget() = %q, want %q", got, valid)
	}
}

func TestSplitPathUsesTargetOperatingSystem(t *testing.T) {
	if got := splitPath(`C:\\OpenCode;D:\\Tools`, "windows"); len(got) != 2 {
		t.Fatalf("Windows splitPath() = %#v, want two entries", got)
	}
	if got := splitPath(`/opt/opencode:/usr/local/bin`, "linux"); len(got) != 2 {
		t.Fatalf("POSIX splitPath() = %#v, want two entries", got)
	}
}

func TestLauncherContentsPreserveExplicitFalse(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX launcher execution is not supported on Windows")
	}
	home := t.TempDir()
	target := filepath.Join(t.TempDir(), "real-opencode")
	if err := os.WriteFile(target, []byte("#!/bin/sh\nprintf '%s|%s' \"${OPENCODE_EXPERIMENTAL_BACKGROUND_SUBAGENTS-unset}\" \"$1\"\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	plan, err := PrepareActivation(home, ActivationOptions{
		OS:    "linux",
		Path:  filepath.Dir(target),
		Shell: "/bin/zsh",
		RunVersion: func(string) (string, error) {
			return "1.15.11", nil
		},
		AddToUserPath: func(string) error { return nil },
		ResolveTarget: func(string, string, string) (string, error) { return target, nil },
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := plan.Apply(); err != nil {
		t.Fatal(err)
	}

	launcher := POSIXLauncherPath(home)
	run := func(value string) string {
		cmd := exec.Command(launcher, "arg")
		env := []string{}
		for _, entry := range os.Environ() {
			if !strings.HasPrefix(entry, BackgroundSubagentsEnv+"=") {
				env = append(env, entry)
			}
		}
		if value != "" {
			env = append(env, BackgroundSubagentsEnv+"="+value)
		}
		cmd.Env = env
		output, err := cmd.Output()
		if err != nil {
			t.Fatalf("launcher output: %v", err)
		}
		return string(output)
	}
	if got := run(""); got != "true|arg" {
		t.Fatalf("unset environment output = %q, want true|arg", got)
	}
	if got := run("false"); got != "false|arg" {
		t.Fatalf("explicit false output = %q, want false|arg", got)
	}
}

func TestWindowsLauncherContents(t *testing.T) {
	contents := launcherContent("windows", `C:\Program Files\OpenCode\opencode.exe`)
	for _, name := range []string{WindowsCMDPathPlaceholder, WindowsPS1PathPlaceholder} {
		content := contents[name]
		if !strings.Contains(content, OwnershipMarker) || !strings.Contains(content, BackgroundSubagentsEnv) {
			t.Fatalf("%s launcher = %q, missing ownership/env contract", name, content)
		}
		if !strings.Contains(content, "opencode.exe") {
			t.Fatalf("%s launcher = %q, missing real target", name, content)
		}
	}
	if strings.Contains(contents[WindowsCMDPathPlaceholder], "opencode.cmd") || strings.Contains(contents[WindowsPS1PathPlaceholder], "opencode.ps1") {
		t.Fatal("Windows launchers must execute the resolved real target, not themselves")
	}
}

func TestPathResolvesToHonorsPATHOrderAndLauncherMode(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX PATH executable semantics are not portable on Windows")
	}
	home := t.TempDir()
	managedDir := BinDir(home)
	launcher := POSIXLauncherPath(home)
	if err := os.MkdirAll(managedDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(launcher, []byte(posixLauncher("/real/opencode")), 0o755); err != nil {
		t.Fatal(err)
	}

	earlier := t.TempDir()
	if err := os.WriteFile(filepath.Join(earlier, "opencode"), []byte("user executable"), 0o755); err != nil {
		t.Fatal(err)
	}
	pathValue := earlier + string(os.PathListSeparator) + managedDir
	if pathResolvesTo(pathValue, launcher, "linux") {
		t.Fatal("PATH resolution reported the managed launcher effective despite an earlier executable")
	}
	// PATH entries are literal data after the shell exports them. exec.LookPath
	// does not trim whitespace or remove quotes, so a malformed entry must not
	// be normalized into the managed directory and reported as effective.
	if pathResolvesTo("  "+`"`+managedDir+`"`+"  ", launcher, "linux") {
		t.Fatal("PATH resolution accepted a malformed quoted, padded managed directory")
	}
	if err := os.Chmod(launcher, 0o644); err != nil {
		t.Fatal(err)
	}
	if pathResolvesTo(managedDir, launcher, "linux") {
		t.Fatal("PATH resolution reported a non-executable managed launcher as effective")
	}
}

func TestResolveManagedLauncherWindowsRejectsCurrentDirectoryDuplicate(t *testing.T) {
	workDir := t.TempDir()
	t.Setenv("PATHEXT", ".cmd")
	t.Setenv("NoDefaultCurrentDirectoryInExePath", "")
	if err := os.Unsetenv("NoDefaultCurrentDirectoryInExePath"); err != nil {
		t.Fatal(err)
	}
	t.Chdir(workDir)

	launcher := filepath.Join(workDir, "opencode.cmd")
	if err := os.WriteFile(launcher, []byte(windowsCMDLauncher(`C:\OpenCode\opencode.exe`)), 0o600); err != nil {
		t.Fatal(err)
	}

	got, err := ResolveManagedLauncher(workDir, launcher, "windows")
	if got != "" {
		t.Fatalf("resolved launcher = %q, want empty on ErrDot", got)
	}
	if !errors.Is(err, exec.ErrDot) {
		t.Fatalf("ResolveManagedLauncher() error = %v, want errors.Is(exec.ErrDot)", err)
	}
}

func TestIsManagedLauncherRejectsIncidentalAndMalformedMarkers(t *testing.T) {
	for _, tt := range []struct {
		name, path, content string
		want                bool
	}{
		{"posix old target", "opencode", posixLauncher("/old/opencode"), true},
		{"cmd old target", "opencode.cmd", windowsCMDLauncher(`C:\old\opencode.exe`), true},
		{"powershell old target", "opencode.ps1", windowsPS1Launcher(`C:\old\opencode.exe`), true},
		{"incidental marker", "opencode", "#!/bin/sh\n# user mentions " + OwnershipMarker + "\necho user\n", false},
		{"truncated generated header", "opencode", "#!/bin/sh\n# " + OwnershipMarker + "\nset -eu\n", false},
		{"wrong launcher path", "custom-opencode", posixLauncher("/old/opencode"), false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsManagedLauncher(tt.path, []byte(tt.content)); got != tt.want {
				t.Fatalf("IsManagedLauncher() = %t, want %t", got, tt.want)
			}
		})
	}
}

func TestIsManagedLauncherRequiresCanonicalGeneratedBytes(t *testing.T) {
	for _, tt := range []struct {
		name, path, content string
	}{
		{"posix shell characters", "opencode", posixLauncher("/old path/a'b;$HOME")},
		{"cmd spaces and quotes", "opencode.cmd", windowsCMDLauncher(`C:\old path\a"b.exe`)},
		{"powershell apostrophe", "opencode.ps1", windowsPS1Launcher(`C:\old path\a'b.exe`)},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if !IsManagedLauncher(tt.path, []byte(tt.content)) {
				t.Fatal("generated historical launcher was rejected")
			}
			for _, forged := range []string{
				"echo injected\n" + tt.content,
				tt.content + "echo injected\n",
				strings.Replace(tt.content, "\n", "\necho injected\n", 1),
				tt.content[:len(tt.content)-1],
			} {
				if IsManagedLauncher(tt.path, []byte(forged)) {
					t.Fatalf("forged launcher accepted: %q", forged)
				}
			}
		})
	}
}

func TestInspectLauncherRejectsSymlinksAndNonRegularPaths(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation requires privileges not guaranteed on Windows")
	}
	home := t.TempDir()
	launcher := POSIXLauncherPath(home)
	target := filepath.Join(t.TempDir(), "target")
	if err := os.WriteFile(target, []byte(posixLauncher("/old/opencode")), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(launcher), 0o755); err != nil || os.Symlink(target, launcher) != nil {
		t.Fatal("create managed launcher symlink")
	}
	if _, _, owned, err := inspectLauncher(launcher); err == nil || owned {
		t.Fatalf("symlink ownership = %t, %v; want rejected", owned, err)
	}
	if err := os.Remove(launcher); err != nil || os.Mkdir(launcher, 0o755) != nil {
		t.Fatal("create launcher directory")
	}
	if _, _, owned, err := inspectLauncher(launcher); err == nil || owned {
		t.Fatalf("directory ownership = %t, %v; want rejected", owned, err)
	}
}

func TestRemoveManagedLauncherRefusesReplacementAtRemoval(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("replacement fixture relies on POSIX rename semantics")
	}
	home := t.TempDir()
	path := POSIXLauncherPath(home)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(posixLauncher("/old/opencode")), 0o755); err != nil {
		t.Fatal(err)
	}
	original := managedLauncherRemovalBeforeDelete
	t.Cleanup(func() { managedLauncherRemovalBeforeDelete = original })
	managedLauncherRemovalBeforeDelete = func(candidate string) {
		if candidate != path {
			return
		}
		managedPath := path + ".managed"
		replacement := path + ".replacement"
		if err := os.WriteFile(replacement, []byte("user replacement"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.Rename(path, managedPath); err != nil {
			t.Fatal(err)
		}
		if err := os.Rename(replacement, path); err != nil {
			t.Fatal(err)
		}
	}

	result, err := RemoveManagedLauncher(path)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := result.Status, ManagedLauncherRemovalRefused; got != want {
		t.Fatalf("removal status = %q, want %q", got, want)
	}
	if got, err := os.ReadFile(path); err != nil || string(got) != "user replacement" {
		t.Fatalf("replacement after refused removal = %q, %v", got, err)
	}
	if got, err := os.ReadFile(path + ".managed"); err != nil || !IsManagedLauncher(path, got) {
		t.Fatalf("original managed launcher after refused removal = %q, %v", got, err)
	}
}

func TestRemoveManagedLauncherPreservesReplacementAfterFinalValidation(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("replacement fixture relies on POSIX rename semantics")
	}
	for _, tt := range []struct {
		name    string
		symlink bool
	}{
		{name: "user file", symlink: false},
		{name: "user symlink", symlink: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			home := t.TempDir()
			path := POSIXLauncherPath(home)
			target := filepath.Join(t.TempDir(), "target")
			managedBytes := []byte(posixLauncher("/old/opencode"))
			replacementBytes := []byte("user replacement")
			if err := os.WriteFile(target, []byte("target"), 0o600); err != nil {
				t.Fatal(err)
			}
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, managedBytes, 0o755); err != nil {
				t.Fatal(err)
			}

			managedPath := path + ".managed"
			replacementPath := path + ".replacement"
			original := managedLauncherRemovalBeforeUnlink
			t.Cleanup(func() { managedLauncherRemovalBeforeUnlink = original })
			managedLauncherRemovalBeforeUnlink = func(candidate string) {
				if candidate != path {
					return
				}
				if _, err := os.Lstat(path); err == nil {
					if err := os.Rename(path, managedPath); err != nil {
						t.Errorf("move validated launcher aside: %v", err)
						return
					}
				} else if !os.IsNotExist(err) {
					t.Errorf("inspect validated launcher path: %v", err)
					return
				}
				if tt.symlink {
					if err := os.Symlink(target, replacementPath); err != nil {
						t.Errorf("create replacement symlink: %v", err)
						return
					}
				} else if err := os.WriteFile(replacementPath, replacementBytes, 0o600); err != nil {
					t.Errorf("create replacement file: %v", err)
					return
				}
				if err := os.Rename(replacementPath, path); err != nil {
					t.Errorf("install replacement: %v", err)
				}
			}

			result, err := RemoveManagedLauncher(path)
			if err != nil {
				t.Fatal(err)
			}
			if got, want := result.Status, ManagedLauncherRemovalRemoved; got != want {
				t.Fatalf("removal status = %q, want %q", got, want)
			}
			info, err := os.Lstat(path)
			if err != nil {
				t.Fatal(err)
			}
			if tt.symlink {
				if info.Mode()&os.ModeSymlink == 0 {
					t.Fatalf("replacement mode = %v, want symlink", info.Mode())
				}
				if got, err := os.Readlink(path); err != nil || got != target {
					t.Fatalf("replacement symlink = %q, %v; want %q", got, err, target)
				}
				return
			}
			if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
				t.Fatalf("replacement mode = %v, want regular file", info.Mode())
			}
			if got, err := os.ReadFile(path); err != nil || string(got) != string(replacementBytes) {
				t.Fatalf("replacement bytes = %q, %v; want %q", got, err, replacementBytes)
			}
			if got, want := info.Mode().Perm(), os.FileMode(0o600); got != want {
				t.Fatalf("replacement mode = %o, want %o", got, want)
			}
		})
	}
}

func TestRemoveManagedLauncherRemovesOwnedLauncher(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX launcher removal is covered by the Windows handle test on Windows")
	}
	home := t.TempDir()
	path := POSIXLauncherPath(home)
	managedBytes := []byte(posixLauncher("/old/opencode"))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, managedBytes, 0o755); err != nil {
		t.Fatal(err)
	}

	result, err := RemoveManagedLauncher(path)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := result.Status, ManagedLauncherRemovalRemoved; got != want {
		t.Fatalf("removal status = %q, want %q", got, want)
	}
	if _, err := os.Lstat(path); !os.IsNotExist(err) {
		t.Fatalf("owned launcher after removal = %v, want absent", err)
	}
}

func TestRemoveManagedLauncherRefusesSymlinkReplacementBeforeCapture(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("replacement fixture relies on POSIX rename semantics")
	}
	home := t.TempDir()
	path := POSIXLauncherPath(home)
	target := filepath.Join(t.TempDir(), "target")
	if err := os.WriteFile(target, []byte("target"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(posixLauncher("/old/opencode")), 0o755); err != nil {
		t.Fatal(err)
	}

	managedPath := path + ".managed"
	original := managedLauncherRemovalBeforeDelete
	t.Cleanup(func() { managedLauncherRemovalBeforeDelete = original })
	managedLauncherRemovalBeforeDelete = func(candidate string) {
		if candidate != path {
			return
		}
		if err := os.Rename(path, managedPath); err != nil {
			t.Errorf("move validated launcher aside: %v", err)
			return
		}
		if err := os.Symlink(target, path); err != nil {
			t.Errorf("install replacement symlink: %v", err)
		}
	}

	result, err := RemoveManagedLauncher(path)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := result.Status, ManagedLauncherRemovalRefused; got != want {
		t.Fatalf("removal status = %q, want %q", got, want)
	}
	info, err := os.Lstat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("replacement mode = %v, want symlink", info.Mode())
	}
	if got, err := os.Readlink(path); err != nil || got != target {
		t.Fatalf("replacement symlink = %q, %v", got, err)
	}
	if got, err := os.ReadFile(managedPath); err != nil || !IsManagedLauncher(path, got) {
		t.Fatalf("original managed launcher = %q, %v; want preserved", got, err)
	}
}

func TestActivationIsIdempotentAndOffRemovesOnlyOwnedFiles(t *testing.T) {
	home := t.TempDir()
	target := filepath.Join(t.TempDir(), "opencode")
	if err := os.WriteFile(target, []byte("real"), 0o755); err != nil {
		t.Fatal(err)
	}
	options := ActivationOptions{
		OS:            "linux",
		Path:          filepath.Dir(target),
		Shell:         "/bin/zsh",
		RunVersion:    func(string) (string, error) { return "1.18.18", nil },
		AddToUserPath: func(string) error { return nil },
		ResolveTarget: func(string, string, string) (string, error) { return target, nil },
	}
	stale, err := PrepareActivation(home, options)
	if err != nil {
		t.Fatal(err)
	}
	path := POSIXLauncherPath(home)
	_ = os.MkdirAll(filepath.Dir(path), 0o755)
	_ = os.WriteFile(path, []byte("user replacement"), 0o600)
	if err := stale.Apply(); err == nil || !strings.Contains(err.Error(), "revalidate") {
		t.Fatalf("stale activation error = %v, want revalidation failure", err)
	}
	_ = os.Remove(path)
	first, err := Activate(home, options)
	if err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	second, err := Activate(home, options)
	if err != nil {
		t.Fatal(err)
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) || len(first.ChangedPaths()) == 0 || len(second.ChangedPaths()) != 0 {
		t.Fatalf("activation changed paths first=%v second=%v", first.ChangedPaths(), second.ChangedPaths())
	}
	stale, err = PrepareDeactivation(home, options)
	if err != nil {
		t.Fatal(err)
	}
	_ = os.WriteFile(path, []byte("user replacement"), 0o600)
	if err := stale.Apply(); err == nil || !strings.Contains(err.Error(), "revalidate") {
		t.Fatalf("stale deactivation error = %v, want revalidation failure", err)
	}
	if err := os.WriteFile(path, before, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := Deactivate(home, options); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("owned launcher stat error = %v, want absent", err)
	}
	if _, err := Deactivate(home, options); err != nil {
		t.Fatal(err)
	}
}

func TestActivationRefusesIncidentalLauncherMarkerAndRefreshesEffectiveResolution(t *testing.T) {
	home := t.TempDir()
	launcher := POSIXLauncherPath(home)
	if err := os.MkdirAll(filepath.Dir(launcher), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(launcher, []byte("#!/bin/sh\n# "+OwnershipMarker+" belongs to user\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	options := ActivationOptions{OS: "linux", Shell: "/bin/zsh", Path: BinDir(home), RunVersion: func(string) (string, error) { return "1.15.11", nil }, ResolveTarget: func(string, string, string) (string, error) { return "/real/opencode", nil }}
	if plan, err := PrepareActivation(home, options); err == nil || plan != nil {
		t.Fatalf("PrepareActivation() = %v, %v; want unowned collision refusal", plan, err)
	}
	if err := os.Remove(launcher); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", BinDir(home)+string(os.PathListSeparator)+os.Getenv("PATH"))
	plan, err := PrepareActivation(home, options)
	if err != nil || plan.Effective() {
		t.Fatalf("prepared activation = %v, effective=%t; want pending", err, plan.Effective())
	}
	if err := plan.Apply(); err != nil {
		t.Fatal(err)
	}
	if !plan.Effective() {
		t.Fatal("Apply() did not refresh effective managed resolution")
	}
}

func TestActivationRollsBackLauncherWritesWhenPathUpdateFails(t *testing.T) {
	home := t.TempDir()
	target := filepath.Join(t.TempDir(), "opencode")
	if err := os.WriteFile(target, []byte("real"), 0o755); err != nil {
		t.Fatal(err)
	}
	pathErr := errors.New("path update failed")
	plan, err := PrepareActivation(home, ActivationOptions{
		OS:            "linux",
		Shell:         "/bin/zsh",
		RunVersion:    func(string) (string, error) { return "1.15.11", nil },
		AddToUserPath: func(string) error { return pathErr },
		ResolveTarget: func(string, string, string) (string, error) { return target, nil },
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := plan.Apply(); err == nil || !strings.Contains(err.Error(), pathErr.Error()) {
		t.Fatalf("Apply() error = %v, want path update failure", err)
	}
	if _, err := os.Stat(POSIXLauncherPath(home)); !os.IsNotExist(err) {
		t.Fatalf("launcher after failed activation = %v, want absent", err)
	}
	if _, err := os.Stat(filepath.Join(home, ".zprofile")); !os.IsNotExist(err) {
		t.Fatalf("profile after failed activation = %v, want absent", err)
	}
}

func TestActivationRefreshesOwnedLauncherMode(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX executable mode is not portable on Windows")
	}
	home := t.TempDir()
	target := filepath.Join(t.TempDir(), "opencode")
	if err := os.WriteFile(target, []byte("real"), 0o755); err != nil {
		t.Fatal(err)
	}
	launcher := POSIXLauncherPath(home)
	if err := os.MkdirAll(filepath.Dir(launcher), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(launcher, []byte(posixLauncher(target)), 0o644); err != nil {
		t.Fatal(err)
	}
	plan, err := PrepareActivation(home, ActivationOptions{
		OS:            "linux",
		Shell:         "/bin/zsh",
		Path:          BinDir(home),
		RunVersion:    func(string) (string, error) { return "1.15.11", nil },
		AddToUserPath: func(string) error { return nil },
		ResolveTarget: func(string, string, string) (string, error) { return target, nil },
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := plan.Apply(); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(launcher)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := info.Mode().Perm(), os.FileMode(0o755); got != want {
		t.Fatalf("owned launcher mode = %o, want %o", got, want)
	}
}

func TestActivationReportIncludesReasonForPreparedAndAppliedPlans(t *testing.T) {
	home := t.TempDir()
	target := filepath.Join(t.TempDir(), "opencode")
	if err := os.WriteFile(target, []byte("real"), 0o755); err != nil {
		t.Fatal(err)
	}
	launcher := POSIXLauncherPath(home)
	if err := os.MkdirAll(filepath.Dir(launcher), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(launcher, []byte(posixLauncher(target)), 0o755); err != nil {
		t.Fatal(err)
	}
	options := ActivationOptions{
		OS:            "linux",
		Shell:         "/bin/zsh",
		Path:          BinDir(home),
		RunVersion:    func(string) (string, error) { return "1.15.11", nil },
		AddToUserPath: func(string) error { return nil },
		ResolveTarget: func(string, string, string) (string, error) { return target, nil },
	}
	plan, err := PrepareActivation(home, options)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := plan.Report().ActivationReason, activationReasonReady; got != want {
		t.Fatalf("prepared activation reason = %q, want %q", got, want)
	}
	if err := plan.Apply(); err != nil {
		t.Fatal(err)
	}
	if got, want := plan.Report().ActivationReason, activationReasonApplied; got != want {
		t.Fatalf("applied activation reason = %q, want %q", got, want)
	}

	deactivation, err := PrepareDeactivation(home, options)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := deactivation.Report().ActivationReason, activationReasonDeactivation; got != want {
		t.Fatalf("deactivation reason = %q, want %q", got, want)
	}
}

func TestActivationEffectivenessUsesPreparedPathSnapshot(t *testing.T) {
	home := t.TempDir()
	target := filepath.Join(t.TempDir(), "opencode")
	if err := os.WriteFile(target, []byte("real"), 0o755); err != nil {
		t.Fatal(err)
	}
	managedDir := BinDir(home)
	ambientDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(ambientDir, "opencode"), []byte("ambient executable"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", ambientDir)
	plan, err := PrepareActivation(home, ActivationOptions{
		OS:    "linux",
		Path:  managedDir,
		Shell: "/bin/zsh",
		RunVersion: func(string) (string, error) {
			return "1.15.11", nil
		},
		AddToUserPath: func(string) error { return nil },
		ResolveTarget: func(string, string, string) (string, error) { return target, nil },
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := plan.Apply(); err != nil {
		t.Fatal(err)
	}
	if !plan.Effective() {
		t.Fatal("activation reported ineffective despite the prepared PATH snapshot resolving the managed launcher")
	}
	if got, want := plan.Report().ActivationReason, activationReasonApplied; got != want {
		t.Fatalf("activation reason = %q, want %q", got, want)
	}
}

func TestCRLFProfileActivationIsIdempotentAndPreservesBytesAndMode(t *testing.T) {
	home := t.TempDir()
	profile := filepath.Join(home, ".zprofile")
	before := []byte("export USER=value\r\n")
	if err := os.WriteFile(profile, before, 0o600); err != nil {
		t.Fatal(err)
	}
	options := testActivationOptions(t, home)
	for i := 0; i < 2; i++ {
		plan, err := PrepareActivation(home, options)
		if err != nil || plan.Apply() != nil {
			t.Fatalf("activate = %v", err)
		}
	}
	block := []byte(profileBlock(BinDir(home), "\r\n"))
	data, err := os.ReadFile(profile)
	if err != nil || !bytes.Equal(data, append(before, block...)) {
		t.Fatalf("CRLF profile = %q, %v", data, err)
	}
	for i := 0; i < 2; i++ {
		plan, err := PrepareDeactivation(home, options)
		if err != nil || plan.Apply() != nil {
			t.Fatalf("deactivate = %v", err)
		}
	}
	data, err = os.ReadFile(profile)
	if err != nil || !bytes.Equal(data, before) {
		t.Fatalf("CRLF profile after deactivation = %q, %v", data, err)
	}
	if info, err := os.Stat(profile); err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("CRLF profile mode = %v, %v", info.Mode(), err)
	}
}
func TestProfileChangesAfterPrepareArePreserved(t *testing.T) {
	home := t.TempDir()
	profile := filepath.Join(home, ".zprofile")
	options := testActivationOptions(t, home)
	plan, err := PrepareActivation(home, options)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(profile, []byte("concurrent activation edit\n"), 0o600); err != nil || plan.Apply() == nil {
		t.Fatal("stale activation did not preserve profile")
	}
	if _, err := Activate(home, options); err != nil {
		t.Fatal(err)
	}
	plan, err = PrepareDeactivation(home, options)
	if err != nil || os.WriteFile(profile, []byte("concurrent deactivation edit\n"), 0o600) != nil || plan.Apply() == nil {
		t.Fatal("stale deactivation did not preserve profile")
	}
}
func testActivationOptions(t *testing.T, home string) ActivationOptions {
	return ActivationOptions{OS: "linux", Shell: "/bin/zsh", Path: t.TempDir(), AddToUserPath: func(string) error { return nil }, RunVersion: func(string) (string, error) { return "1.15.11", nil }, ResolveTarget: func(string, string, string) (string, error) { return "/real/opencode", nil }}
}

func TestPOSIXActivationPersistsAndRemovesOnlyOwnedLoginProfileBlock(t *testing.T) {
	home := t.TempDir()
	profile := filepath.Join(home, ".zprofile")
	if err := os.WriteFile(profile, []byte("export OTHER=value\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	options := ActivationOptions{OS: "darwin", Shell: "/bin/zsh", Path: t.TempDir(), RunVersion: func(string) (string, error) { return "1.15.11", nil }, AddToUserPath: func(string) error { return nil }, ResolveTarget: func(string, string, string) (string, error) { return "/real/opencode", nil }}
	plan, err := Activate(home, options)
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(profile)
	if err != nil || !strings.Contains(string(data), profileStart) || !strings.Contains(string(data), "export OTHER=value") {
		t.Fatalf("profile = %q, %v; want owned block and user content", data, err)
	}
	if plan.Effective() || plan.Report().ActivationReason != activationReasonPathPending {
		t.Fatalf("activation report = %#v, want pending fresh-shell activation", plan.Report())
	}
	if _, err := Deactivate(home, options); err != nil {
		t.Fatal(err)
	}
	data, err = os.ReadFile(profile)
	if err != nil || string(data) != "export OTHER=value\n" {
		t.Fatalf("profile after deactivation = %q, %v; want original user content", data, err)
	}
}

func TestPOSIXActivationLeavesUnsupportedShellPending(t *testing.T) {
	home := t.TempDir()
	plan, err := PrepareActivation(home, ActivationOptions{OS: "darwin", Shell: "/bin/fish", Path: t.TempDir(), RunVersion: func(string) (string, error) { return "1.15.11", nil }, ResolveTarget: func(string, string, string) (string, error) { return "/real/opencode", nil }})
	if err != nil {
		t.Fatal(err)
	}
	if plan.Effective() || !strings.Contains(plan.Report().ActivationReason, "requires zsh") {
		t.Fatalf("activation report = %#v, want actionable pending shell guidance", plan.Report())
	}
	if err := plan.Apply(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(POSIXLauncherPath(home)); !os.IsNotExist(err) {
		t.Fatalf("unsupported shell wrote launcher: %v", err)
	}
}

func TestRemoveProfileChangePreservesMalformedMarkers(t *testing.T) {
	home := t.TempDir()
	path := filepath.Join(home, ".zprofile")
	block := profileBlock(BinDir(home))
	tests := []struct{ name, before, want string }{
		{"orphan start", profileStart + "\nuser\n" + block, profileStart + "\nuser\n"},
		{"interleaved markers", profileStart + "\nuser\n" + profileEnd + "\n" + block, profileStart + "\nuser\n" + profileEnd + "\n"},
		{"duplicate blocks", "before\n" + block + "between\n" + block + "after\n", "before\nbetween\nafter\n"},
		{"user content between malformed markers", profileStart + "\nuser\n" + profileEnd + "\n", profileStart + "\nuser\n" + profileEnd + "\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := os.WriteFile(path, []byte(tt.before), 0o600); err != nil {
				t.Fatal(err)
			}
			change, err := removeProfileChange(path, BinDir(home))
			if err != nil {
				t.Fatal(err)
			}
			if got := string(change.desired); got != tt.want {
				t.Fatalf("profile change = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestPOSIXProfileWritesPreserveModeAndRepeatedOnOffIsIdempotent(t *testing.T) {
	home := t.TempDir()
	profile := filepath.Join(home, ".zprofile")
	if err := os.WriteFile(profile, []byte("user\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	options := ActivationOptions{OS: "darwin", Shell: "/bin/zsh", Path: t.TempDir(), AddToUserPath: func(string) error { return nil }, RunVersion: func(string) (string, error) { return "1.15.11", nil }, ResolveTarget: func(string, string, string) (string, error) { return "/real/opencode", nil }}
	for i := 0; i < 2; i++ {
		if _, err := Activate(home, options); err != nil {
			t.Fatal(err)
		}
	}
	info, err := os.Stat(profile)
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("profile mode = %v, %v; want 0600", info.Mode(), err)
	}
	for i := 0; i < 2; i++ {
		if _, err := Deactivate(home, options); err != nil {
			t.Fatal(err)
		}
	}
	data, err := os.ReadFile(profile)
	if err != nil || string(data) != "user\n" {
		t.Fatalf("profile after repeated deactivation = %q, %v", data, err)
	}
}

func TestPOSIXActivationDoesNotTreatProcessPathMutationAsFreshShellResolution(t *testing.T) {
	home := t.TempDir()
	target := filepath.Join(t.TempDir(), "opencode")
	if err := os.WriteFile(target, []byte("real"), 0o755); err != nil {
		t.Fatal(err)
	}
	pathBeforeMutation := t.TempDir()
	t.Setenv("PATH", pathBeforeMutation)
	plan, err := PrepareActivation(home, ActivationOptions{
		OS:    "linux",
		Path:  pathBeforeMutation,
		Shell: "/bin/zsh",
		RunVersion: func(string) (string, error) {
			return "1.15.11", nil
		},
		AddToUserPath: system.AddToUserPath,
		ResolveTarget: func(string, string, string) (string, error) { return target, nil },
	})
	if err != nil {
		t.Fatal(err)
	}
	if plan.Effective() {
		t.Fatal("prepared activation reported effective before the fresh-shell PATH mutation")
	}
	if err := plan.Apply(); err != nil {
		t.Fatal(err)
	}
	if plan.Effective() {
		t.Fatal("activation treated the mutated process PATH as a fresh-shell resolution")
	}
	if got, want := plan.Report().ActivationReason, activationReasonPathPending; got != want {
		t.Fatalf("activation reason = %q, want %q", got, want)
	}
}

func TestPOSIXProfileWriteRollbackCoversLandedErrors(t *testing.T) {
	tests := []struct {
		name   string
		before []byte
		mode   os.FileMode
	}{
		{name: "new profile", mode: 0o644},
		{name: "pre-existing profile", before: []byte("export USER=value\n"), mode: 0o600},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			home := t.TempDir()
			profile := filepath.Join(home, ".zprofile")
			if tt.before != nil {
				if err := os.WriteFile(profile, tt.before, tt.mode); err != nil {
					t.Fatal(err)
				}
			}
			target := filepath.Join(t.TempDir(), "opencode")
			if err := os.WriteFile(target, []byte("real"), 0o755); err != nil {
				t.Fatal(err)
			}
			writeErr := errors.New("injected landed profile write failure")
			profileWriteCalls := 0
			plan, err := PrepareActivation(home, ActivationOptions{
				OS:            "linux",
				Path:          filepath.Dir(target),
				Shell:         "/bin/zsh",
				RunVersion:    func(string) (string, error) { return "1.15.11", nil },
				AddToUserPath: func(string) error { return nil },
				ResolveTarget: func(string, string, string) (string, error) { return target, nil },
				WriteFile: func(path string, content []byte, mode os.FileMode) (filemerge.WriteResult, error) {
					if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
						return filemerge.WriteResult{}, err
					}
					if err := os.WriteFile(path, content, mode); err != nil {
						return filemerge.WriteResult{}, err
					}
					if err := os.Chmod(path, mode); err != nil {
						return filemerge.WriteResult{}, err
					}
					result := filemerge.WriteResult{Changed: true, Created: tt.before == nil}
					if path == profile {
						profileWriteCalls++
						if profileWriteCalls == 1 {
							return result, writeErr
						}
					}
					return result, nil
				},
			})
			if err != nil {
				t.Fatal(err)
			}
			applyErr := plan.Apply()
			if applyErr == nil || !errors.Is(applyErr, writeErr) {
				t.Fatalf("Apply() error = %v, want %v", applyErr, writeErr)
			}
			if strings.Contains(applyErr.Error(), "rollback restore OpenCode shell profile") {
				t.Fatalf("Apply() reported a profile rollback failure: %v", applyErr)
			}
			if len(plan.profiles) != 1 || !plan.profiles[0].writeResult.Changed {
				t.Fatalf("profile write result = %#v, want landed change recorded", plan.profiles)
			}
			if tt.before == nil {
				if _, err := os.Stat(profile); !os.IsNotExist(err) {
					t.Fatalf("landed new profile after failed activation = %v, want absent", err)
				}
				return
			}
			got, err := os.ReadFile(profile)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(got, tt.before) {
				t.Fatalf("pre-existing profile after failed activation = %q, want original bytes", got)
			}
			info, err := os.Stat(profile)
			if err != nil {
				t.Fatal(err)
			}
			if got, want := info.Mode().Perm(), tt.mode; got != want {
				t.Fatalf("pre-existing profile mode after failed activation = %o, want %o", got, want)
			}
		})
	}
}

func TestActivationRollsBackLauncherWhenWriteLandsThenErrors(t *testing.T) {
	tests := []struct {
		name       string
		before     []byte
		beforeMode os.FileMode
		absent     bool
	}{
		{name: "remove landed new launcher", absent: true},
		{name: "restore landed replacement", before: []byte(posixLauncher("/old/opencode")), beforeMode: 0o644},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			home := t.TempDir()
			target := filepath.Join(t.TempDir(), "opencode")
			if err := os.WriteFile(target, []byte("real"), 0o755); err != nil {
				t.Fatal(err)
			}
			launcher := POSIXLauncherPath(home)
			if !tt.absent {
				if err := os.MkdirAll(filepath.Dir(launcher), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(launcher, tt.before, tt.beforeMode); err != nil {
					t.Fatal(err)
				}
			}

			writeErr := errors.New("injected landed launcher write failure")
			writeCalls := 0
			plan, err := PrepareActivation(home, ActivationOptions{
				OS:            "linux",
				Shell:         "/bin/zsh",
				RunVersion:    func(string) (string, error) { return "1.15.11", nil },
				AddToUserPath: func(string) error { return nil },
				ResolveTarget: func(string, string, string) (string, error) { return target, nil },
				WriteFile: func(path string, content []byte, mode os.FileMode) (filemerge.WriteResult, error) {
					writeCalls++
					if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
						return filemerge.WriteResult{}, err
					}
					if err := os.WriteFile(path, content, mode); err != nil {
						return filemerge.WriteResult{}, err
					}
					if err := os.Chmod(path, mode); err != nil {
						return filemerge.WriteResult{}, err
					}
					result := filemerge.WriteResult{Changed: true, Created: writeCalls == 1 && tt.absent}
					if writeCalls == 1 {
						return result, writeErr
					}
					return result, nil
				},
			})
			if err != nil {
				t.Fatal(err)
			}
			if err := plan.Apply(); err == nil || !strings.Contains(err.Error(), writeErr.Error()) {
				t.Fatalf("Apply() error = %v, want %q", err, writeErr)
			}

			if tt.absent {
				if _, err := os.Stat(launcher); !os.IsNotExist(err) {
					t.Fatalf("landed launcher after failed activation = %v, want absent", err)
				}
				return
			}
			got, err := os.ReadFile(launcher)
			if err != nil {
				t.Fatal(err)
			}
			if string(got) != string(tt.before) {
				t.Fatalf("launcher after failed activation = %q, want original bytes", got)
			}
			info, err := os.Stat(launcher)
			if err != nil {
				t.Fatal(err)
			}
			if got, want := info.Mode().Perm(), tt.beforeMode; got != want {
				t.Fatalf("launcher mode after failed activation = %o, want %o", got, want)
			}
		})
	}
}

func TestEffectiveRequiresAppliedPlan(t *testing.T) {
	home := t.TempDir()
	target := filepath.Join(t.TempDir(), "opencode.exe")
	if err := os.WriteFile(target, []byte("real"), 0o755); err != nil {
		t.Fatal(err)
	}
	plan, err := PrepareActivation(home, ActivationOptions{
		OS:            "windows",
		RunVersion:    func(string) (string, error) { return "1.15.11", nil },
		ResolveTarget: func(string, string, string) (string, error) { return target, nil },
		AddToUserPathWithResult: func(string) (system.UserPathAddition, error) {
			return system.UserPathAddition{}, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if plan.Effective() {
		t.Fatal("prepared Windows activation reported effective before Apply")
	}
	if err := plan.Apply(); err != nil {
		t.Fatal(err)
	}
	if !plan.Effective() {
		t.Fatal("applied Windows activation did not report effective")
	}
}

func TestActivationRevalidationFailureRollsBackEarlierLauncher(t *testing.T) {
	home := t.TempDir()
	target := filepath.Join(t.TempDir(), "opencode.exe")
	if err := os.WriteFile(target, []byte("real"), 0o755); err != nil {
		t.Fatal(err)
	}
	first := WindowsCMDPath(home)
	second := WindowsPS1Path(home)
	writeCalls := 0
	plan, err := PrepareActivation(home, ActivationOptions{
		OS:            "windows",
		RunVersion:    func(string) (string, error) { return "1.15.11", nil },
		ResolveTarget: func(string, string, string) (string, error) { return target, nil },
		WriteFile: func(path string, content []byte, mode os.FileMode) (filemerge.WriteResult, error) {
			writeCalls++
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				return filemerge.WriteResult{}, err
			}
			if err := os.WriteFile(path, content, mode); err != nil {
				return filemerge.WriteResult{}, err
			}
			if writeCalls == 1 {
				if err := os.WriteFile(second, []byte("user replacement"), 0o600); err != nil {
					return filemerge.WriteResult{}, err
				}
			}
			return filemerge.WriteResult{Changed: true, Created: true}, nil
		},
		AddToUserPathWithResult: func(string) (system.UserPathAddition, error) {
			return system.UserPathAddition{}, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := plan.Apply(); err == nil || !strings.Contains(err.Error(), "revalidate") {
		t.Fatalf("Apply() error = %v, want revalidation failure", err)
	}
	if _, err := os.Stat(first); !os.IsNotExist(err) {
		t.Fatalf("earlier launcher after failed activation = %v, want absent", err)
	}
	if got, err := os.ReadFile(second); err != nil || string(got) != "user replacement" {
		t.Fatalf("stale launcher after failed activation = %q, %v; want preserved replacement", got, err)
	}
}

func TestWindowsActivationRollbackRemovesOnlyPlanOwnedPathEntry(t *testing.T) {
	home := t.TempDir()
	target := filepath.Join(t.TempDir(), "opencode.exe")
	if err := os.WriteFile(target, []byte("real"), 0o755); err != nil {
		t.Fatal(err)
	}
	managed := BinDir(home)
	entries := []string{`C:\Tools`, `"C:\Other"`}
	remove := func(dir string) error {
		for i, entry := range entries {
			if strings.EqualFold(strings.Trim(strings.TrimSpace(entry), `"`), dir) {
				entries = append(entries[:i], entries[i+1:]...)
				return nil
			}
		}
		return nil
	}
	plan, err := PrepareActivation(home, ActivationOptions{
		OS: "windows", RunVersion: func(string) (string, error) { return "1.15.11", nil },
		ResolveTarget: func(string, string, string) (string, error) { return target, nil },
		AddToUserPathWithResult: func(string) (system.UserPathAddition, error) {
			entries = append([]string{managed}, entries...)
			return system.UserPathAddition{ProcessAdded: true, PersistentAdded: true}, nil
		},
		RollbackUserPathAddition: func(dir string, addition system.UserPathAddition) error {
			if !addition.PersistentAdded {
				return nil
			}
			return remove(dir)
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := plan.Apply(); err != nil {
		t.Fatal(err)
	}
	if err := plan.Rollback(); err != nil {
		t.Fatal(err)
	}
	if got, want := strings.Join(entries, ";"), `C:\Tools;"C:\Other"`; got != want {
		t.Fatalf("persistent PATH = %q, want unrelated entries preserved as %q", got, want)
	}
}

func TestWindowsActivationRollbackRestoresProcessPathWhenPersistentEntryPreexists(t *testing.T) {
	home := t.TempDir()
	target := filepath.Join(t.TempDir(), "opencode.exe")
	if err := os.WriteFile(target, []byte("real"), 0o755); err != nil {
		t.Fatal(err)
	}
	persistentEntries := []string{`"` + BinDir(home) + `"`, `C:\Tools`}
	processEntries := []string{`C:\Tools`}
	rollbackCalled := false
	plan, err := PrepareActivation(home, ActivationOptions{
		OS: "windows", RunVersion: func(string) (string, error) { return "1.15.11", nil },
		ResolveTarget: func(string, string, string) (string, error) { return target, nil },
		AddToUserPathWithResult: func(dir string) (system.UserPathAddition, error) {
			processEntries = append([]string{dir}, processEntries...)
			return system.UserPathAddition{ProcessAdded: true}, nil
		},
		RollbackUserPathAddition: func(dir string, addition system.UserPathAddition) error {
			rollbackCalled = true
			if !addition.ProcessAdded || addition.PersistentAdded {
				t.Fatalf("rollback addition = %+v, want process-only ownership", addition)
			}
			if processEntries[0] != dir {
				t.Fatalf("process PATH first entry = %q, want %q", processEntries[0], dir)
			}
			processEntries = processEntries[1:]
			return nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := plan.Apply(); err != nil {
		t.Fatal(err)
	}
	if err := plan.Rollback(); err != nil {
		t.Fatal(err)
	}
	if !rollbackCalled || strings.Join(processEntries, ";") != `C:\Tools` {
		t.Fatalf("process PATH rollback called=%t entries=%q, want original process PATH", rollbackCalled, processEntries)
	}
	if got, want := strings.Join(persistentEntries, ";"), `"`+BinDir(home)+`";C:\Tools`; got != want {
		t.Fatalf("persistent PATH = %q, want unchanged %q", got, want)
	}
}

func TestWindowsActivationRollbackJoinsPathRemovalFailureAndRestoresLaunchers(t *testing.T) {
	home := t.TempDir()
	target := filepath.Join(t.TempDir(), "opencode.exe")
	if err := os.WriteFile(target, []byte("real"), 0o755); err != nil {
		t.Fatal(err)
	}
	pathErr := errors.New("path removal failed")
	plan, err := PrepareActivation(home, ActivationOptions{
		OS: "windows", RunVersion: func(string) (string, error) { return "1.15.11", nil },
		ResolveTarget: func(string, string, string) (string, error) { return target, nil },
		AddToUserPathWithResult: func(string) (system.UserPathAddition, error) {
			return system.UserPathAddition{ProcessAdded: true, PersistentAdded: true}, nil
		},
		RollbackUserPathAddition: func(string, system.UserPathAddition) error { return pathErr },
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := plan.Apply(); err != nil {
		t.Fatal(err)
	}
	if err := plan.Rollback(); !errors.Is(err, pathErr) {
		t.Fatalf("Rollback() error = %v, want joined %v", err, pathErr)
	}
	for _, path := range plan.Paths() {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("launcher %q after rollback = %v, want absent", path, err)
		}
	}
}

func TestWindowsActivationFailureAfterPathAdditionRollsBackPath(t *testing.T) {
	home := t.TempDir()
	target := filepath.Join(t.TempDir(), "opencode.exe")
	if err := os.WriteFile(target, []byte("real"), 0o755); err != nil {
		t.Fatal(err)
	}
	pathErr := errors.New("persistent PATH failed after process update")
	entries := []string{`C:\Tools`}
	removed := false
	plan, err := PrepareActivation(home, ActivationOptions{
		OS: "windows", RunVersion: func(string) (string, error) { return "1.15.11", nil },
		ResolveTarget: func(string, string, string) (string, error) { return target, nil },
		AddToUserPathWithResult: func(dir string) (system.UserPathAddition, error) {
			entries = append([]string{dir}, entries...)
			return system.UserPathAddition{ProcessAdded: true, PersistentAdded: true}, pathErr
		},
		RollbackUserPathAddition: func(dir string, addition system.UserPathAddition) error {
			if !addition.PersistentAdded {
				return nil
			}
			removed = true
			entries = entries[1:]
			return nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := plan.Apply(); !errors.Is(err, pathErr) {
		t.Fatalf("Apply() error = %v, want %v", err, pathErr)
	}
	if !removed || strings.Join(entries, ";") != `C:\Tools` {
		t.Fatalf("PATH failure compensation = removed:%t entries:%q, want original entries", removed, entries)
	}
}

func TestWindowsActivationPersistentPathFailurePreservesPreexistingProcessEntry(t *testing.T) {
	home := t.TempDir()
	target := filepath.Join(t.TempDir(), "opencode.exe")
	if err := os.WriteFile(target, []byte("real"), 0o755); err != nil {
		t.Fatal(err)
	}
	preexistingProcessPath := true
	persistentErr := errors.New("persistent PATH failed")
	rollbackCalled := false
	plan, err := PrepareActivation(home, ActivationOptions{
		OS:            "windows",
		RunVersion:    func(string) (string, error) { return "1.15.11", nil },
		ResolveTarget: func(string, string, string) (string, error) { return target, nil },
		AddToUserPathWithResult: func(string) (system.UserPathAddition, error) {
			return system.UserPathAddition{}, persistentErr
		},
		RollbackUserPathAddition: func(string, system.UserPathAddition) error {
			rollbackCalled = true
			preexistingProcessPath = false
			return nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := plan.Apply(); !errors.Is(err, persistentErr) {
		t.Fatalf("Apply() error = %v, want %v", err, persistentErr)
	}
	if rollbackCalled || !preexistingProcessPath {
		t.Fatalf("process PATH compensation called=%t preexisting=%t, want pre-existing entry preserved", rollbackCalled, preexistingProcessPath)
	}
}

// The transaction and parsing conveniences below are test-scoped: production
// callers drive Prepare*/Apply through the pipeline step, so keeping these in
// the shipped package would be dead code under the ratchet.
// Activate prepares and applies a managed activation transaction.
func Activate(homeDir string, options ActivationOptions) (*ActivationPlan, error) {
	plan, err := PrepareActivation(homeDir, options)
	if err != nil {
		return nil, err
	}
	if err := plan.Apply(); err != nil {
		return plan, err
	}
	return plan, nil
}

// Deactivate prepares and applies a managed deactivation transaction.
func Deactivate(homeDir string, options ActivationOptions) (*ActivationPlan, error) {
	plan, err := PrepareDeactivation(homeDir, options)
	if err != nil {
		return nil, err
	}
	if err := plan.Apply(); err != nil {
		return plan, err
	}
	return plan, nil
}

// ParseVersion parses a semantic version such as "v1.15.11".
func ParseVersion(raw string) (Version, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return Version{}, errors.New("OpenCode version is empty")
	}
	if strings.HasPrefix(value, "v") || strings.HasPrefix(value, "V") {
		value = value[1:]
	}
	match := versionPattern.FindStringSubmatch(" " + value + " ")
	if match == nil || strings.TrimSpace(match[0]) != value {
		return Version{}, fmt.Errorf("invalid OpenCode version %q", raw)
	}
	return versionFromMatch(match)
}
