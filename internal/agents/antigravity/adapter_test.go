package antigravity

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v4/internal/model"
	"github.com/gentleman-programming/gentle-ai/v4/internal/system"
)

// makeStatFn returns a statPath function that reports the given paths as existing
// directories. Any path not in the set returns os.ErrNotExist.
func makeStatFn(existingDirs ...string) func(string) statResult {
	set := make(map[string]struct{}, len(existingDirs))
	for _, d := range existingDirs {
		set[d] = struct{}{}
	}
	return func(path string) statResult {
		if _, ok := set[path]; ok {
			return statResult{isDir: true}
		}
		return statResult{err: os.ErrNotExist}
	}
}

// --- antigravityVariantDir ---

func TestAntigravityVariantDir(t *testing.T) {
	home := t.TempDir()
	cliDir := filepath.Join(home, ".gemini", "antigravity-cli")
	desktopDir := filepath.Join(home, ".gemini", "antigravity-desktop")

	tests := []struct {
		name         string
		existingDirs []string
		wantSuffix   string // last two path segments expected
	}{
		{
			name:         "only CLI dir exists resolves to CLI",
			existingDirs: []string{cliDir},
			wantSuffix:   filepath.Join(".gemini", "antigravity-cli"),
		},
		{
			name:         "only Desktop dir exists resolves to Desktop",
			existingDirs: []string{desktopDir},
			wantSuffix:   filepath.Join(".gemini", "antigravity-desktop"),
		},
		{
			name:         "both exist prefers Desktop",
			existingDirs: []string{cliDir, desktopDir},
			wantSuffix:   filepath.Join(".gemini", "antigravity-desktop"),
		},
		{
			name:         "neither exists falls back to CLI",
			existingDirs: []string{},
			wantSuffix:   filepath.Join(".gemini", "antigravity-cli"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := &Adapter{statPath: makeStatFn(tt.existingDirs...)}
			got := a.antigravityVariantDir(home)
			want := filepath.Join(home, tt.wantSuffix)
			if got != want {
				t.Fatalf("antigravityVariantDir() = %q, want %q", got, want)
			}
		})
	}
}

// --- Detect ---

