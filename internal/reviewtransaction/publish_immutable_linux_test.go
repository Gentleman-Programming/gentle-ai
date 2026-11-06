//go:build linux

package reviewtransaction

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/sys/unix"
)

type testImmutablePublicationDestination struct {
	*os.File
	chmodErr error
	syncErr  error
	closeErr error
	synced   bool
	closed   bool
}

func (file *testImmutablePublicationDestination) Chmod(mode os.FileMode) error {
	if file.chmodErr != nil {
		return file.chmodErr
	}
	return file.File.Chmod(mode)
}

func (file *testImmutablePublicationDestination) Sync() error {
	file.synced = true
	if file.syncErr != nil {
		return file.syncErr
	}
	return file.File.Sync()
}

func (file *testImmutablePublicationDestination) Close() error {
	file.closed = true
	err := file.File.Close()
	if file.closeErr != nil {
		return file.closeErr
	}
	return err
}

func forceLinuxCopyFallback(t *testing.T, renameErr error) {
	t.Helper()
	original := renameNoReplace
	renameNoReplace = func(int, string, int, string, uint) error { return renameErr }
	t.Cleanup(func() { renameNoReplace = original })
}

func writeImmutablePublicationSource(t *testing.T, dir string) string {
	t.Helper()
	source := filepath.Join(dir, "source.tmp")
	if err := os.WriteFile(source, []byte("receipt\n"), 0o640); err != nil {
		t.Fatal(err)
	}
	return source
}

func TestPublishNoReplaceLinuxFallsBackToExclusiveCopy(t *testing.T) {
	for _, tt := range []struct {
		name      string
		renameErr error
	}{
		{name: "EXDEV", renameErr: unix.EXDEV},
		{name: "ENOSYS", renameErr: unix.ENOSYS},
		{name: "EINVAL", renameErr: unix.EINVAL},
		{name: "EOPNOTSUPP", renameErr: unix.EOPNOTSUPP},
	} {
		t.Run(tt.name, func(t *testing.T) {
			forceLinuxCopyFallback(t, tt.renameErr)

			dir := t.TempDir()
			source := writeImmutablePublicationSource(t, dir)
			destination := filepath.Join(dir, "receipt.json")
			if err := publishNoReplace(source, destination); err != nil {
				t.Fatalf("publishNoReplace(%s fallback) error = %v", tt.name, err)
			}
			payload, err := os.ReadFile(destination)
			if err != nil {
				t.Fatal(err)
			}
			if string(payload) != "receipt\n" {
				t.Fatalf("destination payload = %q, want copied source content", payload)
			}
			info, err := os.Stat(destination)
			if err != nil {
				t.Fatal(err)
			}
			if info.Mode().Perm() != 0o640 {
				t.Fatalf("destination mode = %v, want 0640", info.Mode().Perm())
			}
			if _, err := os.Stat(source); !os.IsNotExist(err) {
				t.Fatalf("source still exists after durable copy: %v", err)
			}
		})
	}
}

func TestPublishNoReplaceLinuxCopyFallbackNeverReplacesDestination(t *testing.T) {
	forceLinuxCopyFallback(t, unix.EXDEV)

	dir := t.TempDir()
	source := writeImmutablePublicationSource(t, dir)
	destination := filepath.Join(dir, "receipt.json")
	if err := os.WriteFile(destination, []byte("existing\n"), 0o640); err != nil {
		t.Fatal(err)
	}

	if err := publishNoReplace(source, destination); !os.IsExist(err) {
		t.Fatalf("publishNoReplace(existing destination) error = %v, want exists", err)
	}
	payload, err := os.ReadFile(destination)
	if err != nil {
		t.Fatal(err)
	}
	if string(payload) != "existing\n" {
		t.Fatalf("existing destination was replaced: %q", payload)
	}
	if _, err := os.Stat(source); err != nil {
		t.Fatalf("source was removed after destination conflict: %v", err)
	}
}

func TestPublishNoReplaceLinuxCopyFallbackCleansUpCopyFailure(t *testing.T) {
	forceLinuxCopyFallback(t, unix.EXDEV)
	original := copyImmutablePublication
	want := errors.New("copy failed")
	copyImmutablePublication = func(io.Writer, io.Reader) (int64, error) { return 0, want }
	t.Cleanup(func() { copyImmutablePublication = original })

	dir := t.TempDir()
	source := writeImmutablePublicationSource(t, dir)
	destination := filepath.Join(dir, "receipt.json")
	if err := publishNoReplace(source, destination); !errors.Is(err, want) {
		t.Fatalf("publishNoReplace(copy failure) error = %v, want %v", err, want)
	}
	if _, err := os.Stat(destination); !os.IsNotExist(err) {
		t.Fatalf("partial destination remains after copy failure: %v", err)
	}
	if _, err := os.Stat(source); err != nil {
		t.Fatalf("source was removed after copy failure: %v", err)
	}
}

