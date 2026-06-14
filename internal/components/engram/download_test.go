package engram

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
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

// sha256Hex returns the SHA256 hex digest of data.
func sha256Hex(data []byte) string {
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])
}

// makeChecksumsTxt returns a BSD-style checksums.txt entry for the given filename and data.
func makeChecksumsTxt(filename string, data []byte) string {
	return fmt.Sprintf("%s  %s\n", sha256Hex(data), filename)
}

// makeServerWithFakeTarGz returns an httptest.Server that serves:
//   - GET /releases/latest       → GitHub API JSON with the given version
//   - GET /releases/download/…   → a real .tar.gz containing "engram" binary
//   - GET /…/checksums.txt       → a valid checksums.txt covering all arches
func makeServerWithFakeTarGz(t *testing.T, version string) *httptest.Server {
	t.Helper()
	tarContent := buildFakeTarGz(t, "engram")
	// Pre-build a checksums.txt that covers linux/darwin for both amd64 and arm64
	// so the test is not sensitive to the host architecture.
	checksums := ""
	for _, goos := range []string{"linux", "darwin"} {
		for _, goarch := range []string{"amd64", "arm64"} {
			name := engramArchiveName(version, goos, goarch)
			checksums += makeChecksumsTxt(name, tarContent)
		}
	}
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "releases/latest"):
			payload := map[string]string{"tag_name": "v" + version}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(payload)
		case strings.HasSuffix(r.URL.Path, "checksums.txt"):
			w.Header().Set("Content-Type", "text/plain")
			fmt.Fprint(w, checksums)
		default:
			// Binary asset
			w.Header().Set("Content-Type", "application/octet-stream")
			w.WriteHeader(http.StatusOK)
			w.Write(tarContent)
		}
	}))
}

// makeServerWithFakeZip returns a server that serves a zip archive containing
// "engram.exe" (Windows).
func makeServerWithFakeZip(t *testing.T, version string) *httptest.Server {
	t.Helper()
	zipContent := buildFakeZip(t, "engram.exe")
	// Pre-build a checksums.txt that covers windows for both amd64 and arm64
	// so the test is not sensitive to the host architecture.
	checksums := ""
	for _, goarch := range []string{"amd64", "arm64"} {
		name := engramArchiveName(version, "windows", goarch)
		checksums += makeChecksumsTxt(name, zipContent)
	}
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "releases/latest"):
			payload := map[string]string{"tag_name": "v" + version}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(payload)
		case strings.HasSuffix(r.URL.Path, "checksums.txt"):
			w.Header().Set("Content-Type", "text/plain")
			fmt.Fprint(w, checksums)
		default:
			w.Header().Set("Content-Type", "application/octet-stream")
			w.WriteHeader(http.StatusOK)
			w.Write(zipContent)
		}
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
	installedPath, err := DownloadLatestBinary(profile, false)
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
	origStopProcessesFn := engramStopProcessesFn
	engramHTTPClient = server.Client()
	engramGitHubBaseURL = server.URL
	engramStopProcessesFn = func() error { return nil }
	t.Cleanup(func() {
		engramHTTPClient = origClient
		engramGitHubBaseURL = origBaseURL
		engramStopProcessesFn = origStopProcessesFn
	})

	tmpDir := t.TempDir()
	origInstallDirFn := engramInstallDirFn
	engramInstallDirFn = func(goos string) string { return tmpDir }
	t.Cleanup(func() { engramInstallDirFn = origInstallDirFn })

	profile := system.PlatformProfile{OS: "windows", PackageManager: "winget"}
	installedPath, err := DownloadLatestBinary(profile, false)
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

func TestDownloadLatestBinaryDownloadError(t *testing.T) {
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
	_, err := DownloadLatestBinary(profile, false)
	if err == nil {
		t.Fatal("expected error when GitHub API returns 500, got nil")
	}
}

