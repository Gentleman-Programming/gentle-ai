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

func TestDetect(t *testing.T) {
	tests := []struct {
		name            string
		existingDirs    map[string]bool
		statErr         error
		wantInstalled   bool
		wantConfigPath  string
		wantConfigFound bool
		wantErr         bool
	}{
		{
			name:            "config directory antigravity-ide found",
			existingDirs:    map[string]bool{"/tmp/home/.gemini/antigravity-ide": true},
			wantInstalled:   true,
			wantConfigPath:  filepath.Join("/tmp/home", ".gemini", "antigravity-ide"),
			wantConfigFound: true,
		},
		{
			name:            "config directory antigravity-cli found",
			existingDirs:    map[string]bool{"/tmp/home/.gemini/antigravity-cli": true},
			wantInstalled:   true,
			wantConfigPath:  filepath.Join("/tmp/home", ".gemini", "antigravity-cli"),
			wantConfigFound: true,
		},
		{
			name:            "both directories found - prefers ide",
			existingDirs:    map[string]bool{"/tmp/home/.gemini/antigravity-ide": true, "/tmp/home/.gemini/antigravity-cli": true},
			wantInstalled:   true,
			wantConfigPath:  filepath.Join("/tmp/home", ".gemini", "antigravity-ide"),
			wantConfigFound: true,
		},
		{
			name:            "config directory missing",
			existingDirs:    map[string]bool{},
			wantInstalled:   false,
			wantConfigPath:  filepath.Join("/tmp/home", ".gemini", "antigravity-ide"),
			wantConfigFound: false,
		},
		{
			name:    "stat error bubbles up",
			statErr: errors.New("permission denied"),
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := &Adapter{
				statPath: func(path string) statResult {
					if tt.statErr != nil {
						return statResult{err: tt.statErr}
					}
					if tt.existingDirs[path] {
						return statResult{isDir: true}
					}
					return statResult{err: os.ErrNotExist}
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

			if binaryPath != "" {
				t.Fatalf("Detect() binaryPath = %q, want empty string", binaryPath)
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

func TestConfigPathsCrossPlatform(t *testing.T) {
	home := "/tmp/home"

	t.Run("resolves to antigravity-ide when ide folder exists", func(t *testing.T) {
		a := &Adapter{
			statPath: func(path string) statResult {
				if path == filepath.Join(home, ".gemini", "antigravity-ide") {
					return statResult{isDir: true}
				}
				return statResult{err: os.ErrNotExist}
			},
		}

		if got := a.GlobalConfigDir(home); got != filepath.Join(home, ".gemini", "antigravity-ide") {
			t.Fatalf("GlobalConfigDir() = %q, want %q", got, filepath.Join(home, ".gemini", "antigravity-ide"))
		}

		if got := a.SkillsDir(home); got != filepath.Join(home, ".gemini", "antigravity-ide", "skills") {
			t.Fatalf("SkillsDir() = %q, want %q", got, filepath.Join(home, ".gemini", "antigravity-ide", "skills"))
		}

		if got := a.MCPConfigPath(home, "ctx7"); got != filepath.Join(home, ".gemini", "antigravity-ide", "mcp_config.json") {
			t.Fatalf("MCPConfigPath() = %q, want %q", got, filepath.Join(home, ".gemini", "antigravity-ide", "mcp_config.json"))
		}

		if got := a.SystemPromptFile(home); got != filepath.Join(home, ".gemini", "GEMINI.md") {
			t.Fatalf("SystemPromptFile() = %q, want %q", got, filepath.Join(home, ".gemini", "GEMINI.md"))
		}

		if got := a.SettingsPath(home); got != filepath.Join(home, ".gemini", "antigravity-ide", "settings.json") {
			t.Fatalf("SettingsPath() = %q, want %q", got, filepath.Join(home, ".gemini", "antigravity-ide", "settings.json"))
		}

		if got := a.SystemPromptDir(home); got != filepath.Join(home, ".gemini") {
			t.Fatalf("SystemPromptDir() = %q, want %q", got, filepath.Join(home, ".gemini"))
		}
	})

	t.Run("resolves to antigravity-cli when only cli folder exists", func(t *testing.T) {
		a := &Adapter{
			statPath: func(path string) statResult {
				if path == filepath.Join(home, ".gemini", "antigravity-cli") {
					return statResult{isDir: true}
				}
				return statResult{err: os.ErrNotExist}
			},
		}

		if got := a.GlobalConfigDir(home); got != filepath.Join(home, ".gemini", "antigravity-cli") {
			t.Fatalf("GlobalConfigDir() = %q, want %q", got, filepath.Join(home, ".gemini", "antigravity-cli"))
		}

		if got := a.SkillsDir(home); got != filepath.Join(home, ".gemini", "antigravity-cli", "skills") {
			t.Fatalf("SkillsDir() = %q, want %q", got, filepath.Join(home, ".gemini", "antigravity-cli", "skills"))
		}

		if got := a.MCPConfigPath(home, "ctx7"); got != filepath.Join(home, ".gemini", "antigravity-cli", "mcp_config.json") {
			t.Fatalf("MCPConfigPath() = %q, want %q", got, filepath.Join(home, ".gemini", "antigravity-cli", "mcp_config.json"))
		}

		if got := a.SettingsPath(home); got != filepath.Join(home, ".gemini", "antigravity-cli", "settings.json") {
			t.Fatalf("SettingsPath() = %q, want %q", got, filepath.Join(home, ".gemini", "antigravity-cli", "settings.json"))
		}
	})

	t.Run("default fallback when neither folder exists", func(t *testing.T) {
		a := &Adapter{
			statPath: func(path string) statResult {
				return statResult{err: os.ErrNotExist}
			},
		}

		if got := a.GlobalConfigDir(home); got != filepath.Join(home, ".gemini", "antigravity-ide") {
			t.Fatalf("GlobalConfigDir() = %q, want %q", got, filepath.Join(home, ".gemini", "antigravity-ide"))
		}
	})
}

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

func TestStrategies(t *testing.T) {
	a := NewAdapter()

	if got := a.SystemPromptStrategy(); got != model.StrategyAppendToFile {
		t.Fatalf("SystemPromptStrategy() = %v, want StrategyAppendToFile", got)
	}

	if got := a.MCPStrategy(); got != model.StrategyMCPConfigFile {
		t.Fatalf("MCPStrategy() = %v, want StrategyMCPConfigFile", got)
	}
}

func TestIdentity(t *testing.T) {
	a := NewAdapter()

	if got := a.Agent(); got != model.AgentAntigravity {
		t.Fatalf("Agent() = %q, want %q", got, model.AgentAntigravity)
	}

	if got := a.Tier(); got != model.TierFull {
		t.Fatalf("Tier() = %q, want %q", got, model.TierFull)
	}
}
