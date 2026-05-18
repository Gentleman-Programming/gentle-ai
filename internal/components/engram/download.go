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
	"regexp"
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
	goarch := normalizeArch(runtime.GOARCH)
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

// semverTagRe matches tags that are stable semver: vX.Y.Z with optional
// trailing metadata but no pre-release suffix (e.g. "v1.15.13" matches,
// "v0.1.0-alpha" and "pi-v0.1.7" do not).
var semverTagRe = regexp.MustCompile(`^v(\d+\.\d+\.\d+)$`)

// fetchLatestEngramVersion queries the GitHub Releases API to find the latest
// stable semver release of engram with downloadable assets.
// It ignores non-semver tags (e.g. "pi-v0.1.7") and pre-release/draft releases.
func fetchLatestEngramVersion() (string, error) {
	token := githubToken()
	version, status, err := fetchLatestEngramVersionRequest(token)
	if err == nil {
		return version, nil
	}

	// GitHub Actions injects a repository-scoped GITHUB_TOKEN into CI. When that
	// token is forwarded into our Linux E2E containers, the public engram releases
	// endpoint can respond 401/403 for a different repository. Retry anonymously
	// before failing because the release metadata is public.
	if token != "" && (status == http.StatusUnauthorized || status == http.StatusForbidden) {
		version, _, retryErr := fetchLatestEngramVersionRequest("")
		if retryErr == nil {
			return version, nil
		}
	}

	return "", err
}

// githubRelease represents the subset of the GitHub Release API response we need.
type githubRelease struct {
	TagName    string `json:"tag_name"`
	Prerelease bool   `json:"prerelease"`
	Draft      bool   `json:"draft"`
	Assets     []struct {
		Name string `json:"name"`
	} `json:"assets"`
}

// fetchLatestEngramVersionRequest lists releases from the GitHub API and returns
// the version string (without leading "v") of the first stable semver release
// that has at least one matching engram asset.
func fetchLatestEngramVersionRequest(token string) (string, int, error) {
	apiURL := fmt.Sprintf("%s/repos/%s/%s/releases",
		engramAPIBaseURL(), engramOwner, engramRepo)

	req, err := http.NewRequest(http.MethodGet, apiURL, nil)
	if err != nil {
		return "", 0, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := engramHTTPClient.Do(req)
	if err != nil {
		return "", 0, fmt.Errorf("call GitHub API: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", resp.StatusCode, fmt.Errorf("GitHub API returned HTTP %d", resp.StatusCode)
	}

	var releases []githubRelease
	if err := json.NewDecoder(resp.Body).Decode(&releases); err != nil {
		return "", resp.StatusCode, fmt.Errorf("decode releases JSON: %w", err)
	}

	for _, r := range releases {
		// Skip drafts and pre-releases.
		if r.Draft || r.Prerelease {
			continue
		}

		// Only accept stable semver tags like vX.Y.Z.
		m := semverTagRe.FindStringSubmatch(r.TagName)
		if m == nil {
			continue
		}

		version := m[1] // capture group: X.Y.Z without leading "v"

		// Verify the release has at least one engram asset.
		if !hasCompatibleAsset(r.Assets, version) {
			continue
		}

		return version, resp.StatusCode, nil
	}

	return "", resp.StatusCode, fmt.Errorf(
		"no stable engram release with compatible assets found (checked %d releases); "+
			"ensure the engram repo has a vX.Y.Z release with assets named engram_<version>_<os>_<arch>.(zip|tar.gz)",
		len(releases))
}

// hasCompatibleAsset reports whether the assets list contains at least one
// engram binary asset matching the expected naming convention:
//
//	engram_<version>_<os>_<arch>.(zip|tar.gz)
func hasCompatibleAsset(assets []struct {
	Name string `json:"name"`
}, version string) bool {
	prefix := engramName + "_" + version + "_"
	for _, a := range assets {
		if strings.HasPrefix(a.Name, prefix) &&
			(strings.HasSuffix(a.Name, ".zip") || strings.HasSuffix(a.Name, ".tar.gz")) {
			return true
		}
	}
	return false
}

// githubToken returns a GitHub API token from the environment, if available.
// Checks GITHUB_TOKEN first, then GH_TOKEN (used by the gh CLI).
func githubToken() string {
	if t := os.Getenv("GITHUB_TOKEN"); t != "" {
		return t
	}
	return os.Getenv("GH_TOKEN")
}

// normalizeArch maps Go's runtime.GOARCH to the architecture names used in
// engram release assets. Engram only publishes amd64 and arm64 binaries.
// If the current process runs as 386 (32-bit Go on a 64-bit system), we
// map to amd64 since engram doesn't publish 386 builds.
func normalizeArch(goarch string) string {
	switch goarch {
	case "386":
		return "amd64"
	case "arm":
		return "arm64"
	default:
		return goarch
	}
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

		if filepath.Base(hdr.Name) == binaryName &&
			(hdr.Typeflag == tar.TypeReg || hdr.Typeflag == tar.TypeRegA) {
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
// writeExecutable writes a binary to outPath using an atomic rename to avoid
// ETXTBSY ("text file busy") errors on Linux when the target binary is
// currently running (e.g. engram as an MCP server). The rename trick works
// because os.Rename replaces the directory entry — the running process keeps
// its open file descriptor to the old inode, while new executions pick up
// the new binary.
func writeExecutable(r io.Reader, outPath string) error {
	dir := filepath.Dir(outPath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create parent dir: %w", err)
	}

	// Write to a temp file in the same directory so Rename is always
	// same-filesystem (atomic on POSIX).
	tmp, err := os.CreateTemp(dir, ".engram-upgrade-*.tmp")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	tmpPath := tmp.Name()

	// Clean up on any failure path.
	defer func() {
		if tmpPath != "" {
			os.Remove(tmpPath)
		}
	}()

	if _, err := io.Copy(tmp, r); err != nil {
		tmp.Close()
		return fmt.Errorf("write %s: %w", tmpPath, err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temp file: %w", err)
	}

	if err := os.Chmod(tmpPath, 0o755); err != nil {
		return fmt.Errorf("chmod temp file: %w", err)
	}

	if err := os.Rename(tmpPath, outPath); err != nil {
		return fmt.Errorf("rename %s -> %s: %w", tmpPath, outPath, err)
	}

	// Rename succeeded — disarm the deferred cleanup.
	tmpPath = ""
	return nil
}
