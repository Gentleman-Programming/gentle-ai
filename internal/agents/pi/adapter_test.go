package pi

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
			name:            "binary and config directory found",
			lookPathPath:    "/usr/local/bin/pi",
			stat:            statResult{isDir: true},
			wantInstalled:   true,
			wantBinaryPath:  "/usr/local/bin/pi",
			wantConfigPath:  filepath.Join("/tmp/home", ".pi", "agent"),
			wantConfigFound: true,
		},
		{
			name:            "binary missing and config missing",
			lookPathErr:     errors.New("missing"),
			stat:            statResult{err: os.ErrNotExist},
			wantInstalled:   false,
			wantBinaryPath:  "",
			wantConfigPath:  filepath.Join("/tmp/home", ".pi", "agent"),
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

	commands, err := a.InstallCommand(system.PlatformProfile{OS: "darwin"})
	if err == nil {
		t.Fatal("InstallCommand() expected error for manual-install adapter")
	}
	if commands != nil {
		t.Fatalf("InstallCommand() commands = %v, want nil", commands)
	}
}

func TestConfigPaths(t *testing.T) {
	a := NewAdapter()
	home := "/tmp/home"

	tests := []struct {
		name string
		got  string
		want string
	}{
		{
			name: "GlobalConfigDir",
			got:  a.GlobalConfigDir(home),
			want: filepath.Join(home, ".pi", "agent"),
		},
		{
			name: "SystemPromptFile",
			got:  a.SystemPromptFile(home),
			want: filepath.Join(home, ".pi", "agent", "AGENTS.md"),
		},
		{
			name: "SkillsDir",
			got:  a.SkillsDir(home),
			want: filepath.Join(home, ".pi", "agent", "skills"),
		},
		{
			name: "SettingsPath",
			got:  a.SettingsPath(home),
			want: filepath.Join(home, ".pi", "agent", "settings.json"),
		},
		{
			name: "MCPConfigPath",
			got:  a.MCPConfigPath(home, "context7"),
			want: filepath.Join(home, ".pi", "agent", "settings.json"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got != tt.want {
				t.Errorf("%s() = %q, want %q", tt.name, tt.got, tt.want)
			}
		})
	}
}

func TestCapabilities(t *testing.T) {
	a := NewAdapter()

	tests := []struct {
		name string
		got  bool
		want bool
	}{
		{"SupportsAutoInstall", a.SupportsAutoInstall(), false},
		{"SupportsSkills", a.SupportsSkills(), true},
		{"SupportsSystemPrompt", a.SupportsSystemPrompt(), true},
		{"SupportsMCP", a.SupportsMCP(), false},
		{"SupportsSlashCommands", a.SupportsSlashCommands(), false},
		{"SupportsOutputStyles", a.SupportsOutputStyles(), false},
		{"SupportsExtensions", a.SupportsExtensions(), true},
		{"SupportsPromptTemplates", a.SupportsPromptTemplates(), true},
		{"SupportsThemes", a.SupportsThemes(), true},
		{"SupportsPackages", a.SupportsPackages(), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got != tt.want {
				t.Errorf("%s() = %v, want %v", tt.name, tt.got, tt.want)
			}
		})
	}
}

func TestAdapterIdentity(t *testing.T) {
	a := NewAdapter()

	tests := []struct {
		name string
		got  string
		want string
	}{
		{"Agent", string(a.Agent()), string(model.AgentPi)},
		{"Tier", string(a.Tier()), string(model.TierFull)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got != tt.want {
				t.Errorf("%s() = %q, want %q", tt.name, tt.got, tt.want)
			}
		})
	}
}

func TestAdapterStrategies(t *testing.T) {
	a := NewAdapter()

	tests := []struct {
		name string
		got  int
		want int
	}{
		{
			name: "SystemPromptStrategy",
			got:  int(a.SystemPromptStrategy()),
			want: int(model.StrategyMarkdownSections),
		},
		{
			name: "MCPStrategy",
			got:  int(a.MCPStrategy()),
			want: int(model.StrategyMergeIntoSettings),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got != tt.want {
				t.Errorf("%s() = %v, want %v", tt.name, tt.got, tt.want)
			}
		})
	}
}

func TestInstallCommandErrorType(t *testing.T) {
	a := NewAdapter()

	_, err := a.InstallCommand(system.PlatformProfile{})
	if err == nil {
		t.Fatal("InstallCommand() expected error")
	}

	if !reflect.DeepEqual(err.Error(), AgentNotInstallableError{Agent: model.AgentPi}.Error()) {
		t.Fatalf("InstallCommand() error = %q, want %q", err.Error(), AgentNotInstallableError{Agent: model.AgentPi}.Error())
	}
}

func TestExtendedResourcePaths(t *testing.T) {
	a := NewAdapter()
	home := "/tmp/home"

	tests := []struct {
		name string
		got  string
		want string
	}{
		{
			name: "ExtensionsDir",
			got:  a.ExtensionsDir(home),
			want: filepath.Join(home, ".pi", "agent", "extensions"),
		},
		{
			name: "PromptsDir",
			got:  a.PromptsDir(home),
			want: filepath.Join(home, ".pi", "agent", "prompts"),
		},
		{
			name: "ThemesDir",
			got:  a.ThemesDir(home),
			want: filepath.Join(home, ".pi", "agent", "themes"),
		},
		{
			name: "PackagesStatePath",
			got:  a.PackagesStatePath(home),
			want: filepath.Join(home, ".pi", "agent", "settings.json"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got != tt.want {
				t.Errorf("%s() = %q, want %q", tt.name, tt.got, tt.want)
			}
		})
	}
}
