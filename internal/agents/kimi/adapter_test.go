package kimi

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v4/internal/model"
)

func TestNewAdapter(t *testing.T) {
	a := NewAdapter()
	if a == nil {
		t.Fatal("NewAdapter() returned nil")
	}
}

func TestAdapter_Agent(t *testing.T) {
	a := NewAdapter()
	if got := a.Agent(); got != model.AgentKimi {
		t.Errorf("Agent() = %v, want %v", got, model.AgentKimi)
	}
}

func TestAdapter_Tier(t *testing.T) {
	a := NewAdapter()
	if got := a.Tier(); got != model.TierFull {
		t.Errorf("Tier() = %v, want %v", got, model.TierFull)
	}
}

func TestAdapter_ConfigPaths(t *testing.T) {
	// Stub the stat seam so the legacy fallback is deterministic regardless
	// of the host filesystem.
	a := &Adapter{
		statPath: func(string) statResult { return statResult{err: os.ErrNotExist} },
	}
	homeDir := "/home/test"

	tests := []struct {
		name     string
		got      string
		expected string
	}{
		{"GlobalConfigDir", a.GlobalConfigDir(homeDir), filepath.Join(homeDir, ".kimi")},
		{"SystemPromptDir", a.SystemPromptDir(homeDir), filepath.Join(homeDir, ".kimi")},
		{"SystemPromptFile", a.SystemPromptFile(homeDir), filepath.Join(homeDir, ".kimi", "KIMI.md")},
		{"SkillsDir", a.SkillsDir(homeDir), filepath.Join(homeDir, ".config", "agents", "skills")},
		{"SettingsPath", a.SettingsPath(homeDir), filepath.Join(homeDir, ".kimi", "config.toml")},
		{"CommandsDir", a.CommandsDir(homeDir), ""},
		{"SubAgentsDir", a.SubAgentsDir(homeDir), filepath.Join(homeDir, ".kimi", "agents")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got != tt.expected {
				t.Errorf("%s = %v, want %v", tt.name, tt.got, tt.expected)
			}
		})
	}
}

func TestAdapter_Strategies(t *testing.T) {
	a := NewAdapter()

	if got := a.SystemPromptStrategy(); got != model.StrategyJinjaModules {
		t.Errorf("SystemPromptStrategy() = %v, want StrategyJinjaModules", got)
	}

	if got := a.MCPStrategy(); got != model.StrategyMCPConfigFile {
		t.Errorf("MCPStrategy() = %v, want StrategyMCPConfigFile", got)
	}
}

func TestAdapter_Capabilities(t *testing.T) {
	a := NewAdapter()

	tests := []struct {
		name string
		got  bool
		want bool
	}{
		{"SupportsSkills", a.SupportsSkills(), true},
		{"SupportsMCP", a.SupportsMCP(), true},
		{"SupportsSystemPrompt", a.SupportsSystemPrompt(), true},
		{"SupportsSlashCommands", a.SupportsSlashCommands(), false},
		{"SupportsOutputStyles", a.SupportsOutputStyles(), false},
		{"SupportsSubAgents", a.SupportsSubAgents(), true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if tc.got != tc.want {
				t.Errorf("%s = %v, want %v", tc.name, tc.got, tc.want)
			}
		})
	}
}

func TestAdapter_EmbeddedSubAgentsDir(t *testing.T) {
	a := NewAdapter()
	if got := a.EmbeddedSubAgentsDir(); got != "kimi/agents" {
		t.Errorf("EmbeddedSubAgentsDir() = %v, want kimi/agents", got)
	}
}

func TestAdapter_MCPConfigPath(t *testing.T) {
	a := &Adapter{
		statPath: func(string) statResult { return statResult{err: os.ErrNotExist} },
	}
	homeDir := "/home/test"
	serverName := "test-server"

	got := a.MCPConfigPath(homeDir, serverName)
	expected := filepath.Join(homeDir, ".kimi", "mcp.json")

	if got != expected {
		t.Errorf("MCPConfigPath() = %v, want %v", got, expected)
	}
}

