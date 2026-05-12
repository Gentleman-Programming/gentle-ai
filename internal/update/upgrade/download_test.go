package upgrade

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/internal/system"
	"github.com/gentleman-programming/gentle-ai/internal/update"
)

// --- test helpers ---

// makeFakeTarGz creates a minimal .tar.gz in a temp dir containing one executable binary.
// Returns the path to the .tar.gz file.
func makeFakeTarGz(t *testing.T, binaryName string) string {
	t.Helper()

	dir := t.TempDir()
	tarPath := filepath.Join(dir, "release.tar.gz")

	f, err := os.Create(tarPath)
	if err != nil {
		t.Fatalf("create tar.gz: %v", err)
	}
	defer f.Close()

	gw := gzip.NewWriter(f)
	tw := tar.NewWriter(gw)

	content := []byte("#!/bin/sh\necho fake binary")
	hdr := &tar.Header{
		Name: binaryName,
		Mode: 0o755,
		Size: int64(len(content)),
	}
	if err := tw.WriteHeader(hdr); err != nil {
		t.Fatalf("write tar header: %v", err)
	}
	if _, err := tw.Write(content); err != nil {
		t.Fatalf("write tar content: %v", err)
	}
	if err := tw.Close(); err != nil {
		t.Fatalf("close tar writer: %v", err)
	}
	if err := gw.Close(); err != nil {
		t.Fatalf("close gzip writer: %v", err)
	}

	return tarPath
}

// --- TestAssetURLResolution ---

// TestAssetURLResolution verifies that resolveAssetURL produces a correct
// GitHub Releases asset download URL for a given GOOS/GOARCH combination.
func TestAssetURLResolution(t *testing.T) {
	tests := []struct {
		name       string
		owner      string
		repo       string
		version    string
		goos       string
		goarch     string
		wantSubstr string
	}{
		{
			name:       "darwin amd64",
			owner:      "Gentleman-Programming",
			repo:       "gentle-ai",
			version:    "1.5.0",
			goos:       "darwin",
			goarch:     "amd64",
			wantSubstr: "darwin",
		},
		{
			name:       "darwin arm64",
			owner:      "Gentleman-Programming",
			repo:       "gentle-ai",
			version:    "1.5.0",
			goos:       "darwin",
			goarch:     "arm64",
			wantSubstr: "arm64",
		},
		{
			name:       "linux amd64",
			owner:      "Gentleman-Programming",
			repo:       "gga",
			version:    "2.0.0",
			goos:       "linux",
			goarch:     "amd64",
			wantSubstr: "linux",
		},
		{
			name:       "contains version",
			owner:      "Gentleman-Programming",
			repo:       "gentle-ai",
			version:    "1.5.0",
			goos:       "darwin",
			goarch:     "amd64",
			wantSubstr: "1.5.0",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			url := resolveAssetURL(tc.owner, tc.repo, tc.version, tc.goos, tc.goarch)
			if url == "" {
				t.Fatalf("resolveAssetURL returned empty string")
			}
			if !containsSubstr(url, tc.wantSubstr) {
				t.Errorf("resolveAssetURL(%s/%s, %s, %s/%s) = %q, want it to contain %q",
					tc.owner, tc.repo, tc.version, tc.goos, tc.goarch, url, tc.wantSubstr)
			}
		})
	}
}

// --- TestDownloadAndExtract ---

// TestDownloadAndExtract uses an httptest.Server to serve a fake tar.gz
// and verifies that the binary is extracted to a temp file.
func TestDownloadAndExtract(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("binary download not supported on Windows in Phase 1")
	}

	binaryName := "fake-tool"
	tarPath := makeFakeTarGz(t, binaryName)
	tarContent, err := os.ReadFile(tarPath)
	if err != nil {
		t.Fatalf("read fake tar.gz: %v", err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/octet-stream")
		w.WriteHeader(http.StatusOK)
		w.Write(tarContent)
	}))
	defer server.Close()

	origHTTPClient := httpClient
	t.Cleanup(func() { httpClient = origHTTPClient })
	httpClient = server.Client()

	outPath := filepath.Join(t.TempDir(), binaryName)
	err = downloadBinary(context.Background(), server.URL+"/release.tar.gz", binaryName, outPath)
	if err != nil {
		t.Fatalf("downloadBinary: %v", err)
	}

	info, err := os.Stat(outPath)
	if err != nil {
		t.Fatalf("stat output: %v", err)
	}
	if info.Size() == 0 {
		t.Errorf("output file is empty")
	}
	if info.Mode()&0o111 == 0 {
		t.Errorf("output file should be executable")
	}
}

