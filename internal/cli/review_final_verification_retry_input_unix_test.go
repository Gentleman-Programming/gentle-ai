//go:build aix || darwin || dragonfly || freebsd || linux || netbsd || openbsd || solaris

package cli

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

func TestReviewRetryFinalVerificationIncidentNamedFIFOCancellation(t *testing.T) {
	fixture := failedFinalVerificationCLIFixture(t)
	fifo := filepath.Join(t.TempDir(), "incident.fifo")
	if err := unix.Mkfifo(fifo, 0o600); err != nil {
		t.Fatal(err)
	}
	anchorFD, err := unix.Open(fifo, unix.O_RDWR|unix.O_NONBLOCK, 0)
	if err != nil {
		t.Fatal(err)
	}
	anchor := os.NewFile(uintptr(anchorFD), fifo)
	if anchor == nil {
		_ = unix.Close(anchorFD)
		t.Fatal("open FIFO anchor")
	}
	t.Cleanup(func() { _ = anchor.Close() })

	args := finalVerificationRetryCLIArgs(t, fixture, "retry-cancelled-fifo-successor", fifo)
	before := cliReviewAuthoritySnapshot(t, fixture.repo)
	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan error, 1)
	go func() { result <- runReviewRetryFinalVerification(ctx, args[1:], &bytes.Buffer{}) }()
	time.Sleep(25 * time.Millisecond)
	cancel()
	select {
	case err := <-result:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("cancelled FIFO error = %v", err)
		}
	case <-time.After(500 * time.Millisecond):
		_ = anchor.Close()
		<-result
		t.Fatal("cancelled incident FIFO remained blocked")
	}
	if after := cliReviewAuthoritySnapshot(t, fixture.repo); !reflect.DeepEqual(after, before) {
		t.Fatalf("cancelled FIFO mutated authority: %#v != %#v", after, before)
	}
}
