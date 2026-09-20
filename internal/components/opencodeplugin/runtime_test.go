package opencodeplugin

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v3/internal/model"
	"github.com/gentleman-programming/gentle-ai/v3/internal/opencode"
)

func init() {
	opencode.VersionRunnerOverride = func(context.Context, opencode.Command) (opencode.CommandOutput, error) {
		return opencode.CommandOutput{Stdout: []byte("1.18.30")}, nil
	}
}
func TestV2TUIPreservesPlacement(t *testing.T) {
	old := opencode.VersionRunnerOverride
	t.Cleanup(func() { opencode.VersionRunnerOverride = old })
	opencode.VersionRunnerOverride = func(context.Context, opencode.Command) (opencode.CommandOutput, error) {
		return opencode.CommandOutput{Stdout: []byte("2.0.4")}, nil
	}
	for _, id := range []model.OpenCodeCommunityPluginID{model.OpenCodePluginGentleLogo, model.OpenCodePluginSubAgentStatusline, model.OpenCodePluginSDDEngramManage} {
		t.Run(string(id), func(t *testing.T) {
			home := t.TempDir()
			if _, err := Install(home, id); err == nil {
				t.Fatal("unproven TUI silently installed")
			}
			if _, err := os.Stat(filepath.Join(home, ".config")); !os.IsNotExist(err) {
				t.Fatal("TUI refusal wrote files")
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
