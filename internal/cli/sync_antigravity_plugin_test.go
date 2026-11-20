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

// TestRunSyncAntigravityReportsOnlyWrittenFiles pins that a full sync
// lists Antigravity global MCP config as changed only when written.
func TestRunSyncAntigravityReportsOnlyWrittenFiles(t *testing.T) {
	home := setupAntigravitySyncHome(t)
	args := []string{"--agents", "antigravity"}
	global := filepath.Join(home, ".gemini", "config", "mcp_config.json")

	first, err := RunSync(args)
	if err != nil {
		t.Fatalf("RunSync() first error = %v", err)
	}
	if !containsPath(first.ChangedFiles, global) {
		t.Fatalf("first sync ChangedFiles = %v, want written global MCP config %q", first.ChangedFiles, global)
	}

	second, err := RunSync(args)
	if err != nil {
		t.Fatalf("RunSync() second error = %v", err)
	}
	if containsPath(second.ChangedFiles, global) {
		t.Fatalf("second sync ChangedFiles = %v, must not list unchanged global MCP config %q", second.ChangedFiles, global)
	}

	// Rewriting the file causes it to be reported as changed again.
	if err := os.WriteFile(global, []byte("{}\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(global) error = %v", err)
	}
	third, err := RunSync(args)
	if err != nil {
		t.Fatalf("RunSync() third error = %v", err)
	}
	if !containsPath(third.ChangedFiles, global) {
		t.Fatalf("third sync ChangedFiles = %v, want rewritten %q", third.ChangedFiles, global)
	}
}

// TestRunSyncAntigravityRetiresLegacyPlugin retires legacy plugin assets
// left behind by older gentle-ai versions (#5988).
func TestRunSyncAntigravityRetiresLegacyPlugin(t *testing.T) {
	home := setupAntigravitySyncHome(t)
	legacyDir := engram.AntigravityLegacyPluginDir(home)
	if err := engram.WriteManagedAntigravityPluginForTest(legacyDir, "engram"); err != nil {
		t.Fatalf("WriteManagedAntigravityPluginForTest() error = %v", err)
	}
	legacy := engram.AntigravityLegacyPluginAssetPaths(home)

	args := []string{"--agents", "antigravity"}
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
}

// TestRunInstallAntigravityEngramOnlyWithoutGlobalMCPConfig covers an
// engram-only install on a host with no preexisting Antigravity mcp_config.json.
// Engram is registered natively in the global mcp_config.json (#5988).
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

	adapter, err := agents.NewAdapter(model.AgentAntigravity)
	if err != nil {
		t.Fatalf("NewAdapter(antigravity) error = %v", err)
	}
	global := adapter.MCPConfigPath(home, "engram")
	if _, err := os.Stat(global); err != nil {
		t.Fatalf("global Antigravity MCP config %q must be written by Engram; stat err = %v", global, err)
	}

	for _, path := range engram.AntigravityPluginAssetPaths(home) {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("plugin asset %q must not exist after global install: stat err = %v", path, err)
		}
	}

	adapters := []agents.Adapter{adapter}
	selection := model.Selection{Agents: []model.AgentID{model.AgentAntigravity}, Components: []model.ComponentID{model.ComponentEngram}}
	if !containsPath(componentPathsWithWorkspaceScoped(home, "", ScopeGlobal, selection, adapters, model.ComponentEngram), global) {
		t.Fatalf("global Antigravity MCP config %q must be a component path", global)
	}
	if !containsPath(verificationComponentPaths(home, "", ScopeGlobal, selection, adapters, model.ComponentEngram), global) {
		t.Fatalf("global Antigravity MCP config %q must be a verification target", global)
	}
}
