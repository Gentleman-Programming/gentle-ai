package installcmd

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/internal/model"
	"github.com/gentleman-programming/gentle-ai/internal/system"
)

func TestValidateGoForModuleInstall(t *testing.T) {
	tests := []struct {
		name        string
		profile     system.PlatformProfile
		lookPath    func(string) (string, error)
		goVersion   func() ([]byte, error)
		env         map[string]string
		wantErr     bool
		errContains string
	}{
		{
			name:    "go not in PATH returns error mentioning Go 1.24+",
			profile: system.PlatformProfile{OS: "linux", PackageManager: "apt"},
			lookPath: func(file string) (string, error) {
				return "", fmt.Errorf("not found")
			},
			goVersion:   func() ([]byte, error) { return nil, nil },
			env:         map[string]string{},
			wantErr:     true,
			errContains: "Go 1.24+",
		},
		{
			name:    "go version below 1.24 returns error",
			profile: system.PlatformProfile{OS: "linux", PackageManager: "apt"},
			lookPath: func(file string) (string, error) {
				return "/usr/bin/go", nil
			},
			goVersion:   func() ([]byte, error) { return []byte("go version go1.21.0 linux/amd64"), nil },
			env:         map[string]string{},
			wantErr:     true,
			errContains: "Go 1.24+",
		},
		{
			name:    "go version 1.23 returns error",
			profile: system.PlatformProfile{OS: "linux", PackageManager: "apt"},
			lookPath: func(file string) (string, error) {
				return "/usr/bin/go", nil
			},
			goVersion:   func() ([]byte, error) { return []byte("go version go1.23.5 linux/amd64"), nil },
			env:         map[string]string{},
			wantErr:     true,
			errContains: "Go 1.24+",
		},
		{
			name:    "GO111MODULE=off on linux returns error with export fix",
			profile: system.PlatformProfile{OS: "linux", PackageManager: "apt"},
			lookPath: func(file string) (string, error) {
				return "/usr/bin/go", nil
			},
			goVersion:   func() ([]byte, error) { return []byte("go version go1.24.0 linux/amd64"), nil },
			env:         map[string]string{"GO111MODULE": "off"},
			wantErr:     true,
			errContains: "export GO111MODULE=on",
		},
		{
			name:    "GO111MODULE=off on windows returns error with powershell fix",
			profile: system.PlatformProfile{OS: "windows", PackageManager: "winget"},
			lookPath: func(file string) (string, error) {
				return `C:\Go\bin\go.exe`, nil
			},
			goVersion:   func() ([]byte, error) { return []byte("go version go1.24.0 windows/amd64"), nil },
			env:         map[string]string{"GO111MODULE": "off"},
			wantErr:     true,
			errContains: "$env:GO111MODULE",
		},
		{
			name:    "go 1.24 without GO111MODULE=off succeeds",
			profile: system.PlatformProfile{OS: "linux", PackageManager: "apt"},
			lookPath: func(file string) (string, error) {
				return "/usr/bin/go", nil
			},
			goVersion: func() ([]byte, error) { return []byte("go version go1.24.0 linux/amd64"), nil },
			env:       map[string]string{},
			wantErr:   false,
		},
		{
			name:    "go 1.25 succeeds",
			profile: system.PlatformProfile{OS: "linux", PackageManager: "apt"},
			lookPath: func(file string) (string, error) {
				return "/usr/bin/go", nil
			},
			goVersion: func() ([]byte, error) { return []byte("go version go1.25.0 linux/amd64"), nil },
			env:       map[string]string{},
			wantErr:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			origLookPath := cmdLookPath
			origGoVersion := cmdGoVersion
			origGetenv := osGetenv
			cmdLookPath = tt.lookPath
			cmdGoVersion = tt.goVersion
			osGetenv = func(key string) string { return tt.env[key] }
			t.Cleanup(func() {
				cmdLookPath = origLookPath
				cmdGoVersion = origGoVersion
				osGetenv = origGetenv
			})

			err := validateGoForModuleInstall(tt.profile)
			if (err != nil) != tt.wantErr {
				t.Fatalf("validateGoForModuleInstall() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr && !strings.Contains(err.Error(), tt.errContains) {
				t.Errorf("error = %q, want it to contain %q", err.Error(), tt.errContains)
			}
		})
	}
}

