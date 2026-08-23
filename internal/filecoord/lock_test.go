package filecoord

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func mustPath(t *testing.T, root, target string) string {
	t.Helper()
	path, err := LockPath(root, target)
	if err != nil {
		t.Fatal(err)
	}
	return path
}
func assertError(t *testing.T, err, sentinel error, typed any) {
	if !errors.Is(err, sentinel) || !errors.As(err, typed) {
		t.Fatalf("error = %v, want %v and %T", err, sentinel, typed)
	}
}
func TestLockPathHashesCleanAbsoluteTarget(t *testing.T) {
	root := filepath.Join(t.TempDir(), "locks", "..", "locks")
	base := t.TempDir()
	messy := filepath.Join(base, "nested", "..", "target.txt")
	if got, want := mustPath(t, root, messy), mustPath(t, root, filepath.Join(base, "target.txt")); got != want {
		t.Fatalf("clean paths differ: %q != %q", got, want)
	}
	relative := filepath.Join("privacy-marker", "..", "target.txt")
	got := mustPath(t, root, relative)
	absolute, err := filepath.Abs(filepath.Clean(relative))
	if err != nil {
		t.Fatal(err)
	}
	key := sha256.Sum256([]byte(absolute))
	want := filepath.Join(filepath.Clean(root), hex.EncodeToString(key[:])+".lock")
	if got != want || len(filepath.Base(got))-len(".lock") != sha256.Size*2 {
		t.Fatalf("hashed path = %q, want %q", got, want)
	}
	if strings.Contains(filepath.Base(got), "privacy-marker") || strings.Contains(filepath.Base(got), "target.txt") {
		t.Fatalf("target leaked into lock filename: %q", got)
	}
	if _, err := os.Stat(filepath.Clean(root)); !os.IsNotExist(err) {
		t.Fatalf("LockPath changed the root: %v", err)
	}
}

func TestLockPathRejectsInvalidRootAndTarget(t *testing.T) {
	root := filepath.Join(t.TempDir(), "locks")
	for _, test := range []struct {
		name, root, target string
		want               error
		typed              any
	}{
		{"empty root", "", "/target", ErrInvalidRoot, new(*InvalidRootError)},
		{"relative root", "locks", "/target", ErrInvalidRoot, new(*InvalidRootError)},
		{"empty target", root, "", ErrInvalidTarget, new(*InvalidTargetError)},
		{"NUL target", root, "bad\x00target", ErrInvalidTarget, new(*InvalidTargetError)},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, err := LockPath(test.root, test.target)
			assertError(t, err, test.want, test.typed)
		})
	}
}

func TestErrorsExposeTypedTaxonomy(t *testing.T) {
	cause := errors.New("cause")
	for _, test := range []struct {
		err, want error
		typed     any
	}{
		{&BusyError{Cause: cause}, ErrBusy, new(*BusyError)},
		{&UnsupportedError{}, ErrUnsupported, new(*UnsupportedError)},
		{&InvalidTargetError{}, ErrInvalidTarget, new(*InvalidTargetError)},
		{&InvalidRootError{}, ErrInvalidRoot, new(*InvalidRootError)},
		{&OperationalError{Cause: cause}, ErrOperational, new(*OperationalError)},
	} {
		assertError(t, test.err, test.want, test.typed)
	}
}

func TestZeroCauseErrorsDoNotUnwrapNilChildren(t *testing.T) {
	for _, test := range []struct {
		name     string
		err      error
		sentinel error
	}{
		{name: "busy", err: &BusyError{}, sentinel: ErrBusy},
		{name: "operational", err: &OperationalError{}, sentinel: ErrOperational},
	} {
		t.Run(test.name, func(t *testing.T) {
			if !errors.Is(test.err, test.sentinel) {
				t.Fatalf("errors.Is(%T, %v) = false", test.err, test.sentinel)
			}
			children := test.err.(interface{ Unwrap() []error }).Unwrap()
			for i, child := range children {
				if child == nil {
					t.Fatalf("Unwrap()[%d] = nil", i)
				}
			}
		})
	}
}

func TestLeaseReleaseIsConcurrentIdempotentAndOperational(t *testing.T) {
	unlockErr, closeErr := errors.New("unlock failed"), errors.New("close failed")
	var mu sync.Mutex
	unlockCalls, closeCalls := 0, 0
	count := func(n *int, err error) func() error {
		return func() error { mu.Lock(); (*n)++; mu.Unlock(); return err }
	}
	unlock, close := count(&unlockCalls, unlockErr), count(&closeCalls, closeErr)
	lease := &Lease{file: &os.File{}, unlock: func(*os.File) error { return unlock() }, close: close}
	bad := false
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			err := lease.Release()
			if !errors.Is(err, ErrOperational) || !errors.Is(err, unlockErr) || !errors.Is(err, closeErr) {
				mu.Lock()
				bad = true
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	mu.Lock()
	defer mu.Unlock()
	if bad || unlockCalls != 1 || closeCalls != 1 {
		t.Fatalf("release state = bad:%v unlock:%d close:%d", bad, unlockCalls, closeCalls)
	}
}
