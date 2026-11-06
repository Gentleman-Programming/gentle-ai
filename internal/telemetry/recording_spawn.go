package telemetry

import (
	"context"
	"sync"
)

// RecordedSend is one call captured by a RecordingSpawner.
type RecordedSend struct {
	Payload []byte
}

// RecordingSpawner is a Spawner that never starts a real process and never
// reaches the network: it only records the payload it was asked to send.
// Any test binary whose tests exercise install/update/sync/review code
// paths — which call Opportunistic without necessarily injecting their own
// Deps.Spawn — should install one of these (or an equivalent fake) as
// DefaultSpawn from that package's TestMain, exactly as internal/cli's and
// internal/app's TestMain do. It is safe for concurrent use, since Go tests
// in a package may run in parallel.
type RecordingSpawner struct {
	mu    sync.Mutex
	calls []RecordedSend
}

// NewRecordingSpawner returns a ready-to-use recorder.
func NewRecordingSpawner() *RecordingSpawner {
	return &RecordingSpawner{}
}

// Spawn implements Spawner. Assign it directly to DefaultSpawn or Deps.Spawn.
func (r *RecordingSpawner) Spawn(_ context.Context, payload []byte) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls = append(r.calls, RecordedSend{Payload: append([]byte(nil), payload...)})
	return nil
}

// Calls returns a snapshot of every recorded send, in order.
func (r *RecordingSpawner) Calls() []RecordedSend {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]RecordedSend(nil), r.calls...)
}

// Reset clears every recorded call.
func (r *RecordingSpawner) Reset() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls = nil
}
