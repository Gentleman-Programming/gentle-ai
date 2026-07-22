package persona

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/internal/model"
	"github.com/gentleman-programming/gentle-ai/internal/system"
)

// testOutputStyleAdapter is a minimal test adapter that supports output styles.
type testOutputStyleAdapter struct {
	homeDir string
}

func newOutputStyleTestAdapter(t *testing.T, homeDir string) *testOutputStyleAdapter {
	// Ensure directories exist.
	outputStyleDir := filepath.Join(homeDir, ".test", "output-styles")
	if err := os.MkdirAll(outputStyleDir, 0o755); err != nil {
		t.Fatalf("create output style dir: %v", err)
	}

	settingsDir := filepath.Join(homeDir, ".test")
	if err := os.MkdirAll(settingsDir, 0o755); err != nil {
		t.Fatalf("create settings dir: %v", err)
	}

	// Seed empty settings.json.
	settingsPath := filepath.Join(settingsDir, "settings.json")
	if err := os.WriteFile(settingsPath, []byte("{}"), 0o644); err != nil {
		t.Fatalf("seed settings.json: %v", err)
	}

	return &testOutputStyleAdapter{homeDir: homeDir}
}

// --- Adapter interface implementation ---

func (a *testOutputStyleAdapter) Agent() model.AgentID {
	return model.AgentClaudeCode
}

func (a *testOutputStyleAdapter) Tier() model.SupportTier {
	return model.TierFull
}

func (a *testOutputStyleAdapter) Detect(_ context.Context, _ string) (bool, string, string, bool, error) {
	return false, "", "", false, nil
}

func (a *testOutputStyleAdapter) SupportsAutoInstall() bool {
	return false
}

func (a *testOutputStyleAdapter) InstallCommand(_ system.PlatformProfile) ([][]string, error) {
	return nil, nil
}

func (a *testOutputStyleAdapter) GlobalConfigDir(_ string) string {
	return ""
}

func (a *testOutputStyleAdapter) SystemPromptDir(_ string) string {
	return ""
}

func (a *testOutputStyleAdapter) SystemPromptFile(_ string) string {
	return filepath.Join(a.homeDir, ".test", "SYSTEM.md")
}

func (a *testOutputStyleAdapter) SkillsDir(_ string) string {
	return ""
}

func (a *testOutputStyleAdapter) SettingsPath(_ string) string {
	return filepath.Join(a.homeDir, ".test", "settings.json")
}

func (a *testOutputStyleAdapter) SystemPromptStrategy() model.SystemPromptStrategy {
	return model.StrategyMarkdownSections
}

func (a *testOutputStyleAdapter) MCPStrategy() model.MCPStrategy {
	return model.StrategyMCPConfigFile
}

func (a *testOutputStyleAdapter) MCPConfigPath(_ string, _ string) string {
	return ""
}

func (a *testOutputStyleAdapter) SupportsOutputStyles() bool {
	return true
}

func (a *testOutputStyleAdapter) OutputStyleDir(_ string) string {
	return filepath.Join(a.homeDir, ".test", "output-styles")
}

func (a *testOutputStyleAdapter) SupportsSlashCommands() bool {
	return false
}

func (a *testOutputStyleAdapter) CommandsDir(_ string) string {
	return ""
}

func (a *testOutputStyleAdapter) SupportsSubAgents() bool {
	return false
}

func (a *testOutputStyleAdapter) SubAgentsDir(_ string) string {
	return ""
}

func (a *testOutputStyleAdapter) EmbeddedSubAgentsDir() string {
	return ""
}

func (a *testOutputStyleAdapter) SupportsSkills() bool {
	return false
}

func (a *testOutputStyleAdapter) SupportsSystemPrompt() bool {
	return true
}

func (a *testOutputStyleAdapter) SupportsMCP() bool {
	return false
}

// --- Tests ---

