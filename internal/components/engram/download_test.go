package engram

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/internal/system"
)

// --- test helpers ---

// makeServerWithFakeTarGz returns an httptest.Server that serves:
//   - GET /releases/latest  → GitHub API JSON with the given version
//   - GET /releases/download/…  → a real .tar.gz containing "engram" binary
func makeServerWithFakeTarGz(t *testing.T, version string) *httptest.Server {
	t.Helper()
	tarContent := buildFakeTarGz(t, "engram")
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "releases/latest") {
			payload := map[string]string{"tag_name": "v" + version}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(payload)
			return
		}
		// All other requests → binary asset
		w.Header().Set("Content-Type", "application/octet-stream")
		w.WriteHeader(http.StatusOK)
		w.Write(tarContent)
	}))
}

// makeServerWithFakeZip returns a server that serves a zip archive containing
// "engram.exe" (Windows).
func makeServerWithFakeZip(t *testing.T, version string) *httptest.Server {
	t.Helper()
	zipContent := buildFakeZip(t, "engram.exe")
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "releases/latest") {
			payload := map[string]string{"tag_name": "v" + version}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(payload)
			return
		}
		w.Header().Set("Content-Type", "application/octet-stream")
		w.WriteHeader(http.StatusOK)
		w.Write(zipContent)
	}))
}

func buildFakeTarGz(t *testing.T, binaryName string) []byte {
	t.Helper()
	dir := t.TempDir()
	tarPath := filepath.Join(dir, "release.tar.gz")

	f, err := os.Create(tarPath)
	if err != nil {
		t.Fatalf("create tar.gz: %v", err)
	}
	gw := gzip.NewWriter(f)
	tw := tar.NewWriter(gw)

	content := []byte("#!/bin/sh\necho engram fake binary")
	tw.WriteHeader(&tar.Header{Name: binaryName, Mode: 0o755, Size: int64(len(content))})
	tw.Write(content)
	tw.Close()
	gw.Close()
	f.Close()

	data, err := os.ReadFile(tarPath)
	if err != nil {
		t.Fatalf("read tar.gz: %v", err)
	}
	return data
}

func buildFakeZip(t *testing.T, binaryName string) []byte {
	t.Helper()
	dir := t.TempDir()
	zipPath := filepath.Join(dir, "release.zip")

	f, err := os.Create(zipPath)
	if err != nil {
		t.Fatalf("create zip: %v", err)
	}
	zw := zip.NewWriter(f)

	content := []byte("fake engram.exe binary")
	fw, err := zw.Create(binaryName)
	if err != nil {
		t.Fatalf("create zip entry: %v", err)
	}
	fw.Write(content)
	zw.Close()
	f.Close()

	data, err := os.ReadFile(zipPath)
	if err != nil {
		t.Fatalf("read zip: %v", err)
	}
	return data
}

// --- TestEngramAssetURL ---

func TestEngramAssetURL(t *testing.T) {
	tests := []struct {
		name       string
		version    string
		goos       string
		goarch     string
		wantSubstr string
		wantExt    string
	}{
		{
			name:       "linux amd64 uses tar.gz",
			version:    "1.2.3",
			goos:       "linux",
			goarch:     "amd64",
			wantSubstr: "linux_amd64",
			wantExt:    ".tar.gz",
		},
		{
			name:       "linux arm64 uses tar.gz",
			version:    "1.2.3",
			goos:       "linux",
			goarch:     "arm64",
			wantSubstr: "linux_arm64",
			wantExt:    ".tar.gz",
		},
		{
			name:       "windows amd64 uses zip",
			version:    "1.2.3",
			goos:       "windows",
			goarch:     "amd64",
			wantSubstr: "windows_amd64",
			wantExt:    ".zip",
		},
		{
			name:       "darwin arm64 uses tar.gz",
			version:    "1.2.3",
			goos:       "darwin",
			goarch:     "arm64",
			wantSubstr: "darwin_arm64",
			wantExt:    ".tar.gz",
		},
		{
			name:       "url contains version",
			version:    "2.0.0",
			goos:       "linux",
			goarch:     "amd64",
			wantSubstr: "2.0.0",
			wantExt:    ".tar.gz",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			url := engramAssetURL("https://github.com", tt.version, tt.goos, tt.goarch)
			if !strings.Contains(url, tt.wantSubstr) {
				t.Errorf("engramAssetURL(%s, %s) = %q, want it to contain %q", tt.goos, tt.goarch, url, tt.wantSubstr)
			}
			if !strings.HasSuffix(url, tt.wantExt) {
				t.Errorf("engramAssetURL(%s) = %q, want suffix %q", tt.goos, url, tt.wantExt)
			}
		})
	}
}

// --- TestEngramInstallDir ---

func TestEngramInstallDir(t *testing.T) {
	tests := []struct {
		name       string
		goos       string
		wantSubstr string
	}{
		{
			name:       "linux returns /usr/local/bin or ~/.local/bin",
			goos:       "linux",
			wantSubstr: "bin",
		},
		{
			name:       "windows returns LOCALAPPDATA engram bin",
			goos:       "windows",
			wantSubstr: "engram",
		},
		{
			name:       "darwin returns /usr/local/bin",
			goos:       "darwin",
			wantSubstr: "bin",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := engramInstallDir(tt.goos)
			if !strings.Contains(dir, tt.wantSubstr) {
				t.Errorf("engramInstallDir(%s) = %q, want it to contain %q", tt.goos, dir, tt.wantSubstr)
			}
		})
	}
}

// --- TestDownloadLatestBinaryLinux ---

