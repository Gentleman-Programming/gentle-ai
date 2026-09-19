package sdd

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	agent "github.com/gentleman-programming/gentle-ai/v2/internal/agents/opencode"
	"github.com/gentleman-programming/gentle-ai/v2/internal/assets"
	"github.com/gentleman-programming/gentle-ai/v2/internal/opencode"
)

// Existing config tests model V1, never an ambient executable.
func init() {
	opencode.VersionRunnerOverride = func(context.Context, opencode.Command) (opencode.CommandOutput, error) {
		return opencode.CommandOutput{Stdout: []byte("1.18.30")}, nil
	}
}
func TestOpenCodePluginMajorSelection(t *testing.T) {
	old := opencode.VersionRunnerOverride
	t.Cleanup(func() { opencode.VersionRunnerOverride = old })
	for _, version := range []string{"2.0.4", "unknown"} {
		t.Run(version, func(t *testing.T) {
			opencode.VersionRunnerOverride = func(context.Context, opencode.Command) (opencode.CommandOutput, error) {
				return opencode.CommandOutput{Stdout: []byte(version)}, nil
			}
			home := t.TempDir()
			a := agent.NewAdapter()
			assetDir, err := openCodePluginAssetDirectory(a.Agent())
			var result InjectionResult
			if err == nil {
				result, err = installOpenCodePluginsDirectory(home, a, assetDir)
			}
			if version == "unknown" {
				if err == nil || len(result.Files) != 0 {
					t.Fatal("unknown runtime mutated plugins")
				}
				if _, err := os.Stat(a.GlobalConfigDir(home)); !os.IsNotExist(err) {
					t.Fatal("unknown runtime created directory")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			p := filepath.Join(a.GlobalConfigDir(home), "plugins", "skill-registry.ts")
			b, _ := os.ReadFile(p)
			if string(b) != assets.MustRead("opencode/plugins-v2/skill-registry.ts") {
				t.Fatal("V2 selected legacy plugin")
			}
			os.WriteFile(p, []byte("user-owned"), 0600)
			if _, err := RefreshInstalledOpenCodePlugins(home, a); err == nil || !strings.Contains(err.Error(), "preserved") {
				t.Fatal("custom V2 plugin overwritten")
			}
			b, _ = os.ReadFile(p)
			if string(b) != "user-owned" {
				t.Fatal("custom bytes lost")
			}
		})
	}
}
