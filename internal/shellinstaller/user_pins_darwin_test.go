//go:build darwin

package shellinstaller

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// Independent literals: changing production pins cannot silently change the controls.
func TestDarwinPinLiterals(t *testing.T) {
	if privateColdURL != "https://nodejs.org/dist/v24.18.0/node-v24.18.0-darwin-arm64.tar.gz" || privateColdSize != 52087559 || privateColdSHA != "e1a97e14c99c803e96c7339403282ea05a499c32f8d83defe9ef5ec66f979ed1" {
		t.Fatal("fixed Node archive pins changed")
	}
	if privateNativeNodeSize != 120965360 || privateNativeNodeSHA != "ee6fb0e015284d83a91e8ec5213f43a157f8a392b58555301682892ba928c04a" {
		t.Fatal("fixed Node member pins changed")
	}
	if userBinarySize != 16047186 || userBinarySHA != "18a9f7fae55d85c95684b6d512a4a148d0cb24a856325f72573c34caf65159eb" || userNativeManifest != `{"version":"4.0.0","asset":"gentle-ai_4.0.0_darwin_arm64.tar.gz","assetSha256":"d2159caf6d68f367b18830ece6af71ef26963d5f5320d7df6a794773f45cc7e9","binarySha256":"18a9f7fae55d85c95684b6d512a4a148d0cb24a856325f72573c34caf65159eb"}`+"\n" {
		t.Fatal("fixed gentle-ai release pins changed")
	}
	if fmt.Sprint(userToolSources) != "[{fd sharkdp/fd v10.5.0 fd-v10.5.0-aarch64-apple-darwin b67e1836c468e42e411984b56e52fa7abec08c2bd22c867398e7cc134aac5e12 1334374} {rg BurntSushi/ripgrep 15.2.0 ripgrep-15.2.0-aarch64-apple-darwin 3750b2e93f37e0c692657da574d7019a101c0084da05a790c83fd335bad973e4 1764284}]" {
		t.Fatalf("fixed tool pins changed: %v", userToolSources)
	}
}

func darwinColdResponse(t *testing.T, data []byte) *http.Response {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	template := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "nodejs.org"},
		DNSNames:              []string{"nodejs.org"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		BasicConstraintsValid: true,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	request, err := http.NewRequest(http.MethodGet, privateColdURL, nil)
	if err != nil {
		t.Fatal(err)
	}
	return &http.Response{
		StatusCode:    http.StatusOK,
		Header:        make(http.Header),
		Body:          io.NopCloser(bytes.NewReader(data)),
		ContentLength: int64(len(data)),
		Request:       request,
		TLS:           &tls.ConnectionState{Version: tls.VersionTLS12, ServerName: "nodejs.org", PeerCertificates: []*x509.Certificate{cert}, VerifiedChains: [][]*x509.Certificate{{cert}}},
	}
}

// darwinColdVerify runs the shared receive path against the darwin archive URL.
func darwinColdVerify(t *testing.T, data []byte, size int64, digest string) error {
	t.Helper()
	file, err := os.Create(filepath.Join(t.TempDir(), "node.tgz"))
	if err != nil {
		t.Fatal(err)
	}
	return privateColdReceive(context.Background(), darwinColdResponse(t, data), file, size, digest)
}

func darwinAltered(data []byte) []byte {
	altered := append([]byte(nil), data...)
	altered[len(altered)/2] ^= 1
	return altered
}

func darwinArchive(t *testing.T, member string, data []byte) []byte {
	t.Helper()
	var buffer bytes.Buffer
	gz := gzip.NewWriter(&buffer)
	archive := tar.NewWriter(gz)
	if err := archive.WriteHeader(&tar.Header{Name: member, Mode: 0755, Size: int64(len(data)), Typeflag: tar.TypeReg}); err != nil {
		t.Fatal(err)
	}
	if _, err := archive.Write(data); err != nil {
		t.Fatal(err)
	}
	if err := archive.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}