// TestInjectSwitchGentlemanToNeutralRemovesGentlemanArtifacts pins the switch
// scenario end to end: after a gentleman install, injecting neutral must leave
// no gentleman output style and must select the Neutral managed style.
func TestInjectSwitchGentlemanToNeutralRemovesGentlemanArtifacts(t *testing.T) {
	homeDir := t.TempDir()
	adapter := newOutputStyleTestAdapter(t, homeDir)

	if _, err := Inject(homeDir, adapter, model.PersonaGentleman); err != nil {
		t.Fatalf("gentleman install: %v", err)
	}
	gentlemanStyle := filepath.Join(adapter.OutputStyleDir(homeDir), "gentleman.md")
	if _, err := os.Stat(gentlemanStyle); err != nil {
		t.Fatalf("precondition: gentleman.md missing after gentleman install: %v", err)
	}

	if _, err := Inject(homeDir, adapter, model.PersonaNeutral); err != nil {
		t.Fatalf("neutral switch: %v", err)
	}

	if _, err := os.Stat(gentlemanStyle); !os.IsNotExist(err) {
		t.Fatalf("gentleman.md must be removed on switch to neutral, stat err = %v", err)
	}
	neutralStyle := filepath.Join(adapter.OutputStyleDir(homeDir), "neutral.md")
	if _, err := os.Stat(neutralStyle); err != nil {
		t.Fatalf("neutral.md missing after switch: %v", err)
	}
	settings, err := os.ReadFile(adapter.SettingsPath(homeDir))
	if err != nil {
		t.Fatalf("read settings: %v", err)
	}
	if !strings.Contains(string(settings), `"outputStyle": "Neutral"`) {
		t.Fatalf("settings must select the Neutral style, got:\n%s", settings)
	}
	if strings.Contains(string(settings), `"outputStyle": "Gentleman"`) {
		t.Fatal("settings still selects the Gentleman style after switch")
	}
}

// TestInjectNeutralTwiceIsIdempotent pins sync stability: a second neutral
// injection must not report changes.
func TestInjectNeutralTwiceIsIdempotent(t *testing.T) {
	homeDir := t.TempDir()
	adapter := newOutputStyleTestAdapter(t, homeDir)

	if _, err := Inject(homeDir, adapter, model.PersonaNeutral); err != nil {
		t.Fatalf("first inject: %v", err)
	}
	second, err := Inject(homeDir, adapter, model.PersonaNeutral)
	if err != nil {
		t.Fatalf("second inject: %v", err)
	}
	if second.Changed {
		t.Fatalf("second neutral inject reported changes: %v", second.Files)
	}
}

// TestInjectSwitchToleratesMalformedSettings pins the PR #289 review finding:
// JSONC or otherwise malformed settings.json must not make the switch hard-fail,
// and the gentleman output-style file must still be removed (the file cleanup
// does not depend on settings parsing).
func TestInjectSwitchToleratesMalformedSettings(t *testing.T) {
	homeDir := t.TempDir()
	adapter := newOutputStyleTestAdapter(t, homeDir)

	if _, err := Inject(homeDir, adapter, model.PersonaGentleman); err != nil {
		t.Fatalf("gentleman install: %v", err)
	}
	// Simulate a user-managed JSONC settings file (comments are invalid JSON).
	malformed := []byte("{\n  // user comment\n  \"outputStyle\": \"Gentleman\"\n}\n")
	if err := os.WriteFile(adapter.SettingsPath(homeDir), malformed, 0o644); err != nil {
		t.Fatalf("seed malformed settings: %v", err)
	}

	if _, err := Inject(homeDir, adapter, model.PersonaNeutral); err != nil {
		t.Fatalf("neutral switch over malformed settings must not hard-fail: %v", err)
	}
	gentlemanStyle := filepath.Join(adapter.OutputStyleDir(homeDir), "gentleman.md")
	if _, err := os.Stat(gentlemanStyle); !os.IsNotExist(err) {
		t.Fatalf("gentleman.md must be removed even when settings are malformed, stat err = %v", err)
	}
}
