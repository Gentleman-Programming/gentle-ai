package filemerge

import (
	"bytes"
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
	var errno syscall.Errno
	return errors.As(err, &errno) && errno == 1314 // ERROR_PRIVILEGE_NOT_HELD
}

func mustSymlink(t *testing.T, target, link string) {
	t.Helper()
	if err := os.Symlink(target, link); err != nil {
		if isSymlinkPrivilegeError(err) {
			t.Skipf("skipping: SeCreateSymbolicLinkPrivilege not held on this Windows build: %v", err)
		}
		t.Fatalf("Symlink(%q, %q) error = %v", target, link, err)
	}
}

func setTestHome(t *testing.T, home string) {
	t.Helper()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
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

func TestWriteFileAtomicPreservesExistingSymlink(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "target.txt")
	if err := os.WriteFile(target, []byte("old\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(target) error = %v", err)
	}
	path := filepath.Join(dir, "linked.txt")
	mustSymlink(t, target, path)

	result, err := WriteFileAtomic(path, []byte("new\n"), 0o644)
	if err != nil {
		t.Fatalf("WriteFileAtomic(symlink) error = %v, want success", err)
	}
	if !result.Changed || result.Created {
		t.Fatalf("WriteFileAtomic(symlink) result = %+v, want Changed=true Created=false", result)
	}
	info, err := os.Lstat(path)
	if err != nil {
		t.Fatalf("Lstat(symlink) error = %v", err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Fatal("WriteFileAtomic replaced symlink, want symlink preserved")
	}
	if got, readErr := os.ReadFile(target); readErr != nil || string(got) != "new\n" {
		t.Fatalf("target content = %q err=%v, want updated target", got, readErr)
	}
}

func TestWriteFileAtomicSymlinkNoopPreservesLink(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "target.txt")
	content := []byte("same\n")
	if err := os.WriteFile(target, content, 0o644); err != nil {
		t.Fatalf("WriteFile(target) error = %v", err)
	}
	path := filepath.Join(dir, "linked.txt")
	mustSymlink(t, target, path)

	result, err := WriteFileAtomic(path, content, 0o644)
	if err != nil {
		t.Fatalf("WriteFileAtomic(symlink noop) error = %v", err)
	}
	if result.Changed || result.Created {
		t.Fatalf("WriteFileAtomic(symlink noop) result = %+v, want no change", result)
	}
	info, err := os.Lstat(path)
	if err != nil {
		t.Fatalf("Lstat(symlink) error = %v", err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Fatal("WriteFileAtomic replaced symlink on noop write")
	}
}

func TestWriteFileAtomicCreatesDanglingSymlinkTarget(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "missing", "target.txt")
	path := filepath.Join(dir, "linked.txt")
	mustSymlink(t, target, path)

	content := []byte("created\n")
	result, err := WriteFileAtomic(path, content, 0o644)
	if err != nil {
		t.Fatalf("WriteFileAtomic(dangling symlink) error = %v, want success", err)
	}
	if !result.Changed || !result.Created {
		t.Fatalf("WriteFileAtomic(dangling symlink) result = %+v, want Changed=true Created=true", result)
	}
	info, err := os.Lstat(path)
	if err != nil {
		t.Fatalf("Lstat(symlink) error = %v", err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Fatal("WriteFileAtomic replaced dangling symlink, want symlink preserved")
	}
	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("ReadFile(target) error = %v", err)
	}
	if !bytes.Equal(got, content) {
		t.Fatalf("target content = %q, want %q", got, content)
	}
}

func TestWriteFileAtomicAllowsSymlinkWithinHome(t *testing.T) {
	home := t.TempDir()
	setTestHome(t, home)
	target := filepath.Join(home, ".dotfiles", "claude", "CLAUDE.md")
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatalf("MkdirAll(target dir) error = %v", err)
	}
	if err := os.WriteFile(target, []byte("old\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(target) error = %v", err)
	}
	path := filepath.Join(home, ".claude", "CLAUDE.md")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll(path dir) error = %v", err)
	}
	mustSymlink(t, target, path)

	if _, err := WriteFileAtomic(path, []byte("new\n"), 0o644); err != nil {
		t.Fatalf("WriteFileAtomic(home symlink) error = %v, want success", err)
	}
	if got, err := os.ReadFile(target); err != nil || string(got) != "new\n" {
		t.Fatalf("target content = %q err=%v, want updated target", got, err)
	}
}

