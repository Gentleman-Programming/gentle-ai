package permissions

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/internal/agents"
	"github.com/gentleman-programming/gentle-ai/internal/agents/antigravity"
	"github.com/gentleman-programming/gentle-ai/internal/agents/claude"
	"github.com/gentleman-programming/gentle-ai/internal/agents/codex"
	"github.com/gentleman-programming/gentle-ai/internal/agents/cursor"
	"github.com/gentleman-programming/gentle-ai/internal/agents/gemini"
	"github.com/gentleman-programming/gentle-ai/internal/agents/opencode"
	"github.com/gentleman-programming/gentle-ai/internal/agents/vscode"
	"github.com/gentleman-programming/gentle-ai/internal/model"
)

func claudeAdapter() agents.Adapter      { return claude.NewAdapter() }
func opencodeAdapter() agents.Adapter    { return opencode.NewAdapter() }
func geminiAdapter() agents.Adapter      { return gemini.NewAdapter() }
func cursorAdapter() agents.Adapter      { return cursor.NewAdapter() }
func vscodeAdapter() agents.Adapter      { return vscode.NewAdapter() }
func codexAdapter() agents.Adapter       { return codex.NewAdapter() }
func antigravityAdapter() agents.Adapter { return antigravity.NewAdapter() }

func TestInjectOpenCodeIsIdempotent(t *testing.T) {
	home := t.TempDir()

	first, err := Inject(home, opencodeAdapter())
	if err != nil {
		t.Fatalf("Inject() first error = %v", err)
	}
	if !first.Changed {
		t.Fatalf("Inject() first changed = false")
	}

	second, err := Inject(home, opencodeAdapter())
	if err != nil {
		t.Fatalf("Inject() second error = %v", err)
	}
	if second.Changed {
		t.Fatalf("Inject() second changed = true")
	}

	path := filepath.Join(home, ".config", "opencode", "opencode.json")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected config file %q: %v", path, err)
	}

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(opencode.json) error = %v", err)
	}

	text := string(content)
	if !strings.Contains(text, `"permission"`) {
		t.Fatal("opencode.json missing permission key")
	}
	if strings.Contains(text, `"permissions"`) {
		t.Fatal("opencode.json should use 'permission' (singular), not 'permissions'")
	}
	if !strings.Contains(text, `"bash"`) {
		t.Fatal("opencode.json permission missing bash section")
	}
	if !strings.Contains(text, `"read"`) {
		t.Fatal("opencode.json permission missing read section")
	}
}

func TestInjectAddsEnvToDenyList(t *testing.T) {
	home := t.TempDir()

	if _, err := Inject(home, claudeAdapter()); err != nil {
		t.Fatalf("Inject() error = %v", err)
	}

	settingsPath := filepath.Join(home, ".claude", "settings.json")
	content, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("read settings file %q: %v", settingsPath, err)
	}

	var settings map[string]any
	if err := json.Unmarshal(content, &settings); err != nil {
		t.Fatalf("unmarshal settings json: %v", err)
	}

	permissionsNode, ok := settings["permissions"].(map[string]any)
	if !ok {
		t.Fatalf("permissions node missing or invalid: %#v", settings["permissions"])
	}

	denyList, ok := permissionsNode["deny"].([]any)
	if !ok {
		t.Fatalf("deny list missing or invalid: %#v", permissionsNode["deny"])
	}

	for _, entry := range denyList {
		if value, ok := entry.(string); ok && value == "Read(.env)" {
			return
		}
	}

	t.Fatalf("deny list missing explicit .env rule: %#v", denyList)
}

func TestInjectClaudeCodeUsesBypassPermissions(t *testing.T) {
	home := t.TempDir()

	if _, err := Inject(home, claudeAdapter()); err != nil {
		t.Fatalf("Inject() error = %v", err)
	}

	settingsPath := filepath.Join(home, ".claude", "settings.json")
	content, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("read settings file: %v", err)
	}

	var settings map[string]any
	if err := json.Unmarshal(content, &settings); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	perms, ok := settings["permissions"].(map[string]any)
	if !ok {
		t.Fatalf("permissions node missing")
	}

	mode, ok := perms["defaultMode"].(string)
	if !ok || mode != "bypassPermissions" {
		t.Fatalf("expected defaultMode=bypassPermissions, got %q", mode)
	}
}

