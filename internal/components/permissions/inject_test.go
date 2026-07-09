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
	"github.com/gentleman-programming/gentle-ai/internal/agents/hermes"
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
func hermesAdapter() agents.Adapter      { return hermes.NewAdapter() }

func tomlSection(text, header string) string {
	start := strings.Index(text, header)
	if start == -1 {
		return ""
	}
	section := text[start+len(header):]
	if next := strings.Index(section, "\n["); next != -1 {
		section = section[:next]
	}
	return section
}

func assertCodexWorkspaceWriteRulesScoped(t *testing.T, text string) {
	t.Helper()

	rootFilesystem := tomlSection(text, `[permissions.gentle-dev.filesystem]`)
	for _, rule := range []string{`"." = "write"`, `".git/**" = "write"`} {
		if strings.Contains(rootFilesystem, rule) {
			t.Fatalf("root filesystem table contains workspace write rule %q; got:\n%s", rule, rootFilesystem)
		}
	}

	scopedFilesystem := tomlSection(text, `[permissions.gentle-dev.filesystem.":workspace_roots"]`)
	if scopedFilesystem == "" {
		t.Fatalf("config.toml missing workspace-scoped filesystem table; got:\n%s", text)
	}
	for _, rule := range []string{`"." = "write"`, `".git/**" = "write"`} {
		if !strings.Contains(scopedFilesystem, rule) {
			t.Fatalf("workspace-scoped filesystem table missing workspace write rule %q; got:\n%s", rule, scopedFilesystem)
		}
	}
}

// TestInjectHermesSkipsPermissions verifies that Hermes returns nil (no file written)
// because Hermes permission format is undocumented — §14 of spec.
func TestInjectHermesSkipsPermissions(t *testing.T) {
	home := t.TempDir()

	result, err := Inject(home, hermesAdapter())
	if err != nil {
		t.Fatalf("Inject(hermes) error = %v", err)
	}
	if result.Changed {
		t.Fatal("Inject(hermes) changed = true, want false (no file should be written)")
	}
	if len(result.Files) != 0 {
		t.Fatalf("Inject(hermes) files = %v, want [] (no file should be written)", result.Files)
	}

	// Confirm no config.yaml or settings file was created.
	hermesDir := filepath.Join(home, ".hermes")
	if _, err := os.Stat(hermesDir); err == nil {
		t.Fatal("Inject(hermes) created ~/.hermes directory, want no files written")
	}
}

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

func TestInjectCodexWritesGentleDevPermissionsProfile(t *testing.T) {
	home := t.TempDir()

	result, err := Inject(home, codexAdapter())
	if err != nil {
		t.Fatalf("Inject() error = %v", err)
	}
	if !result.Changed {
		t.Fatal("Inject() for Codex changed = false")
	}

	configPath := filepath.Join(home, ".codex", "config.toml")
	if len(result.Files) != 1 || result.Files[0] != configPath {
		t.Fatalf("Inject() files = %v, want [%q]", result.Files, configPath)
	}

	content, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("read config.toml: %v", err)
	}
	text := string(content)

	wantSubstrings := []string{
		`approval_policy = "on-request"`,
		`default_permissions = "gentle-dev"`,
		`[permissions.gentle-dev]`,
		`[permissions.gentle-dev.network]`,
		`enabled = true`,
		`[permissions.gentle-dev.network.domains]`,
		`"*" = "allow"`,
		`[permissions.gentle-dev.filesystem]`,
		`":minimal" = "read"`,
		`"~/.config/git" = "read"`,
		`"~/.gitconfig" = "read"`,
		`"~/.local/state/nix/profiles/home-manager/home-path" = "read"`,
		`"~/.nix-profile" = "read"`,
		`"/nix/store" = "read"`,
		`":tmpdir" = "write"`,
		`":slash_tmp" = "write"`,
		`glob_scan_max_depth = 6`,
		`[permissions.gentle-dev.filesystem.":workspace_roots"]`,
		`"**/.env" = "deny"`,
		`"**/.env.local" = "deny"`,
		`"**/.env.*.local" = "deny"`,
		`"**/*.pem" = "deny"`,
		`"**/*.key" = "deny"`,
		`"**/secrets/**" = "deny"`,
		`"**/.ssh/**" = "deny"`,
		`"**/.credentials/**" = "deny"`,
		`"**/credentials.json" = "deny"`,
		`"**/.aws/credentials" = "deny"`,
		`"**/.config/gh/hosts.yml" = "deny"`,
		`[permissions.gentle-dev.workspace_roots]`,
		`"~" = true`,
	}
	for _, want := range wantSubstrings {
		if !strings.Contains(text, want) {
			t.Fatalf("config.toml missing %q; got:\n%s", want, text)
		}
	}
	rootFilesystem := tomlSection(text, `[permissions.gentle-dev.filesystem]`)
	if !strings.Contains(rootFilesystem, `glob_scan_max_depth = 6`) {
		t.Fatalf("root filesystem table missing glob_scan_max_depth; got:\n%s", rootFilesystem)
	}
	for _, invalidRootGlob := range []string{`"**/.env" = "deny"`, `"**/*.pem" = "deny"`, `"**/*.key" = "deny"`} {
		if strings.Contains(rootFilesystem, invalidRootGlob) {
			t.Fatalf("root filesystem table contains scoped secret deny %q; got:\n%s", invalidRootGlob, rootFilesystem)
		}
	}
	assertCodexWorkspaceWriteRulesScoped(t, text)
	scopedFilesystem := tomlSection(text, `[permissions.gentle-dev.filesystem.":workspace_roots"]`)
	if scopedFilesystem == "" {
		t.Fatalf("config.toml missing workspace-scoped filesystem table; got:\n%s", text)
	}
	for _, want := range []string{`"**/.env" = "deny"`, `"**/*.pem" = "deny"`, `"**/*.key" = "deny"`} {
		if !strings.Contains(scopedFilesystem, want) {
			t.Fatalf("workspace-scoped filesystem table missing %q; got:\n%s", want, scopedFilesystem)
		}
	}
	for _, invalid := range []string{
		`"**/.git" = "write"`,
		`"**/.git/**" = "write"`,
	} {
		if strings.Contains(text, invalid) {
			t.Fatalf("config.toml contains invalid Codex permission entry %q; got:\n%s", invalid, text)
		}
	}
	if strings.Contains(text, `extends = ":workspace"`) {
		t.Fatalf("config.toml should not inherit :workspace because it keeps Codex .git protections; got:\n%s", text)
	}
}