// --- TestDownloadAndExtract_NotFoundReturnsError ---

func TestDownloadAndExtract_NotFoundReturnsError(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("binary download not supported on Windows in Phase 1")
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	origHTTPClient := httpClient
	t.Cleanup(func() { httpClient = origHTTPClient })
	httpClient = server.Client()

	outPath := filepath.Join(t.TempDir(), "fake-tool")
	err := downloadBinary(context.Background(), server.URL+"/missing.tar.gz", "fake-tool", outPath)
	if err == nil {
		t.Errorf("expected error for 404, got nil")
	}
}

// --- TestAtomicReplace ---

// TestAtomicReplace verifies that atomicReplace replaces the destination file
// without leaving temp files around.
func TestAtomicReplace(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("atomic replace uses rename — Windows behavior is different")
	}

	dir := t.TempDir()
	src := filepath.Join(dir, "new-binary")
	dst := filepath.Join(dir, "existing-binary")

	// Write source (new binary)
	if err := os.WriteFile(src, []byte("new content"), 0o755); err != nil {
		t.Fatalf("write src: %v", err)
	}
	// Write destination (old binary)
	if err := os.WriteFile(dst, []byte("old content"), 0o755); err != nil {
		t.Fatalf("write dst: %v", err)
	}

	if err := atomicReplace(src, dst); err != nil {
		t.Fatalf("atomicReplace: %v", err)
	}

	content, err := os.ReadFile(dst)
	if err != nil {
		t.Fatalf("read dst after replace: %v", err)
	}
	if string(content) != "new content" {
		t.Errorf("dst content = %q, want %q", content, "new content")
	}

	// Source should no longer exist (it was moved).
	if _, err := os.Stat(src); !os.IsNotExist(err) {
		t.Errorf("source file should no longer exist after atomic replace")
	}
}

// --- TestDownload_WindowsSkipped ---

// TestDownload_WindowsSkipped is a build-constraint smoke test:
// calling Download on Windows should return a manual fallback error.
func TestDownload_WindowsAlwaysManualFallback(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("only runs on Windows")
	}

	r := update.UpdateResult{
		Tool: update.ToolInfo{
			Name:          "gentle-ai",
			Owner:         "Gentleman-Programming",
			Repo:          "gentle-ai",
			InstallMethod: update.InstallBinary,
		},
		LatestVersion: "1.5.0",
	}
	profile := system.PlatformProfile{OS: "windows", PackageManager: "winget"}

	err := Download(context.Background(), r, profile)
	if err == nil {
		t.Errorf("expected error for Windows binary download, got nil")
	}
}

// --- TestFindBinaryInTar ---

// TestFindBinaryInTar verifies that findBinaryInTar extracts the correct entry
// from a tar that may contain subdirectories.
func TestFindBinaryInTar(t *testing.T) {
	dir := t.TempDir()
	tarPath := filepath.Join(dir, "release.tar.gz")

	f, _ := os.Create(tarPath)
	gw := gzip.NewWriter(f)
	tw := tar.NewWriter(gw)

	content := []byte("#!/bin/sh\necho real binary")
	entries := []struct {
		name    string
		content []byte
	}{
		{"README.md", []byte("readme content")},
		{"gentle-ai_1.5.0_darwin_arm64/gentle-ai", content}, // binary in subdir
	}

	for _, e := range entries {
		tw.WriteHeader(&tar.Header{Name: e.name, Mode: 0o644, Size: int64(len(e.content))})
		tw.Write(e.content)
	}
	tw.Close()
	gw.Close()
	f.Close()

	tarContent, _ := os.ReadFile(tarPath)
	outPath := filepath.Join(t.TempDir(), "gentle-ai")

	// Use an httptest server to serve the tar.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write(tarContent)
	}))
	defer server.Close()

	origHTTPClient := httpClient
	t.Cleanup(func() { httpClient = origHTTPClient })
	httpClient = server.Client()

	err := downloadBinary(context.Background(), server.URL+"/release.tar.gz", "gentle-ai", outPath)
	if err != nil {
		t.Fatalf("downloadBinary: %v", err)
	}

	got, _ := os.ReadFile(outPath)
	if string(got) != string(content) {
		t.Errorf("binary content = %q, want %q", got, content)
	}
}

