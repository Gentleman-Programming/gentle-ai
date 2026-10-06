package uninstall

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v4/internal/assets"
	"github.com/gentleman-programming/gentle-ai/v4/internal/components/opencoderuntimeplugins"
	"github.com/gentleman-programming/gentle-ai/v4/internal/model"
)

// Kilocode receives its own managed plugin set under ~/.config/kilo/plugins; a
// complete Kilocode removal deletes the released bytes there, as it does for
// OpenCode, and keeps edited or foreign bytes.
func TestFullAgentKilocodeUninstallRemovesReleasedPluginBytes(t *testing.T) {
	homeDir := t.TempDir()
	svc, err := NewService(homeDir, t.TempDir(), "dev")
	if err != nil {
		t.Fatal(err)
	}
	adapter, ok := svc.registry.Get(model.AgentKilocode)
	if !ok {
		t.Fatal("Kilocode adapter not found")
	}
	assetDir, err := opencoderuntimeplugins.AssetDirectory(model.AgentKilocode)
	if err != nil {
		t.Fatal(err)
	}
	current := func(name string) []byte {
		data, err := assets.Read(assetDir + name)
		if err != nil {
			t.Fatal(err)
		}
		return []byte(data)
	}
	released := func(rel string) []byte {
		data, err := os.ReadFile(filepath.Join("..", "opencoderuntimeplugins", "testdata", "released", filepath.FromSlash(rel)))
		if err != nil {
			t.Fatal(err)
		}
		return data
	}
	pluginDir := filepath.Join(adapter.GlobalConfigDir(homeDir), "plugins")
	if err := os.MkdirAll(pluginDir, 0o755); err != nil {
		t.Fatal(err)
	}
	owned := map[string][]byte{
		"model-variants.ts":          current("model-variants.ts"),
		"background-agents.ts":       released("v1.33.2/plugins/background-agents.ts"),
		"review-result-artifacts.ts": released("v2.1.7/plugins/review-result-artifacts.ts"),
	}
	user := map[string][]byte{
		"skill-registry.ts": append(current("skill-registry.ts"), "// user edit\n"...),
		"my-plugin.ts":      []byte("export default {}\n"),
	}
	for _, files := range []map[string][]byte{owned, user} {
		for name, data := range files {
			if err := os.WriteFile(filepath.Join(pluginDir, name), data, 0o644); err != nil {
				t.Fatal(err)
			}
		}
	}

	plan, err := svc.buildPlan([]model.AgentID{model.AgentKilocode}, allManagedComponents)
	if err != nil {
		t.Fatal(err)
	}
	result, err := svc.executePlan(plan, []model.AgentID{model.AgentKilocode})
	if err != nil {
		t.Fatal(err)
	}
	for name := range owned {
		path := filepath.Join(pluginDir, name)
		if _, err := os.Lstat(path); !os.IsNotExist(err) {
			t.Errorf("released Kilocode plugin bytes %s kept: %v", name, err)
		}
		if !slices.Contains(result.RemovedFiles, path) {
			t.Errorf("released Kilocode plugin removal not reported: %s", path)
		}
	}
	for name, data := range user {
		if got, err := os.ReadFile(filepath.Join(pluginDir, name)); err != nil || string(got) != string(data) {
			t.Errorf("user Kilocode plugin bytes %s changed: %v", name, err)
		}
	}
}
