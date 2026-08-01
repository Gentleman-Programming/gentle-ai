package mutationjournal

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/components/filemerge"
)

func TestNewRejectsEmptyRoots(t *testing.T) {
	j := New()
	dir := t.TempDir()
	target := filepath.Join(dir, "x.txt")
	if err := j.Capture(target); err == nil || !strings.Contains(err.Error(), "outside mutation journal roots") {
		t.Fatalf("Capture() empty-roots error = %v, want substring %q", err, "outside mutation journal roots")
	}
	if _, err := j.Write(target, []byte("x")); err == nil || !strings.Contains(err.Error(), "outside mutation journal roots") {
		t.Fatalf("Write() empty-roots error = %v, want substring %q", err, "outside mutation journal roots")
	}
}

func TestCaptureBeforeWrite(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "x.txt")
	if err := os.WriteFile(target, []byte("original"), 0o600); err != nil {
		t.Fatalf("seed write error = %v", err)
	}
	j := New(dir)
	if err := j.Capture(target); err != nil {
		t.Fatalf("Capture() error = %v", err)
	}
	if _, err := j.Write(target, []byte("new")); err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("ReadFile() after Write error = %v", err)
	}
	if string(got) != "new" {
		t.Fatalf("after Write, file = %q, want %q", string(got), "new")
	}
	if err := j.Restore(); err != nil {
		t.Fatalf("Restore() error = %v", err)
	}
	got, err = os.ReadFile(target)
	if err != nil {
		t.Fatalf("ReadFile() after Restore error = %v", err)
	}
	if string(got) != "original" {
		t.Fatalf("after Restore, file = %q, want %q", string(got), "original")
	}
}

func TestCaptureNonexistent(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "missing.txt")
	j := New(dir)
	if err := j.Capture(target); err != nil {
		t.Fatalf("Capture() error = %v", err)
	}
	if _, err := j.Write(target, []byte("created")); err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	if _, err := os.Stat(target); err != nil {
		t.Fatalf("Stat() after Write error = %v", err)
	}
	if err := j.Restore(); err != nil {
		t.Fatalf("Restore() error = %v", err)
	}
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Fatalf("Stat() after Restore error = %v, want IsNotExist", err)
	}
}

func TestWriteAutoCaptures(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "auto.txt")
	if err := os.WriteFile(target, []byte("first"), 0o600); err != nil {
		t.Fatalf("seed write error = %v", err)
	}
	j := New(dir)
	if _, err := j.Write(target, []byte("second")); err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	if err := j.Restore(); err != nil {
		t.Fatalf("Restore() error = %v", err)
	}
	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if string(got) != "first" {
		t.Fatalf("after Restore, file = %q, want %q", string(got), "first")
	}
}

func TestRefusesSymlink(t *testing.T) {
	dir := t.TempDir()
	real := filepath.Join(dir, "real.txt")
	if err := os.WriteFile(real, []byte("body"), 0o600); err != nil {
		t.Fatalf("seed write error = %v", err)
	}
	link := filepath.Join(dir, "link.txt")
	if err := os.Symlink(real, link); err != nil {
		t.Skipf("symlink unsupported on this platform: %v", err)
	}
	j := New(dir)
	if err := j.Capture(link); err == nil || !strings.Contains(err.Error(), "refuse symlink") {
		t.Fatalf("Capture() symlink error = %v, want substring %q", err, "refuse symlink")
	}
	if _, err := j.Write(link, []byte("x")); err == nil || !strings.Contains(err.Error(), "refuse symlink") {
		t.Fatalf("Write() symlink error = %v, want substring %q", err, "refuse symlink")
	}
}

func TestRefusesPathOutsideRoots(t *testing.T) {
	base := t.TempDir()
	outside := filepath.Join(t.TempDir(), "escaped.txt")
	j := New(base)
	if err := j.Capture(outside); err == nil || !strings.Contains(err.Error(), "outside mutation journal roots") {
		t.Fatalf("Capture() outside-root error = %v, want substring %q", err, "outside mutation journal roots")
	}
	if _, err := j.Write(outside, []byte("x")); err == nil || !strings.Contains(err.Error(), "outside mutation journal roots") {
		t.Fatalf("Write() outside-root error = %v, want substring %q", err, "outside mutation journal roots")
	}
}

func TestRefusesEmptyPath(t *testing.T) {
	dir := t.TempDir()
	j := New(dir)
	if err := j.Capture(""); err == nil || !strings.Contains(err.Error(), "empty mutation journal path") {
		t.Fatalf("Capture(\"\") error = %v, want substring %q", err, "empty mutation journal path")
	}
	if _, err := j.Write("", []byte("x")); err == nil || !strings.Contains(err.Error(), "empty mutation journal path") {
		t.Fatalf("Write(\"\") error = %v, want substring %q", err, "empty mutation journal path")
	}
}

