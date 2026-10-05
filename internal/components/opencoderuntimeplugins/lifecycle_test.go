package opencoderuntimeplugins

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	agent "github.com/gentleman-programming/gentle-ai/v4/internal/agents/opencode"
	"github.com/gentleman-programming/gentle-ai/v4/internal/assets"
	"github.com/gentleman-programming/gentle-ai/v4/internal/opencode"
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

func TestInstallRefusesNonregularRetiredPluginBeforeWrites(t *testing.T) {
	home := t.TempDir()
	adapter := agent.NewAdapter()
	dir := filepath.Join(adapter.GlobalConfigDir(home), "plugins")
	retired := filepath.Join(dir, "background-agents.ts")
	if err := os.MkdirAll(retired, 0755); err != nil {
		t.Fatal(err)
	}
	result, err := InstallFromDirectory(home, adapter, "opencode/plugins/")
	if err == nil || !strings.Contains(err.Error(), retired) || result.Changed {
		t.Fatalf("nonregular retired plugin accepted: %+v %v", result, err)
	}
	if info, err := os.Stat(retired); err != nil || !info.IsDir() {
		t.Fatalf("directory replaced: %v %v", info, err)
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
				if _, err := Install(home, adapter); err == nil || !strings.Contains(err.Error(), "preserved") {
					t.Fatalf("custom V2 plugin overwritten: %v", err)
				}
				if got, err := os.ReadFile(path); err != nil || string(got) != "user-owned" {
					t.Fatalf("custom bytes lost: %v", err)
				}
			}
		})
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

func useRuntime(t *testing.T, version string) {
	t.Helper()
	old := opencode.VersionRunnerOverride
	t.Cleanup(func() { opencode.VersionRunnerOverride = old })
	opencode.VersionRunnerOverride = func(context.Context, opencode.Command) (opencode.CommandOutput, error) {
		return opencode.CommandOutput{Stdout: []byte(version)}, nil
	}
}

// releasedFixture returns plugin bytes exactly as a published release tag
// shipped them (testdata/released/<tag>/<asset dir>/<name>).
func releasedFixture(t *testing.T, rel string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", "released", filepath.FromSlash(rel)))
	if err != nil {
		t.Fatal(err)
	}
	return data
}

var runtimeAssetDirs = map[string]string{"1.18.30": "opencode/plugins/", "2.0.4": "opencode/plugins-v2/"}

// Issue #5185: bytes a previous release shipped are Gentle AI-owned on both
// runtime majors, whichever asset directory they came from.
func TestInstallUpgradesPreviouslyReleasedPluginBytes(t *testing.T) {
	for version, assetDir := range runtimeAssetDirs {
		for _, fixture := range []string{"v3.7.0/plugins/opencode-review-transport.ts", "v3.7.0/plugins-v2/opencode-review-transport.ts"} {
			t.Run(version+"/"+fixture, func(t *testing.T) {
				useRuntime(t, version)
				home := t.TempDir()
				adapter := agent.NewAdapter()
				dir := filepath.Join(adapter.GlobalConfigDir(home), "plugins")
				if err := os.MkdirAll(dir, 0755); err != nil {
					t.Fatal(err)
				}
				for name, data := range map[string][]byte{
					"opencode-review-transport.ts": releasedFixture(t, fixture),
					"background-agents.ts":         releasedFixture(t, "v1.7.19/plugins/background-agents.ts"),
					LegacyOpenCodeReviewPluginName: releasedFixture(t, "v2.1.7/plugins/review-result-artifacts.ts"),
				} {
					if err := os.WriteFile(filepath.Join(dir, name), data, 0644); err != nil {
						t.Fatal(err)
					}
				}
				result, err := Install(home, adapter)
				if err != nil || !result.Changed {
					t.Fatalf("upgrade refused released bytes: %+v %v", result, err)
				}
				for _, name := range ManagedPluginNames(adapter.Agent()) {
					if got, err := os.ReadFile(filepath.Join(dir, name)); err != nil || string(got) != assets.MustRead(assetDir+name) {
						t.Fatalf("plugin %s not upgraded: %v", name, err)
					}
				}
				for _, name := range []string{"background-agents.ts", LegacyOpenCodeReviewPluginName} {
					if _, err := os.Lstat(filepath.Join(dir, name)); !os.IsNotExist(err) {
						t.Fatalf("retired plugin %s kept: %v", name, err)
					}
				}
			})
		}
	}
}

// Bytes no release shipped stay untouched on both runtime majors, and the
// refusal names the file and its recovery before any plugin is written.
func TestInstallPreservesUserEditedPluginBytes(t *testing.T) {
	for version := range runtimeAssetDirs {
		for _, name := range []string{"skill-registry.ts", "background-agents.ts", LegacyOpenCodeReviewPluginName} {
			t.Run(version+"/"+name, func(t *testing.T) {
				useRuntime(t, version)
				home := t.TempDir()
				adapter := agent.NewAdapter()
				dir := filepath.Join(adapter.GlobalConfigDir(home), "plugins")
				if err := os.MkdirAll(dir, 0755); err != nil {
					t.Fatal(err)
				}
				path := filepath.Join(dir, name)
				edited := append(releasedFixture(t, "v3.7.0/plugins/opencode-review-transport.ts"), "// user edit\n"...)
				if err := os.WriteFile(path, edited, 0600); err != nil {
					t.Fatal(err)
				}
				result, err := Install(home, adapter)
				if err == nil || result.Changed || !strings.Contains(err.Error(), path) || !strings.Contains(err.Error(), "move or delete") {
					t.Fatalf("user-edited plugin not refused actionably: %+v %v", result, err)
				}
				if got, err := os.ReadFile(path); err != nil || string(got) != string(edited) {
					t.Fatalf("user bytes lost: %v", err)
				}
				entries, err := os.ReadDir(dir)
				if err != nil || len(entries) != 1 {
					t.Fatalf("refusal wrote plugins: %v %v", entries, err)
				}
			})
		}
	}
}
