package kimi_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/gentleman-programming/gentle-ai/internal/agents"
	"github.com/gentleman-programming/gentle-ai/internal/agents/kimi"
)

// Verify Adapter implements PluginInstaller at compile time.
var _ agents.PluginInstaller = (*kimi.Adapter)(nil)

func TestPluginDir_KimiCode(t *testing.T) {
	tmpDir := t.TempDir()
	kimiCodeDir := filepath.Join(tmpDir, ".kimi-code")
	if err := os.MkdirAll(kimiCodeDir, 0o755); err != nil {
		t.Fatal(err)
	}

	a := newKimiCodeAdapter(t, tmpDir)
	got := a.PluginDir(tmpDir)
	expected := filepath.Join(tmpDir, ".kimi-code", "plugins", "managed", "gentle-ai")
	if got != expected {
		t.Errorf("PluginDir() = %v, want %v", got, expected)
	}
}

func TestPluginManifestPath_KimiCode(t *testing.T) {
	tmpDir := t.TempDir()
	kimiCodeDir := filepath.Join(tmpDir, ".kimi-code")
	if err := os.MkdirAll(kimiCodeDir, 0o755); err != nil {
		t.Fatal(err)
	}

	a := newKimiCodeAdapter(t, tmpDir)
	got := a.PluginManifestPath(tmpDir)
	expected := filepath.Join(tmpDir, ".kimi-code", "plugins", "managed", "gentle-ai", "kimi.plugin.json")
	if got != expected {
		t.Errorf("PluginManifestPath() = %v, want %v", got, expected)
	}
}

func TestInstallPlugin_CreatesDirAndManifest(t *testing.T) {
	tmpDir := t.TempDir()
	kimiCodeDir := filepath.Join(tmpDir, ".kimi-code")
	if err := os.MkdirAll(kimiCodeDir, 0o755); err != nil {
		t.Fatal(err)
	}

	a := newKimiCodeAdapter(t, tmpDir)
	err := a.InstallPlugin(tmpDir, "1.2.3")
	if err != nil {
		t.Fatalf("InstallPlugin() error = %v", err)
	}

	// Verify directory was created.
	pluginDir := filepath.Join(tmpDir, ".kimi-code", "plugins", "managed", "gentle-ai")
	info, err := os.Stat(pluginDir)
	if err != nil {
		t.Fatalf("plugin dir not created: %v", err)
	}
	if !info.IsDir() {
		t.Errorf("plugin dir path exists but is not a directory")
	}

	// Verify manifest was written.
	manifestPath := filepath.Join(pluginDir, "kimi.plugin.json")
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatalf("manifest not written: %v", err)
	}

	var manifest kimi.KimiPluginManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatalf("manifest JSON invalid: %v", err)
	}

	// Check required fields.
	if manifest.Name != "gentle-ai" {
		t.Errorf("manifest.Name = %q, want %q", manifest.Name, "gentle-ai")
	}
	if manifest.Version != "1.2.3" {
		t.Errorf("manifest.Version = %q, want %q", manifest.Version, "1.2.3")
	}
	if manifest.Description == "" {
		t.Error("manifest.Description is empty")
	}
	if manifest.Skills != "./skills/" {
		t.Errorf("manifest.Skills = %q, want %q", manifest.Skills, "./skills/")
	}
	if manifest.SessionStart == nil || manifest.SessionStart.Skill != "sdd-init" {
		t.Errorf("manifest.SessionStart.Skill = %v, want %q",
			manifest.SessionStart, "sdd-init")
	}
	// The plugin does NOT own the Engram MCP server. Kimi Code namespaces
	// plugin MCP servers as "plugin-<id>:<name>", which would expose Engram as
	// "plugin-gentle-ai:engram" instead of the canonical "engram" name used by
	// every other agent. Engram is provided by the Engram component via
	// ~/.kimi-code/mcp.json instead.
	if hasManifestKey(data, "mcpServers") {
		t.Error("manifest should not contain mcpServers")
	}
}

func TestInstallPlugin_VersionFromArg(t *testing.T) {
	tmpDir := t.TempDir()
	kimiCodeDir := filepath.Join(tmpDir, ".kimi-code")
	if err := os.MkdirAll(kimiCodeDir, 0o755); err != nil {
		t.Fatal(err)
	}

	a := newKimiCodeAdapter(t, tmpDir)
	if err := a.InstallPlugin(tmpDir, "9.9.9"); err != nil {
		t.Fatalf("InstallPlugin() error = %v", err)
	}

	data, err := os.ReadFile(a.PluginManifestPath(tmpDir))
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	var manifest kimi.KimiPluginManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatalf("manifest JSON invalid: %v", err)
	}
	if manifest.Version != "9.9.9" {
		t.Errorf("manifest.Version = %q, want %q", manifest.Version, "9.9.9")
	}
}

