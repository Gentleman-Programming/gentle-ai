package upgrade

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
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

	minisign "github.com/jedisct1/go-minisign"

	"github.com/gentleman-programming/gentle-ai/v2/internal/system"
	"github.com/gentleman-programming/gentle-ai/v2/internal/update"
)

// httpClient is the HTTP client used for asset downloads.
// Package-level var for testability.
var httpClient = &http.Client{Timeout: 5 * time.Minute}

// lookPathFn resolves the binary path. Package-level var for testability.
var lookPathFn = exec.LookPath

// URL builders are package variables so tests can route release traffic to an
// isolated server without weakening the production URL contract.
// Package-level vars for testability.
var (
	resolveAssetURLFn     = resolveAssetURL
	resolveChecksumURLFn  = resolveChecksumURL
	resolveSignatureURLFn = resolveSignatureURL
)

const (
	maxChecksumManifestBytes         = 1 << 20 // 1 MiB
	maxChecksumSignatureBytes        = 16 << 10
	maxReleaseArchiveBytes           = 128 << 20 // 128 MiB
	unsetReleaseMinisignPublicKeys   = "UNSET"
	legacyZeroMinisignKeyPlaceholder = "0000000000000000000000000000000000000000000000000000000000000000"
)

// releaseMinisignPublicKeys is the production trust-anchor injection point.
// GoReleaser sets it with:
//
//	-X github.com/gentleman-programming/gentle-ai/v2/internal/update/upgrade.releaseMinisignPublicKeys=${MINISIGN_PUBLIC_KEYS}
//
// The value is one or two comma-separated minisign public-key payloads (the
// base64 line accepted by `minisign -P`). Two keys permit a bounded overlap
// during rotation. Source and test builds deliberately retain UNSET and fail
// closed if their binary updater is invoked.
var releaseMinisignPublicKeys = unsetReleaseMinisignPublicKeys

var (
	ErrReleaseTrustUnavailable     = errors.New("release trust anchor unavailable")
	ErrSignatureVerificationFailed = errors.New("release signature verification failed")
	ErrSignatureBlobMalformed      = errors.New("release signature is malformed")
	ErrSignatureIdentityMismatch   = errors.New("release signature identity mismatch")
)

var (
	exactReleaseVersionPattern = regexp.MustCompile(`^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$`)
	githubSlugPattern          = regexp.MustCompile(`^[A-Za-z0-9_.-]+$`)
	canonicalPublicKeysPattern = regexp.MustCompile(`^[A-Za-z0-9+/]{56}(,[A-Za-z0-9+/]{56})?$`)
)

