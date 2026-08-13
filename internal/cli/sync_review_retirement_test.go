package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/backup"
	"github.com/gentleman-programming/gentle-ai/v2/internal/components/sdd"
	"github.com/gentleman-programming/gentle-ai/v2/internal/model"
	"github.com/gentleman-programming/gentle-ai/v2/internal/pipeline"
	"github.com/gentleman-programming/gentle-ai/v2/internal/state"
)

// Retirement of the managed review plugin (change #3138, slice 6, task
// 6.3/6.4). The SDD half of review-result-artifacts.ts is native Go
// (sdd_task_result.go), the review half is the reviewer-shim.ts glue over
// native verbs, and the managed plugin itself is retired: re-sync must
// REMOVE a stale installed copy from OpenCode dirs (sources must never
// double-inject: a stale review-result-artifacts.ts next to reviewer-shim.ts
// would intercept every hook twice, violating the single injection source,
// SEN-RPC-15/16), and Kilo dirs must be scrubbed of BOTH retired files.
//
// These tests are RED before the 6.4 removal lands: today
// RefreshInstalledOpenCodePlugins refreshes review-result-artifacts.ts from
// the embedded asset (so a stale copy is refreshed, not removed) and
// removeOpenCodeOnlyReviewPlugin never touches reviewer-shim.ts at all.
// The rollback test is the guard that survives the change: removal must stay
// inside the backup/restore contract (REQ-RPC-10), so a failing sync never
// leaves a partially scrubbed install.

// TestRunSyncRemovesRetiredReviewPluginFromOpenCode pins SEN-RPC-18: once
// the review plugin is retired, a re-synced OpenCode user must not retain
// the stale file -- the runtime must not keep intercepting through a plugin
// the binary no longer manages (single injection source).
func TestRunSyncRemovesRetiredReviewPluginFromOpenCode(t *testing.T) {
	home := t.TempDir()
	if err := state.Write(home, state.InstallState{
		InstalledAgents:     []string{"opencode"},
		SelectionConfigured: true,
		Components:          []model.ComponentID{model.ComponentEngram},
		Persona:             "neutral",
	}); err != nil {
		t.Fatalf("state.Write() error = %v", err)
	}

	pluginsDir := filepath.Join(home, ".config", "opencode", "plugins")
	retired := filepath.Join(pluginsDir, "review-result-artifacts.ts")
	mustWriteFile(t, retired, []byte("// stale pre-slice-6 managed review plugin"))

	restoreHome := osUserHomeDir
	restoreBackupHome := backup.UserHomeDirFn
	osUserHomeDir = func() (string, error) { return home, nil }
	backup.UserHomeDirFn = func() (string, error) { return home, nil }
	t.Cleanup(func() {
		osUserHomeDir = restoreHome
		backup.UserHomeDirFn = restoreBackupHome
	})

	if _, err := RunSync(nil); err != nil {
		t.Fatalf("RunSync() error = %v", err)
	}

	if _, statErr := os.Stat(retired); !os.IsNotExist(statErr) {
		t.Fatalf("sync left retired review plugin installed for OpenCode: %v", statErr)
	}
}

// TestRunSyncScrubsReviewerShimFromKilocode pins decision (a) of slice 6:
// reviewer-shim.ts is OpenCode-only, so a Kilo config dir that received it
// (for example from a previous selection) must be scrubbed of it on re-sync.
func TestRunSyncScrubsReviewerShimFromKilocode(t *testing.T) {
	home := t.TempDir()
	if err := state.Write(home, state.InstallState{
		InstalledAgents:     []string{"kilocode"},
		SelectionConfigured: true,
		Components:          []model.ComponentID{model.ComponentEngram},
		Persona:             "neutral",
	}); err != nil {
		t.Fatalf("state.Write() error = %v", err)
	}

	pluginsDir := filepath.Join(home, ".config", "kilo", "plugins")
	shim := filepath.Join(pluginsDir, "reviewer-shim.ts")
	mustWriteFile(t, shim, []byte("// stale OpenCode-only reviewer shim"))

	restoreHome := osUserHomeDir
	restoreBackupHome := backup.UserHomeDirFn
	osUserHomeDir = func() (string, error) { return home, nil }
	backup.UserHomeDirFn = func() (string, error) { return home, nil }
	t.Cleanup(func() {
		osUserHomeDir = restoreHome
		backup.UserHomeDirFn = restoreBackupHome
	})

	if _, err := RunSync(nil); err != nil {
		t.Fatalf("RunSync() error = %v", err)
	}

	if _, statErr := os.Stat(shim); !os.IsNotExist(statErr) {
		t.Fatalf("sync left OpenCode-only reviewer shim installed for Kilo: %v", statErr)
	}
}