func TestDownloadLatestBinarySkipsLatestReleaseWithoutBinaryAssets(t *testing.T) {
	const binaryVersion = "1.15.13"

	tarContent := buildFakeTarGz(t, "engram")
	// Build checksums.txt covering all linux arches so the test is arch-agnostic.
	checksums := ""
	for _, goarch := range []string{"amd64", "arm64"} {
		checksums += makeChecksumsTxt(engramArchiveName(binaryVersion, "linux", goarch), tarContent)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "releases/latest"):
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]any{
				"tag_name": "pi-v0.1.7",
				"assets":   []any{},
			})
		case strings.Contains(r.URL.Path, "releases") && !strings.Contains(r.URL.Path, "releases/latest") &&
			!strings.Contains(r.URL.Path, "/releases/download") &&
			r.URL.Query().Get("per_page") == "20":
			w.Header().Set("Content-Type", "application/json")
			// Pages beyond 1 return empty to signal end-of-list.
			if r.URL.Query().Get("page") != "1" {
				json.NewEncoder(w).Encode([]map[string]any{})
				return
			}
			json.NewEncoder(w).Encode([]map[string]any{
				{
					"tag_name":   "pi-v0.1.7",
					"draft":      false,
					"prerelease": false,
					"assets":     []any{},
				},
				{
					"tag_name":   "v" + binaryVersion,
					"draft":      false,
					"prerelease": false,
					"assets": []map[string]string{
						{"name": "checksums.txt"},
						{"name": "engram_" + binaryVersion + "_linux_amd64.tar.gz"},
					},
				},
			})
		case strings.HasSuffix(r.URL.Path, "checksums.txt"):
			w.Header().Set("Content-Type", "text/plain")
			fmt.Fprint(w, checksums)
		case strings.Contains(r.URL.Path, "/releases/download/v"+binaryVersion+"/engram_"+binaryVersion+"_linux_"):
			w.Header().Set("Content-Type", "application/octet-stream")
			w.WriteHeader(http.StatusOK)
			w.Write(tarContent)
		default:
			t.Fatalf("unexpected request: %s?%s", r.URL.Path, r.URL.RawQuery)
		}
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

	tmpDir := t.TempDir()
	origInstallDirFn := engramInstallDirFn
	engramInstallDirFn = func(goos string) string { return tmpDir }
	t.Cleanup(func() { engramInstallDirFn = origInstallDirFn })

	profile := system.PlatformProfile{OS: "linux", PackageManager: "apt"}
	installedPath, err := DownloadLatestBinary(profile, false)
	if err != nil {
		t.Fatalf("DownloadLatestBinary() error = %v", err)
	}

	if _, err := os.Stat(installedPath); err != nil {
		t.Fatalf("stat installed binary: %v", err)
	}
}

