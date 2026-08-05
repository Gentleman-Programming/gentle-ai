package upgrade

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func digestOf(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

type failingReader struct {
	data []byte
	err  error
	sent bool
}

func (r *failingReader) Read(p []byte) (int, error) {
	if r.sent {
		return 0, r.err
	}
	r.sent = true
	return copy(p, r.data), nil
}

func assertNoLeftovers(t *testing.T, dir string, allowed ...string) {
	t.Helper()
	keep := map[string]bool{}
	for _, name := range allowed {
		keep[name] = true
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read dir: %v", err)
	}
	for _, entry := range entries {
		if !keep[entry.Name()] {
			t.Errorf("unexpected leftover %q in %s", entry.Name(), dir)
		}
	}
}

// TestAtomicReplaceFailsWhenTheBinaryIsNotReplaced is the #2319 shape reduced to
// its Go core: a rename that reports success without taking effect. The old
// atomicReplace returned nil whenever os.Rename returned nil, which is how the
// updater reported an applied upgrade over an unchanged binary.
func TestAtomicReplaceFailsWhenTheBinaryIsNotReplaced(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "gentle-ai.new")
	dst := filepath.Join(dir, "gentle-ai")
	installed := []byte("the binary that is already installed")
	if err := os.WriteFile(src, []byte("the replacement binary"), 0o755); err != nil {
		t.Fatalf("write src: %v", err)
	}
	if err := os.WriteFile(dst, installed, 0o755); err != nil {
		t.Fatalf("write dst: %v", err)
	}

	original := renameFn
	t.Cleanup(func() { renameFn = original })
	renameFn = func(string, string) error { return nil }

	if err := atomicReplace(src, dst); err == nil {
		t.Fatal("atomicReplace reported success for a rename that never took effect")
	}

	onDisk, err := os.ReadFile(dst)
	if err != nil {
		t.Fatalf("read dst: %v", err)
	}
	if string(onDisk) != string(installed) {
		t.Errorf("installed binary = %q, want the untouched %q", onDisk, installed)
	}
}

// TestAtomicReplaceFailsWhenTheDestinationHoldsDifferentBytes covers the partial
// swap: the name moved but the content at the destination is not what was
// staged, so the upgrade did not happen.
func TestAtomicReplaceFailsWhenTheDestinationHoldsDifferentBytes(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "gentle-ai.new")
	dst := filepath.Join(dir, "gentle-ai")
	if err := os.WriteFile(src, []byte("the replacement binary"), 0o755); err != nil {
		t.Fatalf("write src: %v", err)
	}

	original := renameFn
	t.Cleanup(func() { renameFn = original })
	renameFn = func(oldPath, newPath string) error {
		if err := os.Rename(oldPath, newPath); err != nil {
			return err
		}
		return os.WriteFile(newPath, []byte("something else entirely"), 0o755)
	}

	if err := atomicReplace(src, dst); err == nil {
		t.Fatal("atomicReplace reported success for a destination that holds different bytes")
	}
}

// TestAtomicReplacePublishesTheStagedBinary is the ordinary case, kept so the
// read-back cannot be satisfied by refusing everything.
func TestAtomicReplacePublishesTheStagedBinary(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "gentle-ai.new")
	dst := filepath.Join(dir, "gentle-ai")
	replacement := []byte("the replacement binary")
	if err := os.WriteFile(src, replacement, 0o755); err != nil {
		t.Fatalf("write src: %v", err)
	}
	if err := os.WriteFile(dst, []byte("old"), 0o755); err != nil {
		t.Fatalf("write dst: %v", err)
	}

	if err := atomicReplace(src, dst); err != nil {
		t.Fatalf("atomicReplace: %v", err)
	}
	onDisk, err := os.ReadFile(dst)
	if err != nil {
		t.Fatalf("read dst: %v", err)
	}
	if string(onDisk) != string(replacement) {
		t.Errorf("dst = %q, want %q", onDisk, replacement)
	}
	assertNoLeftovers(t, dir, "gentle-ai")
}

// TestWriteExecutableLeavesNoBinaryWhenTheCopyFails is the #2216 self-update
// site. The old writeExecutable opened the destination with O_TRUNC and copied
// straight into it, so a failed extraction destroyed whatever was there and left
// a partial file behind.
func TestWriteExecutableLeavesNoBinaryWhenTheCopyFails(t *testing.T) {
	dir := t.TempDir()
	outPath := filepath.Join(dir, "gentle-ai.new")
	previous := []byte("a previously staged binary")
	if err := os.WriteFile(outPath, previous, 0o755); err != nil {
		t.Fatalf("seed: %v", err)
	}

	src := &failingReader{data: []byte("partial"), err: errors.New("injected extraction failure")}
	if err := writeExecutable(src, outPath); err == nil {
		t.Fatal("writeExecutable: want the injected error, got nil")
	}

	onDisk, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	if string(onDisk) != string(previous) {
		t.Errorf("destination = %q, want the untouched %q", onDisk, previous)
	}
	assertNoLeftovers(t, dir, "gentle-ai.new")
}

