package kilocode

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/components/filemerge"
	"github.com/gentleman-programming/gentle-ai/v2/internal/model"
	"github.com/gentleman-programming/gentle-ai/v2/internal/system"
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
	want := filepath.Join("/home/user", ".config", "kilo", "kilo.jsonc")
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
	want := filepath.Join("/home/user", ".config", "kilo", "kilo.jsonc")
	if got := a.MCPConfigPath("/home/user", "test-server"); got != want {
		t.Fatalf("MCPConfigPath() = %q, want %q", got, want)
	}
}

func TestResolveConfigPathCandidates(t *testing.T) {
	candidates := []string{
		"kilo.jsonc",
		"kilo.json",
		"opencode.jsonc",
		"opencode.json",
		"config.json",
	}

	for _, candidate := range candidates {
		t.Run("isolated candidate "+candidate, func(t *testing.T) {
			home := t.TempDir()
			configDir := filepath.Join(home, ".config", "kilo")
			if err := os.MkdirAll(configDir, 0o755); err != nil {
				t.Fatalf("MkdirAll() error = %v", err)
			}
			targetFile := filepath.Join(configDir, candidate)
			if err := os.WriteFile(targetFile, []byte("{}"), 0o644); err != nil {
				t.Fatalf("WriteFile() error = %v", err)
			}

			if got := ResolveConfigPath(home); got != targetFile {
				t.Fatalf("ResolveConfigPath() = %q, want %q", got, targetFile)
			}
		})
	}

	t.Run("default when no candidate exists", func(t *testing.T) {
		home := t.TempDir()
		want := filepath.Join(home, ".config", "kilo", "kilo.jsonc")
		if got := ResolveConfigPath(home); got != want {
			t.Fatalf("ResolveConfigPath() = %q, want %q", got, want)
		}
	})

	t.Run("skip directory matching candidate name", func(t *testing.T) {
		home := t.TempDir()
		configDir := filepath.Join(home, ".config", "kilo")
		if err := os.MkdirAll(filepath.Join(configDir, "kilo.jsonc"), 0o755); err != nil {
			t.Fatalf("MkdirAll() error = %v", err)
		}
		opencodeJSON := filepath.Join(configDir, "opencode.json")
		if err := os.WriteFile(opencodeJSON, []byte("{}"), 0o644); err != nil {
			t.Fatalf("WriteFile() error = %v", err)
		}

		if got := ResolveConfigPath(home); got != opencodeJSON {
			t.Fatalf("ResolveConfigPath() = %q, want regular file %q", got, opencodeJSON)
		}
	})
}

func TestResolveConfigPathPrecedence(t *testing.T) {
	tests := []struct {
		name       string
		staged     []string
		wantChosen string
	}{
		{
			name:       "all candidates staged chooses kilo.jsonc",
			staged:     []string{"kilo.jsonc", "kilo.json", "opencode.jsonc", "opencode.json", "config.json"},
			wantChosen: "kilo.jsonc",
		},
		{
			name:       "kilo.json beats opencode and config candidates",
			staged:     []string{"kilo.json", "opencode.jsonc", "opencode.json", "config.json"},
			wantChosen: "kilo.json",
		},
		{
			name:       "opencode.jsonc beats opencode.json and config.json",
			staged:     []string{"opencode.jsonc", "opencode.json", "config.json"},
			wantChosen: "opencode.jsonc",
		},
		{
			name:       "opencode.json beats config.json",
			staged:     []string{"opencode.json", "config.json"},
			wantChosen: "opencode.json",
		},
		{
			name:       "config.json matches when alone",
			staged:     []string{"config.json"},
			wantChosen: "config.json",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			home := t.TempDir()
			configDir := filepath.Join(home, ".config", "kilo")
			if err := os.MkdirAll(configDir, 0o755); err != nil {
				t.Fatalf("MkdirAll() error = %v", err)
			}
			for _, file := range tt.staged {
				if err := os.WriteFile(filepath.Join(configDir, file), []byte("{}"), 0o644); err != nil {
					t.Fatalf("WriteFile(%q) error = %v", file, err)
				}
			}

			wantPath := filepath.Join(configDir, tt.wantChosen)
			if got := ResolveConfigPath(home); got != wantPath {
				t.Fatalf("ResolveConfigPath() = %q, want %q", got, wantPath)
			}
		})
	}
}

func TestSettingsPathAndMCPConfigPathIdentical(t *testing.T) {
	adapter := NewAdapter()
	candidates := []string{
		"kilo.jsonc",
		"kilo.json",
		"opencode.jsonc",
		"opencode.json",
		"config.json",
		"", // no file
	}

	for _, candidate := range candidates {
		t.Run("identical target for candidate "+candidate, func(t *testing.T) {
			home := t.TempDir()
			configDir := filepath.Join(home, ".config", "kilo")
			if candidate != "" {
				if err := os.MkdirAll(configDir, 0o755); err != nil {
					t.Fatalf("MkdirAll() error = %v", err)
				}
				if err := os.WriteFile(filepath.Join(configDir, candidate), []byte("{}"), 0o644); err != nil {
					t.Fatalf("WriteFile() error = %v", err)
				}
			}

			settings := adapter.SettingsPath(home)
			mcp := adapter.MCPConfigPath(home, "context7")

			if settings != mcp {
				t.Fatalf("SettingsPath() = %q != MCPConfigPath() = %q", settings, mcp)
			}
		})
	}
}

