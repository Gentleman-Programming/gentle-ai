package opencodeplugin

import (
	"bytes"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/components/filemerge"
	"github.com/gentleman-programming/gentle-ai/v2/internal/model"
)

func TestInstallAddsCommunityPluginToTUIConfig(t *testing.T) {
	home := t.TempDir()

	result, err := Install(home, model.OpenCodePluginSubAgentStatusline)
	if err != nil {
		t.Fatalf("Install() error = %v", err)
	}
	if !result.Changed {
		t.Fatal("Install() changed = false, want true")
	}

	configPath := filepath.Join(home, ".config", "opencode", "tui.json")
	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("ReadFile(tui.json) error = %v", err)
	}

	var root struct {
		Plugin []string `json:"plugin"`
	}
	if err := json.Unmarshal(data, &root); err != nil {
		t.Fatalf("Unmarshal(tui.json) error = %v", err)
	}
	if len(root.Plugin) != 1 || root.Plugin[0] != "opencode-subagent-statusline" {
		t.Fatalf("plugin list = %#v, want opencode-subagent-statusline", root.Plugin)
	}
}

func TestInstallPreservesExistingTUIPluginsAndIsIdempotent(t *testing.T) {
	home := t.TempDir()
	configDir := filepath.Join(home, ".config", "opencode")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatal(err)
	}
	initial := []byte(`{"$schema":"https://opencode.ai/tui.json","plugin":["existing-plugin"]}`)
	if err := os.WriteFile(filepath.Join(configDir, "tui.json"), initial, 0o644); err != nil {
		t.Fatal(err)
	}

	first, err := Install(home, model.OpenCodePluginSDDEngramManage)
	if err != nil {
		t.Fatalf("first Install() error = %v", err)
	}
	second, err := Install(home, model.OpenCodePluginSDDEngramManage)
	if err != nil {
		t.Fatalf("second Install() error = %v", err)
	}
	if !first.Changed {
		t.Fatal("first Install() changed = false, want true")
	}
	if second.Changed {
		t.Fatal("second Install() changed = true, want false")
	}

	data, err := os.ReadFile(filepath.Join(configDir, "tui.json"))
	if err != nil {
		t.Fatal(err)
	}
	var root struct {
		Plugin []string `json:"plugin"`
	}
	if err := json.Unmarshal(data, &root); err != nil {
		t.Fatal(err)
	}
	want := []string{"existing-plugin", "opencode-sdd-engram-manage"}
	if len(root.Plugin) != len(want) {
		t.Fatalf("plugin list = %#v, want %#v", root.Plugin, want)
	}
	for i := range want {
		if root.Plugin[i] != want[i] {
			t.Fatalf("plugin list = %#v, want %#v", root.Plugin, want)
		}
	}
}

