package minimaxcode

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

// TestDetect covers the two independent install signals. Desktop writes the
// data directory without exposing the CLI binary, and the CLI does the
// opposite on a fresh machine, so both combinations must be recognised.
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
			name:            "cli binary and config directory found",
			lookPathPath:    "/usr/local/bin/mcode",
			stat:            statResult{isDir: true},
			wantInstalled:   true,
			wantBinaryPath:  "/usr/local/bin/mcode",
			wantConfigFound: true,
		},
		{
			name:        "desktop install: config only, no binary on PATH",
			lookPathErr: errors.New("not found"),
			stat:        statResult{isDir: true},
			// installed stays false: the Desktop app has no CLI binary, but
			// configFound is what makes the agent appear in the installer.
			wantInstalled:   false,
			wantBinaryPath:  "",
			wantConfigFound: true,
		},
		{
			name:            "cli install: binary found, no config directory yet",
			lookPathPath:    "/opt/bin/mcode",
			stat:            statResult{err: os.ErrNotExist},
			wantInstalled:   true,
			wantBinaryPath:  "/opt/bin/mcode",
			wantConfigFound: false,
		},
		{
			name:            "neither signal present",
			lookPathErr:     errors.New("not found"),
			stat:            statResult{err: os.ErrNotExist},
			wantInstalled:   false,
			wantBinaryPath:  "",
			wantConfigFound: false,
		},
		{
			name:        "stat error propagates",
			stat:        statResult{err: errors.New("permission denied")},
			lookPathErr: errors.New("not found"),
			wantErr:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			home := t.TempDir()
			a := &Adapter{
				lookPath: func(string) (string, error) {
					if tt.lookPathErr != nil {
						return "", tt.lookPathErr
					}
					return tt.lookPathPath, nil
				},
				statPath: func(string) statResult { return tt.stat },
			}

			installed, binaryPath, configPath, configFound, err := a.Detect(context.Background(), home)
			if (err != nil) != tt.wantErr {
				t.Fatalf("Detect() error = %v, wantErr %t", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			if installed != tt.wantInstalled {
				t.Errorf("installed = %t, want %t", installed, tt.wantInstalled)
			}
			if binaryPath != tt.wantBinaryPath {
				t.Errorf("binaryPath = %q, want %q", binaryPath, tt.wantBinaryPath)
			}
			if configPath != ConfigPath(home) {
				t.Errorf("configPath = %q, want %q", configPath, ConfigPath(home))
			}
			if configFound != tt.wantConfigFound {
				t.Errorf("configFound = %t, want %t", configFound, tt.wantConfigFound)
			}
		})
	}
}

// TestDetectUsesMCLIName pins the public terminal command so a rename upstream
// surfaces here instead of silently disabling CLI detection.
func TestDetectUsesMCLIName(t *testing.T) {
	if binaryName != "mcode" {
		t.Fatalf("binaryName = %q, want %q", binaryName, "mcode")
	}

	var asked string
	a := &Adapter{
		lookPath: func(name string) (string, error) {
			asked = name
			return "", errors.New("not found")
		},
		statPath: func(string) statResult { return statResult{err: os.ErrNotExist} },
	}
	if _, _, _, _, err := a.Detect(context.Background(), t.TempDir()); err != nil {
		t.Fatalf("Detect() error = %v", err)
	}
	if asked != "mcode" {
		t.Errorf("lookPath called with %q, want %q", asked, "mcode")
	}
}

func TestAgentIdentity(t *testing.T) {
	a := NewAdapter()

	if got := a.Agent(); got != model.AgentMiniMaxCode {
		t.Fatalf("Agent() = %v, want %v", got, model.AgentMiniMaxCode)
	}
	if got := a.Tier(); got != model.TierFull {
		t.Fatalf("Tier() = %v, want %v", got, model.TierFull)
	}
}

func TestInstallCommandIsManual(t *testing.T) {
	a := NewAdapter()

	_, err := a.InstallCommand(system.PlatformProfile{})
	if err == nil {
		t.Fatal("InstallCommand() error = nil, want AgentNotInstallableError")
	}

	var notInstallable AgentNotInstallableError
	if !errors.As(err, &notInstallable) {
		t.Fatalf("InstallCommand() error type = %T, want AgentNotInstallableError", err)
	}
	if notInstallable.Agent != model.AgentMiniMaxCode {
		t.Errorf("AgentNotInstallableError.Agent = %v, want %v", notInstallable.Agent, model.AgentMiniMaxCode)
	}
}

// TestConfigPaths pins every managed write target inside the shared
// ~/.minimax data directory.
func TestConfigPaths(t *testing.T) {
	home := t.TempDir()
	a := NewAdapter()
	root := filepath.Join(home, ".minimax")

	if got := a.GlobalConfigDir(home); got != root {
		t.Errorf("GlobalConfigDir() = %q, want %q", got, root)
	}
	if got := a.SkillsDir(home); got != filepath.Join(root, "skills") {
		t.Errorf("SkillsDir() = %q, want skills under the data dir", got)
	}
	if got := a.SystemPromptFile(home); got != filepath.Join(root, "agents", ManagedAgentName, "agent.md") {
		t.Errorf("SystemPromptFile() = %q, want a namespaced agent.md", got)
	}
	if got := a.SubAgentsDir(home); got != filepath.Join(root, "agents") {
		t.Errorf("SubAgentsDir() = %q, want agents under the data dir", got)
	}
}

