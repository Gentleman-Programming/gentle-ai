package communitytool

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRTKAcquireDownloadsVerifiesAndExtracts(t *testing.T) {
	archive := rtkZIP(t, []rtkZIPEntry{{name: "rtk.exe", data: "binary"}})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(archive)
	}))
	t.Cleanup(server.Close)

	stage := t.TempDir()
	got, err := acquireRTKCandidate(server.Client(), stage, rtkTestAsset(server.URL, archive))
	if err != nil {
		t.Fatal(err)
	}
	if got != filepath.Join(stage, "rtk.exe") {
		t.Fatalf("path = %q", got)
	}
	data, err := os.ReadFile(got)
	if err != nil || string(data) != "binary" {
		t.Fatalf("extracted data = %q, error = %v", data, err)
	}
}

type rtkZIPEntry struct {
	name, data string
	store      bool
	mode       os.FileMode
}

func rtkZIP(t *testing.T, entries []rtkZIPEntry) []byte {
	t.Helper()
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	for _, entry := range entries {
		header := &zip.FileHeader{Name: entry.name, Method: zip.Deflate}
		if entry.store {
			header.Method = zip.Store
		}
		if entry.mode != 0 {
			header.SetMode(entry.mode)
		}
		file, err := writer.CreateHeader(header)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := io.WriteString(file, entry.data); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}

func rtkTestAsset(url string, archive []byte) rtkCandidateAsset {
	sum := sha256.Sum256(archive)
	executableSum := sha256.Sum256([]byte("binary"))
	return rtkCandidateAsset{URL: url, ExecutableMember: "rtk.exe", SizeBytes: int64(len(archive)), SHA256: hex.EncodeToString(sum[:]), ExecutableSizeBytes: int64(len("binary")), ExecutableSHA256: hex.EncodeToString(executableSum[:])}
}

func TestRTKAcquireRejectsExtractedExecutableIdentityMismatch(t *testing.T) {
	archive := rtkZIP(t, []rtkZIPEntry{{name: "rtk.exe", data: "binary"}})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write(archive) }))
	defer server.Close()
	asset := rtkTestAsset(server.URL, archive)
	asset.ExecutableSHA256 = strings.Repeat("0", 64)
	stage := t.TempDir()
	if _, err := acquireRTKCandidate(server.Client(), stage, asset); err == nil {
		t.Fatal("error = nil, want extracted executable identity rejection")
	}
	if entries, _ := os.ReadDir(stage); len(entries) != 0 {
		t.Fatalf("staging residue = %v", entries)
	}
}

func TestRTKAcquireRejectsUnsafeArchiveMetadata(t *testing.T) {
	archive := rtkZIP(t, []rtkZIPEntry{{name: "rtk.exe", data: "binary"}})
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { requests++; _, _ = w.Write(archive) }))
	defer server.Close()
	for _, tt := range []struct {
		name string
		size int64
	}{
		{"zero", 0}, {"negative", -1}, {"above cap", rtkMaxArchiveSize + 1}, {"maximum integer", int64(^uint64(0) >> 1)},
	} {
		t.Run(tt.name, func(t *testing.T) {
			stage := t.TempDir()
			asset := rtkTestAsset(server.URL, archive)
			asset.SizeBytes = tt.size
			if _, err := acquireRTKCandidate(server.Client(), stage, asset); err == nil {
				t.Fatal("error = nil")
			}
			if entries, _ := os.ReadDir(stage); requests != 0 || len(entries) != 0 {
				t.Fatalf("requests = %d, staging = %v", requests, entries)
			}
		})
	}
}

func TestRTKAcquireRejectsTarGzOverTotalDecompressedLimit(t *testing.T) {
	archive := rtkTarGz(t, "rtk", strings.Repeat("0", int(rtkMaxArchiveSize)+1))
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write(archive) }))
	defer server.Close()
	asset := rtkTestAsset(server.URL, archive)
	asset.Name, asset.ExecutableMember = "rtk-linux.tar.gz", "rtk"
	stage := t.TempDir()
	if _, err := acquireRTKCandidate(server.Client(), stage, asset); err == nil {
		t.Fatal("error = nil, want total decompressed tar.gz limit rejection")
	}
	if entries, _ := os.ReadDir(stage); len(entries) != 0 {
		t.Fatalf("staging residue = %v", entries)
	}
}

func TestRTKAcquireRejectsUnsafeCandidates(t *testing.T) {
	valid := rtkZIP(t, []rtkZIPEntry{{name: "rtk.exe", data: "binary"}})
	corrupt := rtkZIP(t, []rtkZIPEntry{{name: "rtk.exe", data: "binary", store: true}})
	corrupt = bytes.Replace(corrupt, []byte("binary"), []byte("badary"), 1)
	tests := []struct {
		name    string
		status  int
		archive []byte
		change  func(*rtkCandidateAsset)
	}{
		{"HTTP status", http.StatusBadGateway, valid, nil},
		{"wrong size", http.StatusOK, valid, func(a *rtkCandidateAsset) { a.SizeBytes++ }},
		{"wrong checksum", http.StatusOK, valid, func(a *rtkCandidateAsset) { a.SHA256 = "bad" }},
		{"invalid ZIP", http.StatusOK, []byte("not a ZIP"), nil},
		{"absolute member", http.StatusOK, rtkZIP(t, []rtkZIPEntry{{name: "/rtk.exe"}}), nil},
		{"traversal member", http.StatusOK, rtkZIP(t, []rtkZIPEntry{{name: "../rtk.exe"}}), nil},
		{"directory member", http.StatusOK, rtkZIP(t, []rtkZIPEntry{{name: "rtk.exe/"}}), nil},
		{"symlink member", http.StatusOK, rtkZIP(t, []rtkZIPEntry{{name: "rtk.exe", mode: os.ModeSymlink | 0o777}}), nil},
		{"duplicate member", http.StatusOK, rtkZIP(t, []rtkZIPEntry{{name: "rtk.exe"}, {name: "rtk.exe"}}), nil},
		{"unexpected member", http.StatusOK, rtkZIP(t, []rtkZIPEntry{{name: "other.exe"}}), nil},
		{"oversized member", http.StatusOK, rtkZIP(t, []rtkZIPEntry{{name: "rtk.exe", data: strings.Repeat("x", rtkMaxExecutableSize+1)}}), nil},
		{"corrupt extraction", http.StatusOK, corrupt, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(tt.status); _, _ = w.Write(tt.archive) }))
			defer server.Close()
			stage := t.TempDir()
			asset := rtkTestAsset(server.URL, tt.archive)
			if tt.change != nil {
				tt.change(&asset)
			}
			if _, err := acquireRTKCandidate(server.Client(), stage, asset); err == nil {
				t.Fatal("error = nil")
			}
			if entries, _ := os.ReadDir(stage); len(entries) != 0 {
				t.Fatalf("staging residue = %v", entries)
			}
		})
	}
}
