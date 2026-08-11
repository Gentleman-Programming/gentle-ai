package state

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sync"
	"testing"
	"time"
)

// TestUpdatePreservesConcurrentRDDModeChange is the regression for issue #1809:
// actor A reads, actor B writes rdd_mode=off, then actor A persists its own
// field from the snapshot it captured before B ran. The old Read/mutate/Write
// pattern sent the whole stale document back and restored rdd_mode=on.
func TestUpdatePreservesConcurrentRDDModeChange(t *testing.T) {
	homeDir := t.TempDir()
	if err := Write(homeDir, InstallState{RDDMode: "on", Persona: "gentleman"}); err != nil {
		t.Fatalf("seed state: %v", err)
	}

	stale, err := Read(homeDir) // actor A, held across a long operation
	if err != nil {
		t.Fatalf("actor A read: %v", err)
	}

	// Actor B turns the kill switch off while A is still working.
	if err := Update(homeDir, func(s *InstallState) error {
		s.RDDMode = "off"
		return nil
	}); err != nil {
		t.Fatalf("actor B update: %v", err)
	}

	// Actor A now persists the persona it derived from its stale snapshot.
	if err := Update(homeDir, func(s *InstallState) error {
		if s.RDDMode != "off" {
			t.Errorf("closure saw RDDMode = %q, want off; the re-read must happen inside the lock", s.RDDMode)
		}
		s.Persona = stale.Persona + "-updated"
		return nil
	}); err != nil {
		t.Fatalf("actor A update: %v", err)
	}

	final, err := Read(homeDir)
	if err != nil {
		t.Fatalf("read final: %v", err)
	}
	if final.RDDMode != "off" {
		t.Errorf("RDDMode = %q, want off; actor B's kill-switch decision was clobbered", final.RDDMode)
	}
	if final.Persona != "gentleman-updated" {
		t.Errorf("Persona = %q, want gentleman-updated; actor A's change was lost", final.Persona)
	}
}

// TestConcurrentUpdatesDoNotLoseWrites proves the lock serialises writers that
// contend within one process. Without it, concurrent appends overwrite each
// other and entries disappear. Cross-process contention uses the same advisory
// lock but is not exercised here.
func TestConcurrentUpdatesDoNotLoseWrites(t *testing.T) {
	homeDir := t.TempDir()
	if err := Write(homeDir, InstallState{}); err != nil {
		t.Fatalf("seed state: %v", err)
	}

	const writers = 10
	done := make(chan error, writers)
	for i := range writers {
		go func() {
			done <- Update(homeDir, func(s *InstallState) error {
				s.InstalledAgents = append(s.InstalledAgents, fmt.Sprintf("agent-%d", i))
				return nil
			})
		}()
	}
	for range writers {
		if err := <-done; err != nil {
			t.Fatalf("concurrent update: %v", err)
		}
	}

	final, err := Read(homeDir)
	if err != nil {
		t.Fatalf("read final: %v", err)
	}
	seen := make(map[string]bool, writers)
	for _, agent := range final.InstalledAgents {
		seen[agent] = true
	}
	for i := range writers {
		if want := fmt.Sprintf("agent-%d", i); !seen[want] {
			t.Errorf("missing %s: %d of %d writers survived", want, len(seen), writers)
		}
	}
}

// TestWriteWaitsForUpdateLock is the direct coverage for the invariant Write's
// doc comment asserts: the two share one lock, so a whole-document Write cannot
// land inside an Update read-modify-write window. If Write ever stops taking the
// lock, it completes while the window is open and the first assertion fails.
func TestWriteWaitsForUpdateLock(t *testing.T) {
	homeDir := t.TempDir()
	if err := Write(homeDir, InstallState{Persona: "gentleman"}); err != nil {
		t.Fatalf("seed state: %v", err)
	}

	resume, updateDone := startUpdateWindow(t, homeDir, func(s *InstallState) {
		s.Persona += "-updated"
	})

	writeDone := make(chan error, 1)
	go func() { writeDone <- Write(homeDir, InstallState{Persona: "written"}) }()
	select {
	case err := <-writeDone:
		t.Fatalf("Write completed inside the Update window (err = %v); the two interleaved", err)
	case <-time.After(300 * time.Millisecond):
	}

	resume()
	if err := <-updateDone; err != nil {
		t.Fatalf("Update: %v", err)
	}
	if err := <-writeDone; err != nil {
		t.Fatalf("Write: %v", err)
	}

	final, err := Read(homeDir)
	if err != nil {
		t.Fatalf("read final: %v", err)
	}
	if final.Persona != "written" {
		t.Errorf("Persona = %q, want written; Write must land after the window, not inside it", final.Persona)
	}
}