// Download downloads the GitHub release binary for the given tool, verifies its
// SHA256 checksum against an authenticated checksums.txt, and replaces the
// installed binary atomically.
//
// Verification order is deliberate: validate the configured trust anchor,
// fetch the bounded manifest and detached signature, authenticate both the
// manifest and its exact repository/tag binding, parse one unique archive
// digest, then download/check/extract and finally replace the binary.
//
// This function is not called on Windows — callers (strategy.go) gate it via
// platform check and return a manual fallback error instead.
func Download(ctx context.Context, r update.UpdateResult, profile system.PlatformProfile) error {
	if profile.OS == "windows" {
		hint := r.UpdateHint
		if hint == "" {
			hint = fmt.Sprintf("Download from https://github.com/%s/%s/releases", r.Tool.Owner, r.Tool.Repo)
		}
		return fmt.Errorf("upgrade %q on Windows requires manual update — %s", r.Tool.Name, hint)
	}
	if err := validateReleaseIdentity(r.Tool.Owner, r.Tool.Repo, r.LatestVersion); err != nil {
		return fmt.Errorf("authenticate release: %w", err)
	}
	if _, err := configuredReleasePublicKeys(); err != nil {
		return fmt.Errorf("authenticate release: %w", err)
	}

	// Resolve the current binary path from PATH.
	binaryPath, err := lookPathFn(r.Tool.Name)
	if err != nil {
		return fmt.Errorf("locate %q binary: %w", r.Tool.Name, err)
	}

	archiveName := resolveArchiveName(r.Tool.Repo, r.LatestVersion, profile.OS, runtime.GOARCH)
	assetURL := resolveAssetURLFn(r.Tool.Owner, r.Tool.Repo, r.LatestVersion, profile.OS, runtime.GOARCH)
	checksumURL := resolveChecksumURLFn(r.Tool.Owner, r.Tool.Repo, r.LatestVersion)
	signatureURL := resolveSignatureURLFn(r.Tool.Owner, r.Tool.Repo, r.LatestVersion)

	checksumsContent, err := fetchChecksums(ctx, checksumURL)
	if err != nil {
		return fmt.Errorf("authenticate release: fetch checksums.txt: %w", err)
	}
	signature, err := fetchChecksumSignature(ctx, signatureURL)
	if err != nil {
		return fmt.Errorf("authenticate release: fetch checksums.txt.minisig: %w", err)
	}
	if err := verifyChecksumsSignature([]byte(checksumsContent), signature, r.Tool.Owner, r.Tool.Repo, r.LatestVersion); err != nil {
		return fmt.Errorf("authenticate release: %w", err)
	}
	expectedDigest, err := expectedChecksumFor(checksumsContent, archiveName)
	if err != nil {
		return fmt.Errorf("checksum verification failed: %w", err)
	}

	// Download only after the signed manifest has been authenticated.
	tmpDir, err := os.MkdirTemp("", "gentle-ai-upgrade-*")
	if err != nil {
		return fmt.Errorf("create temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	archivePath := filepath.Join(tmpDir, archiveName)
	actualDigest, err := downloadToFile(ctx, assetURL, archivePath, maxReleaseArchiveBytes)
	if err != nil {
		return fmt.Errorf("download %s: %w", r.Tool.Name, err)
	}
	if actualDigest != expectedDigest {
		return fmt.Errorf("checksum mismatch for %s:\n  expected: %s\n  got:      %s",
			archiveName, expectedDigest, actualDigest)
	}

	// Extract the verified binary.
	tmpBinaryPath := binaryPath + ".new"
	defer os.Remove(tmpBinaryPath)
	f, err := os.Open(archivePath)
	if err != nil {
		return fmt.Errorf("open archive: %w", err)
	}
	defer f.Close()

	if err := extractBinaryFromTarGz(f, r.Tool.Name, tmpBinaryPath); err != nil {
		return fmt.Errorf("extract %s: %w", r.Tool.Name, err)
	}

	// Atomic replace.
	if err := atomicReplace(tmpBinaryPath, binaryPath); err != nil {
		return fmt.Errorf("replace %q: %w", binaryPath, err)
	}

	return nil
}

// resolveArchiveName returns the GoReleaser archive filename for the given
// repo/version/os/arch combination.
//
// Convention: {repo}_{version}_{os}_{arch}.tar.gz
func resolveArchiveName(repo, version, goos, goarch string) string {
	return fmt.Sprintf("%s_%s_%s_%s.tar.gz", repo, version, goos, goarch)
}

// resolveAssetURL constructs the GitHub Releases asset download URL.
func resolveAssetURL(owner, repo, version, goos, goarch string) string {
	filename := resolveArchiveName(repo, version, goos, goarch)
	return fmt.Sprintf("https://github.com/%s/%s/releases/download/v%s/%s",
		owner, repo, version, filename)
}

// resolveChecksumURL constructs the GitHub Releases URL for checksums.txt.
func resolveChecksumURL(owner, repo, version string) string {
	return fmt.Sprintf("https://github.com/%s/%s/releases/download/v%s/checksums.txt",
		owner, repo, version)
}

// resolveSignatureURL constructs the URL for the detached signature over the
// checksum manifest, not a signature over an individual archive.
func resolveSignatureURL(owner, repo, version string) string {
	return resolveChecksumURL(owner, repo, version) + ".minisig"
}

// downloadToFile downloads at most maxBytes from url to outPath and returns
// the SHA256 hex digest. It rejects both oversized Content-Length declarations
// and chunked/unknown-length bodies, removing any partial output on failure.
func downloadToFile(ctx context.Context, url string, outPath string, maxBytes int64) (hexDigest string, err error) {
	if maxBytes <= 0 {
		return "", errors.New("release archive has an invalid size limit")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", fmt.Errorf("build request: %w", err)
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("download %s: %w", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("download %s: HTTP %d", url, resp.StatusCode)
	}
	if resp.ContentLength > maxBytes {
		return "", fmt.Errorf("release archive exceeds %d-byte limit", maxBytes)
	}

	if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
		return "", fmt.Errorf("create dir: %w", err)
	}
	f, err := os.Create(outPath)
	if err != nil {
		return "", fmt.Errorf("create %s: %w", outPath, err)
	}
	completed := false
	defer func() {
		if closeErr := f.Close(); err == nil && closeErr != nil {
			err = fmt.Errorf("close %s: %w", outPath, closeErr)
		}
		if !completed || err != nil {
			_ = os.Remove(outPath)
		}
	}()

	h := sha256.New()
	written, err := io.Copy(io.MultiWriter(f, h), io.LimitReader(resp.Body, maxBytes+1))
	if err != nil {
		return "", fmt.Errorf("write %s: %w", outPath, err)
	}
	if written > maxBytes {
		return "", fmt.Errorf("release archive exceeds %d-byte limit", maxBytes)
	}

	completed = true
	return hex.EncodeToString(h.Sum(nil)), nil
}

// fetchChecksums downloads checksums.txt from url and returns its content.
// Returns an error if the file cannot be fetched or the server returns non-200.
func fetchChecksums(ctx context.Context, url string) (string, error) {
	data, err := fetchBounded(ctx, url, "checksums.txt", maxChecksumManifestBytes)
	return string(data), err
}

func fetchChecksumSignature(ctx context.Context, url string) ([]byte, error) {
	return fetchBounded(ctx, url, "checksums.txt.minisig", maxChecksumSignatureBytes)
}

func fetchBounded(ctx context.Context, url, assetName string, maxBytes int64) ([]byte, error) {
	if maxBytes <= 0 {
		return nil, fmt.Errorf("%s: invalid size limit", assetName)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch %s: %w", assetName, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s: HTTP %d", assetName, resp.StatusCode)
	}
	if resp.ContentLength > maxBytes {
		return nil, fmt.Errorf("%s exceeds %d-byte limit", assetName, maxBytes)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", assetName, err)
	}
	if int64(len(data)) > maxBytes {
		return nil, fmt.Errorf("%s exceeds %d-byte limit", assetName, maxBytes)
	}
	return data, nil
}

// expectedChecksumFor parses checksums.txt content and returns the SHA256 hex
// digest for filename. Returns an error if the filename is not listed.
//
// GoReleaser produces BSD-style checksums.txt: "<digest>  <filename>" per line.
func expectedChecksumFor(content, filename string) (string, error) {
	var digest string
	for _, line := range strings.Split(content, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 || fields[1] != filename {
			continue
		}
		if len(fields) != 2 {
			return "", fmt.Errorf("%q has a malformed checksums.txt entry", filename)
		}
		if digest != "" {
			return "", fmt.Errorf("%q has duplicate checksums.txt entries", filename)
		}
		decoded, err := hex.DecodeString(fields[0])
		if err != nil || len(decoded) != sha256.Size || fields[0] != strings.ToLower(fields[0]) {
			return "", fmt.Errorf("%q has an invalid SHA256 digest", filename)
		}
		digest = fields[0]
	}
	if digest == "" {
		return "", fmt.Errorf("%q not listed in checksums.txt", filename)
	}
	return digest, nil
}

// downloadBinary fetches the asset at url, extracts the binary named binaryName
// from the .tar.gz, and writes it to outPath with executable permissions.
//
// Note: this function does not verify checksums. Use Download for a complete,
// checksum-verified upgrade flow.
func downloadBinary(ctx context.Context, url string, binaryName string, outPath string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("download %s: %w", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download %s: HTTP %d", url, resp.StatusCode)
	}

	return extractBinaryFromTarGz(resp.Body, binaryName, outPath)
}

// extractBinaryFromTarGz reads a .tar.gz stream and requires exactly one
// regular file whose base name matches binaryName. It scans through EOF before
// returning success so duplicate candidates cannot win by archive order.
func extractBinaryFromTarGz(r io.Reader, binaryName string, outPath string) error {
	gr, err := gzip.NewReader(r)
	if err != nil {
		return fmt.Errorf("open gzip: %w", err)
	}
	defer gr.Close()

	tr := tar.NewReader(gr)
	written := false
	defer func() {
		if !written {
			_ = os.Remove(outPath)
		}
	}()
	found := false

	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("read tar: %w", err)
		}

		// Match by base name (handles subdirectory layouts like tool_1.0_os_arch/tool).
		// Only accept regular files — skip symlinks, hardlinks, and special files.
		if filepath.Base(hdr.Name) == binaryName &&
			(hdr.Typeflag == tar.TypeReg || hdr.Typeflag == tar.TypeRegA) {
			if found {
				return fmt.Errorf("binary %q appears more than once in archive", binaryName)
			}
			if err := writeExecutable(tr, outPath); err != nil {
				return err
			}
			found = true
		}
	}

	if !found {
		return fmt.Errorf("binary %q not found in archive", binaryName)
	}
	written = true
	return nil
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

