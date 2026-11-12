package engram

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v4/internal/model"
)

// ─── #5930 S3: agy 1.4 loads plugins only from ~/.gemini/config/plugins ──────

func sharedAntigravityPluginDir(home string) string {
	return filepath.Join(home, ".gemini", "config", "plugins", "gentle-ai-engram")
}

func legacyAntigravityPluginDirForTest(home string) string {
	return filepath.Join(home, ".gemini", "antigravity-cli", "plugins", "gentle-ai-engram")
}

// legacyPreInvocationHooksForTest renders the hooks.json bytes gentle-ai wrote
// before #5895: the same guidance re-injected before every model call.
func legacyPreInvocationHooksForTest(t *testing.T) []byte {
	t.Helper()
	return legacyHooksWithoutTimeoutForTest(t, "PreInvocation")
}

// legacyHooksWithoutTimeoutForTest renders a hooks.json entry for event in the
// handler shape gentle-ai wrote before the explicit timeout was added.
func legacyHooksWithoutTimeoutForTest(t *testing.T, event string) []byte {
	t.Helper()
	payload, err := json.Marshal(map[string]any{
		"injectSteps": []any{map[string]any{"ephemeralMessage": antigravityEngramToolsMessage}},
	})
	if err != nil {
		t.Fatalf("Marshal(payload) error = %v", err)
	}
	cfg := map[string]any{
		"gentle-ai-engram-tools": map[string]any{
			event: []any{map[string]any{"type": "command", "command": "printf '%s\\n' '" + string(payload) + "'"}},
		},
	}
	b, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		t.Fatalf("MarshalIndent(hooks) error = %v", err)
	}
	return append(b, '\n')
}

// seedLegacyAntigravityPlugin writes the three managed assets an older
// gentle-ai installed under ~/.gemini/antigravity-cli/plugins.
func seedLegacyAntigravityPlugin(t *testing.T, home, command string) string {
	t.Helper()
	dir := legacyAntigravityPluginDirForTest(home)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("MkdirAll(%q) error = %v", dir, err)
	}
	for name, content := range map[string][]byte{
		"plugin.json":     []byte(antigravityEngramPluginJSON),
		"mcp_config.json": engramOverlayJSON(model.AgentAntigravity, command),
		"hooks.json":      legacyPreInvocationHooksForTest(t),
	} {
		if err := os.WriteFile(filepath.Join(dir, name), content, 0o644); err != nil {
			t.Fatalf("WriteFile(%q) error = %v", name, err)
		}
	}
	return dir
}

// fakeEngramBinaryForTest writes an executable named engram under a fresh
// temp dir and returns its absolute path.
func fakeEngramBinaryForTest(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "bin", "engram")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll(%q) error = %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatalf("WriteFile(%q) error = %v", path, err)
	}
	return path
}

func TestAntigravityPluginDirUsesSharedConfigRoot(t *testing.T) {
	home := t.TempDir()
	if got, want := AntigravityPluginDir(home), sharedAntigravityPluginDir(home); got != want {
		t.Fatalf("AntigravityPluginDir() = %q, want %q", got, want)
	}
	if got, want := AntigravityLegacyPluginDir(home), legacyAntigravityPluginDirForTest(home); got != want {
		t.Fatalf("AntigravityLegacyPluginDir() = %q, want %q", got, want)
	}
}

func TestInjectAntigravityWritesGlobalMCPConfigAndRetiresPlugins(t *testing.T) {
	home := t.TempDir()

	result, err := Inject(home, antigravityAdapter())
	if err != nil {
		t.Fatalf("Inject(antigravity) error = %v", err)
	}

	globalMCP := filepath.Join(home, ".gemini", "config", "mcp_config.json")
	if _, err := os.Stat(globalMCP); err != nil {
		t.Fatalf("global MCP config %q missing: %v", globalMCP, err)
	}
	if !slices.Contains(result.Files, globalMCP) {
		t.Fatalf("result.Files = %v, want it to report %q", result.Files, globalMCP)
	}
	if _, err := os.Stat(sharedAntigravityPluginDir(home)); !os.IsNotExist(err) {
		t.Fatalf("plugin dir %q must not exist; stat err = %v", sharedAntigravityPluginDir(home), err)
	}
	if _, err := os.Stat(filepath.Join(home, ".gemini", "antigravity-cli", "plugins")); !os.IsNotExist(err) {
		t.Fatalf("no plugin may be written under antigravity-cli/plugins; stat err = %v", err)
	}
}