func TestDownloadLatestBinaryReleaseListFallsBackToAnonymousWhenTokenGets403(t *testing.T) {
	const fakeToken = "ci-token"
	const binaryVersion = "1.15.13"

	tarContent := buildFakeTarGz(t, "engram")
	checksums := ""
	for _, goarch := range []string{"amd64", "arm64"} {
		checksums += makeChecksumsTxt(engramArchiveName(binaryVersion, "linux", goarch), tarContent)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		switch {
		case strings.Contains(r.URL.Path, "releases/latest"):
			if auth != "" {
				// Simulate 403 when authenticated to trigger anonymous retry.
				w.WriteHeader(http.StatusForbidden)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]string{"tag_name": "v" + binaryVersion})
		case strings.Contains(r.URL.Path, "releases") && !strings.Contains(r.URL.Path, "releases/latest") &&
			!strings.Contains(r.URL.Path, "/releases/download") &&
			r.URL.Query().Get("per_page") == "20":
			if auth != "" {
				w.WriteHeader(http.StatusForbidden)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			// Pages beyond 1 return empty to signal end-of-list.
			if r.URL.Query().Get("page") != "1" {
				json.NewEncoder(w).Encode([]map[string]any{})
				return
			}
			json.NewEncoder(w).Encode([]map[string]any{
				{
					"tag_name":   "v" + binaryVersion,
					"draft":      false,
					"prerelease": false,
					"assets": []map[string]string{
						{"name": "engram_" + binaryVersion + "_linux_amd64.tar.gz"},
					},
				},
			})
		case strings.HasSuffix(r.URL.Path, "checksums.txt"):
			w.Header().Set("Content-Type", "text/plain")
			fmt.Fprint(w, checksums)
		case strings.Contains(r.URL.Path, "/releases/download/v"+binaryVersion+"/engram_"+binaryVersion+"_linux_"):
			w.Header().Set("Content-Type", "application/octet-stream")
			w.WriteHeader(http.StatusOK)
			w.Write(tarContent)
		default:
			t.Fatalf("unexpected request: %s?%s", r.URL.Path, r.URL.RawQuery)
		}
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
	installedPath, err := DownloadLatestBinary(profile, false)
	if err != nil {
		t.Fatalf("DownloadLatestBinary() error = %v", err)
	}

	if _, err := os.Stat(installedPath); err != nil {
		t.Fatalf("stat installed binary: %v", err)
	}
}

func TestDownloadLatestBinaryWindowsStopsEngramBeforeReplace(t *testing.T) {
	version := "1.3.0"
	server := makeServerWithFakeZip(t, version)
	defer server.Close()

	origClient := engramHTTPClient
	origBaseURL := engramGitHubBaseURL
	origInstallDirFn := engramInstallDirFn
	origStopProcessesFn := engramStopProcessesFn
	t.Cleanup(func() {
		engramHTTPClient = origClient
		engramGitHubBaseURL = origBaseURL
		engramInstallDirFn = origInstallDirFn
		engramStopProcessesFn = origStopProcessesFn
	})

	engramHTTPClient = server.Client()
	engramGitHubBaseURL = server.URL
	installDir := t.TempDir()
	engramInstallDirFn = func(goos string) string { return installDir }

	stopCalls := 0
	engramStopProcessesFn = func() error {
		stopCalls++
		return nil
	}

	installedPath, err := DownloadLatestBinary(system.PlatformProfile{OS: "windows", PackageManager: "winget"}, false)
	if err != nil {
		t.Fatalf("DownloadLatestBinary(windows) error = %v", err)
	}

	if stopCalls != 1 {
		t.Fatalf("stop calls = %d, want 1", stopCalls)
	}
	if filepath.Base(installedPath) != "engram.exe" {
		t.Fatalf("installed path = %q, want engram.exe", installedPath)
	}
	if _, err := os.Stat(installedPath); err != nil {
		t.Fatalf("stat installed binary: %v", err)
	}
}

func TestDownloadLatestBinaryWindowsStopFailureAbortsBeforeReplace(t *testing.T) {
	version := "1.3.0"
	server := makeServerWithFakeZip(t, version)
	defer server.Close()

	origClient := engramHTTPClient
	origBaseURL := engramGitHubBaseURL
	origInstallDirFn := engramInstallDirFn
	origStopProcessesFn := engramStopProcessesFn
	t.Cleanup(func() {
		engramHTTPClient = origClient
		engramGitHubBaseURL = origBaseURL
		engramInstallDirFn = origInstallDirFn
		engramStopProcessesFn = origStopProcessesFn
	})

	engramHTTPClient = server.Client()
	engramGitHubBaseURL = server.URL
	installDir := t.TempDir()
	engramInstallDirFn = func(goos string) string { return installDir }
	engramStopProcessesFn = func() error { return errors.New("stop denied") }

	_, err := DownloadLatestBinary(system.PlatformProfile{OS: "windows", PackageManager: "winget"}, false)
	if err == nil {
		t.Fatal("expected stop failure, got nil")
	}
	if !strings.Contains(err.Error(), "stop running engram processes before upgrade") {
		t.Fatalf("error = %q, want stop context", err.Error())
	}
	if _, err := os.Stat(filepath.Join(installDir, "engram.exe")); !os.IsNotExist(err) {
		t.Fatalf("engram.exe should not be written after stop failure, stat err: %v", err)
	}
}

// TestDownloadLatestBinaryWindowsStopSucceedsWhenProcessNotRunning verifies that the
// Windows stop-before-replace path does NOT fail when no engram process is running.
// This is the regression case from issue #850: Stop-Process with -ErrorAction Stop
// would exit 1 even when nothing needed stopping (e.g. the process list was empty or
// the process was held by an editor session and Get-Process returned nothing).
// The seam returning nil (no error) is the contract that must hold for the
// "engram not running" case; the implementation no longer uses -ErrorAction Stop.
func TestDownloadLatestBinaryWindowsStopSucceedsWhenProcessNotRunning(t *testing.T) {
	version := "1.3.0"
	server := makeServerWithFakeZip(t, version)
	defer server.Close()

	origClient := engramHTTPClient
	origBaseURL := engramGitHubBaseURL
	origInstallDirFn := engramInstallDirFn
	origStopProcessesFn := engramStopProcessesFn
	t.Cleanup(func() {
		engramHTTPClient = origClient
		engramGitHubBaseURL = origBaseURL
		engramInstallDirFn = origInstallDirFn
		engramStopProcessesFn = origStopProcessesFn
	})

	engramHTTPClient = server.Client()
	engramGitHubBaseURL = server.URL
	installDir := t.TempDir()
	engramInstallDirFn = func(goos string) string { return installDir }

	// Simulate stopEngramProcesses returning nil (no engram process found — clean).
	engramStopProcessesFn = func() error { return nil }

	installedPath, err := DownloadLatestBinary(system.PlatformProfile{OS: "windows", PackageManager: "winget"}, false)
	if err != nil {
		t.Fatalf("DownloadLatestBinary(windows) should succeed when stop returns nil, got: %v", err)
	}
	if _, err := os.Stat(installedPath); err != nil {
		t.Fatalf("stat installed binary: %v", err)
	}
}

// TestDownloadLatestBinaryWindowsStopNilProceedsToInstall verifies that when
// stopEngramProcesses returns nil, DownloadLatestBinary proceeds and installs the
// binary. This covers the caller's "nil means proceed" contract only; it does NOT
// exercise the WARNING-to-stderr emission inside stopEngramProcesses (that branch
// requires a real PowerShell call and is only integration-covered on Windows CI).
func TestDownloadLatestBinaryWindowsStopNilProceedsToInstall(t *testing.T) {
	version := "1.3.0"
	server := makeServerWithFakeZip(t, version)
	defer server.Close()

	origClient := engramHTTPClient
	origBaseURL := engramGitHubBaseURL
	origInstallDirFn := engramInstallDirFn
	origStopProcessesFn := engramStopProcessesFn
	t.Cleanup(func() {
		engramHTTPClient = origClient
		engramGitHubBaseURL = origBaseURL
		engramInstallDirFn = origInstallDirFn
		engramStopProcessesFn = origStopProcessesFn
	})

	engramHTTPClient = server.Client()
	engramGitHubBaseURL = server.URL
	installDir := t.TempDir()
	engramInstallDirFn = func(goos string) string { return installDir }

	// Simulate the resilient case: stop was attempted, some processes could not be
	// stopped (access denied), but stopEngramProcesses returns nil (warning-only).
	// The upgrade should still proceed — Windows may succeed in replacing the file.
	engramStopProcessesFn = func() error {
		// In the real implementation this prints a WARNING to stderr and returns nil.
		return nil
	}

	installedPath, err := DownloadLatestBinary(system.PlatformProfile{OS: "windows", PackageManager: "winget"}, false)
	if err != nil {
		t.Fatalf("DownloadLatestBinary(windows) should not abort when stop returns nil (warning path), got: %v", err)
	}
	if _, err := os.Stat(installedPath); err != nil {
		t.Fatalf("stat installed binary: %v", err)
	}
}

// TestDownloadLatestBinaryIgnoresGentleEngramAndPiTags asserts the CORRECTNESS
// CRUX: when the release list contains a mix of a core engram tag (vX.Y.Z), a
// gentle-engram tag, and a pi-v* tag, DownloadLatestBinary MUST pick the core
// engram version and MUST NOT pick the gentle-engram or pi-* tag.
func TestDownloadLatestBinaryIgnoresGentleEngramAndPiTags(t *testing.T) {
	const binaryVersion = "1.16.3"

	tarContent := buildFakeTarGz(t, "engram")
	checksums := ""
	for _, goarch := range []string{"amd64", "arm64"} {
		checksums += makeChecksumsTxt(engramArchiveName(binaryVersion, "linux", goarch), tarContent)
	}

	// The release list intentionally lists gentle-engram first (highest position)
	// and pi-v* second — both without binary assets — followed by the real core
	// engram release. The download MUST skip the first two and pick the third.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "releases/latest"):
			// /latest points at a gentle-engram tag — the non-core tag that must be skipped.
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]any{
				"tag_name": "gentle-engram v0.1.8",
				"assets":   []any{},
			})
		case strings.Contains(r.URL.Path, "releases") && !strings.Contains(r.URL.Path, "releases/latest") &&
			!strings.Contains(r.URL.Path, "/releases/download") &&
			r.URL.Query().Get("per_page") == "20":
			w.Header().Set("Content-Type", "application/json")
			// Pages beyond 1 return empty to signal end-of-list.
			if r.URL.Query().Get("page") != "1" {
				json.NewEncoder(w).Encode([]map[string]any{})
				return
			}
			json.NewEncoder(w).Encode([]map[string]any{
				{
					"tag_name":   "gentle-engram v0.1.8",
					"draft":      false,
					"prerelease": false,
					"assets":     []any{},
				},
				{
					"tag_name":   "pi-v0.1.7",
					"draft":      false,
					"prerelease": false,
					"assets":     []any{},
				},
				{
					"tag_name":   "v" + binaryVersion,
					"draft":      false,
					"prerelease": false,
					"assets": []map[string]string{
						{"name": "checksums.txt"},
						{"name": "engram_" + binaryVersion + "_linux_amd64.tar.gz"},
					},
				},
			})
		case strings.HasSuffix(r.URL.Path, "checksums.txt"):
			w.Header().Set("Content-Type", "text/plain")
			fmt.Fprint(w, checksums)
		case strings.Contains(r.URL.Path, "/releases/download/v"+binaryVersion+"/engram_"+binaryVersion+"_linux_"):
			w.Header().Set("Content-Type", "application/octet-stream")
			w.WriteHeader(http.StatusOK)
			w.Write(tarContent)
		default:
			t.Fatalf("unexpected request path (should be core engram v%s, not gentle-engram/pi): %s?%s",
				binaryVersion, r.URL.Path, r.URL.RawQuery)
		}
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

	tmpDir := t.TempDir()
	origInstallDirFn := engramInstallDirFn
	engramInstallDirFn = func(goos string) string { return tmpDir }
	t.Cleanup(func() { engramInstallDirFn = origInstallDirFn })

	profile := system.PlatformProfile{OS: "linux", PackageManager: "apt"}
	installedPath, err := DownloadLatestBinary(profile, false)
	if err != nil {
		t.Fatalf("DownloadLatestBinary() error = %v, want core engram v%s to be selected", err, binaryVersion)
	}

	if !strings.HasPrefix(installedPath, tmpDir) {
		t.Errorf("installedPath = %q, want prefix %q", installedPath, tmpDir)
	}
	if _, err := os.Stat(installedPath); err != nil {
		t.Fatalf("stat installed binary: %v", err)
	}
}

