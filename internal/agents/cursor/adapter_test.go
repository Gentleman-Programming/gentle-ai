package cursor

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/model"
	"github.com/gentleman-programming/gentle-ai/v2/internal/system"
)

func TestDetect(t *testing.T) {
	tests := []struct {
		name            string
		stat            statResult
		wantInstalled   bool
		wantConfigPath  string
		wantConfigFound bool
		wantErr         bool
	}{
		{
			name:            "config directory found",
			stat:            statResult{isDir: true},
			wantInstalled:   true,
			wantConfigPath:  filepath.Join("/tmp/home", ".cursor"),
			wantConfigFound: true,
		},
		{
			name:            "config missing",
			stat:            statResult{err: os.ErrNotExist},
			wantInstalled:   false,
			wantConfigPath:  filepath.Join("/tmp/home", ".cursor"),
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
				statPath: func(string) statResult {
					return tt.stat
				},
			}

			installed, _, configPath, configFound, err := a.Detect(context.Background(), "/tmp/home")
			if (err != nil) != tt.wantErr {
				t.Fatalf("Detect() error = %v, wantErr %v", err, tt.wantErr)
			}

			if tt.wantErr {
				return
			}

			if installed != tt.wantInstalled {
				t.Fatalf("Detect() installed = %v, want %v", installed, tt.wantInstalled)
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

func TestConfigPathsCrossPlatform(t *testing.T) {
	a := NewAdapter()
	home := "/tmp/home"

	if got := a.GlobalConfigDir(home); got != filepath.Join(home, ".cursor") {
		t.Fatalf("GlobalConfigDir() = %q, want %q", got, filepath.Join(home, ".cursor"))
	}

	if got := a.SkillsDir(home); got != filepath.Join(home, ".cursor", "skills") {
		t.Fatalf("SkillsDir() = %q, want %q", got, filepath.Join(home, ".cursor", "skills"))
	}

	if got := a.MCPConfigPath(home, "ctx7"); got != filepath.Join(home, ".cursor", "mcp.json") {
		t.Fatalf("MCPConfigPath() = %q, want %q", got, filepath.Join(home, ".cursor", "mcp.json"))
	}

	if got := a.SystemPromptFile(home); got != filepath.Join(home, ".cursor", "rules", "gentle-ai.mdc") {
		t.Fatalf("SystemPromptFile() = %q, want %q", got, filepath.Join(home, ".cursor", "rules", "gentle-ai.mdc"))
	}
}

func TestStrategies(t *testing.T) {
	a := NewAdapter()

	if got := a.SystemPromptStrategy(); got != model.StrategyFileReplace {
		t.Fatalf("SystemPromptStrategy() = %v, want %v", got, model.StrategyFileReplace)
	}

	if got := a.MCPStrategy(); got != model.StrategyMCPConfigFile {
		t.Fatalf("MCPStrategy() = %v, want %v", got, model.StrategyMCPConfigFile)
	}
}

func TestDesktopAppNotAutoInstallable(t *testing.T) {
	a := NewAdapter()

	if a.SupportsAutoInstall() {
		t.Fatalf("Cursor should not support auto-install (desktop app)")
	}

	_, err := a.InstallCommand(system.PlatformProfile{})
	if err == nil {
		t.Fatalf("InstallCommand() should return error for desktop app")
	}
}
