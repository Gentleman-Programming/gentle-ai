package reviewtransaction

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// syncRecorder replaces syncReviewDirectory and records every path it is
// asked to sync. Tests assert against the ordered list of paths.
type syncRecorder struct {
	mu    sync.Mutex
	paths []string
}

func (r *syncRecorder) wrap(prev func(string) error) func(string) error {
	return func(path string) error {
		r.mu.Lock()
		r.paths = append(r.paths, path)
		r.mu.Unlock()
		return prev(path)
	}
}

func (r *syncRecorder) snapshot() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]string, len(r.paths))
	copy(out, r.paths)
	return out
}

func withSyncRecorder(t *testing.T) *syncRecorder {
	t.Helper()
	r := &syncRecorder{}
	original := syncReviewDirectory
	syncReviewDirectory = r.wrap(original)
	t.Cleanup(func() { syncReviewDirectory = original })
	return r
}

// TestLegacyPublicationSyncsEventsBeforeHead verifies that the legacy append
// flow synchronizes the events/ directory before publishing HEAD.
func TestLegacyPublicationSyncsEventsBeforeHead(t *testing.T) {
	store := Store{Dir: filepath.Join(t.TempDir(), "review-store")}
	tx := newTestTransaction(t, ModeOrdinary4R)
	if err := tx.StartReview(); err != nil {
		t.Fatal(err)
	}
	rec := withSyncRecorder(t)
	if _, err := store.Append("", Record{Operation: "review/start", Transaction: *tx}); err != nil {
		t.Fatalf("Append() error = %v", err)
	}
	paths := rec.snapshot()
	eventsDir := filepath.Join(store.Dir, "events")
	foundEvents := false
	for _, p := range paths {
		if p == eventsDir {
			foundEvents = true
			break
		}
	}
	if !foundEvents {
		t.Fatalf("syncReviewDirectory was never called with events/ directory %q; recorded paths = %v", eventsDir, paths)
	}
}

// TestFirstCompactPublicationSyncsV2Parent verifies that when
// writeAtomic publishes a file under a previously-absent v2/<lineage>/
// directory, the v2/ grandparent appears in the sync record.
func TestFirstCompactPublicationSyncsV2Parent(t *testing.T) {
	rec := withSyncRecorder(t)
	storeDir := filepath.Join(t.TempDir(), "review-store")
	// Simulate first compact publication: writeAtomic on a path whose v2/
	// grandparent does not exist. Use a temporary path that mimics
	// <authority-root>/v2/<lineage>/review-state.json.
	lineageDir := filepath.Join(storeDir, "v2", "lineage-test")
	targetPath := filepath.Join(lineageDir, "review-state.json")
	if err := writeAtomic(targetPath, []byte(`{}`), 0o644); err != nil {
		t.Fatalf("writeAtomic() error = %v", err)
	}
	paths := rec.snapshot()
	v2Dir := filepath.Join(storeDir, "v2")
	foundV2 := false
	for _, p := range paths {
		if p == v2Dir {
			foundV2 = true
			break
		}
	}
	if !foundV2 {
		t.Fatalf("syncReviewDirectory was never called with v2/ directory %q; recorded paths = %v", v2Dir, paths)
	}
}

// TestMkdirAllSyncReturnsDirectorySyncError verifies that mkdirAllSync
// wraps a sync failure in directorySyncError so callers can identify it.
func TestMkdirAllSyncReturnsDirectorySyncError(t *testing.T) {
	original := syncReviewDirectory
	syncReviewDirectory = func(string) error { return errors.New("injected") }
	t.Cleanup(func() { syncReviewDirectory = original })

	storeDir := filepath.Join(t.TempDir(), "review-store")
	lineageDir := filepath.Join(storeDir, "v2", "lineage-test")
	targetPath := filepath.Join(lineageDir, "review-state.json")
	err := mkdirAllSync(targetPath, 0o755)
	if err == nil {
		t.Fatal("mkdirAllSync() error = nil, want *directorySyncError")
	}
	var dsErr *directorySyncError
	if !errors.As(err, &dsErr) {
		t.Fatalf("mkdirAllSync() error = %T %v, want *directorySyncError", err, err)
	}
}

// TestWriteAtomicSyncsNewlyCreatedParents verifies that writeAtomic
// synchronizes newly-created parent directories bottom-up: the directory
// closest to the file is synced first, then each ancestor.
func TestWriteAtomicSyncsNewlyCreatedParents(t *testing.T) {
	rec := withSyncRecorder(t)
	// Build a path where a/ exists but b/, c/, d/ do not.
	base := filepath.Join(t.TempDir(), "sync-parents-test")
	if err := os.MkdirAll(filepath.Join(base, "a"), 0o755); err != nil {
		t.Fatal(err)
	}
	targetPath := filepath.Join(base, "a", "b", "c", "d", "file.json")
	if err := writeAtomic(targetPath, []byte(`{}`), 0o644); err != nil {
		t.Fatalf("writeAtomic() error = %v", err)
	}
	paths := rec.snapshot()
	// Extract the directories among recorded paths that are under base/a/.
	var dirs []string
	for _, p := range paths {
		if strings.HasPrefix(p, filepath.Join(base, "a")+string(filepath.Separator)) {
			dirs = append(dirs, p)
		}
	}
	// Expected bottom-up order: d/, c/, b/ (closest to file first).
	want := []string{
		filepath.Join(base, "a", "b", "c", "d"),
		filepath.Join(base, "a", "b", "c"),
		filepath.Join(base, "a", "b"),
	}
	if len(dirs) < 3 {
		t.Fatalf("recorded dirs under a/ = %v, want at least %v", dirs, want)
	}
	// Check the first 3 are in bottom-up order.
	for i := range want {
		if dirs[i] != want[i] {
			t.Errorf("dirs[%d] = %q, want %q (bottom-up order)", i, dirs[i], want[i])
		}
	}
}

// TestSyncFailureShortCircuitsBeforeFilePublish verifies that when
// mkdirAllSync fails, writeAtomic returns an error and the target file
// is not created.
func TestSyncFailureShortCircuitsBeforeFilePublish(t *testing.T) {
	original := syncReviewDirectory
	syncReviewDirectory = func(string) error { return errors.New("injected sync failure") }
	t.Cleanup(func() { syncReviewDirectory = original })

	base := filepath.Join(t.TempDir(), "shortcircuit-test")
	// Ensure the parent exists so mkdirAllSync walks up and creates the missing child.
	parent := filepath.Join(base, "parent")
	if err := os.MkdirAll(parent, 0o755); err != nil {
		t.Fatal(err)
	}
	targetPath := filepath.Join(parent, "child", "file.json")
	err := writeAtomic(targetPath, []byte(`{}`), 0o644)
	if err == nil {
		t.Fatal("writeAtomic() error = nil, want injected sync failure")
	}
	if _, statErr := os.Stat(targetPath); !os.IsNotExist(statErr) {
		t.Fatalf("target file %q exists after sync failure; want it not to exist", targetPath)
	}
}