func TestResolveEngramBrewBypassesGoValidation(t *testing.T) {
	// On macOS, brew manages Go — validation must be skipped entirely.
	origLookPath := cmdLookPath
	cmdLookPath = func(file string) (string, error) {
		return "", fmt.Errorf("go not found")
	}
	t.Cleanup(func() { cmdLookPath = origLookPath })

	profile := system.PlatformProfile{OS: "darwin", PackageManager: "brew"}
	cmds, err := resolveEngramInstall(profile)
	if err != nil {
		t.Fatalf("resolveEngramInstall() unexpected error = %v", err)
	}
	if len(cmds) == 0 {
		t.Fatal("resolveEngramInstall() returned empty CommandSequence")
	}
}

func TestResolveDependencyInstall(t *testing.T) {
	r := NewResolver()

	tests := []struct {
		name    string
		profile system.PlatformProfile
		dep     string
		want    CommandSequence
		wantErr bool
	}{
		{
			name:    "darwin resolves brew command",
			profile: system.PlatformProfile{OS: "darwin", PackageManager: "brew"},
			dep:     "somepkg",
			want:    CommandSequence{{"brew", "install", "somepkg"}},
		},
		{
			name:    "ubuntu resolves apt command",
			profile: system.PlatformProfile{OS: "linux", LinuxDistro: system.LinuxDistroUbuntu, PackageManager: "apt"},
			dep:     "somepkg",
			want:    CommandSequence{{"sudo", "apt-get", "install", "-y", "somepkg"}},
		},
		{
			name:    "arch resolves pacman command",
			profile: system.PlatformProfile{OS: "linux", LinuxDistro: system.LinuxDistroArch, PackageManager: "pacman"},
			dep:     "somepkg",
			want:    CommandSequence{{"sudo", "pacman", "-S", "--noconfirm", "somepkg"}},
		},
		{
			name:    "fedora resolves dnf command",
			profile: system.PlatformProfile{OS: "linux", LinuxDistro: system.LinuxDistroFedora, PackageManager: "dnf"},
			dep:     "somepkg",
			want:    CommandSequence{{"sudo", "dnf", "install", "-y", "somepkg"}},
		},
		{
			name:    "windows resolves winget command",
			profile: system.PlatformProfile{OS: "windows", PackageManager: "winget"},
			dep:     "somepkg",
			want:    CommandSequence{{"winget", "install", "--id", "somepkg", "-e", "--accept-source-agreements", "--accept-package-agreements"}},
		},
		{
			name:    "unsupported package manager returns error",
			profile: system.PlatformProfile{OS: "linux", LinuxDistro: system.LinuxDistroUbuntu, PackageManager: "zypper"},
			dep:     "somepkg",
			wantErr: true,
		},
		{
			name:    "empty dependency returns error",
			profile: system.PlatformProfile{OS: "darwin", PackageManager: "brew"},
			dep:     "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			command, err := r.ResolveDependencyInstall(tt.profile, tt.dep)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ResolveDependencyInstall() error = %v, wantErr %v", err, tt.wantErr)
			}

			if tt.wantErr {
				return
			}

			if !reflect.DeepEqual(command, tt.want) {
				t.Fatalf("ResolveDependencyInstall() = %v, want %v", command, tt.want)
			}
		})
	}
}

func TestGitBashPathResolvesFromGitOnPath(t *testing.T) {
	// Create a fake directory structure mimicking Git for Windows layout:
	// tmpdir/cmd/git.exe  (git binary)
	// tmpdir/bin/bash.exe (git bash)
	tmpDir := t.TempDir()
	cmdDir := filepath.Join(tmpDir, "cmd")
	binDir := filepath.Join(tmpDir, "bin")
	if err := os.MkdirAll(cmdDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatal(err)
	}

	fakeGit := filepath.Join(cmdDir, "git.exe")
	if err := os.WriteFile(fakeGit, []byte("fake"), 0o755); err != nil {
		t.Fatal(err)
	}
	fakeBash := filepath.Join(binDir, "bash.exe")
	if err := os.WriteFile(fakeBash, []byte("fake"), 0o755); err != nil {
		t.Fatal(err)
	}

	// Override cmdLookPath to return our fake git.
	original := cmdLookPath
	cmdLookPath = func(file string) (string, error) {
		if file == "git" {
			return fakeGit, nil
		}
		return "", fmt.Errorf("not found")
	}
	t.Cleanup(func() { cmdLookPath = original })

	got := gitBashPath()
	if got != fakeBash {
		t.Fatalf("gitBashPath() = %q, want %q", got, fakeBash)
	}
}

