package cli

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v4/internal/model"
	opencodeactivation "github.com/gentleman-programming/gentle-ai/v4/internal/opencode"
	"github.com/gentleman-programming/gentle-ai/v4/internal/pipeline"
)

// Issue #5364: OpenCode 2.x receives the gentle-logo like any other runtime.
// A full install followed by sync must ship the pre-built bundle plus the V2
// bridge directory, register the relative directory package in cli.json and
// the managed opencode.jsonc, and retire the stale T1-era absolute tui.json
// registration without touching the user's other entries.
func TestOpenCodeV2SyncWithGentleLogoInstallsAndVerifies(t *testing.T) {
	home := t.TempDir()
	setOpenCodeTestHome(t, home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg"))
	t.Setenv("OPENCODE_CONFIG_DIR", "")
	t.Setenv("DO_NOT_TRACK", "1")
	old := opencodeactivation.VersionRunnerOverride
	t.Cleanup(func() { opencodeactivation.VersionRunnerOverride = old })
	opencodeactivation.VersionRunnerOverride = func(context.Context, opencodeactivation.Command) (opencodeactivation.CommandOutput, error) {
		return opencodeactivation.CommandOutput{Stdout: []byte("2.0.23")}, nil
	}
	mustWriteFile(t, filepath.Join(home, "xdg", "opencode", "node_modules", "@opencode", "plugin", "package.json"), []byte(`{"version":"2.0.4"}`))

	// A T1-era installation registered an absolute .tsx path in tui.json and
	// the user had another plugin registered beside it; OpenCode 2.0.24 has
	// also written its own cli.json, so the durable artifact selects V2.
	configDir := filepath.Join(home, ".config", "opencode")
	tuiPath := filepath.Join(configDir, "tui.json")
	staleAbsolute := filepath.Join(configDir, "tui-plugins", "gentle-logo.tsx")
	mustWriteFile(t, filepath.Join(configDir, "cli.json"), []byte(`{"$schema":"https://opencode.ai/v2/cli.json","plugins":[]}`))
	mustWriteFile(t, tuiPath, []byte(`{"$schema":"https://opencode.ai/tui.json","plugin":["user-plugin","`+filepath.ToSlash(staleAbsolute)+`"]}`))

	selection := model.Selection{
		Agents:     []model.AgentID{model.AgentOpenCode},
		Components: []model.ComponentID{model.ComponentOpenCodeGentleLogo},
	}
	rt := newTestInstallRuntime(t, home, selection)
	if result := pipeline.NewOrchestrator(pipeline.DefaultRollbackPolicy()).Execute(rt.stagePlan()); result.Err != nil {
		t.Fatalf("install on OpenCode 2.x with the Gentle logo selected: %v", result.Err)
	}
	if _, err := RunSyncWithSelection(home, selection); err != nil {
		t.Fatalf("sync on OpenCode 2.x with the Gentle logo selected: %v", err)
	}

	bundlePath := filepath.Join(configDir, "tui-plugins", "gentle-logo.js")
	if _, err := os.Lstat(bundlePath); err != nil {
		t.Fatalf("OpenCode 2.x did not receive the logo bundle: %v", err)
	}
	if _, err := os.Lstat(filepath.Join(configDir, "tui-plugins", "gentle-logo", "tui.js")); err != nil {
		t.Fatalf("OpenCode 2.x did not receive the bridge directory: %v", err)
	}

	cliData, err := os.ReadFile(filepath.Join(configDir, "cli.json"))
	if err != nil {
		t.Fatalf("ReadFile(cli.json) error = %v", err)
	}
	var cliConfig struct {
		Plugins []string `json:"plugins"`
	}
	if err := json.Unmarshal(cliData, &cliConfig); err != nil {
		t.Fatalf("Unmarshal(cli.json) error = %v", err)
	}
	found := false
	for _, registered := range cliConfig.Plugins {
		if registered == "./tui-plugins/gentle-logo" {
			found = true
		}
	}
	if !found {
		t.Fatalf("cli.json plugins = %#v, want the relative bridge entry", cliConfig.Plugins)
	}

	tuiData, err := os.ReadFile(tuiPath)
	if err != nil {
		t.Fatalf("ReadFile(tui.json) error = %v", err)
	}
	var tuiConfig struct {
		Plugin []string `json:"plugin"`
	}
	if err := json.Unmarshal(tuiData, &tuiConfig); err != nil {
		t.Fatalf("Unmarshal(tui.json) error = %v", err)
	}
	if len(tuiConfig.Plugin) != 1 || tuiConfig.Plugin[0] != "user-plugin" {
		t.Fatalf("tui.json plugin = %#v, want only user-plugin (stale T1 entry removed)", tuiConfig.Plugin)
	}
}
