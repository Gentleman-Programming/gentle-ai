package cli

import "github.com/gentleman-programming/gentle-ai/v3/internal/statecoord"

func withInstallStateLock(homeDir string, operation func() error) error {
	return statecoord.WithLock(homeDir, operation)
}