func TestInjectGeminiCLIUsesAutoEditMode(t *testing.T) {
	home := t.TempDir()

	result, err := Inject(home, geminiAdapter())
	if err != nil {
		t.Fatalf("Inject() error = %v", err)
	}
	if !result.Changed {
		t.Fatal("Inject() changed = false")
	}

	settingsPath := filepath.Join(home, ".gemini", "settings.json")
	content, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("read settings file: %v", err)
	}

	var settings map[string]any
	if err := json.Unmarshal(content, &settings); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	general, ok := settings["general"].(map[string]any)
	if !ok {
		t.Fatalf("general node missing: %#v", settings)
	}

	mode, ok := general["defaultApprovalMode"].(string)
	if !ok || mode != "auto_edit" {
		t.Fatalf("expected defaultApprovalMode=auto_edit, got %q", mode)
	}

	// Ensure no Claude Code keys leaked
	if _, exists := settings["permissions"]; exists {
		t.Fatal("gemini settings should not contain 'permissions' key")
	}
}

func TestInjectVSCodeCopilotUsesAutoApprove(t *testing.T) {
	home := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("APPDATA", filepath.Join(home, "AppData", "Roaming"))

	adapter := vscodeAdapter()
	result, err := Inject(home, adapter)
	if err != nil {
		t.Fatalf("Inject() error = %v", err)
	}
	if !result.Changed {
		t.Fatal("Inject() changed = false")
	}

	settingsPath := adapter.SettingsPath(home)
	content, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("read settings file: %v", err)
	}

	var settings map[string]any
	if err := json.Unmarshal(content, &settings); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	autoApprove, ok := settings["chat.tools.autoApprove"].(bool)
	if !ok || !autoApprove {
		t.Fatalf("expected chat.tools.autoApprove=true, got %v", settings["chat.tools.autoApprove"])
	}

	// Ensure no Claude Code keys leaked
	if _, exists := settings["permissions"]; exists {
		t.Fatal("vscode settings should not contain 'permissions' key")
	}
}

func TestInjectVSCodeCopilotMergesIntoJSONCSettings(t *testing.T) {
	home := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("APPDATA", filepath.Join(home, "AppData", "Roaming"))

	adapter := vscodeAdapter()
	settingsPath := adapter.SettingsPath(home)
	if err := os.MkdirAll(filepath.Dir(settingsPath), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}

	baseSettings := `{
	  // User has comments and trailing commas in VS Code settings
	  "editor.formatOnSave": true,
	  "files.exclude": {
	    "**/.git": true,
	  },
	}
`
	if err := os.WriteFile(settingsPath, []byte(baseSettings), 0o644); err != nil {
		t.Fatalf("WriteFile(settings.json) error = %v", err)
	}

	result, err := Inject(home, adapter)
	if err != nil {
		t.Fatalf("Inject() error = %v", err)
	}
	if !result.Changed {
		t.Fatal("Inject() changed = false")
	}

	content, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("read settings file: %v", err)
	}

	var settings map[string]any
	if err := json.Unmarshal(content, &settings); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	autoApprove, ok := settings["chat.tools.autoApprove"].(bool)
	if !ok || !autoApprove {
		t.Fatalf("expected chat.tools.autoApprove=true, got %v", settings["chat.tools.autoApprove"])
	}

	if settings["editor.formatOnSave"] != true {
		t.Fatalf("expected editor.formatOnSave=true, got %v", settings["editor.formatOnSave"])
	}
}

func TestInjectCursorSkipsPermissions(t *testing.T) {
	home := t.TempDir()

	result, err := Inject(home, cursorAdapter())
	if err != nil {
		t.Fatalf("Inject() error = %v", err)
	}
	if result.Changed {
		t.Fatal("Inject() for Cursor should not change anything (permissions via cli-config.json)")
	}
	if len(result.Files) != 0 {
		t.Fatalf("Inject() for Cursor should return no files, got %v", result.Files)
	}
}