// TestLockTimeoutWhenLockHeld covers ErrLockTimeout on both entry points. It
// holds the lock through acquireStateLock, which is exactly what a nested call
// from inside mutate would do, so it also demonstrates the documented
// non-reentrancy.
func TestLockTimeoutWhenLockHeld(t *testing.T) {
	homeDir := t.TempDir()
	previous := lockTimeout
	lockTimeout = 50 * time.Millisecond
	t.Cleanup(func() { lockTimeout = previous })

	release, err := acquireStateLock(homeDir)
	if err != nil {
		t.Fatalf("acquire held lock: %v", err)
	}
	defer release()

	err = Update(homeDir, func(*InstallState) error {
		t.Error("mutate ran while another holder had the lock")
		return nil
	})
	if !errors.Is(err, ErrLockTimeout) {
		t.Errorf("Update error = %v, want ErrLockTimeout", err)
	}
	if err := Write(homeDir, InstallState{Persona: "neutral"}); !errors.Is(err, ErrLockTimeout) {
		t.Errorf("Write error = %v, want ErrLockTimeout", err)
	}
	if _, err := os.Stat(Path(homeDir)); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("os.Stat(state file) = %v, want not-exist; a timed-out call must not write", err)
	}
}

func TestUpdateMutationErrorLeavesFileUnchanged(t *testing.T) {
	homeDir := t.TempDir()
	if err := Write(homeDir, InstallState{Persona: "gentleman"}); err != nil {
		t.Fatalf("seed state: %v", err)
	}

	sentinel := errors.New("mutation rejected")
	err := Update(homeDir, func(s *InstallState) error {
		s.Persona = "neutral"
		return sentinel
	})
	if !errors.Is(err, sentinel) {
		t.Fatalf("Update error = %v, want %v", err, sentinel)
	}

	final, err := Read(homeDir)
	if err != nil {
		t.Fatalf("read final: %v", err)
	}
	if final.Persona != "gentleman" {
		t.Errorf("Persona = %q, want gentleman; a failed mutation must not be persisted", final.Persona)
	}
}

func TestUpdateCreatesStateFromZeroValue(t *testing.T) {
	homeDir := t.TempDir()

	if err := Update(homeDir, func(s *InstallState) error {
		if !reflect.DeepEqual(*s, InstallState{}) {
			t.Errorf("closure received %+v, want the zero value when no state file exists", *s)
		}
		s.Persona = "neutral"
		return nil
	}); err != nil {
		t.Fatalf("Update: %v", err)
	}

	final, err := Read(homeDir)
	if err != nil {
		t.Fatalf("read final: %v", err)
	}
	if final.Persona != "neutral" {
		t.Errorf("Persona = %q, want neutral", final.Persona)
	}
}

// TestUpdateRefusesToOverwriteCorruptState mirrors the no-clobber guards the
// call sites already relied on.
func TestUpdateRefusesToOverwriteCorruptState(t *testing.T) {
	homeDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(homeDir, stateDir), 0o755); err != nil {
		t.Fatalf("create state dir: %v", err)
	}
	const corrupt = "{not json"
	if err := os.WriteFile(Path(homeDir), []byte(corrupt), 0o644); err != nil {
		t.Fatalf("seed corrupt state: %v", err)
	}

	called := false
	if err := Update(homeDir, func(*InstallState) error {
		called = true
		return nil
	}); err == nil {
		t.Fatal("Update returned nil, want a decode error for a corrupt state file")
	}
	if called {
		t.Error("mutation ran against a corrupt state file")
	}

	data, err := os.ReadFile(Path(homeDir))
	if err != nil {
		t.Fatalf("read state file: %v", err)
	}
	if string(data) != corrupt {
		t.Errorf("state file = %q, want it left untouched", string(data))
	}
}