func TestPublishNoReplaceLinuxCopyFallbackCleansUpSyncAndCloseFailures(t *testing.T) {
	for _, tt := range []struct {
		name     string
		syncErr  error
		closeErr error
	}{
		{name: "sync", syncErr: errors.New("sync failed")},
		{name: "close", closeErr: errors.New("close failed")},
	} {
		t.Run(tt.name, func(t *testing.T) {
			forceLinuxCopyFallback(t, unix.EXDEV)
			original := createImmutablePublicationDestination
			var destinationFile *testImmutablePublicationDestination
			createImmutablePublicationDestination = func(path string, flag int, mode os.FileMode) (immutablePublicationDestination, error) {
				file, err := os.OpenFile(path, flag, mode)
				if err != nil {
					return nil, err
				}
				destinationFile = &testImmutablePublicationDestination{File: file, syncErr: tt.syncErr, closeErr: tt.closeErr}
				return destinationFile, nil
			}
			t.Cleanup(func() { createImmutablePublicationDestination = original })

			dir := t.TempDir()
			source := writeImmutablePublicationSource(t, dir)
			destination := filepath.Join(dir, "receipt.json")
			if err := publishNoReplace(source, destination); !errors.Is(err, tt.syncErr) && !errors.Is(err, tt.closeErr) {
				t.Fatalf("publishNoReplace(%s failure) error = %v", tt.name, err)
			}
			if destinationFile == nil || !destinationFile.closed {
				t.Fatal("destination file was not closed on publication failure")
			}
			if _, err := os.Stat(destination); !os.IsNotExist(err) {
				t.Fatalf("partial destination remains after %s failure: %v", tt.name, err)
			}
			if _, err := os.Stat(source); err != nil {
				t.Fatalf("source was removed after %s failure: %v", tt.name, err)
			}
		})
	}
}

func TestPublishNoReplaceLinuxCopyFallbackJoinsDestinationCleanupFailure(t *testing.T) {
	for _, stage := range []string{"chmod", "copy", "sync", "close"} {
		t.Run(stage, func(t *testing.T) {
			forceLinuxCopyFallback(t, unix.EXDEV)
			primary := errors.New(stage + " failed")
			cleanup := errors.New("destination cleanup failed")
			originalCreate, originalCopy, originalRemove := createImmutablePublicationDestination, copyImmutablePublication, removeImmutablePublication
			t.Cleanup(func() {
				createImmutablePublicationDestination, copyImmutablePublication, removeImmutablePublication = originalCreate, originalCopy, originalRemove
			})
			createImmutablePublicationDestination = func(path string, flag int, mode os.FileMode) (immutablePublicationDestination, error) {
				file, err := os.OpenFile(path, flag, mode)
				if err != nil {
					return nil, err
				}
				destination := &testImmutablePublicationDestination{File: file}
				switch stage {
				case "chmod":
					destination.chmodErr = primary
				case "sync":
					destination.syncErr = primary
				case "close":
					destination.closeErr = primary
				}
				return destination, nil
			}
			if stage == "copy" {
				copyImmutablePublication = func(io.Writer, io.Reader) (int64, error) { return 0, primary }
			}

			dir := t.TempDir()
			source := writeImmutablePublicationSource(t, dir)
			destination := filepath.Join(dir, "receipt.json")
			sourceRemoved := false
			removeImmutablePublication = func(path string) error {
				if path == destination {
					return cleanup
				}
				if path == source {
					sourceRemoved = true
				}
				return os.Remove(path)
			}

			err := publishNoReplace(source, destination)
			if !errors.Is(err, primary) || !errors.Is(err, cleanup) {
				t.Fatalf("publishNoReplace(%s) error = %v, want both primary and cleanup causes", stage, err)
			}
			if sourceRemoved {
				t.Fatal("source was removed after failed destination cleanup")
			}
			if _, err := os.Stat(source); err != nil {
				t.Fatalf("source is missing after failed destination cleanup: %v", err)
			}
			if _, err := os.Stat(destination); err != nil {
				t.Fatalf("partial destination was not exposed after failed cleanup: %v", err)
			}
		})
	}
}

func TestPublishNoReplaceLinuxCopyFallbackRemovesSourceOnlyAfterDurableDestination(t *testing.T) {
	forceLinuxCopyFallback(t, unix.EXDEV)
	originalCreate := createImmutablePublicationDestination
	originalRemove := removeImmutablePublication
	var destinationFile *testImmutablePublicationDestination
	createImmutablePublicationDestination = func(path string, flag int, mode os.FileMode) (immutablePublicationDestination, error) {
		file, err := os.OpenFile(path, flag, mode)
		if err != nil {
			return nil, err
		}
		destinationFile = &testImmutablePublicationDestination{File: file}
		return destinationFile, nil
	}
	t.Cleanup(func() { createImmutablePublicationDestination = originalCreate })

	dir := t.TempDir()
	source := writeImmutablePublicationSource(t, dir)
	destination := filepath.Join(dir, "receipt.json")
	removeImmutablePublication = func(path string) error {
		if path == source {
			if destinationFile == nil || !destinationFile.synced || !destinationFile.closed {
				t.Fatal("source removal preceded destination sync and close")
			}
			payload, err := os.ReadFile(destination)
			if err != nil || string(payload) != "receipt\n" {
				t.Fatalf("destination was not durable before source removal: %q, %v", payload, err)
			}
		}
		return os.Remove(path)
	}
	t.Cleanup(func() { removeImmutablePublication = originalRemove })

	if err := publishNoReplace(source, destination); err != nil {
		t.Fatalf("publishNoReplace(copy fallback) error = %v", err)
	}
	if _, err := os.Stat(source); !os.IsNotExist(err) {
		t.Fatalf("source remains after durable publication: %v", err)
	}
}
