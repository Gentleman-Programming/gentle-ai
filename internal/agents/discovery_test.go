package agents

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/gentleman-programming/gentle-ai/internal/model"
	"github.com/gentleman-programming/gentle-ai/internal/system"
)

// stubAdapter is a minimal Adapter implementation for discovery tests.
// It exposes a configurable GlobalConfigDir response.
type stubAdapter struct {
	agent     model.AgentID
	configDir string // value returned by GlobalConfigDir (may be empty)
}

func (s stubAdapter) Agent() model.AgentID      { return s.agent }
func (s stubAdapter) Tier() model.SupportTier   { return model.TierFull }
func (s stubAdapter) SupportsAutoInstall() bool { return false }
func (s stubAdapter) Detect(_ context.Context, _ string) (bool, string, string, bool, error) {
	return false, "", "", false, nil
}
func (s stubAdapter) InstallCommand(system.PlatformProfile) ([][]string, error) { return nil, nil }

// GlobalConfigDir returns the pre-configured dir for the stub — ignores homeDir
// so tests can control the path exactly.
func (s stubAdapter) GlobalConfigDir(_ string) string { return s.configDir }

func (s stubAdapter) SystemPromptDir(_ string) string  { return "" }
func (s stubAdapter) SystemPromptFile(_ string) string { return "" }
func (s stubAdapter) SkillsDir(_ string) string        { return "" }
func (s stubAdapter) SettingsPath(_ string) string     { return "" }
func (s stubAdapter) SystemPromptStrategy() model.SystemPromptStrategy {
	return model.StrategyMarkdownSections
}
func (s stubAdapter) MCPStrategy() model.MCPStrategy          { return model.StrategySeparateMCPFiles }
func (s stubAdapter) MCPConfigPath(_ string, _ string) string { return "" }
func (s stubAdapter) SupportsOutputStyles() bool              { return false }
func (s stubAdapter) OutputStyleDir(_ string) string          { return "" }
func (s stubAdapter) SupportsSlashCommands() bool             { return false }
func (s stubAdapter) CommandsDir(_ string) string             { return "" }
func (s stubAdapter) SupportsSkills() bool                    { return true }
func (s stubAdapter) SupportsSystemPrompt() bool              { return true }
func (s stubAdapter) SupportsMCP() bool                       { return true }

// newStubRegistry creates a Registry from stub adapters.
func newStubRegistry(t *testing.T, adapters ...stubAdapter) *Registry {
	t.Helper()
	ifaces := make([]Adapter, len(adapters))
	for i, a := range adapters {
		ifaces[i] = a
	}
	r, err := NewRegistry(ifaces...)
	if err != nil {
		t.Fatalf("newStubRegistry: %v", err)
	}
	return r
}

// ─── DiscoverInstalled ────────────────────────────────────────────────────