func TestPluginManifest_OverwritesExisting(t *testing.T) {
	tmpDir := t.TempDir()
	kimiCodeDir := filepath.Join(tmpDir, ".kimi-code")
	if err := os.MkdirAll(kimiCodeDir, 0o755); err != nil {
		t.Fatal(err)
	}

	a := newKimiCodeAdapter(t, tmpDir)

	// First call.
	if err := a.InstallPlugin(tmpDir, "1.0.0"); err != nil {
		t.Fatalf("InstallPlugin() first call error = %v", err)
	}

	// Overwrite with new version.
	if err := a.InstallPlugin(tmpDir, "2.0.0"); err != nil {
		t.Fatalf("InstallPlugin() second call error = %v", err)
	}

	data, err := os.ReadFile(a.PluginManifestPath(tmpDir))
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	var manifest kimi.KimiPluginManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatalf("manifest JSON invalid: %v", err)
	}
	if manifest.Version != "2.0.0" {
		t.Errorf("manifest.Version = %q after overwrite, want %q", manifest.Version, "2.0.0")
	}
}

// --- Phase 2: Version Guard Tests ---

func TestSkillsDir_PluginPath_KimiCode(t *testing.T) {
	tmpDir := t.TempDir()
	kimiCodeDir := filepath.Join(tmpDir, ".kimi-code")
	if err := os.MkdirAll(kimiCodeDir, 0o755); err != nil {
		t.Fatal(err)
	}

	a := newKimiCodeAdapter(t, tmpDir)
	got := a.SkillsDir(tmpDir)
	expected := filepath.Join(tmpDir, ".kimi-code", "plugins", "managed", "gentle-ai", "skills")
	if got != expected {
		t.Errorf("SkillsDir() kimi-code = %v, want %v", got, expected)
	}
}

func TestSkillsDir_LegacyPath(t *testing.T) {
	tmpDir := t.TempDir()
	// No .kimi-code dir → legacy path.
	a := newLegacyAdapter(tmpDir)
	got := a.SkillsDir(tmpDir)
	expected := filepath.Join(tmpDir, ".config", "agents", "skills")
	if got != expected {
		t.Errorf("SkillsDir() legacy = %v, want %v", got, expected)
	}
}

func TestAllSkillsDirs_PluginDirFirst_KimiCode(t *testing.T) {
	tmpDir := t.TempDir()
	kimiCodeDir := filepath.Join(tmpDir, ".kimi-code")
	if err := os.MkdirAll(kimiCodeDir, 0o755); err != nil {
		t.Fatal(err)
	}

	a := newKimiCodeAdapter(t, tmpDir)
	dirs := a.AllSkillsDirs(tmpDir)
	if len(dirs) != 4 {
		t.Fatalf("AllSkillsDirs() kimi-code returned %d dirs, want 4: %v", len(dirs), dirs)
	}

	expected := []string{
		filepath.Join(tmpDir, ".kimi-code", "plugins", "managed", "gentle-ai", "skills"),
		filepath.Join(tmpDir, ".kimi-code", "skills"),
		filepath.Join(tmpDir, ".agents", "skills"),
		filepath.Join(tmpDir, ".config", "agents", "skills"),
	}
	for i, want := range expected {
		if dirs[i] != want {
			t.Errorf("AllSkillsDirs()[%d] = %v, want %v", i, dirs[i], want)
		}
	}
}

func TestAllSkillsDirs_LegacyUnchanged(t *testing.T) {
	tmpDir := t.TempDir()
	a := newLegacyAdapter(tmpDir)
	dirs := a.AllSkillsDirs(tmpDir)
	if len(dirs) != 1 {
		t.Fatalf("AllSkillsDirs() legacy returned %d dirs, want 1: %v", len(dirs), dirs)
	}
	expected := filepath.Join(tmpDir, ".config", "agents", "skills")
	if dirs[0] != expected {
		t.Errorf("AllSkillsDirs()[0] = %v, want %v", dirs[0], expected)
	}
}

