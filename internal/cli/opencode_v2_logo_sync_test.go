package cli

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v4/internal/model"
	opencodeactivation "github.com/gentleman-programming/gentle-ai/v4/internal/opencode"
	"github.com/gentleman-programming/gentle-ai/v4/internal/pipeline"
)

// Issue #4940: OpenCode 2.x omits the Gentle logo by design; install and sync
// must not then fail verification by expecting the logo files never written.
func TestOpenCodeV2SyncWithGentleLogoSkipsItsVerification(t *testing.T) {
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
	if _, err := os.Lstat(filepath.Join(home, ".config", "opencode", "tui-plugins", "gentle-logo.tsx")); !os.IsNotExist(err) {
		t.Fatalf("OpenCode 2.x received the logo plugin: %v", err)
	}
}
