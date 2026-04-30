package filemerge

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"testing"
)

// isSymlinkPrivilegeError reports whether err is the Windows
// ERROR_PRIVILEGE_NOT_HELD (1314) error returned by os.Symlink when the
// process lacks SeCreateSymbolicLinkPrivilege. errors.Is does not map this
// errno to os.ErrPermission, so we unwrap and check the raw value.
func isSymlinkPrivilegeError(err error) bool {
	var le *os.LinkError
	if errors.As(err, &le) {
		var errno syscall.Errno
		if errors.As(le.Err, &errno) {
			return errno == 1314 // ERROR_PRIVILEGE_NOT_HELD
		}
	}
	return false
}

func TestWriteFileAtomicReadOnlyDirRelaxesOwnerWritePermission(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("chmod 555 semantics differ on Windows")
	}
	base := t.TempDir()
	skillDir := filepath.Join(base, "sdd-init")
	if err := os.Mkdir(skillDir, 0o555); err != nil {
		t.Fatalf("Mkdir() error = %v", err)
	}

	path := filepath.Join(skillDir, "SKILL.md")
	content := []byte("# SDD Init\n")

	_, err := WriteFileAtomic(path, content, 0o644)
	if err != nil {
		t.Fatalf("WriteFileAtomic() error = %v, want successful write with permission relaxation", err)
	}

	got, readErr := os.ReadFile(path)
	if readErr != nil {
		t.Fatalf("ReadFile() error = %v", readErr)
	}
	if string(got) != string(content) {
		t.Fatalf("file content = %q, want %q", string(got), string(content))
	}
}

func TestWriteFileAtomicCreatesAndIsIdempotent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nested", "config.json")
	content := []byte("{\"ok\":true}\n")

	first, err := WriteFileAtomic(path, content, 0o644)
	if err != nil {
		t.Fatalf("WriteFileAtomic() first write error = %v", err)
	}

	if !first.Changed || !first.Created {
		t.Fatalf("WriteFileAtomic() first write result = %+v", first)
	}

	second, err := WriteFileAtomic(path, content, 0o644)
	if err != nil {
		t.Fatalf("WriteFileAtomic() second write error = %v", err)
	}

	if second.Changed || second.Created {
		t.Fatalf("WriteFileAtomic() second write result = %+v", second)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}

	if string(got) != string(content) {
		t.Fatalf("file content = %q", string(got))
	}
}

func TestWriteFileAtomicRejectsExistingSymlink(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "target.txt")
	if err := os.WriteFile(target, []byte("old\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(target) error = %v", err)
	}
	path := filepath.Join(dir, "linked.txt")
	if err := os.Symlink(target, path); err != nil {
		// On Windows without Developer Mode or admin rights, symlink creation
		// requires SeCreateSymbolicLinkPrivilege (ERROR_PRIVILEGE_NOT_HELD = 1314).
		// Skip gracefully — the test infrastructure lacks the privilege, not the code.
		if isSymlinkPrivilegeError(err) {
			t.Skipf("skipping: SeCreateSymbolicLinkPrivilege not held on this Windows build: %v", err)
		}
		t.Fatalf("Symlink() error = %v", err)
	}

	_, err := WriteFileAtomic(path, []byte("new\n"), 0o644)
	if err == nil || err.Error() == "" {
		t.Fatalf("WriteFileAtomic(symlink) error = %v, want rejection", err)
	}
	if got, readErr := os.ReadFile(target); readErr != nil || string(got) != "old\n" {
		t.Fatalf("target content changed through symlink: got %q err=%v", got, readErr)
	}
}

func TestWriteFileAtomicRejectsOversizedExistingFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "big.txt")
	data := make([]byte, maxAtomicFileSize+1)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("WriteFile(big) error = %v", err)
	}

	_, err := WriteFileAtomic(path, []byte("small\n"), 0o644)
	if err == nil {
		t.Fatal("WriteFileAtomic(big) error = nil, want max-size rejection")
	}
}

