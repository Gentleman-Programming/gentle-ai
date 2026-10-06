package conductor

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v4/internal/model"
	"github.com/gentleman-programming/gentle-ai/v4/internal/system"
)

func TestDetect(t *testing.T) {
	tests := []struct {
		name            string
		stat            statResult
		wantInstalled   bool
		wantConfigFound bool
		wantErr         bool
	}{
		{
			name:            "config directory found",
			stat:            statResult{isDir: true},
			wantInstalled:   true,
			wantConfigFound: true,
		},
		{
			name: "config directory missing",
			stat: statResult{err: os.ErrNotExist},
		},
		{
			name:            "config path exists as file",
			stat:            statResult{isDir: false},
			wantInstalled:   false,
			wantConfigFound: false,
		},
		{
			name:    "stat error propagates",
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
			homeDir := filepath.Join(string(filepath.Separator), "home", "test")

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

			if binaryPath != "" {
				t.Fatalf("Detect() binaryPath = %q, want empty (Conductor has no PATH binary)", binaryPath)
			}

			wantConfigPath := filepath.Join(homeDir, ".conductor")
			if configPath != wantConfigPath {
				t.Fatalf("Detect() configPath = %q, want %q", configPath, wantConfigPath)
			}

			if configFound != tt.wantConfigFound {
				t.Fatalf("Detect() configFound = %v, want %v", configFound, tt.wantConfigFound)
			}
		})
	}
}

func TestDetectRealFilesystem(t *testing.T) {
	home := t.TempDir()
	a := NewAdapter()

	installed, _, configPath, configFound, err := a.Detect(context.Background(), home)
	if err != nil {
		t.Fatalf("Detect() error = %v", err)
	}
	if installed || configFound {
		t.Fatalf("Detect() = (%v, %v) for empty home, want (false, false)", installed, configFound)
	}
	if configPath != ConfigPath(home) {
		t.Fatalf("Detect() configPath = %q, want %q", configPath, ConfigPath(home))
	}

	if err := os.MkdirAll(ConfigPath(home), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	installed, _, _, configFound, err = a.Detect(context.Background(), home)
	if err != nil {
		t.Fatalf("Detect() error = %v", err)
	}
	if !installed || !configFound {
		t.Fatalf("Detect() = (%v, %v) with %s present, want (true, true)", installed, configFound, ConfigPath(home))
	}
}

func TestInstallCommand(t *testing.T) {
	a := NewAdapter()

	commands, err := a.InstallCommand(system.PlatformProfile{})
	if err == nil {
		t.Fatalf("InstallCommand() error = nil, want non-installable error")
	}
	if commands != nil {
		t.Fatalf("InstallCommand() commands = %v, want nil", commands)
	}

	var notInstallable AgentNotInstallableError
	if !errors.As(err, &notInstallable) {
		t.Fatalf("InstallCommand() error type = %T, want AgentNotInstallableError", err)
	}
	if got := err.Error(); !strings.Contains(got, "must be installed manually") {
		t.Fatalf("InstallCommand() error = %q, want message containing 'must be installed manually'", got)
	}
}

func TestWritePathsAreEmpty(t *testing.T) {
	a := NewAdapter()
	homeDir := filepath.Join(string(filepath.Separator), "home", "test")

	// Gentle AI performs no writes for Conductor, so every write-target path
	// must stay empty and fail closed.
	tests := []struct {
		name string
		got  string
	}{
		{"SystemPromptDir", a.SystemPromptDir(homeDir)},
		{"SystemPromptFile", a.SystemPromptFile(homeDir)},
		{"SkillsDir", a.SkillsDir(homeDir)},
		{"SettingsPath", a.SettingsPath(homeDir)},
		{"MCPConfigPath (engram)", a.MCPConfigPath(homeDir, "engram")},
		{"MCPConfigPath (context7)", a.MCPConfigPath(homeDir, "context7")},
		{"OutputStyleDir", a.OutputStyleDir(homeDir)},
		{"CommandsDir", a.CommandsDir(homeDir)},
		{"SubAgentsDir", a.SubAgentsDir(homeDir)},
		{"EmbeddedSubAgentsDir", a.EmbeddedSubAgentsDir()},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got != "" {
				t.Fatalf("%s = %q, want empty", tt.name, tt.got)
			}
		})
	}

	if got := a.GlobalConfigDir(homeDir); got != ConfigPath(homeDir) {
		t.Fatalf("GlobalConfigDir() = %q, want %q", got, ConfigPath(homeDir))
	}
}

func TestCapabilitiesAllFalse(t *testing.T) {
	a := NewAdapter()

	if got := a.Agent(); got != model.AgentConductor {
		t.Fatalf("Agent() = %q, want %q", got, model.AgentConductor)
	}

	if got := a.Tier(); got != model.TierFull {
		t.Fatalf("Tier() = %v, want TierFull", got)
	}

	if a.SupportsOutputStyles() {
		t.Fatalf("SupportsOutputStyles() = true, want false")
	}

	if a.SupportsSlashCommands() {
		t.Fatalf("SupportsSlashCommands() = true, want false")
	}

	if a.SupportsSubAgents() {
		t.Fatalf("SupportsSubAgents() = true, want false")
	}

	if a.SupportsSkills() {
		t.Fatalf("SupportsSkills() = true, want false")
	}

	if a.SupportsSystemPrompt() {
		t.Fatalf("SupportsSystemPrompt() = true, want false")
	}

	if a.SupportsMCP() {
		t.Fatalf("SupportsMCP() = true, want false")
	}

	if got := a.SystemPromptStrategy(); got != model.StrategyMarkdownSections {
		t.Fatalf("SystemPromptStrategy() = %v, want StrategyMarkdownSections", got)
	}

	if got := a.MCPStrategy(); got != model.StrategySeparateMCPFiles {
		t.Fatalf("MCPStrategy() = %v, want StrategySeparateMCPFiles", got)
	}
}

func TestCapabilityManifestMatchesCanonical(t *testing.T) {
	a := NewAdapter()

	manifest := a.CapabilityManifest()
	if err := manifest.Validate(); err != nil {
		t.Fatalf("CapabilityManifest() failed canonical validation: %v", err)
	}

	features := manifest.Features
	if features.OutputStyles || features.SlashCommands || features.FileSubAgents ||
		features.Skills || features.SystemPrompt || features.MCP || features.Workflows {
		t.Fatalf("capability manifest claims write features for Conductor: %+v", features)
	}

	if manifest.Advertises("gentle-ai.review-transport/v1") {
		t.Fatal("Conductor must not advertise the review transport contract")
	}
	if manifest.Advertises("gentle-ai.immutable-review-executor/v1") {
		t.Fatal("Conductor must not advertise the immutable review executor contract")
	}
}
