package gga

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/system"
	"github.com/gentleman-programming/gentle-ai/v2/internal/versions"
)

// resolveGitBashForTest derives the Git Bash path the same way the installcmd
// package does. This keeps the test independent from installcmd's unexported
// gitBashPath() while ensuring the expected value matches what the resolver
// actually produces.
func resolveGitBashForTest() string {
	if gitPath, err := exec.LookPath("git"); err == nil {
		gitDir := filepath.Dir(gitPath)
		parent := filepath.Dir(gitDir)

		if c := filepath.Join(parent, "bin", "bash.exe"); fileExistsForTest(c) {
			return c
		}
		if c := filepath.Join(gitDir, "bash.exe"); fileExistsForTest(c) {
			return c
		}
	}

	for _, c := range []string{
		filepath.Join(os.Getenv("ProgramFiles"), "Git", "bin", "bash.exe"),
		filepath.Join(os.Getenv("ProgramFiles(x86)"), "Git", "bin", "bash.exe"),
		`C:\Program Files\Git\bin\bash.exe`,
	} {
		if c != "" && fileExistsForTest(c) {
			return c
		}
	}

	return "bash"
}

func fileExistsForTest(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func TestInstallCommandByProfile(t *testing.T) {
	cloneDst := filepath.Join(os.TempDir(), "gentleman-guardian-angel")
	bash := resolveGitBashForTest()
	scriptPath := strings.ReplaceAll(filepath.Join(cloneDst, "install.sh"), `\`, "/")

	tests := []struct {
		name    string
		profile system.PlatformProfile
		want    [][]string
		wantErr bool
	}{
		{
			name:    "darwin uses brew tap and reinstall",
			profile: system.PlatformProfile{OS: "darwin", PackageManager: "brew"},
			want:    [][]string{{"brew", "tap", "Gentleman-Programming/homebrew-tap"}, {"brew", "reinstall", "gga"}},
		},
		{
			name:    "ubuntu uses git clone and install.sh",
			profile: system.PlatformProfile{OS: "linux", LinuxDistro: system.LinuxDistroUbuntu, PackageManager: "apt"},
			want: [][]string{
				{"rm", "-rf", "/tmp/gentleman-guardian-angel"},
				{"mkdir", "-p", "/tmp/gentleman-guardian-angel"},
				{"git", "init", "/tmp/gentleman-guardian-angel"},
				{"git", "-C", "/tmp/gentleman-guardian-angel", "fetch", "--depth=1", "https://github.com/Gentleman-Programming/gentleman-guardian-angel.git", "refs/tags/v" + versions.GGAVersion + ":refs/tags/v" + versions.GGAVersion},
				{"git", "-C", "/tmp/gentleman-guardian-angel", "checkout", "-f", "refs/tags/v" + versions.GGAVersion},
				{"bash", "/tmp/gentleman-guardian-angel/install.sh"},
			},
		},
		{
			name:    "arch uses git clone and install.sh",
			profile: system.PlatformProfile{OS: "linux", LinuxDistro: system.LinuxDistroArch, PackageManager: "pacman"},
			want: [][]string{
				{"rm", "-rf", "/tmp/gentleman-guardian-angel"},
				{"mkdir", "-p", "/tmp/gentleman-guardian-angel"},
				{"git", "init", "/tmp/gentleman-guardian-angel"},
				{"git", "-C", "/tmp/gentleman-guardian-angel", "fetch", "--depth=1", "https://github.com/Gentleman-Programming/gentleman-guardian-angel.git", "refs/tags/v" + versions.GGAVersion + ":refs/tags/v" + versions.GGAVersion},
				{"git", "-C", "/tmp/gentleman-guardian-angel", "checkout", "-f", "refs/tags/v" + versions.GGAVersion},
				{"bash", "/tmp/gentleman-guardian-angel/install.sh"},
			},
		},
		{
			name:    "windows uses git bash after runtime cleanup",
			profile: system.PlatformProfile{OS: "windows", PackageManager: "winget"},
			want: [][]string{
				{"git", "clone", "--depth=1", "--branch", "v" + versions.GGAVersion, "https://github.com/Gentleman-Programming/gentleman-guardian-angel.git", cloneDst},
				{bash, scriptPath},
			},
		},
		{
			name:    "fedora uses git clone and install.sh",
			profile: system.PlatformProfile{OS: "linux", LinuxDistro: system.LinuxDistroFedora, PackageManager: "dnf"},
			want: [][]string{
				{"rm", "-rf", "/tmp/gentleman-guardian-angel"},
				{"mkdir", "-p", "/tmp/gentleman-guardian-angel"},
				{"git", "init", "/tmp/gentleman-guardian-angel"},
				{"git", "-C", "/tmp/gentleman-guardian-angel", "fetch", "--depth=1", "https://github.com/Gentleman-Programming/gentleman-guardian-angel.git", "refs/tags/v" + versions.GGAVersion + ":refs/tags/v" + versions.GGAVersion},
				{"git", "-C", "/tmp/gentleman-guardian-angel", "checkout", "-f", "refs/tags/v" + versions.GGAVersion},
				{"bash", "/tmp/gentleman-guardian-angel/install.sh"},
			},
		},
		{
			// Issue #2499: the probe (#2493) accepts any Linux package manager
			// on PATH; only a probe-rejected profile (no manager found) errors.
			name: "linux without package manager returns error",
			profile: system.PlatformProfile{
				OS:             "linux",
				PackageManager: "",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			command, err := InstallCommand(tt.profile)
			if (err != nil) != tt.wantErr {
				t.Fatalf("InstallCommand() error = %v, wantErr %v", err, tt.wantErr)
			}

			if tt.wantErr {
				return
			}

			if !reflect.DeepEqual(command, tt.want) {
				t.Fatalf("InstallCommand() = %v, want %v", command, tt.want)
			}
		})
	}
}

func TestCleanupInstallDirUsesPowerShellResolverAndIsIdempotent(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(t.TempDir(), "gentleman-guardian-angel")
	if err := os.MkdirAll(target, 0o755); err != nil {
		t.Fatal(err)
	}
	host := "pwsh"
	if runtime.GOOS == "windows" {
		host += ".exe"
	}
	if err := os.WriteFile(filepath.Join(dir, host), nil, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	runner := system.NewPowerShellRunner()
	runner.RunCommand = func(_ context.Context, _ string, _ ...string) ([]byte, error) { return nil, os.RemoveAll(target) }

	for i := 0; i < 2; i++ {
		if err := cleanupInstallDirWith(runner, target); err != nil {
			t.Fatalf("cleanupInstallDir() run %d error = %v", i+1, err)
		}
	}
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Fatalf("cleanupInstallDir() left %q behind", target)
	}
}

func TestExternalInstallPathsListsOnlyUpstreamScriptDestinations(t *testing.T) {
	tests := []struct {
		name string
		goos string
		want []string
	}{
		{
			name: "linux",
			goos: "linux",
			want: []string{"/usr/local/bin/gga", filepath.Join("HOME", ".local", "bin", "gga")},
		},
		{
			name: "windows",
			goos: "windows",
			want: []string{filepath.Join("HOME", "bin", "gga"), filepath.Join("HOME", "bin", "gga.bat")},
		},
		{name: "unsupported platform", goos: "darwin"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ExternalInstallPaths(tt.goos, "HOME"); !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("ExternalInstallPaths() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestObserveExternalInstallPathPresenceRecordsEveryPathOrAborts(t *testing.T) {
	info, err := os.Lstat(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	paths := ExternalInstallPaths("linux", "HOME")
	got, err := observeExternalInstallPathPresence("linux", "HOME", func(path string) (os.FileInfo, error) {
		switch path {
		case paths[0]:
			return info, nil
		case paths[1]:
			return nil, os.ErrNotExist
		default:
			t.Fatalf("lstat called for unexpected path %q", path)
			return nil, nil
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	want := []ExternalInstallPathPresence{{Path: paths[0], Present: true}, {Path: paths[1]}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("presence = %#v, want %#v", got, want)
	}

	_, err = observeExternalInstallPathPresence("linux", "HOME", func(string) (os.FileInfo, error) {
		return nil, os.ErrPermission
	})
	if !errors.Is(err, os.ErrPermission) || !strings.Contains(err.Error(), paths[0]) {
		t.Fatalf("observation error = %v, want path-specific permission failure", err)
	}
}

type commandOutputRunnerFunc func(name string, args ...string) ([]byte, error)

func (f commandOutputRunnerFunc) Output(name string, args ...string) ([]byte, error) {
	return f(name, args...)
}

func TestQueryExternalRoutePreservesExactScriptObservations(t *testing.T) {
	paths := []string{"present", "unknown", "absent"}
	info, err := os.Lstat(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	var lstatCalls []string
	result, err := queryExternalRoute("linux", "script", paths, nil, func(path string) (os.FileInfo, error) {
		lstatCalls = append(lstatCalls, path)
		switch path {
		case "present":
			return info, nil
		case "unknown":
			return nil, os.ErrPermission
		default:
			return nil, os.ErrNotExist
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(lstatCalls, paths) {
		t.Fatalf("lstat calls = %v, want %v", lstatCalls, paths)
	}
	if !reflect.DeepEqual(result.PresentPaths, []string{"present"}) {
		t.Fatalf("present paths = %v", result.PresentPaths)
	}
	if len(result.UnknownPaths) != 1 || result.UnknownPaths[0].Path != "unknown" || !errors.Is(result.UnknownPaths[0].Err, os.ErrPermission) {
		t.Fatalf("unknown paths = %#v", result.UnknownPaths)
	}
}

func TestQueryExternalRouteTreatsBrokenSymlinkAsPresent(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows symlink creation may require elevated privileges")
	}
	link := filepath.Join(t.TempDir(), "gga")
	if err := os.Symlink("missing-gga", link); err != nil {
		t.Fatal(err)
	}
	result, err := QueryExternalRoute("linux", "script", []string{link}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(result.PresentPaths, []string{link}) {
		t.Fatalf("present paths = %v, want broken symlink %q", result.PresentPaths, link)
	}
}

func TestQueryExternalRouteQueriesExactBrewFormulaLines(t *testing.T) {
	var commands [][]string
	result, err := QueryExternalRoute("darwin", "brew", nil, commandOutputRunnerFunc(func(name string, args ...string) ([]byte, error) {
		commands = append(commands, append([]string{name}, args...))
		return []byte("git\nnot-gga\ngga\n"), nil
	}))
	if err != nil {
		t.Fatal(err)
	}
	if !result.FormulaPresent {
		t.Fatal("formula presence = false, want true exact gga line")
	}
	if want := [][]string{{"brew", "list", "--formula"}}; !reflect.DeepEqual(commands, want) {
		t.Fatalf("commands = %v, want %v", commands, want)
	}

	_, err = QueryExternalRoute("darwin", "brew", nil, commandOutputRunnerFunc(func(string, ...string) ([]byte, error) {
		return nil, errors.New("brew unavailable")
	}))
	if err == nil || !strings.Contains(err.Error(), "brew list --formula") {
		t.Fatalf("brew error = %v, want runnable retry", err)
	}
}

func TestQueryExternalRouteRejectsUnsupportedRecordedRoutes(t *testing.T) {
	for _, tt := range []struct {
		name  string
		goos  string
		route string
	}{
		{name: "script platform", goos: "darwin", route: "script"},
		{name: "unknown route", goos: "linux", route: "unknown"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			_, err := QueryExternalRoute(tt.goos, tt.route, nil, nil)
			if err == nil || !strings.Contains(err.Error(), "record") {
				t.Fatalf("QueryExternalRoute() error = %v, want actionable recorded-route error", err)
			}
		})
	}
}

func TestShouldInstall(t *testing.T) {
	if !ShouldInstall(true) {
		t.Fatalf("ShouldInstall(true) = false")
	}

	if ShouldInstall(false) {
		t.Fatalf("ShouldInstall(false) = true")
	}
}
