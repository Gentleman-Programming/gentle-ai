package backup

import (
	"archive/tar"
	"compress/gzip"
	"io"
	"os"
	"path/filepath"
	"testing"
)

// TestCreateArchive_CreatesValidTarGz verifies that CreateArchive produces a
// valid tar.gz file at the given path that contains the expected entries.
func TestCreateArchive_CreatesValidTarGz(t *testing.T) {
	dir := t.TempDir()

	// Create two source files.
	srcA := filepath.Join(dir, "a.txt")
	srcB := filepath.Join(dir, "b.txt")
	if err := os.WriteFile(srcA, []byte("hello"), 0o644); err != nil {
		t.Fatalf("WriteFile a.txt: %v", err)
	}
	if err := os.WriteFile(srcB, []byte("world"), 0o644); err != nil {
		t.Fatalf("WriteFile b.txt: %v", err)
	}

	archivePath := filepath.Join(dir, "snapshot.tar.gz")
	entries := []ArchiveEntry{
		{RelPath: "files/a.txt", SourcePath: srcA, Mode: 0o644},
		{RelPath: "files/b.txt", SourcePath: srcB, Mode: 0o644},
	}

	if err := CreateArchive(archivePath, entries); err != nil {
		t.Fatalf("CreateArchive() error = %v", err)
	}

	// Archive must exist.
	if _, err := os.Stat(archivePath); err != nil {
		t.Fatalf("snapshot.tar.gz not created: %v", err)
	}

	// Open and verify the archive contains both entries.
	found := openAndListTar(t, archivePath)
	if _, ok := found["files/a.txt"]; !ok {
		t.Errorf("archive does not contain files/a.txt; got %v", found)
	}
	if _, ok := found["files/b.txt"]; !ok {
		t.Errorf("archive does not contain files/b.txt; got %v", found)
	}
}

// TestCreateArchive_PreservesPermissions verifies that file modes are preserved
// inside the archive header.
func TestCreateArchive_PreservesPermissions(t *testing.T) {
	dir := t.TempDir()

	src := filepath.Join(dir, "exec.sh")
	if err := os.WriteFile(src, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatalf("WriteFile exec.sh: %v", err)
	}

	archivePath := filepath.Join(dir, "snapshot.tar.gz")
	entries := []ArchiveEntry{
		{RelPath: "files/exec.sh", SourcePath: src, Mode: 0o755},
	}

	if err := CreateArchive(archivePath, entries); err != nil {
		t.Fatalf("CreateArchive() error = %v", err)
	}

	found := openAndListTar(t, archivePath)
	hdr, ok := found["files/exec.sh"]
	if !ok {
		t.Fatalf("archive does not contain files/exec.sh")
	}
	// Tar header mode should include execute bits.
	if hdr.Mode&0o111 == 0 {
		t.Errorf("exec.sh mode = %04o, want execute bits set (e.g. 0755)", hdr.Mode)
	}
}

// TestExtractArchive_RoundTrip verifies that files written by CreateArchive can
// be extracted by ExtractArchive with matching content.
func TestExtractArchive_RoundTrip(t *testing.T) {
	dir := t.TempDir()

	srcA := filepath.Join(dir, "hello.txt")
	srcB := filepath.Join(dir, "data.json")
	if err := os.WriteFile(srcA, []byte("hello world"), 0o644); err != nil {
		t.Fatalf("WriteFile hello.txt: %v", err)
	}
	if err := os.WriteFile(srcB, []byte(`{"key":"value"}`), 0o644); err != nil {
		t.Fatalf("WriteFile data.json: %v", err)
	}

	archivePath := filepath.Join(dir, "snapshot.tar.gz")
	entries := []ArchiveEntry{
		{RelPath: "files/hello.txt", SourcePath: srcA, Mode: 0o644},
		{RelPath: "files/data.json", SourcePath: srcB, Mode: 0o644},
	}

	if err := CreateArchive(archivePath, entries); err != nil {
		t.Fatalf("CreateArchive() error = %v", err)
	}

	destDir := filepath.Join(dir, "extracted")
	extracted, err := ExtractArchive(archivePath, destDir)
	if err != nil {
		t.Fatalf("ExtractArchive() error = %v", err)
	}

	// Must return 2 extracted entries.
	if len(extracted) != 2 {
		t.Fatalf("ExtractArchive() returned %d entries, want 2", len(extracted))
	}

	// Build a map from RelPath → SourcePath for easier lookup.
	byRel := make(map[string]string, len(extracted))
	for _, e := range extracted {
		byRel[e.RelPath] = e.SourcePath
	}

	// Verify hello.txt content.
	checkExtractedContent(t, byRel, "files/hello.txt", "hello world")
	// Verify data.json content.
	checkExtractedContent(t, byRel, "files/data.json", `{"key":"value"}`)
}

