package upgrade

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	minisign "github.com/jedisct1/go-minisign"

	"github.com/gentleman-programming/gentle-ai/v2/internal/system"
	"github.com/gentleman-programming/gentle-ai/v2/internal/update"
)

const testMinisignKeyDomain = "gentle-ai issue 359 minisign TEST KEY; never use for releases"

func testMinisignKey(t *testing.T) (string, minisign.PrivateKey) {
	t.Helper()
	return minisignKeyFromDomain(t, testMinisignKeyDomain)
}

func minisignKeyFromDomain(t *testing.T, domain string) (string, minisign.PrivateKey) {
	t.Helper()
	seed := sha256.Sum256([]byte(domain))
	private := ed25519.NewKeyFromSeed(seed[:])
	public := private.Public().(ed25519.PublicKey)
	keyIDHash := sha256.Sum256(append([]byte("test-key-id:"), public...))

	var secret minisign.PrivateKey
	secret.SignatureAlgorithm = [2]byte{'E', 'd'}
	secret.ChecksumAlgorithm = [2]byte{'B', '2'}
	copy(secret.KeyId[:], keyIDHash[:8])
	copy(secret.SecretKey[:], private)

	keyBytes := make([]byte, 0, 2+8+ed25519.PublicKeySize)
	keyBytes = append(keyBytes, 'E', 'd')
	keyBytes = append(keyBytes, secret.KeyId[:]...)
	keyBytes = append(keyBytes, public...)
	return base64.StdEncoding.EncodeToString(keyBytes), secret
}

func signTestManifest(t *testing.T, secret minisign.PrivateKey, manifest []byte, owner, repo, version string) []byte {
	t.Helper()
	signature, err := secret.Sign(manifest, minisign.SignOptions{
		Hashed:           true,
		UntrustedComment: "gentle-ai test signature",
		TrustedComment:   releaseTrustedComment(owner, repo, version),
	})
	if err != nil {
		t.Fatalf("sign test manifest: %v", err)
	}
	return signature.Encode()
}

func useTestReleaseKey(t *testing.T) minisign.PrivateKey {
	t.Helper()
	publicKey, privateKey := testMinisignKey(t)
	original := releaseMinisignPublicKeys
	releaseMinisignPublicKeys = publicKey
	t.Cleanup(func() { releaseMinisignPublicKeys = original })
	return privateKey
}

func TestVerifyChecksumsSignatureFailsClosed(t *testing.T) {
	const (
		owner   = "Gentleman-Programming"
		repo    = "gentle-ai"
		version = "2.2.0"
	)
	manifest := []byte(strings.Repeat("a", sha256.Size*2) + "  gentle-ai_2.2.0_linux_amd64.tar.gz\n")
	publicKey, privateKey := testMinisignKey(t)
	rotationKey, _ := minisignKeyFromDomain(t, testMinisignKeyDomain+" rotation fixture")
	validSignature := signTestManifest(t, privateKey, manifest, owner, repo, version)
	noncanonicalEnvelope := bytes.Replace(
		bytes.Clone(validSignature),
		[]byte("untrusted comment: "),
		[]byte("comment: "),
		1,
	)

	tests := []struct {
		name      string
		keys      string
		manifest  []byte
		signature []byte
		owner     string
		repo      string
		version   string
		wantErr   bool
	}{
		{name: "valid", keys: publicKey, manifest: manifest, signature: validSignature, owner: owner, repo: repo, version: version},
		{name: "valid with signing key second in rotation overlap", keys: rotationKey + "," + publicKey, manifest: manifest, signature: validSignature, owner: owner, repo: repo, version: version},
		{name: "unset trust anchor", keys: unsetReleaseMinisignPublicKeys, manifest: manifest, signature: validSignature, owner: owner, repo: repo, version: version, wantErr: true},
		{name: "more than two trust anchors", keys: rotationKey + "," + publicKey + "," + rotationKey, manifest: manifest, signature: validSignature, owner: owner, repo: repo, version: version, wantErr: true},
		{name: "duplicate trust anchors", keys: publicKey + "," + publicKey, manifest: manifest, signature: validSignature, owner: owner, repo: repo, version: version, wantErr: true},
		{name: "leading whitespace in trust anchor", keys: " " + publicKey, manifest: manifest, signature: validSignature, owner: owner, repo: repo, version: version, wantErr: true},
		{name: "trailing separator in trust anchors", keys: publicKey + ",", manifest: manifest, signature: validSignature, owner: owner, repo: repo, version: version, wantErr: true},
		{name: "newline linker injection in trust anchors", keys: publicKey + "\n-X override=value", manifest: manifest, signature: validSignature, owner: owner, repo: repo, version: version, wantErr: true},
		{name: "tampered manifest", keys: publicKey, manifest: append(bytes.Clone(manifest), 'x'), signature: validSignature, owner: owner, repo: repo, version: version, wantErr: true},
		{name: "wrong repository binding", keys: publicKey, manifest: manifest, signature: validSignature, owner: owner, repo: "lookalike", version: version, wantErr: true},
		{name: "wrong tag binding", keys: publicKey, manifest: manifest, signature: validSignature, owner: owner, repo: repo, version: "2.2.1", wantErr: true},
		{name: "version prefix rejected", keys: publicKey, manifest: manifest, signature: validSignature, owner: owner, repo: repo, version: "v2.2.0", wantErr: true},
		{name: "prerelease rejected", keys: publicKey, manifest: manifest, signature: validSignature, owner: owner, repo: repo, version: "2.2.0-rc.1", wantErr: true},
		{name: "comment injection rejected", keys: publicKey, manifest: manifest, signature: validSignature, owner: owner, repo: "gentle-ai\ntag=v2.2.0", version: version, wantErr: true},
		{name: "noncanonical untrusted comment prefix rejected", keys: publicKey, manifest: manifest, signature: noncanonicalEnvelope, owner: owner, repo: repo, version: version, wantErr: true},
		{name: "malformed signature", keys: publicKey, manifest: manifest, signature: []byte("not minisign"), owner: owner, repo: repo, version: version, wantErr: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			original := releaseMinisignPublicKeys
			releaseMinisignPublicKeys = tc.keys
			t.Cleanup(func() { releaseMinisignPublicKeys = original })
			err := verifyChecksumsSignature(tc.manifest, tc.signature, tc.owner, tc.repo, tc.version)
			if (err != nil) != tc.wantErr {
				t.Fatalf("verifyChecksumsSignature() error = %v, wantErr %v", err, tc.wantErr)
			}
		})
	}
}