func TestInjectAntigravityMigratesLegacyManagedPlugin(t *testing.T) {
	home := t.TempDir()
	customCommand := fakeEngramBinaryForTest(t)
	legacyDir := seedLegacyAntigravityPlugin(t, home, customCommand)
	userPlugin := filepath.Join(home, ".gemini", "antigravity-cli", "plugins", "my-plugin", "plugin.json")
	if err := os.MkdirAll(filepath.Dir(userPlugin), 0o755); err != nil {
		t.Fatalf("MkdirAll(user plugin) error = %v", err)
	}
	if err := os.WriteFile(userPlugin, []byte(`{"name":"my-plugin"}`), 0o644); err != nil {
		t.Fatalf("WriteFile(user plugin) error = %v", err)
	}

	result, err := Inject(home, antigravityAdapter())
	if err != nil {
		t.Fatalf("Inject(antigravity) error = %v", err)
	}

	// The registration survives: the legacy command is carried over to global mcp_config.json.
	gotMCP := readAntigravityFile(t, filepath.Join(home, ".gemini", "config", "mcp_config.json"))
	if want := engramOverlayJSON(model.AgentAntigravity, customCommand); !bytes.Equal(gotMCP, want) {
		t.Fatalf("migrated global MCP config must keep the legacy command\nwant:\n%s\ngot:\n%s", want, gotMCP)
	}
	for _, name := range []string{"plugin.json", "mcp_config.json", "hooks.json"} {
		path := filepath.Join(legacyDir, name)
		if _, statErr := os.Lstat(path); !os.IsNotExist(statErr) {
			t.Fatalf("legacy managed asset %q must be removed; lstat err = %v", path, statErr)
		}
		if !slices.Contains(result.Files, path) {
			t.Fatalf("result.Files = %v, want it to report removed legacy asset %q", result.Files, path)
		}
	}
	if _, statErr := os.Lstat(legacyDir); !os.IsNotExist(statErr) {
		t.Fatalf("empty legacy plugin dir must be removed; lstat err = %v", statErr)
	}
	if got := readAntigravityFile(t, userPlugin); string(got) != `{"name":"my-plugin"}` {
		t.Fatalf("unrelated user plugin changed: %s", got)
	}

	// A second pass is a no-op.
	again, err := Inject(home, antigravityAdapter())
	if err != nil {
		t.Fatalf("second Inject(antigravity) error = %v", err)
	}
	if again.Changed {
		t.Fatalf("second Inject(antigravity) changed = true, want idempotent; files = %v", again.Files)
	}
}

func TestInjectAntigravityKeepsEditedLegacyPluginAsset(t *testing.T) {
	home := t.TempDir()
	legacyDir := seedLegacyAntigravityPlugin(t, home, "/opt/engram/bin/engram")
	edited := filepath.Join(legacyDir, "hooks.json")
	custom := []byte("{\"my-own-hook\": {}}\n")
	if err := os.WriteFile(edited, custom, 0o644); err != nil {
		t.Fatalf("WriteFile(edited hooks) error = %v", err)
	}

	if _, err := Inject(home, antigravityAdapter()); err != nil {
		t.Fatalf("Inject(antigravity) error = %v", err)
	}

	if got := readAntigravityFile(t, edited); !bytes.Equal(got, custom) {
		t.Fatalf("user-edited legacy asset changed:\n%s", got)
	}
	for _, name := range []string{"plugin.json", "mcp_config.json"} {
		if _, statErr := os.Lstat(filepath.Join(legacyDir, name)); !os.IsNotExist(statErr) {
			t.Fatalf("legacy managed asset %q must be removed; lstat err = %v", name, statErr)
		}
	}
}

func TestInjectAntigravityKeepsLegacyPluginWhenNewRegistrationFails(t *testing.T) {
	home := t.TempDir()
	legacyDir := seedLegacyAntigravityPlugin(t, home, "/opt/engram/bin/engram")
	failAntigravityWrite(t, filepath.Join(".gemini", "config", "mcp_config.json"), false)

	if _, err := Inject(home, antigravityAdapter()); err == nil {
		t.Fatalf("Inject(antigravity) error = nil, want the injected write fault")
	}

	for _, name := range []string{"plugin.json", "mcp_config.json", "hooks.json"} {
		if _, statErr := os.Stat(filepath.Join(legacyDir, name)); statErr != nil {
			t.Fatalf("legacy asset %q must survive a failed new registration: %v", name, statErr)
		}
	}
}

