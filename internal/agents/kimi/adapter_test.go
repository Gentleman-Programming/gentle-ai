package kimi

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/internal/model"
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
	a := NewAdapter()
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
		{"SupportsAutoInstall", a.SupportsAutoInstall(), true},
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
	a := NewAdapter()
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

func TestConfigPath(t *testing.T) {
	homeDir := "/home/test"
	got := ConfigPath(homeDir)
	expected := filepath.Join(homeDir, ".kimi")
	if got != expected {
		t.Errorf("ConfigPath() = %v, want %v", got, expected)
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
			a := NewAdapter()
			// Mock homeDir relative to expected path style.
			// We use a safe path like /tmp/test which filepath.FromSlash will normalize
			// to \tmp\test on Windows.
			homeDir := "/tmp/test"
			if tt.os == "windows" {
				homeDir = `C:\Users\test`
			}
			
			msg := a.PostInstallMessage(homeDir)

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