func TestWriteFileAtomicRejectsSymlinkParentDirectory(t *testing.T) {
	base := t.TempDir()
	realDir := filepath.Join(base, "real")
	if err := os.Mkdir(realDir, 0o755); err != nil {
		t.Fatalf("Mkdir(realDir) error = %v", err)
	}
	linkDir := filepath.Join(base, "linked")
	if err := os.Symlink(realDir, linkDir); err != nil {
		// On Windows without Developer Mode or admin rights, symlink creation
		// requires SeCreateSymbolicLinkPrivilege (ERROR_PRIVILEGE_NOT_HELD = 1314).
		// Skip gracefully — the test infrastructure lacks the privilege, not the code.
		if isSymlinkPrivilegeError(err) {
			t.Skipf("skipping: SeCreateSymbolicLinkPrivilege not held on this Windows build: %v", err)
		}
		t.Fatalf("Symlink(linkDir) error = %v", err)
	}

	path := filepath.Join(linkDir, "config.txt")
	_, err := WriteFileAtomic(path, []byte("value\n"), 0o644)
	if err == nil {
		t.Fatal("WriteFileAtomic() error = nil, want symlink parent rejection")
	}
}

// TestWriteFileAtomicIgnoresPermissionErrorFromSyncDirOnWindows verifies that
// ErrPermission from syncDirFn is silently tolerated on Windows — NTFS returns
// ACCESS_DENIED when syncing a directory fd, which must not fail the write.
func TestWriteFileAtomicIgnoresPermissionErrorFromSyncDirOnWindows(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nested", "config.json")
	content := []byte("{\"ok\":true}\n")

	origGOOS := runtimeGOOS
	origSyncDir := syncDirFn
	t.Cleanup(func() {
		runtimeGOOS = origGOOS
		syncDirFn = origSyncDir
	})

	runtimeGOOS = func() string { return "windows" }
	syncDirFn = func(string) error { return os.ErrPermission }

	result, err := WriteFileAtomic(path, content, 0o644)
	if err != nil {
		t.Fatalf("WriteFileAtomic() error = %v, want nil on windows permission-denied dir sync", err)
	}
	if !result.Changed || !result.Created {
		t.Fatalf("WriteFileAtomic() result = %+v, want Changed=true Created=true", result)
	}
	got, readErr := os.ReadFile(path)
	if readErr != nil {
		t.Fatalf("ReadFile() error = %v", readErr)
	}
	if string(got) != string(content) {
		t.Fatalf("file content = %q, want %q", string(got), string(content))
	}
}

// TestWriteFileAtomicPropagatesSyncDirErrorOnUnix verifies that any syncDirFn
// error is propagated on non-Windows platforms — no silent swallowing.
func TestWriteFileAtomicPropagatesSyncDirErrorOnUnix(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nested", "config.json")
	content := []byte("{\"ok\":true}\n")

	origGOOS := runtimeGOOS
	origSyncDir := syncDirFn
	t.Cleanup(func() {
		runtimeGOOS = origGOOS
		syncDirFn = origSyncDir
	})

	runtimeGOOS = func() string { return "linux" }
	syncDirFn = func(string) error { return os.ErrPermission }

	_, err := WriteFileAtomic(path, content, 0o644)
	if err == nil {
		t.Fatal("WriteFileAtomic() error = nil, want sync parent directory failure on unix")
	}
	if !strings.Contains(err.Error(), "sync parent directory") {
		t.Fatalf("WriteFileAtomic() error = %v, want sync parent directory context", err)
	}
}

// TestWriteFileAtomicPropagatesUnexpectedSyncDirErrorOnWindows verifies that
// non-ErrPermission errors from syncDirFn are still propagated on Windows —
// only the specific NTFS directory-sync permission error is tolerated.
func TestWriteFileAtomicPropagatesUnexpectedSyncDirErrorOnWindows(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nested", "config.json")
	content := []byte("{\"ok\":true}\n")
	boom := errors.New("boom")

	origGOOS := runtimeGOOS
	origSyncDir := syncDirFn
	t.Cleanup(func() {
		runtimeGOOS = origGOOS
		syncDirFn = origSyncDir
	})

	runtimeGOOS = func() string { return "windows" }
	syncDirFn = func(string) error { return boom }

	_, err := WriteFileAtomic(path, content, 0o644)
	if err == nil {
		t.Fatal("WriteFileAtomic() error = nil, want unexpected sync dir error on windows")
	}
	if !errors.Is(err, boom) {
		t.Fatalf("WriteFileAtomic() error = %v, want wrapped boom", err)
	}
}