// --- TestEngramChecksumVerification ---
//
// Table-driven tests covering all checksum verification paths:
//
//   - success: valid checksums.txt with correct digest → install succeeds
//   - missing checksums.txt: server returns 404 → fail closed
//   - digest mismatch: checksums.txt lists wrong digest → fail closed
//   - malformed checksums.txt: content has no parseable entries → fail closed
func TestEngramChecksumVerification(t *testing.T) {
	version := "1.3.0"

	// tarContent is a real .tar.gz archive used across sub-tests.
	tarContent := buildFakeTarGz(t, "engram")
	correctDigest := sha256Hex(tarContent)
	archiveName := engramArchiveName(version, "linux", normalizeArch(runtime.GOARCH))

	tests := []struct {
		name          string
		checksumBody  string // content served at /…/checksums.txt; empty = serve 404
		checksumCode  int    // HTTP status for checksums.txt (0 → use 200 when body set)
		wantErrSubstr string // expected substring in error; empty = success
	}{
		{
			name:         "success: valid checksum passes",
			checksumBody: fmt.Sprintf("%s  %s\n", correctDigest, archiveName),
		},
		{
			name:          "missing checksums.txt: 404 fails closed",
			checksumCode:  http.StatusNotFound,
			wantErrSubstr: "checksums.txt unavailable",
		},
		{
			name:          "digest mismatch: wrong hash fails closed",
			checksumBody:  fmt.Sprintf("%s  %s\n", strings.Repeat("a", 64), archiveName),
			wantErrSubstr: "checksum mismatch",
		},
		{
			name:          "malformed checksums.txt: no matching entry fails closed",
			checksumBody:  "thisisnotavalidchecksumline\n",
			wantErrSubstr: "not listed in checksums.txt",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch {
				case strings.Contains(r.URL.Path, "releases/latest"):
					w.Header().Set("Content-Type", "application/json")
					json.NewEncoder(w).Encode(map[string]string{"tag_name": "v" + version})

				case strings.HasSuffix(r.URL.Path, "checksums.txt"):
					code := tt.checksumCode
					if code == 0 {
						code = http.StatusOK
					}
					w.WriteHeader(code)
					if tt.checksumBody != "" {
						fmt.Fprint(w, tt.checksumBody)
					}

				default:
					w.Header().Set("Content-Type", "application/octet-stream")
					w.WriteHeader(http.StatusOK)
					w.Write(tarContent)
				}
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

			tmpDir := t.TempDir()
			origInstallDirFn := engramInstallDirFn
			engramInstallDirFn = func(goos string) string { return tmpDir }
			t.Cleanup(func() { engramInstallDirFn = origInstallDirFn })

			profile := system.PlatformProfile{OS: "linux", PackageManager: "apt"}
			_, err := DownloadLatestBinary(profile, false)

			if tt.wantErrSubstr == "" {
				if err != nil {
					t.Fatalf("expected success, got error: %v", err)
				}
				return
			}

			if err == nil {
				t.Fatalf("expected error containing %q, got nil", tt.wantErrSubstr)
			}
			if !strings.Contains(err.Error(), tt.wantErrSubstr) {
				t.Errorf("error = %q, want it to contain %q", err.Error(), tt.wantErrSubstr)
			}
		})
	}
}

