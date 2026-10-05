package opencode

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v4/internal/system"
)

// readinessFixture models the OpenCode installer layout: the real binary lives
// in ~/.opencode/bin and the inherited PATH does not contain the managed dir.
type readinessFixture struct {
	home    string
	target  string
	options ActivationOptions
}

func newReadinessFixture(t *testing.T, shell string) readinessFixture {
	t.Helper()
	skipProfileTestOnWindows(t)
	// Activation and doctor read $SHELL/$ZDOTDIR by default; pin both.
	t.Setenv("SHELL", shell)
	t.Setenv("ZDOTDIR", "")
	home := t.TempDir()
	target := filepath.Join(home, ".opencode", "bin", "opencode")
	writeExecutable(t, target, "real")
	return readinessFixture{
		home:   home,
		target: target,
		options: ActivationOptions{
			OS:            "linux",
			Shell:         shell,
			Path:          "/usr/bin:/bin",
			RunVersion:    func(string) (string, error) { return "1.15.11", nil },
			AddToUserPath: func(string) error { return nil },
			ResolveTarget: func(string, string, string) (string, error) { return target, nil },
		},
	}
}

func writeExecutable(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
		t.Fatal(err)
	}
}

func writeStartupFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func assertUnchanged(t *testing.T, path, want string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil || string(data) != want {
		t.Fatalf("%s = %q, %v; want untouched %q", path, data, err, want)
	}
}

// Issue #3453: capability readiness is not activation readiness. A new login
// shell must resolve bare opencode to the managed launcher before the report
// claims ready.
func TestActivationReportsReadyOnlyWhenNewLoginShellResolvesLauncher(t *testing.T) {
	fixture := newReadinessFixture(t, "/bin/zsh")
	plan, err := PrepareActivation(fixture.home, fixture.options)
	if err != nil {
		t.Fatal(err)
	}
	if report := plan.Report(); report.Status != ActivationStatusPending || report.Effective {
		t.Fatalf("prepared report = %#v, want pending and not effective before apply", report)
	}
	if err := plan.Apply(); err != nil {
		t.Fatal(err)
	}
	report := plan.Report()
	if report.Status != ActivationStatusReady || !report.Effective {
		t.Fatalf("applied report = %#v, want ready and effective", report)
	}
	if !strings.Contains(report.ActivationReason, POSIXLauncherPath(fixture.home)) || !strings.Contains(report.ActivationReason, "new login shell") {
		t.Fatalf("activation reason = %q, want launcher path and new-login-shell guidance", report.ActivationReason)
	}
	if strings.Contains(plan.RestartGuidance(), "Login profile") {
		t.Fatalf("restart guidance = %q, profile outcome belongs in ActivationReason", plan.RestartGuidance())
	}
}

// The OpenCode installer appends `export PATH=<home>/.opencode/bin:$PATH` to
// .zshrc/.bashrc. Interactive rc files run after the login profile, so the real
// binary precedes the launcher; the report must say so and leave rc files alone.
func TestActivationReportsShadowedWhenRcFilePrependsRealOpenCode(t *testing.T) {
	for _, tt := range []struct {
		name    string
		shell   string
		profile string
		rc      string
		setup   string
	}{
		{name: "zsh installer line in zshrc", shell: "/bin/zsh", rc: ".zshrc", setup: "export PATH=%s:$PATH\n"},
		{name: "zsh HOME-relative zshrc line", shell: "/bin/zsh", rc: ".zshrc", setup: "export PATH=\"$HOME/.opencode/bin:$PATH\"\n"},
		{name: "zsh path array in zshrc", shell: "/bin/zsh", rc: ".zshrc", setup: "path=(~/.opencode/bin $path)\n"},
		{name: "bash rc sourced by profile", shell: "/bin/bash", profile: ".profile", rc: ".bashrc", setup: "# opencode\nexport PATH=%s:$PATH\n"},
		{name: "bash rc read by interactive shells", shell: "/bin/bash", profile: ".bash_profile", rc: ".bashrc", setup: "export PATH=%s:$PATH\n"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			fixture := newReadinessFixture(t, tt.shell)
			if tt.profile == ".profile" {
				writeStartupFile(t, filepath.Join(fixture.home, ".profile"), "if [ -n \"$BASH_VERSION\" ]; then\n  if [ -f \"$HOME/.bashrc\" ]; then\n    . \"$HOME/.bashrc\"\n  fi\nfi\n")
			} else if tt.profile != "" {
				writeStartupFile(t, filepath.Join(fixture.home, tt.profile), "# login\n")
			}
			rcPath := filepath.Join(fixture.home, tt.rc)
			rcContent := tt.setup
			if strings.Contains(rcContent, "%s") {
				rcContent = strings.ReplaceAll(rcContent, "%s", filepath.Dir(fixture.target))
			}
			writeStartupFile(t, rcPath, rcContent)

			plan, err := PrepareActivation(fixture.home, fixture.options)
			if err != nil {
				t.Fatal(err)
			}
			if prepared := plan.Report(); prepared.Status != ActivationStatusShadowed {
				t.Fatalf("dry-run report = %#v, want predicted shadowed", prepared)
			}
			if err := plan.Apply(); err != nil {
				t.Fatal(err)
			}
			report := plan.Report()
			if report.Status != ActivationStatusShadowed || report.Effective {
				t.Fatalf("report = %#v, want shadowed and not effective", report)
			}
			for _, want := range []string{rcPath, fixture.target, ProfileExportLine(BinDir(fixture.home))} {
				if !strings.Contains(report.ActivationReason, want) {
					t.Fatalf("activation reason = %q, want %q", report.ActivationReason, want)
				}
			}
			assertUnchanged(t, rcPath, rcContent)
		})
	}
}

