package engram

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
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

func TestAntigravityPluginDirUsesSharedConfigRoot(t *testing.T) {
	home := t.TempDir()
	if got, want := AntigravityPluginDir(home), sharedAntigravityPluginDir(home); got != want {
		t.Fatalf("AntigravityPluginDir() = %q, want %q", got, want)
	}
	if got, want := AntigravityLegacyPluginDir(home), legacyAntigravityPluginDirForTest(home); got != want {
		t.Fatalf("AntigravityLegacyPluginDir() = %q, want %q", got, want)
	}
}

func TestInjectAntigravityWritesPluginWhereAgyLoadsIt(t *testing.T) {
	home := t.TempDir()

	result, err := Inject(home, antigravityAdapter())
	if err != nil {
		t.Fatalf("Inject(antigravity) error = %v", err)
	}
	for _, name := range []string{"plugin.json", "mcp_config.json", "hooks.json"} {
		path := filepath.Join(sharedAntigravityPluginDir(home), name)
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("plugin asset %q missing under the shared config root: %v", path, err)
		}
		if !slices.Contains(result.Files, path) {
			t.Fatalf("result.Files = %v, want it to report %q", result.Files, path)
		}
	}
	if _, err := os.Stat(filepath.Join(home, ".gemini", "antigravity-cli", "plugins")); !os.IsNotExist(err) {
		t.Fatalf("no plugin may be written under antigravity-cli/plugins; stat err = %v", err)
	}
}

func TestInjectAntigravityHookRunsOncePerSession(t *testing.T) {
	home := t.TempDir()
	if _, err := Inject(home, antigravityAdapter()); err != nil {
		t.Fatalf("Inject(antigravity) error = %v", err)
	}
	raw := readAntigravityFile(t, filepath.Join(sharedAntigravityPluginDir(home), "hooks.json"))
	var hooks map[string]map[string][]struct {
		Type    string `json:"type"`
		Command string `json:"command"`
		Timeout int    `json:"timeout"`
	}
	if err := json.Unmarshal(raw, &hooks); err != nil {
		t.Fatalf("Unmarshal(hooks.json) error = %v\n%s", err, raw)
	}
	events := hooks["gentle-ai-engram-tools"]
	if len(events) != 1 || len(events["SessionStart"]) != 1 {
		t.Fatalf("hooks must register exactly one SessionStart command and no per-invocation hook; got:\n%s", raw)
	}
	entry := events["SessionStart"][0]
	if entry.Type != "command" {
		t.Fatalf("SessionStart hook type = %q, want command", entry.Type)
	}
	if entry.Timeout != 10 {
		t.Fatalf("SessionStart hook timeout = %d, want an explicit 10", entry.Timeout)
	}
	if runtime.GOOS == "windows" {
		return
	}
	out, err := exec.Command("sh", "-c", entry.Command).Output()
	if err != nil {
		t.Fatalf("run SessionStart hook command error = %v", err)
	}
	var payload struct {
		InjectSteps []struct {
			EphemeralMessage string `json:"ephemeralMessage"`
		} `json:"injectSteps"`
	}
	if err := json.Unmarshal(out, &payload); err != nil {
		t.Fatalf("SessionStart hook stdout is not the injectSteps contract: %v\n%s", err, out)
	}
	if len(payload.InjectSteps) != 1 || payload.InjectSteps[0].EphemeralMessage != antigravityEngramToolsMessage {
		t.Fatalf("SessionStart hook must emit the Engram guidance as one ephemeral step; got %s", out)
	}
}

func TestInjectAntigravityMigratesLegacyManagedPlugin(t *testing.T) {
	home := t.TempDir()
	legacyDir := seedLegacyAntigravityPlugin(t, home, "/opt/engram/bin/engram")
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

	// The registration survives the move: the legacy command is carried over.
	gotMCP := readAntigravityFile(t, filepath.Join(sharedAntigravityPluginDir(home), "mcp_config.json"))
	if want := engramOverlayJSON(model.AgentAntigravity, "/opt/engram/bin/engram"); !bytes.Equal(gotMCP, want) {
		t.Fatalf("migrated plugin MCP config must keep the legacy command\nwant:\n%s\ngot:\n%s", want, gotMCP)
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
	failAntigravityWrite(t, filepath.Join(".gemini", "config", "plugins", "gentle-ai-engram", "hooks.json"), false)

	if _, err := Inject(home, antigravityAdapter()); err == nil {
		t.Fatalf("Inject(antigravity) error = nil, want the injected write fault")
	}

	for _, name := range []string{"plugin.json", "mcp_config.json", "hooks.json"} {
		if _, statErr := os.Stat(filepath.Join(legacyDir, name)); statErr != nil {
			t.Fatalf("legacy asset %q must survive a failed new registration: %v", name, statErr)
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