// TestFetchLatestEngramVersionWithAssetsPaginates asserts that
// fetchLatestEngramVersionWithAssets paginates beyond the first page when
// page 1 contains only non-core tags (pi-v* / gentle-engram) and the valid
// core vX.Y.Z release with a binary asset is on page 2.
//
// Regression test for the issue where per_page=20 capped discovery and returned
// "no engram release with downloadable binary assets found" even though a valid
// core release existed just beyond the window.
func TestFetchLatestEngramVersionWithAssetsPaginates(t *testing.T) {
	const binaryVersion = "1.17.0"

	tarContent := buildFakeTarGz(t, "engram")
	checksums := ""
	for _, goarch := range []string{"amd64", "arm64"} {
		checksums += makeChecksumsTxt(engramArchiveName(binaryVersion, "linux", goarch), tarContent)
	}

	// Track which pages were requested so we can assert pagination occurred.
	var pagesRequested []string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "releases/latest"):
			// /latest points at a pi-v* tag so the single-release path falls
			// through to fetchLatestEngramVersionWithAssets.
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]any{
				"tag_name": "pi-v0.2.0",
				"assets":   []any{},
			})

		case strings.Contains(r.URL.Path, "/repos/") && strings.Contains(r.URL.Path, "/releases") &&
			!strings.Contains(r.URL.Path, "/releases/download") &&
			!strings.Contains(r.URL.Path, "releases/latest"):
			// Release list endpoint — track the page parameter.
			page := r.URL.Query().Get("page")
			if page == "" {
				page = "1"
			}
			pagesRequested = append(pagesRequested, page)

			w.Header().Set("Content-Type", "application/json")
			switch page {
			case "1":
				// Page 1: only non-core tags — must NOT pick any of these.
				json.NewEncoder(w).Encode([]map[string]any{
					{
						"tag_name":   "pi-v0.2.0",
						"draft":      false,
						"prerelease": false,
						"assets":     []any{},
					},
					{
						"tag_name":   "gentle-engram v0.2.0",
						"draft":      false,
						"prerelease": false,
						"assets":     []any{},
					},
					{
						"tag_name":   "pi-v0.1.9",
						"draft":      false,
						"prerelease": false,
						"assets":     []any{},
					},
				})
			case "2":
				// Page 2: the real core engram release.
				json.NewEncoder(w).Encode([]map[string]any{
					{
						"tag_name":   "v" + binaryVersion,
						"draft":      false,
						"prerelease": false,
						"assets": []map[string]string{
							{"name": "checksums.txt"},
							{"name": "engram_" + binaryVersion + "_linux_amd64.tar.gz"},
						},
					},
				})
			default:
				// Any further page is empty — signals end of releases.
				json.NewEncoder(w).Encode([]map[string]any{})
			}

		case strings.HasSuffix(r.URL.Path, "checksums.txt"):
			w.Header().Set("Content-Type", "text/plain")
			fmt.Fprint(w, checksums)

		case strings.Contains(r.URL.Path, "/releases/download/v"+binaryVersion+"/engram_"+binaryVersion+"_linux_"):
			w.Header().Set("Content-Type", "application/octet-stream")
			w.WriteHeader(http.StatusOK)
			w.Write(tarContent)

		default:
			t.Fatalf("unexpected request: %s?%s", r.URL.Path, r.URL.RawQuery)
		}
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

	tmpDir := t.TempDir()
	origInstallDirFn := engramInstallDirFn
	engramInstallDirFn = func(goos string) string { return tmpDir }
	t.Cleanup(func() { engramInstallDirFn = origInstallDirFn })

	profile := system.PlatformProfile{OS: "linux", PackageManager: "apt"}
	installedPath, err := DownloadLatestBinary(profile, false)
	if err != nil {
		t.Fatalf("DownloadLatestBinary() error = %v, want core engram v%s selected from page 2", err, binaryVersion)
	}

	if _, err := os.Stat(installedPath); err != nil {
		t.Fatalf("stat installed binary: %v", err)
	}

	// Verify that pagination actually happened — both page 1 and page 2 must have been requested.
	gotPages := strings.Join(pagesRequested, ",")
	if !strings.Contains(gotPages, "1") || !strings.Contains(gotPages, "2") {
		t.Errorf("expected pagination across pages 1 and 2, got pages requested: [%s]", gotPages)
	}
}

