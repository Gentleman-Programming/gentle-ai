package filecoord

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
	"time"
)

// lockedBuffer collects subprocess output that may still be written while a
// failure path reads it back for diagnostics.
type lockedBuffer struct {
	mu     sync.Mutex
	buffer bytes.Buffer
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buffer.Write(p)
}

func (b *lockedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buffer.String()
}

// canonicalBackendTempDir resolves the test temporary directory through
// EvalSymlinks: the secure open walk rejects symlinked path components by
// design (macOS /tmp and /var are symlinks), so lock roots used in tests must
// be canonical, exactly as production callers must pass canonical roots.
func canonicalBackendTempDir(t *testing.T) string {
	t.Helper()
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestAcquireGrantsExclusiveLeaseUntilRelease(t *testing.T) {
	base := canonicalBackendTempDir(t)
	root := filepath.Join(base, "locks")
	target := filepath.Join(base, "target.txt")

	lease, err := Acquire(context.Background(), target, root)
	if err != nil || lease == nil {
		t.Fatalf("Acquire() = %v, %v; want a live lease", lease, err)
	}
	lockPath := mustPath(t, root, target)
	if _, statErr := os.Lstat(lockPath); statErr != nil {
		t.Fatalf("acquired lease has no lock file at %s: %v", lockPath, statErr)
	}

	_, busyErr := Acquire(context.Background(), target, root)
	if !errors.Is(busyErr, ErrBusy) || !errors.As(busyErr, new(*BusyError)) {
		t.Fatalf("second Acquire() error = %v, want BusyError", busyErr)
	}

	if releaseErr := lease.Release(); releaseErr != nil {
		t.Fatalf("Release() = %v, want nil", releaseErr)
	}
	if repeated := lease.Release(); repeated != nil {
		t.Fatalf("repeated Release() = %v, want idempotent nil", repeated)
	}

	reacquired, err := Acquire(context.Background(), target, root)
	if err != nil || reacquired == nil {
		t.Fatalf("Acquire() after release = %v, %v; want a live lease", reacquired, err)
	}
	if err := reacquired.Release(); err != nil {
		t.Fatalf("Release() of reacquired lease = %v", err)
	}
}

func TestAcquireDistinguishesTargetsUnderOneRoot(t *testing.T) {
	base := canonicalBackendTempDir(t)
	root := filepath.Join(base, "locks")

	first, err := Acquire(context.Background(), filepath.Join(base, "one.txt"), root)
	if err != nil {
		t.Fatalf("Acquire(one) = %v", err)
	}
	defer func() { _ = first.Release() }()

	second, err := Acquire(context.Background(), filepath.Join(base, "two.txt"), root)
	if err != nil {
		t.Fatalf("Acquire(two) under the same root = %v, want independent lease", err)
	}
	if err := second.Release(); err != nil {
		t.Fatalf("Release(two) = %v", err)
	}
}

func TestAcquireHonorsCancelledContextWithoutFilesystemWork(t *testing.T) {
	base := canonicalBackendTempDir(t)
	root := filepath.Join(base, "never-created-locks")
	target := filepath.Join(base, "target.txt")

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	lease, err := Acquire(ctx, target, root)
	if lease != nil {
		t.Fatal("cancelled Acquire returned a lease")
	}
	if !errors.Is(err, ErrOperational) || !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled Acquire error = %v, want operational with context cause", err)
	}
	if _, statErr := os.Lstat(root); !os.IsNotExist(statErr) {
		t.Fatalf("cancelled Acquire touched the lock root: %v", statErr)
	}
}

func TestAcquireRejectsSymlinkedLockPath(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink fixtures are unavailable on Windows runners")
	}
	base := canonicalBackendTempDir(t)
	root := filepath.Join(base, "locks")
	target := filepath.Join(base, "target.txt")
	if err := os.MkdirAll(root, 0o700); err != nil {
		t.Fatal(err)
	}
	victim := filepath.Join(base, "victim.txt")
	if err := os.WriteFile(victim, []byte("victim bytes\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	lockPath := mustPath(t, root, target)
	if err := os.Symlink(victim, lockPath); err != nil {
		t.Skipf("symlink fixture is unavailable: %v", err)
	}

	lease, err := Acquire(context.Background(), target, root)
	if lease != nil || !errors.Is(err, ErrOperational) {
		t.Fatalf("Acquire() through a symlinked lock path = %v, %v; want operational refusal", lease, err)
	}
	if info, statErr := os.Lstat(lockPath); statErr != nil || info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("symlinked lock path was not preserved: %v, %v", info, statErr)
	}
	data, readErr := os.ReadFile(victim)
	if readErr != nil || string(data) != "victim bytes\n" {
		t.Fatalf("symlink target mutated: %q, %v", data, readErr)
	}
}

