package freebuff

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/gentleman-programming/gentle-ai/internal/model"
	"github.com/gentleman-programming/gentle-ai/internal/system"
	"github.com/gentleman-programming/gentle-ai/internal/versions"
)

func TestAdapterIdentityAndStrategies(t *testing.T) {
	a := NewAdapter()
	homeDir := filepath.Join(string(filepath.Separator), "tmp", "home")

	if got := a.Agent(); got != model.AgentFreebuff {
		t.Fatalf("Agent() = %q, want %q", got, model.AgentFreebuff)
	}

	if got := a.Tier(); got != model.TierFull {
		t.Fatalf("Tier() = %q, want %q", got, model.TierFull)
	}

	if got := a.SystemPromptStrategy(); got != model.StrategyMarkdownSections {
		t.Fatalf("SystemPromptStrategy() = %v, want %v", got, model.StrategyMarkdownSections)
	}

	if got := a.MCPStrategy(); got != model.StrategyMCPConfigFile {
		t.Fatalf("MCPStrategy() = %v, want %v", got, model.StrategyMCPConfigFile)
	}

	if !a.SupportsSystemPrompt() {
		t.Fatalf("SupportsSystemPrompt() = false, want true")
	}

	if !a.SupportsMCP() {
		t.Fatalf("SupportsMCP() = false, want true")
	}

	if a.SupportsOutputStyles() {
		t.Fatalf("SupportsOutputStyles() = true, want false")
	}

	if got := a.SystemPromptFile(homeDir); got != filepath.Join(homeDir, "knowledge.md") {
		t.Fatalf("SystemPromptFile() = %q, want workspace knowledge.md", got)
	}

	if got := a.OutputStyleDir(homeDir); got != "" {
		t.Fatalf("OutputStyleDir() = %q, want empty", got)
	}
}

func TestInstallCommand(t *testing.T) {
	a := NewAdapter()

	wantPkg := "freebuff@" + versions.Freebuff
	wantCmd := []string{"npm", "install", "-g", "--ignore-scripts", wantPkg}

	commands, err := a.InstallCommand(system.PlatformProfile{})
	if err != nil {
		t.Fatalf("InstallCommand() unexpected error: %v", err)
	}
	if len(commands) != 1 {
		t.Fatalf("InstallCommand() got %d commands, want 1", len(commands))
	}
	got := commands[0]
	if len(got) != len(wantCmd) {
		t.Fatalf("InstallCommand() command = %v, want %v", got, wantCmd)
	}
	for i, tok := range wantCmd {
		if got[i] != tok {
			t.Fatalf("InstallCommand() command[%d] = %q, want %q (full: %v)", i, got[i], tok, got)
		}
	}
}

func TestAdapterConfigPaths(t *testing.T) {
	a := NewAdapter()
	homeDir := filepath.Join(string(filepath.Separator), "tmp", "home")
	configDir := filepath.Join(homeDir, ".agents")

	paths := map[string]string{
		"GlobalConfigDir": a.GlobalConfigDir(homeDir),
		"SettingsPath":    a.SettingsPath(homeDir),
		"MCPConfigPath":   a.MCPConfigPath(homeDir, "engram"),
		"SkillsDir":       a.SkillsDir(homeDir),
	}

	if got := paths["GlobalConfigDir"]; got != configDir {
		t.Fatalf("GlobalConfigDir() = %q, want %q", got, configDir)
	}

	if got := paths["SettingsPath"]; got != "" {
		t.Fatalf("SettingsPath() = %q, want empty", got)
	}

	if got := paths["MCPConfigPath"]; got != filepath.Join(configDir, "mcp.json") {
		t.Fatalf("MCPConfigPath() = %q, want %q", got, filepath.Join(configDir, "mcp.json"))
	}

	if got := paths["SkillsDir"]; got != filepath.Join(configDir, "skills") {
		t.Fatalf("SkillsDir() = %q, want Freebuff skills dir", got)
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
		wantConfigFound bool
		wantErr         bool
	}{
		{
			name:            "binary and config directory found",
			lookPathPath:    "/usr/local/bin/freebuff",
			stat:            statResult{isDir: true},
			wantInstalled:   true,
			wantBinaryPath:  "/usr/local/bin/freebuff",
			wantConfigFound: true,
		},
		{
			name:            "binary missing and config missing",
			lookPathErr:     errors.New("missing"),
			stat:            statResult{err: os.ErrNotExist},
			wantInstalled:   false,
			wantBinaryPath:  "",
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
			homeDir := filepath.Join(string(filepath.Separator), "tmp", "home")

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

			wantConfigPath := filepath.Join(homeDir, ".agents")
			if configPath != wantConfigPath {
				t.Fatalf("Detect() configPath = %q, want %q", configPath, wantConfigPath)
			}

			if configFound != tt.wantConfigFound {
				t.Fatalf("Detect() configFound = %v, want %v", configFound, tt.wantConfigFound)
			}
		})
	}
}