// atomicReplace moves src to dst atomically using os.Rename.
// This is safe on Unix (same-filesystem rename) but NOT safe on Windows
// when the binary is running. The caller must guard against Windows before calling.
func atomicReplace(src, dst string) error {
	if err := os.Rename(src, dst); err != nil {
		return fmt.Errorf("rename %s -> %s: %w", src, dst, err)
	}
	return nil
}

func validateReleaseIdentity(owner, repo, version string) error {
	if !githubSlugPattern.MatchString(owner) || !githubSlugPattern.MatchString(repo) {
		return fmt.Errorf("invalid GitHub repository identity %q", owner+"/"+repo)
	}
	if !exactReleaseVersionPattern.MatchString(version) {
		return fmt.Errorf("release version %q is not exact stable semver", version)
	}
	return nil
}

func releaseTrustedComment(owner, repo, version string) string {
	return fmt.Sprintf("repo=%s/%s;tag=v%s", owner, repo, version)
}

func configuredReleasePublicKeys() ([]minisign.PublicKey, error) {
	raw := releaseMinisignPublicKeys
	if raw == "" || raw == unsetReleaseMinisignPublicKeys || raw == legacyZeroMinisignKeyPlaceholder {
		return nil, ErrReleaseTrustUnavailable
	}
	if !canonicalPublicKeysPattern.MatchString(raw) {
		return nil, fmt.Errorf("%w: expected canonical one-key or two-key grammar", ErrReleaseTrustUnavailable)
	}
	parts := strings.Split(raw, ",")
	if len(parts) < 1 || len(parts) > 2 {
		return nil, fmt.Errorf("%w: expected one key or a two-key rotation overlap", ErrReleaseTrustUnavailable)
	}
	keys := make([]minisign.PublicKey, 0, len(parts))
	seen := make(map[string]struct{}, len(parts))
	for _, part := range parts {
		encoded := part
		if encoded == "" || encoded == unsetReleaseMinisignPublicKeys || encoded == legacyZeroMinisignKeyPlaceholder {
			return nil, ErrReleaseTrustUnavailable
		}
		if _, duplicate := seen[encoded]; duplicate {
			return nil, fmt.Errorf("%w: duplicate public key", ErrReleaseTrustUnavailable)
		}
		key, err := minisign.NewPublicKey(encoded)
		if err != nil {
			return nil, fmt.Errorf("%w: malformed public key: %v", ErrReleaseTrustUnavailable, err)
		}
		seen[encoded] = struct{}{}
		keys = append(keys, key)
	}
	return keys, nil
}

