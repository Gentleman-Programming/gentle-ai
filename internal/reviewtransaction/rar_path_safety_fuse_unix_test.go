//go:build !windows

package reviewtransaction

import (
	"errors"
	"io/fs"
	"strings"
	"syscall"
	"testing"
	"time"
)

// fakeUnixDirInfo is a minimal fs.FileInfo whose Sys() reports a
// caller-chosen *syscall.Stat_t, so the ownership predicate can be exercised
// against an arbitrary uid without a real filesystem entry actually owned by
// it (which would require root).
type fakeUnixDirInfo struct{ uid uint32 }

func (f fakeUnixDirInfo) Name() string       { return "fake" }
func (f fakeUnixDirInfo) Size() int64        { return 0 }
func (f fakeUnixDirInfo) Mode() fs.FileMode  { return fs.ModeDir | 0o755 }
func (f fakeUnixDirInfo) ModTime() time.Time { return time.Time{} }
func (f fakeUnixDirInfo) IsDir() bool        { return true }
func (f fakeUnixDirInfo) Sys() any           { return &syscall.Stat_t{Uid: f.uid} }

// TestRarRepositoryDirectorySafeRefusesUid0WithSeamedEUID pins #2838's
// starting condition deterministically: a directory that reports owner uid 0
// (exactly what ntfs-3g reports for every path when mounted with a fixed
// user_id=0) must still fail rarRepositoryDirectorySafe's ownership check
// when the current process's euid is seamed to something else, regardless of
// whichever uid actually runs this test.
func TestRarRepositoryDirectorySafeRefusesUid0WithSeamedEUID(t *testing.T) {
	previous := rarCurrentEUID
	rarCurrentEUID = func() int { return 1000 }
	t.Cleanup(func() { rarCurrentEUID = previous })

	if rarRepositoryDirectorySafe("ignored", fakeUnixDirInfo{uid: 0}) {
		t.Fatal("rarRepositoryDirectorySafe accepted a directory reporting uid 0 while the seamed euid is 1000")
	}
}

// TestFormatRARAuthorityRefusalNamesAFUSERemountRouteInsteadOfChown proves the
// #2838 fix: when the injected FUSE-projected-ownership seam reports true,
// the refusal must be the distinct typed error naming a real remedy (remount
// without a fixed owner projection, or move the repository), never the
// generic chown guidance that cannot succeed on such a mount.
func TestFormatRARAuthorityRefusalNamesAFUSERemountRouteInsteadOfChown(t *testing.T) {
	dir := t.TempDir()
	previous := rarPOSIXFUSEProjectedOwnership
	rarPOSIXFUSEProjectedOwnership = func(string) (bool, error) { return true, nil }
	t.Cleanup(func() { rarPOSIXFUSEProjectedOwnership = previous })

	err := formatRARAuthorityRefusal(dir)
	if err == nil {
		t.Fatal("formatRARAuthorityRefusal returned nil for a FUSE-projected owner")
	}
	if !errors.Is(err, errUnsafeRARAuthorityPath) {
		t.Fatalf("err = %v, want errors.Is(_, errUnsafeRARAuthorityPath)", err)
	}
	if !errors.Is(err, errFUSEProjectedRAROwnership) {
		t.Fatalf("err = %v, want errors.Is(_, errFUSEProjectedRAROwnership)", err)
	}
	message := err.Error()
	if strings.Contains(message, "chown") {
		t.Fatalf("FUSE refusal still names the unactionable chown remedy: %q", message)
	}
	if !strings.Contains(message, "remount") {
		t.Fatalf("FUSE refusal names no runnable route: %q", message)
	}
}

// TestFormatRARAuthorityRefusalKeepsChownGuidanceWhenNotFUSE proves the fix is
// additive: an ordinary wrong-owner directory that is NOT on a FUSE mount
// must keep the existing chown guidance unchanged.
func TestFormatRARAuthorityRefusalKeepsChownGuidanceWhenNotFUSE(t *testing.T) {
	dir := t.TempDir()
	previous := rarPOSIXFUSEProjectedOwnership
	rarPOSIXFUSEProjectedOwnership = func(string) (bool, error) { return false, nil }
	t.Cleanup(func() { rarPOSIXFUSEProjectedOwnership = previous })

	err := formatRARAuthorityRefusal(dir)
	if err == nil || !strings.Contains(err.Error(), "chown") {
		t.Fatalf("err = %v, want the existing chown guidance for a non-FUSE refusal", err)
	}
	if errors.Is(err, errFUSEProjectedRAROwnership) {
		t.Fatalf("err = %v, an ordinary refusal must not be mislabelled as FUSE-projected", err)
	}
}

// TestFormatRARAuthorityRefusalKeepsChownGuidanceWhenProbeFails proves the
// fix fails toward the known behavior: an inconclusive FUSE probe must not be
// treated as a positive match.
func TestFormatRARAuthorityRefusalKeepsChownGuidanceWhenProbeFails(t *testing.T) {
	dir := t.TempDir()
	previous := rarPOSIXFUSEProjectedOwnership
	rarPOSIXFUSEProjectedOwnership = func(string) (bool, error) { return false, errors.New("statfs unavailable") }
	t.Cleanup(func() { rarPOSIXFUSEProjectedOwnership = previous })

	err := formatRARAuthorityRefusal(dir)
	if err == nil || !strings.Contains(err.Error(), "chown") {
		t.Fatalf("err = %v, want the existing chown guidance when the FUSE probe itself fails", err)
	}
}