func TestDetect(t *testing.T) {
	home := t.TempDir()
	cliDir := filepath.Join(home, ".gemini", "antigravity-cli")
	desktopDir := filepath.Join(home, ".gemini", "antigravity-desktop")
	sharedDir := filepath.Join(home, ".gemini", "config")

	tests := []struct {
		name            string
		existingDirs    []string
		overrideStat    func(string) statResult // optional; overrides makeStatFn when set
		wantInstalled   bool
		wantBinaryPath  string
		wantConfigPath  string
		wantConfigFound bool
		wantErr         bool
	}{
		{
			name:            "CLI dir found, no Desktop",
			existingDirs:    []string{cliDir},
			wantInstalled:   true,
			wantConfigPath:  sharedDir,
			wantConfigFound: true,
		},
		{
			name:            "Desktop dir found, no CLI",
			existingDirs:    []string{desktopDir},
			wantInstalled:   true,
			wantConfigPath:  sharedDir,
			wantConfigFound: true,
		},
		{
			name:            "both exist, prefers Desktop",
			existingDirs:    []string{cliDir, desktopDir},
			wantInstalled:   true,
			wantConfigPath:  sharedDir,
			wantConfigFound: true,
		},
		{
			name:            "neither exists, reports shared root",
			existingDirs:    []string{},
			wantInstalled:   false,
			wantConfigPath:  sharedDir,
			wantConfigFound: false,
		},
		{
			name: "stat error bubbles up",
			overrideStat: func(string) statResult {
				return statResult{err: errors.New("permission denied")}
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			statFn := makeStatFn(tt.existingDirs...)
			if tt.overrideStat != nil {
				statFn = tt.overrideStat
			}
			a := &Adapter{statPath: statFn}

			installed, binaryPath, configPath, configFound, err := a.Detect(context.Background(), home)
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

// --- Config paths ---

// TestConfigPathsUseSharedRoot pins agy 1.4's shared customization root:
// skills, MCP config and the global config dir resolve under ~/.gemini/config
// on every layout, while settings stay in the CLI-private variant directory.
func TestConfigPathsUseSharedRoot(t *testing.T) {
	home := t.TempDir()
	shared := filepath.Join(home, ".gemini", "config")
	cliDir := filepath.Join(home, ".gemini", "antigravity-cli")
	desktopDir := filepath.Join(home, ".gemini", "antigravity-desktop")
	legacy := filepath.Join(home, ".gemini", "antigravity")
	marker := filepath.Join(shared, ".migrated")

	tests := []struct {
		name         string
		existingDirs []string
		wantSettings string
	}{
		{name: "fresh home", existingDirs: nil, wantSettings: filepath.Join(cliDir, "settings.json")},
		{name: "CLI only", existingDirs: []string{cliDir}, wantSettings: filepath.Join(cliDir, "settings.json")},
		{name: "Desktop only", existingDirs: []string{desktopDir}, wantSettings: filepath.Join(desktopDir, "settings.json")},
		{name: "legacy root only", existingDirs: []string{legacy}, wantSettings: filepath.Join(cliDir, "settings.json")},
		{name: "migrated layout", existingDirs: []string{shared, marker, cliDir}, wantSettings: filepath.Join(cliDir, "settings.json")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := &Adapter{statPath: makeStatFn(tt.existingDirs...)}

			if got := a.GlobalConfigDir(home); got != shared {
				t.Fatalf("GlobalConfigDir() = %q, want %q", got, shared)
			}
			if got := a.SkillsDir(home); got != filepath.Join(shared, "skills") {
				t.Fatalf("SkillsDir() = %q, want %q", got, filepath.Join(shared, "skills"))
			}
			if got := a.MCPConfigPath(home, "ctx7"); got != filepath.Join(shared, "mcp_config.json") {
				t.Fatalf("MCPConfigPath() = %q, want %q", got, filepath.Join(shared, "mcp_config.json"))
			}
			if got := a.SettingsPath(home); got != tt.wantSettings {
				t.Fatalf("SettingsPath() = %q, want %q", got, tt.wantSettings)
			}
		})
	}
}

func TestConfigDirsListsSharedLegacyAndVariantRoots(t *testing.T) {
	home := t.TempDir()
	desktopDir := filepath.Join(home, ".gemini", "antigravity-desktop")
	want := []string{
		filepath.Join(home, ".gemini", "config"),
		filepath.Join(home, ".gemini", "antigravity"),
		filepath.Join(home, ".gemini", "antigravity-cli"),
		desktopDir,
	}
	// Both variant roots are listed whichever one exists, so doctor accepts
	// a host that only has the desktop or only has the CLI directory.
	for _, existing := range [][]string{nil, {desktopDir}} {
		a := &Adapter{statPath: makeStatFn(existing...)}
		if got := a.ConfigDirs(home); !slices.Equal(got, want) {
			t.Fatalf("ConfigDirs() with %v = %v, want %v", existing, got, want)
		}
	}
}

func TestUpstreamDetectionScenarios(t *testing.T) {
	home := t.TempDir()
	legacy := filepath.Join(home, ".gemini", "antigravity")
	config := filepath.Join(home, ".gemini", "config")
	marker := filepath.Join(config, ".migrated")
	geminiOnly := filepath.Join(home, ".gemini")
	tests := []struct {
		name     string
		existing []string
		want     bool
		root     string
	}{
		{"legacy", []string{legacy}, true, config},
		{"unified", []string{config, marker}, true, config},
		{"half migrated marker", []string{marker}, true, config},
		{"Gemini only", []string{geminiOnly}, false, config},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := &Adapter{statPath: makeStatFn(tt.existing...)}
			installed, _, _, _, err := a.Detect(context.Background(), home)
			if err != nil || installed != tt.want {
				t.Fatalf("Detect() = %v, %v, want %v", installed, err, tt.want)
			}
			if got := a.GlobalConfigDir(home); got != tt.root {
				t.Errorf("GlobalConfigDir() = %q, want %q", got, tt.root)
			}
		})
	}
}

func TestConfigPathsStaticPaths(t *testing.T) {
	// SystemPromptDir and SystemPromptFile are not variant-dependent.
	a := NewAdapter()
	home := "/tmp/home"

	if got := a.SystemPromptDir(home); got != filepath.Join(home, ".gemini") {
		t.Fatalf("SystemPromptDir() = %q, want %q", got, filepath.Join(home, ".gemini"))
	}
	if got := a.SystemPromptFile(home); got != filepath.Join(home, ".gemini", "GEMINI.md") {
		t.Fatalf("SystemPromptFile() = %q, want %q", got, filepath.Join(home, ".gemini", "GEMINI.md"))
	}
}

// --- Installation ---

func TestInstallCommand(t *testing.T) {
	a := NewAdapter()

	_, err := a.InstallCommand(system.PlatformProfile{OS: "darwin"})
	if err == nil {
		t.Fatal("InstallCommand() expected error for CLI agent, got nil")
	}

	var notInstallable AgentNotInstallableError
	if !errors.As(err, &notInstallable) {
		t.Fatalf("InstallCommand() error type = %T, want AgentNotInstallableError", err)
	}

	if notInstallable.Agent != model.AgentAntigravity {
		t.Fatalf("AgentNotInstallableError.Agent = %q, want %q", notInstallable.Agent, model.AgentAntigravity)
	}
}

// --- Capabilities ---

func TestCapabilities(t *testing.T) {
	a := NewAdapter()

	if !a.SupportsSkills() {
		t.Fatal("SupportsSkills() = false, want true")
	}
	if !a.SupportsSystemPrompt() {
		t.Fatal("SupportsSystemPrompt() = false, want true")
	}
	if !a.SupportsMCP() {
		t.Fatal("SupportsMCP() = false, want true")
	}
	if a.SupportsOutputStyles() {
		t.Fatal("SupportsOutputStyles() = true, want false")
	}
	if a.SupportsSlashCommands() {
		t.Fatal("SupportsSlashCommands() = true, want false")
	}
	if !a.SupportsSubAgents() {
		t.Fatal("SupportsSubAgents() = false, want true")
	}
	if got := a.OutputStyleDir("/tmp/home"); got != "" {
		t.Fatalf("OutputStyleDir() = %q, want empty string", got)
	}
	if got := a.CommandsDir("/tmp/home"); got != "" {
		t.Fatalf("CommandsDir() = %q, want empty string", got)
	}
	if got := a.EmbeddedSubAgentsDir(); got != "antigravity/agents" {
		t.Fatalf("EmbeddedSubAgentsDir() = %q, want antigravity/agents", got)
	}
}

// TestSubAgentsDirUsesSharedConfigRoot proves custom agents always land in
// agy's shared ~/.gemini/config/agents, whatever layout evidence exists.
func TestSubAgentsDirUsesSharedConfigRoot(t *testing.T) {
	home := t.TempDir()
	want := filepath.Join(home, ".gemini", "config", "agents")
	layouts := map[string][]string{
		"fresh home":      nil,
		"migrated marker": {filepath.Join(home, ".gemini", "config", ".migrated")},
		"legacy root":     {filepath.Join(home, ".gemini", "antigravity")},
		"cli variant":     {filepath.Join(home, ".gemini", "antigravity-cli")},
		"desktop variant": {filepath.Join(home, ".gemini", "antigravity-desktop")},
	}
	for name, existing := range layouts {
		t.Run(name, func(t *testing.T) {
			a := &Adapter{statPath: makeStatFn(existing...)}
			if got := a.SubAgentsDir(home); got != want {
				t.Fatalf("SubAgentsDir() = %q, want %q", got, want)
			}
		})
	}
}

// --- Strategies ---

func TestStrategies(t *testing.T) {
	a := NewAdapter()

	if got := a.SystemPromptStrategy(); got != model.StrategyAppendToFile {
		t.Fatalf("SystemPromptStrategy() = %v, want StrategyAppendToFile", got)
	}
	if got := a.MCPStrategy(); got != model.StrategyMCPConfigFile {
		t.Fatalf("MCPStrategy() = %v, want StrategyMCPConfigFile", got)
	}
}

// --- Identity ---

func TestIdentity(t *testing.T) {
	a := NewAdapter()

	if got := a.Agent(); got != model.AgentAntigravity {
		t.Fatalf("Agent() = %q, want %q", got, model.AgentAntigravity)
	}
	if got := a.Tier(); got != model.TierFull {
		t.Fatalf("Tier() = %q, want %q", got, model.TierFull)
	}
}
