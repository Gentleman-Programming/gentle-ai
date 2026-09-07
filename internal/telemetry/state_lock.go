package telemetry

import (
	"os"
	"path/filepath"
)

func lockFilePath(homeDir string) string {
	return Path(homeDir) + ".lock"
}

// lockState acquires an exclusive, cross-process, blocking lock guarding the
// state file's read-modify-write cycle. Every call opens its own fd/handle,
// so same-process callers correctly block on each other too. The returned
// func releases the lock; call it exactly once.
func lockState(homeDir string) (func(), error) {
	dir := filepath.Join(homeDir, stateDir)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(lockFilePath(homeDir), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	if err := lockFileExclusive(f); err != nil {
		_ = f.Close()
		return nil, err
	}
	return func() {
		_ = unlockFile(f)
		_ = f.Close()
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