// --- TestEngramExpectedChecksumFor ---
//
// Table-driven unit tests for the BSD-style checksums.txt parser.
func TestEngramExpectedChecksumFor(t *testing.T) {
	tests := []struct {
		name          string
		content       string
		filename      string
		wantDigest    string
		wantErrSubstr string
	}{
		{
			name:       "exact match returns digest",
			content:    "abc123  engram_1.0.0_linux_amd64.tar.gz\n",
			filename:   "engram_1.0.0_linux_amd64.tar.gz",
			wantDigest: "abc123",
		},
		{
			name: "finds correct entry among multiple",
			content: "aaa111  engram_1.0.0_linux_amd64.tar.gz\n" +
				"bbb222  engram_1.0.0_linux_arm64.tar.gz\n" +
				"ccc333  engram_1.0.0_windows_amd64.zip\n",
			filename:   "engram_1.0.0_linux_arm64.tar.gz",
			wantDigest: "bbb222",
		},
		{
			name:          "missing filename returns error",
			content:       "abc123  engram_1.0.0_linux_amd64.tar.gz\n",
			filename:      "engram_1.0.0_darwin_arm64.tar.gz",
			wantErrSubstr: "not listed in checksums.txt",
		},
		{
			name:          "empty content returns error",
			content:       "",
			filename:      "engram_1.0.0_linux_amd64.tar.gz",
			wantErrSubstr: "not listed in checksums.txt",
		},
		{
			name:          "malformed lines (single field) are skipped",
			content:       "justonefield\n",
			filename:      "justonefield",
			wantErrSubstr: "not listed in checksums.txt",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := engramExpectedChecksumFor(tt.content, tt.filename)

			if tt.wantErrSubstr != "" {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tt.wantErrSubstr)
				}
				if !strings.Contains(err.Error(), tt.wantErrSubstr) {
					t.Errorf("error = %q, want it to contain %q", err.Error(), tt.wantErrSubstr)
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.wantDigest {
				t.Errorf("got digest %q, want %q", got, tt.wantDigest)
			}
		})
	}
}