func TestProductionReleaseKeyNeverEqualsTestKey(t *testing.T) {
	testPublicKey, _ := testMinisignKey(t)
	for _, configured := range strings.Split(releaseMinisignPublicKeys, ",") {
		if strings.TrimSpace(configured) == testPublicKey {
			t.Fatal("production release key injection must never use the isolated test key")
		}
	}
}

func TestIsolatedMinisignTestKeyMatchesFixture(t *testing.T) {
	want, _ := testMinisignKey(t)
	fixture, err := os.ReadFile(filepath.Join("testdata", "minisign-test.pub"))
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(string(fixture)); got != want {
		t.Fatalf("isolated test public-key fixture = %q, want derived test key %q", got, want)
	}
}

func TestFetchBounded(t *testing.T) {
	const limit = int64(32)
	tests := []struct {
		name    string
		status  int
		body    string
		chunked bool
		wantErr bool
	}{
		{name: "at limit", status: http.StatusOK, body: strings.Repeat("x", int(limit))},
		{name: "over limit", status: http.StatusOK, body: strings.Repeat("x", int(limit+1)), wantErr: true},
		{name: "chunked over limit", status: http.StatusOK, body: strings.Repeat("x", int(limit+1)), chunked: true, wantErr: true},
		{name: "non 200", status: http.StatusNotFound, wantErr: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tc.status)
				if tc.chunked {
					w.(http.Flusher).Flush()
				}
				_, _ = fmt.Fprint(w, tc.body)
			}))
			defer server.Close()

			original := httpClient
			httpClient = server.Client()
			t.Cleanup(func() { httpClient = original })
			got, err := fetchBounded(context.Background(), server.URL, "test metadata", limit)
			if (err != nil) != tc.wantErr {
				t.Fatalf("fetchBounded() error = %v, wantErr %v", err, tc.wantErr)
			}
			if !tc.wantErr && len(got) != len(tc.body) {
				t.Fatalf("fetchBounded() length = %d, want %d", len(got), len(tc.body))
			}
		})
	}
}