func TestWriteFileAtomicCreatesNestedDanglingSymlinkTarget(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "linked.txt")
	mid := filepath.Join(dir, "mid-link.txt")
	target := filepath.Join(dir, "missing", "target.txt")
	mustSymlink(t, mid, path)
	mustSymlink(t, target, mid)

	content := []byte("created\n")
	if _, err := WriteFileAtomic(path, content, 0o644); err != nil {
		t.Fatalf("WriteFileAtomic(nested dangling symlink) error = %v, want success", err)
	}
	for _, link := range []string{path, mid} {
		info, err := os.Lstat(link)
		if err != nil {
			t.Fatalf("Lstat(%q) error = %v", link, err)
		}
		if info.Mode()&os.ModeSymlink == 0 {
			t.Fatalf("%q was replaced, want symlink preserved", link)
		}
	}
	if got, err := os.ReadFile(target); err != nil || !bytes.Equal(got, content) {
		t.Fatalf("target content = %q err=%v, want %q", got, err, content)
	}
}

func TestWriteFileAtomicRejectsSymlinkEscapes(t *testing.T) {
	base := t.TempDir()
	setTestHome(t, base)
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "target.txt"), []byte("outside\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(outside target) error = %v", err)
	}

	tests := []struct {
		name  string
		setup func(t *testing.T) string
	}{
		{
			name: "absolute target outside root",
			setup: func(t *testing.T) string {
				path := filepath.Join(base, "absolute-link.txt")
				mustSymlink(t, filepath.Join(outside, "target.txt"), path)
				return path
			},
		},
		{
			name: "relative target escapes root",
			setup: func(t *testing.T) string {
				managed := filepath.Join(base, "managed")
				if err := os.Mkdir(managed, 0o755); err != nil {
					t.Fatalf("Mkdir(managed) error = %v", err)
				}
				path := filepath.Join(managed, "relative-link.txt")
				mustSymlink(t, filepath.Join("..", "..", filepath.Base(outside), "target.txt"), path)
				return path
			},
		},
		{
			name: "chained target escapes root",
			setup: func(t *testing.T) string {
				mid := filepath.Join(base, "mid-link.txt")
				mustSymlink(t, filepath.Join(outside, "target.txt"), mid)
				path := filepath.Join(base, "chained-link.txt")
				mustSymlink(t, mid, path)
				return path
			},
		},
		{
			name: "dangling target escapes root",
			setup: func(t *testing.T) string {
				path := filepath.Join(base, "dangling-link.txt")
				mustSymlink(t, filepath.Join(outside, "missing.txt"), path)
				return path
			},
		},
		{
			name: "nested dangling target escapes root",
			setup: func(t *testing.T) string {
				mid := filepath.Join(base, "dangling-mid-link.txt")
				mustSymlink(t, filepath.Join(outside, "missing.txt"), mid)
				path := filepath.Join(base, "dangling-chain-link.txt")
				mustSymlink(t, mid, path)
				return path
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := WriteFileAtomic(tt.setup(t), []byte("new\n"), 0o644)
			if err == nil || !strings.Contains(err.Error(), "outside allowed root") {
				t.Fatalf("WriteFileAtomic() error = %v, want allowed-root rejection", err)
			}
		})
	}
}

func TestWriteFileAtomicRejectsSymlinkLoop(t *testing.T) {
	dir := t.TempDir()
	first := filepath.Join(dir, "first")
	second := filepath.Join(dir, "second")
	mustSymlink(t, second, first)
	mustSymlink(t, first, second)

	if _, err := WriteFileAtomic(first, []byte("new\n"), 0o644); err == nil {
		t.Fatal("WriteFileAtomic(loop) error = nil, want failure")
	}
}

func TestWriteFileAtomicRejectsEscapingSymlinkParentDirectory(t *testing.T) {
	base := t.TempDir()
	setTestHome(t, base)
	outside := t.TempDir()
	linkDir := filepath.Join(base, "linked")
	mustSymlink(t, outside, linkDir)

	_, err := WriteFileAtomic(filepath.Join(linkDir, "config.txt"), []byte("value\n"), 0o644)
	if err == nil || !strings.Contains(err.Error(), "outside allowed root") {
		t.Fatalf("WriteFileAtomic(parent escape) error = %v, want allowed-root rejection", err)
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

func TestWriteFileAtomicFollowsSymlinkParentDirectory(t *testing.T) {
	base := t.TempDir()
	realDir := filepath.Join(base, "real")
	if err := os.Mkdir(realDir, 0o755); err != nil {
		t.Fatalf("Mkdir(realDir) error = %v", err)
	}
	linkDir := filepath.Join(base, "linked")
	mustSymlink(t, realDir, linkDir)

	content := []byte("value\n")
	path := filepath.Join(linkDir, "config.txt")
	_, err := WriteFileAtomic(path, content, 0o644)
	if err != nil {
		t.Fatalf("WriteFileAtomic() via symlink parent error = %v, want success", err)
	}

	realPath := filepath.Join(realDir, "config.txt")
	got, readErr := os.ReadFile(realPath)
	if readErr != nil {
		t.Fatalf("ReadFile(realPath) error = %v", readErr)
	}
	if !bytes.Equal(got, content) {
		t.Fatalf("content = %q, want %q", got, content)
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
