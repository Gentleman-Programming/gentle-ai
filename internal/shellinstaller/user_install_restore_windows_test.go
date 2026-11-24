//go:build windows

package shellinstaller

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// userWindowsRetiredFixture publishes an owned destination with a marker in a
// private parent and retires it through the installer's own retire step.
func userWindowsRetiredFixture(t *testing.T) (destination, backup, identity string) {
	t.Helper()
	parent := filepath.Join(t.TempDir(), "parent")
	if err := os.Mkdir(parent, 0700); err != nil {
		t.Fatal(err)
	}
	if err := userWindowsPrivate(parent); err != nil {
		t.Fatal(err)
	}
	destination = filepath.Join(parent, "gentle-shell")
	userWindowsPrivateDir(t, destination)
	if err := os.WriteFile(filepath.Join(destination, "marker.txt"), []byte("previous"), 0600); err != nil {
		t.Fatal(err)
	}
	backup, identity, err := userWindowsRetireReplacement(destination, time.Date(2026, 1, 1, 13, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("retire: %v", err)
	}
	if backup != destination+".replaced-20260101T130000Z" {
		t.Fatalf("backup path = %q", backup)
	}
	if _, err := os.Lstat(destination); !os.IsNotExist(err) {
		t.Fatalf("destination still present after retire: %v", err)
	}
	userWindowsAssertMarker(t, backup, "previous")
	return destination, backup, identity
}

func userWindowsPrivateDir(t *testing.T, path string) {
	t.Helper()
	if err := os.Mkdir(path, 0700); err != nil {
		t.Fatal(err)
	}
	if err := userWindowsPrivate(path); err != nil {
		t.Fatal(err)
	}
}

func userWindowsAssertMarker(t *testing.T, root, want string) {
	t.Helper()
	if got, err := os.ReadFile(filepath.Join(root, "marker.txt")); err != nil || string(got) != want {
		t.Fatalf("%s marker = %q, %v; want %q", root, got, err, want)
	}
}

func TestUserWindowsReplacementRestoredAfterFailure(t *testing.T) {
	destination, backup, identity := userWindowsRetiredFixture(t)
	err := userWindowsReplacementFailure(errors.New("node download failed"), destination, backup, identity)
	if err == nil || !strings.Contains(err.Error(), "node download failed") || !strings.Contains(err.Error(), "previous Windows installation restored to") {
		t.Fatalf("failure did not report the restore: %v", err)
	}
	userWindowsAssertMarker(t, destination, "previous")
	if _, statErr := os.Lstat(backup); !os.IsNotExist(statErr) {
		t.Fatalf("backup still present after restore: %v", statErr)
	}
}

func TestUserWindowsReplacementKeepsBothTreesWhenDestinationExists(t *testing.T) {
	destination, backup, identity := userWindowsRetiredFixture(t)
	if err := os.Mkdir(destination, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(destination, "marker.txt"), []byte("published"), 0600); err != nil {
		t.Fatal(err)
	}
	err := userWindowsReplacementFailure(errors.New("verify failed"), destination, backup, identity)
	if err == nil || !strings.Contains(err.Error(), "verify failed") || !strings.Contains(err.Error(), "both trees retained") || !strings.Contains(err.Error(), "backup retained at "+backup) {
		t.Fatalf("existing destination was not reported: %v", err)
	}
	if strings.Contains(err.Error(), "previous Windows installation restored") {
		t.Fatalf("restore claimed over an existing destination: %v", err)
	}
	userWindowsAssertMarker(t, destination, "published")
	userWindowsAssertMarker(t, backup, "previous")
}

func TestUserWindowsReplacementRefusesChangedBackupIdentity(t *testing.T) {
	destination, backup, identity := userWindowsRetiredFixture(t)
	if err := os.RemoveAll(backup); err != nil {
		t.Fatal(err)
	}
	userWindowsPrivateDir(t, backup)
	if err := os.WriteFile(filepath.Join(backup, "marker.txt"), []byte("foreign"), 0600); err != nil {
		t.Fatal(err)
	}
	err := userWindowsReplacementFailure(errors.New("provision failed"), destination, backup, identity)
	if err == nil || !strings.Contains(err.Error(), "provision failed") || !strings.Contains(err.Error(), "backup retained at "+backup) {
		t.Fatalf("changed backup identity was not refused: %v", err)
	}
	if _, statErr := os.Lstat(destination); !os.IsNotExist(statErr) {
		t.Fatalf("a foreign backup was restored: %v", statErr)
	}
	userWindowsAssertMarker(t, backup, "foreign")
}

func TestUserWindowsRetireRefusesOccupiedBackup(t *testing.T) {
	destination, backup, _ := userWindowsRetiredFixture(t)
	if err := os.Mkdir(destination, 0700); err != nil {
		t.Fatal(err)
	}
	if _, _, err := userWindowsRetireReplacement(destination, time.Date(2026, 1, 1, 13, 0, 0, 0, time.UTC)); err == nil || !strings.Contains(err.Error(), "already occupied") {
		t.Fatalf("occupied backup path accepted: %v", err)
	}
	if _, err := os.Lstat(destination); err != nil {
		t.Fatalf("destination moved despite the refusal: %v", err)
	}
	userWindowsAssertMarker(t, backup, "previous")
}

func TestUserWindowsWorkerStopCancelsAndJoins(t *testing.T) {
	var finished atomic.Bool
	started := make(chan struct{})
	worker := userWindowsStartWorker(context.Background(), func(ctx context.Context) error {
		close(started)
		<-ctx.Done()
		time.Sleep(50 * time.Millisecond) // Still writing after the cancellation.
		finished.Store(true)
		return ctx.Err()
	})
	<-started
	worker.Stop()
	if !finished.Load() {
		t.Fatal("Stop returned before the worker finished")
	}
	worker.Stop() // A second stop is a no-op.
}

func TestUserWindowsWorkerWaitReturnsResult(t *testing.T) {
	want := errors.New("codegraph failed")
	worker := userWindowsStartWorker(context.Background(), func(ctx context.Context) error {
		if ctx.Err() != nil {
			return errors.New("worker canceled before completing")
		}
		return want
	})
	if err := worker.Wait(); !errors.Is(err, want) {
		t.Fatalf("Wait = %v, want %v", err, want)
	}
	worker.Stop() // A stop after the join must not block or change the result.
	if err := worker.Wait(); !errors.Is(err, want) {
		t.Fatalf("second Wait = %v", err)
	}
}