func TestDownloadToFileEnforcesLimitAndCleansPartialOutput(t *testing.T) {
	const limit = int64(32)
	tests := []struct {
		name    string
		chunked bool
	}{
		{name: "declared content length"},
		{name: "chunked unknown length", chunked: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				if !tc.chunked {
					w.Header().Set("Content-Length", fmt.Sprint(limit+1))
					w.WriteHeader(http.StatusOK)
					return
				}
				w.WriteHeader(http.StatusOK)
				w.(http.Flusher).Flush()
				_, _ = w.Write([]byte(strings.Repeat("x", int(limit+1))))
			}))
			defer server.Close()

			original := httpClient
			httpClient = server.Client()
			t.Cleanup(func() { httpClient = original })

			outPath := filepath.Join(t.TempDir(), "oversized.tar.gz")
			_, err := downloadToFile(context.Background(), server.URL, outPath, limit)
			if err == nil || !strings.Contains(err.Error(), "exceeds 32-byte limit") {
				t.Fatalf("downloadToFile() error = %v, want archive size-limit error", err)
			}
			if _, statErr := os.Stat(outPath); !os.IsNotExist(statErr) {
				t.Fatalf("oversized download left partial output behind: %v", statErr)
			}
		})
	}
}

func TestExpectedChecksumForRequiresUniqueSHA256Entry(t *testing.T) {
	const filename = "gentle-ai_2.2.0_linux_amd64.tar.gz"
	digest := strings.Repeat("a", sha256.Size*2)
	tests := []struct {
		name    string
		body    string
		wantErr bool
	}{
		{name: "one canonical entry", body: digest + "  " + filename + "\n"},
		{name: "duplicate entry", body: digest + "  " + filename + "\n" + digest + "  " + filename + "\n", wantErr: true},
		{name: "short digest", body: "deadbeef  " + filename + "\n", wantErr: true},
		{name: "non hex digest", body: strings.Repeat("z", sha256.Size*2) + "  " + filename + "\n", wantErr: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := expectedChecksumFor(tc.body, filename)
			if (err != nil) != tc.wantErr {
				t.Fatalf("expectedChecksumFor() error = %v, wantErr %v", err, tc.wantErr)
			}
		})
	}
}

func TestExtractBinaryRejectsDuplicateEntriesAndCleansOutput(t *testing.T) {
	var archive bytes.Buffer
	gzipWriter := gzip.NewWriter(&archive)
	tarWriter := tar.NewWriter(gzipWriter)
	for _, name := range []string{"gentle-ai", "nested/gentle-ai"} {
		content := []byte(name)
		if err := tarWriter.WriteHeader(&tar.Header{Name: name, Mode: 0o755, Size: int64(len(content)), Typeflag: tar.TypeReg}); err != nil {
			t.Fatal(err)
		}
		if _, err := tarWriter.Write(content); err != nil {
			t.Fatal(err)
		}
	}
	if err := tarWriter.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gzipWriter.Close(); err != nil {
		t.Fatal(err)
	}

	outPath := filepath.Join(t.TempDir(), "gentle-ai.new")
	if err := extractBinaryFromTarGz(bytes.NewReader(archive.Bytes()), "gentle-ai", outPath); err == nil {
		t.Fatal("extractBinaryFromTarGz() accepted duplicate binary entries")
	}
	if _, err := os.Stat(outPath); !os.IsNotExist(err) {
		t.Fatalf("failed extraction left output behind: %v", err)
	}
}

