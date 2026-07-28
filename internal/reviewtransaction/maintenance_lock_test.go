package reviewtransaction

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestAuthorityLockCancellationPreservesSentinelAndCallerDeadline(t *testing.T) {
	err := &AuthorityLockCancelledError{Cause: context.DeadlineExceeded}
	if !errors.Is(err, ErrAuthorityLockCancelled) {
		t.Fatal("lock cancellation lost its typed authority sentinel")
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal("lock cancellation masked the caller deadline")
	}
}

func TestCompactStatusLoadContextCancelsUnderExclusiveMaintenance(t *testing.T) {
	repo := initSnapshotRepo(t)
	state := newCompactTestState(t, repo, "status-lock-cancelled")
	store, err := CompactAuthoritativeStore(context.Background(), repo, state.LineageID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Replace("", "review/start", state); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(store.StatePath())
	if err != nil {
		t.Fatal(err)
	}
	lockContext, releaseContext := context.WithTimeout(context.Background(), time.Second)
	defer releaseContext()
	exclusive, err := AcquireReviewMaintenanceExclusive(lockContext, repo)
	if err != nil {
		t.Fatal(err)
	}
	defer exclusive.Release()

	loadContext, cancelLoad := context.WithCancel(context.Background())
	time.AfterFunc(50*time.Millisecond, cancelLoad)
	started := time.Now()
	_, err = store.LoadContext(loadContext)
	if elapsed := time.Since(started); elapsed >= time.Second {
		t.Fatalf("cancelled status load took %s", elapsed)
	}
	var cancelled *AuthorityLockCancelledError
	if !errors.As(err, &cancelled) || !errors.Is(err, ErrAuthorityLockCancelled) || !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled status load = %#v", err)
	}
	after, readErr := os.ReadFile(store.StatePath())
	if readErr != nil {
		t.Fatal(readErr)
	}
	if !bytes.Equal(before, after) {
		t.Fatal("cancelled status load changed authority bytes")
	}
}

func TestMaintenanceLockModesAndRelease(t *testing.T) {
	path := filepath.Join(t.TempDir(), "gentle-ai", "review-transactions", "MAINTENANCE.lock")
	shared, err := acquireMaintenanceLock(context.Background(), path, maintenanceShared)
	if err != nil {
		t.Fatal(err)
	}
	secondShared, err := acquireMaintenanceLock(context.Background(), path, maintenanceShared)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := acquireMaintenanceLock(context.Background(), path, maintenanceExclusive); !errors.Is(err, ErrAuthorityLockTimeout) {
		t.Fatalf("exclusive while shared = %v, want timeout", err)
	}
	if err := secondShared.Release(); err != nil {
		t.Fatal(err)
	}
	if err := shared.Release(); err != nil {
		t.Fatal(err)
	}
	exclusive, err := acquireMaintenanceLock(context.Background(), path, maintenanceExclusive)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := acquireMaintenanceLock(context.Background(), path, maintenanceShared); !errors.Is(err, ErrAuthorityLockTimeout) {
		t.Fatalf("shared while exclusive = %v, want timeout", err)
	}
	if _, err := acquireMaintenanceLock(context.Background(), path, maintenanceExclusive); !errors.Is(err, ErrAuthorityLockTimeout) {
		t.Fatalf("exclusive while exclusive = %v, want timeout", err)
	}
	if err := exclusive.Release(); err != nil {
		t.Fatal(err)
	}
	reacquired, err := acquireMaintenanceLock(context.Background(), path, maintenanceShared)
	if err != nil {
		t.Fatalf("shared after release: %v", err)
	}
	if err := reacquired.Release(); err != nil {
		t.Fatal(err)
	}
}

func TestMaintenanceLockRejectsSymlinksAndStaleBytesAreNotOwnership(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "gentle-ai", "review-transactions", "MAINTENANCE.lock")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("stale owner metadata\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	lock, err := acquireMaintenanceLock(context.Background(), path, maintenanceExclusive)
	if err != nil {
		t.Fatalf("stale bytes must not own lock: %v", err)
	}
	if err := lock.Release(); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(dir, "outside"), path); err != nil {
		t.Fatal(err)
	}
	if _, err := acquireMaintenanceLock(context.Background(), path, maintenanceShared); err == nil {
		t.Fatal("symlinked maintenance lock was accepted")
	}
}