func TestInjectCodexPermissionsSkipsNixStoreOnWindows(t *testing.T) {
	home := t.TempDir()
	origGOOS := codexPermissionsGOOS
	codexPermissionsGOOS = "windows"
	t.Cleanup(func() { codexPermissionsGOOS = origGOOS })

	if _, err := Inject(home, codexAdapter()); err != nil {
		t.Fatalf("Inject() error = %v", err)
	}

	configPath := filepath.Join(home, ".codex", "config.toml")
	content, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("read config.toml: %v", err)
	}
	text := string(content)

	if strings.Contains(text, `"/nix/store"`) {
		t.Fatalf("Windows Codex config should not include /nix/store; got:\n%s", text)
	}
	for _, want := range []string{
		`"~/.local/state/nix/profiles/home-manager/home-path" = "read"`,
		`"~/.nix-profile" = "read"`,
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("Windows Codex config missing non-fatal Nix home path %q; got:\n%s", want, text)
		}
	}
}

func TestInjectCodexPermissionsAllowsEnvExamples(t *testing.T) {
	home := t.TempDir()

	if _, err := Inject(home, codexAdapter()); err != nil {
		t.Fatalf("Inject() error = %v", err)
	}

	configPath := filepath.Join(home, ".codex", "config.toml")
	content, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("read config.toml: %v", err)
	}
	text := string(content)

	for _, forbidden := range []string{
		`"**/.env.*" = "deny"`,
		`"*.env.*" = "deny"`,
	} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("config.toml contains over-broad env deny rule %q; got:\n%s", forbidden, text)
		}
	}

	for _, allowedExample := range []string{".env.example", ".env.template"} {
		if strings.Contains(text, allowedExample) {
			t.Fatalf("config.toml should not mention versioned env template %q; got:\n%s", allowedExample, text)
		}
	}
}

