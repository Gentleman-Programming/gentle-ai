package opencodeplugin

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/internal/model"
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
