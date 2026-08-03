package gga

import (
	"context"
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
			name: "unsupported package manager returns error",
			profile: system.PlatformProfile{
				OS:             "linux",
				PackageManager: "zypper",
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

func TestShouldInstall(t *testing.T) {
	if !ShouldInstall(true) {
		t.Fatalf("ShouldInstall(true) = false")
	}

	if ShouldInstall(false) {
		t.Fatalf("ShouldInstall(false) = true")
	}
}
