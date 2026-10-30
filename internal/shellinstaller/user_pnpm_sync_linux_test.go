//go:build linux && amd64

package shellinstaller

import (
	"errors"
	"sync/atomic"
	"testing"
)

type pnpmSyncProbe struct {
	syncFile  func() error
	closeFile func() error
}

func (p pnpmSyncProbe) Sync() error  { return p.syncFile() }
func (p pnpmSyncProbe) Close() error { return p.closeFile() }

func TestUserPNPMFileSyncBatchBoundedDrain(t *testing.T) {
	batch := newUserPNPMFileSyncBatch()
	entered := make(chan struct{}, 16)
	release := make(chan struct{})
	var active, peak, synced, closed atomic.Int32
	probe := pnpmSyncProbe{
		syncFile: func() error {
			n := active.Add(1)
			for old := peak.Load(); n > old && !peak.CompareAndSwap(old, n); old = peak.Load() {
			}
			entered <- struct{}{}
			<-release
			active.Add(-1)
			synced.Add(1)
			return nil
		},
		closeFile: func() error { closed.Add(1); return nil },
	}
	// All eight workers must overlap; a serial implementation cannot proceed.
	for range 8 {
		if err := batch.submit(probe); err != nil {
			t.Fatal(err)
		}
	}
	for range 8 {
		<-entered
	}
	// Queued work must also be flushed and closed before finish returns.
	if err := batch.submit(probe); err != nil {
		t.Fatal(err)
	}
	close(release)
	if err := batch.finish(); err != nil {
		t.Fatal(err)
	}
	if peak.Load() != 8 || synced.Load() != 9 || closed.Load() != 9 || active.Load() != 0 {
		t.Fatalf("peak=%d synced=%d closed=%d active=%d", peak.Load(), synced.Load(), closed.Load(), active.Load())
	}
	if err := batch.finish(); err != nil {
		t.Fatal(err)
	}
	if closed.Load() != 9 {
		t.Fatal("finish closed files twice")
	}
}

func TestUserPNPMFileSyncBatchRetainsFailures(t *testing.T) {
	syncErr, closeErr := errors.New("sync failed"), errors.New("close failed")
	batch := newUserPNPMFileSyncBatch()
	var closed atomic.Int32
	_ = batch.submit(pnpmSyncProbe{
		syncFile:  func() error { return syncErr },
		closeFile: func() error { closed.Add(1); return closeErr },
	})
	err := batch.finish()
	if !errors.Is(err, syncErr) || !errors.Is(err, closeErr) || closed.Load() != 1 {
		t.Fatalf("failure or file close lost: %v, closed=%d", err, closed.Load())
	}
}
