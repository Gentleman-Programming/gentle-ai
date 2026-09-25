package opencoderuntimeplugins

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	agent "github.com/gentleman-programming/gentle-ai/v3/internal/agents/opencode"
	"github.com/gentleman-programming/gentle-ai/v3/internal/assets"
	"github.com/gentleman-programming/gentle-ai/v3/internal/opencode"
)

func TestInstallRefusesNonregularPluginBeforeWrites(t *testing.T) {
	home := t.TempDir()
	adapter := agent.NewAdapter()
	dir := filepath.Join(adapter.GlobalConfigDir(home), "plugins")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(home, "user-owned.ts")
	if err := os.WriteFile(target, []byte("user-owned"), 0644); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "skill-registry.ts")
	if err := os.Symlink(target, path); err != nil {
		t.Skipf("symlink unsupported: %v", err)
	}
	result, err := InstallFromDirectory(home, adapter, "opencode/plugins/")
	if err == nil || !strings.Contains(err.Error(), "preserved") || result.Changed {
		t.Fatalf("nonregular plugin accepted: %+v %v", result, err)
	}
	if info, err := os.Lstat(path); err != nil || info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("symlink replaced: %v %v", info, err)
	}
	if _, err := os.Stat(filepath.Join(dir, "model-variants.ts")); !os.IsNotExist(err) {
		t.Fatalf("partial write: %v", err)
	}
}

func TestPluginVersionSelectionAndUserEdits(t *testing.T) {
	old := opencode.VersionRunnerOverride
	t.Cleanup(func() { opencode.VersionRunnerOverride = old })
	for _, tc := range []struct{ version, assetDir string }{{"1.18.30", "opencode/plugins/"}, {"2.0.4", "opencode/plugins-v2/"}, {"unknown", ""}} {
		t.Run(tc.version, func(t *testing.T) {
			opencode.VersionRunnerOverride = func(context.Context, opencode.Command) (opencode.CommandOutput, error) {
				return opencode.CommandOutput{Stdout: []byte(tc.version)}, nil
			}
			home := t.TempDir()
			adapter := agent.NewAdapter()
			result, err := Install(home, adapter)
			if tc.assetDir == "" {
				if err == nil || result.Changed || len(result.Files) != 0 {
					t.Fatalf("unknown runtime installed plugins: %+v %v", result, err)
				}
				if _, err := os.Stat(adapter.GlobalConfigDir(home)); !os.IsNotExist(err) {
					t.Fatalf("unknown runtime created config: %v", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(adapter.GlobalConfigDir(home), "plugins", "skill-registry.ts")
			got, err := os.ReadFile(path)
			if err != nil || string(got) != assets.MustRead(tc.assetDir+"skill-registry.ts") {
				t.Fatalf("wrong plugin version: %v", err)
			}
			if tc.version == "2.0.4" {
				if err := os.WriteFile(path, []byte("user-owned"), 0600); err != nil {
					t.Fatal(err)
				}
				if _, err := Refresh(home, adapter); err == nil || !strings.Contains(err.Error(), "preserved") {
					t.Fatalf("custom V2 plugin overwritten: %v", err)
				}
				if got, err := os.ReadFile(path); err != nil || string(got) != "user-owned" {
					t.Fatalf("custom bytes lost: %v", err)
				}
			}
		})
	}
}

func TestRefreshSkipsNonregularPaths(t *testing.T) {
	old := opencode.VersionRunnerOverride
	t.Cleanup(func() { opencode.VersionRunnerOverride = old })
	opencode.VersionRunnerOverride = func(context.Context, opencode.Command) (opencode.CommandOutput, error) {
		return opencode.CommandOutput{Stdout: []byte("1.18.30")}, nil
	}
	home := t.TempDir()
	adapter := agent.NewAdapter()
	dir := filepath.Join(adapter.GlobalConfigDir(home), "plugins")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(home, "user-owned.ts")
	if err := os.WriteFile(target, []byte("user-owned"), 0644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "opencode-review-transport.ts")
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlink unsupported: %v", err)
	}
	if err := os.Mkdir(filepath.Join(dir, "model-variants.ts"), 0755); err != nil {
		t.Fatal(err)
	}
	stale := filepath.Join(dir, "skill-registry.ts")
	if err := os.WriteFile(stale, []byte("stale"), 0644); err != nil {
		t.Fatal(err)
	}
	result, err := Refresh(home, adapter)
	if err != nil || !result.Changed {
		t.Fatalf("refresh: %+v %v", result, err)
	}
	if info, err := os.Lstat(link); err != nil || info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("symlink replaced: %v %v", info, err)
	}
	if got, err := os.ReadFile(target); err != nil || string(got) != "user-owned" {
		t.Fatalf("target changed: %v", err)
	}
	if info, err := os.Stat(filepath.Join(dir, "model-variants.ts")); err != nil || !info.IsDir() {
		t.Fatalf("directory replaced: %v %v", info, err)
	}
	if got, err := os.ReadFile(stale); err != nil || string(got) != assets.MustRead("opencode/plugins/skill-registry.ts") {
		t.Fatalf("regular plugin not refreshed: %v", err)
	}
}

func TestManagedOpenCodePluginLifecycle(t *testing.T) {
	old := opencode.VersionRunnerOverride
	t.Cleanup(func() { opencode.VersionRunnerOverride = old })
	opencode.VersionRunnerOverride = func(context.Context, opencode.Command) (opencode.CommandOutput, error) {
		return opencode.CommandOutput{Stdout: []byte("2.0.4")}, nil
	}
	home := t.TempDir()
	adapter := agent.NewAdapter()
	result, err := Install(home, adapter)
	if err != nil || !result.Changed {
		t.Fatalf("install: %+v %v", result, err)
	}
	dir := filepath.Join(adapter.GlobalConfigDir(home), "plugins")
	for _, name := range ManagedPluginNames(adapter.Agent()) {
		got, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil || string(got) != assets.MustRead("opencode/plugins-v2/"+name) {
			t.Fatalf("plugin %s: %v", name, err)
		}
	}
	again, err := Install(home, adapter)
	if err != nil || again.Changed {
		t.Fatalf("idempotent install: %+v %v", again, err)
	}
}
