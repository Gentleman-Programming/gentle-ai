package hermes

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v3/internal/model"
	"github.com/gentleman-programming/gentle-ai/v3/internal/system"
)

// wantHermesHome is the expected ResolveHome result for a sandboxed homeDir
// that is never the real user home: environment overrides must not apply, so
// the result depends only on GOOS.
func wantHermesHome(homeDir string) string {
	if runtime.GOOS == "windows" {
		return filepath.Join(homeDir, "AppData", "Local", "hermes")
	}
	return filepath.Join(homeDir, ".hermes")
}

func TestDetect(t *testing.T) {
	tests := []struct {
		name            string
		lookPathPath    string
		lookPathErr     error
		stat            statResult
		wantInstalled   bool
		wantBinaryPath  string
		wantConfigFound bool
		wantErr         bool
	}{
		{
			name:            "binary and config directory found",
			lookPathPath:    "/usr/local/bin/hermes",
			stat:            statResult{isDir: true},
			wantInstalled:   true,
			wantBinaryPath:  "/usr/local/bin/hermes",
			wantConfigFound: true,
		},
		{
			name:            "binary missing config missing",
			lookPathErr:     errors.New("missing"),
			stat:            statResult{err: os.ErrNotExist},
			wantInstalled:   false,
			wantBinaryPath:  "",
			wantConfigFound: false,
		},
		{
			name:            "binary found config missing",
			lookPathPath:    "/usr/local/bin/hermes",
			stat:            statResult{err: os.ErrNotExist},
			wantInstalled:   true,
			wantBinaryPath:  "/usr/local/bin/hermes",
			wantConfigFound: false,
		},
		{
			name:            "binary missing config found",
			lookPathErr:     errors.New("missing"),
			stat:            statResult{isDir: true},
			wantInstalled:   false,
			wantBinaryPath:  "",
			wantConfigFound: true,
		},
		{
			name:    "stat error propagates",
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
			homeDir := filepath.Join(string(filepath.Separator), "home", "test")

			installed, binaryPath, configPath, configFound, err := a.Detect(context.Background(), homeDir)
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

			wantConfigPath := wantHermesHome(homeDir)
			if configPath != wantConfigPath {
				t.Fatalf("Detect() configPath = %q, want %q", configPath, wantConfigPath)
			}

			if configFound != tt.wantConfigFound {
				t.Fatalf("Detect() configFound = %v, want %v", configFound, tt.wantConfigFound)
			}
		})
	}
}

func TestInstallCommand(t *testing.T) {
	a := NewAdapter()

	commands, err := a.InstallCommand(system.PlatformProfile{})
	if err == nil {
		t.Fatalf("InstallCommand() error = nil, want non-installable error")
	}
	if commands != nil {
		t.Fatalf("InstallCommand() commands = %v, want nil", commands)
	}

	var notInstallable AgentNotInstallableError
	if !errors.As(err, &notInstallable) {
		t.Fatalf("InstallCommand() error type = %T, want AgentNotInstallableError", err)
	}
	if got := err.Error(); !strings.Contains(got, "must be installed manually") {
		t.Fatalf("InstallCommand() error = %q, want message containing 'must be installed manually'", got)
	}
}

func TestConfigPaths(t *testing.T) {
	a := NewAdapter()
	homeDir := filepath.Join(string(filepath.Separator), "home", "test")
	configDir := wantHermesHome(homeDir)
	configYAML := filepath.Join(configDir, "config.yaml")
	soulMD := filepath.Join(configDir, "SOUL.md")
	skillsDir := filepath.Join(configDir, "skills")

	tests := []struct {
		name string
		got  string
		want string
	}{
		{"GlobalConfigDir", a.GlobalConfigDir(homeDir), configDir},
		{"SystemPromptDir", a.SystemPromptDir(homeDir), configDir},
		{"SystemPromptFile", a.SystemPromptFile(homeDir), soulMD},
		{"SkillsDir", a.SkillsDir(homeDir), skillsDir},
		{"SettingsPath", a.SettingsPath(homeDir), configYAML},
		{"MCPConfigPath (engram)", a.MCPConfigPath(homeDir, "engram"), configYAML},
		{"MCPConfigPath (context7)", a.MCPConfigPath(homeDir, "context7"), configYAML},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got != tt.want {
				t.Fatalf("%s = %q, want %q", tt.name, tt.got, tt.want)
			}
		})
	}
}

