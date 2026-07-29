//go:build darwin

package reviewtransaction

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/sys/unix"
)

// TestPublishNoReplaceDarwinRealRenameSucceeds proves the ordinary path is
// unchanged: on a filesystem that supports the exclusive rename syscall (the
// working configuration this fail-safe must never regress), publishing
// succeeds through the real syscall and a second publish to the same
// destination is refused without falling back to the copy path.
func TestPublishNoReplaceDarwinRealRenameSucceeds(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "source.tmp")
	destination := filepath.Join(dir, "receipt.json")
	if err := os.WriteFile(source, []byte("receipt\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := publishNoReplace(source, destination); err != nil {
		t.Fatalf("publishNoReplace(real rename) error = %v", err)
	}
	if _, err := os.Stat(source); !os.IsNotExist(err) {
		t.Fatalf("source still exists after real rename: %v", err)
	}
	payload, err := os.ReadFile(destination)
	if err != nil {
		t.Fatal(err)
	}
	if string(payload) != "receipt\n" {
		t.Fatalf("destination payload = %q, want the source content", payload)
	}

	second := filepath.Join(dir, "second.tmp")
	if err := os.WriteFile(second, []byte("conflict\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := publishNoReplace(second, destination); err == nil {
		t.Fatal("publishNoReplace(existing destination) unexpectedly succeeded")
	}
}

// TestPublishNoReplaceDarwinENOTSUPFallsBackToCopy proves 1804: when the
// platform rename syscall reports it is unsupported — the real shape on an
// ExFAT-hosted Git common directory, where hard links are unavailable too —
// publication falls back to exclusive-create, copy, fsync, remove-source
// instead of failing the publication outright.
func TestPublishNoReplaceDarwinENOTSUPFallsBackToCopy(t *testing.T) {
	for _, tt := range []struct {
		name  string
		errno error
	}{
		{name: "ENOTSUP", errno: unix.ENOTSUP},
		{name: "EINVAL", errno: unix.EINVAL},
		{name: "ENOSYS", errno: unix.ENOSYS},
	} {
		t.Run(tt.name, func(t *testing.T) {
			original := renameNoReplace
			renameNoReplace = func(int, string, int, string, uint32) error { return tt.errno }
			t.Cleanup(func() { renameNoReplace = original })

			dir := t.TempDir()
			source := filepath.Join(dir, "source.tmp")
			destination := filepath.Join(dir, "receipt.json")
			if err := os.WriteFile(source, []byte("receipt\n"), 0o640); err != nil {
				t.Fatal(err)
			}
			if err := publishNoReplace(source, destination); err != nil {
				t.Fatalf("publishNoReplace(%s fallback) error = %v", tt.name, err)
			}
			payload, err := os.ReadFile(destination)
			if err != nil {
				t.Fatal(err)
			}
			if string(payload) != "receipt\n" {
				t.Fatalf("destination payload = %q, want the copied source content", payload)
			}
			info, err := os.Stat(destination)
			if err != nil {
				t.Fatal(err)
			}
			if info.Mode().Perm() != 0o640 {
				t.Fatalf("destination mode = %v, want the source mode 0640 preserved", info.Mode().Perm())
			}
			if _, err := os.Stat(source); !os.IsNotExist(err) {
				t.Fatalf("source still exists after copy fallback: %v", err)
			}
		})
	}
}

// TestPublishNoReplaceDarwinENOTSUPFallbackNeverReplaces proves the copy
// fallback keeps the exclusive-create invariant: an existing destination
// refuses the fallback exactly like the real syscall would, and neither the
// destination content nor the source is touched.
func TestPublishNoReplaceDarwinENOTSUPFallbackNeverReplaces(t *testing.T) {
	original := renameNoReplace
	renameNoReplace = func(int, string, int, string, uint32) error { return unix.ENOTSUP }
	t.Cleanup(func() { renameNoReplace = original })

	dir := t.TempDir()
	source := filepath.Join(dir, "source.tmp")
	destination := filepath.Join(dir, "receipt.json")
	if err := os.WriteFile(source, []byte("new\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(destination, []byte("existing\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := publishNoReplace(source, destination); err == nil {
		t.Fatal("publishNoReplace(ENOTSUP fallback, existing destination) unexpectedly succeeded")
	}
	payload, err := os.ReadFile(destination)
	if err != nil {
		t.Fatal(err)
	}
	if string(payload) != "existing\n" {
		t.Fatalf("existing destination was overwritten: %q", payload)
	}
	if _, err := os.Stat(source); err != nil {
		t.Fatalf("source was removed on a refused publish: %v", err)
	}
}

// TestPublishNoReplaceDarwinOtherErrnoPropagatesUnchanged proves every other
// rename errno still propagates unchanged instead of falling back to copy.
func TestPublishNoReplaceDarwinOtherErrnoPropagatesUnchanged(t *testing.T) {
	original := renameNoReplace
	sentinel := errors.New("some other rename failure")
	renameNoReplace = func(int, string, int, string, uint32) error { return sentinel }
	t.Cleanup(func() { renameNoReplace = original })

	dir := t.TempDir()
	source := filepath.Join(dir, "source.tmp")
	destination := filepath.Join(dir, "receipt.json")
	if err := os.WriteFile(source, []byte("receipt\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := publishNoReplace(source, destination); !errors.Is(err, sentinel) {
		t.Fatalf("publishNoReplace(other errno) = %v, want the unchanged sentinel", err)
	}
	if _, err := os.Stat(destination); !os.IsNotExist(err) {
		t.Fatalf("destination was created for a non-ENOTSUP rename failure: %v", err)
	}
	if _, err := os.Stat(source); err != nil {
		t.Fatalf("source was removed for a non-ENOTSUP rename failure: %v", err)
	}
}