func TestDownloadVerifiesSignatureBeforeArchiveAndPreservesInstalledBinary(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("binary replacement is Unix-only")
	}
	const (
		owner      = "Gentleman-Programming"
		repo       = "gentle-ai"
		version    = "2.2.0"
		binaryName = "gentle-ai"
	)
	privateKey := useTestReleaseKey(t)
	tarPath := makeFakeTarGz(t, binaryName)
	archive, err := os.ReadFile(tarPath)
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(archive)
	archiveName := resolveArchiveName(repo, version, runtime.GOOS, runtime.GOARCH)
	manifest := []byte(hex.EncodeToString(digest[:]) + "  " + archiveName + "\n")
	validSignature := signTestManifest(t, privateKey, manifest, owner, repo, version)
	wrongManifest := []byte(strings.Repeat("b", sha256.Size*2) + "  " + archiveName + "\n")
	wrongManifestSignature := signTestManifest(t, privateKey, wrongManifest, owner, repo, version)
	duplicateManifest := append(bytes.Clone(manifest), manifest...)
	duplicateManifestSignature := signTestManifest(t, privateKey, duplicateManifest, owner, repo, version)

	tests := []struct {
		name                 string
		manifest             []byte
		signature            []byte
		archiveContentLength int64
		wantErr              bool
		wantArchiveGET       bool
		errContains          string
	}{
		{name: "valid metadata replaces binary", manifest: manifest, signature: validSignature, wantArchiveGET: true},
		{name: "invalid signature never fetches archive", manifest: manifest, signature: append(bytes.Clone(validSignature), 'x'), wantErr: true},
		{name: "duplicate signed checksum entry never fetches archive", manifest: duplicateManifest, signature: duplicateManifestSignature, wantErr: true},
		{name: "checksum mismatch preserves binary and cleans download", manifest: wrongManifest, signature: wrongManifestSignature, wantErr: true, wantArchiveGET: true},
		{
			name:                 "oversized declared archive preserves binary and cleans download",
			manifest:             manifest,
			signature:            validSignature,
			archiveContentLength: maxReleaseArchiveBytes + 1,
			wantErr:              true,
			wantArchiveGET:       true,
			errContains:          "exceeds 134217728-byte limit",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tmpRoot := t.TempDir()
			t.Setenv("TMPDIR", tmpRoot)
			var requests []string
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests = append(requests, r.URL.Path)
				switch r.URL.Path {
				case "/checksums.txt":
					_, _ = w.Write(tc.manifest)
				case "/checksums.txt.minisig":
					_, _ = w.Write(tc.signature)
				case "/" + archiveName:
					if tc.archiveContentLength > 0 {
						w.Header().Set("Content-Length", fmt.Sprint(tc.archiveContentLength))
						w.WriteHeader(http.StatusOK)
						return
					}
					_, _ = w.Write(archive)
				default:
					http.NotFound(w, r)
				}
			}))
			defer server.Close()

			originalClient := httpClient
			httpClient = server.Client()
			t.Cleanup(func() { httpClient = originalClient })
			originalAsset, originalChecksum, originalSignature := resolveAssetURLFn, resolveChecksumURLFn, resolveSignatureURLFn
			resolveAssetURLFn = func(_, _, _, _, _ string) string { return server.URL + "/" + archiveName }
			resolveChecksumURLFn = func(_, _, _ string) string { return server.URL + "/checksums.txt" }
			resolveSignatureURLFn = func(_, _, _ string) string { return server.URL + "/checksums.txt.minisig" }
			t.Cleanup(func() {
				resolveAssetURLFn, resolveChecksumURLFn, resolveSignatureURLFn = originalAsset, originalChecksum, originalSignature
			})

			binaryPath := filepath.Join(t.TempDir(), binaryName)
			if err := os.WriteFile(binaryPath, []byte("old binary"), 0o755); err != nil {
				t.Fatal(err)
			}
			originalLookPath := lookPathFn
			lookPathFn = func(string) (string, error) { return binaryPath, nil }
			t.Cleanup(func() { lookPathFn = originalLookPath })

			err := Download(context.Background(), update.UpdateResult{
				Tool:          update.ToolInfo{Name: binaryName, Owner: owner, Repo: repo},
				LatestVersion: version,
			}, system.PlatformProfile{OS: runtime.GOOS})
			if (err != nil) != tc.wantErr {
				t.Fatalf("Download() error = %v, wantErr %v", err, tc.wantErr)
			}
			if tc.errContains != "" && (err == nil || !strings.Contains(err.Error(), tc.errContains)) {
				t.Fatalf("Download() error = %v, want substring %q", err, tc.errContains)
			}

			archiveFetched := false
			for _, path := range requests {
				if path == "/"+archiveName {
					archiveFetched = true
				}
			}
			if archiveFetched != tc.wantArchiveGET {
				t.Fatalf("archive fetched = %v, want %v; requests = %v", archiveFetched, tc.wantArchiveGET, requests)
			}
			got, readErr := os.ReadFile(binaryPath)
			if readErr != nil {
				t.Fatal(readErr)
			}
			if tc.wantErr && string(got) != "old binary" {
				t.Fatalf("failed upgrade changed installed binary to %q", got)
			}
			if !tc.wantErr && string(got) == "old binary" {
				t.Fatal("successful authenticated upgrade did not replace the installed binary")
			}
			if _, statErr := os.Stat(binaryPath + ".new"); !os.IsNotExist(statErr) {
				t.Fatalf("upgrade left temporary binary behind: %v", statErr)
			}
			entries, readDirErr := os.ReadDir(tmpRoot)
			if readDirErr != nil {
				t.Fatal(readDirErr)
			}
			if len(entries) != 0 {
				t.Fatalf("upgrade left temporary download state behind: %v", entries)
			}
		})
	}
}
