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
