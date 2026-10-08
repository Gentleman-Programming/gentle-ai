package cli

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v4/internal/model"
	"github.com/gentleman-programming/gentle-ai/v4/internal/opencode"
	"github.com/gentleman-programming/gentle-ai/v4/internal/pipeline"
)

// Issue #5364: gentle-logo ships a V2-shaped module, so on OpenCode 2.x both
// the install and sync pipeline steps must succeed and deliver the managed
// logo files instead of skipping with an omission reason.
func TestOpenCodeV2LogoInstallsAndSyncsWithoutSkipping(t *testing.T) {
	old := opencode.VersionRunnerOverride
	t.Cleanup(func() { opencode.VersionRunnerOverride = old })
	opencode.VersionRunnerOverride = func(context.Context, opencode.Command) (opencode.CommandOutput, error) {
		return opencode.CommandOutput{Stdout: []byte("2.0.4")}, nil
	}
	home := t.TempDir()
	for _, step := range []pipeline.Step{
		componentApplyStep{id: "logo", component: model.ComponentOpenCodeGentleLogo, homeDir: home, agents: []model.AgentID{model.AgentOpenCode}},
		componentSyncStep{id: "sync-logo", component: model.ComponentOpenCodeGentleLogo, homeDir: home, agents: []model.AgentID{model.AgentOpenCode}, changedFiles: &[]string{}},
	} {
		result := (pipeline.Runner{}).Run(pipeline.StageApply, []pipeline.Step{step})
		if !result.Success || result.Err != nil || len(result.Steps) != 1 || result.Steps[0].Status != pipeline.StepStatusSucceeded {
			t.Fatalf("%s: %#v", step.ID(), result)
		}
		if result.Steps[0].Err != nil {
			t.Fatalf("%s: step error = %v", step.ID(), result.Steps[0].Err)
		}
	}

	pluginPath := filepath.Join(home, ".config", "opencode", "tui-plugins", "gentle-logo.tsx")
	data, err := os.ReadFile(pluginPath)
	if err != nil {
		t.Fatalf("ReadFile(plugin) error = %v", err)
	}
	content := string(data)
	for _, snippet := range []string{
		`Plugin.define({ id, setup, tui })`,
		`export default`,
		`home_logo`,
		`typeof api.slots.register === "function"`,
	} {
		if !strings.Contains(content, snippet) {
			t.Fatalf("V2 plugin source missing snippet %q", snippet)
		}
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
	if len(config.Plugin) != 1 || config.Plugin[0] != pluginPath {
		t.Fatalf("tui.json plugin registrations = %#v, want [%q]", config.Plugin, pluginPath)
	}
}
