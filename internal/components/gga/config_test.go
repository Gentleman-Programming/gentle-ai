package gga

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/internal/model"
)

func TestProviderForAgents(t *testing.T) {
	tests := []struct {
		name   string
		agents []model.AgentID
		want   string
	}{
		{
			name:   "claude-code maps to claude",
			agents: []model.AgentID{model.AgentClaudeCode},
			want:   "claude",
		},
		{
			name:   "opencode maps to opencode",
			agents: []model.AgentID{model.AgentOpenCode},
			want:   "opencode",
		},
		{
			name:   "gemini-cli maps to gemini",
			agents: []model.AgentID{model.AgentGeminiCLI},
			want:   "gemini",
		},
		{
			name:   "both claude and opencode defaults to claude",
			agents: []model.AgentID{model.AgentClaudeCode, model.AgentOpenCode},
			want:   "claude",
		},
		{
			name:   "opencode and gemini defaults to opencode",
			agents: []model.AgentID{model.AgentOpenCode, model.AgentGeminiCLI},
			want:   "opencode",
		},
		{
			name:   "empty agents defaults to claude",
			agents: []model.AgentID{},
			want:   "claude",
		},
		{
			name:   "nil agents defaults to claude",
			agents: nil,
			want:   "claude",
		},
		{
			name:   "cursor only defaults to claude",
			agents: []model.AgentID{model.AgentCursor},
			want:   "claude",
		},
		{
			name:   "codex maps to codex",
			agents: []model.AgentID{model.AgentCodex},
			want:   "codex",
		},
		{
			name:   "antigravity maps to gemini",
			agents: []model.AgentID{model.AgentAntigravity},
			want:   "gemini",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ProviderForAgents(tt.agents)
			if got != tt.want {
				t.Fatalf("ProviderForAgents(%v) = %q, want %q", tt.agents, got, tt.want)
			}
		})
	}
}

func TestBuildConfig(t *testing.T) {
	config := string(BuildConfig("claude"))

	requiredFields := []string{
		`PROVIDER="claude"`,
		`FILE_PATTERNS=`,
		`EXCLUDE_PATTERNS=`,
		`RULES_FILE="AGENTS.md"`,
		`STRICT_MODE="true"`,
		`TIMEOUT="300"`,
	}

	for _, field := range requiredFields {
		if !strings.Contains(config, field) {
			t.Errorf("BuildConfig() missing field %q", field)
		}
	}

	// Config should NOT be JSON — it's shell-sourced.
	if strings.Contains(config, "{") || strings.Contains(config, "}") {
		t.Error("BuildConfig() should produce shell-sourced format, not JSON")
	}

	// Verify header comment.
	if !strings.HasPrefix(config, "# Gentleman Guardian Angel") {
		t.Error("BuildConfig() should start with a header comment")
	}
}

func TestBuildConfigDifferentProviders(t *testing.T) {
	for _, provider := range []string{"claude", "opencode", "gemini", "ollama:llama3.2"} {
		config := string(BuildConfig(provider))
		expected := `PROVIDER="` + provider + `"`
		if !strings.Contains(config, expected) {
			t.Errorf("BuildConfig(%q) missing %q", provider, expected)
		}
	}
}

func TestInjectWritesConfigAndAgents(t *testing.T) {
	home := t.TempDir()

	result, err := Inject(home, []model.AgentID{model.AgentClaudeCode})
	if err != nil {
		t.Fatalf("Inject() error = %v", err)
	}

	// Config file created.
	if result.ConfigFile == "" {
		t.Fatal("Inject() ConfigFile is empty")
	}
	if !result.ConfigChanged {
		t.Fatal("Inject() ConfigChanged = false on first run")
	}

	configContent, err := os.ReadFile(result.ConfigFile)
	if err != nil {
		t.Fatalf("read config file: %v", err)
	}
	if !strings.Contains(string(configContent), `PROVIDER="claude"`) {
		t.Error("config file missing PROVIDER=claude")
	}

	// AGENTS.md template created.
	if result.AgentsFile == "" {
		t.Fatal("Inject() AgentsFile is empty")
	}
	if !result.AgentsChanged {
		t.Fatal("Inject() AgentsChanged = false on first run")
	}

	agentsContent, err := os.ReadFile(result.AgentsFile)
	if err != nil {
		t.Fatalf("read agents file: %v", err)
	}
	if !strings.Contains(string(agentsContent), "# Code Review Rules") {
		t.Error("AGENTS.md template missing expected header")
	}
	if !strings.Contains(string(agentsContent), "STATUS: PASSED") {
		t.Error("AGENTS.md template missing response format section")
	}
}

