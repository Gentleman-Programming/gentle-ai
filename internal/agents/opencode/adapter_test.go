package opencode

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/gentleman-programming/gentle-ai/internal/model"
	"github.com/gentleman-programming/gentle-ai/internal/system"
	"github.com/gentleman-programming/gentle-ai/internal/versions"
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
			name:            "binary and config directory found",
			lookPathPath:    "/opt/homebrew/bin/opencode",
			stat:            statResult{isDir: true},
			wantInstalled:   true,
			wantBinaryPath:  "/opt/homebrew/bin/opencode",
			wantConfigPath:  filepath.Join("/tmp/home", ".config", "opencode"),
			wantConfigFound: true,
		},
		{
			name:            "binary missing and config missing",
			lookPathErr:     errors.New("missing"),
			stat:            statResult{err: os.ErrNotExist},
			wantInstalled:   false,
			wantBinaryPath:  "",
			wantConfigPath:  filepath.Join("/tmp/home", ".config", "opencode"),
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

func TestInstallCommand(t *testing.T) {
	a := NewAdapter()

	tests := []struct {
		name    string
		profile system.PlatformProfile
		want    [][]string
		wantErr bool
	}{
		{
			name:    "darwin resolves official anomalyco brew tap",
			profile: system.PlatformProfile{OS: "darwin", PackageManager: "brew"},
			want:    [][]string{{"brew", "install", "anomalyco/tap/opencode"}},
		},
		{
			name:    "ubuntu resolves npm install",
			profile: system.PlatformProfile{OS: "linux", LinuxDistro: system.LinuxDistroUbuntu, PackageManager: "apt"},
			want:    [][]string{{"sudo", "npm", "install", "-g", "--ignore-scripts", "opencode-ai@" + versions.OpenCode}},
		},
		{
			name:    "arch resolves npm install",
			profile: system.PlatformProfile{OS: "linux", LinuxDistro: system.LinuxDistroArch, PackageManager: "pacman"},
			want:    [][]string{{"sudo", "npm", "install", "-g", "--ignore-scripts", "opencode-ai@" + versions.OpenCode}},
		},
		{
			name:    "fedora resolves npm install",
			profile: system.PlatformProfile{OS: "linux", LinuxDistro: system.LinuxDistroFedora, PackageManager: "dnf"},
			want:    [][]string{{"sudo", "npm", "install", "-g", "--ignore-scripts", "opencode-ai@" + versions.OpenCode}},
		},
		{
			name:    "fedora with writable npm skips sudo",
			profile: system.PlatformProfile{OS: "linux", LinuxDistro: system.LinuxDistroFedora, PackageManager: "dnf", NpmWritable: true},
			want:    [][]string{{"npm", "install", "-g", "--ignore-scripts", "opencode-ai@" + versions.OpenCode}},
		},
		{
			name:    "unsupported package manager returns error",
			profile: system.PlatformProfile{OS: "linux", LinuxDistro: system.LinuxDistroUbuntu, PackageManager: "zypper"},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			command, err := a.InstallCommand(tt.profile)
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

func TestAdapterIdentityAndCapabilities(t *testing.T) {
	a := NewAdapter()

	if a.Agent() != model.AgentOpenCode {
		t.Errorf("Agent() = %q, want %q", a.Agent(), model.AgentOpenCode)
	}

	if a.Tier() != model.TierFull {
		t.Errorf("Tier() = %q, want %q", a.Tier(), model.TierFull)
	}

	if !a.SupportsAutoInstall() {
		t.Error("SupportsAutoInstall() should be true")
	}

	if !a.SupportsSkills() {
		t.Error("SupportsSkills() should be true")
	}

	if !a.SupportsSystemPrompt() {
		t.Error("SupportsSystemPrompt() should be true")
	}

	if !a.SupportsMCP() {
		t.Error("SupportsMCP() should be true")
	}

	if a.SupportsOutputStyles() {
		t.Error("SupportsOutputStyles() should be false")
	}

	if a.OutputStyleDir("") != "" {
		t.Error("OutputStyleDir() should be empty")
	}

	if !a.SupportsSlashCommands() {
		t.Error("SupportsSlashCommands() should be true")
	}

	if a.SupportsSubAgents() {
		t.Error("SupportsSubAgents() should be false")
	}

	if a.SubAgentsDir("") != "" {
		t.Error("SubAgentsDir() should be empty")
	}

	if a.EmbeddedSubAgentsDir() != "" {
		t.Error("EmbeddedSubAgentsDir() should be empty")
	}
}

func TestAdapterConfigPathsAndStrategies(t *testing.T) {
	a := NewAdapter()
	home := "/home/user"

	if got := a.GlobalConfigDir(home); got != filepath.Join(home, ".config", "opencode") {
		t.Errorf("GlobalConfigDir = %q, want %q", got, filepath.Join(home, ".config", "opencode"))
	}

	if got := a.SystemPromptDir(home); got != filepath.Join(home, ".config", "opencode") {
		t.Errorf("SystemPromptDir = %q, want %q", got, filepath.Join(home, ".config", "opencode"))
	}

	if got := a.SystemPromptFile(home); got != filepath.Join(home, ".config", "opencode", "AGENTS.md") {
		t.Errorf("SystemPromptFile = %q, want %q", got, filepath.Join(home, ".config", "opencode", "AGENTS.md"))
	}

	if got := a.SkillsDir(home); got != filepath.Join(home, ".config", "opencode", "skills") {
		t.Errorf("SkillsDir = %q, want %q", got, filepath.Join(home, ".config", "opencode", "skills"))
	}

	if got := a.SettingsPath(home); got != filepath.Join(home, ".config", "opencode", "opencode.json") {
		t.Errorf("SettingsPath = %q, want %q", got, filepath.Join(home, ".config", "opencode", "opencode.json"))
	}

	if got := a.MCPConfigPath(home, "server"); got != filepath.Join(home, ".config", "opencode", "opencode.json") {
		t.Errorf("MCPConfigPath = %q, want %q", got, filepath.Join(home, ".config", "opencode", "opencode.json"))
	}

	if got := a.CommandsDir(home); got != filepath.Join(home, ".config", "opencode", "commands") {
		t.Errorf("CommandsDir = %q, want %q", got, filepath.Join(home, ".config", "opencode", "commands"))
	}

	if got := a.SystemPromptStrategy(); got != model.StrategyFileReplace {
		t.Errorf("SystemPromptStrategy = %v, want %v", got, model.StrategyFileReplace)
	}

	if got := a.MCPStrategy(); got != model.StrategyMergeIntoSettings {
		t.Errorf("MCPStrategy = %v, want %v", got, model.StrategyMergeIntoSettings)
	}
}

func TestConfigPathHelper(t *testing.T) {
	home := "/home/user"
	got := ConfigPath(home)
	want := filepath.Join(home, ".config", "opencode")
	if got != want {
		t.Errorf("ConfigPath = %q, want %q", got, want)
	}
}

func TestAdapterInstallCommand_NilResolver(t *testing.T) {
	a := &Adapter{
		resolver: nil,
	}

	profile := system.PlatformProfile{OS: "darwin", PackageManager: "brew"}
	got, err := a.InstallCommand(profile)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := [][]string{{"brew", "install", "anomalyco/tap/opencode"}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("InstallCommand with nil resolver = %v, want %v", got, want)
	}
}

func TestDefaultStat(t *testing.T) {
	t.Run("non-existent file", func(t *testing.T) {
		res := defaultStat("/nonexistent/file/path")
		if res.err == nil {
			t.Error("expected error for non-existent file")
		}
	})

	t.Run("existing directory", func(t *testing.T) {
		tmpDir := t.TempDir()
		res := defaultStat(tmpDir)
		if res.err != nil {
			t.Fatalf("unexpected error: %v", res.err)
		}
		if !res.isDir {
			t.Error("expected isDir to be true")
		}
	})
}