func TestUpdateReleasesLockOnPanic(t *testing.T) {
	homeDir := t.TempDir()
	if err := Write(homeDir, InstallState{}); err != nil {
		t.Fatalf("seed state: %v", err)
	}

	func() {
		defer func() { _ = recover() }()
		_ = Update(homeDir, func(*InstallState) error { panic("mutation panic") })
	}()

	done := make(chan error, 1)
	go func() {
		done <- Update(homeDir, func(s *InstallState) error {
			s.Persona = "neutral"
			return nil
		})
	}()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Update after panic: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Update after panic blocked; the lock was not released")
	}
}

// TestLockTimeoutDoesNotOvershoot pins the acquisition deadline. The retry loop
// must check the deadline before each attempt and cap its sleep to the time
// remaining, otherwise a 20ms timeout waits a full 100ms lockRetryDelay and can
// even acquire the lock after the deadline has already passed.
func TestLockTimeoutDoesNotOvershoot(t *testing.T) {
	homeDir := t.TempDir()
	previous := lockTimeout
	lockTimeout = 20 * time.Millisecond
	t.Cleanup(func() { lockTimeout = previous })

	release, err := acquireStateLock(homeDir)
	if err != nil {
		t.Fatalf("acquire held lock: %v", err)
	}
	t.Cleanup(release)

	start := time.Now()
	if _, err := acquireStateLock(homeDir); !errors.Is(err, ErrLockTimeout) {
		t.Fatalf("second acquire error = %v, want ErrLockTimeout", err)
	}
	elapsed := time.Since(start)
	if elapsed > 60*time.Millisecond {
		t.Errorf("acquire took %v for a %v timeout; the loop sleeps past its own deadline", elapsed, lockTimeout)
	}
}

// TestUpdateWindowIsReleasedWhenTheSubtestEnds pins the scaffolding contract. A
// test that opens an Update window and returns without resuming it must still
// release the state lock, or the window stays open and every later test in the
// package times out — which is exactly what happens when a real test fails
// early inside the window. The subtest below leaves the window open on purpose;
// the outer assertion proves the helper's cleanup closed it.
//
// The subtest must PASS. An earlier draft made it fail deliberately, but Go
// propagates a subtest failure to its parent and to the package result, so that
// design reported FAIL forever. Leaving the window open exercises the same
// cleanup path without poisoning the suite.
func TestUpdateWindowIsReleasedWhenTheSubtestEnds(t *testing.T) {
	homeDir := t.TempDir()
	if err := Write(homeDir, InstallState{Persona: "gentleman"}); err != nil {
		t.Fatalf("seed state: %v", err)
	}

	t.Run("leaves the window open", func(t *testing.T) {
		startUpdateWindow(t, homeDir, func(s *InstallState) { s.Persona = "resumed-by-cleanup" })
	})

	previous := lockTimeout
	lockTimeout = 500 * time.Millisecond
	t.Cleanup(func() { lockTimeout = previous })

	release, err := acquireStateLock(homeDir)
	if err != nil {
		t.Fatalf("state lock still held after the subtest: %v", err)
	}
	release()
}

// startUpdateWindow enters an Update read-modify-write window and blocks inside
// mutate until the returned resume func runs. resume is registered with
// t.Cleanup and guarded by sync.Once, so every exit path — a passing test, a
// failed assertion, or an explicit call — releases the state lock exactly once.
func startUpdateWindow(t *testing.T, homeDir string, apply func(*InstallState)) (resume func(), done <-chan error) {
	t.Helper()
	inMutate, resumeCh := make(chan struct{}), make(chan struct{})
	var once sync.Once
	resume = func() { once.Do(func() { close(resumeCh) }) }
	t.Cleanup(resume)

	errCh := make(chan error, 1)
	go func() {
		errCh <- Update(homeDir, func(s *InstallState) error {
			close(inMutate)
			<-resumeCh
			apply(s)
			return nil
		})
	}()
	<-inMutate
	return resume, errCh
}
