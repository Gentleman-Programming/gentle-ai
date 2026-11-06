//go:build linux && amd64

package shellinstaller

import (
	"errors"
	"sync"
)

// Each accepted file is synchronized and closed before finish returns. The
// bounded queue overlaps storage latency without weakening publication durability.
type userPNPMSyncFile interface {
	Sync() error
	Close() error
}

type userPNPMFileSyncBatch struct {
	jobs       chan userPNPMSyncFile
	workers    sync.WaitGroup
	finishOnce sync.Once
	mu         sync.Mutex
	err        error
}

func newUserPNPMFileSyncBatch() *userPNPMFileSyncBatch {
	const concurrency = 8
	batch := &userPNPMFileSyncBatch{jobs: make(chan userPNPMSyncFile, concurrency)}
	batch.workers.Add(concurrency)
	for range concurrency {
		go func() {
			defer batch.workers.Done()
			for file := range batch.jobs {
				err := errors.Join(file.Sync(), file.Close())
				batch.mu.Lock()
				if batch.err == nil {
					batch.err = err
				}
				batch.mu.Unlock()
			}
		}()
	}
	return batch
}

// submit transfers file ownership even when a prior synchronization failed.
// The staging walk is the sole submitter and stops on the returned error.
func (batch *userPNPMFileSyncBatch) submit(file userPNPMSyncFile) error {
	batch.jobs <- file
	return batch.failure()
}

func (batch *userPNPMFileSyncBatch) failure() error {
	batch.mu.Lock()
	defer batch.mu.Unlock()
	return batch.err
}

// finish is required before inventory, publication, and error cleanup. Calling
// it again is harmless; no further submissions are permitted after it begins.
func (batch *userPNPMFileSyncBatch) finish() error {
	batch.finishOnce.Do(func() {
		close(batch.jobs)
		batch.workers.Wait()
	})
	return batch.failure()
}
