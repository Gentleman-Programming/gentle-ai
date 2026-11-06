package telemetry

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestIncrementSyncsUnderKillSwitchCreatesNoStateFileAndReturnsNil(t *testing.T) {
	for _, tc := range []struct {
		key   string
		value string
	}{
		{"DO_NOT_TRACK", "1"},
		{"GENTLE_AI_TELEMETRY", "0"},
		{"CI", "true"},
		{"GITHUB_ACTIONS", "true"},
	} {
		t.Run(tc.key, func(t *testing.T) {
			home := t.TempDir()
			for _, key := range []string{"DO_NOT_TRACK", "GENTLE_AI_TELEMETRY", "CI", "GITHUB_ACTIONS"} {
				t.Setenv(key, "")
			}
			t.Setenv(tc.key, tc.value)
			if err := IncrementSyncs(home); err != nil {
				t.Fatalf("IncrementSyncs under a kill switch returned %v, want nil", err)
			}
			entries, err := os.ReadDir(home)
			if err != nil {
				t.Fatal(err)
			}
			if len(entries) != 0 {
				t.Fatalf("disabled increment touched disk: %v", entries)
			}
		})
	}
}

func TestIncrementSyncsWaitsForWriterBeforeReadingPolicy(t *testing.T) {
	home := t.TempDir()
	t.Setenv("DO_NOT_TRACK", "")
	t.Setenv("GENTLE_AI_TELEMETRY", "")
	t.Setenv("CI", "")
	t.Setenv("GITHUB_ACTIONS", "")

	if err := IncrementSyncs(home); err != nil {
		t.Fatal(err)
	}
	s, err := Load(home)
	if err != nil {
		t.Fatal(err)
	}
	unlock, err := lockState(home)
	if err != nil {
		t.Fatal(err)
	}
	// Represent a state that must not be inspected until its writer releases
	// the lock. An early policy read would silently discard the increment.
	if err := os.WriteFile(Path(home), []byte("incomplete state"), 0o600); err != nil {
		unlock()
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- IncrementSyncs(home) }()
	var completedEarly bool
	select {
	case err := <-done:
		completedEarly = true
		t.Errorf("increment completed before the writer released the lock: %v", err)
	case <-time.After(150 * time.Millisecond):
	}
	// Restore the valid state while still holding the writer's lock.
	err = Save(home, s)
	unlock()
	if err != nil {
		t.Fatal(err)
	}
	if !completedEarly {
		select {
		case err := <-done:
			if err != nil {
				t.Fatal(err)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("increment did not complete after the writer released the lock")
		}
	}
	s, err = Load(home)
	if err != nil {
		t.Fatal(err)
	}
	if s.Counters.Syncs != 2 {
		t.Fatalf("syncs = %d, want 2", s.Counters.Syncs)
	}
}

func TestIncrementSyncsDisabledOrUnreadableStateRemainsUnchanged(t *testing.T) {
	for _, tc := range []struct {
		name string
		data string
	}{
		{"persisted opt-out", `{"install_id":"existing", "enabled":false, "counters":{"syncs":7}}`},
		{"unreadable state", "invalid JSON"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("DO_NOT_TRACK", "")
			t.Setenv("GENTLE_AI_TELEMETRY", "")
			t.Setenv("CI", "")
			t.Setenv("GITHUB_ACTIONS", "")
			if err := os.MkdirAll(filepath.Join(home, stateDir), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(Path(home), []byte(tc.data), 0o600); err != nil {
				t.Fatal(err)
			}
			if err := IncrementSyncs(home); err != nil {
				t.Fatalf("increment = %v, want nil", err)
			}
			data, err := os.ReadFile(Path(home))
			if err != nil {
				t.Fatal(err)
			}
			if string(data) != tc.data {
				t.Fatalf("state changed: got %q, want %q", data, tc.data)
			}
		})
	}
}

func TestIncrementSyncsWhenEnabledPersistsTheCounter(t *testing.T) {
	home := t.TempDir()
	// Pin every kill switch: the gate reads the ambient environment, and a
	// developer shell or CI runner may carry any of them.
	t.Setenv("DO_NOT_TRACK", "")
	t.Setenv("GENTLE_AI_TELEMETRY", "")
	t.Setenv("CI", "")
	t.Setenv("GITHUB_ACTIONS", "")

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
// same home must all land. The hybrid lock must cover policy reads as well
// as the load-mutate-save sequence.
func TestIncrementSyncsConcurrentDoesNotLoseUpdates(t *testing.T) {
	home := t.TempDir()
	t.Setenv("DO_NOT_TRACK", "")
	t.Setenv("GENTLE_AI_TELEMETRY", "")
	t.Setenv("CI", "")
	t.Setenv("GITHUB_ACTIONS", "")

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