func TestGitBashPathFallsBackToBareWhenNoGit(t *testing.T) {
	origLookPath := cmdLookPath
	cmdLookPath = func(file string) (string, error) {
		return "", fmt.Errorf("not found")
	}
	t.Cleanup(func() { cmdLookPath = origLookPath })

	origStat := osStat
	osStat = func(name string) (os.FileInfo, error) {
		return nil, fmt.Errorf("not found")
	}
	t.Cleanup(func() { osStat = origStat })

	got := gitBashPath()
	if got != "bash" {
		t.Fatalf("gitBashPath() = %q, want %q", got, "bash")
	}
}

func TestBashScriptPathWindowsUsesForwardSlashes(t *testing.T) {
	profile := system.PlatformProfile{OS: "windows", PackageManager: "winget"}
	got := bashScriptPath(profile, `C:\Users\jorge\AppData\Local\Temp\gentleman-guardian-angel\install.sh`)
	want := "C:/Users/jorge/AppData/Local/Temp/gentleman-guardian-angel/install.sh"
	if got != want {
		t.Fatalf("bashScriptPath() = %q, want %q", got, want)
	}
}

func TestResolveAgentInstall(t *testing.T) {
	r := NewResolver()

	tests := []struct {
		name    string
		profile system.PlatformProfile
		agent   model.AgentID
		want    CommandSequence
		wantErr bool
	}{
		{
			name:    "claude-code on darwin uses npm without sudo",
			profile: system.PlatformProfile{OS: "darwin", PackageManager: "brew"},
			agent:   model.AgentClaudeCode,
			want:    CommandSequence{{"npm", "install", "-g", "@anthropic-ai/claude-code"}},
		},
		{
			name:    "claude-code on linux system npm uses sudo",
			profile: system.PlatformProfile{OS: "linux", LinuxDistro: system.LinuxDistroUbuntu, PackageManager: "apt"},
			agent:   model.AgentClaudeCode,
			want:    CommandSequence{{"sudo", "npm", "install", "-g", "@anthropic-ai/claude-code"}},
		},
		{
			name:    "claude-code on linux nvm skips sudo",
			profile: system.PlatformProfile{OS: "linux", LinuxDistro: system.LinuxDistroUbuntu, PackageManager: "apt", NpmWritable: true},
			agent:   model.AgentClaudeCode,
			want:    CommandSequence{{"npm", "install", "-g", "@anthropic-ai/claude-code"}},
		},
		{
			name:    "claude-code on arch system npm uses sudo",
			profile: system.PlatformProfile{OS: "linux", LinuxDistro: system.LinuxDistroArch, PackageManager: "pacman"},
			agent:   model.AgentClaudeCode,
			want:    CommandSequence{{"sudo", "npm", "install", "-g", "@anthropic-ai/claude-code"}},
		},
		{
			name:    "claude-code on fedora nvm skips sudo",
			profile: system.PlatformProfile{OS: "linux", LinuxDistro: system.LinuxDistroFedora, PackageManager: "dnf", NpmWritable: true},
			agent:   model.AgentClaudeCode,
			want:    CommandSequence{{"npm", "install", "-g", "@anthropic-ai/claude-code"}},
		},
		{
			name:    "opencode on darwin uses official anomalyco brew tap",
			profile: system.PlatformProfile{OS: "darwin", PackageManager: "brew"},
			agent:   model.AgentOpenCode,
			want:    CommandSequence{{"brew", "install", "anomalyco/tap/opencode"}},
		},
		{
			name:    "opencode on ubuntu system npm uses sudo",
			profile: system.PlatformProfile{OS: "linux", LinuxDistro: system.LinuxDistroUbuntu, PackageManager: "apt"},
			agent:   model.AgentOpenCode,
			want:    CommandSequence{{"sudo", "npm", "install", "-g", "opencode-ai"}},
		},
		{
			name:    "opencode on ubuntu nvm skips sudo",
			profile: system.PlatformProfile{OS: "linux", LinuxDistro: system.LinuxDistroUbuntu, PackageManager: "apt", NpmWritable: true},
			agent:   model.AgentOpenCode,
			want:    CommandSequence{{"npm", "install", "-g", "opencode-ai"}},
		},
		{
			name:    "opencode on arch system npm uses sudo",
			profile: system.PlatformProfile{OS: "linux", LinuxDistro: system.LinuxDistroArch, PackageManager: "pacman"},
			agent:   model.AgentOpenCode,
			want:    CommandSequence{{"sudo", "npm", "install", "-g", "opencode-ai"}},
		},
		{
			name:    "opencode on fedora system npm uses sudo",
			profile: system.PlatformProfile{OS: "linux", LinuxDistro: system.LinuxDistroFedora, PackageManager: "dnf"},
			agent:   model.AgentOpenCode,
			want:    CommandSequence{{"sudo", "npm", "install", "-g", "opencode-ai"}},
		},
		{
			name:    "opencode on fedora nvm skips sudo",
			profile: system.PlatformProfile{OS: "linux", LinuxDistro: system.LinuxDistroFedora, PackageManager: "dnf", NpmWritable: true},
			agent:   model.AgentOpenCode,
			want:    CommandSequence{{"npm", "install", "-g", "opencode-ai"}},
		},
		{
			name:    "claude-code on windows uses npm without sudo",
			profile: system.PlatformProfile{OS: "windows", PackageManager: "winget", NpmWritable: true},
			agent:   model.AgentClaudeCode,
			want:    CommandSequence{{"npm", "install", "-g", "@anthropic-ai/claude-code"}},
		},
		{
			name:    "opencode on windows uses npm without sudo",
			profile: system.PlatformProfile{OS: "windows", PackageManager: "winget"},
			agent:   model.AgentOpenCode,
			want:    CommandSequence{{"npm", "install", "-g", "opencode-ai"}},
		},
		{
			name:    "unsupported agent returns error",
			profile: system.PlatformProfile{OS: "darwin", PackageManager: "brew"},
			agent:   "unsupported",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			command, err := r.ResolveAgentInstall(tt.profile, tt.agent)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ResolveAgentInstall() error = %v, wantErr %v", err, tt.wantErr)
			}

			if tt.wantErr {
				return
			}

			if !reflect.DeepEqual(command, tt.want) {
				t.Fatalf("ResolveAgentInstall() = %v, want %v", command, tt.want)
			}
		})
	}
}

