package engram

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"

	"github.com/gentleman-programming/gentle-ai/internal/system"
)

const (
	engramOwner            = "Gentleman-Programming"
	engramRepo             = "engram"
	engramName             = "engram"
	engramCanonicalModule  = "github.com/Gentleman-Programming/engram"
	engramCanonicalPackage = engramCanonicalModule + "/cmd/engram"
)

// Package-level vars for testability.
var (
	engramHTTPClient      = &http.Client{Timeout: 5 * time.Minute}
	engramGitHubBaseURL   = "https://github.com"
	engramInstallDirFn    = engramInstallDir
	engramChecksumURLFn   = engramChecksumURL
	engramStopProcessesFn = stopEngramProcesses

	// engramGoInstallFn runs `go install <pkg>` and returns the path to the installed binary.
	// Package-level var for testability — swapped in tests to avoid real go install calls.
	engramGoInstallFn = engramGoInstallFromMain

	// engramGoInstallCmdFn executes `go install <pkg>`. Package-level var for testability.
	engramGoInstallCmdFn = func(pkg string) error {
		cmd := exec.Command("go", "install", pkg)
		cmd.Env = goPrivateModuleEnv(os.Environ(), engramCanonicalModule)
		cmd.Stdin = nil
		out, err := cmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("go install %s: %w (output: %s)", pkg, err, strings.TrimSpace(string(out)))
		}
		return nil
	}

	// engramGoEnvFn queries the Go toolchain's effective environment for the
	// given keys (e.g. "GOBIN", "GOPATH"). Package-level var for testability —
	// swapped in tests to simulate values set via `go env -w` without mutating
	// the real Go env file.
	engramGoEnvFn = func(keys ...string) (map[string]string, error) {
		args := append([]string{"env"}, keys...)
		out, err := exec.Command("go", args...).Output()
		if err != nil {
			return nil, err
		}
		lines := strings.Split(strings.TrimRight(string(out), "\r\n"), "\n")
		values := make(map[string]string, len(keys))
		for i, key := range keys {
			if i < len(lines) {
				values[key] = strings.TrimSpace(lines[i])
			}
		}
		return values, nil
	}
)

func goPrivateModuleEnv(base []string, modulePath string) []string {
	values := map[string]string{
		"GONOSUMDB": modulePath,
		"GOPRIVATE": modulePath,
		"GONOPROXY": modulePath,
	}
	merged := make([]string, 0, len(base)+3)
	for _, entry := range base {
		key, _, ok := strings.Cut(entry, "=")
		if ok {
			if required, managed := values[key]; managed {
				values[key] = appendGoEnvPattern(required, strings.TrimPrefix(entry, key+"="))
				continue
			}
		}
		merged = append(merged, entry)
	}
	return append(merged,
		"GONOSUMDB="+values["GONOSUMDB"],
		"GOPRIVATE="+values["GOPRIVATE"],
		"GONOPROXY="+values["GONOPROXY"],
	)
}

func appendGoEnvPattern(required, existing string) string {
	existing = strings.TrimSpace(existing)
	if existing == "" {
		return required
	}
	for _, part := range strings.Split(existing, ",") {
		if strings.TrimSpace(part) == required {
			return existing
		}
	}
	return existing + "," + required
}

func canonicalEngramGoInstallPackage(pkg string) string {
	const lowerPackage = "github.com/gentleman-programming/engram/cmd/engram"
	if strings.HasPrefix(strings.ToLower(pkg), lowerPackage) {
		return engramCanonicalPackage + pkg[len(lowerPackage):]
	}
	return pkg
}

// engramCoreTagPattern matches only plain semver tags (vX.Y.Z) that identify
// core engram binary releases. The Gentleman-Programming/engram repository also
// publishes gentle-engram npm and pi releases under tags like
// "gentle-engram vX.Y.Z" or "pi-vX.Y.Z" in the same release stream. This
// pattern intentionally excludes those so a gentle-engram/pi tag can never be
// selected as the core engram binary version. It mirrors the ReleaseTagPattern
// used by the update-check path in internal/update/registry.go.
const engramCoreTagPattern = `^v[0-9]+\.[0-9]+\.[0-9]+$`

