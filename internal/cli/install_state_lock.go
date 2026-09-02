package cli

import "github.com/gentleman-programming/gentle-ai/v2/internal/statecoord"

func installStateLockPath(homeDir string) (string, error) {
	return statecoord.LockPath(homeDir)
}

func withInstallStateLock(homeDir string, operation func() error) error {
	return statecoord.WithLock(homeDir, operation)
}
