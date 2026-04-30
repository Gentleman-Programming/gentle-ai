package kilocode

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/gentleman-programming/gentle-ai/internal/model"
	"github.com/gentleman-programming/gentle-ai/internal/system"
)

func TestDetect(t *testing.T) {
	tests := []struct {
		name            string
		lookPathPath    string
		lookPathErr     error
		stat            statResult
		wantInstalled   bool
		wantBinaryPath  string
		wantConfigPath  string
		wantConfigFound bool
		wantErr         bool
	}{
		{
			name:            "binary found and config directory found",
			lookPathPath:    "/usr/local/bin/kilo",
			stat:            statResult{isDir: true},
			wantInstalled:   true,
			wantBinaryPath:  "/usr/local/bin/kilo",
			wantConfigPath:  filepath.Join("/tmp/home", ".config", "kilo"),
			wantConfigFound: true,
		},
		{
			name:            "binary not found and config exists",
			lookPathErr:     errors.New("executable file not found"),
			stat:            statResult{isDir: true},
			wantInstalled:   false,
			wantBinaryPath:  "",
			wantConfigPath:  filepath.Join("/tmp/home", ".config", "kilo"),
			wantConfigFound: true,
		},
		{
			name:            "binary found and config not exists",
			lookPathPath:    "/usr/local/bin/kilo",
			stat:            statResult{err: os.ErrNotExist},
			wantInstalled:   true,
			wantBinaryPath:  "/usr/local/bin/kilo",
			wantConfigPath:  filepath.Join("/tmp/home", ".config", "kilo"),
			wantConfigFound: false,
		},
		{
			name:            "binary not found and config not exists",
			lookPathErr:     errors.New("executable file not found"),
			stat:            statResult{err: os.ErrNotExist},
			wantInstalled:   false,
			wantBinaryPath:  "",
			wantConfigPath:  filepath.Join("/tmp/home", ".config", "kilo"),
			wantConfigFound: false,
		},
		{
			name:    "stat error bubbles up",
			stat:    statResult{err: errors.New("permission denied")},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := &Adapter{
				lookPath: func(string) (string, error) {
					return tt.lookPathPath, tt.lookPathErr
				},
				statPath: func(string) statResult {
					return tt.stat
				},
			}

			installed, binaryPath, configPath, configFound, err := a.Detect(context.Background(), "/tmp/home")
			if (err != nil) != tt.wantErr {
				t.Fatalf("Detect() error = %v, wantErr %v", err, tt.wantErr)
			}

			if tt.wantErr {
				return
			}

			if installed != tt.wantInstalled {
				t.Fatalf("Detect() installed = %v, want %v", installed, tt.wantInstalled)
			}

			if binaryPath != tt.wantBinaryPath {
				t.Fatalf("Detect() binaryPath = %q, want %q", binaryPath, tt.wantBinaryPath)
			}

			if configPath != tt.wantConfigPath {
				t.Fatalf("Detect() configPath = %q, want %q", configPath, tt.wantConfigPath)
			}

			if configFound != tt.wantConfigFound {
				t.Fatalf("Detect() configFound = %v, want %v", configFound, tt.wantConfigFound)
			}
		})
	}
}

func TestGlobalConfigDir(t *testing.T) {
	a := NewAdapter()
	want := filepath.Join("/home/user", ".config", "kilo")
	if got := a.GlobalConfigDir("/home/user"); got != want {
		t.Fatalf("GlobalConfigDir() = %q, want %q", got, want)
	}
}

func TestSystemPromptFile(t *testing.T) {
	a := NewAdapter()
	want := filepath.Join("/home/user", ".config", "kilo", "AGENTS.md")
	if got := a.SystemPromptFile("/home/user"); got != want {
		t.Fatalf("SystemPromptFile() = %q, want %q", got, want)
	}
}

func TestSkillsDir(t *testing.T) {
	a := NewAdapter()
	want := filepath.Join("/home/user", ".config", "kilo", "skills")
	if got := a.SkillsDir("/home/user"); got != want {
		t.Fatalf("SkillsDir() = %q, want %q", got, want)
	}
}

