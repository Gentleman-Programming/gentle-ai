package opencodeplugin

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v4/internal/model"
	"github.com/gentleman-programming/gentle-ai/v4/internal/opencode"
)

func init() {
	opencode.VersionRunnerOverride = func(context.Context, opencode.Command) (opencode.CommandOutput, error) {
		return opencode.CommandOutput{Stdout: []byte("1.18.30")}, nil
	}
}
func TestV2RuntimeInstallsGentleLogoAndLegacyPluginsStillRefuse(t *testing.T) {
	old := opencode.VersionRunnerOverride
	t.Cleanup(func() { opencode.VersionRunnerOverride = old })
	opencode.VersionRunnerOverride = func(context.Context, opencode.Command) (opencode.CommandOutput, error) {
		return opencode.CommandOutput{Stdout: []byte("2.0.4")}, nil
	}

	t.Run("gentle-logo installs and registers on V2", func(t *testing.T) {
		home := t.TempDir()
		result, err := Install(home, model.OpenCodePluginGentleLogo)
		if err != nil {
			t.Fatalf("Install(gentle-logo) error = %v, want V2 install", err)
		}
		if !result.Changed {
			t.Fatal("Install(gentle-logo) changed = false, want true")
		}
		configDir := filepath.Join(home, ".config", "opencode")
		pluginData, err := os.ReadFile(filepath.Join(configDir, "tui-plugins", "gentle-logo.js"))
		if err != nil {
			t.Fatalf("ReadFile(bundle) error = %v", err)
		}
		content := string(pluginData)
		for _, snippet := range []string{
			"home.footer",
			"home_logo",
			"gentle-logo",
			"as default",
		} {
			if !strings.Contains(content, snippet) {
				t.Fatalf("bundled plugin source missing snippet %q", snippet)
			}
		}
		if _, err := os.Stat(filepath.Join(configDir, "tui-plugins", "gentle-logo", "tui.js")); err != nil {
			t.Fatalf("V2 bridge directory missing: %v", err)
		}
		configData, err := os.ReadFile(filepath.Join(configDir, "cli.json"))
		if err != nil {
			t.Fatalf("ReadFile(cli.json) error = %v", err)
		}
		var config struct {
			Plugins []string `json:"plugins"`
		}
		if err := json.Unmarshal(configData, &config); err != nil {
			t.Fatalf("Unmarshal(cli.json) error = %v", err)
		}
		if len(config.Plugins) != 1 || config.Plugins[0] != "./tui-plugins/gentle-logo" {
			t.Fatalf("cli.json plugins = %#v, want [./tui-plugins/gentle-logo]", config.Plugins)
		}
	})

	for _, id := range []model.OpenCodeCommunityPluginID{model.OpenCodePluginSubAgentStatusline, model.OpenCodePluginSDDEngramManage} {
		t.Run(string(id), func(t *testing.T) {
			home := t.TempDir()
			if _, err := Install(home, id); err == nil {
				t.Fatal("legacy plugin silently installed")
			}
			if _, err := os.Stat(filepath.Join(home, ".config")); !os.IsNotExist(err) {
				t.Fatal("legacy plugin refusal wrote files")
			}
		})
	}
}

func TestLogoUnknownRuntimeStillRefuses(t *testing.T) {
	old := opencode.VersionRunnerOverride
	t.Cleanup(func() { opencode.VersionRunnerOverride = old })
	opencode.VersionRunnerOverride = func(context.Context, opencode.Command) (opencode.CommandOutput, error) {
		return opencode.CommandOutput{Stdout: []byte("unknown")}, nil
	}
	home := t.TempDir()
	_, err := Install(home, model.OpenCodePluginGentleLogo)
	if err == nil {
		t.Fatal("unknown runtime accepted")
	}
	if _, ok := err.(interface{ SkipReason() string }); ok {
		t.Fatal("unknown runtime silently skipped")
	}
	entries, _ := os.ReadDir(home)
	if len(entries) != 0 {
		t.Fatal("unknown runtime wrote files")
	}
}