func TestInjectCodexPermissionsProfileIsIdempotent(t *testing.T) {
	home := t.TempDir()
	configPath := filepath.Join(home, ".codex", "config.toml")
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	initial := `model = "gpt-5.5"

[permissions.gentle-dev]
glob_scan_max_depth = 6

[mcp_servers.engram]
command = "engram"
args = ["mcp", "--tools=agent"]
`
	if err := os.WriteFile(configPath, []byte(initial), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	first, err := Inject(home, codexAdapter())
	if err != nil {
		t.Fatalf("Inject() first error = %v", err)
	}
	if !first.Changed {
		t.Fatal("Inject() first changed = false")
	}

	firstContent, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("ReadFile() first error = %v", err)
	}

	second, err := Inject(home, codexAdapter())
	if err != nil {
		t.Fatalf("Inject() second error = %v", err)
	}
	if second.Changed {
		t.Fatal("Inject() second changed = true")
	}

	secondContent, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("ReadFile() second error = %v", err)
	}
	if string(firstContent) != string(secondContent) {
		t.Fatalf("Codex permissions injection is not idempotent:\nfirst:\n%s\nsecond:\n%s", firstContent, secondContent)
	}

	text := string(secondContent)
	if !strings.Contains(text, `model = "gpt-5.5"`) || !strings.Contains(text, `[mcp_servers.engram]`) {
		t.Fatalf("Codex permissions injection did not preserve existing config; got:\n%s", text)
	}
	for _, section := range []string{
		"[permissions.gentle-dev]",
		"[permissions.gentle-dev.filesystem]",
		`[permissions.gentle-dev.filesystem.":workspace_roots"]`,
		"[permissions.gentle-dev.network]",
		"[permissions.gentle-dev.network.domains]",
		"[permissions.gentle-dev.workspace_roots]",
	} {
		if count := strings.Count(text, section); count != 1 {
			t.Fatalf("section %q count = %d, want 1; got:\n%s", section, count, text)
		}
	}
	rootFilesystem := tomlSection(text, `[permissions.gentle-dev.filesystem]`)
	for _, invalid := range []string{`"**/*.key" = "deny"`, `"**/*.pem" = "deny"`} {
		if strings.Contains(rootFilesystem, invalid) {
			t.Fatalf("root filesystem table should not contain invalid/stale entry %q; got:\n%s", invalid, rootFilesystem)
		}
	}
	for _, want := range []string{`":tmpdir" = "write"`, `":slash_tmp" = "write"`} {
		if !strings.Contains(text, want) {
			t.Fatalf("config.toml missing compatible Codex permission entry %q; got:\n%s", want, text)
		}
	}
	assertCodexWorkspaceWriteRulesScoped(t, text)
}

func TestInjectCodexPermissionsRelocatesSecretDeniesToWorkspaceRootsTable(t *testing.T) {
	home := t.TempDir()
	configPath := filepath.Join(home, ".codex", "config.toml")
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}

	initial := `[permissions.gentle-dev.filesystem]
"**/*.key" = "deny"
"**/*.pem" = "deny"

[permissions.gentle-dev.filesystem.":workspace_roots"]
"**/.git" = "write"
"**/.git/**" = "write"
"**/.env" = "deny"
"**/.env.local" = "deny"
"**/.env.*.local" = "deny"
"**/*.pem" = "deny"
"**/*.key" = "deny"
"**/secrets/*" = "deny"
`
	if err := os.WriteFile(configPath, []byte(initial), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	if _, err := Inject(home, codexAdapter()); err != nil {
		t.Fatalf("Inject() error = %v", err)
	}

	content, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	text := string(content)

	rootFilesystem := tomlSection(text, `[permissions.gentle-dev.filesystem]`)
	for _, invalid := range []string{`"**/*.key" = "deny"`, `"**/*.pem" = "deny"`} {
		if strings.Contains(rootFilesystem, invalid) {
			t.Fatalf("root filesystem table still contains stale secret deny %q; got:\n%s", invalid, rootFilesystem)
		}
	}

	scopedFilesystem := tomlSection(text, `[permissions.gentle-dev.filesystem.":workspace_roots"]`)
	if scopedFilesystem == "" {
		t.Fatalf("config.toml should keep workspace-scoped table for secret denies; got:\n%s", text)
	}
	for _, want := range []string{`"**/.env" = "deny"`, `"**/*.key" = "deny"`, `"**/*.pem" = "deny"`} {
		if !strings.Contains(scopedFilesystem, want) {
			t.Fatalf("workspace-scoped table missing relocated secret deny %q; got:\n%s", want, scopedFilesystem)
		}
	}
	for _, invalid := range []string{`"**/.git" = "write"`, `"**/.git/**" = "write"`} {
		if strings.Contains(text, invalid) {
			t.Fatalf("config.toml still contains invalid git write rule %q; got:\n%s", invalid, text)
		}
	}
}

func TestInjectCodexPermissionsRemovesObsoleteWorkspaceRootDenyRules(t *testing.T) {
	home := t.TempDir()
	configPath := filepath.Join(home, ".codex", "config.toml")
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}

	initial := `[permissions.gentle-dev.filesystem.":workspace_roots"]
"**/.env.*" = "deny"
"*.env.*" = "deny"
"**/.env" = "deny"
"**/.env.local" = "deny"
"**/.env.*.local" = "deny"
`
	if err := os.WriteFile(configPath, []byte(initial), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	if _, err := Inject(home, codexAdapter()); err != nil {
		t.Fatalf("Inject() error = %v", err)
	}

	content, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	text := string(content)

	for _, obsolete := range []string{
		`"**/.env.*" = "deny"`,
		`"*.env.*" = "deny"`,
	} {
		if strings.Contains(text, obsolete) {
			t.Fatalf("config.toml still contains obsolete workspace root deny entry %q; got:\n%s", obsolete, text)
		}
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