// #5930 T8: a preserved command that no longer resolves would leave agy with
// a disconnected Engram, so the fresh-install command replaces it.
func TestInjectAntigravityReplacesUnresolvableLegacyCommand(t *testing.T) {
	nonExecutable := filepath.Join(t.TempDir(), "engram")
	if err := os.WriteFile(nonExecutable, []byte("not a binary"), 0o644); err != nil {
		t.Fatalf("WriteFile(%q) error = %v", nonExecutable, err)
	}
	tests := []struct {
		name    string
		command string
	}{
		{name: "missing absolute path", command: filepath.Join(t.TempDir(), "gone", "engram")},
		{name: "bare name not on PATH", command: "engram"},
	}
	if runtime.GOOS != "windows" {
		tests = append(tests, struct {
			name    string
			command string
		}{name: "non-executable file", command: nonExecutable})
	}
	for _, tt := range tests {
		for _, migrated := range []bool{false, true} {
			name := tt.name + " in legacy plugin"
			if migrated {
				name = tt.name + " in migrated plugin"
			}
			t.Run(name, func(t *testing.T) {
				home := t.TempDir()
				fresh := fakeEngramBinaryForTest(t)
				t.Setenv("GENTLE_AI_ENGRAM_BIN", fresh)
				SetLookPathForTest(t, "", "engram not on PATH")
				if migrated {
					pluginMCP := filepath.Join(sharedAntigravityPluginDir(home), "mcp_config.json")
					if err := os.MkdirAll(filepath.Dir(pluginMCP), 0o755); err != nil {
						t.Fatalf("MkdirAll(plugin) error = %v", err)
					}
					if err := os.WriteFile(pluginMCP, engramOverlayJSON(model.AgentAntigravity, tt.command), 0o644); err != nil {
						t.Fatalf("WriteFile(plugin mcp) error = %v", err)
					}
				} else {
					seedLegacyAntigravityPlugin(t, home, tt.command)
				}

				if _, err := Inject(home, antigravityAdapter()); err != nil {
					t.Fatalf("Inject(antigravity) error = %v", err)
				}

				gotMCP := readAntigravityFile(t, filepath.Join(home, ".gemini", "config", "mcp_config.json"))
				if want := engramOverlayJSON(model.AgentAntigravity, fresh); !bytes.Equal(gotMCP, want) {
					t.Fatalf("global MCP config must use the fresh-install command instead of %q\nwant:\n%s\ngot:\n%s", tt.command, want, gotMCP)
				}
			})
		}
	}
}

func TestRemoveManagedAntigravityPluginAssetRecognizesLegacyHooks(t *testing.T) {
	for _, event := range []string{"PreInvocation", "SessionStart"} {
		t.Run(event+" without timeout", func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "hooks.json")
			if err := os.WriteFile(path, legacyHooksWithoutTimeoutForTest(t, event), 0o644); err != nil {
				t.Fatalf("WriteFile(hooks) error = %v", err)
			}
			removed, err := RemoveManagedAntigravityPluginAsset(path)
			if err != nil || !removed {
				t.Fatalf("RemoveManagedAntigravityPluginAsset(legacy %s hooks) = %v, %v; want removed", event, removed, err)
			}
		})
	}
}

// ─── #5930 S7 / #5607: doctor reads the plugin-owned registration ────────────

func TestReadPersistedStdioCommandsReadsAntigravityPluginRegistration(t *testing.T) {
	home := t.TempDir()
	pluginMCP := filepath.Join(sharedAntigravityPluginDir(home), "mcp_config.json")
	if err := os.MkdirAll(filepath.Dir(pluginMCP), 0o755); err != nil {
		t.Fatalf("MkdirAll(plugin) error = %v", err)
	}
	if err := os.WriteFile(pluginMCP, engramOverlayJSON(model.AgentAntigravity, "/configured/agy-engram"), 0o644); err != nil {
		t.Fatalf("WriteFile(plugin mcp) error = %v", err)
	}

	commands, err := ReadPersistedStdioCommands(home, []string{string(model.AgentAntigravity)})
	if err != nil {
		t.Fatalf("ReadPersistedStdioCommands() error = %v", err)
	}
	if len(commands) != 1 {
		t.Fatalf("ReadPersistedStdioCommands() = %+v, want the plugin-owned registration", commands)
	}
	got := commands[0]
	if got.Command != "/configured/agy-engram" || got.Source != pluginMCP || !slices.Equal(got.Args, []string{"mcp", "--tools=agent"}) {
		t.Fatalf("plugin registration = %+v, want /configured/agy-engram [mcp --tools=agent] from %q", got, pluginMCP)
	}
}