// The user-applied remedy (re-adding the managed dir after the installer
// line) makes the launcher win again.
func TestActivationReadyWhenManagedDirIsPrependedAfterRcShadow(t *testing.T) {
	fixture := newReadinessFixture(t, "/bin/zsh")
	writeStartupFile(t, filepath.Join(fixture.home, ".zshrc"), "export PATH="+filepath.Dir(fixture.target)+":$PATH\nexport PATH=\"$HOME/.gentle-ai/bin:$PATH\" # gentle-ai first\n")
	plan, err := Activate(fixture.home, fixture.options)
	if err != nil {
		t.Fatal(err)
	}
	if report := plan.Report(); report.Status != ActivationStatusReady || !report.Effective {
		t.Fatalf("report = %#v, want ready after the managed dir is re-prepended", report)
	}
}

// Without PATH persistence the activation is pending with the manual export
// line in ActivationReason rather than in restart guidance.
func TestActivationPendingWhenLoginProfileCannotPersistPath(t *testing.T) {
	fixture := newReadinessFixture(t, "/usr/bin/fish")
	plan, err := Activate(fixture.home, fixture.options)
	if err != nil {
		t.Fatal(err)
	}
	report := plan.Report()
	if report.Status != ActivationStatusPending || report.Effective {
		t.Fatalf("report = %#v, want pending", report)
	}
	if !strings.Contains(report.ActivationReason, "not supported") || !strings.Contains(report.ActivationReason, ProfileExportLine(BinDir(fixture.home))) {
		t.Fatalf("activation reason = %q, want shell refusal and manual export line", report.ActivationReason)
	}
}

func TestResolveLoginShellActivationMatchesActivationModel(t *testing.T) {
	fixture := newReadinessFixture(t, "/bin/zsh")
	if _, err := Activate(fixture.home, fixture.options); err != nil {
		t.Fatal(err)
	}
	if got := ResolveLoginShellActivation(fixture.home, fixture.options); got.Status != ActivationStatusReady || got.Resolved != POSIXLauncherPath(fixture.home) {
		t.Fatalf("ResolveLoginShellActivation() = %#v, want ready at launcher", got)
	}
	zshrc := filepath.Join(fixture.home, ".zshrc")
	writeStartupFile(t, zshrc, "export PATH="+filepath.Dir(fixture.target)+":$PATH\n")
	got := ResolveLoginShellActivation(fixture.home, fixture.options)
	if got.Status != ActivationStatusShadowed || got.Resolved != fixture.target || got.Source != zshrc {
		t.Fatalf("ResolveLoginShellActivation() = %#v, want shadowed by %s from %s", got, fixture.target, zshrc)
	}
}

func TestResolveManagedLauncherRequiresFirstOwnedExecutable(t *testing.T) {
	skipProfileTestOnWindows(t)
	home := t.TempDir()
	launcher := POSIXLauncherPath(home)
	other := filepath.Join(t.TempDir(), "opencode")
	writeExecutable(t, other, "real")
	writeExecutable(t, launcher, posixLauncher(other))
	binDir := BinDir(home)
	if got, err := ResolveManagedLauncher(home, binDir+":"+filepath.Dir(other), "linux"); err != nil || got != launcher {
		t.Fatalf("ResolveManagedLauncher(managed first) = %q, %v; want %q", got, err, launcher)
	}
	if _, err := ResolveManagedLauncher(home, filepath.Dir(other)+":"+binDir, "linux"); err == nil || !strings.Contains(err.Error(), other) {
		t.Fatalf("ResolveManagedLauncher(shadowed) error = %v, want shadowing executable named", err)
	}
	writeExecutable(t, launcher, "#!/bin/sh\n# "+OwnershipMarker+"\nexec /bin/true\n")
	if _, err := ResolveManagedLauncher(home, binDir, "linux"); err == nil || !strings.Contains(err.Error(), "not Gentle-owned") {
		t.Fatalf("ResolveManagedLauncher(user file) error = %v, want ownership refusal", err)
	}
}