// TestExtractArchive_RejectsPathTraversal verifies that archive entries whose
// relative path contains ".." are rejected by ExtractArchive.
func TestExtractArchive_RejectsPathTraversal(t *testing.T) {
	dir := t.TempDir()

	// Manually craft a tar.gz with a ".." path traversal entry.
	archivePath := filepath.Join(dir, "evil.tar.gz")
	func() {
		f, err := os.Create(archivePath)
		if err != nil {
			t.Fatalf("Create evil.tar.gz: %v", err)
		}
		defer f.Close()

		gw := gzip.NewWriter(f)
		defer gw.Close()

		tw := tar.NewWriter(gw)
		defer tw.Close()

		content := []byte("evil content")
		hdr := &tar.Header{
			Name: "../escaped.txt",
			Mode: 0o644,
			Size: int64(len(content)),
		}
		if err := tw.WriteHeader(hdr); err != nil {
			t.Fatalf("WriteHeader: %v", err)
		}
		if _, err := tw.Write(content); err != nil {
			t.Fatalf("Write content: %v", err)
		}
	}()

	destDir := filepath.Join(dir, "dest")
	_, err := ExtractArchive(archivePath, destDir)
	if err == nil {
		t.Fatal("ExtractArchive() should return error for path traversal entry (..)")
	}
}

// TestExtractArchive_SkipsSymlinkEntries verifies that archive entries with
// TypeSymlink are silently skipped and not created on disk.
func TestExtractArchive_SkipsSymlinkEntries(t *testing.T) {
	dir := t.TempDir()

	archivePath := filepath.Join(dir, "with-symlink.tar.gz")
	func() {
		f, err := os.Create(archivePath)
		if err != nil {
			t.Fatalf("Create archive: %v", err)
		}
		defer f.Close()

		gw := gzip.NewWriter(f)
		defer gw.Close()

		tw := tar.NewWriter(gw)
		defer tw.Close()

		// Add a symlink entry — should be skipped.
		symlinkHdr := &tar.Header{
			Typeflag: tar.TypeSymlink,
			Name:     "evil-link",
			Linkname: "/etc/passwd",
			Mode:     0o777,
		}
		if err := tw.WriteHeader(symlinkHdr); err != nil {
			t.Fatalf("WriteHeader symlink: %v", err)
		}

		// Add a real regular file so extraction succeeds.
		content := []byte("safe content")
		regHdr := &tar.Header{
			Typeflag: tar.TypeReg,
			Name:     "safe.txt",
			Mode:     0o644,
			Size:     int64(len(content)),
		}
		if err := tw.WriteHeader(regHdr); err != nil {
			t.Fatalf("WriteHeader regular: %v", err)
		}
		if _, err := tw.Write(content); err != nil {
			t.Fatalf("Write content: %v", err)
		}
	}()

	destDir := filepath.Join(dir, "dest")
	extracted, err := ExtractArchive(archivePath, destDir)
	if err != nil {
		t.Fatalf("ExtractArchive() error = %v", err)
	}

	// Only the regular file should be extracted.
	if len(extracted) != 1 {
		t.Fatalf("ExtractArchive() returned %d entries, want 1 (symlink must be skipped)", len(extracted))
	}
	if extracted[0].RelPath != "safe.txt" {
		t.Errorf("extracted entry = %q, want %q", extracted[0].RelPath, "safe.txt")
	}

	// The symlink must NOT exist on disk.
	symlinkOnDisk := filepath.Join(destDir, "evil-link")
	if _, statErr := os.Lstat(symlinkOnDisk); statErr == nil {
		t.Errorf("symlink entry was created on disk at %q, but should have been skipped", symlinkOnDisk)
	}
}

