package telemetry

import (
	"os"
	"path/filepath"
	"sync"
)

// lockFileExclusive is a package-level var so tests can sabotage the
// cross-process file lock and prove same-process exclusion survives it
// (injectable-seam precedent: loadCompactRecoveryRecords in
// internal/reviewtransaction/compact_inspect.go). The real implementation
// is the platform function in the build-tagged state_lock_{unix,windows}.go
// files; swapping it never changes same-process exclusion, which the path
// mutex below carries.
var lockFileExclusive = lockFileExclusivePlatform

func lockFilePath(homeDir string) string {
	return Path(homeDir) + ".lock"
}

// statePathLocks carries same-process exclusion for lockState. Entries are
// never deleted, so the map is bounded by the number of distinct home
// directories the process ever locks — effectively one per test or one per
// resolved user home.
var (
	statePathLocksMu sync.Mutex
	statePathLocks   = map[string]*sync.Mutex{}
)

func statePathLock(key string) *sync.Mutex {
	statePathLocksMu.Lock()
	defer statePathLocksMu.Unlock()
	mu, ok := statePathLocks[key]
	if !ok {
		mu = &sync.Mutex{}
		statePathLocks[key] = mu
	}
	return mu
}

// lockState acquires an exclusive, blocking lock guarding the state file's
// read-modify-write cycle. Exclusion is a hybrid: same-process callers are
// serialized by the path-keyed mutex (keyed on the absolute lock file path),
// while cross-process callers are serialized by the OS file lock (flock on
// unix, LockFileEx on windows). Same-process exclusion therefore no longer
// depends on byte-range lock semantics at all: on Windows, LockFileEx with
// separate handles from one process intermittently fails to overlap ranges,
// which lost updates (ga-4882); with the mutex carrying same-process
// exclusion, that failure mode is unreachable by construction. The mutex is
// not re-entrant: never call lockState again while holding it (Update →
// EnsureState must stay lock-free, see EnsureState). The returned func
// releases the file lock first, then closes the handle, then releases the
// path mutex; call it exactly once.
func lockState(homeDir string) (func(), error) {
	// Abs normalizes the key so different spellings of one home resolve to
	// one mutex; a failed Getwd falls back to the raw path, which only
	// matters for relative homes whose cwd changes mid-flight.
	key, err := filepath.Abs(lockFilePath(homeDir))
	if err != nil {
		key = lockFilePath(homeDir)
	}
	mu := statePathLock(key)
	mu.Lock()

	dir := filepath.Join(homeDir, stateDir)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		mu.Unlock()
		return nil, err
	}
	f, err := os.OpenFile(lockFilePath(homeDir), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		mu.Unlock()
		return nil, err
	}
	if err := lockFileExclusive(f); err != nil {
		_ = f.Close()
		mu.Unlock()
		return nil, err
	}
	return func() {
		_ = unlockFile(f)
		_ = f.Close()
		mu.Unlock()
	}, nil
}

// Update locks, loads (creating as EnsureState would), mutates, and saves.
// Every state writer must go through this to avoid lost updates.
func Update(homeDir string, mutate func(*State)) error {
	unlock, err := lockState(homeDir)
	if err != nil {
		return err
	}
	defer unlock()
	s, err := EnsureState(homeDir)
	if err != nil {
		return err
	}
	mutate(&s)
	return Save(homeDir, s)
}