// --- TestDownloadLatestBinary_ChannelRouting (Slice 3) ---

// TestDownloadLatestBinary_StableChannelUsesRelease verifies that calling
// DownloadLatestBinary with isBeta=false fetches from the release download
// path (the existing GitHub Releases flow).
func TestDownloadLatestBinary_StableChannelUsesRelease(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("skipping on Windows: uses tar.gz server fixture")
	}

	server := makeServerWithFakeTarGz(t, "1.3.0")
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

	profile := system.PlatformProfile{OS: "linux", PackageManager: "apt"}
	// isBeta=false → should use the release download path.
	installedPath, err := DownloadLatestBinary(profile, false)
	if err != nil {
		t.Fatalf("DownloadLatestBinary(stable): unexpected error: %v", err)
	}
	if !strings.HasPrefix(installedPath, tmpDir) {
		t.Errorf("installedPath = %q, want prefix %q", installedPath, tmpDir)
	}
}

// TestDownloadLatestBinary_BetaChannelUsesGoInstallMain verifies that calling
// DownloadLatestBinary with isBeta=true performs go install @main instead of
// fetching a release archive.
func TestDownloadLatestBinary_BetaChannelUsesGoInstallMain(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping: requires go install command")
	}

	origGoInstallFn := engramGoInstallFn
	t.Cleanup(func() { engramGoInstallFn = origGoInstallFn })

	var gotPkg string
	engramGoInstallFn = func(pkg string) (string, error) {
		gotPkg = pkg
		return "/tmp/engram-beta", nil
	}

	profile := system.PlatformProfile{OS: "linux", PackageManager: "apt"}
	// isBeta=true → should use go install @main, not release download.
	installedPath, err := DownloadLatestBinary(profile, true)
	if err != nil {
		t.Fatalf("DownloadLatestBinary(beta): unexpected error: %v", err)
	}
	if !strings.Contains(gotPkg, "@main") {
		t.Errorf("go install pkg = %q, want @main suffix", gotPkg)
	}
	if installedPath == "" {
		t.Error("expected non-empty installedPath for beta channel")
	}
}

// TestEngramGoInstallFromMain_UsesGoEnvForBinDir verifies that
// engramGoInstallFromMain resolves the install directory via `go env GOBIN GOPATH`
// (the effective Go environment) rather than reading raw shell env vars.
// This matters when GOBIN is set via `go env -w GOBIN=...` (stored in Go's
// env file) but NOT exported into the shell environment.
func TestEngramGoInstallFromMain_UsesGoEnvForBinDir(t *testing.T) {
	const fakeInstallDir = "/custom/gobin/via/go-env"

	origGoEnvFn := engramGoEnvFn
	t.Cleanup(func() { engramGoEnvFn = origGoEnvFn })

	// Simulate GOBIN set only via `go env -w` (not in shell env).
	// The raw os.Getenv("GOBIN") would return "", but go env GOBIN returns
	// the persisted value from the Go env file.
	t.Setenv("GOBIN", "")
	t.Setenv("GOPATH", "")

	engramGoEnvFn = func(keys ...string) (map[string]string, error) {
		result := make(map[string]string, len(keys))
		for _, k := range keys {
			if k == "GOBIN" {
				result[k] = fakeInstallDir
			} else {
				result[k] = ""
			}
		}
		return result, nil
	}

	// Inject a fake go install that does nothing (no real network/build).
	origGoInstallCmdFn := engramGoInstallCmdFn
	t.Cleanup(func() { engramGoInstallCmdFn = origGoInstallCmdFn })
	engramGoInstallCmdFn = func(pkg string) error { return nil }

	binaryPath, err := engramGoInstallFromMain("github.com/Gentleman-Programming/engram/cmd/engram@main")
	if err != nil {
		t.Fatalf("engramGoInstallFromMain: unexpected error: %v", err)
	}

	wantDir := fakeInstallDir
	gotDir := filepath.Dir(binaryPath)
	if gotDir != wantDir {
		t.Errorf("binary dir = %q, want %q (from go env GOBIN)", gotDir, wantDir)
	}
}