func TestEnsureMaintenanceLockPathRejectsRelativePaths(t *testing.T) {
	for _, path := range []string{
		"MAINTENANCE.lock",
		"review-transactions/MAINTENANCE.lock",
		"./review-transactions/MAINTENANCE.lock",
	} {
		t.Run(path, func(t *testing.T) {
			if err := ensureMaintenanceLockPath(path); err == nil {
				t.Fatal("relative maintenance lock path was accepted")
			}
		})
	}
}

func TestEnsureMaintenanceLockPathAcceptsCanonicalAbsolutePath(t *testing.T) {
	path := filepath.Join(t.TempDir(), "gentle-ai", "REVIEW-MAINTENANCE.lock")
	if err := ensureMaintenanceLockPath(path); err != nil {
		t.Fatalf("canonical absolute maintenance lock path was rejected: %v", err)
	}
	if info, err := os.Stat(filepath.Dir(path)); err != nil || !info.IsDir() {
		t.Fatalf("maintenance lock directory = %v, %v", info, err)
	}
}

func TestMaintenanceLockHonorsCancellation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "MAINTENANCE.lock")
	held, err := acquireMaintenanceLock(context.Background(), path, maintenanceExclusive)
	if err != nil {
		t.Fatal(err)
	}
	defer held.Release()
	ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond)
	defer cancel()
	if _, err := acquireMaintenanceLock(ctx, path, maintenanceShared); !errors.Is(err, ErrAuthorityLockCancelled) {
		t.Fatalf("cancelled acquisition = %v", err)
	}
}

func TestMaintenanceLockIsReleasedWhenOwnerProcessExits(t *testing.T) {
	if os.Getenv("GENTLE_AI_MAINTENANCE_LOCK_EXIT_HELPER") == "1" {
		lock, err := acquireMaintenanceLock(context.Background(), os.Getenv("GENTLE_AI_MAINTENANCE_LOCK_EXIT_PATH"), maintenanceExclusive)
		if err != nil {
			t.Fatal(err)
		}
		_ = lock
		return
	}
	path := filepath.Join(t.TempDir(), "MAINTENANCE.lock")
	command := exec.Command(os.Args[0], "-test.run=^TestMaintenanceLockIsReleasedWhenOwnerProcessExits$")
	command.Env = append(os.Environ(), "GENTLE_AI_MAINTENANCE_LOCK_EXIT_HELPER=1", "GENTLE_AI_MAINTENANCE_LOCK_EXIT_PATH="+path)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("maintenance lock owner helper: %v\n%s", err, output)
	}
	lock, err := acquireMaintenanceLock(context.Background(), path, maintenanceExclusive)
	if err != nil {
		t.Fatalf("kernel maintenance lock remained held after process exit: %v", err)
	}
	defer lock.Release()
}