func TestAdapter_Detect_KimiInstalled(t *testing.T) {
	tmpDir := t.TempDir()
	kimiDir := filepath.Join(tmpDir, ".kimi")
	if err := os.MkdirAll(kimiDir, 0755); err != nil {
		t.Fatal(err)
	}

	a := &Adapter{
		lookPath: func(string) (string, error) {
			return "/usr/bin/kimi", nil
		},
		statPath: func(path string) statResult {
			info, err := os.Stat(path)
			return statResult{isDir: info != nil && info.IsDir(), err: err}
		},
		pathExists: func(string) bool { return false },
		userHomeDir: func() (string, error) {
			return tmpDir, nil
		},
	}

	installed, binaryPath, configPath, configFound, err := a.Detect(context.Background(), tmpDir)
	if err != nil {
		t.Fatalf("Detect() error = %v", err)
	}

	if !installed {
		t.Error("Detect() installed = false, want true")
	}
	if binaryPath != "/usr/bin/kimi" {
		t.Errorf("Detect() binaryPath = %v, want /usr/bin/kimi", binaryPath)
	}
	if !configFound {
		t.Error("Detect() configFound = false, want true")
	}
	if configPath != filepath.Join(tmpDir, ".kimi") {
		t.Errorf("Detect() configPath = %v", configPath)
	}
}

func TestAdapter_Detect_KimiNotInstalled(t *testing.T) {
	tmpDir := t.TempDir()

	a := &Adapter{
		lookPath: func(string) (string, error) {
			return "", os.ErrNotExist
		},
		statPath: func(path string) statResult {
			return statResult{err: os.ErrNotExist}
		},
		pathExists: func(string) bool { return false },
		userHomeDir: func() (string, error) {
			return tmpDir, nil
		},
	}

	installed, binaryPath, configPath, configFound, err := a.Detect(context.Background(), tmpDir)
	if err != nil {
		t.Fatalf("Detect() error = %v", err)
	}

	if installed {
		t.Error("Detect() installed = true, want false")
	}
	if binaryPath != "" {
		t.Errorf("Detect() binaryPath = %v, want empty", binaryPath)
	}
	if configFound {
		t.Error("Detect() configFound = true, want false")
	}
	if configPath != filepath.Join(tmpDir, ".kimi") {
		t.Errorf("Detect() configPath wrong: %v", configPath)
	}
}

func TestAdapter_Detect_FallbackPaths(t *testing.T) {
	tmpDir := t.TempDir()
	kimiDir := filepath.Join(tmpDir, ".kimi")
	if err := os.MkdirAll(kimiDir, 0755); err != nil {
		t.Fatal(err)
	}

	a := &Adapter{
		lookPath: func(string) (string, error) {
			return "", os.ErrNotExist // Not in PATH
		},
		statPath: func(path string) statResult {
			info, err := os.Stat(path)
			return statResult{isDir: info != nil && info.IsDir(), err: err}
		},
		pathExists: func(path string) bool {
			return path == filepath.Join(tmpDir, ".local", "bin", binaryName())
		},
		userHomeDir: func() (string, error) {
			return tmpDir, nil
		},
	}

	installed, binaryPath, _, _, err := a.Detect(context.Background(), tmpDir)
	if err != nil {
		t.Fatalf("Detect() error = %v", err)
	}
	if !installed {
		t.Fatal("Detect() installed = false, want true when fallback path exists")
	}
	if binaryPath != filepath.Join(tmpDir, ".local", "bin", binaryName()) {
		t.Fatalf("Detect() binaryPath = %q, want fallback path", binaryPath)
	}
}

func TestResolveConfigRoot(t *testing.T) {
	tests := []struct {
		name    string
		home    string
		stat    func(string) statResult
		setup   func(t *testing.T, home string)
		want    string
		current bool
	}{
		{
			name: "legacy fallback when no config dirs exist",
			home: t.TempDir(),
			want: "",
		},
		{
			name: "legacy fallback when only legacy dir exists",
			home: t.TempDir(),
			setup: func(t *testing.T, home string) {
				if err := os.MkdirAll(filepath.Join(home, ".kimi"), 0o755); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "current root preferred when .kimi-code dir exists",
			home: t.TempDir(),
			setup: func(t *testing.T, home string) {
				if err := os.MkdirAll(filepath.Join(home, ".kimi-code"), 0o755); err != nil {
					t.Fatal(err)
				}
			},
			want:    "",
			current: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.setup != nil {
				tt.setup(t, tt.home)
			}
			want := tt.want
			if tt.current {
				want = filepath.Join(tt.home, ".kimi-code")
			} else if want == "" {
				want = filepath.Join(tt.home, ".kimi")
			}
			got, layout, err := resolveConfigRoot(defaultStat, tt.home)
			if err != nil {
				t.Fatalf("resolveConfigRoot(%q) error = %v", tt.home, err)
			}
			if got != want {
				t.Errorf("resolveConfigRoot(%q) = %q, want %q", tt.home, got, want)
			}
			wantLayout := LayoutLegacy
			if tt.current {
				wantLayout = LayoutCurrent
			}
			if layout != wantLayout {
				t.Errorf("resolveConfigRoot(%q) layout = %v, want %v", tt.home, layout, wantLayout)
			}
		})
	}
}

