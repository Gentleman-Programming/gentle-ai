package claude

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v3/internal/components/filemerge"
	"github.com/gentleman-programming/gentle-ai/v3/internal/system"
)

// TestMergeUserConfigForcesOAuthFileTo0600OnContentChange pins gentle-ai#5006(F5):
// the OAuth-bearing ~/.claude.json must always end at 0600, tightening it if the
// file was created insecurely, even though rewriting an existing file otherwise
// preserves its current mode by default.
func TestMergeUserConfigForcesOAuthFileTo0600OnContentChange(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX permission bits are not meaningful on Windows")
	}
	home := t.TempDir()
	configPath := UserConfigPath(home)
	if err := os.WriteFile(configPath, []byte(`{"already":"set"}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, _, err := MergeUserConfig(home, []byte(`{"added":"value"}`)); err != nil {
		t.Fatalf("MergeUserConfig() error = %v", err)
	}

	info, err := os.Stat(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("mode after merge = %v, want 0600", got)
	}
}

// TestMergeUserConfigForcesOAuthFileTo0600OnIdenticalContent covers the no-op
// merge path: the file must still be tightened to 0600 even when the merged
// bytes are byte-identical to what is already on disk.
func TestMergeUserConfigForcesOAuthFileTo0600OnIdenticalContent(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX permission bits are not meaningful on Windows")
	}
	home := t.TempDir()
	configPath := UserConfigPath(home)
	overlay := []byte(`{"already":"set"}`)
	canonical, err := filemerge.MergeJSONObjects(nil, overlay)
	if err != nil {
		t.Fatalf("MergeJSONObjects() error = %v", err)
	}
	if err := os.WriteFile(configPath, canonical, 0o644); err != nil {
		t.Fatal(err)
	}

	if _, _, err := MergeUserConfig(home, overlay); err != nil {
		t.Fatalf("MergeUserConfig() error = %v", err)
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != string(canonical) {
		t.Fatalf("content changed = %q, want unchanged %q", data, canonical)
	}

	info, err := os.Stat(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("mode after no-op merge = %v, want 0600 enforced", got)
	}
}

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
			lookPathPath:    "/usr/local/bin/claude",
			stat:            statResult{isDir: true},
			wantInstalled:   true,
			wantBinaryPath:  "/usr/local/bin/claude",
			wantConfigPath:  filepath.Join("/tmp/home", ".claude"),
			wantConfigFound: true,
		},
		{
			name:            "binary missing and config missing",
			lookPathErr:     errors.New("missing"),
			stat:            statResult{err: os.ErrNotExist},
			wantInstalled:   false,
			wantBinaryPath:  "",
			wantConfigPath:  filepath.Join("/tmp/home", ".claude"),
			wantConfigFound: false,
		},
		{
			name:           "stat error bubbles up",
			lookPathPath:   "/usr/local/bin/claude",
			stat:           statResult{err: errors.New("permission denied")},
			wantConfigPath: "",
			wantErr:        true,
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

func TestAdapter_SubAgentCapability(t *testing.T) {
	a := NewAdapter()

	if got := a.SupportsSubAgents(); got != true {
		t.Errorf("SupportsSubAgents() = %v, want true", got)
	}

	homeDir := "/home/test"
	wantDir := filepath.Join(homeDir, ".claude", "agents")
	if got := a.SubAgentsDir(homeDir); got != wantDir {
		t.Errorf("SubAgentsDir(%q) = %q, want %q", homeDir, got, wantDir)
	}

	if got := a.EmbeddedSubAgentsDir(); got != "claude/agents" {
		t.Errorf("EmbeddedSubAgentsDir() = %q, want %q", got, "claude/agents")
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
			name:    "darwin profile uses npm without sudo",
			profile: system.PlatformProfile{OS: "darwin", PackageManager: "brew"},
			want:    [][]string{{"npm", "install", "-g", "--ignore-scripts", "@anthropic-ai/claude-code@latest"}},
		},
		{
			name:    "ubuntu profile uses sudo npm",
			profile: system.PlatformProfile{OS: "linux", LinuxDistro: system.LinuxDistroUbuntu, PackageManager: "apt"},
			want:    [][]string{{"sudo", "npm", "install", "-g", "--ignore-scripts", "@anthropic-ai/claude-code@latest"}},
		},
		{
			name:    "arch profile uses sudo npm",
			profile: system.PlatformProfile{OS: "linux", LinuxDistro: system.LinuxDistroArch, PackageManager: "pacman"},
			want:    [][]string{{"sudo", "npm", "install", "-g", "--ignore-scripts", "@anthropic-ai/claude-code@latest"}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			command, err := a.InstallCommand(tt.profile)
			if err != nil {
				t.Fatalf("InstallCommand() returned error: %v", err)
			}

			if !reflect.DeepEqual(command, tt.want) {
				t.Fatalf("InstallCommand() = %v, want %v", command, tt.want)
			}
		})
	}
}

func TestSlashCommands(t *testing.T) {
	a := NewAdapter()

	if !a.SupportsSlashCommands() {
		t.Fatal("SupportsSlashCommands() = false, want true")
	}

	got := a.CommandsDir("/home/u")
	want := filepath.Join("/home/u", ".claude", "commands")
	if got != want {
		t.Fatalf("CommandsDir() = %q, want %q", got, want)
	}
}
