package opencode

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/system"
)

func TestDetect(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "")
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
			want:    [][]string{{"sudo", "npm", "install", "-g", "--ignore-scripts", "opencode-ai@latest"}},
		},
		{
			name:    "arch resolves npm install",
			profile: system.PlatformProfile{OS: "linux", LinuxDistro: system.LinuxDistroArch, PackageManager: "pacman"},
			want:    [][]string{{"sudo", "npm", "install", "-g", "--ignore-scripts", "opencode-ai@latest"}},
		},
		{
			name:    "fedora resolves npm install",
			profile: system.PlatformProfile{OS: "linux", LinuxDistro: system.LinuxDistroFedora, PackageManager: "dnf"},
			want:    [][]string{{"sudo", "npm", "install", "-g", "--ignore-scripts", "opencode-ai@latest"}},
		},
		{
			name:    "fedora with writable npm skips sudo",
			profile: system.PlatformProfile{OS: "linux", LinuxDistro: system.LinuxDistroFedora, PackageManager: "dnf", NpmWritable: true},
			want:    [][]string{{"npm", "install", "-g", "--ignore-scripts", "opencode-ai@latest"}},
		},
		{
			// Issue #2499: the probe (#2493) accepts any Linux package manager
			// on PATH, so zypper resolves like every other probed manager.
			name:    "opensuse resolves npm install",
			profile: system.PlatformProfile{OS: "linux", LinuxDistro: "opensuse-leap", PackageManager: "zypper"},
			want:    [][]string{{"sudo", "npm", "install", "-g", "--ignore-scripts", "opencode-ai@latest"}},
		},
		{
			name:    "linux without package manager returns error",
			profile: system.PlatformProfile{OS: "linux", LinuxDistro: system.LinuxDistroUbuntu, PackageManager: ""},
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

func TestConfigPathsRespectXDGConfigHome(t *testing.T) {
	home := t.TempDir()
	xdg := filepath.Join(t.TempDir(), "xdg")
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("XDG_CONFIG_HOME", xdg)
	a := NewAdapter()
	wantDir := filepath.Join(xdg, "opencode")

	if got := a.GlobalConfigDir(home); got != wantDir {
		t.Fatalf("GlobalConfigDir() = %q, want %q", got, wantDir)
	}
	if got := a.SettingsPath(home); got != filepath.Join(wantDir, "opencode.json") {
		t.Fatalf("SettingsPath() = %q, want XDG path", got)
	}
	if got := a.SystemPromptFile(home); got != filepath.Join(wantDir, "AGENTS.md") {
		t.Fatalf("SystemPromptFile() = %q, want XDG path", got)
	}
	if got := a.SystemPromptDir(home); got != wantDir {
		t.Fatalf("SystemPromptDir() = %q, want %q", got, wantDir)
	}
	if got := a.SkillsDir(home); got != filepath.Join(wantDir, "skills") {
		t.Fatalf("SkillsDir() = %q, want XDG path", got)
	}
	if got := a.CommandsDir(home); got != filepath.Join(wantDir, "commands") {
		t.Fatalf("CommandsDir() = %q, want XDG path", got)
	}
	if got := a.MCPConfigPath(home, "codegraph"); got != filepath.Join(wantDir, "opencode.json") {
		t.Fatalf("MCPConfigPath() = %q, want XDG path", got)
	}
}

func TestEffectiveCodeGraphWiring(t *testing.T) {
	tests := []struct {
		name       string
		fileName   string
		content    string
		configured bool
	}{
		{name: "effective JSON", fileName: "opencode.json", content: `{"mcp":{"codegraph":{"type":"local","command":["codegraph","serve","--mcp"],"enabled":true}}}`, configured: true},
		{name: "effective JSONC", fileName: "opencode.jsonc", content: "{\n// preserved comment\n\"mcp\": {\"codegraph\": {\"type\": \"local\", \"command\": [\"codegraph\", \"serve\", \"--mcp\"],},},\n}", configured: true},
		{name: "disabled entry", fileName: "opencode.json", content: `{"mcp":{"codegraph":{"type":"local","command":["codegraph","serve","--mcp"],"enabled":false}}}`},
		{name: "wrong command", fileName: "opencode.json", content: `{"mcp":{"codegraph":{"type":"local","command":["other","serve","--mcp"]}}}`},
		{name: "malformed config", fileName: "opencode.json", content: `{"mcp":`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			t.Setenv("USERPROFILE", home)
			t.Setenv("XDG_CONFIG_HOME", "")
			path := filepath.Join(ConfigPath(home), tt.fileName)
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, []byte(tt.content), 0o600); err != nil {
				t.Fatal(err)
			}

			gotPath, configured := NewAdapter().EffectiveCodeGraphWiring(home)
			if configured != tt.configured {
				t.Fatalf("EffectiveCodeGraphWiring() configured = %v, want %v", configured, tt.configured)
			}
			if configured && gotPath != path {
				t.Fatalf("EffectiveCodeGraphWiring() path = %q, want %q", gotPath, path)
			}
		})
	}
}

func TestConfigPathIgnoresRelativeXDGConfigHome(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join("relative", "config"))
	want := filepath.Join(home, ".config", "opencode")
	if got := ConfigPath(home); got != want {
		t.Fatalf("ConfigPath() = %q, want fallback %q", got, want)
	}
}
