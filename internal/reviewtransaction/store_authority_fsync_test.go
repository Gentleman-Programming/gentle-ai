package reviewtransaction

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func newFsyncTestRecord(t *testing.T) Record {
	t.Helper()
	tx, err := NewTransaction(boundedStart(t, []string{LensReliability}))
	if err != nil {
		t.Fatal(err)
	}
	if err := tx.StartReview(); err != nil {
		t.Fatal(err)
	}
	return Record{Operation: "review/start", Transaction: *tx}
}

func TestLegacyPublicationSyncsEventsBeforeHead(t *testing.T) {
	store := Store{Dir: filepath.Join(t.TempDir(), "legacy-store"), lineageID: "bounded-lineage"}
	var mu sync.Mutex
	var syncLog []string
	var eventsSyncedBeforeHead bool

	originalSync := syncReviewDirectory
	syncReviewDirectory = func(path string) error {
		mu.Lock()
		defer mu.Unlock()
		syncLog = append(syncLog, path)
		if strings.HasSuffix(filepath.Clean(path), "events") {
			if _, err := os.Stat(filepath.Join(store.Dir, "HEAD")); os.IsNotExist(err) {
				eventsSyncedBeforeHead = true
			}
		}
		return nil
	}
	t.Cleanup(func() { syncReviewDirectory = originalSync })

	if _, err := store.Append("", newFsyncTestRecord(t)); err != nil {
		t.Fatalf("Append() error = %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	if !eventsSyncedBeforeHead {
		t.Fatalf("events directory not synced before HEAD; log = %v", syncLog)
	}
}

func TestLegacyPublicationFailsBeforeHeadWhenEventsSyncFails(t *testing.T) {
	store := Store{Dir: filepath.Join(t.TempDir(), "legacy-store-fail"), lineageID: "bounded-lineage"}
	injectedErr := errors.New("injected events fsync failure")
	originalSync := syncReviewDirectory
	syncReviewDirectory = func(path string) error {
		if strings.HasSuffix(filepath.Clean(path), "events") {
			return injectedErr
		}
		return nil
	}
	t.Cleanup(func() { syncReviewDirectory = originalSync })

	if _, err := store.Append("", newFsyncTestRecord(t)); err == nil || !errors.Is(err, injectedErr) {
		t.Fatalf("Append() error = %v, want %v", err, injectedErr)
	}
	if _, statErr := os.Stat(filepath.Join(store.Dir, "HEAD")); !os.IsNotExist(statErr) {
		t.Fatal("HEAD exists despite events sync failure")
	}
}

func setupBundleFsyncFixture(t *testing.T, dir string) (Store, ValidatedChain, []ChainBundleEvent) {
	t.Helper()
	store := Store{Dir: dir, lineageID: "bounded-lineage"}
	record := newFsyncTestRecord(t)
	record.Schema = RecordSchema
	payload, err := json.Marshal(record)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(payload)
	rev := "sha256:" + hex.EncodeToString(sum[:])
	chain := ValidatedChain{
		Records: []Record{record}, Revisions: []string{rev},
		GenesisRevision: rev, HeadRevision: rev, Identity: chainIdentity([]string{rev}),
	}
	return store, chain, []ChainBundleEvent{{Revision: rev, Payload: payload}}
}

func TestBundleInstallationSyncsEventsBeforeHead(t *testing.T) {
	store, chain, events := setupBundleFsyncFixture(t, filepath.Join(t.TempDir(), "bundle-store"))
	var mu sync.Mutex
	var syncLog []string
	var eventsSyncedBeforeHead bool

	originalSync := syncReviewDirectory
	syncReviewDirectory = func(path string) error {
		mu.Lock()
		defer mu.Unlock()
		syncLog = append(syncLog, path)
		if strings.HasSuffix(filepath.Clean(path), "events") {
			if _, err := os.Stat(filepath.Join(store.Dir, "HEAD")); os.IsNotExist(err) {
				eventsSyncedBeforeHead = true
			}
		}
		return nil
	}
	t.Cleanup(func() { syncReviewDirectory = originalSync })

	if _, err := store.installBundle(chain, events); err != nil {
		t.Fatalf("installBundle() error = %v", err)
	}
	mu.Lock()
	defer mu.Unlock()
	if !eventsSyncedBeforeHead {
		t.Fatalf("bundle events not synced before HEAD; log = %v", syncLog)
	}
}

func TestBundleInstallationFailsBeforeHeadWhenEventsSyncFails(t *testing.T) {
	store, chain, events := setupBundleFsyncFixture(t, filepath.Join(t.TempDir(), "bundle-fail"))
	injectedErr := errors.New("injected bundle events sync error")
	originalSync := syncReviewDirectory
	syncReviewDirectory = func(path string) error {
		if strings.HasSuffix(filepath.Clean(path), "events") {
			return injectedErr
		}
		return nil
	}
	t.Cleanup(func() { syncReviewDirectory = originalSync })

	if _, err := store.installBundle(chain, events); err == nil || !errors.Is(err, injectedErr) {
		t.Fatalf("installBundle() error = %v, want %v", err, injectedErr)
	}
	if _, statErr := os.Stat(filepath.Join(store.Dir, "HEAD")); !os.IsNotExist(statErr) {
		t.Fatal("HEAD exists despite bundle events sync failure")
	}
}

func TestFirstCompactPublicationSyncsParentsBottomUp(t *testing.T) {
	authorityRoot := filepath.Join(t.TempDir(), "authority-root")
	lineageID := "lineage-compact-fsync-test"
	v2Dir := filepath.Join(authorityRoot, "v2")
	lineageDir := filepath.Join(v2Dir, lineageID)

	var mu sync.Mutex
	var syncLog []string
	originalSync := syncReviewDirectory
	syncReviewDirectory = func(path string) error {
		mu.Lock()
		defer mu.Unlock()
		syncLog = append(syncLog, filepath.Clean(path))
		return nil
	}
	t.Cleanup(func() { syncReviewDirectory = originalSync })

	repo := initSnapshotRepo(t)
	writeSnapshotFile(t, repo, "tracked.txt", "candidate\n")
	snapshot, err := (SnapshotBuilder{Repo: repo}).Build(context.Background(), Target{Kind: TargetCurrentChanges, IntendedUntracked: []string{}})
	if err != nil {
		t.Fatal(err)
	}
	risk, lines, err := (SnapshotBuilder{Repo: repo}).ClassifySnapshotRisk(context.Background(), snapshot)
	if err != nil {
		t.Fatal(err)
	}

	store := CompactStore{Dir: lineageDir, lineageID: lineageID, lockPath: filepath.Join(v2Dir, "LOCK")}
	state, err := NewCompactState(Start{
		LineageID: lineageID, Mode: ModeOrdinaryBounded, Generation: 1,
		Snapshot: snapshot, PolicyHash: hash("a"), RiskLevel: risk,
		SelectedLenses: []string{LensReliability}, OriginalChangedLines: &lines,
	})
	if err != nil {
		t.Fatalf("NewCompactState: %v", err)
	}

	if _, err := store.ReplaceContext(context.Background(), "", "review/start", state); err != nil {
		t.Fatalf("ReplaceContext() error = %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	v2Clean := filepath.Clean(v2Dir)
	lineageClean := filepath.Clean(lineageDir)
	v2Idx, lineageIdx := -1, -1
	for i, p := range syncLog {
		if p == v2Clean && v2Idx == -1 {
			v2Idx = i
		}
		if p == lineageClean && lineageIdx == -1 {
			lineageIdx = i
		}
	}
	if v2Idx == -1 || lineageIdx == -1 || v2Idx > lineageIdx {
		t.Fatalf("invalid sync order: v2Idx=%d, lineageIdx=%d, log=%v", v2Idx, lineageIdx, syncLog)
	}
}

func TestMkdirAllSyncBottomUpOrderingAndDurability(t *testing.T) {
	base := filepath.Join(t.TempDir(), "base")
	if err := os.Mkdir(base, 0o755); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(base, "a", "b", "c", "d")

	var mu sync.Mutex
	var syncLog []string
	originalSync := syncReviewDirectory
	syncReviewDirectory = func(path string) error {
		mu.Lock()
		defer mu.Unlock()
		syncLog = append(syncLog, filepath.Clean(path))
		return nil
	}
	t.Cleanup(func() { syncReviewDirectory = originalSync })

	if err := mkdirAllSync(target, 0o755); err != nil {
		t.Fatalf("mkdirAllSync() error = %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	expectedOrder := []string{
		filepath.Clean(base),
		filepath.Clean(filepath.Join(base, "a")),
		filepath.Clean(filepath.Join(base, "a", "b")),
		filepath.Clean(filepath.Join(base, "a", "b", "c")),
	}
	if !reflect.DeepEqual(syncLog, expectedOrder) {
		t.Fatalf("mkdirAllSync sync log = %v, want %v", syncLog, expectedOrder)
	}
}

func TestMkdirAllSyncPrePublicationSyncFailurePreventsFileCreation(t *testing.T) {
	base := filepath.Join(t.TempDir(), "base-fail")
	if err := os.Mkdir(base, 0o755); err != nil {
		t.Fatal(err)
	}
	targetFile := filepath.Join(base, "parent", "child", "test.json")

	injectedErr := errors.New("parent directory sync failed")
	originalSync := syncReviewDirectory
	syncReviewDirectory = func(path string) error {
		if strings.HasSuffix(filepath.Clean(path), "parent") {
			return injectedErr
		}
		return nil
	}
	t.Cleanup(func() { syncReviewDirectory = originalSync })

	if err := writeAtomic(targetFile, []byte("data\n"), 0o644); err == nil || !errors.Is(err, injectedErr) {
		t.Fatalf("writeAtomic() error = %v, want %v", err, injectedErr)
	}
	if _, statErr := os.Stat(targetFile); !os.IsNotExist(statErr) {
		t.Fatal("target file was created despite parent directory sync failure")
	}
}

func TestMkdirAllSyncRejectsExistingNonDirectory(t *testing.T) {
	filePath := filepath.Join(t.TempDir(), "file.txt")
	if err := os.WriteFile(filePath, []byte("hello\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := mkdirAllSync(filepath.Join(filePath, "child"), 0o755); err == nil {
		t.Fatal("mkdirAllSync() succeeded on regular file parent, want error")
	}
	if err := mkdirAllSync(filePath, 0o755); err == nil {
		t.Fatal("mkdirAllSync() succeeded on regular file target, want error")
	}
}

func TestMkdirAllSyncFastPathWhenDirectoryExists(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "existing", "dir")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}

	var syncCount int
	originalSync := syncReviewDirectory
	syncReviewDirectory = func(path string) error {
		syncCount++
		return nil
	}
	t.Cleanup(func() { syncReviewDirectory = originalSync })

	if err := mkdirAllSync(dir, 0o755); err != nil {
		t.Fatalf("mkdirAllSync() error = %v", err)
	}
	if syncCount != 0 {
		t.Fatalf("mkdirAllSync() called syncReviewDirectory %d times on existing directory; want 0", syncCount)
	}
}

func TestMkdirAllSyncRejectsSymlinkedTarget(t *testing.T) {
	tempDir := t.TempDir()
	outsideDir := filepath.Join(tempDir, "outside")
	if err := os.Mkdir(outsideDir, 0o755); err != nil {
		t.Fatal(err)
	}

	storeDir := filepath.Join(tempDir, "store")
	if err := os.Mkdir(storeDir, 0o755); err != nil {
		t.Fatal(err)
	}

	symlinkedEvents := filepath.Join(storeDir, "events")
	if err := os.Symlink(outsideDir, symlinkedEvents); err != nil {
		t.Fatal(err)
	}

	if err := mkdirAllSync(symlinkedEvents, 0o755); err == nil {
		t.Fatal("mkdirAllSync() succeeded on symlinked target directory, want error")
	}
}

func TestMkdirAllSyncRejectsSymlinkedAncestor(t *testing.T) {
	tempDir := t.TempDir()
	outsideDir := filepath.Join(tempDir, "outside")
	if err := os.Mkdir(outsideDir, 0o755); err != nil {
		t.Fatal(err)
	}

	symlinkedParent := filepath.Join(tempDir, "symlinked-parent")
	if err := os.Symlink(outsideDir, symlinkedParent); err != nil {
		t.Fatal(err)
	}

	target := filepath.Join(symlinkedParent, "nested", "events")
	if err := mkdirAllSync(target, 0o755); err == nil {
		t.Fatal("mkdirAllSync() succeeded with symlinked ancestor component, want error")
	}
}

func TestMkdirAllSyncConcurrentWritersCoordinateParentSync(t *testing.T) {
	tempDir := t.TempDir()
	target := filepath.Join(tempDir, "a", "b")

	var writer1SyncDone atomic.Bool
	var writer2ObservedBeforeSync atomic.Bool

	mkdirHookCalled := make(chan struct{})
	unblockWriter1 := make(chan struct{})

	originalHook := mkdirAllSyncAfterMkdir
	mkdirAllSyncAfterMkdir = func(path string) {
		if path == target {
			close(mkdirHookCalled)
			<-unblockWriter1
		}
	}
	t.Cleanup(func() { mkdirAllSyncAfterMkdir = originalHook })

	originalSync := syncReviewDirectory
	syncReviewDirectory = func(path string) error {
		if strings.HasSuffix(filepath.Clean(path), "a") {
			writer1SyncDone.Store(true)
		}
		return nil
	}
	t.Cleanup(func() { syncReviewDirectory = originalSync })

	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		if err := mkdirAllSync(target, 0o755); err != nil {
			t.Errorf("writer 1 mkdirAllSync error = %v", err)
		}
	}()

	<-mkdirHookCalled

	go func() {
		defer wg.Done()
		if err := mkdirAllSync(target, 0o755); err != nil {
			t.Errorf("writer 2 mkdirAllSync error = %v", err)
		}
		if !writer1SyncDone.Load() {
			writer2ObservedBeforeSync.Store(true)
		}
	}()

	time.Sleep(50 * time.Millisecond)
	close(unblockWriter1)

	wg.Wait()

	if writer2ObservedBeforeSync.Load() {
		t.Fatal("writer 2 returned from mkdirAllSync before writer 1 finished parent directory sync")
	}
}
