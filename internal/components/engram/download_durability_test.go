package engram

import (
	"context"
	"errors"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// useTestClient points the package HTTP client at server for the test's
// duration.
func useTestClient(t *testing.T, server *httptest.Server) {
	t.Helper()
	original := engramHTTPClient
	t.Cleanup(func() { engramHTTPClient = original })
	engramHTTPClient = server.Client()
}

// TestEngramDownloadDigestDescribesTheFileOnDisk is the #1998 contract at this
// layer: the digest handed to checksum verification must describe the bytes at
// outPath. The fault-injection proof that a stream digest cannot answer for the
// destination lives with the writer, in
// filemerge.TestWriteStreamAtomicFailsWhenReplacementDoesNotLand.
func TestEngramDownloadDigestDescribesTheFileOnDisk(t *testing.T) {
	payload := []byte(strings.Repeat("release-archive-bytes", 4096))
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(payload)
	}))
	t.Cleanup(server.Close)
	useTestClient(t, server)

	outPath := filepath.Join(t.TempDir(), "nested", "engram.tar.gz")
	digest, err := engramDownloadToFile(context.Background(), server.URL, outPath)
	if err != nil {
		t.Fatalf("engramDownloadToFile: %v", err)
	}

	onDisk, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	if digest != sha256Hex(onDisk) {
		t.Errorf("digest = %s, want %s (sha256 of the bytes at %s)", digest, sha256Hex(onDisk), outPath)
	}
	if string(onDisk) != string(payload) {
		t.Error("on-disk content differs from the served payload")
	}
	// A download directory stays traversable. The generic writer creates parents
	// at 0700 for private config files; this path must not inherit that.
	parent, err := os.Stat(filepath.Dir(outPath))
	if err != nil {
		t.Fatalf("stat parent: %v", err)
	}
	if runtime.GOOS != "windows" && parent.Mode().Perm() != 0o755 {
		t.Errorf("parent directory mode = %v, want 0755", parent.Mode().Perm())
	}
}

// TestEngramDownloadLeavesNoArchiveWhenTheCopyFails is #1998's other half. The
// old implementation created outPath up front and returned on a copy error with
// the truncated bytes still there: a partial archive at the exact path the
// caller then hands to extraction.
func TestEngramDownloadLeavesNoArchiveWhenTheCopyFails(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Length", "4096")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("truncated"))
		w.(http.Flusher).Flush()
		// Abort mid-body so the client's copy fails after the response is
		// already open, standing in for a connection lost partway through a
		// release download.
		panic(http.ErrAbortHandler)
	}))
	t.Cleanup(server.Close)
	server.Config.ErrorLog = quietLogger()
	useTestClient(t, server)

	dir := t.TempDir()
	outPath := filepath.Join(dir, "engram.tar.gz")
	digest, err := engramDownloadToFile(context.Background(), server.URL, outPath)
	if err == nil {
		t.Fatalf("engramDownloadToFile reported success (digest %s) for a truncated body", digest)
	}
	if _, statErr := os.Stat(outPath); !errors.Is(statErr, os.ErrNotExist) {
		t.Errorf("partial archive survived at %s (stat err = %v)", outPath, statErr)
	}
	assertDirIsEmpty(t, dir)
}

// TestEngramDownloadLeavesNoArchiveOnHTTPFailure keeps the non-200 path clean
// too, so nothing extractable is left at the archive path.
func TestEngramDownloadLeavesNoArchiveOnHTTPFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	t.Cleanup(server.Close)
	useTestClient(t, server)

	dir := t.TempDir()
	outPath := filepath.Join(dir, "engram.tar.gz")
	if _, err := engramDownloadToFile(context.Background(), server.URL, outPath); err == nil {
		t.Fatal("engramDownloadToFile: want HTTP error, got nil")
	}
	assertDirIsEmpty(t, dir)
}

// TestEngramWriteExecutableLeavesNoBinaryWhenTheCopyFails covers the #2216 site:
// a failed extraction must not publish a partial binary over the installed one.
func TestEngramWriteExecutableLeavesNoBinaryWhenTheCopyFails(t *testing.T) {
	dir := t.TempDir()
	outPath := filepath.Join(dir, "engram")
	previous := []byte("the previously installed binary")
	if err := os.WriteFile(outPath, previous, 0o755); err != nil {
		t.Fatalf("seed: %v", err)
	}

	src := &failingReader{data: []byte("partial new binary"), err: errors.New("injected extraction failure")}
	if err := writeExecutable(src, outPath); err == nil {
		t.Fatal("writeExecutable: want the injected error, got nil")
	}

	onDisk, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	if string(onDisk) != string(previous) {
		t.Errorf("installed binary = %q, want the untouched %q", onDisk, previous)
	}
	assertOnlyEntry(t, dir, "engram")
}

// TestEngramWriteExecutablePublishesAnExecutable is the ordinary case: the
// binary lands with its executable mode intact.
func TestEngramWriteExecutablePublishesAnExecutable(t *testing.T) {
	dir := t.TempDir()
	outPath := filepath.Join(dir, "engram")
	content := []byte("#!/bin/sh\necho engram\n")

	if err := writeExecutable(strings.NewReader(string(content)), outPath); err != nil {
		t.Fatalf("writeExecutable: %v", err)
	}
	info, err := os.Stat(outPath)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm()&0o111 == 0 {
		t.Errorf("mode = %v, want the executable bits set", info.Mode().Perm())
	}
	onDisk, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	if string(onDisk) != string(content) {
		t.Errorf("content = %q, want %q", onDisk, content)
	}
	assertOnlyEntry(t, dir, "engram")
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
	n := copy(p, r.data)
	return n, nil
}

// quietLogger swallows the "abort handler" noise the truncated-body test
// deliberately provokes.
func quietLogger() *log.Logger { return log.New(io.Discard, "", 0) }

func assertDirIsEmpty(t *testing.T, dir string) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read dir: %v", err)
	}
	for _, entry := range entries {
		t.Errorf("unexpected leftover %q in %s", entry.Name(), dir)
	}
}

func assertOnlyEntry(t *testing.T, dir, name string) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read dir: %v", err)
	}
	for _, entry := range entries {
		if entry.Name() != name {
			t.Errorf("unexpected leftover %q in %s", entry.Name(), dir)
		}
	}
}