func TestRestoreMultipleFilesDeterministic(t *testing.T) {
	base := t.TempDir()
	j := New(base)
	names := []string{"c.txt", "a.txt", "b.txt", "e.txt", "d.txt"}
	for _, name := range names {
		p := filepath.Join(base, name)
		if err := os.WriteFile(p, []byte("orig-"+name), 0o600); err != nil {
			t.Fatalf("seed %q: %v", name, err)
		}
		if err := j.Capture(p); err != nil {
			t.Fatalf("capture %q: %v", name, err)
		}
		if _, err := j.WriteWithMode(p, []byte("new-"+name), 0o600); err != nil {
			t.Fatalf("write %q: %v", name, err)
		}
	}
	originalWrite := writeFileAtomic
	var restored []string
	writeFileAtomic = func(path string, _ []byte, _ os.FileMode) (filemerge.WriteResult, error) {
		restored = append(restored, filepath.Base(path))
		return filemerge.WriteResult{}, errors.New("injected restore failure")
	}
	t.Cleanup(func() { writeFileAtomic = originalWrite })
	err := j.Restore()
	if err == nil {
		t.Fatalf("Restore() error = nil, want non-nil")
	}
	msg := err.Error()
	prev := -1
	ordered := []string{"a.txt", "b.txt", "c.txt", "d.txt", "e.txt"}
	for _, name := range ordered {
		idx := strings.Index(msg, name)
		if idx < 0 {
			t.Fatalf("Restore() error = %q, want mention of %q", msg, name)
		}
		if idx <= prev {
			t.Fatalf("Restore() error = %q, want %q (idx %d) after previous name (idx %d)", msg, name, idx, prev)
		}
		prev = idx
	}
	if strings.Join(restored, ",") != strings.Join(ordered, ",") {
		t.Fatalf("restore call order = %v, want %v", restored, ordered)
	}
}

func TestRestoreAggregatesErrorsWithJoin(t *testing.T) {
	base := t.TempDir()
	j := New(base)
	for _, name := range []string{"a.txt", "b.txt", "c.txt"} {
		p := filepath.Join(base, name)
		if err := os.WriteFile(p, []byte("orig-"+name), 0o600); err != nil {
			t.Fatalf("seed %q: %v", name, err)
		}
		if err := j.Capture(p); err != nil {
			t.Fatalf("capture %q: %v", name, err)
		}
		if _, err := j.WriteWithMode(p, []byte("new-"+name), 0o600); err != nil {
			t.Fatalf("write %q: %v", name, err)
		}
	}
	originalWrite := writeFileAtomic
	writeFileAtomic = func(path string, _ []byte, _ os.FileMode) (filemerge.WriteResult, error) {
		return filemerge.WriteResult{}, fmt.Errorf("injected restore failure for %s", filepath.Base(path))
	}
	t.Cleanup(func() { writeFileAtomic = originalWrite })
	err := j.Restore()
	if err == nil {
		t.Fatalf("Restore() error = nil, want non-nil")
	}
	type unwrapMulti interface{ Unwrap() []error }
	joined, ok := err.(unwrapMulti)
	if !ok {
		t.Fatalf("Restore() error type = %T, want errors.Join-style multi-error", err)
	}
	causes := joined.Unwrap()
	if len(causes) != 3 {
		t.Fatalf("Restore() joined causes count = %d, want 3", len(causes))
	}
	seen := map[string]bool{}
	for i, cause := range causes {
		key := cause.Error()
		if key == "" {
			t.Fatalf("Restore() causes[%d] error string empty", i)
		}
		if seen[key] {
			t.Fatalf("Restore() causes[%d] = %q, duplicate", i, key)
		}
		seen[key] = true
	}
}

func TestOwnedFileBeforeAfterHash(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "hello.txt")
	if err := os.WriteFile(target, []byte("hello"), 0o600); err != nil {
		t.Fatalf("seed: %v", err)
	}
	j := New(dir)
	if err := j.Capture(target); err != nil {
		t.Fatalf("capture: %v", err)
	}
	of := j.OwnedFile(target, "world", false, false)
	if of.Before == nil {
		t.Fatalf("Before = nil, want pointer to %q", "hello")
	}
	if *of.Before != "hello" {
		t.Fatalf("Before = %q, want %q", *of.Before, "hello")
	}
	if of.After != "world" {
		t.Fatalf("After = %q, want %q", of.After, "world")
	}
	want := sha256Hex("world")
	if of.AfterHash != want {
		t.Fatalf("AfterHash = %q, want %q", of.AfterHash, want)
	}
}