// TestResolveConfigRoot_UnexpectedStatErrorFails verifies that stat failures
// other than absence (ENOENT/ENOTDIR) are returned instead of silently
// selecting the legacy layout.
func TestResolveConfigRoot_UnexpectedStatErrorFails(t *testing.T) {
	home := t.TempDir()
	statErr := os.ErrPermission
	_, _, err := resolveConfigRoot(func(string) statResult { return statResult{err: statErr} }, home)
	if err == nil {
		t.Fatal("resolveConfigRoot() error = nil, want the unexpected stat error to propagate")
	}
	if !errors.Is(err, statErr) {
		t.Fatalf("resolveConfigRoot() error = %v, want it to wrap %v", err, statErr)
	}
}

func TestAdapter_PostInstallMessage(t *testing.T) {
	tests := []struct {
		name     string
		os       string
		expected string
	}{
		{
			name:     "Unix paths",
			os:       "linux",
			expected: "/.kimi/agents/gentleman.yaml",
		},
		{
			name:     "Windows paths",
			os:       "windows",
			expected: `\.kimi\agents\gentleman.yaml`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Stub the stat seam so the legacy layout (and its --agent-file
			// guidance) is deterministic regardless of the host filesystem.
			a := &Adapter{
				statPath: func(string) statResult { return statResult{err: os.ErrNotExist} },
			}
			// Mock homeDir relative to expected path style.
			// We use a safe path like /tmp/test which filepath.FromSlash will normalize
			// to \tmp\test on Windows.
			homeDir := "/tmp/test"
			if tt.os == "windows" {
				homeDir = `C:\Users\test`
			}

			msg := a.PostInstallMessage(homeDir)
			if strings.Contains(strings.ToLower(msg), "sdd") || strings.Contains(msg, "/skill:") {
				t.Fatalf("PostInstallMessage() advertises retired skill commands:\n%s", msg)
			}
			if !strings.Contains(msg, "ODD") || !strings.Contains(msg, "kimi --agent-file") {
				t.Fatalf("PostInstallMessage() missing ODD launch instructions:\n%s", msg)
			}
			if !strings.Contains(msg, `"`+filepath.Join(homeDir, ".config", "agents", "skills")+`"`) {
				t.Fatalf("PostInstallMessage() missing quoted skills root:\n%s", msg)
			}

			// Construct expected path to verify against quoted output
			gentlemanYaml := filepath.Join(homeDir, ".kimi", "agents", "gentleman.yaml")

			// Normalize the expected string to the current host's separator.
			// Since the code uses filepath.Join, it will use \ on Windows and / on Linux.
			// The test should expect the host's actual separator if we want it to PASS
			// while running on that host.
			normalizedExpected := filepath.FromSlash(tt.expected)

			// On Windows, if we are simulating we want backslashes.
			// If we are on Windows and testing 'Unix paths' case, it will fail because
			// the code (running on Windows) used \. This is expected.
			// We skip the cross-platform check if it contradicts the host's logic,
			// or we only check the one matching the current host.
			// On Windows, if we are simulating we want backslashes.
			// If we are on Windows and testing 'Unix paths' case, it will fail because
			// the code (running on Windows) used \. This is expected.
			// We skip the cross-platform check if it contradicts the host's logic,
			// or we only check the one matching the current host.
			if (runtime.GOOS == "windows" && tt.os == "windows") || (runtime.GOOS != "windows" && tt.os == "linux") {
				// Verify path is present
				if !strings.Contains(msg, normalizedExpected) {
					t.Errorf("PostInstallMessage() for %s missing expected path: %q\ngot: %q", tt.os, normalizedExpected, msg)
				}
				// Verify path is quoted (specifically the gentleman.yaml path)
				quotedExpected := `"` + gentlemanYaml + `"`
				if !strings.Contains(msg, quotedExpected) {
					t.Errorf("PostInstallMessage() for %s: path not quoted: %q", tt.os, quotedExpected)
				}
			}
		})
	}
}

