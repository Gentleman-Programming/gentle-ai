//go:build windows

package shellinstaller

import (
	"os"
	"path/filepath"
	"testing"
)

// userWindowsManageFixture publishes an active Separate installation for the
// Manage listing and prune entry points, reusing the retention fixture so
// candidates are exactly what the validated primitive already accepts.
func userWindowsManageFixture(t *testing.T) (root string, install UserShellInstall) {
	t.Helper()
	root, destination := userWindowsRetentionFixture(t)
	return root, UserShellInstall{Target: destination, Mode: "separate"}
}

// userWindowsManageTreeBytes sums every regular file with an unbounded walk so
// tests can compare the bounded size walk against an independent exact total.
func userWindowsManageTreeBytes(t *testing.T, root string) int64 {
	t.Helper()
	var total int64
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, cause error) error {
		if cause != nil {
			return cause
		}
		if entry.IsDir() {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		total += info.Size()
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return total
}

// (a) Listing returns exact canonical siblings sorted by UTC stamp, each with
// a positive exact size, and marks only the lexically latest candidate.
func TestUserReplacedBackupsSortedCandidatesLatestAndSizes(t *testing.T) {
	root, install := userWindowsManageFixture(t)
	// Create out of chronological order to prove the list is sorted.
	for _, stamp := range []string{"20260101T130000Z", "20260101T110000Z", "20260101T120000Z"} {
		userWindowsRetentionBackup(t, root, install.Target, stamp, nil)
	}

	backups, err := UserReplacedBackups(install)
	if err != nil {
		t.Fatalf("listing refused valid candidates: %v", err)
	}
	if len(backups) != 3 {
		t.Fatalf("listing returned %d candidates, want 3: %+v", len(backups), backups)
	}
	want := []string{"20260101T110000Z", "20260101T120000Z", "20260101T130000Z"}
	for index, backup := range backups {
		if backup.Stamp != want[index] {
			t.Fatalf("candidate %d has stamp %q, want %q", index, backup.Stamp, want[index])
		}
		if backup.Path != filepath.Join(root, "install.replaced-"+want[index]) {
			t.Fatalf("candidate %d has path %q", index, backup.Path)
		}
		if backup.Bytes <= 0 {
			t.Fatalf("candidate %d reported a non-positive size %d", index, backup.Bytes)
		}
		if !backup.Complete {
			t.Fatalf("candidate %d size walk is incomplete", index)
		}
		if backup.Latest != (index == len(backups)-1) {
			t.Fatalf("candidate %d has Latest=%v, want %v", index, backup.Latest, index == len(backups)-1)
		}
	}
}

// (b) Listing ignores names that are not exact replaced siblings with a
// canonical UTC timestamp.
func TestUserReplacedBackupsIgnoresUnrelatedNames(t *testing.T) {
	root, install := userWindowsManageFixture(t)
	for _, name := range []string{
		"other-install.replaced-20260101T110000Z", // unrelated base
		"install.old",                           // wrong marker
		"install.replaced-20260101T110000",      // missing Z
		"install.replaced-2026-01-01T11-00-00Z", // noncanonical layout
	} {
		if err := os.MkdirAll(filepath.Join(root, name), 0700); err != nil {
			t.Fatal(err)
		}
	}
	valid := userWindowsRetentionBackup(t, root, install.Target, "20260101T120000Z", nil)

	backups, err := UserReplacedBackups(install)
	if err != nil {
		t.Fatalf("listing reported an unrelated sibling: %v", err)
	}
	if len(backups) != 1 || backups[0].Path != valid {
		t.Fatalf("listing returned %+v, want only %q", backups, valid)
	}
}

// (c) The size walk is bounded: past the entry bound it reports the partial
// sum as an incomplete lower bound, never a fabricated exact total.
func TestUserReplacedBackupsBoundedSizeWalkIncompleteLowerBound(t *testing.T) {
	root, install := userWindowsManageFixture(t)
	userWindowsRetentionBackup(t, root, install.Target, "20260101T120000Z", nil)
	total := userWindowsManageTreeBytes(t, filepath.Join(root, "install.replaced-20260101T120000Z"))

	previous := userWindowsReplacedSizeEntries
	userWindowsReplacedSizeEntries = 4
	t.Cleanup(func() { userWindowsReplacedSizeEntries = previous })

	backups, err := UserReplacedBackups(install)
	if err != nil {
		t.Fatalf("listing refused the candidate: %v", err)
	}
	if len(backups) != 1 {
		t.Fatalf("listing returned %d candidates, want 1", len(backups))
	}
	candidate := backups[0]
	if candidate.Complete {
		t.Fatal("size walk completed despite exceeding the entry bound")
	}
	if candidate.Bytes <= 0 {
		t.Fatalf("incomplete size walk reported a non-positive lower bound %d", candidate.Bytes)
	}
	if candidate.Bytes >= total {
		t.Fatalf("incomplete size walk reported %d, not a lower bound below total %d", candidate.Bytes, total)
	}
}

// (d) Batch prune removes every older valid candidate and retains only the
// lexically latest backup beside the untouched active installation.
func TestUserPruneReplacedBackupsRemovesOlderKeepsLatest(t *testing.T) {
	root, install := userWindowsManageFixture(t)
	userWindowsRetentionBackup(t, root, install.Target, "20260101T110000Z", nil)
	userWindowsRetentionBackup(t, root, install.Target, "20260101T120000Z", nil)
	latest := userWindowsRetentionBackup(t, root, install.Target, "20260101T130000Z", nil)

	if err := UserPruneReplacedBackups(install); err != nil {
		t.Fatalf("batch prune refused valid candidates: %v", err)
	}
	userWindowsRetentionAssertEntries(t, root, "install", "install.replaced-20260101T130000Z")
	userWindowsRetentionAssertWhole(t, latest, "the retained latest backup")
	userWindowsRetentionAssertWhole(t, install.Target, "the active installation")
}

// (e) A completed size walk reports the exact byte total, matching an
// independent unbounded walk of the same tree, so listing never underreports
// a fully walked candidate.
func TestUserReplacedBackupsExactCompleteByteCount(t *testing.T) {
	root, install := userWindowsManageFixture(t)
	userWindowsRetentionBackup(t, root, install.Target, "20260101T120000Z", nil)
	want := userWindowsManageTreeBytes(t, filepath.Join(root, "install.replaced-20260101T120000Z"))

	backups, err := UserReplacedBackups(install)
	if err != nil {
		t.Fatalf("listing refused candidates: %v", err)
	}
	if len(backups) != 1 {
		t.Fatalf("listing returned %d candidates, want 1", len(backups))
	}
	candidate := backups[0]
	if !candidate.Complete {
		t.Fatal("size walk reported incomplete for a small tree")
	}
	if candidate.Bytes != want {
		t.Fatalf("completed size walk reported %d, want exact total %d", candidate.Bytes, want)
	}
}

// (f) Listing is read-only: it enumerates exact canonical siblings without
// validating manifests, so a candidate whose manifest is damaged is still
// listed and never reported as an error.
func TestUserReplacedBackupsListsInvalidManifest(t *testing.T) {
	root, install := userWindowsManageFixture(t)
	backup := userWindowsRetentionBackup(t, root, install.Target, "20260101T120000Z", nil)
	if err := os.WriteFile(filepath.Join(backup, "manifest.json"), []byte(`{"not":"a manifest"}`), 0600); err != nil {
		t.Fatal(err)
	}

	backups, err := UserReplacedBackups(install)
	if err != nil {
		t.Fatalf("listing refused an invalid manifest instead of listing read-only: %v", err)
	}
	if len(backups) != 1 || backups[0].Path != backup || backups[0].Stamp != "20260101T120000Z" {
		t.Fatalf("listing dropped the invalid-manifest candidate: %+v", backups)
	}
	if !backups[0].Latest {
		t.Fatal("the sole candidate was not marked latest")
	}
}

// (g) The latest marker compares canonical stamps, never full names or paths:
// a destination base that sorts below the stamp digits (for example "1install")
// must still mark the lexically latest future candidate.
func TestUserReplacedBackupsLatestLowSortingBase(t *testing.T) {
	root := t.TempDir()
	if err := userWindowsPrivate(root); err != nil {
		t.Fatal(err)
	}
	destination := filepath.Join(root, "1install")
	userWindowsRetentionWriteTree(t, destination, destination, nil)
	install := UserShellInstall{Target: destination, Mode: "separate"}
	for _, stamp := range []string{"20260101T140000Z", "20260101T110000Z", "20260101T120000Z"} {
		userWindowsRetentionBackup(t, root, destination, stamp, nil)
	}

	backups, err := UserReplacedBackups(install)
	if err != nil {
		t.Fatalf("listing refused candidates: %v", err)
	}
	want := []string{"20260101T110000Z", "20260101T120000Z", "20260101T140000Z"}
	if len(backups) != len(want) {
		t.Fatalf("listing returned %d candidates, want %d: %+v", len(backups), len(want), backups)
	}
	for index, backup := range backups {
		if backup.Stamp != want[index] {
			t.Fatalf("candidate %d has stamp %q, want %q", index, backup.Stamp, want[index])
		}
		if backup.Latest != (index == len(backups)-1) {
			t.Fatalf("candidate %d has Latest=%v, want %v", index, backup.Latest, index == len(backups)-1)
		}
	}
}

// (h) Prune with no candidates is a no-op: the active installation is the only
// entry and nothing is reported.
func TestUserPruneReplacedBackupsZeroCandidatesNoOp(t *testing.T) {
	root, install := userWindowsManageFixture(t)
	if err := UserPruneReplacedBackups(install); err != nil {
		t.Fatalf("zero-candidate prune reported an error: %v", err)
	}
	userWindowsRetentionAssertEntries(t, root, "install")
	userWindowsRetentionAssertWhole(t, install.Target, "the active installation")
}

// (i) Prune with a single candidate is a no-op: that candidate is the
// lexically latest and must be kept, not removed.
func TestUserPruneReplacedBackupsSingleCandidateNoOp(t *testing.T) {
	root, install := userWindowsManageFixture(t)
	backup := userWindowsRetentionBackup(t, root, install.Target, "20260101T120000Z", nil)
	if err := UserPruneReplacedBackups(install); err != nil {
		t.Fatalf("single-candidate prune reported an error: %v", err)
	}
	userWindowsRetentionAssertEntries(t, root, "install", "install.replaced-20260101T120000Z")
	userWindowsRetentionAssertWhole(t, backup, "the sole retained candidate")
	userWindowsRetentionAssertWhole(t, install.Target, "the active installation")
}

// (j) Batch prune propagates the joined preservation error from the validated
// primitive: a valid older candidate is removed, but the uncertain candidate is
// preserved whole and named, beside the retained latest candidate.
func TestUserPruneReplacedBackupsJoinedErrorPropagation(t *testing.T) {
	root, install := userWindowsManageFixture(t)
	userWindowsRetentionBackup(t, root, install.Target, "20260101T110000Z", nil)
	uncertainName := "install.replaced-20260101T120000Z"
	uncertain := userWindowsRetentionBackup(t, root, install.Target, "20260101T120000Z", nil)
	if err := os.WriteFile(filepath.Join(uncertain, "selection.json"), []byte(`{"Destination":"tampered","Mode":"separate","Channel":"stable"}`), 0600); err != nil {
		t.Fatal(err)
	}
	latest := userWindowsRetentionBackup(t, root, install.Target, "20260101T130000Z", nil)

	err := UserPruneReplacedBackups(install)
	userWindowsRetentionJoined(t, err, uncertainName)
	userWindowsRetentionAssertEntries(t, root, "install", uncertainName, "install.replaced-20260101T130000Z")
	userWindowsRetentionAssertWhole(t, uncertain, "an uncertain candidate")
	userWindowsRetentionAssertWhole(t, latest, "the retained latest candidate")
	userWindowsRetentionAssertWhole(t, install.Target, "the active installation")
}

// (k) The latest marker from listing is exactly the keep the batch prune
// retains: listing and prune agree on which candidate survives.
func TestUserPruneReplacedBackupsLatestAgreement(t *testing.T) {
	root, install := userWindowsManageFixture(t)
	userWindowsRetentionBackup(t, root, install.Target, "20260101T110000Z", nil)
	userWindowsRetentionBackup(t, root, install.Target, "20260101T120000Z", nil)
	userWindowsRetentionBackup(t, root, install.Target, "20260101T130000Z", nil)

	backups, err := UserReplacedBackups(install)
	if err != nil {
		t.Fatalf("listing refused candidates: %v", err)
	}
	latest := ""
	for _, backup := range backups {
		if backup.Latest {
			latest = backup.Path
		}
	}
	if latest != filepath.Join(root, "install.replaced-20260101T130000Z") {
		t.Fatalf("listing marked %q as latest, want the 130000Z candidate", latest)
	}

	if err := UserPruneReplacedBackups(install); err != nil {
		t.Fatalf("prune refused candidates: %v", err)
	}
	userWindowsRetentionAssertEntries(t, root, "install", "install.replaced-20260101T130000Z")
	userWindowsRetentionAssertWhole(t, latest, "the candidate listing marked latest")
	userWindowsRetentionAssertWhole(t, install.Target, "the active installation")
}
