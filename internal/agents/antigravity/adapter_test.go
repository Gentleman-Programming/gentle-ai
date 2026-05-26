package antigravity

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/gentleman-programming/gentle-ai/internal/model"
	"github.com/gentleman-programming/gentle-ai/internal/system"
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
			wantConfigPath:  cliDir,
			wantConfigFound: true,
		},
		{
			name:            "Desktop dir found, no CLI",
			existingDirs:    []string{desktopDir},
			wantInstalled:   true,
			wantConfigPath:  desktopDir,
			wantConfigFound: true,
		},
		{
			name:            "both exist, prefers Desktop",
			existingDirs:    []string{cliDir, desktopDir},
			wantInstalled:   true,
			wantConfigPath:  desktopDir,
			wantConfigFound: true,
		},
		{
			name:            "neither exists, falls back to CLI path",
			existingDirs:    []string{},
			wantInstalled:   false,
			wantConfigPath:  cliDir,
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

func TestConfigPathsCLIOnly(t *testing.T) {
	home := t.TempDir()
	cliDir := filepath.Join(home, ".gemini", "antigravity-cli")
	a := &Adapter{statPath: makeStatFn(cliDir)}

	if got := a.GlobalConfigDir(home); got != cliDir {
		t.Fatalf("GlobalConfigDir() = %q, want %q", got, cliDir)
	}
	if got := a.SkillsDir(home); got != filepath.Join(cliDir, "skills") {
		t.Fatalf("SkillsDir() = %q, want %q", got, filepath.Join(cliDir, "skills"))
	}
	if got := a.SettingsPath(home); got != filepath.Join(cliDir, "settings.json") {
		t.Fatalf("SettingsPath() = %q, want %q", got, filepath.Join(cliDir, "settings.json"))
	}
	if got := a.MCPConfigPath(home, "ctx7"); got != filepath.Join(cliDir, "mcp_config.json") {
		t.Fatalf("MCPConfigPath() = %q, want %q", got, filepath.Join(cliDir, "mcp_config.json"))
	}
}

func TestConfigPathsDesktopOnly(t *testing.T) {
	home := t.TempDir()
	desktopDir := filepath.Join(home, ".gemini", "antigravity-desktop")
	a := &Adapter{statPath: makeStatFn(desktopDir)}

	if got := a.GlobalConfigDir(home); got != desktopDir {
		t.Fatalf("GlobalConfigDir() = %q, want %q", got, desktopDir)
	}
	if got := a.SkillsDir(home); got != filepath.Join(desktopDir, "skills") {
		t.Fatalf("SkillsDir() = %q, want %q", got, filepath.Join(desktopDir, "skills"))
	}
	if got := a.SettingsPath(home); got != filepath.Join(desktopDir, "settings.json") {
		t.Fatalf("SettingsPath() = %q, want %q", got, filepath.Join(desktopDir, "settings.json"))
	}
	if got := a.MCPConfigPath(home, "ctx7"); got != filepath.Join(desktopDir, "mcp_config.json") {
		t.Fatalf("MCPConfigPath() = %q, want %q", got, filepath.Join(desktopDir, "mcp_config.json"))
	}
}

func TestConfigPathsBothExistPrefersDesktop(t *testing.T) {
	home := t.TempDir()
	cliDir := filepath.Join(home, ".gemini", "antigravity-cli")
	desktopDir := filepath.Join(home, ".gemini", "antigravity-desktop")
	a := &Adapter{statPath: makeStatFn(cliDir, desktopDir)}

	if got := a.GlobalConfigDir(home); got != desktopDir {
		t.Fatalf("GlobalConfigDir() = %q, want %q (should prefer Desktop)", got, desktopDir)
	}
	if got := a.SkillsDir(home); got != filepath.Join(desktopDir, "skills") {
		t.Fatalf("SkillsDir() = %q, want %q", got, filepath.Join(desktopDir, "skills"))
	}
}

func TestConfigPathsNeitherExistsFallsBackToCLI(t *testing.T) {
	home := t.TempDir()
	cliDir := filepath.Join(home, ".gemini", "antigravity-cli")
	a := &Adapter{statPath: makeStatFn()}

	if got := a.GlobalConfigDir(home); got != cliDir {
		t.Fatalf("GlobalConfigDir() = %q, want %q (should fall back to CLI)", got, cliDir)
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

func TestSupportsAutoInstall(t *testing.T) {
	a := NewAdapter()

	if a.SupportsAutoInstall() {
		t.Fatal("SupportsAutoInstall() = true, want false for Antigravity")
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
	if a.SupportsSubAgents() {
		t.Fatal("SupportsSubAgents() = true, want false")
	}
	if got := a.OutputStyleDir("/tmp/home"); got != "" {
		t.Fatalf("OutputStyleDir() = %q, want empty string", got)
	}
	if got := a.CommandsDir("/tmp/home"); got != "" {
		t.Fatalf("CommandsDir() = %q, want empty string", got)
	}
	if got := a.SubAgentsDir("/tmp/home"); got != "" {
		t.Fatalf("SubAgentsDir() = %q, want empty string", got)
	}
	if got := a.EmbeddedSubAgentsDir(); got != "" {
		t.Fatalf("EmbeddedSubAgentsDir() = %q, want empty string", got)
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
