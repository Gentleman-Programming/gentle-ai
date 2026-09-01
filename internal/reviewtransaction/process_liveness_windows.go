//go:build windows

package reviewtransaction

import (
	"errors"

	"golang.org/x/sys/windows"
)

// processVerifiedDead reports whether pid verifiably does not name a running
// process on this host. Only a definitive answer counts: access refusals and
// every other ambiguity report false, so stale-lock removal guidance is only
// ever derived from proof (issue #3342).
func processVerifiedDead(pid int) bool {
	if pid <= 0 {
		return false
	}
	handle, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid))
	if err != nil {
		// OpenProcess answers ERROR_INVALID_PARAMETER for a pid that names no
		// process at all; anything else (access denied, transient fault) is
		// not proof of death.
		return errors.Is(err, windows.ERROR_INVALID_PARAMETER)
	}
	defer windows.CloseHandle(handle)
	// STILL_ACTIVE (259): GetExitCodeProcess answers it for a running
	// process; any real exit code proves the recorded holder exited.
	const stillActive = 259
	var code uint32
	if err := windows.GetExitCodeProcess(handle, &code); err != nil {
		return false
	}
	return code != stillActive
}
