package app

import (
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v3/internal/model"
	"github.com/gentleman-programming/gentle-ai/v3/internal/reviewtransaction"
	"github.com/gentleman-programming/gentle-ai/v3/internal/state"
	"github.com/gentleman-programming/gentle-ai/v3/internal/statecoord"
)

// holdCanonicalInstallStateLock acquires the real canonical install-state lock
// for home so a subsequent write site fails fast on contention, mirroring the
// contention shape used by internal/cli/install_state_lock_test.go.
func holdCanonicalInstallStateLock(t *testing.T, home string) *reviewtransaction.AuthorityFileLock {
	t.Helper()
	lockPath, err := statecoord.LockPath(home)
	if err != nil {
		t.Fatalf("install state lock path: %v", err)
	}
	held, err := reviewtransaction.AcquireAuthorityFileLock(lockPath)
	if err != nil {
		t.Fatalf("acquire install state lock for contention: %v", err)
	}
	return held
}

// TestClearPendingSyncAfterDeferredSyncReReadsLatestStateUnderLock proves the
// deferred-sync PendingSync clear (site 1, #1809) no longer clobbers a
// concurrent writer: the read happens inside the canonical lock, so a field
// written by another writer after the deferred sync survives the clear.
func TestClearPendingSyncAfterDeferredSyncReReadsLatestStateUnderLock(t *testing.T) {
	home := t.TempDir()
	held := holdCanonicalInstallStateLock(t, home)
	if err := clearPendingSyncAfterDeferredSync(home, state.InstallState{PendingSync: true}); err == nil || !strings.Contains(err.Error(), "acquire install state lock") {
		t.Fatalf("contended deferred-sync clear error = %v, want lock acquisition failure", err)
	}
	if err := state.Write(home, state.InstallState{PendingSync: true, RDDMode: "on", Persona: "concurrent"}); err != nil {
		t.Fatal(err)
	}
	if err := held.Release(); err != nil {
		t.Fatal(err)
	}

	if err := clearPendingSyncAfterDeferredSync(home, state.InstallState{PendingSync: true}); err != nil {
		t.Fatalf("clearPendingSyncAfterDeferredSync() error = %v", err)
	}
	got, err := state.Read(home)
	if err != nil {
		t.Fatal(err)
	}
	if got.PendingSync {
		t.Errorf("PendingSync = true, want false after deferred sync")
	}
	if got.RDDMode != "on" || got.Persona != "concurrent" {
		t.Fatalf("concurrent writer state was clobbered: %#v", got)
	}
}

// TestPersistAssignmentsReReadsLatestStateUnderLock proves persistAssignments
// (site 2, #1809) applies the assignments on top of the latest state inside
// the canonical lock instead of a stale pre-lock read, so a concurrent
// writer's changes survive.
func TestPersistAssignmentsReReadsLatestStateUnderLock(t *testing.T) {
	home := t.TempDir()
	held := holdCanonicalInstallStateLock(t, home)
	selection := model.Selection{
		CodexCarrilModelAssignments: map[string]string{"sdd-strong": "test-model"},
	}
	if err := persistAssignments(home, selection); err == nil || !strings.Contains(err.Error(), "acquire install state lock") {
		t.Fatalf("contended persistAssignments error = %v, want lock acquisition failure", err)
	}
	if err := state.Write(home, state.InstallState{RDDMode: "on", Persona: "concurrent"}); err != nil {
		t.Fatal(err)
	}
	if err := held.Release(); err != nil {
		t.Fatal(err)
	}

	if err := persistAssignments(home, selection); err != nil {
		t.Fatalf("persistAssignments() error = %v", err)
	}
	got, err := state.Read(home)
	if err != nil {
		t.Fatal(err)
	}
	if got.CodexCarrilModelAssignments["sdd-strong"] != "test-model" {
		t.Fatalf("assignments were not persisted: %#v", got.CodexCarrilModelAssignments)
	}
	if got.RDDMode != "on" || got.Persona != "concurrent" {
		t.Fatalf("concurrent writer state was clobbered: %#v", got)
	}
}

// TestMarkPendingSyncAfterSelfUpdateReReadsLatestStateUnderLock proves the
// self-update PendingSync set (site 3, #1809) re-reads the latest state inside
// the canonical lock instead of writing a stale pre-lock read, so a
// concurrent writer's changes survive.
func TestMarkPendingSyncAfterSelfUpdateReReadsLatestStateUnderLock(t *testing.T) {
	home := t.TempDir()
	held := holdCanonicalInstallStateLock(t, home)
	if err := markPendingSyncAfterSelfUpdate(home); err == nil || !strings.Contains(err.Error(), "acquire install state lock") {
		t.Fatalf("contended self-update PendingSync set error = %v, want lock acquisition failure", err)
	}
	if err := state.Write(home, state.InstallState{RDDMode: "on", Persona: "concurrent"}); err != nil {
		t.Fatal(err)
	}
	if err := held.Release(); err != nil {
		t.Fatal(err)
	}

	if err := markPendingSyncAfterSelfUpdate(home); err != nil {
		t.Fatalf("markPendingSyncAfterSelfUpdate() error = %v", err)
	}
	got, err := state.Read(home)
	if err != nil {
		t.Fatal(err)
	}
	if !got.PendingSync {
		t.Errorf("PendingSync = false, want true after self-update")
	}
	if got.RDDMode != "on" || got.Persona != "concurrent" {
		t.Fatalf("concurrent writer state was clobbered: %#v", got)
	}
}