func TestReadPersistedStdioCommandsKeepsGeminiConfigUnchanged(t *testing.T) {
	home := t.TempDir()
	settings := filepath.Join(home, ".gemini", "settings.json")
	if err := os.MkdirAll(filepath.Dir(settings), 0o755); err != nil {
		t.Fatalf("MkdirAll(gemini) error = %v", err)
	}
	if err := os.WriteFile(settings, []byte(`{"mcpServers":{"engram":{"command":"/configured/gemini-engram","args":["mcp"]}}}`), 0o644); err != nil {
		t.Fatalf("WriteFile(gemini settings) error = %v", err)
	}

	commands, err := ReadPersistedStdioCommands(home, []string{string(model.AgentGeminiCLI)})
	if err != nil {
		t.Fatalf("ReadPersistedStdioCommands() error = %v", err)
	}
	if len(commands) != 1 || commands[0].Command != "/configured/gemini-engram" || commands[0].Source != settings {
		t.Fatalf("gemini-cli registration = %+v, want only %q", commands, settings)
	}
}

func TestAntigravityCoordinationLockKeyStaysStableAcrossPluginMove(t *testing.T) {
	home := t.TempDir()
	target, err := antigravityLockTarget(home)
	if err != nil {
		t.Fatalf("antigravityLockTarget() error = %v", err)
	}
	canonical, err := filepath.EvalSymlinks(home)
	if err != nil {
		t.Fatalf("EvalSymlinks() error = %v", err)
	}
	if want := legacyAntigravityPluginDirForTest(canonical); target != want {
		t.Fatalf("lock key = %q, want the stable legacy key %q so older writers stay serialized", target, want)
	}
}