func TestResolveHome(t *testing.T) {
	t.Run("sandbox home ignores HERMES_HOME", func(t *testing.T) {
		t.Setenv("HERMES_HOME", filepath.Join(t.TempDir(), "hermes"))
		homeDir := filepath.Join(string(filepath.Separator), "home", "sandbox")
		if got := ResolveHome(homeDir); got != wantHermesHome(homeDir) {
			t.Fatalf("ResolveHome(%q) = %q, want %q (ambient env must not escape a sandbox root)", homeDir, got, wantHermesHome(homeDir))
		}
	})

	t.Run("sandbox home ignores LOCALAPPDATA", func(t *testing.T) {
		t.Setenv("HERMES_HOME", "")
		t.Setenv("LOCALAPPDATA", filepath.Join(t.TempDir(), "local"))
		homeDir := filepath.Join(string(filepath.Separator), "home", "sandbox")
		if got := ResolveHome(homeDir); got != wantHermesHome(homeDir) {
			t.Fatalf("ResolveHome(%q) = %q, want %q (ambient env must not escape a sandbox root)", homeDir, got, wantHermesHome(homeDir))
		}
	})

	t.Run("real home honors HERMES_HOME", func(t *testing.T) {
		realHome, err := os.UserHomeDir()
		if err != nil {
			t.Skip("os.UserHomeDir unavailable")
		}
		want := filepath.Join(t.TempDir(), "hermes-home")
		t.Setenv("HERMES_HOME", want)
		if got := ResolveHome(realHome); got != want {
			t.Fatalf("ResolveHome(realHome) = %q, want HERMES_HOME %q", got, want)
		}
	})

	t.Run("real home ignores relative HERMES_HOME", func(t *testing.T) {
		realHome, err := os.UserHomeDir()
		if err != nil {
			t.Skip("os.UserHomeDir unavailable")
		}
		t.Setenv("HERMES_HOME", filepath.Join("relative", "hermes"))
		var want string
		if runtime.GOOS == "windows" {
			t.Setenv("LOCALAPPDATA", "")
			want = filepath.Join(realHome, "AppData", "Local", "hermes")
		} else {
			want = filepath.Join(realHome, ".hermes")
		}
		if got := ResolveHome(realHome); got != want {
			t.Fatalf("ResolveHome(realHome) = %q, want %q (relative HERMES_HOME must be ignored)", got, want)
		}
	})

	t.Run("real home honors LOCALAPPDATA on Windows", func(t *testing.T) {
		if runtime.GOOS != "windows" {
			t.Skip("Windows-only resolution")
		}
		realHome, err := os.UserHomeDir()
		if err != nil {
			t.Skip("os.UserHomeDir unavailable")
		}
		t.Setenv("HERMES_HOME", "")
		localAppData := filepath.Join(t.TempDir(), "local")
		t.Setenv("LOCALAPPDATA", localAppData)
		want := filepath.Join(localAppData, "hermes")
		if got := ResolveHome(realHome); got != want {
			t.Fatalf("ResolveHome(realHome) = %q, want %q", got, want)
		}
	})

	t.Run("ConfigPath stays POSIX", func(t *testing.T) {
		homeDir := filepath.Join(string(filepath.Separator), "home", "test")
		if got, want := ConfigPath(homeDir), filepath.Join(homeDir, ".hermes"); got != want {
			t.Fatalf("ConfigPath() = %q, want %q", got, want)
		}
	})
}

func TestCapabilities(t *testing.T) {
	a := NewAdapter()

	if got := a.Agent(); got != model.AgentHermes {
		t.Fatalf("Agent() = %q, want %q", got, model.AgentHermes)
	}

	if got := a.Tier(); got != model.TierFull {
		t.Fatalf("Tier() = %v, want TierFull", got)
	}

	if a.SupportsOutputStyles() {
		t.Fatalf("SupportsOutputStyles() = true, want false")
	}

	if got := a.OutputStyleDir("/home/test"); got != "" {
		t.Fatalf("OutputStyleDir() = %q, want empty", got)
	}

	if a.SupportsSlashCommands() {
		t.Fatalf("SupportsSlashCommands() = true, want false")
	}

	if got := a.CommandsDir("/home/test"); got != "" {
		t.Fatalf("CommandsDir() = %q, want empty", got)
	}

	if a.SupportsSubAgents() {
		t.Fatalf("SupportsSubAgents() = true, want false")
	}

	if got := a.SubAgentsDir("/home/test"); got != "" {
		t.Fatalf("SubAgentsDir() = %q, want empty", got)
	}

	if got := a.EmbeddedSubAgentsDir(); got != "" {
		t.Fatalf("EmbeddedSubAgentsDir() = %q, want empty", got)
	}

	if !a.SupportsSkills() {
		t.Fatalf("SupportsSkills() = false, want true")
	}

	if !a.SupportsSystemPrompt() {
		t.Fatalf("SupportsSystemPrompt() = false, want true")
	}

	if !a.SupportsMCP() {
		t.Fatalf("SupportsMCP() = false, want true")
	}

	if got := a.SystemPromptStrategy(); got != model.StrategyMarkdownSections {
		t.Fatalf("SystemPromptStrategy() = %v, want StrategyMarkdownSections", got)
	}

	if got := a.MCPStrategy(); got != model.StrategyMergeIntoYAML {
		t.Fatalf("MCPStrategy() = %v, want StrategyMergeIntoYAML", got)
	}
}
