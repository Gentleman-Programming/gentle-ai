package engram

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/system"
)

type durabilityTestFile struct {
	file     *os.File
	writeErr error
	syncErr  error
	closeErr error
	syncs    int
	closes   int
}

func (f *durabilityTestFile) Write(p []byte) (int, error) {
	if f.writeErr != nil {
		if len(p) > 0 {
			if _, err := f.file.Write(p[:1]); err != nil {
				return 0, err
			}
			return 1, f.writeErr
		}
		return 0, f.writeErr
	}
	return f.file.Write(p)
}

func (f *durabilityTestFile) Sync() error {
	f.syncs++
	if f.syncErr != nil {
		return f.syncErr
	}
	return f.file.Sync()
}

func (f *durabilityTestFile) Close() error {
	f.closes++
	if err := f.file.Close(); err != nil {
		return err
	}
	return f.closeErr
}

// TestEngramDownloadToFileDurabilityJourney exercises the release response to
// archive-file boundary where a successful digest becomes usable download data.
func TestEngramDownloadToFileDurabilityJourney(t *testing.T) {
	archive := []byte("engram release archive")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(archive)
	}))
	t.Cleanup(server.Close)

	tests := []struct {
		name       string
		writeErr   error
		syncErr    error
		closeErr   error
		wantError  string
		wantDigest bool
		wantSyncs  int
		wantCloses int
	}{
		{
			name:       "write failure removes incomplete archive",
			writeErr:   errors.New("injected write failure"),
			wantError:  "write",
			wantSyncs:  0,
			wantCloses: 1,
		},
		{
			name:       "write and close failures both propagate",
			writeErr:   errors.New("injected write failure"),
			closeErr:   errors.New("injected close failure"),
			wantError:  "write",
			wantSyncs:  0,
			wantCloses: 1,
		},
		{
			name:       "sync failure removes incomplete archive",
			syncErr:    errors.New("injected sync failure"),
			wantError:  "sync download file",
			wantSyncs:  1,
			wantCloses: 1,
		},
		{
			name:       "close failure removes incomplete archive",
			closeErr:   errors.New("injected close failure"),
			wantError:  "close download file",
			wantSyncs:  1,
			wantCloses: 1,
		},
		{
			name:       "sync and close failures both propagate",
			syncErr:    errors.New("injected sync failure"),
			closeErr:   errors.New("injected close failure"),
			wantError:  "sync download file",
			wantSyncs:  1,
			wantCloses: 1,
		},
		{
			name:       "synced and closed archive preserves bytes and digest",
			wantDigest: true,
			wantSyncs:  1,
			wantCloses: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			outPath := filepath.Join(t.TempDir(), "archive.tar.gz")
			var downloaded *durabilityTestFile
			originalCreate := engramCreateDownloadFileFn
			engramCreateDownloadFileFn = func(path string) (engramDownloadFile, error) {
				file, err := os.Create(path)
				if err != nil {
					return nil, err
				}
				downloaded = &durabilityTestFile{file: file, writeErr: tt.writeErr, syncErr: tt.syncErr, closeErr: tt.closeErr}
				return downloaded, nil
			}
			t.Cleanup(func() { engramCreateDownloadFileFn = originalCreate })

			digest, err := engramDownloadToFile(context.Background(), server.URL, outPath)
			if downloaded == nil {
				t.Fatal("download file was not created")
			}
			if downloaded.syncs != tt.wantSyncs || downloaded.closes != tt.wantCloses {
				t.Fatalf("Sync calls = %d, Close calls = %d, want %d and %d", downloaded.syncs, downloaded.closes, tt.wantSyncs, tt.wantCloses)
			}

			if !tt.wantDigest {
				if err == nil {
					t.Fatal("download succeeded, want error")
				}
				if !strings.Contains(err.Error(), tt.wantError) {
					t.Fatalf("error = %q, want it to identify %q", err, tt.wantError)
				}
				if tt.writeErr != nil && !errors.Is(err, tt.writeErr) {
					t.Fatalf("error = %v, want write error %v", err, tt.writeErr)
				}
				if tt.syncErr != nil && !errors.Is(err, tt.syncErr) {
					t.Fatalf("error = %v, want sync error %v", err, tt.syncErr)
				}
				if tt.closeErr != nil && !errors.Is(err, tt.closeErr) {
					t.Fatalf("error = %v, want close error %v", err, tt.closeErr)
				}
				if _, statErr := os.Stat(outPath); !errors.Is(statErr, os.ErrNotExist) {
					t.Fatalf("incomplete archive stat error = %v, want not exist", statErr)
				}
				return
			}

			if err != nil {
				t.Fatalf("download error = %v", err)
			}
			if digest != sha256Hex(archive) {
				t.Fatalf("digest = %q, want %q", digest, sha256Hex(archive))
			}
			persisted, err := os.ReadFile(outPath)
			if err != nil {
				t.Fatalf("read persisted archive: %v", err)
			}
			if !bytes.Equal(persisted, archive) {
				t.Fatalf("persisted archive = %q, want %q", persisted, archive)
			}
		})
	}
}

func TestDownloadLatestBinaryPropagatesArchiveDurabilityFailures(t *testing.T) {
	server := makeServerWithFakeTarGz(t, "1.3.0")
	t.Cleanup(server.Close)

	originalClient := engramHTTPClient
	originalBaseURL := engramGitHubBaseURL
	originalInstallDir := engramInstallDirFn
	originalCreate := engramCreateDownloadFileFn
	engramHTTPClient = server.Client()
	engramGitHubBaseURL = server.URL
	engramInstallDirFn = func(string) string { return t.TempDir() }
	t.Cleanup(func() {
		engramHTTPClient = originalClient
		engramGitHubBaseURL = originalBaseURL
		engramInstallDirFn = originalInstallDir
		engramCreateDownloadFileFn = originalCreate
	})

	tests := []struct {
		name      string
		syncErr   error
		closeErr  error
		wantError string
	}{
		{
			name:      "sync failure",
			syncErr:   errors.New("injected sync failure"),
			wantError: "sync download file",
		},
		{
			name:      "close failure",
			closeErr:  errors.New("injected close failure"),
			wantError: "close download file",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			engramCreateDownloadFileFn = func(path string) (engramDownloadFile, error) {
				file, err := os.Create(path)
				if err != nil {
					return nil, err
				}
				return &durabilityTestFile{file: file, syncErr: tt.syncErr, closeErr: tt.closeErr}, nil
			}

			_, err := DownloadLatestBinary(system.PlatformProfile{OS: "linux", PackageManager: "apt"}, false)
			if err == nil {
				t.Fatal("DownloadLatestBinary succeeded, want error")
			}
			if !strings.Contains(err.Error(), tt.wantError) {
				t.Fatalf("error = %q, want it to identify %q", err, tt.wantError)
			}
			if tt.syncErr != nil && !errors.Is(err, tt.syncErr) {
				t.Fatalf("error = %v, want sync error %v", err, tt.syncErr)
			}
			if tt.closeErr != nil && !errors.Is(err, tt.closeErr) {
				t.Fatalf("error = %v, want close error %v", err, tt.closeErr)
			}
		})
	}
}