// newLegacyAdapter creates a test Adapter without the kimi-code layout (no .kimi-code dir).
func newLegacyAdapter(homeDir string) *kimi.Adapter {
	return kimi.NewTestAdapter(
		kimi.WithStatPath(func(path string) kimi.StatResult {
			return kimi.StatResult{Err: os.ErrNotExist}
		}),
		kimi.WithPathExists(func(path string) bool { return false }),
	)
}

// newKimiCodeAdapter creates a test Adapter with the kimi-code layout detected.
func newKimiCodeAdapter(t *testing.T, homeDir string) *kimi.Adapter {
	t.Helper()
	kimiCodeDir := filepath.Join(homeDir, ".kimi-code")
	return kimi.NewTestAdapter(
		kimi.WithStatPath(func(path string) kimi.StatResult {
			if path == kimiCodeDir {
				return kimi.StatResult{IsDir: true}
			}
			return kimi.StatResult{Err: os.ErrNotExist}
		}),
		kimi.WithPathExists(func(path string) bool { return path == kimiCodeDir }),
	)
}

// --- Phase 4: Edge Case Tests ---

func TestInstallPlugin_DirCreationFails(t *testing.T) {
	tmpDir := t.TempDir()
	kimiCodeDir := filepath.Join(tmpDir, ".kimi-code")
	if err := os.MkdirAll(kimiCodeDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// Make the plugins dir unwritable by creating a file in the way.
	blocker := filepath.Join(tmpDir, ".kimi-code", "plugins")
	if err := os.WriteFile(blocker, []byte("block"), 0o644); err != nil {
		t.Fatal(err)
	}

	a := newKimiCodeAdapter(t, tmpDir)
	err := a.InstallPlugin(tmpDir, "1.0.0")
	// Should return an error (not panic).
	if err == nil {
		t.Fatal("InstallPlugin() expected error when dir creation fails, got nil")
	}
}

func TestInstallPlugin_LegacyNoPluginDir(t *testing.T) {
	tmpDir := t.TempDir()
	// No .kimi-code dir — legacy adapter.
	a := newLegacyAdapter(tmpDir)

	// PluginDir still returns a path even for legacy (it's a pure path function).
	// It is derived from resolveConfigDir, so for legacy it falls back to ~/.kimi.
	got := a.PluginDir(tmpDir)
	expected := filepath.Join(tmpDir, ".kimi", "plugins", "managed", "gentle-ai")
	if got != expected {
		t.Errorf("PluginDir() legacy = %v, want %v", got, expected)
	}
}

func TestPluginManifest_NoMCPServers(t *testing.T) {
	tmpDir := t.TempDir()
	kimiCodeDir := filepath.Join(tmpDir, ".kimi-code")
	if err := os.MkdirAll(kimiCodeDir, 0o755); err != nil {
		t.Fatal(err)
	}

	a := newKimiCodeAdapter(t, tmpDir)
	if err := a.InstallPlugin(tmpDir, "1.0.0"); err != nil {
		t.Fatalf("InstallPlugin() error = %v", err)
	}

	data, err := os.ReadFile(a.PluginManifestPath(tmpDir))
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}

	var manifest kimi.KimiPluginManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatalf("manifest JSON invalid: %v", err)
	}

	if hasManifestKey(data, "mcpServers") {
		t.Error("manifest should not contain mcpServers")
	}
}