func TestEngramGoInstallFromMain_BypassesPublicGoProxy(t *testing.T) {
	binDir := t.TempDir()
	goPath := filepath.Join(binDir, "go")
	recordPath := filepath.Join(t.TempDir(), "go-env.txt")
	fakeGo := filepath.Join(binDir, "go")
	script := "#!/usr/bin/env bash\n" +
		"printf 'GONOSUMDB=%s\\nGOPRIVATE=%s\\nGONOPROXY=%s\\n' \"${GONOSUMDB:-}\" \"${GOPRIVATE:-}\" \"${GONOPROXY:-}\" > \"$GO_ENV_RECORD\"\n"
	if err := os.WriteFile(fakeGo, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("GO_ENV_RECORD", recordPath)

	origGoEnvFn := engramGoEnvFn
	t.Cleanup(func() { engramGoEnvFn = origGoEnvFn })
	engramGoEnvFn = func(keys ...string) (map[string]string, error) {
		return map[string]string{"GOBIN": goPath, "GOPATH": filepath.Join(t.TempDir(), "gopath")}, nil
	}

	if _, err := engramGoInstallFromMain("github.com/Gentleman-Programming/engram/cmd/engram@main"); err != nil {
		t.Fatalf("engramGoInstallFromMain() error = %v", err)
	}

	recorded, err := os.ReadFile(recordPath)
	if err != nil {
		t.Fatalf("ReadFile(%q) error = %v", recordPath, err)
	}
	for _, want := range []string{
		"GONOSUMDB=github.com/Gentleman-Programming/engram",
		"GOPRIVATE=github.com/Gentleman-Programming/engram",
		"GONOPROXY=github.com/Gentleman-Programming/engram",
	} {
		if !strings.Contains(string(recorded), want) {
			t.Fatalf("go install env missing %q\nrecorded:\n%s", want, recorded)
		}
	}
}

// TestEngramStopScriptIsDefensive locks in the shape of the PowerShell stop
// script so the clean-install regression from issue #815 cannot reappear.
//
// On Windows PowerShell 5.1, piping an empty `Get-Process` result straight into
// Stop-Process flips `$?` and makes powershell.exe exit 1 with no output, which
// the Go installer surfaces as a bare "exit status 1 (output: )" and marks the
// whole engram component FAILED on a fresh machine. The fix is structural: guard
// the pipeline with `if ($procs)` and never use `-ErrorAction Stop`. Because the
// script only runs on real Windows (integration-covered on Windows CI), this
// unit test asserts the invariants on the generated string so a future "cleanup"
// that reintroduces the direct pipe or a terminating error action fails here.
func TestEngramStopScriptIsDefensive(t *testing.T) {
	script := engramStopScript()

	// The lookup must not treat a missing process as a terminating error.
	if !strings.Contains(script, "Get-Process -Name engram -ErrorAction SilentlyContinue") {
		t.Errorf("stop script must look up engram with -ErrorAction SilentlyContinue\nscript:\n%s", script)
	}

	// The Stop-Process pipeline must be guarded so it never runs on an empty
	// result — this is the core of the issue #815 fix.
	if !strings.Contains(script, "if ($procs)") {
		t.Errorf("stop script must guard Stop-Process behind `if ($procs)` (issue #815)\nscript:\n%s", script)
	}

	// -ErrorAction Stop on Stop-Process is exactly what produced exit 1 when the
	// pipeline was empty. It must never come back.
	if strings.Contains(script, "Stop-Process -Force -ErrorAction Stop") {
		t.Errorf("stop script must not use -ErrorAction Stop on Stop-Process (reintroduces issue #815/#850)\nscript:\n%s", script)
	}
}