// --- TestExpectedChecksumFor ---

func TestExpectedChecksumFor(t *testing.T) {
	content := "abc123  gentle-ai_1.0.0_darwin_arm64.tar.gz\ndef456  gentle-ai_1.0.0_linux_amd64.tar.gz\n"

	tests := []struct {
		name      string
		content   string
		filename  string
		want      string
		wantErr   bool
	}{
		{
			name:     "found first entry",
			content:  content,
			filename: "gentle-ai_1.0.0_darwin_arm64.tar.gz",
			want:     "abc123",
		},
		{
			name:     "found second entry",
			content:  content,
			filename: "gentle-ai_1.0.0_linux_amd64.tar.gz",
			want:     "def456",
		},
		{
			name:     "not found returns error",
			content:  content,
			filename: "gentle-ai_1.0.0_windows_amd64.zip",
			wantErr:  true,
		},
		{
			name:     "empty content returns error",
			content:  "",
			filename: "gentle-ai_1.0.0_darwin_arm64.tar.gz",
			wantErr:  true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := expectedChecksumFor(tc.content, tc.filename)
			if (err != nil) != tc.wantErr {
				t.Errorf("expectedChecksumFor(%q) error = %v, wantErr %v", tc.filename, err, tc.wantErr)
			}
			if got != tc.want {
				t.Errorf("expectedChecksumFor(%q) = %q, want %q", tc.filename, got, tc.want)
			}
		})
	}
}

// --- TestFetchChecksums ---

func TestFetchChecksums(t *testing.T) {
	const fakeContent = "abc123  gentle-ai_1.0.0_darwin_arm64.tar.gz\n"

	t.Run("success returns content", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			fmt.Fprint(w, fakeContent)
		}))
		defer server.Close()

		orig := httpClient
		t.Cleanup(func() { httpClient = orig })
		httpClient = server.Client()

		got, err := fetchChecksums(context.Background(), server.URL+"/checksums.txt")
		if err != nil {
			t.Fatalf("fetchChecksums: unexpected error: %v", err)
		}
		if got != fakeContent {
			t.Errorf("fetchChecksums = %q, want %q", got, fakeContent)
		}
	})

	t.Run("HTTP 404 returns error", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
		}))
		defer server.Close()

		orig := httpClient
		t.Cleanup(func() { httpClient = orig })
		httpClient = server.Client()

		_, err := fetchChecksums(context.Background(), server.URL+"/checksums.txt")
		if err == nil {
			t.Error("expected error for HTTP 404, got nil")
		}
	})
}

// --- TestDownloadToFile ---

func TestDownloadToFile(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("binary download not supported on Windows")
	}

	content := []byte("test archive content")
	h := sha256.New()
	h.Write(content)
	wantDigest := hex.EncodeToString(h.Sum(nil))

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write(content) //nolint:errcheck
	}))
	defer server.Close()

	orig := httpClient
	t.Cleanup(func() { httpClient = orig })
	httpClient = server.Client()

	outPath := filepath.Join(t.TempDir(), "downloaded.tar.gz")
	gotDigest, err := downloadToFile(context.Background(), server.URL+"/file", outPath)
	if err != nil {
		t.Fatalf("downloadToFile: %v", err)
	}
	if gotDigest != wantDigest {
		t.Errorf("digest = %q, want %q", gotDigest, wantDigest)
	}
	if _, err := os.Stat(outPath); err != nil {
		t.Errorf("output file not created: %v", err)
	}
}

// --- TestDownload_ChecksumVerification ---