func TestInstallPlugin_CreatesSkillsSubdir(t *testing.T) {
	tmpDir := t.TempDir()
	kimiCodeDir := filepath.Join(tmpDir, ".kimi-code")
	if err := os.MkdirAll(kimiCodeDir, 0o755); err != nil {
		t.Fatal(err)
	}

	a := newKimiCodeAdapter(t, tmpDir)
	if err := a.InstallPlugin(tmpDir, "1.0.0"); err != nil {
		t.Fatalf("InstallPlugin() error = %v", err)
	}

	skillsDir := filepath.Join(tmpDir, ".kimi-code", "plugins", "managed", "gentle-ai", "skills")
	info, err := os.Stat(skillsDir)
	if err != nil {
		t.Fatalf("skills/ subdir not created: %v", err)
	}
	if !info.IsDir() {
		t.Error("skills/ path exists but is not a directory")
	}

	// Verify skills/ is empty on fresh install.
	entries, err := os.ReadDir(skillsDir)
	if err != nil {
		t.Fatalf("ReadDir(skills/) error = %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("skills/ should be empty on fresh install, got %d entries", len(entries))
	}
}

func TestInstallPlugin_SkillsSubdirIdempotent(t *testing.T) {
	tmpDir := t.TempDir()
	kimiCodeDir := filepath.Join(tmpDir, ".kimi-code")
	if err := os.MkdirAll(kimiCodeDir, 0o755); err != nil {
		t.Fatal(err)
	}

	a := newKimiCodeAdapter(t, tmpDir)

	// First call.
	if err := a.InstallPlugin(tmpDir, "1.0.0"); err != nil {
		t.Fatalf("InstallPlugin() first call error = %v", err)
	}

	// Write a marker file into skills/ to verify it's preserved.
	skillsDir := filepath.Join(tmpDir, ".kimi-code", "plugins", "managed", "gentle-ai", "skills")
	markerPath := filepath.Join(skillsDir, "marker.txt")
	if err := os.WriteFile(markerPath, []byte("keep-me"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Second call — should not fail and should preserve marker.
	if err := a.InstallPlugin(tmpDir, "1.0.0"); err != nil {
		t.Fatalf("InstallPlugin() second call error = %v", err)
	}

	data, err := os.ReadFile(markerPath)
	if err != nil {
		t.Fatalf("marker file lost after second InstallPlugin: %v", err)
	}
	if string(data) != "keep-me" {
		t.Errorf("marker file content = %q, want %q", string(data), "keep-me")
	}
}

func TestPluginManifest_SkillsPathRelative(t *testing.T) {
	tmpDir := t.TempDir()
	kimiCodeDir := filepath.Join(tmpDir, ".kimi-code")
	if err := os.MkdirAll(kimiCodeDir, 0o755); err != nil {
		t.Fatal(err)
	}

	a := newKimiCodeAdapter(t, tmpDir)
	if err := a.InstallPlugin(tmpDir, "1.0.0"); err != nil {
		t.Fatalf("InstallPlugin() error = %v", err)
	}

	data, err := os.ReadFile(a.PluginManifestPath(tmpDir))
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}

	var manifest kimi.KimiPluginManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatalf("manifest JSON invalid: %v", err)
	}

	// Skills path should be relative ("./skills/"), not absolute.
	if manifest.Skills != "./skills/" {
		t.Errorf("manifest.Skills = %q, want relative './skills/'", manifest.Skills)
	}
}

func TestPluginDir_KIMI_CODE_HOME(t *testing.T) {
	tmpDir := t.TempDir()
	customDir := filepath.Join(tmpDir, "custom-kimi")
	if err := os.MkdirAll(customDir, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("KIMI_CODE_HOME", customDir)

	a := newKimiCodeAdapter(t, tmpDir)
	got := a.PluginDir(tmpDir)
	expected := filepath.Join(customDir, "plugins", "managed", "gentle-ai")
	if got != expected {
		t.Errorf("PluginDir() = %v, want %v", got, expected)
	}
}

func TestInstallPlugin_KIMI_CODE_HOME(t *testing.T) {
	tmpDir := t.TempDir()
	customDir := filepath.Join(tmpDir, "custom-kimi")
	if err := os.MkdirAll(customDir, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("KIMI_CODE_HOME", customDir)

	a := newKimiCodeAdapter(t, tmpDir)
	if err := a.InstallPlugin(tmpDir, "1.0.0"); err != nil {
		t.Fatalf("InstallPlugin() error = %v", err)
	}

	// Manifest and installed.json must live under KIMI_CODE_HOME, not ~/.kimi-code.
	manifestPath := filepath.Join(customDir, "plugins", "managed", "gentle-ai", "kimi.plugin.json")
	if _, err := os.Stat(manifestPath); os.IsNotExist(err) {
		t.Errorf("manifest not written under KIMI_CODE_HOME: %s", manifestPath)
	}

	installedPath := filepath.Join(customDir, "plugins", "installed.json")
	if _, err := os.Stat(installedPath); os.IsNotExist(err) {
		t.Errorf("installed.json not written under KIMI_CODE_HOME: %s", installedPath)
	}

	// Ensure it did NOT write to the default ~/.kimi-code tree.
	defaultPluginDir := filepath.Join(tmpDir, ".kimi-code", "plugins", "managed", "gentle-ai")
	if _, err := os.Stat(defaultPluginDir); !os.IsNotExist(err) {
		t.Errorf("plugin dir unexpectedly created under default ~/.kimi-code: %s", defaultPluginDir)
	}
}

// hasManifestKey reports whether the raw plugin manifest JSON contains the
// given top-level key.
func hasManifestKey(data []byte, key string) bool {
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		return false
	}
	_, ok := raw[key]
	return ok
}