func TestDownloadLatestBinaryLinux(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("this test verifies Linux path behaviour, not applicable on Windows")
	}

	server := makeServerWithFakeTarGz(t, "1.3.0")
	defer server.Close()

	// Override the HTTP client and the base URL for GitHub API.
	origClient := engramHTTPClient
	origBaseURL := engramGitHubBaseURL
	engramHTTPClient = server.Client()
	engramGitHubBaseURL = server.URL
	t.Cleanup(func() {
		engramHTTPClient = origClient
		engramGitHubBaseURL = origBaseURL
	})

	// Override install dir to a temp directory (avoids needing root).
	tmpDir := t.TempDir()
	origInstallDirFn := engramInstallDirFn
	engramInstallDirFn = func(goos string) string { return tmpDir }
	t.Cleanup(func() { engramInstallDirFn = origInstallDirFn })

	profile := system.PlatformProfile{OS: "linux", PackageManager: "apt"}
	installedPath, err := DownloadLatestBinary(profile)
	if err != nil {
		t.Fatalf("DownloadLatestBinary() error = %v", err)
	}

	// The installed path must be inside the temp dir.
	if !strings.HasPrefix(installedPath, tmpDir) {
		t.Errorf("installedPath = %q, want prefix %q", installedPath, tmpDir)
	}

	// The binary must exist and be executable.
	info, err := os.Stat(installedPath)
	if err != nil {
		t.Fatalf("stat installed binary: %v", err)
	}
	if info.Size() == 0 {
		t.Errorf("installed binary is empty")
	}
	if info.Mode()&0o111 == 0 {
		t.Errorf("installed binary is not executable")
	}
}

// --- TestDownloadLatestBinaryWindows ---

func TestDownloadLatestBinaryWindows(t *testing.T) {
	server := makeServerWithFakeZip(t, "1.3.0")
	defer server.Close()

	origClient := engramHTTPClient
	origBaseURL := engramGitHubBaseURL
	engramHTTPClient = server.Client()
	engramGitHubBaseURL = server.URL
	t.Cleanup(func() {
		engramHTTPClient = origClient
		engramGitHubBaseURL = origBaseURL
	})

	tmpDir := t.TempDir()
	origInstallDirFn := engramInstallDirFn
	engramInstallDirFn = func(goos string) string { return tmpDir }
	t.Cleanup(func() { engramInstallDirFn = origInstallDirFn })

	profile := system.PlatformProfile{OS: "windows", PackageManager: "winget"}
	installedPath, err := DownloadLatestBinary(profile)
	if err != nil {
		t.Fatalf("DownloadLatestBinary() error = %v", err)
	}

	if !strings.HasPrefix(installedPath, tmpDir) {
		t.Errorf("installedPath = %q, want prefix %q", installedPath, tmpDir)
	}

	info, err := os.Stat(installedPath)
	if err != nil {
		t.Fatalf("stat installed binary: %v", err)
	}
	if info.Size() == 0 {
		t.Errorf("installed binary is empty")
	}
	// On Windows .exe files don't need Unix exec bit, just check it exists.
	if !strings.HasSuffix(installedPath, ".exe") {
		t.Errorf("Windows binary path should end in .exe, got %q", installedPath)
	}
}

// --- TestDownloadLatestBinaryAPIError ---

func TestDownloadLatestBinaryAPIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	origClient := engramHTTPClient
	origBaseURL := engramGitHubBaseURL
	engramHTTPClient = server.Client()
	engramGitHubBaseURL = server.URL
	t.Cleanup(func() {
		engramHTTPClient = origClient
		engramGitHubBaseURL = origBaseURL
	})

	profile := system.PlatformProfile{OS: "linux", PackageManager: "apt"}
	_, err := DownloadLatestBinary(profile)
	if err == nil {
		t.Fatal("expected error when GitHub API returns 500, got nil")
	}
}

func TestDownloadLatestBinaryFallsBackToAnonymousWhenTokenGets403(t *testing.T) {
	const fakeToken = "ci-token"
	const version = "1.3.0"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "releases/latest") {
			if r.Header.Get("Authorization") == "Bearer "+fakeToken {
				w.WriteHeader(http.StatusForbidden)
				return
			}

			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]string{"tag_name": "v" + version})
			return
		}

		w.Header().Set("Content-Type", "application/octet-stream")
		w.WriteHeader(http.StatusOK)
		w.Write(buildFakeTarGz(t, "engram"))
	}))
	defer server.Close()

	origClient := engramHTTPClient
	origBaseURL := engramGitHubBaseURL
	engramHTTPClient = server.Client()
	engramGitHubBaseURL = server.URL
	t.Cleanup(func() {
		engramHTTPClient = origClient
		engramGitHubBaseURL = origBaseURL
	})

	t.Setenv("GITHUB_TOKEN", fakeToken)
	t.Setenv("GH_TOKEN", "")

	tmpDir := t.TempDir()
	origInstallDirFn := engramInstallDirFn
	engramInstallDirFn = func(goos string) string { return tmpDir }
	t.Cleanup(func() { engramInstallDirFn = origInstallDirFn })

	profile := system.PlatformProfile{OS: "linux", PackageManager: "apt"}
	installedPath, err := DownloadLatestBinary(profile)
	if err != nil {
		t.Fatalf("DownloadLatestBinary() error = %v", err)
	}

	if !strings.HasPrefix(installedPath, tmpDir) {
		t.Errorf("installedPath = %q, want prefix %q", installedPath, tmpDir)
	}

	if _, err := os.Stat(installedPath); err != nil {
		t.Fatalf("stat installed binary: %v", err)
	}
}