// TestExtractArchive_RejectsDotEntry verifies that an archive entry named "."
// is rejected by ExtractArchive because it resolves to the destination directory
// itself rather than a file inside it.
func TestExtractArchive_RejectsDotEntry(t *testing.T) {
	dir := t.TempDir()

	archivePath := filepath.Join(dir, "dot-entry.tar.gz")
	func() {
		f, err := os.Create(archivePath)
		if err != nil {
			t.Fatalf("Create dot-entry.tar.gz: %v", err)
		}
		defer f.Close()

		gw := gzip.NewWriter(f)
		defer gw.Close()

		tw := tar.NewWriter(gw)
		defer tw.Close()

		content := []byte("dot content")
		hdr := &tar.Header{
			Typeflag: tar.TypeReg,
			Name:     ".",
			Mode:     0o644,
			Size:     int64(len(content)),
		}
		if err := tw.WriteHeader(hdr); err != nil {
			t.Fatalf("WriteHeader: %v", err)
		}
		if _, err := tw.Write(content); err != nil {
			t.Fatalf("Write content: %v", err)
		}
	}()

	destDir := filepath.Join(dir, "dest")
	_, err := ExtractArchive(archivePath, destDir)
	if err == nil {
		t.Fatal("ExtractArchive() should return error for '.' entry")
	}
}

// TestExtractArchive_HandlesTypeRegA verifies that archive entries with Typeflag
// set to TypeRegA (legacy '\x00') are extracted as regular files, matching the
// behaviour of TypeReg entries.
func TestExtractArchive_HandlesTypeRegA(t *testing.T) {
	dir := t.TempDir()

	archivePath := filepath.Join(dir, "rega.tar.gz")
	content := []byte("legacy regular file content")

	func() {
		f, err := os.Create(archivePath)
		if err != nil {
			t.Fatalf("Create archive: %v", err)
		}
		defer f.Close()

		gw := gzip.NewWriter(f)
		defer gw.Close()

		tw := tar.NewWriter(gw)
		defer tw.Close()

		hdr := &tar.Header{
			Typeflag: tar.TypeRegA, // legacy '\x00'
			Name:     "files/legacy.txt",
			Mode:     0o644,
			Size:     int64(len(content)),
		}
		if err := tw.WriteHeader(hdr); err != nil {
			t.Fatalf("WriteHeader: %v", err)
		}
		if _, err := tw.Write(content); err != nil {
			t.Fatalf("Write content: %v", err)
		}
	}()

	destDir := filepath.Join(dir, "dest")
	extracted, err := ExtractArchive(archivePath, destDir)
	if err != nil {
		t.Fatalf("ExtractArchive() error = %v", err)
	}

	if len(extracted) != 1 {
		t.Fatalf("ExtractArchive() returned %d entries, want 1", len(extracted))
	}
	if extracted[0].RelPath != "files/legacy.txt" {
		t.Errorf("extracted RelPath = %q, want %q", extracted[0].RelPath, "files/legacy.txt")
	}

	data, err := os.ReadFile(extracted[0].SourcePath)
	if err != nil {
		t.Fatalf("ReadFile extracted: %v", err)
	}
	if string(data) != string(content) {
		t.Errorf("extracted content = %q, want %q", string(data), string(content))
	}
}

// --- helpers ---

// openAndListTar opens a tar.gz file and returns a map of entry names → headers.
func openAndListTar(t *testing.T, archivePath string) map[string]*tar.Header {
	t.Helper()
	f, err := os.Open(archivePath)
	if err != nil {
		t.Fatalf("Open archive: %v", err)
	}
	defer f.Close()

	gr, err := gzip.NewReader(f)
	if err != nil {
		t.Fatalf("gzip.NewReader: %v", err)
	}
	defer gr.Close()

	tr := tar.NewReader(gr)
	found := make(map[string]*tar.Header)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("tar.Next: %v", err)
		}
		// Copy header so we can store it safely.
		h := *hdr
		found[hdr.Name] = &h
	}
	return found
}

// checkExtractedContent reads the file at byRel[relPath] and asserts its content.
func checkExtractedContent(t *testing.T, byRel map[string]string, relPath, wantContent string) {
	t.Helper()
	absPath, ok := byRel[relPath]
	if !ok {
		t.Errorf("extracted entries do not include %q; got %v", relPath, byRel)
		return
	}
	data, err := os.ReadFile(absPath)
	if err != nil {
		t.Errorf("ReadFile %q: %v", absPath, err)
		return
	}
	if string(data) != wantContent {
		t.Errorf("content of %q = %q, want %q", relPath, string(data), wantContent)
	}
}