func TestWriteRejectsChangesAfterCapture(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "idempotent.txt")
	if err := os.WriteFile(target, []byte("first"), 0o600); err != nil {
		t.Fatalf("seed: %v", err)
	}
	j := New(dir)
	if err := j.Capture(target); err != nil {
		t.Fatalf("first Capture: %v", err)
	}
	if err := os.WriteFile(target, []byte("second"), 0o600); err != nil {
		t.Fatalf("rewrite: %v", err)
	}
	if err := j.Capture(target); err != nil {
		t.Fatalf("second Capture: %v", err)
	}
	if _, err := j.WriteWithMode(target, []byte("third"), 0o600); err == nil || !strings.Contains(err.Error(), "changed after capture") {
		t.Fatalf("WriteWithMode error = %v, want capture conflict", err)
	}
	if err := j.Restore(); err != nil {
		t.Fatalf("Restore: %v", err)
	}
	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(got) != "second" {
		t.Fatalf("after rejected Write, file = %q, want concurrent content %q", string(got), "second")
	}
}

func TestRefusesSymlinkedAncestorOutsideRoot(t *testing.T) {
	home := t.TempDir()
	outside := t.TempDir()
	link := filepath.Join(home, "linked")
	if err := os.Symlink(outside, link); err != nil {
		t.Skipf("symlink unsupported on this platform: %v", err)
	}
	j := New(home)
	err := j.Capture(filepath.Join(link, "target.json"))
	if err == nil || !strings.Contains(err.Error(), "outside mutation journal roots") {
		t.Fatalf("Capture() error = %v, want resolved outside-root rejection", err)
	}
}

func TestWriteWithModePreservesExistingMode(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX permission bits are not enforced on Windows")
	}
	dir := t.TempDir()
	target := filepath.Join(dir, "mode.txt")
	if err := os.WriteFile(target, []byte("x"), 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}
	j := New(dir)
	if _, err := j.WriteWithMode(target, []byte("y"), 0o600); err != nil {
		t.Fatalf("WriteWithMode: %v", err)
	}
	info, err := os.Stat(target)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if got := info.Mode().Perm(); got != 0o644 {
		t.Fatalf("mode = %o, want 0o644 (preserve existing)", got)
	}
}

func TestWriteWithModeAppliesToNewFile(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX permission bits are not enforced on Windows")
	}
	dir := t.TempDir()
	target := filepath.Join(dir, "newmode.txt")
	j := New(dir)
	if _, err := j.WriteWithMode(target, []byte("y"), 0o755); err != nil {
		t.Fatalf("WriteWithMode: %v", err)
	}
	info, err := os.Stat(target)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if got := info.Mode().Perm(); got != 0o755 {
		t.Fatalf("mode = %o, want 0o755", got)
	}
}

func TestHashMatchesSha256(t *testing.T) {
	j := New(t.TempDir())
	of := j.OwnedFile(filepath.Join(t.TempDir(), "never-used.txt"), "hello world", false, false)
	want := sha256Hex("hello world")
	if of.AfterHash != want {
		t.Fatalf("AfterHash = %q, want %q", of.AfterHash, want)
	}
}