func TestInstallDoesNotRunPackageManager(t *testing.T) {
	home := t.TempDir()

	if _, err := Install(home, model.OpenCodePluginSubAgentStatusline); err != nil {
		t.Fatalf("Install() error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(home, ".config", "opencode", "node_modules")); !os.IsNotExist(err) {
		t.Fatalf("Install() should not create node_modules; stat err = %v", err)
	}
}

func TestInstallGentleLogoWritesLocalTUIPluginAndRegistersAbsolutePath(t *testing.T) {
	home := t.TempDir()

	result, err := Install(home, model.OpenCodePluginGentleLogo)
	if err != nil {
		t.Fatalf("Install() error = %v", err)
	}
	if !result.Changed {
		t.Fatal("Install() changed = false, want true")
	}

	pluginPath := filepath.Join(home, ".config", "opencode", "tui-plugins", "gentle-logo.tsx")
	configPath := filepath.Join(home, ".config", "opencode", "tui.json")
	wantFiles := map[string]bool{pluginPath: true, configPath: true}
	for _, file := range result.Files {
		delete(wantFiles, file)
	}
	if len(wantFiles) != 0 {
		t.Fatalf("Install() files = %#v, missing %#v", result.Files, wantFiles)
	}

	pluginData, err := os.ReadFile(pluginPath)
	if err != nil {
		t.Fatalf("ReadFile(plugin) error = %v", err)
	}
	pluginContent := string(pluginData)
	for _, snippet := range []string{
		`id = "gentle-logo"`,
		`home_logo`,
		`const plugin = { id: "gentle-logo", tui }`,
		`export default plugin`,
	} {
		if !strings.Contains(pluginContent, snippet) {
			t.Fatalf("plugin missing snippet %q", snippet)
		}
	}
	if strings.Contains(pluginContent, "server") {
		t.Fatalf("plugin must not export or define server shape")
	}
	for _, forbidden := range []string{"TuiThemeCurrent", "ctx.theme", "props.theme"} {
		if strings.Contains(pluginContent, forbidden) {
			t.Fatalf("plugin must not subscribe to theme state (%q) because OpenCode can destroy TextBuffer during theme swaps", forbidden)
		}
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("ReadFile(tui.json) error = %v", err)
	}
	var root struct {
		Plugin []string `json:"plugin"`
	}
	if err := json.Unmarshal(data, &root); err != nil {
		t.Fatalf("Unmarshal(tui.json) error = %v", err)
	}
	if len(root.Plugin) != 1 || root.Plugin[0] != pluginPath || !filepath.IsAbs(root.Plugin[0]) {
		t.Fatalf("plugin registration = %#v, want absolute %q", root.Plugin, pluginPath)
	}
}

func TestInstallGentleLogoRollsBackSourceWhenRegistrationFails(t *testing.T) {
	home := t.TempDir()
	configDir := filepath.Join(home, ".config", "opencode")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// Create malformed tui.json so ensureTUIPlugin returns a JSON parse error
	tuiPath := filepath.Join(configDir, "tui.json")
	if err := os.WriteFile(tuiPath, []byte("invalid json {{"), 0o644); err != nil {
		t.Fatal(err)
	}

	pluginPath := filepath.Join(configDir, "tui-plugins", "gentle-logo.tsx")

	// Case 1: File did not exist before -> should be removed on failure
	_, err := Install(home, model.OpenCodePluginGentleLogo)
	if err == nil {
		t.Fatal("Install(GentleLogo) error = nil, want parse error")
	}
	if _, statErr := os.Stat(pluginPath); !os.IsNotExist(statErr) {
		t.Fatalf("Install(GentleLogo) left orphan plugin file after registration failure: %v", statErr)
	}

	// Case 2: File existed before -> should be restored to original content on failure
	pluginDir := filepath.Join(configDir, "tui-plugins")
	if err := os.MkdirAll(pluginDir, 0o755); err != nil {
		t.Fatal(err)
	}
	originalContent := []byte{0x00, 0xff, 'G', 'A', 'I', '\n'}
	if err := os.WriteFile(pluginPath, originalContent, 0o600); err != nil {
		t.Fatal(err)
	}

	_, err = Install(home, model.OpenCodePluginGentleLogo)
	if err == nil {
		t.Fatal("Install(GentleLogo) error = nil, want parse error")
	}
	data, readErr := os.ReadFile(pluginPath)
	if readErr != nil {
		t.Fatalf("ReadFile(pluginPath) error = %v", readErr)
	}
	if !bytes.Equal(data, originalContent) {
		t.Fatalf("pluginPath content = %v, want restored original %v", data, originalContent)
	}
	info, statErr := os.Stat(pluginPath)
	if statErr != nil {
		t.Fatalf("Stat(pluginPath) error = %v", statErr)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
		t.Fatalf("pluginPath mode = %o, want restored 600", info.Mode().Perm())
	}
}

func TestInstallGentleLogoJoinsRegistrationAndRollbackErrors(t *testing.T) {
	home := t.TempDir()
	configDir := filepath.Join(home, ".config", "opencode")
	pluginPath := filepath.Join(configDir, "tui-plugins", gentleLogoPluginFile)
	if err := os.MkdirAll(filepath.Dir(pluginPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(pluginPath, []byte("original"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "tui.json"), []byte("invalid json {{"), 0o644); err != nil {
		t.Fatal(err)
	}

	rollbackErr := errors.New("rollback write failed")
	originalWrite := writeGentleLogoFileAtomic
	writes := 0
	writeGentleLogoFileAtomic = func(path string, content []byte, perm fs.FileMode) (filemerge.WriteResult, error) {
		writes++
		if writes == 2 {
			return filemerge.WriteResult{}, rollbackErr
		}
		return originalWrite(path, content, perm)
	}
	t.Cleanup(func() { writeGentleLogoFileAtomic = originalWrite })

	_, err := Install(home, model.OpenCodePluginGentleLogo)
	if err == nil || !strings.Contains(err.Error(), "parse OpenCode TUI config") || !errors.Is(err, rollbackErr) {
		t.Fatalf("Install(GentleLogo) error = %v, want joined registration and rollback errors", err)
	}
}