// DownloadLatestBinary fetches the latest engram release from GitHub and
// installs it to the appropriate directory for the given platform.
// It returns the full path to the installed binary.
//
// When isBeta is true, engram is installed from source via `go install @main`
// instead of downloading a release archive. This mirrors the install-time beta
// path used by the CLI and ensures the upgrade executor honors GENTLE_AI_CHANNEL.
//
// Checksum verification is mandatory for the stable (release) path: the install
// fails if checksums.txt is unavailable, if the archive is not listed, or if
// the digest does not match.
//
// This is the non-brew installation method for Linux and Windows.
// On macOS, brew handles engram transitively and this should not be called.
func DownloadLatestBinary(profile system.PlatformProfile, isBeta bool) (string, error) {
	// Beta channel: install from HEAD via go install rather than a release archive.
	// This mirrors the installBetaEngramFromMain path used at install time.
	if isBeta {
		return engramGoInstallFn(engramCanonicalPackage + "@main")
	}

	ctx := context.Background()

	// 1. Fetch the latest version tag from GitHub API. Only tags matching the
	// core engram pattern (vX.Y.Z) are considered; gentle-engram/pi tags are
	// excluded so the download and update-check paths share the same source of truth.
	version, err := fetchLatestEngramVersion()
	if err != nil {
		return "", fmt.Errorf("fetch latest engram version: %w", err)
	}

	// 2. Determine binary name and archive URL.
	goos := profile.OS
	goarch := normalizeArch(runtime.GOARCH)
	assetURL := engramAssetURL(engramGitHubBaseURL, version, goos, goarch)
	archiveName := engramArchiveName(version, goos, goarch)
	checksumURL := engramChecksumURLFn(engramGitHubBaseURL, version)

	// 3. Determine install directory.
	installDir := engramInstallDirFn(goos)
	if err := os.MkdirAll(installDir, 0o755); err != nil {
		return "", fmt.Errorf("create engram install dir %q: %w", installDir, err)
	}

	// 4. Download archive to a temp dir so we can verify before extracting.
	binaryName := engramName
	if goos == "windows" {
		binaryName = engramName + ".exe"
	}
	outPath := filepath.Join(installDir, binaryName)

	tmpDir, err := os.MkdirTemp("", "gentle-ai-engram-*")
	if err != nil {
		return "", fmt.Errorf("create temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	archivePath := filepath.Join(tmpDir, archiveName)
	actualDigest, err := engramDownloadToFile(ctx, assetURL, archivePath)
	if err != nil {
		return "", fmt.Errorf("download engram archive: %w", err)
	}

	// 5. Verify checksum — fail closed if checksums.txt is unavailable or mismatched.
	checksumsContent, err := engramFetchChecksums(ctx, checksumURL)
	if err != nil {
		return "", fmt.Errorf("checksum verification failed: checksums.txt unavailable: %w", err)
	}
	expectedDigest, err := engramExpectedChecksumFor(checksumsContent, archiveName)
	if err != nil {
		return "", fmt.Errorf("checksum verification failed: %w", err)
	}
	if actualDigest != expectedDigest {
		return "", fmt.Errorf("checksum mismatch for %s:\n  expected: %s\n  got:      %s",
			archiveName, expectedDigest, actualDigest)
	}

	// 6. On Windows, stop running Engram processes before replacing engram.exe.
	// Windows locks running executables, unlike POSIX where atomic rename can
	// replace the directory entry while the old process keeps its inode.
	if goos == "windows" {
		if err := engramStopProcessesFn(); err != nil {
			return "", fmt.Errorf("stop running engram processes before upgrade: %w", err)
		}
	}

	// 7. Extract the verified binary.
	f, err := os.Open(archivePath)
	if err != nil {
		return "", fmt.Errorf("open archive: %w", err)
	}
	defer f.Close()

	if strings.HasSuffix(assetURL, ".zip") {
		data, err := io.ReadAll(f)
		if err != nil {
			return "", fmt.Errorf("read zip archive: %w", err)
		}
		if err := extractZipBinary(data, binaryName, outPath); err != nil {
			return "", fmt.Errorf("extract engram zip: %w", err)
		}
	} else {
		if err := extractBinaryFromTarGz(f, engramName, outPath); err != nil {
			return "", fmt.Errorf("extract engram tar.gz: %w", err)
		}
	}

	return outPath, nil
}

// engramCoreTagRE is the compiled form of engramCoreTagPattern, used to filter
// GitHub release tags so only core engram binary releases are selected.
var engramCoreTagRE = regexp.MustCompile(engramCoreTagPattern)

// fetchLatestEngramVersion queries the GitHub Releases API for the latest
// core engram binary release and returns the version string (without leading "v").
// Only releases whose tag matches engramCoreTagPattern (vX.Y.Z) are considered,
// so gentle-engram/pi tags published in the same release stream are ignored.
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

func fetchLatestEngramVersionRequest(token string) (string, int, error) {
	apiURL := fmt.Sprintf("%s/repos/%s/%s/releases/latest",
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

	var release struct {
		TagName string             `json:"tag_name"`
		Assets  *[]json.RawMessage `json:"assets"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return "", resp.StatusCode, fmt.Errorf("decode release JSON: %w", err)
	}

	// Reject tags that don't match the core engram pattern (e.g. gentle-engram/pi-v* tags).
	// Fall through to the release-list scan when /latest points at a non-core release.
	if !engramCoreTagRE.MatchString(release.TagName) {
		fallbackVersion, fallbackStatus, err := fetchLatestEngramVersionWithAssets(token)
		if err == nil {
			return fallbackVersion, resp.StatusCode, nil
		}
		if token != "" && (fallbackStatus == http.StatusUnauthorized || fallbackStatus == http.StatusForbidden) {
			fallbackVersion, _, retryErr := fetchLatestEngramVersionWithAssets("")
			if retryErr == nil {
				return fallbackVersion, resp.StatusCode, nil
			}
		}
		return "", resp.StatusCode, err
	}

	version := strings.TrimPrefix(release.TagName, "v")
	if version == "" {
		return "", resp.StatusCode, fmt.Errorf("empty tag_name in GitHub release response")
	}

	// Older tests and non-GitHub-compatible mocks may omit assets entirely; in
	// that case keep the historical latest-release behavior. GitHub returns an
	// explicit assets array, so skip releases that do not publish core engram
	// binaries (for example pi-v* gentle-engram package releases, which are
	// separate from core engram binary releases).
	if release.Assets != nil && !hasEngramBinaryAsset(*release.Assets) {
		fallbackVersion, fallbackStatus, err := fetchLatestEngramVersionWithAssets(token)
		if err == nil {
			return fallbackVersion, resp.StatusCode, nil
		}
		if token != "" && (fallbackStatus == http.StatusUnauthorized || fallbackStatus == http.StatusForbidden) {
			fallbackVersion, _, retryErr := fetchLatestEngramVersionWithAssets("")
			if retryErr == nil {
				return fallbackVersion, resp.StatusCode, nil
			}
		}
		return "", resp.StatusCode, err
	}

	return version, resp.StatusCode, nil
}

func hasEngramBinaryAsset(assets []json.RawMessage) bool {
	for _, raw := range assets {
		var asset struct {
			Name string `json:"name"`
		}
		if err := json.Unmarshal(raw, &asset); err == nil && strings.HasPrefix(asset.Name, engramRepo+"_") {
			return true
		}
	}
	return false
}

// engramReleasePageSize is the number of releases requested per page when
// paginating the GitHub Releases list. GitHub's maximum is 100; 20 is
// sufficient for typical cadence while keeping response payloads small.
const engramReleasePageSize = 20

// engramReleaseMaxPages caps the pagination loop so it can never run forever.
// At 20 releases/page this covers 100 releases — enough runway even when the
// Gentleman-Programming/engram repo publishes many pi-v*/gentle-engram entries
// between core vX.Y.Z releases.
const engramReleaseMaxPages = 5

func fetchLatestEngramVersionWithAssets(token string) (string, int, error) {
	lastStatus := 0

	for page := 1; page <= engramReleaseMaxPages; page++ {
		apiURL := fmt.Sprintf("%s/repos/%s/%s/releases?per_page=%d&page=%d",
			engramAPIBaseURL(), engramOwner, engramRepo, engramReleasePageSize, page)

		req, err := http.NewRequest(http.MethodGet, apiURL, nil)
		if err != nil {
			return "", 0, fmt.Errorf("build releases request: %w", err)
		}
		req.Header.Set("Accept", "application/vnd.github+json")
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}

		resp, err := engramHTTPClient.Do(req)
		if err != nil {
			return "", lastStatus, fmt.Errorf("call GitHub releases API: %w", err)
		}

		if resp.StatusCode != http.StatusOK {
			status := resp.StatusCode
			resp.Body.Close()
			return "", status, fmt.Errorf("GitHub releases API returned HTTP %d", status)
		}
		lastStatus = resp.StatusCode

		var releases []struct {
			TagName    string `json:"tag_name"`
			Draft      bool   `json:"draft"`
			Prerelease bool   `json:"prerelease"`
			Assets     []struct {
				Name string `json:"name"`
			} `json:"assets"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&releases); err != nil {
			resp.Body.Close()
			return "", lastStatus, fmt.Errorf("decode releases JSON: %w", err)
		}
		resp.Body.Close()

		// An empty page means GitHub has no more releases — stop early.
		if len(releases) == 0 {
			break
		}

		for _, release := range releases {
			if release.Draft || release.Prerelease || len(release.Assets) == 0 {
				continue
			}
			// Skip tags that don't match the core engram pattern (e.g. gentle-engram/pi-v* tags).
			if !engramCoreTagRE.MatchString(release.TagName) {
				continue
			}
			for _, asset := range release.Assets {
				if strings.HasPrefix(asset.Name, engramRepo+"_") {
					version := strings.TrimPrefix(release.TagName, "v")
					if version != "" {
						return version, lastStatus, nil
					}
				}
			}
		}
	}

	return "", lastStatus, fmt.Errorf("no engram release with downloadable binary assets found")
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

// engramArchiveName returns the GoReleaser archive filename for the given
// version/os/arch combination.
//
// Convention: engram_{version}_{os}_{arch}.tar.gz (or .zip on Windows)
func engramArchiveName(version, goos, goarch string) string {
	ext := ".tar.gz"
	if goos == "windows" {
		ext = ".zip"
	}
	return fmt.Sprintf("%s_%s_%s_%s%s", engramRepo, version, goos, goarch, ext)
}

// engramAssetURL constructs the download URL for the engram release asset.
func engramAssetURL(baseURL, version, goos, goarch string) string {
	filename := engramArchiveName(version, goos, goarch)
	return fmt.Sprintf("%s/%s/%s/releases/download/v%s/%s",
		baseURL, engramOwner, engramRepo, version, filename)
}

// engramChecksumURL constructs the GitHub Releases URL for checksums.txt.
func engramChecksumURL(baseURL, version string) string {
	return fmt.Sprintf("%s/%s/%s/releases/download/v%s/checksums.txt",
		baseURL, engramOwner, engramRepo, version)
}

// engramDownloadToFile downloads the resource at url to outPath and returns
// the SHA256 hex digest of the downloaded content.
func engramDownloadToFile(ctx context.Context, url string, outPath string) (hexDigest string, err error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", fmt.Errorf("build request: %w", err)
	}
	resp, err := engramHTTPClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("download %s: %w", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("download %s: HTTP %d", url, resp.StatusCode)
	}

	if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
		return "", fmt.Errorf("create dir: %w", err)
	}
	f, err := os.Create(outPath)
	if err != nil {
		return "", fmt.Errorf("create %s: %w", outPath, err)
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(io.MultiWriter(f, h), resp.Body); err != nil {
		return "", fmt.Errorf("write %s: %w", outPath, err)
	}

	return hex.EncodeToString(h.Sum(nil)), nil
}

// engramFetchChecksums downloads checksums.txt from url and returns its content.
// Returns an error if the file cannot be fetched or the server returns non-200.
func engramFetchChecksums(ctx context.Context, url string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", fmt.Errorf("build request: %w", err)
	}
	resp, err := engramHTTPClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("fetch checksums.txt: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("checksums.txt: HTTP %d", resp.StatusCode)
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read checksums.txt: %w", err)
	}
	return string(data), nil
}

// engramExpectedChecksumFor parses checksums.txt content and returns the SHA256
// hex digest for filename. Returns an error if the filename is not listed.
//
// GoReleaser produces BSD-style checksums.txt: "<digest>  <filename>" per line.
func engramExpectedChecksumFor(content, filename string) (string, error) {
	for _, line := range strings.Split(content, "\n") {
		fields := strings.Fields(line)
		if len(fields) == 2 && fields[1] == filename {
			return fields[0], nil
		}
	}
	return "", fmt.Errorf("%q not listed in checksums.txt", filename)
}

// extractZipBinary extracts the binary named binaryName from the zip data
// and writes it to outPath.
func extractZipBinary(data []byte, binaryName, outPath string) error {
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

// engramStopScript returns the PowerShell script that stops running Engram
// processes so Windows can replace engram.exe during an upgrade.
//
// The script is written defensively so a clean install (no engram process
// running) is a clean no-op instead of a failure:
//  1. Get-Process uses -ErrorAction SilentlyContinue, so a missing process is
//     not a terminating error.
//  2. The Stop-Process call is guarded by `if ($procs)`, so the pipeline never
//     runs on an empty result. This matters on Windows PowerShell 5.1, where
//     piping an empty Get-Process result still flips `$?` and makes
//     powershell.exe exit 1 with empty output — the clean-install regression in
//     issue #815.
//  3. Stop-Process uses -ErrorAction SilentlyContinue (never -ErrorAction Stop),
//     so an access-denied condition (e.g. the binary is held by the running
//     editor session) does not abort the install.
//  4. If processes were found but could not all be stopped, the script emits a
//     WARNING line so the caller can surface it without failing the install.
func engramStopScript() string {
	return `
$procs = Get-Process -Name engram -ErrorAction SilentlyContinue
if ($procs) {
    $procs | Stop-Process -Force -ErrorAction SilentlyContinue
    $remaining = Get-Process -Name engram -ErrorAction SilentlyContinue
    if ($remaining) {
        Write-Output "WARNING: $($remaining.Count) engram process(es) could not be stopped (access denied or still running). The upgrade may fail if the file is still locked."
    }
}
exit 0
`
}

// stopEngramProcesses runs the defensive stop script (see engramStopScript) and
// returns a non-nil error only when powershell.exe itself fails to launch or
// exits non-zero. A WARNING line (processes found but not all stopped) is
// surfaced to stderr but is treated as non-fatal.
func stopEngramProcesses() error {
	cmd := exec.Command("powershell.exe",
		"-NoProfile",
		"-NonInteractive",
		"-Command",
		engramStopScript(),
	)
	cmd.Stdin = nil
	out, err := cmd.CombinedOutput()
	if err != nil {
		// powershell itself failed to launch or returned non-zero despite
		// our SilentlyContinue guards — surface the raw output so the user
		// has something actionable.
		return fmt.Errorf("powershell Stop-Process engram: %w (output: %s)", err, strings.TrimSpace(string(out)))
	}
	// If the script emitted a WARNING line, surface it but do not fail.
	// The caller decides whether to abort based on the returned error being nil.
	msg := strings.TrimSpace(string(out))
	if strings.HasPrefix(msg, "WARNING:") {
		// Non-fatal: log to stderr so operators can diagnose, but return nil.
		fmt.Fprintf(os.Stderr, "gentle-ai: engram stop: %s\n", msg)
	}
	return nil
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

// engramGoInstallFromMain installs engram from the given Go package path (expected
// to be engramCanonicalPackage + "@main") using `go install`.
// It returns the path to the installed binary. This is the beta-channel upgrade path.
//
// The install directory is resolved via `go env GOBIN GOPATH` (the effective Go
// environment) so that values set via `go env -w GOBIN=...` (stored in Go's env
// file, NOT in shell env) are honored correctly. This mirrors the resolution done
// by goInstallBinDirFromGoEnv in internal/cli/run.go.
func engramGoInstallFromMain(pkg string) (string, error) {
	pkg = canonicalEngramGoInstallPackage(pkg)
	if err := engramGoInstallCmdFn(pkg); err != nil {
		return "", err
	}

	// Resolve the directory where `go install` placed the binary using the
	// effective Go environment (honors `go env -w GOBIN=...`, not just shell env).
	values, err := engramGoEnvFn("GOBIN", "GOPATH")
	if err != nil {
		return "", fmt.Errorf("resolve go install bin dir: %w", err)
	}
	gobin := strings.TrimSpace(values["GOBIN"])
	gopath := strings.TrimSpace(values["GOPATH"])

	if gobin == "" && gopath != "" {
		gobin = filepath.Join(gopath, "bin")
	}
	if gobin == "" {
		home, _ := os.UserHomeDir()
		gobin = filepath.Join(home, "go", "bin")
	}

	binaryName := engramName
	if runtime.GOOS == "windows" {
		binaryName += ".exe"
	}
	return filepath.Join(gobin, binaryName), nil
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
