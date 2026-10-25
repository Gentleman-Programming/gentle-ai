package telemetry

import (
	"os"
	"sync"
	"testing"
	"time"
)

// TestLockStateExcludesSameProcessCallers pins the lockState contract
// directly: while one caller holds the state lock, a second same-process
// caller must block instead of proceeding, and must complete once the
// holder releases. Same-process exclusion must hold by construction (path
// mutex), not by how the OS byte-range lock treats handles opened by one
// process — the ga-4882 Windows failure mode.
func TestLockStateExcludesSameProcessCallers(t *testing.T) {
	home := t.TempDir()

	unlockA, err := lockState(home)
	if err != nil {
		t.Fatalf("first lockState: %v", err)
	}

	type lockResult struct {
		unlock func()
		err    error
	}
	acquired := make(chan lockResult, 1)
	go func() {
		unlock, err := lockState(home)
		acquired <- lockResult{unlock: unlock, err: err}
	}()

	// The second caller must still be blocked while the first holds the
	// lock; the margin is generous so slow CI cannot flake it.
	select {
	case res := <-acquired:
		if res.unlock != nil {
			res.unlock()
		}
		t.Fatal("second lockState completed while the first still held the lock")
	case <-time.After(150 * time.Millisecond):
	}

	unlockA()

	var res lockResult
	select {
	case res = <-acquired:
	case <-time.After(5 * time.Second):
		t.Fatal("second lockState never completed within 5s after the holder released")
	}
	if res.err != nil {
		t.Fatalf("second lockState: %v", res.err)
	}
	res.unlock()
}

// assertConcurrentUpdatesAllLand drives N concurrent Update calls against
// one home and fails unless every increment persisted.
func assertConcurrentUpdatesAllLand(t *testing.T) {
	t.Helper()
	home := t.TempDir()

	const n = 20
	var wg sync.WaitGroup
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func() {
			defer wg.Done()
			if err := Update(home, func(s *State) { s.Counters.Syncs++ }); err != nil {
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
		t.Fatalf("Counters.Syncs = %d, want %d: same-process updates were lost", s.Counters.Syncs, n)
	}
}

// TestUpdateSameProcessExclusionSurvivesBrokenFileLock reproduces the
// ga-4882 Windows failure mode on every OS by sabotaging the cross-process
// file lock: with lockFileExclusive a no-op, same-process mutual exclusion
// must still hold (carried by the path mutex), so no update is lost. The
// second config keeps the file lock working but slow, proving the mutex
// serializes independently of how long the file lock takes.
func TestUpdateSameProcessExclusionSurvivesBrokenFileLock(t *testing.T) {
	t.Run("no-op file lock", func(t *testing.T) {
		original := lockFileExclusive
		t.Cleanup(func() { lockFileExclusive = original })
		lockFileExclusive = func(*os.File) error { return nil }

		assertConcurrentUpdatesAllLand(t)
	})

	t.Run("slow file lock", func(t *testing.T) {
		original := lockFileExclusive
		t.Cleanup(func() { lockFileExclusive = original })
		lockFileExclusive = func(*os.File) error { time.Sleep(10 * time.Millisecond); return nil }

		assertConcurrentUpdatesAllLand(t)
	})
}