func TestInjectAntigravitySkipsPermissions(t *testing.T) {
	overlay := agentOverlay(model.AgentAntigravity)
	if overlay != nil {
		t.Errorf("expected nil overlay for Antigravity, got %s", overlay)
	}
}

func TestInjectCodexSkipsPermissions(t *testing.T) {
	home := t.TempDir()

	result, err := Inject(home, codexAdapter())
	if err != nil {
		t.Fatalf("Inject() error = %v", err)
	}
	if result.Changed {
		t.Fatal("Inject() for Codex should not change anything (no known settings.json path)")
	}
	if len(result.Files) != 0 {
		t.Fatalf("Inject() for Codex should return no files, got %v", result.Files)
	}
}

// TestInjectClaudeCodeSensitivePathsDenied verifies that the default sensitive-path
// deny list is present in the Claude Code permissions block.
func TestInjectClaudeCodeSensitivePathsDenied(t *testing.T) {
	sensitivePatterns := []string{
		"Read(.ssh/*)",
		"Edit(.ssh/*)",
		"Read(.credentials/*)",
		"Edit(.credentials/*)",
		"Read(Library/Keychains/*)",
		"Edit(Library/Keychains/*)",
		"Read(.aws/credentials)",
		"Edit(.aws/credentials)",
		"Read(.config/gh/hosts.yml)",
		"Edit(.config/gh/hosts.yml)",
		"Read(**/*.pem)",
		"Edit(**/*.pem)",
		"Read(**/*.key)",
		"Edit(**/*.key)",
		"Read(**/secrets/*)",
		"Edit(**/secrets/*)",
	}

	home := t.TempDir()
	if _, err := Inject(home, claudeAdapter()); err != nil {
		t.Fatalf("Inject() error = %v", err)
	}

	settingsPath := filepath.Join(home, ".claude", "settings.json")
	content, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("read settings file %q: %v", settingsPath, err)
	}

	var settings map[string]any
	if err := json.Unmarshal(content, &settings); err != nil {
		t.Fatalf("unmarshal settings json: %v", err)
	}

	permissionsNode, ok := settings["permissions"].(map[string]any)
	if !ok {
		t.Fatalf("permissions node missing or invalid: %#v", settings["permissions"])
	}

	denyList, ok := permissionsNode["deny"].([]any)
	if !ok {
		t.Fatalf("deny list missing or invalid: %#v", permissionsNode["deny"])
	}

	denySet := make(map[string]bool, len(denyList))
	for _, entry := range denyList {
		if v, ok := entry.(string); ok {
			denySet[v] = true
		}
	}

	for _, pattern := range sensitivePatterns {
		t.Run(pattern, func(t *testing.T) {
			if !denySet[pattern] {
				t.Errorf("deny list missing pattern %q; got: %v", pattern, denyList)
			}
		})
	}
}

// TestInjectOpenCodeSensitivePathsDenied verifies that the default sensitive-path
// deny list is present in the OpenCode/Kilocode read permissions block.
func TestInjectOpenCodeSensitivePathsDenied(t *testing.T) {
	sensitivePatterns := []string{
		"**/.ssh/**",
		"**/.credentials/**",
		"**/Library/Keychains/**",
		"**/.aws/credentials",
		"**/.config/gh/hosts.yml",
		"**/*.pem",
		"**/*.key",
	}

	tests := []struct {
		name    string
		adapter agents.Adapter
	}{
		{"opencode", opencodeAdapter()},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			home := t.TempDir()
			if _, err := Inject(home, tt.adapter); err != nil {
				t.Fatalf("Inject() error = %v", err)
			}

			settingsPath := tt.adapter.SettingsPath(home)
			content, err := os.ReadFile(settingsPath)
			if err != nil {
				t.Fatalf("read settings file %q: %v", settingsPath, err)
			}

			var settings map[string]any
			if err := json.Unmarshal(content, &settings); err != nil {
				t.Fatalf("unmarshal settings json: %v", err)
			}

			permNode, ok := settings["permission"].(map[string]any)
			if !ok {
				t.Fatalf("permission node missing or invalid: %#v", settings["permission"])
			}

			readNode, ok := permNode["read"].(map[string]any)
			if !ok {
				t.Fatalf("read node missing or invalid: %#v", permNode["read"])
			}

			for _, pattern := range sensitivePatterns {
				t.Run(pattern, func(t *testing.T) {
					val, exists := readNode[pattern]
					if !exists {
						t.Errorf("read deny list missing pattern %q", pattern)
						return
					}
					if val != "deny" {
						t.Errorf("pattern %q has value %q, want %q", pattern, val, "deny")
					}
				})
			}
		})
	}
}

