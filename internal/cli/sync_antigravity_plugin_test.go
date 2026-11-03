package cli

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v4/internal/agents"
	"github.com/gentleman-programming/gentle-ai/v4/internal/backup"
	"github.com/gentleman-programming/gentle-ai/v4/internal/components/engram"
	"github.com/gentleman-programming/gentle-ai/v4/internal/model"
)

func setupAntigravitySyncHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	restoreHome := osUserHomeDir
	restoreBackupHome := backup.UserHomeDirFn
	restoreCommand := runCommand
	restoreLookPath := cmdLookPath
	t.Cleanup(func() {
		osUserHomeDir = restoreHome
		backup.UserHomeDirFn = restoreBackupHome
		runCommand = restoreCommand
		cmdLookPath = restoreLookPath
	})
	osUserHomeDir = func() (string, error) { return home, nil }
	backup.UserHomeDirFn = func() (string, error) { return home, nil }
	runCommand = func(string, ...string) error { return nil }
	cmdLookPath = func(name string) (string, error) { return "/usr/bin/" + name, nil }
	engram.SetLookPathForTest(t, "/opt/homebrew/bin/engram", "")
	return home
}

// TestRunSyncAntigravityReportsOnlyWrittenPluginFiles pins #5614: a full sync
// listed the three Engram plugin files as changed even when they were current.
func TestRunSyncAntigravityReportsOnlyWrittenPluginFiles(t *testing.T) {
	home := setupAntigravitySyncHome(t)
	args := []string{"--agents", "antigravity"}
	plugin := engram.AntigravityPluginAssetPaths(home)

	first, err := RunSync(args)
	if err != nil {
		t.Fatalf("RunSync() first error = %v", err)
	}
	for _, path := range plugin {
		if !containsPath(first.ChangedFiles, path) {
			t.Fatalf("first sync ChangedFiles = %v, want written plugin asset %q", first.ChangedFiles, path)
		}
	}

	second, err := RunSync(args)
	if err != nil {
		t.Fatalf("RunSync() second error = %v", err)
	}
	for _, path := range plugin {
		if containsPath(second.ChangedFiles, path) {
			t.Fatalf("second sync ChangedFiles = %v, must not list unchanged plugin asset %q", second.ChangedFiles, path)
		}
	}

	// A really changed asset is still reported.
	if err := os.WriteFile(plugin[2], []byte("{}\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(hooks) error = %v", err)
	}
	third, err := RunSync(args)
	if err != nil {
		t.Fatalf("RunSync() third error = %v", err)
	}
	if !containsPath(third.ChangedFiles, plugin[2]) || containsPath(third.ChangedFiles, plugin[0]) {
		t.Fatalf("third sync ChangedFiles = %v, want only the rewritten %q among plugin assets", third.ChangedFiles, plugin[2])
	}
}

// TestRunSyncAntigravityMigratesLegacyPlugin moves the plugin an older
// gentle-ai wrote under antigravity-cli/plugins to the shared config root
// (#5930) and reports the retired legacy assets.
func TestRunSyncAntigravityMigratesLegacyPlugin(t *testing.T) {
	home := setupAntigravitySyncHome(t)
	args := []string{"--agents", "antigravity"}
	if _, err := RunSync(args); err != nil {
		t.Fatalf("RunSync() seed error = %v", err)
	}
	legacy := engram.AntigravityLegacyPluginAssetPaths(home)
	for i, path := range engram.AntigravityPluginAssetPaths(home) {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("ReadFile(%q) error = %v", path, err)
		}
		if err := os.MkdirAll(filepath.Dir(legacy[i]), 0o755); err != nil {
			t.Fatalf("MkdirAll(legacy) error = %v", err)
		}
		if err := os.WriteFile(legacy[i], raw, 0o644); err != nil {
			t.Fatalf("WriteFile(%q) error = %v", legacy[i], err)
		}
	}

	result, err := RunSync(args)
	if err != nil {
		t.Fatalf("RunSync() error = %v", err)
	}
	for _, path := range legacy {
		if _, statErr := os.Lstat(path); !os.IsNotExist(statErr) {
			t.Fatalf("legacy plugin asset %q must be retired; lstat err = %v", path, statErr)
		}
		if !containsPath(result.ChangedFiles, path) {
			t.Fatalf("ChangedFiles = %v, want retired legacy asset %q", result.ChangedFiles, path)
		}
	}
	for _, path := range engram.AntigravityPluginAssetPaths(home) {
		if containsPath(result.ChangedFiles, path) {
			t.Fatalf("ChangedFiles = %v, must not list unchanged plugin asset %q", result.ChangedFiles, path)
		}
	}
}

// TestRunInstallAntigravityEngramOnlyWithoutGlobalMCPConfig covers an
// engram-only install on a host with no global Antigravity mcp_config.json.
// Engram's registration is plugin-owned, so verification must not require the
// global config Engram never writes, and the plugin must survive.
func TestRunInstallAntigravityEngramOnlyWithoutGlobalMCPConfig(t *testing.T) {
	home := setupAntigravitySyncHome(t)
	restoreVersion := verifyEngramVersionCommand
	restoreProbe := probeEngramProtocolFlagCommand
	t.Cleanup(func() {
		verifyEngramVersionCommand = restoreVersion
		probeEngramProtocolFlagCommand = restoreProbe
	})
	verifyEngramVersionCommand = func(string) (string, error) { return "engram 1.20.0", nil }
	probeEngramProtocolFlagCommand = func(context.Context, string) (string, error) {
		return "Usage: engram setup <slug> --protocol=full", nil
	}

	if _, err := RunInstall([]string{"--agent", "antigravity", "--component", "engram"}, macOSDetectionResult()); err != nil {
		t.Fatalf("RunInstall(antigravity, engram) error = %v", err)
	}

	for _, path := range engram.AntigravityPluginAssetPaths(home) {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("plugin asset %q must survive the install: %v", path, err)
		}
	}
	adapter, err := agents.NewAdapter(model.AgentAntigravity)
	if err != nil {
		t.Fatalf("NewAdapter(antigravity) error = %v", err)
	}
	global := adapter.MCPConfigPath(home, "engram")
	if _, err := os.Stat(global); !os.IsNotExist(err) {
		t.Fatalf("global Antigravity MCP config %q must not be written by Engram; stat err = %v", global, err)
	}
	// Engram may still remove a managed duplicate from it, so it stays a
	// backup target while verification no longer requires it.
	adapters := []agents.Adapter{adapter}
	selection := model.Selection{Agents: []model.AgentID{model.AgentAntigravity}, Components: []model.ComponentID{model.ComponentEngram}}
	if !containsPath(componentPathsWithWorkspaceScoped(home, "", ScopeGlobal, selection, adapters, model.ComponentEngram), global) {
		t.Fatalf("global Antigravity MCP config %q must stay a backup target", global)
	}
	if containsPath(verificationComponentPaths(home, "", ScopeGlobal, selection, adapters, model.ComponentEngram), global) {
		t.Fatalf("global Antigravity MCP config %q must not be a verification target", global)
	}
}