func TestInjectIsIdempotent(t *testing.T) {
	home := t.TempDir()

	first, err := Inject(home, []model.AgentID{model.AgentOpenCode})
	if err != nil {
		t.Fatalf("Inject() first error = %v", err)
	}
	if !first.ConfigChanged || !first.AgentsChanged {
		t.Fatal("Inject() first run should report changed")
	}

	second, err := Inject(home, []model.AgentID{model.AgentOpenCode})
	if err != nil {
		t.Fatalf("Inject() second error = %v", err)
	}
	if second.ConfigChanged || second.AgentsChanged {
		t.Fatal("Inject() second run should not report changed (idempotent)")
	}
}

func TestConfigPath(t *testing.T) {
	// On non-Windows hosts the path follows XDG convention.
	path := ConfigPath("/home/testuser")

	// Must NOT have .json extension (shell-sourced, no extension).
	if strings.HasSuffix(path, ".json") {
		t.Error("ConfigPath() should NOT end with .json")
	}
	// Must end with the filename "config".
	if filepath.Base(path) != "config" {
		t.Fatalf("ConfigPath() base = %q, want \"config\"", filepath.Base(path))
	}
}

func TestAgentsTemplatePath(t *testing.T) {
	path := AgentsTemplatePath("/home/testuser")
	if filepath.Base(path) != "AGENTS.md" {
		t.Fatalf("AgentsTemplatePath() base = %q, want \"AGENTS.md\"", filepath.Base(path))
	}
}

func TestGGAConfigDirLinux(t *testing.T) {
	tests := []struct {
		name    string
		homeDir string
		want    string
	}{
		{
			name:    "standard home dir",
			homeDir: "/home/user",
			want:    filepath.Join("/home/user", ".config", "gga"),
		},
		{
			name:    "root home dir",
			homeDir: "/root",
			want:    filepath.Join("/root", ".config", "gga"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ggaConfigDir(tt.homeDir, "linux")
			if got != tt.want {
				t.Fatalf("ggaConfigDir(%q, \"linux\") = %q, want %q", tt.homeDir, got, tt.want)
			}
		})
	}
}

func TestGGAConfigDirDarwin(t *testing.T) {
	got := ggaConfigDir("/Users/testuser", "darwin")
	want := filepath.Join("/Users/testuser", ".config", "gga")
	if got != want {
		t.Fatalf("ggaConfigDir(%q, \"darwin\") = %q, want %q", "/Users/testuser", got, want)
	}
}

func TestGGAConfigDirWindows(t *testing.T) {
	tests := []struct {
		name        string
		homeDir     string
		appDataEnv  string
		wantSuffix  string
	}{
		{
			name:       "APPDATA set to standard roaming path",
			homeDir:    `C:\Users\testuser`,
			appDataEnv: `C:\Users\testuser\AppData\Roaming`,
			wantSuffix: filepath.Join(`C:\Users\testuser\AppData\Roaming`, "gga"),
		},
		{
			name:       "APPDATA set to custom path",
			homeDir:    `C:\Users\other`,
			appDataEnv: `D:\custom\appdata`,
			wantSuffix: filepath.Join(`D:\custom\appdata`, "gga"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("APPDATA", tt.appDataEnv)
			got := ggaConfigDir(tt.homeDir, "windows")
			if got != tt.wantSuffix {
				t.Fatalf("ggaConfigDir(%q, \"windows\") = %q, want %q", tt.homeDir, got, tt.wantSuffix)
			}
		})
	}
}

func TestGGAConfigDirWindowsNoAPPDATA(t *testing.T) {
	// When APPDATA is unset, the fallback must be derived from homeDir.
	t.Setenv("APPDATA", "")
	got := ggaConfigDir(`C:\Users\testuser`, "windows")
	want := filepath.Join(`C:\Users\testuser`, "AppData", "Roaming", "gga")
	if got != want {
		t.Fatalf("ggaConfigDir fallback = %q, want %q", got, want)
	}
}

func TestFilesWritten(t *testing.T) {
	result := InjectionResult{
		ConfigFile: "/home/user/.config/gga/config",
		AgentsFile: "/home/user/.config/gga/AGENTS.md",
	}

	files := result.FilesWritten()
	if len(files) != 2 {
		t.Fatalf("FilesWritten() = %d files, want 2", len(files))
	}
}

func TestFilesWrittenEmpty(t *testing.T) {
	result := InjectionResult{}
	files := result.FilesWritten()
	if len(files) != 0 {
		t.Fatalf("FilesWritten() = %d files, want 0 for empty result", len(files))
	}
}

func TestPostInstallMessages(t *testing.T) {
	msgs := PostInstallMessages()
	if len(msgs) != 2 {
		t.Fatalf("PostInstallMessages() = %d messages, want 2", len(msgs))
	}

	if !strings.Contains(msgs[0], "gga install") {
		t.Error("first message should mention gga install")
	}
	if !strings.Contains(msgs[1], "AGENTS.md") {
		t.Error("second message should mention AGENTS.md")
	}
}
