package agents

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/gentleman-programming/gentle-ai/v2/internal/agents/capabilitymanifest"
	"github.com/gentleman-programming/gentle-ai/v2/internal/model"
	"github.com/gentleman-programming/gentle-ai/v2/internal/state"
	"github.com/gentleman-programming/gentle-ai/v2/internal/system"
)

// stubAdapter is a minimal Adapter implementation for discovery tests.
// It exposes a configurable GlobalConfigDir response.
type stubAdapter struct {
	agent     model.AgentID
	configDir string // value returned by GlobalConfigDir (may be empty)
}

func (s stubAdapter) Agent() model.AgentID    { return s.agent }
func (s stubAdapter) Tier() model.SupportTier { return model.TierFull }
func (s stubAdapter) CapabilityManifest() capabilitymanifest.AgentCapabilityManifest {
	return capabilitymanifest.MustForAgent(s.agent)
}
func (s stubAdapter) Detect(_ context.Context, _ string) (bool, string, string, bool, error) {
	info, err := os.Stat(s.configDir)
	return false, "", s.configDir, err == nil && info.IsDir(), nil
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
func (s stubAdapter) SupportsOutputStyles() bool {
	return s.CapabilityManifest().Features.OutputStyles
}
func (s stubAdapter) OutputStyleDir(_ string) string { return "" }
func (s stubAdapter) SupportsSlashCommands() bool {
	return s.CapabilityManifest().Features.SlashCommands
}
func (s stubAdapter) CommandsDir(_ string) string { return "" }
func (s stubAdapter) SupportsSubAgents() bool {
	return s.CapabilityManifest().Features.FileSubAgents
}
func (s stubAdapter) SubAgentsDir(_ string) string { return "" }
func (s stubAdapter) EmbeddedSubAgentsDir() string { return "" }
func (s stubAdapter) SupportsSkills() bool {
	return s.CapabilityManifest().Features.Skills
}
func (s stubAdapter) SupportsSystemPrompt() bool {
	return s.CapabilityManifest().Features.SystemPrompt
}
func (s stubAdapter) SupportsMCP() bool {
	return s.CapabilityManifest().Features.MCP
}

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

// ─── DiscoverSelected ────────────────────────────────────────────────────

// writeInstalledAgentsState persists an install state listing only ids.
func writeInstalledAgentsState(t *testing.T, home string, ids ...model.AgentID) {
	t.Helper()
	installed := make([]string, 0, len(ids))
	for _, id := range ids {
		installed = append(installed, string(id))
	}
	if err := state.Write(home, state.InstallState{InstalledAgents: installed}); err != nil {
		t.Fatalf("state.Write: %v", err)
	}
}

// TestDiscoverSelected_HonoursSelectionInBothDirections is the regression test
// Scope is the intersection: an agent installed but never
// selected is excluded (the reported bug — Codex and Cursor were rewritten on
// every sync), and an agent selected but not installed is excluded too, so
// selection never conjures a config directory that is not there.
func TestDiscoverSelected_HonoursSelectionInBothDirections(t *testing.T) {
	home := t.TempDir()

	opencodeDir := filepath.Join(home, ".config", "opencode")
	codexDir := filepath.Join(home, ".codex")
	cursorDir := filepath.Join(home, ".cursor")
	for _, dir := range []string{opencodeDir, codexDir, cursorDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("MkdirAll(%q): %v", dir, err)
		}
	}
	// .claude is selected below but deliberately never created on disk.
	writeInstalledAgentsState(t, home, model.AgentOpenCode, model.AgentClaudeCode)

	reg := newStubRegistry(t,
		stubAdapter{agent: model.AgentOpenCode, configDir: opencodeDir},
		stubAdapter{agent: model.AgentCodex, configDir: codexDir},
		stubAdapter{agent: model.AgentCursor, configDir: cursorDir},
		stubAdapter{agent: model.AgentClaudeCode, configDir: filepath.Join(home, ".claude")},
	)

	got := DiscoverSelected(reg, home)

	if len(got) != 1 || got[0].ID != model.AgentOpenCode {
		t.Fatalf("DiscoverSelected() = %v, want only %q", got, model.AgentOpenCode)
	}
}

// TestReadSelectionScope is the semantic matrix shared by every selection
// consumer. Only an explicit configured-empty selection is authoritative; missing
// and incidental state retain filesystem fallback, while unreadable state fails
// closed.
func TestReadSelectionScope(t *testing.T) {
	now := time.Now()
	tests := []struct {
		name      string
		state     *state.InstallState
		malformed bool
		wantMode  SelectionScopeMode
		wantIDs   []model.AgentID
	}{
		{name: "missing state falls back to filesystem", wantMode: SelectionScopeFilesystemFallback},
		{name: "unreadable state fails closed", malformed: true, wantMode: SelectionScopeUnavailable},
		{name: "configured empty selection is authoritative", state: &state.InstallState{SelectionConfigured: true}, wantMode: SelectionScopeConfigured},
		{name: "cooldown state falls back to filesystem", state: &state.InstallState{LastUpdateCheck: &now}, wantMode: SelectionScopeFilesystemFallback},
		{name: "non-empty selection is authoritative", state: &state.InstallState{InstalledAgents: []string{"opencode", "codex"}}, wantMode: SelectionScopeConfigured, wantIDs: []model.AgentID{model.AgentOpenCode, model.AgentCodex}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			home := t.TempDir()
			if tt.malformed {
				if err := os.MkdirAll(filepath.Dir(state.Path(home)), 0o755); err != nil {
					t.Fatalf("MkdirAll: %v", err)
				}
				if err := os.WriteFile(state.Path(home), []byte("{not json"), 0o644); err != nil {
					t.Fatalf("WriteFile: %v", err)
				}
			} else if tt.state != nil {
				if err := state.Write(home, *tt.state); err != nil {
					t.Fatalf("state.Write: %v", err)
				}
			}

			got := ReadSelectionScope(home)
			if got.Mode != tt.wantMode || len(got.AgentIDs) != len(tt.wantIDs) {
				t.Fatalf("ReadSelectionScope() = %+v, want mode %v and IDs %v", got, tt.wantMode, tt.wantIDs)
			}
			for i, want := range tt.wantIDs {
				if got.AgentIDs[i] != want {
					t.Errorf("ReadSelectionScope().AgentIDs[%d] = %q, want %q", i, got.AgentIDs[i], want)
				}
			}
		})
	}
}
