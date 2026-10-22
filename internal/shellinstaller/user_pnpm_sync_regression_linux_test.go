//go:build linux && amd64

package shellinstaller

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

func TestIndependentPNPMSyncQueueAndFinish(t *testing.T) {
	batch := newUserPNPMFileSyncBatch()
	entered := make(chan struct{}, 8)
	release := make(chan struct{})
	var closed atomic.Int32
	probe := pnpmSyncProbe{
		syncFile: func() error {
			entered <- struct{}{}
			<-release
			return nil
		},
		closeFile: func() error {
			closed.Add(1)
			return nil
		},
	}
	for range 8 {
		if err := batch.submit(probe); err != nil {
			t.Fatal(err)
		}
	}
	for range 8 {
		<-entered
	}
	// Remaining probes do not signal entry: precisely eight active plus eight queued.
	queued := pnpmSyncProbe{syncFile: func() error { <-release; return nil }, closeFile: probe.closeFile}
	for range 8 {
		if err := batch.submit(queued); err != nil {
			t.Fatal(err)
		}
	}
	submitted := make(chan error, 1)
	go func() { submitted <- batch.submit(queued) }()
	select {
	case err := <-submitted:
		t.Fatal("queue exceeded eight pending files", err)
	case <-time.After(30 * time.Millisecond):
	}
	close(release)
	if err := <-submitted; err != nil {
		t.Fatal(err)
	}
	if err := batch.finish(); err != nil {
		t.Fatal(err)
	}
	if closed.Load() != 17 {
		t.Fatal("accepted files not closed", closed.Load())
	}
	if err := batch.finish(); err != nil || closed.Load() != 17 {
		t.Fatal("non-idempotent finish", err)
	}
}
func TestIndependentPNPMSyncCancelDrain(t *testing.T) {
	parent := t.TempDir()
	source := filepath.Join(parent, "source")
	target := filepath.Join(parent, "target")
	if err := os.WriteFile(source, []byte("safe supplier"), 0600); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(source)
	if err != nil {
		t.Fatal(err)
	}
	batch := newUserPNPMFileSyncBatch()
	entered, release := make(chan struct{}), make(chan struct{})
	closed := make(chan struct{})
	if err := batch.submit(pnpmSyncProbe{syncFile: func() error { close(entered); <-release; return nil }, closeFile: func() error { close(closed); return nil }}); err != nil {
		t.Fatal(err)
	}
	<-entered
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err = userPNPMCopyFileWithSync(ctx, source, target, info, true, "/original", batch.submit)
	if !errors.Is(err, context.Canceled) {
		t.Fatal("copy cancellation lost", err)
	}
	finished := make(chan error, 1)
	go func() { finished <- batch.finish() }()
	select {
	case <-finished:
		t.Fatal("finish returned while file pending")
	case <-time.After(30 * time.Millisecond):
	}
	close(release)
	if err := <-finished; err != nil {
		t.Fatal(err)
	}
	<-closed
	f, err := os.OpenFile(target, os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
}