// TestSyncRollbackRestoresRemovedRetiredReviewPlugins is the rollback guard
// for the retirement: the removal step does not exist yet, so the sync
// transaction leaves both stale files untouched and the assertions below
// pass trivially. When 6.4 adds removal, the removed files MUST still be
// restored by the transaction's rollback -- the backup/snapshot contract
// that covers them today (syncBackupTargets over ManagedOpenCodePluginNames)
// must keep covering the retired candidates (REQ-RPC-10).
func TestSyncRollbackRestoresRemovedRetiredReviewPlugins(t *testing.T) {
	home := t.TempDir()
	openCodeRetired := filepath.Join(home, ".config", "opencode", "plugins", "review-result-artifacts.ts")
	kiloShim := filepath.Join(home, ".config", "kilo", "plugins", "reviewer-shim.ts")
	type stale struct {
		path string
		data []byte
	}
	staleBytes := []byte("// stale pre-slice-6 managed plugin")
	before := []stale{
		{openCodeRetired, staleBytes},
		{kiloShim, staleBytes},
	}
	for _, file := range before {
		mustWriteFile(t, file.path, file.data)
	}

	selection := model.Selection{
		Agents:     []model.AgentID{model.AgentOpenCode, model.AgentKilocode},
		Components: []model.ComponentID{model.ComponentEngram},
	}
	rt, err := newSyncRuntime(home, selection)
	if err != nil {
		t.Fatal(err)
	}
	plan := rt.stagePlan()
	plan.Apply = append(plan.Apply, failingSyncStep{})
	result := pipeline.NewOrchestrator(pipeline.DefaultRollbackPolicy()).Execute(plan)
	if result.Err == nil {
		t.Fatal("injected failure did not trigger sync rollback")
	}
	if !result.Rollback.Success {
		t.Fatalf("sync rollback failed: error=%v steps=%#v", result.Rollback.Err, result.Rollback.Steps)
	}
	for _, file := range before {
		got, readErr := os.ReadFile(file.path)
		if readErr != nil || !bytes.Equal(got, file.data) {
			t.Errorf("rollback did not restore %q: got=%q error=%v", file.path, got, readErr)
		}
	}
}

// TestRetiredOpenCodePluginBackupTargetsGuardsRollbackKeep pins the backup
// contract that makes the rollback above possible: every plugin-receiving
// agent dir must back up the retired review plugin candidates, so a failed
// sync can restore a removed stale copy byte-for-byte.
func TestRetiredOpenCodePluginBackupTargetsGuardRollback(t *testing.T) {
	home := t.TempDir()
	sel := model.Selection{
		Agents:     []model.AgentID{model.AgentOpenCode, model.AgentKilocode},
		Components: []model.ComponentID{model.ComponentEngram},
	}

	targets, err := syncBackupTargets(home, "", sel, resolveAdapters(sel.Agents))
	if err != nil {
		t.Fatalf("syncBackupTargets() error = %v", err)
	}

	for _, configDir := range []string{"opencode", "kilo"} {
		for _, plugin := range sdd.RetiredOpenCodePluginNames() {
			want := filepath.Join(home, ".config", configDir, "plugins", plugin)
			if !containsPath(targets, want) {
				t.Errorf("syncBackupTargets missing retired plugin backup path %q\ntargets = %v", want, targets)
			}
		}
	}
}