// TestInjectClaudeCodeDefaultDenyRulesApplied ensures that the default deny
// rules (including sensitive paths) are written into settings.json even when
// a pre-existing permissions block is already present with other top-level keys.
func TestInjectClaudeCodeDefaultDenyRulesApplied(t *testing.T) {
	home := t.TempDir()
	settingsPath := filepath.Join(home, ".claude", "settings.json")
	if err := os.MkdirAll(filepath.Dir(settingsPath), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}

	// Pre-existing settings with a sibling key under permissions (not deny).
	existing := `{
  "permissions": {
    "defaultMode": "default"
  }
}`
	if err := os.WriteFile(settingsPath, []byte(existing), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	if _, err := Inject(home, claudeAdapter()); err != nil {
		t.Fatalf("Inject() error = %v", err)
	}

	content, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("read settings file: %v", err)
	}

	var settings map[string]any
	if err := json.Unmarshal(content, &settings); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	perms, ok := settings["permissions"].(map[string]any)
	if !ok {
		t.Fatalf("permissions node missing")
	}

	denyList, ok := perms["deny"].([]any)
	if !ok {
		t.Fatalf("deny list missing")
	}

	denySet := make(map[string]bool, len(denyList))
	for _, entry := range denyList {
		if v, ok := entry.(string); ok {
			denySet[v] = true
		}
	}

	// Sensitive-path rules must be present after overlay application.
	for _, rule := range []string{"Read(.ssh/*)", "Read(**/*.pem)", "Read(**/*.key)"} {
		if !denySet[rule] {
			t.Errorf("default deny rule %q was not present; got: %v", rule, denyList)
		}
	}

	// The overlay wins for defaultMode because arrays replace but maps deep-merge.
	mode, _ := perms["defaultMode"].(string)
	if mode != "bypassPermissions" {
		t.Errorf("expected defaultMode=bypassPermissions after overlay, got %q", mode)
	}
}

// TestInjectOpenCodePreservesExistingDenyRules ensures that user-managed read deny
// entries already present in settings.json are not removed when the overlay is applied.
func TestInjectOpenCodePreservesExistingDenyRules(t *testing.T) {
	home := t.TempDir()
	settingsPath := filepath.Join(home, ".config", "opencode", "opencode.json")
	if err := os.MkdirAll(filepath.Dir(settingsPath), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}

	existing := `{
  "permission": {
    "read": {
      "**/my-secret/**": "deny"
    }
  }
}`
	if err := os.WriteFile(settingsPath, []byte(existing), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	if _, err := Inject(home, opencodeAdapter()); err != nil {
		t.Fatalf("Inject() error = %v", err)
	}

	content, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("read settings file: %v", err)
	}

	var settings map[string]any
	if err := json.Unmarshal(content, &settings); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	permNode, ok := settings["permission"].(map[string]any)
	if !ok {
		t.Fatalf("permission node missing")
	}

	readNode, ok := permNode["read"].(map[string]any)
	if !ok {
		t.Fatalf("read node missing")
	}

	// Original user rule must still be present
	if readNode["**/my-secret/**"] != "deny" {
		t.Errorf("user-managed read deny rule '**/my-secret/**' was removed; got: %v", readNode)
	}

	// New sensitive-path rules must also be present
	if readNode["**/.ssh/**"] != "deny" {
		t.Errorf("default read deny rule '**/.ssh/**' was not added; got: %v", readNode)
	}
}