func TestResolveComponentInstall(t *testing.T) {
	r := NewResolver()

	// Simulate a valid Go 1.24+ environment for all engram resolution tests.
	// Specific error scenarios are covered in TestValidateGoForModuleInstall.
	origGoVersion := cmdGoVersion
	origGetenv := osGetenv
	origLookPath := cmdLookPath
	cmdGoVersion = func() ([]byte, error) { return []byte("go version go1.24.0 linux/amd64"), nil }
	osGetenv = func(key string) string { return "" }
	cmdLookPath = func(file string) (string, error) {
		if file == "go" {
			return "/usr/bin/go", nil
		}
		return origLookPath(file)
	}
	t.Cleanup(func() {
		cmdGoVersion = origGoVersion
		osGetenv = origGetenv
		cmdLookPath = origLookPath
	})

	tests := []struct {
		name      string
		profile   system.PlatformProfile
		component model.ComponentID
		want      CommandSequence
		wantErr   bool
	}{
		{
			name:      "engram on darwin uses brew tap and install",
			profile:   system.PlatformProfile{OS: "darwin", PackageManager: "brew"},
			component: model.ComponentEngram,
			want:      CommandSequence{{"brew", "tap", "Gentleman-Programming/homebrew-tap"}, {"brew", "install", "engram"}},
		},
		{
			name:      "engram on ubuntu uses go install with correct module path",
			profile:   system.PlatformProfile{OS: "linux", LinuxDistro: system.LinuxDistroUbuntu, PackageManager: "apt"},
			component: model.ComponentEngram,
			want:      CommandSequence{{"env", "CGO_ENABLED=0", "go", "install", "github.com/Gentleman-Programming/engram/cmd/engram@latest"}},
		},
		{
			name:      "engram on arch uses go install with correct module path",
			profile:   system.PlatformProfile{OS: "linux", LinuxDistro: system.LinuxDistroArch, PackageManager: "pacman"},
			component: model.ComponentEngram,
			want:      CommandSequence{{"env", "CGO_ENABLED=0", "go", "install", "github.com/Gentleman-Programming/engram/cmd/engram@latest"}},
		},
		{
			name:      "engram on fedora uses go install with correct module path",
			profile:   system.PlatformProfile{OS: "linux", LinuxDistro: system.LinuxDistroFedora, PackageManager: "dnf"},
			component: model.ComponentEngram,
			want:      CommandSequence{{"env", "CGO_ENABLED=0", "go", "install", "github.com/Gentleman-Programming/engram/cmd/engram@latest"}},
		},
		{
			name:      "engram on nix uses go install",
			profile:   system.PlatformProfile{OS: "linux", PackageManager: "nix", NixFlakes: true},
			component: model.ComponentEngram,
			want:      CommandSequence{{"env", "CGO_ENABLED=0", "go", "install", "github.com/Gentleman-Programming/engram/cmd/engram@latest"}},
		},
		{
			name:      "engram on nix legacy uses go install",
			profile:   system.PlatformProfile{OS: "linux", PackageManager: "nix", NixFlakes: false},
			component: model.ComponentEngram,
			want:      CommandSequence{{"env", "CGO_ENABLED=0", "go", "install", "github.com/Gentleman-Programming/engram/cmd/engram@latest"}},
		},
		{
			name:      "gga on darwin uses brew tap and install",
			profile:   system.PlatformProfile{OS: "darwin", PackageManager: "brew"},
			component: model.ComponentGGA,
			want:      CommandSequence{{"brew", "tap", "Gentleman-Programming/homebrew-tap"}, {"brew", "install", "gga"}},
		},
		{
			name:      "gga on ubuntu uses git clone and install.sh",
			profile:   system.PlatformProfile{OS: "linux", LinuxDistro: system.LinuxDistroUbuntu, PackageManager: "apt"},
			component: model.ComponentGGA,
			want: CommandSequence{
				{"rm", "-rf", "/tmp/gentleman-guardian-angel"},
				{"git", "clone", "https://github.com/Gentleman-Programming/gentleman-guardian-angel.git", "/tmp/gentleman-guardian-angel"},
				{"bash", "/tmp/gentleman-guardian-angel/install.sh"},
			},
		},
		{
			name:      "gga on arch uses git clone and install.sh",
			profile:   system.PlatformProfile{OS: "linux", LinuxDistro: system.LinuxDistroArch, PackageManager: "pacman"},
			component: model.ComponentGGA,
			want: CommandSequence{
				{"rm", "-rf", "/tmp/gentleman-guardian-angel"},
				{"git", "clone", "https://github.com/Gentleman-Programming/gentleman-guardian-angel.git", "/tmp/gentleman-guardian-angel"},
				{"bash", "/tmp/gentleman-guardian-angel/install.sh"},
			},
		},
		{
			name:      "gga on fedora uses git clone and install.sh",
			profile:   system.PlatformProfile{OS: "linux", LinuxDistro: system.LinuxDistroFedora, PackageManager: "dnf"},
			component: model.ComponentGGA,
			want: CommandSequence{
				{"rm", "-rf", "/tmp/gentleman-guardian-angel"},
				{"git", "clone", "https://github.com/Gentleman-Programming/gentleman-guardian-angel.git", "/tmp/gentleman-guardian-angel"},
				{"bash", "/tmp/gentleman-guardian-angel/install.sh"},
			},
		},
		{
			name:      "gga on nix uses git clone and install.sh",
			profile:   system.PlatformProfile{OS: "linux", LinuxDistro: system.LinuxDistroNixos, PackageManager: "nix", NixFlakes: true},
			component: model.ComponentGGA,
			want: CommandSequence{
				{"rm", "-rf", "/tmp/gentleman-guardian-angel"},
				{"git", "clone", "https://github.com/Gentleman-Programming/gentleman-guardian-angel.git", "/tmp/gentleman-guardian-angel"},
				{"bash", "/tmp/gentleman-guardian-angel/install.sh"},
			},
		},
		{
			name:      "engram on windows uses go install",
			profile:   system.PlatformProfile{OS: "windows", PackageManager: "winget"},
			component: model.ComponentEngram,
			want:      CommandSequence{{"go", "install", "github.com/Gentleman-Programming/engram/cmd/engram@latest"}},
		},
		{
			name:      "gga on windows cleans temp dir and uses git bash",
			profile:   system.PlatformProfile{OS: "windows", PackageManager: "winget"},
			component: model.ComponentGGA,
			want: CommandSequence{
				{"powershell", "-NoProfile", "-Command", fmt.Sprintf("Remove-Item -Recurse -Force -ErrorAction SilentlyContinue '%s'; exit 0", filepath.Join(os.TempDir(), "gentleman-guardian-angel"))},
				{"git", "clone", "https://github.com/Gentleman-Programming/gentleman-guardian-angel.git", filepath.Join(os.TempDir(), "gentleman-guardian-angel")},
				{gitBashPath(), bashScriptPath(system.PlatformProfile{OS: "windows"}, filepath.Join(os.TempDir(), "gentleman-guardian-angel", "install.sh"))},
			},
		},
		{
			name:      "unsupported component returns error",
			profile:   system.PlatformProfile{OS: "darwin", PackageManager: "brew"},
			component: "unsupported",
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			command, err := r.ResolveComponentInstall(tt.profile, tt.component)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ResolveComponentInstall() error = %v, wantErr %v", err, tt.wantErr)
			}

			if tt.wantErr {
				return
			}

			if !reflect.DeepEqual(command, tt.want) {
				t.Fatalf("ResolveComponentInstall() = %v, want %v", command, tt.want)
			}
		})
	}
}

