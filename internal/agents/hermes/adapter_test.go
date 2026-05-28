package hermes

import (
	"context"
	"errors"
	"os"
	"path/filepath"
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
			lookPathPath:    "/usr/local/bin/hermes",
			stat:            statResult{isDir: true},
			wantInstalled:   true,
			wantBinaryPath:  "/usr/local/bin/hermes",
			wantConfigPath:  filepath.Join("/tmp/home", ".hermes"),
			wantConfigFound: true,
		},
		{
			name:            "binary missing and config missing",
			lookPathErr:     errors.New("missing"),
			stat:            statResult{err: os.ErrNotExist},
			wantInstalled:   false,
			wantBinaryPath:  "",
			wantConfigPath:  filepath.Join("/tmp/home", ".hermes"),
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
	profile := system.PlatformProfile{OS: "linux"}
	_, err := a.InstallCommand(profile)
	if err == nil {
		t.Fatal("InstallCommand() should return error for Hermes since it does not support auto-install")
	}

	var installErr AgentNotInstallableError
	if !errors.As(err, &installErr) {
		t.Fatalf("expected AgentNotInstallableError, got: %T (%v)", err, err)
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
			want: filepath.Join(home, ".hermes"),
		},
		{
			name: "SystemPromptFile",
			got:  a.SystemPromptFile(home),
			want: filepath.Join(home, ".hermes", "SOUL.md"),
		},
		{
			name: "SkillsDir",
			got:  a.SkillsDir(home),
			want: filepath.Join(home, ".hermes", "skills"),
		},
		{
			name: "SettingsPath",
			got:  a.SettingsPath(home),
			want: filepath.Join(home, ".hermes", "config.yaml"),
		},
		{
			name: "MCPConfigPath",
			got:  a.MCPConfigPath(home, "ctx7"),
			want: filepath.Join(home, ".hermes", "mcp.json"),
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
		{"SupportsMCP", a.SupportsMCP(), true},
		{"SupportsSlashCommands", a.SupportsSlashCommands(), false},
		{"SupportsOutputStyles", a.SupportsOutputStyles(), false},
		{"SupportsSubAgents", a.SupportsSubAgents(), false},
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
		{"Agent", string(a.Agent()), string(model.AgentHermes)},
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
			want: int(model.StrategyAppendToFile),
		},
		{
			name: "MCPStrategy",
			got:  int(a.MCPStrategy()),
			want: int(model.StrategyMCPConfigFile),
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
