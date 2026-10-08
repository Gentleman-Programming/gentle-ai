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

// Issue #5364: OpenCode 2.x receives the Gentle logo like any other runtime.
// A full install followed by sync must write the V2-shaped plugin, register
// it in tui.json, and leave the managed files in place for backup and
// verification (whose target lists now include them on V2).
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

	pluginPath := filepath.Join(home, ".config", "opencode", "tui-plugins", "gentle-logo.tsx")
	if _, err := os.Lstat(pluginPath); err != nil {
		t.Fatalf("OpenCode 2.x did not receive the logo plugin: %v", err)
	}
	configData, err := os.ReadFile(filepath.Join(home, ".config", "opencode", "tui.json"))
	if err != nil {
		t.Fatalf("ReadFile(tui.json) error = %v", err)
	}
	var config struct {
		Plugin []string `json:"plugin"`
	}
	if err := json.Unmarshal(configData, &config); err != nil {
		t.Fatalf("Unmarshal(tui.json) error = %v", err)
	}
	found := false
	for _, registered := range config.Plugin {
		if registered == pluginPath {
			found = true
		}
	}
	if !found {
		t.Fatalf("tui.json plugin registrations = %#v, want %q", config.Plugin, pluginPath)
	}
}