// --- Tests for Nix package manager resolution (R-NIX-005) ---

func TestResolveDependencyInstallNixFlakes(t *testing.T) {
	r := NewResolver()
	profile := system.PlatformProfile{OS: "linux", PackageManager: "nix", NixFlakes: true}

	command, err := r.ResolveDependencyInstall(profile, "git")
	if err != nil {
		t.Fatalf("ResolveDependencyInstall() unexpected error = %v", err)
	}
	if len(command) != 1 {
		t.Fatalf("ResolveDependencyInstall(nix) = %d commands, want 1", len(command))
	}
	if command[0][0] != "nix" || command[0][1] != "profile" || command[0][2] != "install" {
		t.Fatalf("ResolveDependencyInstall(nix flakes) = %v, want nix profile install", command[0])
	}
}

func TestResolveDependencyInstallNixLegacy(t *testing.T) {
	r := NewResolver()
	profile := system.PlatformProfile{OS: "linux", PackageManager: "nix", NixFlakes: false}

	command, err := r.ResolveDependencyInstall(profile, "curl")
	if err != nil {
		t.Fatalf("ResolveDependencyInstall() unexpected error = %v", err)
	}
	if len(command) != 1 {
		t.Fatalf("ResolveDependencyInstall(nix) = %d commands, want 1", len(command))
	}
	if command[0][0] != "nix-env" || command[0][1] != "-iA" {
		t.Fatalf("ResolveDependencyInstall(nix legacy) = %v, want nix-env -iA", command[0])
	}
}

func TestResolveDependencyInstallNixAnyPackage(t *testing.T) {
	r := NewResolver()
	profile := system.PlatformProfile{OS: "linux", PackageManager: "nix", NixFlakes: true}

	// Test that any package can be installed via nix
	command, err := r.ResolveDependencyInstall(profile, "python310")
	if err != nil {
		t.Fatalf("ResolveDependencyInstall() unexpected error = %v", err)
	}
	if !strings.Contains(command[0][3], "python310") {
		t.Fatalf("ResolveDependencyInstall(python310) = %v, want nix profile install nixpkgs.python310", command[0])
	}
}
