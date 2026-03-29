package engram

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/gentleman-programming/gentle-ai/internal/system"
)

const (
	engramOwner = "Gentleman-Programming"
	engramRepo  = "engram"
	engramName  = "engram"
)

// Package-level vars for testability.
var (
	engramHTTPClient    = &http.Client{Timeout: 5 * time.Minute}
	engramGitHubBaseURL = "https://github.com"
	engramInstallDirFn  = engramInstallDir
)

// DownloadLatestBinary fetches the latest engram release from GitHub and
// installs it to the appropriate directory for the given platform.
// It returns the full path to the installed binary.
//
// This is the non-brew installation method for Linux and Windows.
// On macOS, brew handles engram transitively and this should not be called.
func DownloadLatestBinary(profile system.PlatformProfile) (string, error) {
	// 1. Fetch the latest version tag from GitHub API.
	version, err := fetchLatestEngramVersion()
	if err != nil {
		return "", fmt.Errorf("fetch latest engram version: %w", err)
	}

	// 2. Determine binary name and archive URL.
	goos := profile.OS
	goarch := runtime.GOARCH
	assetURL := engramAssetURL(engramGitHubBaseURL, version, goos, goarch)

	// 3. Determine install directory.
	installDir := engramInstallDirFn(goos)
	if err := os.MkdirAll(installDir, 0o755); err != nil {
		return "", fmt.Errorf("create engram install dir %q: %w", installDir, err)
	}

	// 4. Download and extract binary.
	binaryName := engramName
	if goos == "windows" {
		binaryName = engramName + ".exe"
	}
	outPath := filepath.Join(installDir, binaryName)

	if strings.HasSuffix(assetURL, ".zip") {
		if err := downloadAndExtractZip(assetURL, binaryName, outPath); err != nil {
			return "", fmt.Errorf("download engram zip: %w", err)
		}
	} else {
		if err := downloadAndExtractTarGz(assetURL, engramName, outPath); err != nil {
			return "", fmt.Errorf("download engram tar.gz: %w", err)
		}
	}

	return outPath, nil
}

