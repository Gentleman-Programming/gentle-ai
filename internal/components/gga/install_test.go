package gga

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/internal/system"
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
				{"git", "clone", "https://github.com/Gentleman-Programming/gentleman-guardian-angel.git", "/tmp/gentleman-guardian-angel"},
				{"bash", "/tmp/gentleman-guardian-angel/install.sh"},
			},
		},
		{
			name:    "arch uses git clone and install.sh",
			profile: system.PlatformProfile{OS: "linux", LinuxDistro: system.LinuxDistroArch, PackageManager: "pacman"},
			want: [][]string{
				{"rm", "-rf", "/tmp/gentleman-guardian-angel"},
				{"git", "clone", "https://github.com/Gentleman-Programming/gentleman-guardian-angel.git", "/tmp/gentleman-guardian-angel"},
				{"bash", "/tmp/gentleman-guardian-angel/install.sh"},
			},
		},
		{
			name:    "windows cleans temp dir and uses git bash",
			profile: system.PlatformProfile{OS: "windows", PackageManager: "winget"},
			want: [][]string{
				{"powershell", "-NoProfile", "-Command", fmt.Sprintf("Remove-Item -Recurse -Force -ErrorAction SilentlyContinue '%s'; exit 0", cloneDst)},
				{"git", "clone", "https://github.com/Gentleman-Programming/gentleman-guardian-angel.git", cloneDst},
				{bash, scriptPath},
			},
		},
		{
			name:    "fedora uses git clone and install.sh",
			profile: system.PlatformProfile{OS: "linux", LinuxDistro: system.LinuxDistroFedora, PackageManager: "dnf"},
			want: [][]string{
				{"rm", "-rf", "/tmp/gentleman-guardian-angel"},
				{"git", "clone", "https://github.com/Gentleman-Programming/gentleman-guardian-angel.git", "/tmp/gentleman-guardian-angel"},
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

func TestShouldInstall(t *testing.T) {
	if !ShouldInstall(true) {
		t.Fatalf("ShouldInstall(true) = false")
	}

	if ShouldInstall(false) {
		t.Fatalf("ShouldInstall(false) = true")
	}
}
