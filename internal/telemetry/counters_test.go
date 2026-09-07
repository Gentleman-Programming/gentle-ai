package telemetry

import (
	"os"
	"sync"
	"testing"
)

func TestIncrementSyncsUnderKillSwitchCreatesNoStateFileAndReturnsNil(t *testing.T) {
	home := t.TempDir()
	t.Setenv("DO_NOT_TRACK", "1")

	if err := IncrementSyncs(home); err != nil {
		t.Fatalf("IncrementSyncs under a kill switch returned %v, want nil", err)
	}
	if _, err := os.Stat(Path(home)); !os.IsNotExist(err) {
		t.Fatalf("state file stat err = %v, want a disabled IncrementSyncs to leave no state file behind", err)
	}
}

func TestIncrementSyncsWhenEnabledPersistsTheCounter(t *testing.T) {
	home := t.TempDir()
	// Pin every kill switch: the gate reads the ambient environment, and a
	// developer shell or CI runner may carry any of them.
	t.Setenv("DO_NOT_TRACK", "")
	t.Setenv("GENTLE_AI_TELEMETRY", "")
	t.Setenv("CI", "")

	if err := IncrementSyncs(home); err != nil {
		t.Fatal(err)
	}
	s, err := Load(home)
	if err != nil {
		t.Fatal(err)
	}
	if s.Counters.Syncs != 1 {
		t.Fatalf("syncs = %d, want 1", s.Counters.Syncs)
	}
}

// Guards R4-state-lost-update: N concurrent IncrementSyncs calls against the
// same home must all land. Each opens its own lock fd, so this exercises
// real flock/LockFileEx contention, not an in-process mutex.
func TestIncrementSyncsConcurrentDoesNotLoseUpdates(t *testing.T) {
	home := t.TempDir()
	t.Setenv("DO_NOT_TRACK", "")
	t.Setenv("GENTLE_AI_TELEMETRY", "")
	t.Setenv("CI", "")

	const n = 20
	var wg sync.WaitGroup
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func() {
			defer wg.Done()
			if err := IncrementSyncs(home); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()

	s, err := Load(home)
	if err != nil {
		t.Fatal(err)
	}
	if s.Counters.Syncs != n {
		t.Fatalf("Counters.Syncs = %d, want %d", s.Counters.Syncs, n)
	}
}