// fetchLatestEngramVersion queries the GitHub Releases API for the latest engram
// release and returns the version string (without leading "v").
func fetchLatestEngramVersion() (string, error) {
	apiURL := fmt.Sprintf("%s/repos/%s/%s/releases/latest",
		engramAPIBaseURL(), engramOwner, engramRepo)

	req, err := http.NewRequest(http.MethodGet, apiURL, nil)
	if err != nil {
		return "", fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	if token := githubToken(); token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := engramHTTPClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("call GitHub API: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("GitHub API returned HTTP %d", resp.StatusCode)
	}

	var release struct {
		TagName string `json:"tag_name"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return "", fmt.Errorf("decode release JSON: %w", err)
	}

	version := strings.TrimPrefix(release.TagName, "v")
	if version == "" {
		return "", fmt.Errorf("empty tag_name in GitHub release response")
	}

	return version, nil
}

// githubToken returns a GitHub API token from the environment, if available.
// Checks GITHUB_TOKEN first, then GH_TOKEN (used by the gh CLI).
func githubToken() string {
	if t := os.Getenv("GITHUB_TOKEN"); t != "" {
		return t
	}
	return os.Getenv("GH_TOKEN")
}

// engramAPIBaseURL returns the GitHub API base URL for fetching release info.
// In tests, the mock server handles both API and download under the same URL,
// so we derive the API base from engramGitHubBaseURL.
func engramAPIBaseURL() string {
	base := engramGitHubBaseURL
	if strings.Contains(base, "127.0.0.1") || strings.Contains(base, "localhost") {
		return base
	}
	return "https://api.github.com"
}

// engramAssetURL constructs the download URL for the engram release asset.
func engramAssetURL(baseURL, version, goos, goarch string) string {
	ext := ".tar.gz"
	if goos == "windows" {
		ext = ".zip"
	}
	filename := fmt.Sprintf("%s_%s_%s_%s%s", engramRepo, version, goos, goarch, ext)
	return fmt.Sprintf("%s/%s/%s/releases/download/v%s/%s",
		baseURL, engramOwner, engramRepo, version, filename)
}

// engramInstallDir returns the directory where the engram binary should be installed
// for the given OS.
//   - Linux/macOS: /usr/local/bin (fallback: ~/.local/bin if not writable)
//   - Windows: %LOCALAPPDATA%\engram\bin
func engramInstallDir(goos string) string {
	if goos == "windows" {
		localAppData := os.Getenv("LOCALAPPDATA")
		if localAppData == "" {
			home, _ := os.UserHomeDir()
			localAppData = filepath.Join(home, "AppData", "Local")
		}
		return filepath.Join(localAppData, "engram", "bin")
	}

	// Linux/macOS: try /usr/local/bin first.
	candidate := "/usr/local/bin"
	if isWritableDir(candidate) {
		return candidate
	}

	// Fallback to ~/.local/bin.
	home, err := os.UserHomeDir()
	if err != nil {
		return "/usr/local/bin"
	}
	return filepath.Join(home, ".local", "bin")
}

// isWritableDir reports whether the directory exists and the process can write to it.
func isWritableDir(dir string) bool {
	info, err := os.Stat(dir)
	if err != nil || !info.IsDir() {
		return false
	}
	tmp, err := os.CreateTemp(dir, ".engram-write-test-*")
	if err != nil {
		return false
	}
	tmp.Close()
	os.Remove(tmp.Name())
	return true
}

// downloadAndExtractTarGz downloads the asset at url, extracts the binary named binaryName,
// and writes it to outPath with executable permissions.
func downloadAndExtractTarGz(url, binaryName, outPath string) error {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}

	resp, err := engramHTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("download %s: %w", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download %s: HTTP %d", url, resp.StatusCode)
	}

	return extractBinaryFromTarGz(resp.Body, binaryName, outPath)
}

// extractBinaryFromTarGz reads a .tar.gz stream and extracts the first file
// whose base name matches binaryName, writing it to outPath.
func extractBinaryFromTarGz(r io.Reader, binaryName, outPath string) error {
	gr, err := gzip.NewReader(r)
	if err != nil {
		return fmt.Errorf("open gzip: %w", err)
	}
	defer gr.Close()

	tr := tar.NewReader(gr)

	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("read tar: %w", err)
		}

		if filepath.Base(hdr.Name) == binaryName && hdr.Typeflag != tar.TypeDir {
			return writeExecutable(tr, outPath)
		}
	}

	return fmt.Errorf("binary %q not found in archive", binaryName)
}

// downloadAndExtractZip downloads the asset at url, extracts the binary named binaryName
// from the .zip archive, and writes it to outPath.
func downloadAndExtractZip(url, binaryName, outPath string) error {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}

	resp, err := engramHTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("download %s: %w", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download %s: HTTP %d", url, resp.StatusCode)
	}

	// zip.NewReader requires io.ReaderAt + size; read the entire body first.
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read response body: %w", err)
	}

	zr, err := zip.NewReader(&byteReaderAt{data: data}, int64(len(data)))
	if err != nil {
		return fmt.Errorf("open zip: %w", err)
	}

	for _, f := range zr.File {
		if filepath.Base(f.Name) == binaryName && !f.FileInfo().IsDir() {
			rc, err := f.Open()
			if err != nil {
				return fmt.Errorf("open zip entry %q: %w", f.Name, err)
			}
			defer rc.Close()
			return writeExecutable(rc, outPath)
		}
	}

	return fmt.Errorf("binary %q not found in zip archive", binaryName)
}

// byteReaderAt implements io.ReaderAt over a byte slice.
type byteReaderAt struct {
	data []byte
}

func (b *byteReaderAt) ReadAt(p []byte, off int64) (int, error) {
	if off < 0 || int(off) >= len(b.data) {
		return 0, io.EOF
	}
	n := copy(p, b.data[off:])
	if n < len(p) {
		return n, io.EOF
	}
	return n, nil
}

// writeExecutable writes the content from r to outPath with executable permissions.
func writeExecutable(r io.Reader, outPath string) error {
	if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
		return fmt.Errorf("create parent dir: %w", err)
	}

	f, err := os.OpenFile(outPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
	if err != nil {
		return fmt.Errorf("create %s: %w", outPath, err)
	}
	defer f.Close()

	if _, err := io.Copy(f, r); err != nil {
		return fmt.Errorf("write %s: %w", outPath, err)
	}

	return nil
}
