package mutationjournal

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
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
	if runtime.GOOS == "windows" {
		t.Skip("chmod read-only semantics differ on Windows")
	}
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
	if err := os.Chmod(base, 0o500); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(base, 0o700) })
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
}

func TestRestoreAggregatesErrorsWithJoin(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("chmod read-only semantics differ on Windows")
	}
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
	if err := os.Chmod(base, 0o500); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(base, 0o700) })
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

func TestCaptureIdempotent(t *testing.T) {
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
	if _, err := j.WriteWithMode(target, []byte("third"), 0o600); err != nil {
		t.Fatalf("WriteWithMode: %v", err)
	}
	if err := j.Restore(); err != nil {
		t.Fatalf("Restore: %v", err)
	}
	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(got) != "first" {
		t.Fatalf("after Restore, file = %q, want %q (idempotent Capture preserved first before-image)", string(got), "first")
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