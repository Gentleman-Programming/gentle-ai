package pi

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
			lookPathPath:    "/usr/local/bin/pi",
			stat:            statResult{isDir: true},
			wantInstalled:   true,
			wantBinaryPath:  "/usr/local/bin/pi",
			wantConfigPath:  filepath.Join("/tmp/home", ".pi", "agent"),
			wantConfigFound: true,
		},
		{
			name:            "binary missing and config missing",
			lookPathErr:     errors.New("missing"),
			stat:            statResult{err: os.ErrNotExist},
			wantInstalled:   false,
			wantBinaryPath:  "",
			wantConfigPath:  filepath.Join("/tmp/home", ".pi", "agent"),
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

func TestInstallCommandReturnsNotInstallableError(t *testing.T) {
	a := NewAdapter()

	_, err := a.InstallCommand(system.PlatformProfile{OS: "linux", PackageManager: "apt"})
	if err == nil {
		t.Fatal("InstallCommand() error = nil, want error")
	}

	var notInstallable AgentNotInstallableError
	if !errors.As(err, &notInstallable) {
		t.Fatalf("InstallCommand() error = %T, want AgentNotInstallableError", err)
	}
}

func TestAdapterBoundaryPathsAndCapabilities(t *testing.T) {
	homeDir := t.TempDir()
	a := NewAdapter()

	if got := a.Agent(); got != model.AgentPiCodingAgent {
		t.Fatalf("Agent() = %q, want %q", got, model.AgentPiCodingAgent)
	}

	if got := a.GlobalConfigDir(homeDir); got != filepath.Join(homeDir, ".pi", "agent") {
		t.Fatalf("GlobalConfigDir() = %q, want %q", got, filepath.Join(homeDir, ".pi", "agent"))
	}

	if got := a.SettingsPath(homeDir); got != filepath.Join(homeDir, ".pi", "agent", "settings.json") {
		t.Fatalf("SettingsPath() = %q, want %q", got, filepath.Join(homeDir, ".pi", "agent", "settings.json"))
	}

	if got := a.SkillsDir(homeDir); got != filepath.Join(homeDir, ".pi", "agent", "skills") {
		t.Fatalf("SkillsDir() = %q, want %q", got, filepath.Join(homeDir, ".pi", "agent", "skills"))
	}

	if got := a.CommandsDir(homeDir); got != filepath.Join(homeDir, ".pi", "agent", "commands") {
		t.Fatalf("CommandsDir() = %q, want %q", got, filepath.Join(homeDir, ".pi", "agent", "commands"))
	}

	if got := a.MCPStrategy(); got != model.StrategyMergeIntoSettings {
		t.Fatalf("MCPStrategy() = %v, want %v", got, model.StrategyMergeIntoSettings)
	}

	if a.SupportsAutoInstall() {
		t.Fatal("SupportsAutoInstall() = true, want false")
	}

	if a.SupportsSlashCommands() {
		t.Fatal("SupportsSlashCommands() = true, want false")
	}

	if a.SupportsSubAgents() {
		t.Fatal("SupportsSubAgents() = true, want false")
	}

	if a.SupportsOutputStyles() {
		t.Fatal("SupportsOutputStyles() = true, want false")
	}

	if !a.SupportsSkills() {
		t.Fatal("SupportsSkills() = false, want true")
	}

	if !a.SupportsSystemPrompt() {
		t.Fatal("SupportsSystemPrompt() = false, want true")
	}

	if !a.SupportsMCP() {
		t.Fatal("SupportsMCP() = false, want true")
	}

	if got := a.SupportsProfiles(); got {
		t.Fatal("SupportsProfiles() = true, want false")
	}

	if got := a.SupportsModelPicker(); got {
		t.Fatal("SupportsModelPicker() = true, want false")
	}

	if got := a.SupportsGeneratedMultiProfiles(); got {
		t.Fatal("SupportsGeneratedMultiProfiles() = true, want false")
	}
}

func TestDetectMarksConfigFoundWhenLegacyDirectoryExists(t *testing.T) {
	a := &Adapter{
		lookPath: func(string) (string, error) { return "", errors.New("missing") },
		statPath: func(path string) statResult {
			switch path {
			case filepath.Join("/tmp/home", ".pi", "agent"):
				return statResult{err: os.ErrNotExist}
			case filepath.Join("/tmp/home", ".config", "pi-coding-agent"):
				return statResult{isDir: true}
			default:
				return statResult{err: os.ErrNotExist}
			}
		},
	}

	installed, _, configPath, configFound, err := a.Detect(context.Background(), "/tmp/home")
	if err != nil {
		t.Fatalf("Detect() error = %v, want nil", err)
	}
	if installed {
		t.Fatal("Detect() installed = true, want false")
	}
	if configPath != filepath.Join("/tmp/home", ".pi", "agent") {
		t.Fatalf("Detect() configPath = %q, want %q", configPath, filepath.Join("/tmp/home", ".pi", "agent"))
	}
	if !configFound {
		t.Fatal("Detect() configFound = false, want true when legacy dir exists")
	}
}