// TestWriteExecutablePublishesAnExecutable pins the happy path, including the
// mode the installer depends on.
func TestWriteExecutablePublishesAnExecutable(t *testing.T) {
	dir := t.TempDir()
	outPath := filepath.Join(dir, "nested", "gentle-ai.new")
	content := []byte("#!/bin/sh\necho gentle-ai\n")

	if err := writeExecutable(strings.NewReader(string(content)), outPath); err != nil {
		t.Fatalf("writeExecutable: %v", err)
	}
	info, err := os.Stat(outPath)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if info.Mode().Perm()&0o111 == 0 {
		t.Errorf("mode = %v, want the executable bits set", info.Mode().Perm())
	}
	onDisk, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	if string(onDisk) != string(content) {
		t.Errorf("content = %q, want %q", onDisk, content)
	}
	// An install directory on PATH stays traversable. The generic writer creates
	// parents at 0700 for private config files; this path must not inherit that.
	parent, err := os.Stat(filepath.Dir(outPath))
	if err != nil {
		t.Fatalf("stat parent: %v", err)
	}
	if parent.Mode().Perm() != 0o755 {
		t.Errorf("parent directory mode = %v, want 0755", parent.Mode().Perm())
	}
}

// TestDownloadToFileDigestDescribesTheFileOnDisk pins the release-verification
// contract: the digest compared against the signed manifest must describe the
// archive at outPath.
func TestDownloadToFileDigestDescribesTheFileOnDisk(t *testing.T) {
	payload := []byte(strings.Repeat("release-archive-bytes", 4096))
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(payload)
	}))
	t.Cleanup(server.Close)
	original := httpClient
	t.Cleanup(func() { httpClient = original })
	httpClient = server.Client()

	outPath := filepath.Join(t.TempDir(), "nested", "archive.tar.gz")
	digest, err := downloadToFile(context.Background(), server.URL, outPath, maxReleaseArchiveBytes)
	if err != nil {
		t.Fatalf("downloadToFile: %v", err)
	}
	onDisk, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	if digest != digestOf(onDisk) {
		t.Errorf("digest = %s, want %s (sha256 of the bytes at %s)", digest, digestOf(onDisk), outPath)
	}
}

// TestDownloadToFileRejectsAndRemovesOversizedArchives keeps the byte cap
// meaningful now that the writer publishes before the caller can measure: an
// over-limit archive must not survive at the path the caller then extracts.
func TestDownloadToFileRejectsAndRemovesOversizedArchives(t *testing.T) {
	payload := []byte(strings.Repeat("x", 4096))
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		// No Content-Length, so the declared-length precheck cannot catch it.
		w.(http.Flusher).Flush()
		_, _ = w.Write(payload)
	}))
	t.Cleanup(server.Close)
	original := httpClient
	t.Cleanup(func() { httpClient = original })
	httpClient = server.Client()

	dir := t.TempDir()
	outPath := filepath.Join(dir, "archive.tar.gz")
	if _, err := downloadToFile(context.Background(), server.URL, outPath, 128); err == nil {
		t.Fatal("downloadToFile: want the size-limit error, got nil")
	}
	assertNoLeftovers(t, dir)
}

// TestDownloadToFileRemovesPartialArchiveWhenTheBodyIsTruncated keeps a lost
// connection from leaving something extractable at the archive path.
func TestDownloadToFileRemovesPartialArchiveWhenTheBodyIsTruncated(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Length", "4096")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("truncated"))
		w.(http.Flusher).Flush()
		panic(http.ErrAbortHandler)
	}))
	t.Cleanup(server.Close)
	server.Config.ErrorLog = log.New(io.Discard, "", 0)
	original := httpClient
	t.Cleanup(func() { httpClient = original })
	httpClient = server.Client()

	dir := t.TempDir()
	outPath := filepath.Join(dir, "archive.tar.gz")
	if _, err := downloadToFile(context.Background(), server.URL, outPath, maxReleaseArchiveBytes); err == nil {
		t.Fatal("downloadToFile: want the truncated-body error, got nil")
	}
	assertNoLeftovers(t, dir)
}