func TestDarwinColdReceiveVerifiesPinnedBytes(t *testing.T) {
	fixture := bytes.Repeat([]byte("synthetic darwin node archive\n"), 128)
	digest := fmt.Sprintf("%x", sha256.Sum256(fixture))
	if err := darwinColdVerify(t, fixture, int64(len(fixture)), digest); err != nil {
		t.Fatalf("pinned bytes refused: %v", err)
	}
	if err := darwinColdVerify(t, darwinAltered(fixture), int64(len(fixture)), digest); err == nil {
		t.Fatal("one-byte-altered archive accepted")
	}
	if err := darwinColdVerify(t, fixture[1:], int64(len(fixture)), digest); err == nil {
		t.Fatal("short archive accepted")
	}
}

func TestDarwinToolMemberVerifiesPinnedArchive(t *testing.T) {
	member := userToolSources[0].stem + "/" + userToolSources[0].name
	tool := []byte("synthetic fd\n")
	archive := darwinArchive(t, member, tool)
	pin := fmt.Sprintf("%x", sha256.Sum256(archive))
	data, err := userToolMember(context.Background(), archive, pin, member)
	if err != nil || !bytes.Equal(data, tool) {
		t.Fatalf("pinned tool archive refused: %v", err)
	}
	if _, err := userToolMember(context.Background(), darwinAltered(archive), pin, member); err == nil {
		t.Fatal("one-byte-altered tool archive accepted")
	}
}

// Opt-in evidence against the real publisher bytes; unit tests above never need the network.
func TestDarwinPinnedArtifacts(t *testing.T) {
	dir := os.Getenv("GENTLE_SHELL_PIN_ARTIFACTS")
	if dir == "" {
		t.Skip("GENTLE_SHELL_PIN_ARTIFACTS names no downloaded publisher artifacts")
	}
	read := func(name string) []byte {
		data, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			t.Fatal(err)
		}
		return data
	}
	node := read(filepath.Base(privateColdPath))
	if err := darwinColdVerify(t, node, privateColdSize, privateColdSHA); err != nil {
		t.Fatalf("publisher Node archive refused: %v", err)
	}
	if err := darwinColdVerify(t, darwinAltered(node), privateColdSize, privateColdSHA); err == nil {
		t.Fatal("one-byte-altered Node archive accepted")
	}
	gz, err := gzip.NewReader(bytes.NewReader(node))
	if err != nil {
		t.Fatal(err)
	}
	for archive := tar.NewReader(gz); ; {
		header, err := archive.Next()
		if err != nil {
			t.Fatalf("Node member absent: %v", err)
		}
		if header.Name == "node-v24.18.0-darwin-arm64/bin/node" {
			binary, err := io.ReadAll(archive)
			if err != nil || int64(len(binary)) != privateNativeNodeSize || fmt.Sprintf("%x", sha256.Sum256(binary)) != privateNativeNodeSHA {
				t.Fatalf("Node member pin differs: %v", err)
			}
			break
		}
	}
	var manifest struct{ Asset, AssetSha256 string }
	if err := json.Unmarshal([]byte(userNativeManifest), &manifest); err != nil {
		t.Fatal(err)
	}
	release := read(manifest.Asset)
	binary, err := userToolMember(context.Background(), release, manifest.AssetSha256, "gentle-ai")
	if err != nil || int64(len(binary)) != userBinarySize || fmt.Sprintf("%x", sha256.Sum256(binary)) != userBinarySHA {
		t.Fatalf("gentle-ai release pin differs: %v", err)
	}
	for _, source := range userToolSources {
		data := read(source.stem + ".tar.gz")
		if int64(len(data)) != source.size {
			t.Fatalf("%s archive size differs", source.name)
		}
		if _, err := userToolMember(context.Background(), data, source.pin, source.stem+"/"+source.name); err != nil {
			t.Fatalf("publisher %s archive refused: %v", source.name, err)
		}
		if _, err := userToolMember(context.Background(), darwinAltered(data), source.pin, source.stem+"/"+source.name); err == nil {
			t.Fatalf("one-byte-altered %s archive accepted", source.name)
		}
	}
}
