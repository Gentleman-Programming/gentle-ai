//go:build aix || darwin || dragonfly || freebsd || linux || netbsd || openbsd || solaris

package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

func TestPR1588RevalidationNamedFIFOCancellation(t *testing.T) {
	repo, started, store, record, _ := capturedArtifacts(t, false)
	path := t.TempDir() + "/manifest.fifo"
	if err := unix.Mkfifo(path, 0o600); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan error, 1)
	go func() {
		result <- runReviewFacadeFinalize(ctx, []string{
			"--cwd", repo,
			"--lineage", started.LineageID,
			"--result-artifact-file", path,
		}, io.Discard)
	}()

	// Give the path acquisition time to enter a blocking FIFO open before
	// cancellation. A safe implementation may reject the FIFO before this point.
	time.Sleep(100 * time.Millisecond)
	cancel()

	select {
	case err := <-result:
		if !errors.Is(err, context.Canceled) && (err == nil || err.Error() != "read reviewer artifact manifest 1: artifact manifest input must be a regular file") {
			t.Fatalf("named FIFO cancellation error = %v", err)
		}
	case <-time.After(time.Second):
		// Connect a writer only to release an implementation blocked in open(2),
		// then prove the cancellation was unable to interrupt path acquisition.
		fd, err := unix.Open(path, unix.O_WRONLY|unix.O_NONBLOCK, 0)
		if err == nil {
			_ = unix.Close(fd)
		}
		select {
		case <-result:
		case <-time.After(time.Second):
			t.Fatal("named FIFO finalize did not stop after cleanup")
		}
		t.Fatal("named FIFO path acquisition stayed blocked after cancellation")
	}
	assertArtifactRevision(t, store, record.Revision)
	if fd, err := unix.Open(path, unix.O_WRONLY|unix.O_NONBLOCK, 0); err == nil {
		_ = unix.Close(fd)
		t.Fatal("named FIFO rejection left a reader descriptor open")
	}
}

func TestReviewFinalizeRejectsNonRegularArtifactFilePaths(t *testing.T) {
	tests := []struct {
		name string
		path func(*testing.T) string
	}{
		{
			name: "directory",
			path: func(t *testing.T) string { return t.TempDir() },
		},
		{
			name: "device",
			path: func(t *testing.T) string {
				if _, err := os.Stat(os.DevNull); err != nil {
					t.Skipf("null device unavailable: %v", err)
				}
				return os.DevNull
			},
		},
		{
			name: "socket",
			path: func(t *testing.T) string {
				path := t.TempDir() + "/manifest.sock"
				listener, err := net.Listen("unix", path)
				if err != nil {
					t.Skipf("Unix socket unavailable: %v", err)
				}
				t.Cleanup(func() { _ = listener.Close() })
				return path
			},
		},
		{
			name: "open pipe descriptor",
			path: func(t *testing.T) string {
				reader, writer, err := os.Pipe()
				if err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() {
					_ = writer.Close()
					_ = reader.Close()
				})
				path := fmt.Sprintf("/dev/fd/%d", reader.Fd())
				if _, err := os.Stat(path); err != nil {
					t.Skipf("open pipe descriptor path unavailable: %v", err)
				}
				return path
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo, started, store, record, _ := capturedArtifacts(t, false)
			err := RunReviewFacadeFinalize([]string{
				"--cwd", repo,
				"--lineage", started.LineageID,
				"--result-artifact-file", tt.path(t),
			}, io.Discard)
			if err == nil || err.Error() != "read reviewer artifact manifest 1: artifact manifest input must be a regular file" {
				t.Fatalf("nonregular artifact input error = %v", err)
			}
			assertArtifactRevision(t, store, record.Revision)
		})
	}
}
