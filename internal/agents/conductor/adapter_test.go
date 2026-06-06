package conductor

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/gentleman-programming/gentle-ai/internal/model"
	"github.com/gentleman-programming/gentle-ai/internal/system"
)

const testHome = "/tmp/home"

func TestDetect(t *testing.T) {
	tests := []struct {
		name            string
		statErr         error
		statIsDir       bool
		wantInstalled   bool
		wantConfigPath  string
		wantConfigFound bool
		wantErr         bool
	}{
		{
			name:            "config directory found",
			statIsDir:       true,
			wantInstalled:   true,
			wantConfigPath:  filepath.Join(testHome, ".conductor"),
			wantConfigFound: true,
		},
		{
			name:            "config missing",
			statErr:         os.ErrNotExist,
			wantInstalled:   false,
			wantConfigPath:  filepath.Join(testHome, ".conductor"),
			wantConfigFound: false,
		},
		{
			name:            "config exists but is a file not a dir",
			statIsDir:       false,
			wantInstalled:   false,
			wantConfigPath:  filepath.Join(testHome, ".conductor"),
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
				statPath: func(string) (os.FileInfo, error) {
					if tt.statErr != nil {
						return nil, tt.statErr
					}
					return &fakeFileInfo{isDir: tt.statIsDir}, nil
				},
			}
			installed, _, configPath, configFound, err := a.Detect(context.Background(), testHome)
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

func TestConfigPaths(t *testing.T) {
	a := NewAdapter()

	wantGlobal := filepath.Join(testHome, ".conductor")
	if got := a.GlobalConfigDir(testHome); got != wantGlobal {
		t.Fatalf("GlobalConfigDir() = %q, want %q", got, wantGlobal)
	}

	if got := a.SystemPromptDir(testHome); got != "" {
		t.Fatalf("SystemPromptDir() = %q, want empty (no standalone config)", got)
	}
	if got := a.SystemPromptFile(testHome); got != "" {
		t.Fatalf("SystemPromptFile() = %q, want empty (no standalone config)", got)
	}
	if got := a.SkillsDir(testHome); got != "" {
		t.Fatalf("SkillsDir() = %q, want empty (no standalone config)", got)
	}
	if got := a.SettingsPath(testHome); got != "" {
		t.Fatalf("SettingsPath() = %q, want empty (no standalone config)", got)
	}
	if got := a.MCPConfigPath(testHome, "any-server"); got != "" {
		t.Fatalf("MCPConfigPath() = %q, want empty (no standalone config)", got)
	}
}

func TestAgentIdentity(t *testing.T) {
	a := NewAdapter()

	if got := a.Agent(); got != model.AgentConductor {
		t.Fatalf("Agent() = %v, want %v", got, model.AgentConductor)
	}
	if got := a.Tier(); got != model.TierFull {
		t.Fatalf("Tier() = %v, want %v", got, model.TierFull)
	}
}

func TestCapabilities(t *testing.T) {
	a := NewAdapter()

	if a.SupportsSkills() {
		t.Fatal("Conductor should NOT support skills (inherited from Claude Code)")
	}
	if a.SupportsSystemPrompt() {
		t.Fatal("Conductor should NOT support system prompt (inherited from Claude Code)")
	}
	if a.SupportsMCP() {
		t.Fatal("Conductor should NOT support MCP (inherited from Claude Code)")
	}
	if a.SupportsAutoInstall() {
		t.Fatal("Conductor should NOT support auto-install (desktop app)")
	}
	if a.SupportsOutputStyles() {
		t.Fatal("Conductor should NOT support output styles")
	}
	if a.SupportsSlashCommands() {
		t.Fatal("Conductor should NOT support slash commands")
	}
	if a.SupportsSubAgents() {
		t.Fatal("Conductor should NOT support sub-agents")
	}
}

func TestDesktopAppNotAutoInstallable(t *testing.T) {
	a := NewAdapter()

	_, err := a.InstallCommand(system.PlatformProfile{})
	if err == nil {
		t.Fatal("InstallCommand() should return error for desktop app")
	}

	notInstallable, ok := err.(AgentNotInstallableError)
	if !ok {
		t.Fatalf("InstallCommand() error type = %T, want AgentNotInstallableError", err)
	}
	if notInstallable.Agent != model.AgentConductor {
		t.Fatalf("AgentNotInstallableError.Agent = %v, want %v", notInstallable.Agent, model.AgentConductor)
	}
}

// fakeFileInfo implements os.FileInfo for testing.
type fakeFileInfo struct {
	isDir bool
}

func (f *fakeFileInfo) Name() string      { return ".conductor" }
func (f *fakeFileInfo) Size() int64       { return 0 }
func (f *fakeFileInfo) Mode() os.FileMode { return 0o755 }
func (f *fakeFileInfo) ModTime() time.Time { return time.Time{} }
func (f *fakeFileInfo) IsDir() bool        { return f.isDir }
func (f *fakeFileInfo) Sys() interface{}   { return nil }