func TestAcquireRejectsSymlinkedRootComponent(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink fixtures are unavailable on Windows runners")
	}
	base := canonicalBackendTempDir(t)
	realRoot := filepath.Join(base, "real-locks")
	if err := os.MkdirAll(realRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	linkedRoot := filepath.Join(base, "linked-locks")
	if err := os.Symlink(realRoot, linkedRoot); err != nil {
		t.Skipf("symlink fixture is unavailable: %v", err)
	}

	lease, err := Acquire(context.Background(), filepath.Join(base, "target.txt"), linkedRoot)
	if lease != nil || !errors.Is(err, ErrOperational) {
		t.Fatalf("Acquire() through a symlinked root = %v, %v; want operational refusal", lease, err)
	}
}

const (
	filecoordHolderEnv        = "GENTLE_AI_TEST_FILECOORD_HOLDER"
	filecoordHolderTargetEnv  = "GENTLE_AI_TEST_FILECOORD_TARGET"
	filecoordHolderRootEnv    = "GENTLE_AI_TEST_FILECOORD_ROOT"
	filecoordHolderReadyEnv   = "GENTLE_AI_TEST_FILECOORD_READY"
	filecoordHolderReleaseEnv = "GENTLE_AI_TEST_FILECOORD_RELEASE"
)

// TestFilecoordLockHolderHelperProcess is not a test: it is the subprocess
// body for the cross-process contention proof below, gated by environment.
func TestFilecoordLockHolderHelperProcess(t *testing.T) {
	if os.Getenv(filecoordHolderEnv) != "1" {
		return
	}
	lease, err := Acquire(context.Background(), os.Getenv(filecoordHolderTargetEnv), os.Getenv(filecoordHolderRootEnv))
	if err != nil {
		t.Fatalf("helper Acquire() = %v", err)
	}
	if err := os.WriteFile(os.Getenv(filecoordHolderReadyEnv), []byte("ready\n"), 0o600); err != nil {
		t.Fatalf("helper ready signal: %v", err)
	}
	release := os.Getenv(filecoordHolderReleaseEnv)
	deadline := time.Now().Add(20 * time.Second)
	for {
		if _, err := os.Stat(release); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("helper timed out waiting for the release signal")
		}
		time.Sleep(10 * time.Millisecond)
	}
	if err := lease.Release(); err != nil {
		t.Fatalf("helper Release() = %v", err)
	}
}

func TestAcquireContendsAcrossRealProcesses(t *testing.T) {
	if testing.Short() {
		t.Skip("cross-process contention is not run in short mode")
	}
	base := canonicalBackendTempDir(t)
	root := filepath.Join(base, "locks")
	target := filepath.Join(base, "target.txt")
	ready := filepath.Join(base, "holder.ready")
	release := filepath.Join(base, "holder.release")

	holder := exec.Command(os.Args[0], "-test.run=^TestFilecoordLockHolderHelperProcess$", "-test.v")
	holder.Env = append(os.Environ(),
		filecoordHolderEnv+"=1",
		filecoordHolderTargetEnv+"="+target,
		filecoordHolderRootEnv+"="+root,
		filecoordHolderReadyEnv+"="+ready,
		filecoordHolderReleaseEnv+"="+release,
	)
	output := &lockedBuffer{}
	holder.Stdout, holder.Stderr = output, output
	if err := holder.Start(); err != nil {
		t.Fatalf("start holder process: %v", err)
	}
	holderDone := make(chan error, 1)
	go func() { holderDone <- holder.Wait() }()
	holderConsumed := false
	defer func() {
		_ = os.WriteFile(release, []byte("release\n"), 0o600)
		if holderConsumed {
			return
		}
		select {
		case <-holderDone:
		case <-time.After(20 * time.Second):
			_ = holder.Process.Kill()
		}
	}()

	deadline := time.Now().Add(20 * time.Second)
	for {
		if _, err := os.Stat(ready); err == nil {
			break
		}
		select {
		case err := <-holderDone:
			holderConsumed = true
			t.Fatalf("holder exited before signalling ready: %v\n%s", err, output.String())
		default:
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for the holder to acquire\n%s", output.String())
		}
		time.Sleep(10 * time.Millisecond)
	}

	_, err := Acquire(context.Background(), target, root)
	if !errors.Is(err, ErrBusy) {
		t.Fatalf("Acquire() against a live external holder = %v, want ErrBusy", err)
	}

	if err := os.WriteFile(release, []byte("release\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	holderErr := <-holderDone
	holderConsumed = true
	if holderErr != nil {
		t.Fatalf("holder process failed: %v\n%s", holderErr, output.String())
	}

	lease, err := Acquire(context.Background(), target, root)
	if err != nil || lease == nil {
		t.Fatalf("Acquire() after the holder exited = %v, %v; want a live lease", lease, err)
	}
	if err := lease.Release(); err != nil {
		t.Fatalf("Release() = %v", err)
	}
}
