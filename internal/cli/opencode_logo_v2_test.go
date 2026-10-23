package cli

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v4/internal/assets"
	"github.com/gentleman-programming/gentle-ai/v4/internal/model"
	"github.com/gentleman-programming/gentle-ai/v4/internal/opencode"
	"github.com/gentleman-programming/gentle-ai/v4/internal/pipeline"
)

// Issue #5364: gentle-logo ships a pre-built ESM bundle, so on OpenCode 2.x
// both the install and sync pipeline steps must succeed and deliver the
// bundle, the V2 bridge directory, and the cli.json/opencode.jsonc
// registrations instead of skipping or writing raw TSX.
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

	configDir := filepath.Join(home, ".config", "opencode")
	bundlePath := filepath.Join(configDir, "tui-plugins", "gentle-logo.js")
	data, err := os.ReadFile(bundlePath)
	if err != nil {
		t.Fatalf("ReadFile(bundle) error = %v", err)
	}
	content := string(data)
	for _, snippet := range []string{"home.footer", "home_logo", "gentle-logo", "as default"} {
		if !strings.Contains(content, snippet) {
			t.Fatalf("bundled plugin missing snippet %q", snippet)
		}
	}
	embedded, embedErr := assets.Read("tui/gentle-logo.js")
	if embedErr != nil || len(embedded) == 0 || embedded != content {
		t.Fatalf("installed bundle must be the go:embedded artifact (embedErr=%v, equal=%v)", embedErr, content == embedded)
	}

	bridgeTUI := filepath.Join(configDir, "tui-plugins", "gentle-logo", "tui.js")
	bridgeData, err := os.ReadFile(bridgeTUI)
	if err != nil {
		t.Fatalf("ReadFile(bridge tui.js) error = %v", err)
	}
	for _, snippet := range []string{`export * from "../gentle-logo.js"`, `export { default } from "../gentle-logo.js"`} {
		if !strings.Contains(string(bridgeData), snippet) {
			t.Fatalf("bridge tui.js missing re-export %q", snippet)
		}
	}
	manifestData, err := os.ReadFile(filepath.Join(configDir, "tui-plugins", "gentle-logo", "package.json"))
	if err != nil {
		t.Fatalf("ReadFile(bridge package.json) error = %v", err)
	}
	var manifest struct {
		Exports map[string]string `json:"exports"`
	}
	if err := json.Unmarshal(manifestData, &manifest); err != nil {
		t.Fatalf("Unmarshal(bridge package.json) error = %v", err)
	}
	if manifest.Exports["."] != "./tui.js" || manifest.Exports["./tui"] != "./tui.js" {
		t.Fatalf("bridge exports = %#v, want ./tui.js for both \".\" and \"./tui\"", manifest.Exports)
	}

	cliData, err := os.ReadFile(filepath.Join(configDir, "cli.json"))
	if err != nil {
		t.Fatalf("ReadFile(cli.json) error = %v", err)
	}
	var cliConfig struct {
		Schema  string   `json:"$schema"`
		Plugins []string `json:"plugins"`
	}
	if err := json.Unmarshal(cliData, &cliConfig); err != nil {
		t.Fatalf("Unmarshal(cli.json) error = %v", err)
	}
	if cliConfig.Schema != "https://opencode.ai/v2/cli.json" {
		t.Fatalf("cli.json schema = %q, want the V2 schema", cliConfig.Schema)
	}
	if len(cliConfig.Plugins) != 1 || cliConfig.Plugins[0] != "./tui-plugins/gentle-logo" {
		t.Fatalf("cli.json plugins = %#v, want [./tui-plugins/gentle-logo]", cliConfig.Plugins)
	}

	opencodeData, err := os.ReadFile(filepath.Join(configDir, "opencode.jsonc"))
	if err != nil {
		t.Fatalf("ReadFile(opencode.jsonc) error = %v", err)
	}
	var opencodeConfig struct {
		Schema  string   `json:"$schema"`
		Plugins []string `json:"plugins"`
	}
	if err := json.Unmarshal(opencodeData, &opencodeConfig); err != nil {
		t.Fatalf("Unmarshal(opencode.jsonc) error = %v", err)
	}
	if len(opencodeConfig.Plugins) != 1 || opencodeConfig.Plugins[0] != "./tui-plugins/gentle-logo" {
		t.Fatalf("opencode.jsonc plugins = %#v, want the union entry", opencodeConfig.Plugins)
	}

	if _, err := os.Stat(filepath.Join(configDir, "tui.json")); !os.IsNotExist(err) {
		t.Fatalf("V2 install must not create tui.json; stat err = %v", err)
	}
}