// TestDiscoverInstalled_ReturnsOnlyInstalledAgents verifies that only agents
// whose GlobalConfigDir exists on disk are returned.
func TestDiscoverInstalled_ReturnsOnlyInstalledAgents(t *testing.T) {
	home := t.TempDir()

	claudeDir := filepath.Join(home, ".claude")
	if err := os.MkdirAll(claudeDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	// opencode dir intentionally NOT created.

	reg := newStubRegistry(t,
		stubAdapter{agent: model.AgentClaudeCode, configDir: claudeDir},
		stubAdapter{agent: model.AgentOpenCode, configDir: filepath.Join(home, ".config", "opencode")},
	)

	got := DiscoverInstalled(reg, home)

	if len(got) != 1 {
		t.Fatalf("DiscoverInstalled() returned %d agents, want 1; got %v", len(got), got)
	}
	if got[0].ID != model.AgentClaudeCode {
		t.Errorf("DiscoverInstalled() agent = %q, want %q", got[0].ID, model.AgentClaudeCode)
	}
	if got[0].ConfigDir != claudeDir {
		t.Errorf("DiscoverInstalled() ConfigDir = %q, want %q", got[0].ConfigDir, claudeDir)
	}
}

// TestDiscoverInstalled_EmptyGlobalConfigDirIsSkipped verifies that an adapter
// returning an empty string from GlobalConfigDir is silently excluded.
func TestDiscoverInstalled_EmptyGlobalConfigDirIsSkipped(t *testing.T) {
	home := t.TempDir()

	reg := newStubRegistry(t,
		stubAdapter{agent: model.AgentClaudeCode, configDir: ""},
	)

	got := DiscoverInstalled(reg, home)

	if len(got) != 0 {
		t.Errorf("DiscoverInstalled() expected empty result for empty GlobalConfigDir, got %v", got)
	}
}

// TestDiscoverInstalled_MissingDirIsSkipped verifies that a non-existent directory
// is silently excluded — not an error.
func TestDiscoverInstalled_MissingDirIsSkipped(t *testing.T) {
	home := t.TempDir()
	// Config dir not created on disk.

	reg := newStubRegistry(t,
		stubAdapter{agent: model.AgentOpenCode, configDir: filepath.Join(home, ".config", "opencode")},
	)

	got := DiscoverInstalled(reg, home)

	if len(got) != 0 {
		t.Errorf("DiscoverInstalled() expected empty result for missing dir, got %v", got)
	}
}

// TestDiscoverInstalled_MultipleInstalled verifies that all agents with existing
// config dirs are returned.
func TestDiscoverInstalled_MultipleInstalled(t *testing.T) {
	home := t.TempDir()

	claudeDir := filepath.Join(home, ".claude")
	opencodeDir := filepath.Join(home, ".config", "opencode")

	for _, dir := range []string{claudeDir, opencodeDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("MkdirAll(%q): %v", dir, err)
		}
	}

	reg := newStubRegistry(t,
		stubAdapter{agent: model.AgentClaudeCode, configDir: claudeDir},
		stubAdapter{agent: model.AgentOpenCode, configDir: opencodeDir},
		stubAdapter{agent: model.AgentGeminiCLI, configDir: filepath.Join(home, ".gemini")}, // not created
	)

	got := DiscoverInstalled(reg, home)

	if len(got) != 2 {
		t.Fatalf("DiscoverInstalled() returned %d agents, want 2; got %v", len(got), got)
	}
}

// TestDiscoverInstalled_EmptyRegistryReturnsEmpty verifies that an empty registry
// yields an empty result without panicking.
func TestDiscoverInstalled_EmptyRegistryReturnsEmpty(t *testing.T) {
	home := t.TempDir()

	reg := newStubRegistry(t) // no adapters

	got := DiscoverInstalled(reg, home)

	if len(got) != 0 {
		t.Errorf("DiscoverInstalled() expected empty slice for empty registry, got %v", got)
	}
}

// ─── ConfigRootsForBackup ────────────────────────────────────────────────