// newTempHomeWithKimiCode creates a home directory containing a real
// ~/.kimi-code directory (the kimi-code v0.11+ layout).
func newTempHomeWithKimiCode(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, CurrentConfigDirName), 0o755); err != nil {
		t.Fatal(err)
	}
	return home
}

func TestAdapter_PathMethods_PreferKimiCodeDir(t *testing.T) {
	home := newTempHomeWithKimiCode(t)
	a := NewAdapter()

	wantRoot := filepath.Join(home, CurrentConfigDirName)
	tests := []struct {
		name string
		got  string
		want string
	}{
		{"GlobalConfigDir", a.GlobalConfigDir(home), wantRoot},
		{"SystemPromptDir", a.SystemPromptDir(home), wantRoot},
		// The current kimi-code v0.11+ layout reads AGENTS.md, not KIMI.md.
		{"SystemPromptFile", a.SystemPromptFile(home), filepath.Join(wantRoot, "AGENTS.md")},
		{"SettingsPath", a.SettingsPath(home), filepath.Join(wantRoot, "config.toml")},
		{"MCPConfigPath", a.MCPConfigPath(home, "srv"), filepath.Join(wantRoot, "mcp.json")},
		// YAML agents are only discovered by the legacy layout, so the agents
		// directory stays on ~/.kimi/agents even when the current layout is
		// preferred.
		{"SubAgentsDir", a.SubAgentsDir(home), filepath.Join(home, LegacyConfigDirName, "agents")},
		{"SkillsDir", a.SkillsDir(home), filepath.Join(wantRoot, "skills")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got != tt.want {
				t.Errorf("%s = %q, want %q", tt.name, tt.got, tt.want)
			}
		})
	}
}

func TestAdapter_SkillsDir_LegacyLayoutKeepsSharedSkillsPath(t *testing.T) {
	home := t.TempDir()
	// No config dirs created: legacy layout resolution.
	a := NewAdapter()
	if got, want := a.SkillsDir(home), filepath.Join(home, ".config", "agents", "skills"); got != want {
		t.Errorf("SkillsDir(%q) = %q, want %q", home, got, want)
	}
}

func TestAdapter_PathMethods_KimiCodeFileFallsBackToLegacy(t *testing.T) {
	home := t.TempDir()
	// A plain file named .kimi-code is not a v0.11+ layout.
	if err := os.WriteFile(filepath.Join(home, CurrentConfigDirName), []byte("not a dir"), 0o644); err != nil {
		t.Fatal(err)
	}

	a := NewAdapter()
	if got, want := a.GlobalConfigDir(home), filepath.Join(home, LegacyConfigDirName); got != want {
		t.Errorf("GlobalConfigDir(%q) = %q, want %q", home, got, want)
	}
	if got, want := a.SettingsPath(home), filepath.Join(home, LegacyConfigDirName, "config.toml"); got != want {
		t.Errorf("SettingsPath(%q) = %q, want %q", home, got, want)
	}
}

func TestAdapter_Detect_PrefersKimiCodeDir(t *testing.T) {
	home := newTempHomeWithKimiCode(t)

	a := &Adapter{
		lookPath:    func(string) (string, error) { return "/usr/bin/kimi", nil },
		statPath:    defaultStat,
		pathExists:  func(string) bool { return false },
		userHomeDir: func() (string, error) { return home, nil },
	}

	installed, _, configPath, configFound, err := a.Detect(context.Background(), home)
	if err != nil {
		t.Fatalf("Detect() error = %v", err)
	}
	if !installed || !configFound {
		t.Fatalf("Detect() installed=%v configFound=%v, want both true", installed, configFound)
	}
	if configPath != filepath.Join(home, CurrentConfigDirName) {
		t.Errorf("Detect() configPath = %q, want the .kimi-code root", configPath)
	}
}

