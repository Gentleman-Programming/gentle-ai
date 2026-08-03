package state

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
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

// TestConcurrentUpdatesDoNotLoseWrites proves the lock serialises writers.
// Without it, concurrent appends overwrite each other and entries disappear.
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

// TestTryLockFileReportsBusyForSecondHandle covers the platform primitive
// directly: a second open handle on an already-locked file reports busy rather
// than returning an error or blocking.
func TestTryLockFileReportsBusyForSecondHandle(t *testing.T) {
	path := filepath.Join(t.TempDir(), "lock")

	first, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		t.Fatalf("open first handle: %v", err)
	}
	defer first.Close()
	locked, err := tryLockFile(first)
	if err != nil {
		t.Fatalf("lock first handle: %v", err)
	}
	if !locked {
		t.Fatal("first handle could not take a free lock")
	}

	second, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		t.Fatalf("open second handle: %v", err)
	}
	defer second.Close()
	locked, err = tryLockFile(second)
	if err != nil {
		t.Fatalf("lock second handle: %v", err)
	}
	if locked {
		t.Error("second handle took a held lock; the lock does not exclude")
	}

	if err := unlockFile(first); err != nil {
		t.Fatalf("unlock first handle: %v", err)
	}
	locked, err = tryLockFile(second)
	if err != nil {
		t.Fatalf("relock second handle: %v", err)
	}
	if !locked {
		t.Error("second handle could not take the lock after release")
	}
}