func TestManagedLauncherTargetReturnsDelegationTarget(t *testing.T) {
	dir := t.TempDir()
	for _, tt := range []struct {
		name    string
		content string
		want    string
		ok      bool
	}{
		{name: "opencode", content: posixLauncher("/home/u/.opencode/bin/opencode"), want: "/home/u/.opencode/bin/opencode", ok: true},
		{name: "opencode.cmd", content: windowsCMDLauncher(`C:\Users\u\AppData\Local\pnpm\opencode.CMD`), want: `C:\Users\u\AppData\Local\pnpm\opencode.CMD`, ok: true},
		{name: "opencode.ps1", content: windowsPS1Launcher(`C:\scoop\shims\opencode.exe`), want: `C:\scoop\shims\opencode.exe`, ok: true},
		{name: "opencode", content: "#!/bin/sh\n# " + OwnershipMarker + "\nexec /x \"$@\"\n"},
	} {
		path := filepath.Join(dir, tt.name)
		if err := os.WriteFile(path, []byte(tt.content), 0o755); err != nil {
			t.Fatal(err)
		}
		if got, ok := ManagedLauncherTarget(path); got != tt.want || ok != tt.ok {
			t.Fatalf("ManagedLauncherTarget(%s) = %q, %t; want %q, %t", tt.name, got, ok, tt.want, tt.ok)
		}
	}
}

// cmd.exe expands %VAR% (and !VAR! under delayed expansion) inside the quoted
// target, so such targets must fail clearly before any launcher is written.
func TestWindowsActivationRefusesCMDExpansionCharactersInTarget(t *testing.T) {
	for _, target := range []string{`C:\Users\100%\opencode.exe`, `C:\Tools\%PATH%\opencode.exe`, `C:\Bang!\opencode.exe`} {
		t.Run(target, func(t *testing.T) {
			home := t.TempDir()
			added := false
			plan, err := Activate(home, ActivationOptions{
				OS:         "windows",
				Path:       `C:\Tools`,
				RunVersion: func(string) (string, error) { return "1.15.11", nil },
				AddToUserPathWithResult: func(string) (system.UserPathAddition, error) {
					added = true
					return system.UserPathAddition{}, nil
				},
				ResolveTarget: func(string, string, string) (string, error) { return target, nil },
			})
			if err != nil {
				t.Fatal(err)
			}
			report := plan.Report()
			if report.Capability.Status != CapabilityUnsupported || report.Status != ActivationStatusUnsupported || !strings.Contains(report.Capability.Reason, "cmd.exe") {
				t.Fatalf("report = %#v, want unsupported with cmd.exe expansion reason", report)
			}
			if added {
				t.Fatal("PATH was changed for a refused Windows target")
			}
			for _, path := range ManagedLauncherPaths(home, "windows") {
				if _, err := os.Stat(path); !os.IsNotExist(err) {
					t.Fatalf("launcher %s stat error = %v, want absent", path, err)
				}
			}
		})
	}
}

// Windows reports ready only when the PATH a new terminal inherits resolves
// to the managed launcher; a preceding package-manager shim shadows it.
func TestWindowsActivationStatusFollowsNewShellPath(t *testing.T) {
	t.Setenv("PATHEXT", ".COM;.EXE;.BAT;.CMD")
	home := t.TempDir()
	scoop := filepath.Join(t.TempDir(), "scoop", "shims")
	target := filepath.Join(scoop, "opencode.exe")
	writeExecutable(t, target, "real")
	for _, tt := range []struct {
		name     string
		newShell func() (string, error)
		want     ActivationStatus
	}{
		{name: "managed first", newShell: func() (string, error) { return BinDir(home) + ";" + scoop, nil }, want: ActivationStatusReady},
		{name: "shim first", newShell: func() (string, error) { return scoop + ";" + BinDir(home), nil }, want: ActivationStatusShadowed},
		{name: "unreadable", newShell: func() (string, error) { return "", os.ErrPermission }, want: ActivationStatusUnknown},
	} {
		t.Run(tt.name, func(t *testing.T) {
			plan, err := Activate(home, ActivationOptions{
				OS:         "windows",
				Path:       scoop,
				RunVersion: func(string) (string, error) { return "1.15.11", nil },
				AddToUserPathWithResult: func(string) (system.UserPathAddition, error) {
					return system.UserPathAddition{}, nil
				},
				ResolveTarget: func(string, string, string) (string, error) { return target, nil },
				NewShellPath:  tt.newShell,
			})
			if err != nil {
				t.Fatal(err)
			}
			if report := plan.Report(); report.Status != tt.want || report.Effective != (tt.want == ActivationStatusReady) || report.ActivationReason == "" {
				t.Fatalf("report = %#v, want status %q with a reason", report, tt.want)
			}
		})
	}
}

