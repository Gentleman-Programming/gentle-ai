package codex

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
			lookPathPath:    "/usr/local/bin/codex",
			stat:            statResult{isDir: true},
			wantInstalled:   true,
			wantBinaryPath:  "/usr/local/bin/codex",
			wantConfigPath:  filepath.Join("/tmp/home", ".codex"),
			wantConfigFound: true,
		},
		{
			name:            "binary missing and config missing",
			lookPathErr:     errors.New("missing"),
			stat:            statResult{err: os.ErrNotExist},
			wantInstalled:   false,
			wantBinaryPath:  "",
			wantConfigPath:  filepath.Join("/tmp/home", ".codex"),
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
	}{
		{
			name:    "darwin uses npm without sudo",
			profile: system.PlatformProfile{OS: "darwin", PackageManager: "brew"},
			want:    [][]string{{"npm", "install", "-g", "--ignore-scripts", "@openai/codex@" + versions.Codex}},
		},
		{
			name:    "linux system npm uses sudo",
			profile: system.PlatformProfile{OS: "linux", LinuxDistro: system.LinuxDistroUbuntu, PackageManager: "apt"},
			want:    [][]string{{"sudo", "npm", "install", "-g", "--ignore-scripts", "@openai/codex@" + versions.Codex}},
		},
		{
			name:    "linux nvm skips sudo",
			profile: system.PlatformProfile{OS: "linux", LinuxDistro: system.LinuxDistroUbuntu, PackageManager: "apt", NpmWritable: true},
			want:    [][]string{{"npm", "install", "-g", "--ignore-scripts", "@openai/codex@" + versions.Codex}},
		},
		{
			name:    "windows uses npm without sudo",
			profile: system.PlatformProfile{OS: "windows", PackageManager: "winget", NpmWritable: true},
			want:    [][]string{{"npm", "install", "-g", "--ignore-scripts", "@openai/codex@" + versions.Codex}},
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

func TestConfigPathsCrossPlatform(t *testing.T) {
	a := NewAdapter()
	home := "/tmp/home"

	if got := a.GlobalConfigDir(home); got != filepath.Join(home, ".codex") {
		t.Fatalf("GlobalConfigDir() = %q, want %q", got, filepath.Join(home, ".codex"))
	}

	if got := a.SkillsDir(home); got != filepath.Join(home, ".codex", "skills") {
		t.Fatalf("SkillsDir() = %q, want %q", got, filepath.Join(home, ".codex", "skills"))
	}

	if got := a.SystemPromptFile(home); got != filepath.Join(home, ".codex", "AGENTS.md") {
		t.Fatalf("SystemPromptFile() = %q, want %q", got, filepath.Join(home, ".codex", "AGENTS.md"))
	}

	// Codex has no settings path.
	if got := a.SettingsPath(home); got != "" {
		t.Fatalf("SettingsPath() = %q, want \"\"", got)
	}

	// RED: Codex MCP config path should now be ~/.codex/config.toml.
	want := filepath.Join(home, ".codex", "config.toml")
	if got := a.MCPConfigPath(home, "engram"); got != want {
		t.Fatalf("MCPConfigPath() = %q, want %q", got, want)
	}
	// Server name argument is ignored — always returns config.toml.
	if got := a.MCPConfigPath(home, "ctx7"); got != want {
		t.Fatalf("MCPConfigPath(ctx7) = %q, want %q (server name should be ignored)", got, want)
	}
}

// TestAdapterSystemPromptFile_UsesUppercaseAGENTSmd asserts that the system
// prompt file path uses the exact uppercase filename "AGENTS.md" that the
// codex CLI expects. Lowercase "agents.md" causes the file to be silently
// ignored on case-sensitive filesystems (Linux) — regression for #299.
func TestAdapterSystemPromptFile_UsesUppercaseAGENTSmd(t *testing.T) {
	a := NewAdapter()
	got := a.SystemPromptFile("/home/user")
	const want = "AGENTS.md"
	if filepath.Base(got) != want {
		t.Fatalf("SystemPromptFile() base = %q, want %q (codex CLI requires uppercase AGENTS.md)", filepath.Base(got), want)
	}
}

// TestAdapterSubAgentsStayFalse is a REGRESSION GUARD.
//
// Codex multi-agent delegation is config+asset driven (features.multi_agent in
// ~/.codex/config.toml + sdd-orchestrator.md capability gate). It does NOT use
// the file-based sub-agents directory mechanism that SupportsSubAgents() gates.
//
// Flipping SupportsSubAgents() to true with an empty EmbeddedSubAgentsDir()
// would cause sdd/inject.go and uninstall/service.go to call
// assets.FS.ReadDir("") — which returns the embedded root and would copy the
// entire asset tree into ~/.codex/agents/. This is catastrophic. Therefore
// SupportsSubAgents() MUST remain false indefinitely for the Codex adapter.
// Do NOT remove or relax this test without a full audit of those call sites.
func TestAdapterSubAgentsStayFalse(t *testing.T) {
	a := NewAdapter()

	if got := a.SupportsSubAgents(); got {
		t.Fatal("SupportsSubAgents() = true — MUST stay false for Codex: Codex multi-agent is config+asset driven, not file-directory driven. Flipping this flag would copy the embedded asset root into ~/.codex/agents/.")
	}
	if got := a.SubAgentsDir("/home/user"); got != "" {
		t.Fatalf("SubAgentsDir() = %q, want \"\" — must stay empty for Codex", got)
	}
	if got := a.EmbeddedSubAgentsDir(); got != "" {
		t.Fatalf("EmbeddedSubAgentsDir() = %q, want \"\" — must stay empty for Codex", got)
	}
}

func TestCapabilities(t *testing.T) {
	a := NewAdapter()

	if got := a.Agent(); got != model.AgentCodex {
		t.Fatalf("Agent() = %q, want %q", got, model.AgentCodex)
	}

	// RED: Codex now supports real MCP via ~/.codex/config.toml.
	if got := a.SupportsMCP(); !got {
		t.Fatal("SupportsMCP() = false, want true (Codex MCP via config.toml)")
	}

	// RED: Codex uses TOML strategy.
	if got := a.MCPStrategy(); got != model.StrategyTOMLFile {
		t.Fatalf("MCPStrategy() = %v, want StrategyTOMLFile", got)
	}

	if got := a.SupportsSkills(); !got {
		t.Fatal("SupportsSkills() = false, want true")
	}

	if got := a.SupportsAutoInstall(); !got {
		t.Fatal("SupportsAutoInstall() = false, want true")
	}

	if got := a.SupportsSlashCommands(); got {
		t.Fatal("SupportsSlashCommands() = true, want false")
	}

	if got := a.SupportsOutputStyles(); got {
		t.Fatal("SupportsOutputStyles() = true, want false")
	}

	if got := a.SystemPromptStrategy(); got != model.StrategyFileReplace {
		t.Fatalf("SystemPromptStrategy() = %v, want StrategyFileReplace", got)
	}
}
