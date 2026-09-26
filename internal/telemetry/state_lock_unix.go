//go:build unix

package telemetry

import (
	"os"
	"syscall"
)

// lockFileExclusivePlatform blocks for an exclusive flock on the open file
// description, used only for cross-process exclusion. lockState installs it
// as lockFileExclusive; see there.
func lockFileExclusivePlatform(f *os.File) error {
	return syscall.Flock(int(f.Fd()), syscall.LOCK_EX)
}

func unlockFile(f *os.File) error {
	return syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
}