func TestRetireManagedAntigravityPluginsMigratesCustomEnvAndCwdToGlobalMCP(t *testing.T) {
	home := t.TempDir()
	pluginDir := AntigravityPluginDir(home)
	if err := os.MkdirAll(pluginDir, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(pluginDir, "plugin.json"), []byte(antigravityEngramPluginJSON), 0o644); err != nil {
		t.Fatalf("WriteFile(plugin.json) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(pluginDir, "hooks.json"), antigravityEngramHooksJSON(), 0o644); err != nil {
		t.Fatalf("WriteFile(hooks.json) error = %v", err)
	}
	pluginMCP := `{
  "mcpServers": {
    "engram": {
      "command": "engram",
      "args": ["mcp", "--tools=agent"],
      "env": {"CUSTOM_KEY": "custom_val"},
      "cwd": "/custom/workspace"
    }
  }
}
`
	if err := os.WriteFile(filepath.Join(pluginDir, "mcp_config.json"), []byte(pluginMCP), 0o644); err != nil {
		t.Fatalf("WriteFile(mcp_config.json) error = %v", err)
	}

	globalMCP := filepath.Join(home, ".gemini", "config", "mcp_config.json")
	if err := os.MkdirAll(filepath.Dir(globalMCP), 0o755); err != nil {
		t.Fatalf("MkdirAll(global) error = %v", err)
	}
	defaultGlobal := `{
  "mcpServers": {
    "engram": {
      "command": "engram",
      "args": ["mcp", "--tools=agent"]
    }
  }
}
`
	if err := os.WriteFile(globalMCP, []byte(defaultGlobal), 0o644); err != nil {
		t.Fatalf("WriteFile(global) error = %v", err)
	}

	removed, err := RetireManagedAntigravityPlugins(home)
	if err != nil {
		t.Fatalf("RetireManagedAntigravityPlugins() error = %v", err)
	}
	if len(removed) == 0 {
		t.Fatalf("RetireManagedAntigravityPlugins() removed no files")
	}

	// Verify plugin dir was retired.
	if _, err := os.Stat(pluginDir); !os.IsNotExist(err) {
		t.Fatalf("plugin dir %q should be removed after migration; stat err = %v", pluginDir, err)
	}

	// Verify global mcp_config.json now contains the migrated env and cwd.
	raw, err := os.ReadFile(globalMCP)
	if err != nil {
		t.Fatalf("ReadFile(globalMCP) error = %v", err)
	}
	var cfg struct {
		MCPServers map[string]struct {
			Command string            `json:"command"`
			Args    []string          `json:"args"`
			Env     map[string]string `json:"env"`
			Cwd     string            `json:"cwd"`
		} `json:"mcpServers"`
	}
	if err := json.Unmarshal(raw, &cfg); err != nil {
		t.Fatalf("Unmarshal(globalMCP) error = %v", err)
	}
	engramServer := cfg.MCPServers["engram"]
	if engramServer.Env["CUSTOM_KEY"] != "custom_val" {
		t.Fatalf("global engram env = %v, want CUSTOM_KEY=custom_val", engramServer.Env)
	}
	if engramServer.Cwd != "/custom/workspace" {
		t.Fatalf("global engram cwd = %q, want /custom/workspace", engramServer.Cwd)
	}
}

func TestRetireManagedAntigravityPluginsReportsConflictWhenGlobalMCPHasConflictingEnvOrCwd(t *testing.T) {
	home := t.TempDir()
	pluginDir := AntigravityPluginDir(home)
	if err := os.MkdirAll(pluginDir, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(pluginDir, "plugin.json"), []byte(antigravityEngramPluginJSON), 0o644); err != nil {
		t.Fatalf("WriteFile(plugin.json) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(pluginDir, "hooks.json"), antigravityEngramHooksJSON(), 0o644); err != nil {
		t.Fatalf("WriteFile(hooks.json) error = %v", err)
	}
	pluginMCP := `{
  "mcpServers": {
    "engram": {
      "command": "engram",
      "args": ["mcp", "--tools=agent"],
      "env": {"KEY": "from_plugin"}
    }
  }
}
`
	if err := os.WriteFile(filepath.Join(pluginDir, "mcp_config.json"), []byte(pluginMCP), 0o644); err != nil {
		t.Fatalf("WriteFile(mcp_config.json) error = %v", err)
	}

	globalMCP := filepath.Join(home, ".gemini", "config", "mcp_config.json")
	if err := os.MkdirAll(filepath.Dir(globalMCP), 0o755); err != nil {
		t.Fatalf("MkdirAll(global) error = %v", err)
	}
	conflictingGlobal := `{
  "mcpServers": {
    "engram": {
      "command": "engram",
      "args": ["mcp", "--tools=agent"],
      "env": {"KEY": "from_global"}
    }
  }
}
`
	if err := os.WriteFile(globalMCP, []byte(conflictingGlobal), 0o644); err != nil {
		t.Fatalf("WriteFile(global) error = %v", err)
	}

	_, err := RetireManagedAntigravityPlugins(home)
	if err == nil {
		t.Fatalf("RetireManagedAntigravityPlugins() error = nil, want conflict error")
	}
	if !strings.Contains(err.Error(), "conflict") {
		t.Fatalf("error = %v, want conflict error", err)
	}

	// Verify plugin manifest survives and edited mcp_config.json is not left inactive.
	if _, err := os.Stat(filepath.Join(pluginDir, "plugin.json")); err != nil {
		t.Fatalf("plugin.json must survive when retirement is stopped: %v", err)
	}
	if _, err := os.Stat(filepath.Join(pluginDir, "mcp_config.json")); err != nil {
		t.Fatalf("mcp_config.json must survive when retirement is stopped: %v", err)
	}
}

func TestRemoveManagedAntigravityPluginAssetReportsConflictForConflictingPluginMCP(t *testing.T) {
	home := t.TempDir()
	pluginDir := AntigravityPluginDir(home)
	if err := os.MkdirAll(pluginDir, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	pluginJSON := filepath.Join(pluginDir, "plugin.json")
	if err := os.WriteFile(pluginJSON, []byte(antigravityEngramPluginJSON), 0o644); err != nil {
		t.Fatalf("WriteFile(plugin.json) error = %v", err)
	}
	pluginMCP := `{
  "mcpServers": {
    "engram": {
      "command": "engram",
      "args": ["mcp", "--tools=agent"],
      "cwd": "/plugin/cwd"
    }
  }
}
`
	if err := os.WriteFile(filepath.Join(pluginDir, "mcp_config.json"), []byte(pluginMCP), 0o644); err != nil {
		t.Fatalf("WriteFile(mcp_config.json) error = %v", err)
	}

	globalMCP := filepath.Join(home, ".gemini", "config", "mcp_config.json")
	if err := os.MkdirAll(filepath.Dir(globalMCP), 0o755); err != nil {
		t.Fatalf("MkdirAll(global) error = %v", err)
	}
	conflictingGlobal := `{
  "mcpServers": {
    "engram": {
      "command": "engram",
      "args": ["mcp", "--tools=agent"],
      "cwd": "/global/cwd"
    }
  }
}
`
	if err := os.WriteFile(globalMCP, []byte(conflictingGlobal), 0o644); err != nil {
		t.Fatalf("WriteFile(global) error = %v", err)
	}

	removed, err := RemoveManagedAntigravityPluginAsset(pluginJSON)
	if err == nil {
		t.Fatalf("RemoveManagedAntigravityPluginAsset(plugin.json) error = nil, want conflict error")
	}
	if removed {
		t.Fatalf("RemoveManagedAntigravityPluginAsset(plugin.json) returned true, want false")
	}
	if !strings.Contains(err.Error(), "conflict") {
		t.Fatalf("error = %v, want conflict error", err)
	}
	if _, err := os.Stat(pluginJSON); err != nil {
		t.Fatalf("plugin.json must survive when retirement is stopped: %v", err)
	}
}

func TestRemoveManagedAntigravityPluginAssetForUninstallDoesNotReRegisterGlobalEngram(t *testing.T) {
	home := t.TempDir()
	pluginDir := AntigravityPluginDir(home)
	if err := os.MkdirAll(pluginDir, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	pluginJSON := filepath.Join(pluginDir, "plugin.json")
	if err := os.WriteFile(pluginJSON, []byte(antigravityEngramPluginJSON), 0o644); err != nil {
		t.Fatalf("WriteFile(plugin.json) error = %v", err)
	}
	pluginMCP := `{
  "mcpServers": {
    "engram": {
      "command": "engram",
      "args": ["mcp", "--tools=agent"],
      "env": {"CUSTOM_KEY": "custom_val"}
    }
  }
}
`
	if err := os.WriteFile(filepath.Join(pluginDir, "mcp_config.json"), []byte(pluginMCP), 0o644); err != nil {
		t.Fatalf("WriteFile(mcp_config.json) error = %v", err)
	}

	globalMCP := filepath.Join(home, ".gemini", "config", "mcp_config.json")

	// Call the uninstall-specific remover.
	removed, err := RemoveManagedAntigravityPluginAssetForUninstall(pluginJSON)
	if err != nil {
		t.Fatalf("RemoveManagedAntigravityPluginAssetForUninstall error = %v", err)
	}
	if !removed {
		t.Fatalf("RemoveManagedAntigravityPluginAssetForUninstall returned false, want true")
	}

	// Verify global MCP was NOT created/re-registered.
	if _, err := os.Stat(globalMCP); !os.IsNotExist(err) {
		t.Fatalf("global MCP must not be created or re-registered during uninstall")
	}
}

func TestRemoveManagedAntigravityPluginAssetReturnsReadErrorForUnreadableSiblingMCP(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("permission checks rely on POSIX mode bits")
	}
	home := t.TempDir()
	pluginDir := AntigravityPluginDir(home)
	if err := os.MkdirAll(pluginDir, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	pluginJSON := filepath.Join(pluginDir, "plugin.json")
	if err := os.WriteFile(pluginJSON, []byte(antigravityEngramPluginJSON), 0o644); err != nil {
		t.Fatalf("WriteFile(plugin.json) error = %v", err)
	}
	mcpPath := filepath.Join(pluginDir, "mcp_config.json")
	if err := os.WriteFile(mcpPath, []byte("{}"), 0o000); err != nil {
		t.Fatalf("WriteFile(mcp_config.json) error = %v", err)
	}

	removed, err := RemoveManagedAntigravityPluginAsset(pluginJSON)
	if err == nil {
		t.Fatalf("RemoveManagedAntigravityPluginAsset should fail when sibling MCP is unreadable")
	}
	if removed {
		t.Fatalf("RemoveManagedAntigravityPluginAsset returned true, want false")
	}
	if !strings.Contains(err.Error(), "read sibling plugin MCP config") {
		t.Fatalf("error = %v, want 'read sibling plugin MCP config'", err)
	}
	if _, err := os.Stat(pluginJSON); err != nil {
		t.Fatalf("plugin.json must survive when sibling read fails: %v", err)
	}
}

func TestRetireManagedAntigravityPluginsMigratesCustomArgs(t *testing.T) {
	home := t.TempDir()
	pluginDir := AntigravityPluginDir(home)
	if err := os.MkdirAll(pluginDir, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(pluginDir, "plugin.json"), []byte(antigravityEngramPluginJSON), 0o644); err != nil {
		t.Fatalf("WriteFile(plugin.json) error = %v", err)
	}
	pluginMCP := `{
  "mcpServers": {
    "engram": {
      "command": "engram",
      "args": ["mcp", "--tools=custom", "--verbose"]
    }
  }
}
`
	if err := os.WriteFile(filepath.Join(pluginDir, "mcp_config.json"), []byte(pluginMCP), 0o644); err != nil {
		t.Fatalf("WriteFile(mcp_config.json) error = %v", err)
	}

	globalMCP := filepath.Join(home, ".gemini", "config", "mcp_config.json")
	if err := os.MkdirAll(filepath.Dir(globalMCP), 0o755); err != nil {
		t.Fatalf("MkdirAll(global) error = %v", err)
	}
	globalContent := `{
  "mcpServers": {
    "engram": {
      "command": "engram",
      "args": ["mcp", "--tools=agent"]
    }
  }
}
`
	if err := os.WriteFile(globalMCP, []byte(globalContent), 0o644); err != nil {
		t.Fatalf("WriteFile(global) error = %v", err)
	}

	changed, err := RetireManagedAntigravityPlugins(home)
	if err != nil {
		t.Fatalf("RetireManagedAntigravityPlugins error = %v", err)
	}
	if !slices.Contains(changed, globalMCP) {
		t.Fatalf("RetireManagedAntigravityPlugins changed = %v, want globalMCP %q", changed, globalMCP)
	}

	// Verify migrated args.
	data, err := os.ReadFile(globalMCP)
	if err != nil {
		t.Fatalf("ReadFile(globalMCP) error = %v", err)
	}
	var cfg struct {
		MCPServers map[string]struct {
			Args []string `json:"args"`
		} `json:"mcpServers"`
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("Unmarshal error = %v", err)
	}
	server := cfg.MCPServers["engram"]
	wantArgs := []string{"mcp", "--tools=custom", "--verbose"}
	if !slices.Equal(server.Args, wantArgs) {
		t.Fatalf("migrated args = %v, want %v", server.Args, wantArgs)
	}
}

func TestRetireManagedAntigravityPluginsRestrictsPermissionsWhenMigratingSecrets(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("permission checks rely on POSIX mode bits")
	}
	home := t.TempDir()
	pluginDir := AntigravityPluginDir(home)
	if err := os.MkdirAll(pluginDir, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(pluginDir, "plugin.json"), []byte(antigravityEngramPluginJSON), 0o644); err != nil {
		t.Fatalf("WriteFile(plugin.json) error = %v", err)
	}
	pluginMCP := `{
  "mcpServers": {
    "engram": {
      "command": "engram",
      "args": ["mcp", "--tools=agent"],
      "env": {"API_SECRET_TOKEN": "supersecret"}
    }
  }
}
`
	if err := os.WriteFile(filepath.Join(pluginDir, "mcp_config.json"), []byte(pluginMCP), 0o644); err != nil {
		t.Fatalf("WriteFile(mcp_config.json) error = %v", err)
	}

	globalMCP := filepath.Join(home, ".gemini", "config", "mcp_config.json")
	if err := os.MkdirAll(filepath.Dir(globalMCP), 0o755); err != nil {
		t.Fatalf("MkdirAll(global) error = %v", err)
	}
	// Initial global MCP has 0644 mode
	if err := os.WriteFile(globalMCP, []byte(`{"mcpServers":{}}`), 0o644); err != nil {
		t.Fatalf("WriteFile(global) error = %v", err)
	}

	if _, err := RetireManagedAntigravityPlugins(home); err != nil {
		t.Fatalf("RetireManagedAntigravityPlugins error = %v", err)
	}

	info, err := os.Stat(globalMCP)
	if err != nil {
		t.Fatalf("Stat(globalMCP) error = %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Fatalf("globalMCP mode = %04o, want 0600", perm)
	}
}

func TestRetireManagedAntigravityPluginsRedactsEnvValuesInConflictError(t *testing.T) {
	home := t.TempDir()
	pluginDir := AntigravityPluginDir(home)
	if err := os.MkdirAll(pluginDir, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(pluginDir, "plugin.json"), []byte(antigravityEngramPluginJSON), 0o644); err != nil {
		t.Fatalf("WriteFile(plugin.json) error = %v", err)
	}
	secretPluginVal := "SUPER_SECRET_PLUGIN_VALUE"
	secretGlobalVal := "SUPER_SECRET_GLOBAL_VALUE"
	pluginMCP := `{
  "mcpServers": {
    "engram": {
      "command": "engram",
      "args": ["mcp", "--tools=agent"],
      "env": {"SECRET_KEY": "` + secretPluginVal + `"}
    }
  }
}
`
	if err := os.WriteFile(filepath.Join(pluginDir, "mcp_config.json"), []byte(pluginMCP), 0o644); err != nil {
		t.Fatalf("WriteFile(mcp_config.json) error = %v", err)
	}

	globalMCP := filepath.Join(home, ".gemini", "config", "mcp_config.json")
	if err := os.MkdirAll(filepath.Dir(globalMCP), 0o755); err != nil {
		t.Fatalf("MkdirAll(global) error = %v", err)
	}
	globalMCPContent := `{
  "mcpServers": {
    "engram": {
      "command": "engram",
      "args": ["mcp", "--tools=agent"],
      "env": {"SECRET_KEY": "` + secretGlobalVal + `"}
    }
  }
}
`
	if err := os.WriteFile(globalMCP, []byte(globalMCPContent), 0o644); err != nil {
		t.Fatalf("WriteFile(global) error = %v", err)
	}

	_, err := RetireManagedAntigravityPlugins(home)
	if err == nil {
		t.Fatalf("RetireManagedAntigravityPlugins want conflict error, got nil")
	}
	errStr := err.Error()
	if !strings.Contains(errStr, "SECRET_KEY") {
		t.Fatalf("error should name key 'SECRET_KEY', got: %v", errStr)
	}
	if strings.Contains(errStr, secretPluginVal) || strings.Contains(errStr, secretGlobalVal) {
		t.Fatalf("error must not leak secret values; got: %v", errStr)
	}
}

func TestRetireManagedAntigravityPluginsDoesNotConflictOnDefaultEngramCommand(t *testing.T) {
	home := t.TempDir()
	pluginDir := AntigravityPluginDir(home)
	if err := os.MkdirAll(pluginDir, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(pluginDir, "plugin.json"), []byte(antigravityEngramPluginJSON), 0o644); err != nil {
		t.Fatalf("WriteFile(plugin.json) error = %v", err)
	}
	pluginMCP := `{
  "mcpServers": {
    "engram": {
      "command": "engram",
      "args": ["mcp", "--tools=agent"],
      "env": {"CUSTOM_ENV": "1"}
    }
  }
}
`
	if err := os.WriteFile(filepath.Join(pluginDir, "mcp_config.json"), []byte(pluginMCP), 0o644); err != nil {
		t.Fatalf("WriteFile(mcp_config.json) error = %v", err)
	}

	globalMCP := filepath.Join(home, ".gemini", "config", "mcp_config.json")
	if err := os.MkdirAll(filepath.Dir(globalMCP), 0o755); err != nil {
		t.Fatalf("MkdirAll(global) error = %v", err)
	}
	globalMCPContent := `{
  "mcpServers": {
    "engram": {
      "command": "/usr/local/bin/engram",
      "args": ["mcp", "--tools=agent"]
    }
  }
}
`
	if err := os.WriteFile(globalMCP, []byte(globalMCPContent), 0o644); err != nil {
		t.Fatalf("WriteFile(global) error = %v", err)
	}

	if _, err := RetireManagedAntigravityPlugins(home); err != nil {
		t.Fatalf("RetireManagedAntigravityPlugins error = %v, want success (engram vs /usr/local/bin/engram is not conflict)", err)
	}
}