// TestManagedWritesAvoidBuiltinDirectories is the guard that matters most here:
// the runtime owns .builtin-skills/ and agents/.builtin/, and Gentle AI must
// never write into either.
func TestManagedWritesAvoidBuiltinDirectories(t *testing.T) {
	home := t.TempDir()
	a := NewAdapter()

	for _, path := range []string{a.SkillsDir(home), a.SystemPromptFile(home), a.SubAgentsDir(home)} {
		if strings.Contains(filepath.ToSlash(path), ".builtin-skills") {
			t.Errorf("managed path %q writes into the runtime's .builtin-skills", path)
		}
		if strings.Contains(filepath.ToSlash(path), ".builtin/") {
			t.Errorf("managed path %q writes into the runtime's built-in agents", path)
		}
	}
}

// TestMCPConfigPathIsShared pins the single-file MCP layout: every server lands
// in one ~/.minimax/mcp.json, so serverName must be ignored.
func TestMCPConfigPathIsShared(t *testing.T) {
	home := t.TempDir()
	a := NewAdapter()
	want := filepath.Join(home, ".minimax", "mcp.json")

	if got := a.MCPConfigPath(home, "context7"); got != want {
		t.Errorf("MCPConfigPath(context7) = %q, want %q", got, want)
	}
	if got := a.MCPConfigPath(home, "engram"); got != want {
		t.Errorf("MCPConfigPath(engram) = %q, want %q", got, want)
	}
}

func TestMCPStrategyWritesDedicatedJSON(t *testing.T) {
	a := NewAdapter()

	if got := a.MCPStrategy(); got != model.StrategyMCPConfigFile {
		t.Errorf("MCPStrategy() = %v, want %v", got, model.StrategyMCPConfigFile)
	}
}

func TestSystemPromptStrategy(t *testing.T) {
	a := NewAdapter()

	if got := a.SystemPromptStrategy(); got != model.StrategyMarkdownSections {
		t.Errorf("SystemPromptStrategy() = %v, want %v", got, model.StrategyMarkdownSections)
	}
}

// TestSettingsPathIsEmptyBecauseConfigIsNotManagedJSON pins the reason the
// adapter exposes no settings surface. config.yaml is YAML, and pointing the
// uninstaller's JSON settings cleaner at it makes `uninstall` fail with an
// unmarshal error instead of removing the agent. Returning "" opts the agent
// out of that cleaner entirely; MCP, skills, and the prompt all have their own
// dedicated paths.
func TestSettingsPathIsEmptyBecauseConfigIsNotManagedJSON(t *testing.T) {
	a := NewAdapter()

	if got := a.SettingsPath(t.TempDir()); got != "" {
		t.Errorf("SettingsPath() = %q, want empty so the JSON settings cleaner is not pointed at config.yaml", got)
	}
}

// TestSupportedCapabilitiesMatchManifest keeps the adapter's optional
// projections consistent with its declared manifest.
func TestSupportedCapabilitiesMatchManifest(t *testing.T) {
	a := NewAdapter()
	features := a.CapabilityManifest().Features

	if got := a.SupportsSkills(); got != features.Skills {
		t.Errorf("SupportsSkills() = %t, manifest %t", got, features.Skills)
	}
	if got := a.SupportsSystemPrompt(); got != features.SystemPrompt {
		t.Errorf("SupportsSystemPrompt() = %t, manifest %t", got, features.SystemPrompt)
	}
	if got := a.SupportsMCP(); got != features.MCP {
		t.Errorf("SupportsMCP() = %t, manifest %t", got, features.MCP)
	}
	if got := a.SupportsSubAgents(); got != features.FileSubAgents {
		t.Errorf("SupportsSubAgents() = %t, manifest %t", got, features.FileSubAgents)
	}
	if got := a.SupportsOutputStyles(); got != features.OutputStyles {
		t.Errorf("SupportsOutputStyles() = %t, manifest %t", got, features.OutputStyles)
	}
	if got := a.SupportsSlashCommands(); got != features.SlashCommands {
		t.Errorf("SupportsSlashCommands() = %t, manifest %t", got, features.SlashCommands)
	}
}

// TestCapabilityManifestIsCanonical rejects a manifest that was edited into an
// unsupported shape.
func TestCapabilityManifestIsCanonical(t *testing.T) {
	a := NewAdapter()

	manifest := a.CapabilityManifest()
	if err := manifest.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
	if !manifest.Features.Skills || !manifest.Features.SystemPrompt || !manifest.Features.MCP {
		t.Error("manifest must advertise skills, system prompt, and MCP")
	}
}

func TestConfigPath(t *testing.T) {
	got := ConfigPath("/home/tester")
	want := filepath.Join("/home/tester", ".minimax")
	if got != want {
		t.Errorf("ConfigPath() = %q, want %q", got, want)
	}
}

func TestDefaultStat(t *testing.T) {
	dir := t.TempDir()

	if result := defaultStat(dir); result.err != nil || !result.isDir {
		t.Errorf("defaultStat(dir) = %+v, want isDir true and no error", result)
	}

	file := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(file, []byte("a: b\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if result := defaultStat(file); result.err != nil || result.isDir {
		t.Errorf("defaultStat(file) = %+v, want isDir false and no error", result)
	}

	if result := defaultStat(filepath.Join(dir, "missing")); !os.IsNotExist(result.err) {
		t.Errorf("defaultStat(missing) err = %v, want a not-exist error", result.err)
	}
}

// TestNewAdapterWiresSeams guards against a constructor that leaves the seams
// nil and would panic on first use.
func TestNewAdapterWiresSeams(t *testing.T) {
	a := NewAdapter()

	if a.lookPath == nil {
		t.Error("NewAdapter() left lookPath nil")
	}
	if a.statPath == nil {
		t.Error("NewAdapter() left statPath nil")
	}
}