func TestAdapter_Detect_KimiCodeFileFallsBackToLegacy(t *testing.T) {
	home := t.TempDir()
	if err := os.WriteFile(filepath.Join(home, CurrentConfigDirName), []byte("not a dir"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(home, LegacyConfigDirName), 0o755); err != nil {
		t.Fatal(err)
	}

	a := &Adapter{
		lookPath:    func(string) (string, error) { return "", os.ErrNotExist },
		statPath:    defaultStat,
		pathExists:  func(string) bool { return false },
		userHomeDir: func() (string, error) { return home, nil },
	}

	_, _, configPath, configFound, err := a.Detect(context.Background(), home)
	if err != nil {
		t.Fatalf("Detect() error = %v", err)
	}
	if !configFound {
		t.Fatal("Detect() configFound = false, want true (legacy .kimi dir exists)")
	}
	if configPath != filepath.Join(home, LegacyConfigDirName) {
		t.Errorf("Detect() configPath = %q, want the legacy .kimi root", configPath)
	}
}

func TestAdapter_BootstrapTemplate_LegacyKeepsJinjaRouter(t *testing.T) {
	home := t.TempDir()
	a := NewAdapter()

	if err := a.BootstrapTemplate(home); err != nil {
		t.Fatalf("BootstrapTemplate() error = %v", err)
	}

	content, err := os.ReadFile(filepath.Join(home, LegacyConfigDirName, "KIMI.md"))
	if err != nil {
		t.Fatalf("ReadFile(KIMI.md) error = %v", err)
	}
	text := string(content)
	if !strings.Contains(text, `{% include "persona.md" ignore missing %}`) {
		t.Fatalf("legacy KIMI.md lost Jinja include router:\n%s", text)
	}
	if _, err := os.Stat(filepath.Join(home, LegacyConfigDirName, "config.toml")); err != nil {
		t.Fatalf("config.toml not created: %v", err)
	}
}

func TestAdapter_BootstrapTemplate_CurrentExpandsJinjaModules(t *testing.T) {
	home := newTempHomeWithKimiCode(t)
	configDir := filepath.Join(home, CurrentConfigDirName)
	if err := os.WriteFile(filepath.Join(configDir, "persona.md"), []byte("PERSONA MODULE\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "agent-routing.md"), []byte("ROUTING MODULE\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	a := NewAdapter()

	if err := a.BootstrapTemplate(home); err != nil {
		t.Fatalf("BootstrapTemplate() error = %v", err)
	}

	content, err := os.ReadFile(filepath.Join(configDir, "AGENTS.md"))
	if err != nil {
		t.Fatalf("ReadFile(AGENTS.md) error = %v", err)
	}
	text := string(content)
	for _, forbidden := range []string{`{% include "persona.md" ignore missing %}`, `{% include "agent-routing.md" ignore missing %}`, "${KIMI_AGENTS_MD}", "${KIMI_SKILLS}", "## Project Instructions", "## Loaded Skills"} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("current AGENTS.md retained legacy-only template content %q:\n%s", forbidden, text)
		}
	}
	for _, want := range []string{"<!-- gentle-ai:kimi-agents-hub -->", "PERSONA MODULE", "ROUTING MODULE", "kimi-code module persona.md", "kimi-code module agent-routing.md"} {
		if !strings.Contains(text, want) {
			t.Fatalf("current AGENTS.md missing expanded module content %q:\n%s", want, text)
		}
	}
}

func TestAdapter_BootstrapTemplate_CurrentPreservesUserAuthoredAgentsContent(t *testing.T) {
	home := newTempHomeWithKimiCode(t)
	configDir := filepath.Join(home, CurrentConfigDirName)
	agentsPath := filepath.Join(configDir, "AGENTS.md")
	if err := os.WriteFile(agentsPath, []byte("# Team Kimi Notes\n\nKeep this user rule.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "agent-routing.md"), []byte("FIRST ROUTING MODULE\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	a := NewAdapter()

	if err := a.BootstrapTemplate(home); err != nil {
		t.Fatalf("BootstrapTemplate() first error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "agent-routing.md"), []byte("SECOND ROUTING MODULE\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := a.BootstrapTemplate(home); err != nil {
		t.Fatalf("BootstrapTemplate() second error = %v", err)
	}

	content, err := os.ReadFile(agentsPath)
	if err != nil {
		t.Fatalf("ReadFile(AGENTS.md) error = %v", err)
	}
	text := string(content)
	if !strings.Contains(text, "Keep this user rule.") {
		t.Fatalf("current AGENTS.md lost user-authored content:\n%s", text)
	}
	if !strings.Contains(text, "SECOND ROUTING MODULE") || strings.Contains(text, "FIRST ROUTING MODULE") {
		t.Fatalf("current AGENTS.md did not replace only the managed hub section:\n%s", text)
	}
	if count := strings.Count(text, "<!-- gentle-ai:kimi-agents-hub -->"); count != 1 {
		t.Fatalf("current AGENTS.md has %d managed hub sections, want 1:\n%s", count, text)
	}
}

// TestAdapter_Detect_UnexpectedStatErrorPropagates verifies that a stat
// failure other than absence on the current config root is surfaced by
// Detect instead of silently selecting the legacy layout.
func TestAdapter_Detect_UnexpectedStatErrorPropagates(t *testing.T) {
	tmpDir := t.TempDir()
	statErr := os.ErrPermission

	a := &Adapter{
		statPath: func(string) statResult { return statResult{err: statErr} },
	}

	_, _, _, _, err := a.Detect(context.Background(), tmpDir)
	if err == nil {
		t.Fatal("Detect() error = nil, want the unexpected stat error to propagate")
	}
	if !errors.Is(err, statErr) {
		t.Fatalf("Detect() error = %v, want it to wrap %v", err, statErr)
	}
}

// TestAdapter_BootstrapTemplate_UnexpectedStatErrorPropagates verifies that
// BootstrapTemplate surfaces an undeterminable layout instead of writing the
// skeleton into the legacy config root.
func TestAdapter_BootstrapTemplate_UnexpectedStatErrorPropagates(t *testing.T) {
	home := t.TempDir()
	statErr := os.ErrPermission

	a := &Adapter{
		statPath: func(string) statResult { return statResult{err: statErr} },
	}

	if err := a.BootstrapTemplate(home); !errors.Is(err, statErr) {
		t.Fatalf("BootstrapTemplate() error = %v, want it to wrap %v", err, statErr)
	}
	// Nothing must have been written while the layout was undeterminable.
	if _, err := os.Stat(filepath.Join(home, LegacyConfigDirName, "KIMI.md")); !os.IsNotExist(err) {
		t.Fatalf("BootstrapTemplate wrote the legacy skeleton (stat err = %v), want it absent", err)
	}
}

// TestAdapter_SystemPromptFile_LegacyLayoutKeepsKimiMD pins the legacy
// half of the system prompt filename split.
func TestAdapter_SystemPromptFile_LegacyLayoutKeepsKimiMD(t *testing.T) {
	home := t.TempDir()
	a := NewAdapter()
	if got, want := a.SystemPromptFile(home), filepath.Join(home, LegacyConfigDirName, "KIMI.md"); got != want {
		t.Errorf("SystemPromptFile(%q) = %q, want %q", home, got, want)
	}
}

// TestAdapter_SubAgentsDir_AlwaysLegacyLayout pins that YAML agents are
// installed only where a Kimi CLI can actually discover them.
func TestAdapter_SubAgentsDir_AlwaysLegacyLayout(t *testing.T) {
	// Current layout active: SubAgentsDir must still point at ~/.kimi/agents.
	home := newTempHomeWithKimiCode(t)
	a := NewAdapter()
	if got, want := a.SubAgentsDir(home), filepath.Join(home, LegacyConfigDirName, "agents"); got != want {
		t.Errorf("SubAgentsDir(current layout) = %q, want %q", got, want)
	}

	// Legacy layout: unchanged behavior.
	home = t.TempDir()
	if got, want := a.SubAgentsDir(home), filepath.Join(home, LegacyConfigDirName, "agents"); got != want {
		t.Errorf("SubAgentsDir(legacy layout) = %q, want %q", got, want)
	}
}

func TestAdapter_PostInstallMessage_CurrentLayoutOmitsAgentFile(t *testing.T) {
	home := newTempHomeWithKimiCode(t)
	a := NewAdapter()

	msg := a.PostInstallMessage(home)
	if strings.Contains(msg, "--agent-file") {
		t.Errorf("PostInstallMessage() advertises retired --agent-file for v0.11+ layout:\n%s", msg)
	}
	if !strings.Contains(msg, `"`+filepath.Join(home, CurrentConfigDirName, "skills")+`"`) {
		t.Errorf("PostInstallMessage() missing quoted current skills root:\n%s", msg)
	}
	if !strings.Contains(msg, "Kimi Code configured!") {
		t.Errorf("PostInstallMessage() missing header:\n%s", msg)
	}
}

func TestAdapter_PostInstallMessage_LegacyLayoutKeepsAgentFileGuidance(t *testing.T) {
	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, LegacyConfigDirName), 0o755); err != nil {
		t.Fatal(err)
	}
	a := NewAdapter()

	msg := a.PostInstallMessage(home)
	if !strings.Contains(msg, "kimi --agent-file") {
		t.Errorf("PostInstallMessage() legacy layout missing --agent-file guidance:\n%s", msg)
	}
	if !strings.Contains(msg, "ODD") {
		t.Errorf("PostInstallMessage() legacy layout missing ODD guidance:\n%s", msg)
	}
}