func TestWindowsResolveTargetFollowsPATHEXTOrder(t *testing.T) {
	home := t.TempDir()
	dir := t.TempDir()
	writeExecutable(t, filepath.Join(dir, "opencode.exe"), "exe")
	writeExecutable(t, filepath.Join(dir, "opencode.cmd"), "cmd")
	t.Setenv("PATHEXT", ".CMD;.EXE")
	got, err := ResolveTarget(home, "windows", dir)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(got) != "opencode.cmd" {
		t.Fatalf("ResolveTarget() = %q, want PATHEXT-first opencode.cmd", got)
	}
}

// The startup model replays only what it can expand statically and ignores
// everything else instead of guessing.
func TestLoginShellModelReplaysOnlyStaticPathEdits(t *testing.T) {
	for _, tt := range []struct {
		name  string
		os    string
		shell string
		file  string
		rc    string
		want  ActivationStatus
	}{
		{name: "commented installer line", shell: "/bin/zsh", file: ".zshrc", rc: "# export PATH=%s:$PATH\n", want: ActivationStatusReady},
		{name: "trailing comment does not hide edit", shell: "/bin/zsh", file: ".zshrc", rc: "export PATH=%s:$PATH # opencode\n", want: ActivationStatusShadowed},
		{name: "append keeps launcher first", shell: "/bin/zsh", file: ".zshrc", rc: "export PATH=\"$PATH:%s\"\n", want: ActivationStatusReady},
		{name: "variable defined earlier", shell: "/bin/zsh", file: ".zshrc", rc: "OPENCODE_HOME=%s\nexport PATH=\"$OPENCODE_HOME:$PATH\"\n", want: ActivationStatusShadowed},
		{name: "command substitution is ignored", shell: "/bin/zsh", file: ".zshrc", rc: "export PATH=\"$(opencode-prefix):$PATH\"\n", want: ActivationStatusReady},
		{name: "unquoted substitution with blanks", shell: "/bin/zsh", file: ".zshrc", rc: "export PATH=$(brew --prefix | head -1)/bin:$PATH; PATH=%s:$PATH\n", want: ActivationStatusShadowed},
		{name: "one-line conditional", shell: "/bin/zsh", file: ".zshrc", rc: "if [ -d %s ]; then PATH=%s:$PATH; fi\n", want: ActivationStatusShadowed},
		{name: "assignment scoped to a command", shell: "/bin/zsh", file: ".zshrc", rc: "PATH=%s:$PATH opencode --version\n", want: ActivationStatusReady},
		{name: "zlogin runs last", shell: "/bin/zsh", file: ".zlogin", rc: "path=(%s $path)\n", want: ActivationStatusShadowed},
		{name: "macOS bash does not read unsourced bashrc", os: "darwin", shell: "/bin/bash", file: ".bashrc", rc: "export PATH=%s:$PATH\n", want: ActivationStatusReady},
	} {
		t.Run(tt.name, func(t *testing.T) {
			fixture := newReadinessFixture(t, tt.shell)
			if tt.os != "" {
				fixture.options.OS = tt.os
			}
			rc := strings.ReplaceAll(tt.rc, "%s", filepath.Dir(fixture.target))
			writeStartupFile(t, filepath.Join(fixture.home, tt.file), rc)
			plan, err := Activate(fixture.home, fixture.options)
			if err != nil {
				t.Fatal(err)
			}
			if report := plan.Report(); report.Status != tt.want {
				t.Fatalf("report = %#v, want %q", report, tt.want)
			}
		})
	}
}

func TestResolveLoginShellActivationFollowsLinksToTheLauncher(t *testing.T) {
	fixture := newReadinessFixture(t, "/bin/sh")
	writeExecutable(t, POSIXLauncherPath(fixture.home), posixLauncher(fixture.target))
	linkDir := t.TempDir()
	if err := os.Symlink(POSIXLauncherPath(fixture.home), filepath.Join(linkDir, "opencode")); err != nil {
		t.Skipf("symlink creation unavailable: %v", err)
	}
	fixture.options.Path = linkDir + ":" + filepath.Dir(fixture.target)
	if got := ResolveLoginShellActivation(fixture.home, fixture.options); got.Status != ActivationStatusReady {
		t.Fatalf("ResolveLoginShellActivation() = %#v, want ready through the link", got)
	}
}