func TestKilocodeJSONCCommentPreservationAndIdempotency(t *testing.T) {
	home := t.TempDir()
	configDir := filepath.Join(home, ".config", "kilo")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}

	originalJSONC := []byte(`{
  // User comments for Kilocode settings
  "theme": "dark",
  // Existing MCP server note
  "mcp": {
    "custom-server": {
      "command": "custom-mcp"
    }
  },
  "agent": {
    "my-agent": {
      "model": "anthropic/claude-3-5-sonnet"
    }
  }
}
`)

	jsoncPath := filepath.Join(configDir, "kilo.jsonc")
	if err := os.WriteFile(jsoncPath, originalJSONC, 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	adapter := NewAdapter()
	selectedPath := adapter.SettingsPath(home)
	if selectedPath != jsoncPath {
		t.Fatalf("SettingsPath() = %q, want %q", selectedPath, jsoncPath)
	}

	overlay := []byte(`{
  "mcp": {
    "context7": {
      "command": "npx",
      "args": ["-y", "@upstash/context7-mcp"]
    }
  },
  "agent": {
    "sdd-orchestrator": {
      "model": "anthropic/claude-3-7-sonnet"
    }
  }
}`)

	firstMerge, err := filemerge.MergeJSONObjectsForPath(selectedPath, originalJSONC, overlay)
	if err != nil {
		t.Fatalf("MergeJSONObjectsForPath() error = %v", err)
	}

	firstStr := string(firstMerge)
	if !strings.Contains(firstStr, "// User comments for Kilocode settings") {
		t.Fatal("lost top comment in kilo.jsonc")
	}
	if !strings.Contains(firstStr, "// Existing MCP server note") {
		t.Fatal("lost existing server note comment in kilo.jsonc")
	}
	if !strings.Contains(firstStr, "custom-server") {
		t.Fatal("lost existing custom MCP server")
	}
	if !strings.Contains(firstStr, "my-agent") {
		t.Fatal("lost existing custom agent")
	}
	if !strings.Contains(firstStr, "context7") {
		t.Fatal("missing injected context7 MCP server")
	}
	if !strings.Contains(firstStr, "sdd-orchestrator") {
		t.Fatal("missing injected sdd-orchestrator agent")
	}

	// Test idempotency: re-merging the same overlay on the merged output yields identical result
	secondMerge, err := filemerge.MergeJSONObjectsForPath(selectedPath, firstMerge, overlay)
	if err != nil {
		t.Fatalf("second MergeJSONObjectsForPath() error = %v", err)
	}

	if !bytes.Equal(firstMerge, secondMerge) {
		t.Fatalf("idempotency violation:\nFirst:\n%s\nSecond:\n%s", string(firstMerge), string(secondMerge))
	}
}

func TestKilocodeNoParallelFileCreated(t *testing.T) {
	home := t.TempDir()
	configDir := filepath.Join(home, ".config", "kilo")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}

	existingFile := filepath.Join(configDir, "opencode.json")
	if err := os.WriteFile(existingFile, []byte(`{"theme":"light"}`), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	adapter := NewAdapter()
	selected := adapter.SettingsPath(home)
	if selected != existingFile {
		t.Fatalf("SettingsPath() = %q, want existing %q", selected, existingFile)
	}

	overlay := []byte(`{"mcp":{"context7":{"command":"context7"}}}`)
	merged, err := filemerge.MergeJSONObjectsForPath(selected, []byte(`{"theme":"light"}`), overlay)
	if err != nil {
		t.Fatalf("MergeJSONObjectsForPath() error = %v", err)
	}

	if _, err := filemerge.WriteFileAtomic(selected, merged, 0o644); err != nil {
		t.Fatalf("WriteFileAtomic() error = %v", err)
	}

	// Verify no parallel kilo.jsonc or other candidate was created
	for _, candidate := range []string{"kilo.jsonc", "kilo.json", "opencode.jsonc", "config.json"} {
		parallel := filepath.Join(configDir, candidate)
		if _, err := os.Stat(parallel); !os.IsNotExist(err) {
			t.Fatalf("parallel file %q should not exist", parallel)
		}
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
			want:    [][]string{{"npm", "install", "-g", "--ignore-scripts", "@kilocode/cli@latest"}},
		},
		{
			name:    "ubuntu resolves npm install with sudo",
			profile: system.PlatformProfile{OS: "linux", LinuxDistro: system.LinuxDistroUbuntu, PackageManager: "apt"},
			want:    [][]string{{"sudo", "npm", "install", "-g", "--ignore-scripts", "@kilocode/cli@latest"}},
		},
		{
			name:    "arch resolves npm install with sudo",
			profile: system.PlatformProfile{OS: "linux", LinuxDistro: system.LinuxDistroArch, PackageManager: "pacman"},
			want:    [][]string{{"sudo", "npm", "install", "-g", "--ignore-scripts", "@kilocode/cli@latest"}},
		},
		{
			name:    "fedora resolves npm install with sudo",
			profile: system.PlatformProfile{OS: "linux", LinuxDistro: system.LinuxDistroFedora, PackageManager: "dnf"},
			want:    [][]string{{"sudo", "npm", "install", "-g", "--ignore-scripts", "@kilocode/cli@latest"}},
		},
		{
			name:    "linux with writable npm skips sudo",
			profile: system.PlatformProfile{OS: "linux", LinuxDistro: system.LinuxDistroFedora, PackageManager: "dnf", NpmWritable: true},
			want:    [][]string{{"npm", "install", "-g", "--ignore-scripts", "@kilocode/cli@latest"}},
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