// TestDownload_ChecksumVerification exercises all four checksum failure modes
// from issue #245: match, mismatch, missing checksums.txt, and archive not listed.
func TestDownload_ChecksumVerification(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("binary download not supported on Windows")
	}

	binaryName := "fake-tool"
	tarPath := makeFakeTarGz(t, binaryName)
	tarContent, err := os.ReadFile(tarPath)
	if err != nil {
		t.Fatalf("read fake tar.gz: %v", err)
	}

	// Compute the real SHA256 of the archive so we can produce a matching digest.
	h := sha256.New()
	h.Write(tarContent)
	realDigest := hex.EncodeToString(h.Sum(nil))

	// archiveName is what Download computes internally; we match it exactly.
	archiveName := resolveArchiveName(binaryName, "1.0.0", runtime.GOOS, runtime.GOARCH)

	tests := []struct {
		name            string
		checksumsBody   string
		checksumsStatus int
		wantErr         bool
		errContains     string
	}{
		{
			name:            "matching checksum succeeds",
			checksumsBody:   fmt.Sprintf("%s  %s\n", realDigest, archiveName),
			checksumsStatus: http.StatusOK,
			wantErr:         false,
		},
		{
			name:            "checksum mismatch returns error",
			checksumsBody:   fmt.Sprintf("%s  %s\n", "deadbeefdeadbeef", archiveName),
			checksumsStatus: http.StatusOK,
			wantErr:         true,
			errContains:     "checksum mismatch",
		},
		{
			name:            "missing checksums.txt returns error",
			checksumsBody:   "",
			checksumsStatus: http.StatusNotFound,
			wantErr:         true,
			errContains:     "checksums.txt unavailable",
		},
		{
			name:            "archive not in checksums.txt returns error",
			checksumsBody:   "abc123  other-tool_1.0.0_linux_amd64.tar.gz\n",
			checksumsStatus: http.StatusOK,
			wantErr:         true,
			errContains:     "not listed in checksums.txt",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if strings.HasSuffix(r.URL.Path, "checksums.txt") {
					w.WriteHeader(tc.checksumsStatus)
					fmt.Fprint(w, tc.checksumsBody)
				} else {
					w.WriteHeader(http.StatusOK)
					w.Write(tarContent) //nolint:errcheck
				}
			}))
			defer server.Close()

			// Mock HTTP client.
			origClient := httpClient
			t.Cleanup(func() { httpClient = origClient })
			httpClient = server.Client()

			// Mock URL builders to redirect to the test server.
			origAssetURLFn := resolveAssetURLFn
			origChecksumURLFn := resolveChecksumURLFn
			t.Cleanup(func() {
				resolveAssetURLFn = origAssetURLFn
				resolveChecksumURLFn = origChecksumURLFn
			})
			resolveAssetURLFn = func(owner, repo, version, goos, goarch string) string {
				return server.URL + "/" + archiveName
			}
			resolveChecksumURLFn = func(owner, repo, version string) string {
				return server.URL + "/checksums.txt"
			}

			// Mock lookPathFn with a real temp binary (atomicReplace needs a valid path).
			tmpBinary := filepath.Join(t.TempDir(), binaryName)
			if err := os.WriteFile(tmpBinary, []byte("old binary"), 0o755); err != nil {
				t.Fatalf("write temp binary: %v", err)
			}
			origLookPath := lookPathFn
			t.Cleanup(func() { lookPathFn = origLookPath })
			lookPathFn = func(name string) (string, error) { return tmpBinary, nil }

			r := update.UpdateResult{
				Tool: update.ToolInfo{
					Name:  binaryName,
					Owner: "test-owner",
					Repo:  binaryName,
				},
				LatestVersion: "1.0.0",
			}
			profile := system.PlatformProfile{OS: runtime.GOOS}

			err := Download(context.Background(), r, profile)
			if (err != nil) != tc.wantErr {
				t.Errorf("Download() error = %v, wantErr = %v", err, tc.wantErr)
			}
			if tc.errContains != "" && err != nil && !strings.Contains(err.Error(), tc.errContains) {
				t.Errorf("Download() error = %q, want it to contain %q", err.Error(), tc.errContains)
			}
		})
	}
}

// --- helpers ---

func containsSubstr(s, sub string) bool {
	return len(s) >= len(sub) && func() bool {
		for i := 0; i <= len(s)-len(sub); i++ {
			if s[i:i+len(sub)] == sub {
				return true
			}
		}
		return false
	}()
}

// dummyReadCloser wraps a reader for test use.
type dummyReadCloser struct{ io.Reader }

func (d dummyReadCloser) Close() error { return nil }

// Suppress unused import warnings in case fmt is needed.
var _ = fmt.Sprintf