func TestMaintenanceExclusivePreventsAuthorityDirectoryRecreation(t *testing.T) {
	repo := initSnapshotRepo(t)
	authorityRoot, _, err := reviewAuthorityRoot(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	held, err := AcquireReviewMaintenanceExclusive(ctx, repo)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(authorityRoot); err != nil {
		t.Fatal(err)
	}
	lockPath := filepath.Join(authorityRoot, "v2", "LOCK")
	if _, err := acquireStoreLock(lockPath); !errors.Is(err, ErrAuthorityLockTimeout) {
		t.Fatalf("writer acquired replaced authority while maintenance held: %v", err)
	}
	if _, err := os.Stat(lockPath); !os.IsNotExist(err) {
		t.Fatalf("writer mutated replaced authority while maintenance held: %v", err)
	}
	if err := held.Release(); err != nil {
		t.Fatal(err)
	}
	lock, err := acquireStoreLock(lockPath)
	if err != nil {
		t.Fatalf("writer did not acquire after maintenance release: %v", err)
	}
	defer lock.release()
}

func TestMaintenanceExclusiveBlocksReviewTransactionsLineageLock(t *testing.T) {
	repo := initSnapshotRepo(t)
	authorityRoot, _, err := reviewAuthorityRoot(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	held, err := AcquireReviewMaintenanceExclusive(ctx, repo)
	if err != nil {
		t.Fatal(err)
	}
	lockPath := filepath.Join(authorityRoot, "v1", "review-transactions", "LOCK")
	if _, err := acquireStoreLock(lockPath); !errors.Is(err, ErrAuthorityLockTimeout) {
		t.Fatalf("review-transactions lineage acquired while maintenance held: %v", err)
	}
	if _, err := os.Stat(lockPath); !os.IsNotExist(err) {
		t.Fatalf("review-transactions lineage mutated while maintenance held: %v", err)
	}
	if err := held.Release(); err != nil {
		t.Fatal(err)
	}
	lock, err := acquireStoreLock(lockPath)
	if err != nil {
		t.Fatalf("review-transactions lineage did not acquire after release: %v", err)
	}
	defer lock.release()
}

func TestMaintenanceExclusiveBlocksDiscoveredCompactStoreReplace(t *testing.T) {
	repo := initSnapshotRepo(t)
	started, err := StartCompactAuthority(context.Background(), repo, CompactStartRequest{State: newCompactTestState(t, repo, "maintenance-discovery")})
	if err != nil {
		t.Fatal(err)
	}
	stores, err := DiscoverCompactStores(context.Background(), repo)
	if err != nil || len(stores) != 1 {
		t.Fatalf("discover compact stores = %d, %v", len(stores), err)
	}
	discovered := stores[0]
	if discovered.maintenanceLockPath == "" {
		t.Fatal("discovered compact store has no maintenance lock identity")
	}
	before, err := os.ReadFile(discovered.StatePath())
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	held, err := AcquireReviewMaintenanceExclusive(ctx, repo)
	if err != nil {
		t.Fatal(err)
	}
	blocked, blockedCancel := context.WithTimeout(context.Background(), time.Millisecond)
	defer blockedCancel()
	if _, err := discovered.ReplaceContext(blocked, started.Record.Revision, "review/replay", started.Record.State); !errors.Is(err, ErrAuthorityLockCancelled) {
		t.Fatalf("discovered replace while maintenance held = %v", err)
	}
	after, err := os.ReadFile(discovered.StatePath())
	if err != nil || !bytes.Equal(before, after) {
		t.Fatalf("authority changed while replace blocked: %v", err)
	}
	if err := held.Release(); err != nil {
		t.Fatal(err)
	}
	if _, err := discovered.ReplaceContext(context.Background(), started.Record.Revision, "review/replay", started.Record.State); err != nil {
		t.Fatalf("discovered replace after release: %v", err)
	}
}

func TestMaintenanceLockUsesGitCommonDirAcrossWorktrees(t *testing.T) {
	repo := initSnapshotRepo(t)
	linked := filepath.Join(t.TempDir(), "linked")
	gitSnapshot(t, repo, "worktree", "add", "--detach", linked)
	t.Cleanup(func() { gitSnapshot(t, repo, "worktree", "remove", "--force", linked) })
	t.Setenv("GIT_DIR", filepath.Join(t.TempDir(), "forged-git-dir"))
	mainStore, err := AuthoritativeStore(context.Background(), repo, "common-dir-lock")
	if err != nil {
		t.Fatal(err)
	}
	linkedStore, err := AuthoritativeStore(context.Background(), linked, "common-dir-lock")
	if err != nil {
		t.Fatal(err)
	}
	if mainStore.maintenanceLockPath != linkedStore.maintenanceLockPath {
		t.Fatalf("maintenance paths differ: %q != %q", mainStore.maintenanceLockPath, linkedStore.maintenanceLockPath)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	held, err := AcquireReviewMaintenanceExclusive(ctx, repo)
	if err != nil {
		t.Fatal(err)
	}
	defer held.Release()
	blocked, cancelBlocked := context.WithTimeout(context.Background(), time.Millisecond)
	defer cancelBlocked()
	if _, err := acquireMaintenanceLock(blocked, linkedStore.maintenanceLockPath, maintenanceShared); !errors.Is(err, ErrAuthorityLockCancelled) {
		t.Fatalf("linked worktree shared lock while exclusive held = %v", err)
	}
}

func TestMaintenanceLockRejectsSymlinkedAuthorityComponent(t *testing.T) {
	dir := t.TempDir()
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(dir, "gentle-ai")); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "gentle-ai", "review-transactions", "MAINTENANCE.lock")
	if _, err := acquireMaintenanceLock(context.Background(), path, maintenanceShared); err == nil {
		t.Fatal("symlinked authority root was accepted")
	}
}