func sha256Hex(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

func TestRestoreRecoversFromWriteErrorAfterAtomicReplacement(t *testing.T) {
	origWriteFileAtomic := writeFileAtomic
	t.Cleanup(func() {
		writeFileAtomic = origWriteFileAtomic
	})

	dir := t.TempDir()
	target := filepath.Join(dir, "atomic_fail.txt")
	if err := os.WriteFile(target, []byte("before-content"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	j := New(dir)
	if err := j.Capture(target); err != nil {
		t.Fatalf("Capture: %v", err)
	}

	// Stub writeFileAtomic to write the new bytes to disk (simulating os.Rename
	// succeeding), but return a parent-directory sync error on the first write.
	calls := 0
	writeFileAtomic = func(path string, data []byte, perm os.FileMode) (filemerge.WriteResult, error) {
		calls++
		if calls == 1 {
			if err := os.WriteFile(path, data, perm); err != nil {
				return filemerge.WriteResult{}, err
			}
			return filemerge.WriteResult{Changed: true, Replaced: true}, errors.New("sync parent directory: injected error")
		}
		return origWriteFileAtomic(path, data, perm)
	}

	_, err := j.Write(target, []byte("replacement-content"))
	if err == nil {
		t.Fatal("Write expected error, got nil")
	}

	// Verify that replacement content is currently on disk despite the Write error
	onDisk, readErr := os.ReadFile(target)
	if readErr != nil || string(onDisk) != "replacement-content" {
		t.Fatalf("file on disk = %q, want replacement-content", string(onDisk))
	}

	// Restore must recognize that disk state changed and revert it to before-content
	if restoreErr := j.Restore(); restoreErr != nil {
		t.Fatalf("Restore error: %v", restoreErr)
	}

	restored, readErr := os.ReadFile(target)
	if readErr != nil || string(restored) != "before-content" {
		t.Fatalf("file on disk after Restore = %q, want before-content", string(restored))
	}
	if runtime.GOOS != "windows" {
		info, statErr := os.Stat(target)
		if statErr != nil {
			t.Fatalf("Stat after Restore: %v", statErr)
		}
		if got := info.Mode().Perm(); got != 0o600 {
			t.Fatalf("file mode after Restore = %o, want 0600", got)
		}
	}
}

func TestRestoreRemovesNewFileAfterWriteErrorAfterAtomicReplacement(t *testing.T) {
	originalWriter := writeFileAtomic
	t.Cleanup(func() { writeFileAtomic = originalWriter })

	dir := t.TempDir()
	target := filepath.Join(dir, "new-file.txt")
	j := New(dir)

	writeFileAtomic = func(path string, data []byte, mode os.FileMode) (filemerge.WriteResult, error) {
		if err := os.WriteFile(path, data, mode); err != nil {
			return filemerge.WriteResult{}, err
		}
		return filemerge.WriteResult{Changed: true, Created: true, Replaced: true},
			errors.New("sync parent directory: injected error")
	}

	if _, err := j.WriteWithMode(target, []byte("created"), 0o640); err == nil {
		t.Fatal("WriteWithMode expected injected failure")
	}
	if got, err := os.ReadFile(target); err != nil || string(got) != "created" {
		t.Fatalf("file after WriteWithMode = %q, err = %v, want created", got, err)
	}
	if err := j.Restore(); err != nil {
		t.Fatalf("Restore: %v", err)
	}
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Fatalf("Stat after Restore error = %v, want IsNotExist", err)
	}
}

func TestRestorePreservesConcurrentEditAfterWriteErrorAfterAtomicReplacement(t *testing.T) {
	originalWriter := writeFileAtomic
	t.Cleanup(func() { writeFileAtomic = originalWriter })

	dir := t.TempDir()
	target := filepath.Join(dir, "settings.json")
	if err := os.WriteFile(target, []byte("before"), 0o644); err != nil {
		t.Fatal(err)
	}

	j := New(dir)
	writeFileAtomic = func(path string, data []byte, mode os.FileMode) (filemerge.WriteResult, error) {
		if err := os.WriteFile(path, data, mode); err != nil {
			return filemerge.WriteResult{}, err
		}
		return filemerge.WriteResult{Changed: true, Replaced: true},
			errors.New("sync parent directory: injected error")
	}

	if _, err := j.Write(target, []byte("candidate")); err == nil {
		t.Fatal("Write expected injected failure")
	}
	if err := os.WriteFile(target, []byte("concurrent"), 0o644); err != nil {
		t.Fatal(err)
	}

	err := j.Restore()
	if err == nil || !strings.Contains(err.Error(), "mutation journal conflict") {
		t.Fatalf("Restore error = %v, want mutation journal conflict", err)
	}
	got, readErr := os.ReadFile(target)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if string(got) != "concurrent" {
		t.Fatalf("Restore overwrote concurrent edit: got %q, want %q", got, "concurrent")
	}
}

func TestWriteErrorBeforeReplacementPreservesConcurrentEdit(t *testing.T) {
	originalWriter := writeFileAtomic
	t.Cleanup(func() { writeFileAtomic = originalWriter })

	dir := t.TempDir()
	target := filepath.Join(dir, "settings.json")
	if err := os.WriteFile(target, []byte("before"), 0o644); err != nil {
		t.Fatal(err)
	}

	j := New(dir)
	if err := j.Capture(target); err != nil {
		t.Fatal(err)
	}

	startConcurrentWrite := make(chan struct{})
	concurrentWriteDone := make(chan error, 1)
	go func() {
		<-startConcurrentWrite
		concurrentWriteDone <- os.WriteFile(target, []byte("concurrent"), 0o644)
	}()

	calls := 0
	writeFileAtomic = func(path string, data []byte, mode os.FileMode) (filemerge.WriteResult, error) {
		calls++
		if calls > 1 {
			return originalWriter(path, data, mode)
		}
		close(startConcurrentWrite)
		if err := <-concurrentWriteDone; err != nil {
			t.Fatalf("concurrent write: %v", err)
		}
		return filemerge.WriteResult{}, errors.New("create temp file: injected pre-replacement failure")
	}

	if _, err := j.Write(target, []byte("candidate")); err == nil {
		t.Fatal("Write expected injected failure")
	}
	if err := j.Restore(); err != nil {
		t.Fatalf("Restore: %v", err)
	}

	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "concurrent" {
		t.Fatalf("Restore overwrote unrelated concurrent edit: got %q, want %q", got, "concurrent")
	}
}