// TestConfigRootsForBackup_ReturnsInstalledDirs verifies that only dirs for
// installed agents are returned.
func TestConfigRootsForBackup_ReturnsInstalledDirs(t *testing.T) {
	home := t.TempDir()

	claudeDir := filepath.Join(home, ".claude")
	if err := os.MkdirAll(claudeDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	reg := newStubRegistry(t,
		stubAdapter{agent: model.AgentClaudeCode, configDir: claudeDir},
		stubAdapter{agent: model.AgentOpenCode, configDir: filepath.Join(home, ".config", "opencode")}, // missing
	)

	roots := ConfigRootsForBackup(reg, home)

	if len(roots) != 1 {
		t.Fatalf("ConfigRootsForBackup() returned %d roots, want 1; got %v", len(roots), roots)
	}
	if roots[0] != claudeDir {
		t.Errorf("ConfigRootsForBackup() root = %q, want %q", roots[0], claudeDir)
	}
}

// TestConfigRootsForBackup_DeduplicatesSharedDirs verifies that when two agents
// share the same GlobalConfigDir, only one entry appears in the result.
func TestConfigRootsForBackup_DeduplicatesSharedDirs(t *testing.T) {
	home := t.TempDir()

	sharedDir := filepath.Join(home, ".shared-config")
	if err := os.MkdirAll(sharedDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	reg := newStubRegistry(t,
		stubAdapter{agent: model.AgentClaudeCode, configDir: sharedDir},
		stubAdapter{agent: model.AgentOpenCode, configDir: sharedDir},
	)

	roots := ConfigRootsForBackup(reg, home)

	if len(roots) != 1 {
		t.Fatalf("ConfigRootsForBackup() returned %d roots with duplicate dir, want 1; got %v", len(roots), roots)
	}
	if roots[0] != sharedDir {
		t.Errorf("ConfigRootsForBackup() root = %q, want %q", roots[0], sharedDir)
	}
}

// TestConfigRootsForBackup_EmptyWhenNoAgentsInstalled verifies that an empty
// result is returned when no agent config dirs exist.
func TestConfigRootsForBackup_EmptyWhenNoAgentsInstalled(t *testing.T) {
	home := t.TempDir()

	reg := newStubRegistry(t,
		stubAdapter{agent: model.AgentClaudeCode, configDir: filepath.Join(home, ".claude")}, // not created
	)

	roots := ConfigRootsForBackup(reg, home)

	if len(roots) != 0 {
		t.Errorf("ConfigRootsForBackup() expected empty, got %v", roots)
	}
}

// TestConfigRootsForBackup_NilSafeOnEmptyRegistry verifies no panic on empty reg.
func TestConfigRootsForBackup_NilSafeOnEmptyRegistry(t *testing.T) {
	home := t.TempDir()
	reg := newStubRegistry(t)

	roots := ConfigRootsForBackup(reg, home)

	if roots == nil {
		t.Errorf("ConfigRootsForBackup() returned nil, want non-nil slice")
	}
	if len(roots) != 0 {
		t.Errorf("ConfigRootsForBackup() expected empty for empty registry, got %v", roots)
	}
}

// ─── Integration: DefaultRegistry ────────────────────────────────────────

// TestDiscoverInstalled_WithDefaultRegistryAndRealFS verifies that DiscoverInstalled
// works correctly with the real default registry and a real temp directory.
// Only agents whose config dirs are created are returned.
func TestDiscoverInstalled_WithDefaultRegistryAndRealFS(t *testing.T) {
	home := t.TempDir()

	// Create claude-code config dir only.
	claudeDir := filepath.Join(home, ".claude")
	if err := os.MkdirAll(claudeDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	reg, err := NewDefaultRegistry()
	if err != nil {
		t.Fatalf("NewDefaultRegistry: %v", err)
	}

	got := DiscoverInstalled(reg, home)

	// Exactly one agent should be returned (claude-code).
	if len(got) != 1 {
		t.Fatalf("DiscoverInstalled() with real registry returned %d agents, want 1; got %v", len(got), got)
	}
	if got[0].ID != model.AgentClaudeCode {
		t.Errorf("DiscoverInstalled() agent = %q, want %q", got[0].ID, model.AgentClaudeCode)
	}
	if got[0].ConfigDir != claudeDir {
		t.Errorf("DiscoverInstalled() ConfigDir = %q, want %q", got[0].ConfigDir, claudeDir)
	}
}

// TestConfigRootsForBackup_WithDefaultRegistryCoversCreatedDirs verifies that
// ConfigRootsForBackup returns exactly the dirs created on disk.
func TestConfigRootsForBackup_WithDefaultRegistryCoversCreatedDirs(t *testing.T) {
	home := t.TempDir()

	// Create two agent config dirs.
	dirs := []string{
		filepath.Join(home, ".claude"),
		filepath.Join(home, ".config", "opencode"),
	}
	for _, d := range dirs {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatalf("MkdirAll(%q): %v", d, err)
		}
	}

	reg, err := NewDefaultRegistry()
	if err != nil {
		t.Fatalf("NewDefaultRegistry: %v", err)
	}

	roots := ConfigRootsForBackup(reg, home)

	// Must contain exactly the two dirs we created.
	if len(roots) != 2 {
		t.Fatalf("ConfigRootsForBackup() returned %d roots, want 2; got %v", len(roots), roots)
	}

	rootSet := make(map[string]struct{}, len(roots))
	for _, r := range roots {
		rootSet[r] = struct{}{}
	}
	for _, want := range dirs {
		if _, ok := rootSet[want]; !ok {
			t.Errorf("ConfigRootsForBackup() missing %q in roots %v", want, roots)
		}
	}
}