func verifyChecksumsSignature(manifest, signatureBlob []byte, owner, repo, version string) error {
	if err := validateReleaseIdentity(owner, repo, version); err != nil {
		return err
	}
	keys, err := configuredReleasePublicKeys()
	if err != nil {
		return err
	}
	signature, err := decodeCanonicalMinisignSignature(signatureBlob)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrSignatureBlobMalformed, err)
	}
	for i := range keys {
		verified, verifyErr := keys[i].Verify(manifest, signature)
		if verifyErr != nil || !verified {
			continue
		}
		expected := "trusted comment: " + releaseTrustedComment(owner, repo, version)
		if signature.TrustedComment != expected {
			return fmt.Errorf("%w: got %q, want %q", ErrSignatureIdentityMismatch, signature.TrustedComment, expected)
		}
		return nil
	}
	return ErrSignatureVerificationFailed
}

func decodeCanonicalMinisignSignature(signatureBlob []byte) (minisign.Signature, error) {
	lines := strings.SplitN(string(signatureBlob), "\n", 4)
	if len(lines) < 4 {
		return minisign.Signature{}, errors.New("incomplete signature envelope")
	}
	untrustedComment := strings.TrimSuffix(lines[0], "\r")
	trustedComment := strings.TrimSuffix(lines[2], "\r")
	if !strings.HasPrefix(untrustedComment, "untrusted comment: ") {
		return minisign.Signature{}, errors.New("unexpected untrusted comment prefix")
	}
	if !strings.HasPrefix(trustedComment, "trusted comment: ") {
		return minisign.Signature{}, errors.New("unexpected trusted comment prefix")
	}
	return minisign.DecodeSignature(string(signatureBlob))
}