func TestSettingsPath(t *testing.T) {
	a := NewAdapter()
	want := filepath.Join("/home/user", ".config", "kilo", "opencode.json")
	if got := a.SettingsPath("/home/user"); got != want {
		t.Fatalf("SettingsPath() = %q, want %q", got, want)
	}
}

func TestCapabilities(t *testing.T) {
	a := NewAdapter()

	tests := []struct {
		name   string
		method func() bool
		want   bool
	}{
		{"SupportsSkills", a.SupportsSkills, true},
		{"SupportsMCP", a.SupportsMCP, true},
		{"SupportsSystemPrompt", a.SupportsSystemPrompt, true},
		{"SupportsSlashCommands", a.SupportsSlashCommands, true},
		{"SupportsOutputStyles", a.SupportsOutputStyles, false},
		{"SupportsAutoInstall", a.SupportsAutoInstall, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.method(); got != tt.want {
				t.Fatalf("%s() = %v, want %v", tt.name, got, tt.want)
			}
		})
	}
}

func TestStrategies(t *testing.T) {
	a := NewAdapter()

	if got := a.SystemPromptStrategy(); got != model.StrategyFileReplace {
		t.Fatalf("SystemPromptStrategy() = %v, want %v", got, model.StrategyFileReplace)
	}

	if got := a.MCPStrategy(); got != model.StrategyMergeIntoSettings {
		t.Fatalf("MCPStrategy() = %v, want %v", got, model.StrategyMergeIntoSettings)
	}
}

func TestCommandsDir(t *testing.T) {
	a := NewAdapter()
	want := filepath.Join("/home/user", ".config", "kilo", "commands")
	if got := a.CommandsDir("/home/user"); got != want {
		t.Fatalf("CommandsDir() = %q, want %q", got, want)
	}
}

func TestMCPConfigPath(t *testing.T) {
	a := NewAdapter()
	want := filepath.Join("/home/user", ".config", "kilo", "opencode.json")
	if got := a.MCPConfigPath("/home/user", "test-server"); got != want {
		t.Fatalf("MCPConfigPath() = %q, want %q", got, want)
	}
}

func TestInstallCommand(t *testing.T) {
	a := NewAdapter()

	tests := []struct {
		name    string
		profile system.PlatformProfile
		want    [][]string
	}{
		{
			name:    "darwin resolves npm install without sudo",
			profile: system.PlatformProfile{OS: "darwin", PackageManager: "brew"},
			want:    [][]string{{"npm", "install", "-g", "@kilocode/cli"}},
		},
		{
			name:    "ubuntu resolves npm install with sudo",
			profile: system.PlatformProfile{OS: "linux", LinuxDistro: system.LinuxDistroUbuntu, PackageManager: "apt"},
			want:    [][]string{{"sudo", "npm", "install", "-g", "@kilocode/cli"}},
		},
		{
			name:    "arch resolves npm install with sudo",
			profile: system.PlatformProfile{OS: "linux", LinuxDistro: system.LinuxDistroArch, PackageManager: "pacman"},
			want:    [][]string{{"sudo", "npm", "install", "-g", "@kilocode/cli"}},
		},
		{
			name:    "fedora resolves npm install with sudo",
			profile: system.PlatformProfile{OS: "linux", LinuxDistro: system.LinuxDistroFedora, PackageManager: "dnf"},
			want:    [][]string{{"sudo", "npm", "install", "-g", "@kilocode/cli"}},
		},
		{
			name:    "linux with writable npm skips sudo",
			profile: system.PlatformProfile{OS: "linux", LinuxDistro: system.LinuxDistroFedora, PackageManager: "dnf", NpmWritable: true},
			want:    [][]string{{"npm", "install", "-g", "@kilocode/cli"}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			command, err := a.InstallCommand(tt.profile)
			if err != nil {
				t.Fatalf("InstallCommand() unexpected error = %v", err)
			}

			if !reflect.DeepEqual(command, tt.want) {
				t.Fatalf("InstallCommand() = %v, want %v", command, tt.want)
			}
		})
	}
}
